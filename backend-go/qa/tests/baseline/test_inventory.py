import json
import os
from pathlib import Path
import re
import pytest


@pytest.mark.case("M00-T01-local-inventory")
def test_local_inventory_never_claims_live_database_verification():
    report = json.loads(Path(os.environ["QA_BASELINE_REPORT"]).read_text(encoding="utf-8"))
    assert report["mode"] == "LOCAL_SOURCE_READ_ONLY"
    assert {r["label"] for r in report["repositories"]} == {"backend", "backend-live"}
    for repo in report["repositories"]:
        assert re.fullmatch(r"[0-9a-f]{40}", repo["checkout_commit"])
        assert re.fullmatch(r"[0-9a-f]{64}", repo["source_hash"])
        assert repo["deployed_commit"] == "UNKNOWN"
        assert repo["typescript_files_scanned"] > 0
        assert repo["declarations"]
        assert all(d["line"] > 0 and "raw" not in d for d in repo["declarations"])
    assert all(c["database_identity_verified"] is False for c in report["local_config_observations"])
    assert "DATABASE_IDENTITIES_NOT_VERIFIED" in report["blockers"]
    assert "DEPLOYED_ARTIFACT_NOT_VERIFIED" in report["blockers"]
