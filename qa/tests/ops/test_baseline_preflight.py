"""Actual Go CLI, synthetic declarations, no database or encryption invocation."""
from datetime import datetime, timezone
import json
import os
from pathlib import Path
import subprocess

import pytest

from qa.run import ROOT, clean_environment


def request(root):
    return {"version": 1, "run_id": "cli-fixture-candidate", "artifact_root": str(root),
            "source": {"identity_sha256": "a" * 64, "schema_sha256": "b" * 64,
                       "deployed_commit": "c" * 40, "receipt_ref": "synthetic-not-verified",
                       "observed_at": datetime.now(timezone.utc).isoformat()},
            "target": {"identity_sha256": "d" * 64, "kind": "new-local-restore", "namespace": "fixture"},
            "schemas": ["public"],
            "declarations": {"tls_mode": "verify-full", "source_read_only": True, "access_reviewed": True,
                             "owner_ref": "fixture-owner", "reviewer_ref": "fixture-reviewer",
                             "approval_ref": "fixture-not-approval", "key_custody_ref": "fixture-not-key",
                             "artifact_scope_ref": "fixture-no-objects"},
            "capacity": {"dump_bytes": 1024, "sql_bytes": 1024, "object_bytes": 0,
                         "restore_bytes": 1024, "reserve_bytes": 8 * 1024 ** 3}}


def invoke(raw, *args):
    artifacts = Path(os.environ["QA_ARTIFACT_ROOT"]).resolve(strict=True)
    binary = Path(os.environ["QA_BASELINE_PREFLIGHT_BINARY"]).resolve(strict=True)
    assert binary.is_relative_to(artifacts), "CLI must be built for this run"
    env = clean_environment(ROOT, os.environ)
    # CLI must not interpret a DB connection environment variable as authority.
    env["DATABASE_URL"] = "postgresql://synthetic.invalid/DO_NOT_CONNECT"
    return subprocess.run([str(binary), *args], input=raw, env=env, capture_output=True,
                          text=True, timeout=10)


@pytest.mark.case("M01-intake-cli-observed-capacity")
def test_actual_offline_cli_observes_disk_without_authorizing_or_creating_candidate(qa_scratch):
    payload = request(qa_scratch)
    result = invoke(json.dumps(payload), "--plan-only")
    value = json.loads(result.stdout)
    # This real CLI check requires real headroom; no fake capacity override.
    assert result.returncode == 0, value["blockers"]
    assert value["status"] == "OFFLINE_PREFLIGHT_VALID_NOT_AUTHORIZED"
    assert value["observed_free_bytes"] >= value["required_bytes"] >= 8 * 1024 ** 3
    assert value["filesystem_checked"] is True and value["external_gates"]
    assert value["authorizes_execution"] is value["real_baseline_verified"] is False
    assert value["database_access"] == "NONE" and not list(qa_scratch.iterdir())
    assert "fixture-not-key" not in result.stdout and "fixture-owner" not in result.stdout


@pytest.mark.case("M01-intake-cli-strict-redaction")
def test_actual_cli_rejects_ambiguous_input_flags_and_secret_fields(qa_scratch):
    payload = json.dumps(request(qa_scratch))
    for raw in (payload + "{}", '{"version":1,"version":1}', "null", "x" * 65537,
                '{"version":1,"password":"PRIVATE_FIXTURE_SECRET"}'):
        result = invoke(raw, "--plan-only")
        assert result.returncode == 2 and not result.stdout
        assert result.stderr.strip() == "BASELINE_PREFLIGHT_INVALID_INPUT"
        assert "PRIVATE_FIXTURE_SECRET" not in result.stderr
    for flags in ((), ("--apply",), ("--plan-only", "--restore")):
        result = invoke(payload, *flags)
        assert result.returncode == 2 and not result.stdout
    assert not list(qa_scratch.iterdir())


@pytest.mark.case("M01-intake-cli-no-overwrite")
def test_actual_cli_refuses_existing_target_same_identity_and_insufficient_space(qa_scratch):
    payload = request(qa_scratch)
    existing = qa_scratch / payload["run_id"]
    existing.mkdir()
    sentinel = existing / "preserve.txt"
    sentinel.write_text("synthetic preserved pre-state")
    result = invoke(json.dumps(payload), "--plan-only")
    assert result.returncode == 2 and "CANDIDATE_EXISTS_OR_UNAVAILABLE" in json.loads(result.stdout)["blockers"]
    payload["run_id"] = "different-fixture-candidate"
    payload["target"]["identity_sha256"] = payload["source"]["identity_sha256"]
    result = invoke(json.dumps(payload), "--plan-only")
    assert result.returncode == 2 and not json.loads(result.stdout)["filesystem_checked"]
    payload["target"]["identity_sha256"] = "d" * 64
    payload["capacity"]["dump_bytes"] = 2 ** 63
    result = invoke(json.dumps(payload), "--plan-only")
    assert result.returncode == 2 and "FILESYSTEM_CAPACITY_INSUFFICIENT" in json.loads(result.stdout)["blockers"]
    assert sentinel.read_text() == "synthetic preserved pre-state"
    assert sorted(p.name for p in qa_scratch.iterdir()) == [existing.name]
