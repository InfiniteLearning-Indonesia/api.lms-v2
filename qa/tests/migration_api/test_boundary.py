import pytest
import json
from pathlib import Path
import os


@pytest.mark.case("M15-CP01-no-public-migration-route")
def test_preflight_is_not_an_http_capability(lab):
    admin, csrf = lab["login"]("admin")
    for path in ("/v3/migrations/preflight", "/v3/migration/dry-run", "/v3/import"):
        response = lab["mutate"](admin, csrf, path, {"source_environment": "synthetic"})
        assert response.status == 404
        assert "synthetic-source-1" not in response.text()


@pytest.mark.case("M15-CP02-no-landing-leak")
def test_landing_is_not_readable_or_mutable_from_http(lab):
    admin, csrf = lab["login"]("admin")
    student, student_csrf = lab["login"]("student")
    for actor, token in ((admin, csrf), (student, student_csrf)):
        for path in ("/v3/migration/landing", "/v3/migrations/snapshots/cp02-landing", "/v3/landing/users/same-pk"):
            assert actor.get(path).status == 404
            response = lab["mutate"](actor, token, path, {"sealed_ref": "sealed/users/1"})
            assert response.status == 404
            assert "sealed/users/1" not in response.text()


@pytest.mark.case("M15-CP03-no-public-mapping-route")
def test_mapping_decisions_are_not_an_http_capability(lab):
    admin, csrf = lab["login"]("admin")
    for path in ("/v3/migrations/mappings", "/v3/migration/decisions/cp03-run-a"):
        assert admin.get(path).status == 404
        assert lab["mutate"](admin, csrf, path, {"reviewer": "qa-domain-owner"}).status == 404


@pytest.mark.case("M15-CP04-shadow-not-operational")
def test_private_shadow_class_cannot_be_read_or_imported_from_http(lab):
    root = Path(os.environ["QA_ARTIFACT_ROOT"]).resolve(strict=True)
    target_id = json.loads((root / "m15cp04-shadow-id.json").read_text(encoding="utf-8"))["target_id"]
    admin, csrf = lab["login"]("admin")
    assert admin.get(f"/v3/classes/{target_id}").status == 404
    for path in ("/v3/migrations/backfill", "/v3/migration/shadow-classes", f"/v3/migration/receipts/{target_id}"):
        assert admin.get(path).status == 404
        assert lab["mutate"](admin, csrf, path, {"run_id": "cp03-run-a"}).status == 404


@pytest.mark.case("M15-CP05-no-public-reconcile-route")
def test_reconciliation_is_operator_only(lab):
    admin, csrf = lab["login"]("admin")
    for path in ("/v3/migrations/reconcile", "/v3/migration/reports/cp03-run-a"):
        assert admin.get(path).status == 404
        assert lab["mutate"](admin, csrf, path, {"run_id": "cp03-run-a"}).status == 404
