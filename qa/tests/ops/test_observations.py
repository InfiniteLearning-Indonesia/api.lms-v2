import copy
from datetime import datetime, timezone
import json
import pytest
from qa.observations import reconcile, observation_digest
from qa.evidence import source_fingerprint
from hashlib import sha256


@pytest.mark.case("Q00-operational-evidence")
def test_operational_evidence(tmp_path):
    tree = tmp_path / "tree"
    tree.mkdir()
    (tree / "Z.go").write_bytes(b"upper")
    (tree / "a.go").write_bytes(b"lower")
    expected_tree = sha256(b"Z.go\0upper\0a.go\0lower\0").hexdigest()
    assert source_fingerprint(tree) == expected_tree
    now = datetime(2026, 9, 6, 12, tzinfo=timezone.utc)
    obs = dict(connection_fingerprint="b"*64, observed_at=now.isoformat(),
               environment="USER_IDENTIFIED_STAGING", status="READ_ONLY_OBSERVATION_COMPLETE",
               tls_verified=True, read_only_transaction=True, database_mutations="NONE",
               public_tables=[], catalog_sha256="c"*64)
    good = dict(run_id="20260906T120000Z-staging-inspect-012345abcdef", observation=obs,
                source_hash="a"*64, current_source_hash="a"*64, probe_exit_code=0,
                config_reference=".env.staging (user-identified staging)",
                backup="NOT_EXECUTED", migration="NOT_EXECUTED", observation_sha256=observation_digest(obs))
    path = tmp_path / "receipt.json"
    def check(value, target="b"*64):
        path.write_text(json.dumps(value), encoding="utf-8")
        return reconcile(path, "a"*64, target, now)
    r = check(good)
    assert r["status"] == "OBSERVED_NOT_APPROVED" and r["tls"] == "VERIFIED"
    assert r["production_identity"] == r["least_privilege"] == "PENDING"
    assert r["authorizes_mutation"] is False
    assert check(good, "d"*64)["status"] == "REJECTED"
    for key, value in [("source_hash", "old"), ("current_source_hash", "old")]:
        assert check({**good, key: value})["status"] == "STALE_SOURCE"
    for key, value in [("tls_verified", False), ("read_only_transaction", False),
                       ("database_mutations", "UNKNOWN"), ("public_tables", None)]:
        bad = copy.deepcopy(good)
        bad["observation"][key] = value
        bad["observation_sha256"] = observation_digest(bad["observation"])
        assert check(bad)["status"] == "REJECTED"
    bad = copy.deepcopy(good)
    bad["observation"]["observed_at"] = "2026-09-01T00:00:00Z"
    assert check(bad)["status"] == "STALE_TIME"
    assert check({**good, "observation_sha256": "wrong"})["status"] == "REJECTED"
    assert check({**good, "probe_exit_code": 2})["status"] == "REJECTED"
    assert check({})["status"] == "REJECTED"
