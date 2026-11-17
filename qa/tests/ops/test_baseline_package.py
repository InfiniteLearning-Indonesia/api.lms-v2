"""Real Go offline validator/planner on tiny generated files; no DB process."""
from datetime import datetime, timezone
from hashlib import sha256
import json
import os
from pathlib import Path
import subprocess

import pytest

from qa.run import ROOT, clean_environment
from qa.tests.ops.test_baseline_preflight import request


def package(root):
    intake = request(root)
    snapshot = {"id": "fixture-snapshot", "source_identity_sha256": "a" * 64,
                "schema_sha256": "b" * 64, "exported_at": datetime.now(timezone.utc).isoformat(),
                "consistency_ref": "fixture-not-snapshot-proof", "dump_format": "postgres-custom",
                "server_major": 18, "dump_client_major": 18}
    catalog = {"version": 1, "snapshot_id": snapshot["id"], "source_identity_sha256": "a" * 64,
               "schema_sha256": "b" * 64, "schemas": ["public"],
               "tables": [{"schema": "public", "name": "users", "rows": 2}],
               "extensions": [{"name": "plpgsql", "version": "1.0"}], "complete_ref": "fixture-only-claim"}
    files = {"version": 1, "snapshot_id": snapshot["id"], "source_identity_sha256": "a" * 64,
             "objects": [], "bundle_sha256": "", "no_objects_review_ref": "fixture-no-files-review"}
    artifacts = []
    for role, name, data in [("db_custom", "database.dump", b"PGDMP synthetic; not restoreable"),
                             ("db_sql", "database.sql", b"-- synthetic; never execute\n"),
                             ("schema_inventory", "schema.json", json.dumps(catalog).encode()),
                             ("file_inventory", "files.json", json.dumps(files).encode())]:
        (root / name).write_bytes(data)
        artifacts.append({"role": role, "name": name, "snapshot_id": snapshot["id"],
                          "bytes": len(data), "sha256": sha256(data).hexdigest(), "encryption": "none"})
    return {"version": 1, "intake": intake, "snapshot": snapshot, "artifacts": artifacts}


def invoke(value, *flags, raw=None):
    root = Path(os.environ["QA_ARTIFACT_ROOT"]).resolve(strict=True)
    binary = Path(os.environ["QA_BASELINE_PACKAGE_BINARY"]).resolve(strict=True)
    assert binary.is_relative_to(root), "binary must belong to this QA run"
    env = clean_environment(ROOT, os.environ)
    env["DATABASE_URL"] = "postgresql://synthetic.invalid/DO_NOT_CONNECT"
    return subprocess.run([str(binary), *flags], input=json.dumps(value) if raw is None else raw,
                          env=env, capture_output=True, text=True, timeout=15)


@pytest.mark.case("M01-package-cli-integrity-boundary")
def test_actual_validator_checks_all_artifacts_but_cannot_claim_restore_or_protection(qa_scratch):
    r = package(qa_scratch)
    before = {p.name: p.read_bytes() for p in qa_scratch.iterdir()}
    result = invoke(r, "--verify-only")
    report = json.loads(result.stdout)
    assert result.returncode == 0, report["blockers"]
    assert report["status"] == "OFFLINE_PACKAGE_INTEGRITY_VALID_NOT_AUTHORIZED"
    assert report["verified_artifact_count"] == 4 and report["declared_rows"] == 2
    assert report["authorizes_execution"] is report["actual_restore_verified"] is report["real_baseline_verified"] is report["archive_authentication_verified"] is False
    assert report["database_access"] == "NONE" and report["external_gates"]
    assert before == {p.name: p.read_bytes() for p in qa_scratch.iterdir()}
    assert "fixture-not-snapshot-proof" not in result.stdout


@pytest.mark.case("M01-package-cli-refusal-redaction")
def test_actual_validator_rejects_tampering_unknown_input_and_executable_flags(qa_scratch):
    r = package(qa_scratch)
    for raw in ('{"version":1,"password":"PRIVATE_FIXTURE_SECRET"}', json.dumps(r) + "{}",
                json.dumps(r).replace('"role":', '"Role":', 1)):
        result = invoke(r, "--verify-only", raw=raw)
        assert result.returncode == 2 and not result.stdout and result.stderr.strip() == "BASELINE_PACKAGE_INVALID_INPUT"
        assert "PRIVATE_FIXTURE_SECRET" not in result.stderr
    for flags in ((), ("--restore",), ("--verify-only", "--clean")):
        assert invoke(r, *flags).returncode == 2
    original = (qa_scratch / "database.dump").read_bytes()
    (qa_scratch / "database.dump").write_bytes(b"corrupt fixture")
    assert invoke(r, "--verify-only").returncode == 2
    (qa_scratch / "database.dump").write_bytes(original)
    (qa_scratch / "unlisted.key").write_text("PRIVATE_FIXTURE_ONLY")
    result = invoke(r, "--verify-only")
    assert result.returncode == 2 and "PACKAGE_DIRECTORY_SCOPE_MISMATCH" in json.loads(result.stdout)["blockers"]
    assert "PRIVATE_FIXTURE_ONLY" not in result.stdout and (qa_scratch / "unlisted.key").exists()


@pytest.mark.case("M01-package-cli-restore-plan-only")
def test_actual_restore_plan_requires_isolation_review_versions_and_valid_package(qa_scratch):
    r = {"version": 1, "package": package(qa_scratch),
         "restore": {"target_identity_sha256": "d" * 64, "database_name": "v3_restore_fixture",
                     "target_kind": "new-local-restore", "postgres_major": 18,
                     "available_extensions": [{"name": "plpgsql", "version": "1.0"}],
                     "network_disabled": True, "jobs_disabled": True, "executable_dump_reviewed": True,
                     "run_as_superuser": False, "review_ref": "fixture-not-real-review"}}
    before = {p.name: p.read_bytes() for p in qa_scratch.iterdir()}
    result = invoke(r, "--plan-restore")
    report = json.loads(result.stdout)
    assert result.returncode == 0, report["blockers"]
    assert report["status"] == "OFFLINE_RESTORE_PLAN_VALID_NOT_AUTHORIZED" and len(report["steps"]) == 9
    assert report["authorizes_execution"] is report["actual_restore_verified"] is False
    assert report["database_access"] == "NONE" and report["external_gates"]
    assert "pg_restore" not in result.stdout and "v3_restore_fixture" not in result.stdout
    r["restore"]["network_disabled"] = False
    assert invoke(r, "--plan-restore").returncode == 2
    r["restore"]["network_disabled"] = True
    r["restore"]["available_extensions"] = []
    assert invoke(r, "--plan-restore").returncode == 2
    assert before == {p.name: p.read_bytes() for p in qa_scratch.iterdir()}


@pytest.mark.case("M01-authenticated-package-cli-refusal")
def test_actual_authenticate_mode_cannot_promote_incomplete_receipts_or_leak_key_paths(qa_scratch):
    r = {"version": 1, "package": package(qa_scratch),
         "tool": {"path": "PRIVATE_FIXTURE_SECRET", "sha256": "", "module_sum": ""},
         "identity_path": "PRIVATE_FIXTURE_SECRET", "artifacts": []}
    before = {p.name: p.read_bytes() for p in qa_scratch.iterdir()}
    result = invoke(r, "--authenticate-only")
    report = json.loads(result.stdout)
    assert result.returncode == 2 and report["status"] == "BLOCKED"
    assert report["blockers"] == ["AUTH_ARTIFACT_COVERAGE_OR_BINDING_INVALID"]
    assert report["archive_authentication_verified"] is report["authorizes_execution"] is report["actual_restore_verified"] is False
    assert report["database_access"] == "NONE" and "PRIVATE_FIXTURE_SECRET" not in result.stdout + result.stderr
    for raw in (json.dumps(r) + "{}", '{"version":1,"private_key":"PRIVATE_FIXTURE_SECRET"}'):
        result = invoke(r, "--authenticate-only", raw=raw)
        assert result.returncode == 2 and not result.stdout and result.stderr.strip() == "BASELINE_PACKAGE_INVALID_INPUT"
    assert before == {p.name: p.read_bytes() for p in qa_scratch.iterdir()}


@pytest.mark.case("M01-local-transfer-cli-refusal")
def test_actual_transfer_modes_refuse_unverified_paths_and_never_emit_plaintext(qa_scratch):
    for flag in ("--seal-only", "--recover-only"):
        result = invoke({"version": 1}, flag)
        assert result.returncode == 2 and not result.stdout
        assert result.stderr.strip() == "BASELINE_PACKAGE_LOCAL_TRANSFER_REJECTED"
        result = invoke({}, flag, raw='{"version":1,"private_key":"PRIVATE_FIXTURE_SECRET"}')
        assert result.returncode == 2 and "PRIVATE_FIXTURE_SECRET" not in result.stderr + result.stdout
    assert not list(qa_scratch.iterdir())
