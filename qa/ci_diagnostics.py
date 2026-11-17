"""Bounded public diagnostics: allowlisted identifiers, never command output."""
import json
import re

from qa.evidence import write_new_json

COMMANDS = frozenset({"go-version", "go-root", "go-format", "go-build-api", "go-build-worker",
                      "go-build-baseline", "go-build-db-inspect", "go-build-backup-plan",
                      "go-unit", "go-vet", "go-coverage", "source-inventory", "pytest", "go-build-baseline-preflight", "go-build-baseline-package"})
STATUSES = frozenset({"PASS", "FAIL", "ERROR", "SKIPPED", "BLOCKED", "NOT_RUN", "INTERRUPTED", "FLAKY"})
MAX_BYTES = 64 * 1024


def identifier(value):
    return isinstance(value, str) and re.fullmatch(r"[A-Za-z0-9_:/.-]{1,192}", value) is not None


def digest(value):
    return value if isinstance(value, str) and re.fullmatch(r"[a-f0-9]{64}", value) else None


def build_diagnostic(result, manifest, go_manifest, go_cases):
    """Input is private/raw evidence. Emit only the explicitly public projection."""
    expected = manifest["expected_checks"]
    go_expected = go_manifest["expected_tests"]
    if (not expected or len(expected) != len(set(expected)) or not all(map(identifier, expected))
            or not go_expected or len(go_expected) != len(set(go_expected))
            or not all(map(identifier, go_expected))):
        raise ValueError("INVALID_DIAGNOSTIC_REGISTRY")
    known = set(expected)
    go_known = set(go_expected)
    cases = result.get("cases", [])
    if (not isinstance(cases, list) or not isinstance(go_cases, list)
            or any(not isinstance(c, dict) or not isinstance(c.get("id"), str) for c in cases)
            or any(not isinstance(c, dict) or not isinstance(c.get("package"), str)
                   or not isinstance(c.get("name"), str) for c in go_cases)):
        raise ValueError("INVALID_DIAGNOSTIC_CASES")
    observed = [c.get("id") for c in cases if isinstance(c, dict)]
    go_ids = [str(c.get("package", "")) + ":" + str(c.get("name", ""))
              for c in go_cases if isinstance(c, dict)]
    commands = []
    for command in result.get("commands", []):
        if not isinstance(command, dict) or command.get("name") not in COMMANDS:
            continue
        code = command.get("return_code")
        commands.append({"name": command["name"],
                         "return_code": code if type(code) is int and -2147483648 <= code <= 2147483647 else None,
                         "status": command.get("status") if command.get("status") in
                         {"FINISHED", "TIMEOUT", "COMMAND_UNAVAILABLE"} else "UNKNOWN"})
    passed = (result.get("automated_checks") == "PASS"
              and set(observed) == known and len(observed) == len(known)
              and all(isinstance(c, dict) and c.get("status") == "PASS" for c in cases)
              and set(go_ids) == go_known and len(go_ids) == len(go_known)
              and all(c.get("status") == "pass" for c in go_cases)
              and digest(result.get("source_hash")) is not None
              and digest(result.get("manifest_sha256")) is not None
              and result.get("source_hash") == result.get("tested_source_hash"))
    report = {
        "schema_version": 1, "scope": "BOOTSTRAP_DIAGNOSTICS_ONLY",
        "status": "PASS_BOOTSTRAP_ONLY" if passed else "FAIL_OR_INCOMPLETE",
        "source_hash": digest(result.get("source_hash")),
        "tested_source_hash": digest(result.get("tested_source_hash")),
        "manifest_sha256": digest(result.get("manifest_sha256")),
        "expected_check_count": len(known), "observed_check_count": len(observed),
        "missing_check_ids": sorted(known - set(i for i in observed if isinstance(i, str))),
        "failed_checks": sorted([{"id": c["id"], "status": c.get("status") if c.get("status") in
                                  STATUSES else "UNKNOWN"} for c in cases
                                 if isinstance(c, dict) and c.get("id") in known and c.get("status") != "PASS"],
                                key=lambda c: c["id"]),
        "unexpected_check_count": sum(not isinstance(i, str) or i not in known for i in observed),
        "duplicate_check_count": len(observed) - len(set(i for i in observed if isinstance(i, str))),
        "go_expected_count": len(go_known), "go_observed_count": len(go_ids),
        "go_missing_ids": sorted(go_known - set(go_ids)),
        "go_failed_ids": sorted(i for i, c in zip(go_ids, go_cases) if i in go_known and c.get("status") != "pass"),
        "go_unregistered_count": sum(i not in go_known for i in go_ids),
        "commands": commands,
        "raw_logs_included": False, "database_artifacts_included": False,
        "release_verified": False, "migration_authorized": False,
    }
    if len(json.dumps(report).encode("utf-8")) > MAX_BYTES:
        # Preserve failure, not an oversized or silently truncated diagnostic.
        raise ValueError("DIAGNOSTIC_SIZE_LIMIT")
    return report


def publish_diagnostic(destination, result, manifest, go_manifest, go_cases):
    report = build_diagnostic(result, manifest, go_manifest, go_cases)
    write_new_json(destination / "ci-diagnostic.json", report)
    return report
