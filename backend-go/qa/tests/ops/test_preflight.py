import pytest
from qa.preflight import (GateBlocked, require_local_api, require_distinct_verified_databases,
                          require_backup_receipt, require_handoff_evidence)


@pytest.mark.case("Q00-local-api-boundary")
def test_local_api_boundary():
    require_local_api("http://127.0.0.1:8123")
    require_local_api("http://[::1]:8123")
    for value in ("https://production.invalid", "http://localhost:8000", "http://0.0.0.0:8000",
                  "http://127.0.0.1:8000/path", "http://user:password@127.0.0.1:8000",
                  "http://127.0.0.1:8000?sid=secret", "http://127.0.0.1:99999", "http://127.0.0.1"):
        with pytest.raises(GateBlocked):
            require_local_api(value)


def receipt(project):
    return dict(project_id=project, database_id="fixture-db", server_receipt_hash="synthetic",
                ca_fingerprint="synthetic", identity_verified=True, tls_mode="verify-full", role_read_only=True)


@pytest.mark.case("M00-T02-fail-closed")
def test_database_predicate_fails_closed():
    source, target = receipt("source-fixture"), receipt("target-fixture")
    require_distinct_verified_databases(source, target)
    with pytest.raises(GateBlocked):
        require_distinct_verified_databases(source, source)
    for field, value in (("identity_verified", False), ("tls_mode", "require"),
                         ("ca_fingerprint", ""), ("server_receipt_hash", ""), ("role_read_only", False)):
        with pytest.raises(GateBlocked):
            require_distinct_verified_databases({**source, field: value}, target)


@pytest.mark.case("M00-T05-handoff-gate")
def test_incomplete_handoff_rejected():
    with pytest.raises(GateBlocked):
        require_handoff_evidence({"owner": "fixture"})
    require_handoff_evidence(dict(owner="fixture", reviewer="fixture-reviewer", source_schema_hash="fixture",
                                  operation_checksum="fixture", allowed_operations=["read-only"]))


@pytest.mark.case("M01-T01-backup-predicate")
def test_backup_requires_restore_and_scope():
    value = dict(backup_id="synthetic", artifact_sha256="synthetic", restore_evidence_id="synthetic",
                 reviewer_reference="synthetic", status="RESTORE_VERIFIED", operation_checksum="scope-a")
    require_backup_receipt(value, "scope-a")
    for update in ({"status": "FAILED"}, {"restore_evidence_id": ""}, {"operation_checksum": "scope-b"}):
        with pytest.raises(GateBlocked):
            require_backup_receipt({**value, **update}, "scope-a")
