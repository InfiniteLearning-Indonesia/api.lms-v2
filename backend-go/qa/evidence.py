"""Deterministic result aggregation; no test/evidence means no completion."""
from collections import Counter
from hashlib import sha256
from pathlib import Path
import json
import os
import uuid


KNOWN = {"PASS", "FAIL", "ERROR", "SKIPPED", "BLOCKED", "NOT_RUN", "INTERRUPTED", "FLAKY"}


def verify_go_cases(expected: list[str], cases: list[dict]) -> bool:
    if not expected or len(expected) != len(set(expected)):
        return False
    actual = [case["package"] + ":" + case["name"] for case in cases]
    return (len(actual) == len(set(actual)) and set(actual) == set(expected)
            and all(case["status"] == "pass" for case in cases))


def summarize(manifest: dict, cases: list[dict], source_hash: str, tested_hash: str,
              command_ok: bool) -> dict:
    expected = manifest["expected_checks"]
    if not expected or len(expected) != len(set(expected)):
        raise ValueError("nonempty unique expected checks required")
    deliverables = manifest["deliverables"]
    if not deliverables or len({item["id"] for item in deliverables}) != len(deliverables):
        raise ValueError("nonempty unique deliverables required")
    if any(set(item["checks"]) - set(expected) for item in deliverables):
        raise ValueError("deliverable references undeclared check")
    ids = [case["id"] for case in cases]
    if len(ids) != len(set(ids)):
        raise ValueError("duplicate test result IDs")
    if any(case["status"] not in KNOWN for case in cases):
        raise ValueError("unknown result status")
    results = {case["id"]: case["status"] for case in cases}
    unexpected = sorted(set(results) - set(expected))
    missing = sorted(set(expected) - set(results))
    current = source_hash == tested_hash and bool(source_hash)
    tests_pass = (command_ok and current and not unexpected and not missing
                  and all(results.get(case) == "PASS" for case in expected))
    verified = []
    pending = {}
    for item in manifest["deliverables"]:
        requirements = item["checks"]
        reasons = list(item.get("pending_evidence", []))
        if not requirements:
            reasons.append("NO_EXECUTABLE_CRITERIA")
        if any(results.get(case) != "PASS" for case in requirements):
            reasons.append("REQUIRED_CHECK_NOT_PASS")
        if not tests_pass:
            reasons.append("RUN_OR_REGRESSION_NOT_PASS")
        if reasons:
            pending[item["id"]] = sorted(set(reasons))
        else:
            verified.append(item["id"])
    all_gates = bool(manifest.get("required_gates")) and all(
        gate.get("status") == "PASS" for gate in manifest["required_gates"]
    )
    complete = (tests_pass and all_gates and manifest.get("full_suite_implemented") is True
                and len(verified) == len(manifest["deliverables"]))
    total = len(manifest["deliverables"])
    return {
        "automated_checks": "PASS" if tests_pass else "FAIL",
        "package_status": "BACKEND_VERIFIED" if complete else "BLOCKED",
        "evidence_currency": "CURRENT" if current else "STALE",
        "counts": dict(Counter(results.get(case, "NOT_RUN") for case in expected)),
        "expected_check_count": len(expected),
        "missing_check_ids": missing,
        "unexpected_check_ids": unexpected,
        "verified_deliverables": verified,
        "pending_deliverables": pending,
        "deliverable_coverage": {"verified": len(verified), "total": total,
                                 "percent": 100 * len(verified) / total if total else None},
        # Bootstrap checks are not the entire future package acceptance suite.
        "full_package_test_pass_rate": None if not manifest.get("full_suite_implemented") else
            100 * sum(results.get(case) == "PASS" for case in expected) / len(expected),
        "release_status": "NOT_VERIFIED",
    }


def source_fingerprint(root: Path) -> str:
    """Exclude generated evidence/docs but include every executable source input."""
    files = []
    for directory, directories, names in os.walk(root):
        directories[:] = [name for name in directories if not name.startswith(".") and name != "__pycache__"]
        for name in names:
            p = Path(directory) / name
            if p.suffix in {".go", ".py", ".json", ".lock", ".sql", ".yaml", ".yml", ".toml", ".sh", ".ps1"} or p.name in {"go.mod", "go.sum", "pytest.ini", "Dockerfile"}:
                files.append(p)
    # User-supplied root .env.example is now private, not executable source.
    files += [p for p in (root / ".gitignore",) if p.is_file()]
    files += list((root / "docs" / "examples").glob("*.example"))
    digest = sha256()
    # Path ordering differs between Windows (case-insensitive) and Linux.
    # Canonical POSIX strings keep the same tree hash across QA environments.
    for path in sorted(files, key=lambda p: p.relative_to(root).as_posix()):
        digest.update(path.relative_to(root).as_posix().encode())
        digest.update(b"\0")
        digest.update(path.read_bytes())
        digest.update(b"\0")
    # GitHub discovers workflows at repository root, outside backend-go.
    workflow = root.parent / ".github" / "workflows" / "backend-go-qa.yml"
    if workflow.is_file():
        digest.update(b"repository-workflow\0")
        digest.update(workflow.read_bytes())
    return digest.hexdigest()


def write_new_json(path: Path, value: dict) -> None:
    # Exclusive create preserves any existing run evidence instead of overwrite.
    with path.open("x", encoding="utf-8") as handle:
        json.dump(value, handle, indent=2, sort_keys=True)
        handle.write("\n")


def render_section(existing: str, package: str, result: dict, run_id: str) -> str:
    start, end = f"<!-- QA:{package}:START -->", f"<!-- QA:{package}:END -->"
    cov = result["deliverable_coverage"]
    section = (f"{start}\n## {package}: latest automated checkpoint\n\n"
               f"- Run: `{run_id}`; automated checks: **{result['automated_checks']}**.\n"
               f"- Package: **{result['package_status']}**; evidence: `{result['evidence_currency']}`.\n"
               f"- Verified deliverables: **{cov['verified']}/{cov['total']} ({cov['percent']:.0f}%)**.\n"
               f"- Bootstrap checks: {result['counts'].get('PASS', 0)}/{result['expected_check_count']} PASS; other outcomes: `{json.dumps({k: v for k, v in result['counts'].items() if k != 'PASS'}, sort_keys=True)}`.\n"
               f"- Full-package test pass rate: N/A while acceptance suite is incomplete.\n"
               f"- Release/migration/deployment: NOT_VERIFIED / NOT_EXECUTED / NOT_DEPLOYED.\n"
               f"- This is bootstrap evidence, not a completed module. See private run manifest for blockers.\n{end}")
    if "operational_observation" in result:
        obs = result["operational_observation"]
        section = section.replace(end, f"- Separate operator evidence: `{obs['status']}`; TLS `{obs['tls']}`; public catalog `{obs['public_catalog']}`. Production identity, least privilege, full inventory and backup remain PENDING; no mutation authority.\n{end}")
    if existing.count(start) != existing.count(end) or existing.count(start) > 1:
        raise ValueError("ambiguous generated section")
    if start not in existing:
        return existing.rstrip() + "\n\n" + section + "\n"
    first, last = existing.index(start), existing.index(end) + len(end)
    if first > existing.index(end):
        raise ValueError("invalid generated section order")
    return existing[:first] + section + existing[last:]


def replace_if_unchanged(path: Path, original: str, updated: str) -> None:
    """Atomic per file; refuse to overwrite a concurrent human edit."""
    if path.is_symlink():
        raise ValueError("report destination cannot be a symlink")
    temporary = path.with_name(path.name + ".qa-" + uuid.uuid4().hex + ".tmp")
    with temporary.open("x", encoding="utf-8", newline="\n") as handle:
        handle.write(updated)
        handle.flush()
        os.fsync(handle.fileno())
    if path.read_text(encoding="utf-8") != original:
        # Preserve the temporary candidate for review after conflict; no cleanup.
        raise ValueError("documentation changed during report generation")
    os.replace(temporary, path)


def publish_checkpoint(root: Path, package: str, result: dict) -> dict:
    """Update only generated sections; caller holds the environment lock.

    Local reports are reproducible diagnostics, not signed operator receipts.
    There is no multi-file filesystem transaction: a missing publication receipt
    or different run IDs between files means incomplete publication, not PASS.
    """
    if package not in {"M00", "M03"} or result["package"] != package:
        raise ValueError("unsupported report package")
    if result["source_hash"] != source_fingerprint(root):
        raise ValueError("stale checkpoint cannot update documentation")
    docs = (root / "docs").resolve(strict=True)
    paths = [docs / "modules" / f"{package}.md", docs / "IMPLEMENTATION_STATUS.md"]
    drafts = []
    for path in paths:
        if not path.resolve(strict=True).is_relative_to(docs):
            raise ValueError("documentation destination escaped scope")
        original = path.read_text(encoding="utf-8")
        drafts.append((path, original, render_section(original, package, result, result["run_id"])))
    for path, original, updated in drafts:
        replace_if_unchanged(path, original, updated)
    return {"run_id": result["run_id"], "publication": "COMPLETE",
            "document_hashes_at_publication": {p.relative_to(root).as_posix(): sha256(p.read_bytes()).hexdigest() for p in paths}}
