import json

import pytest

from qa.ci_diagnostics import build_diagnostic, publish_diagnostic


def fixture():
    manifest = {"expected_checks": ["Q00-go-unit", "Q00-pytest"]}
    go = {"expected_tests": ["lmsv3/fixture:TestOne"]}
    cases = [{"package": "lmsv3/fixture", "name": "TestOne", "status": "pass"}]
    result = {"automated_checks": "PASS", "source_hash": "a" * 64, "tested_source_hash": "a" * 64,
              "manifest_sha256": "b" * 64, "cases": [{"id": i, "status": "PASS"} for i in manifest["expected_checks"]],
              "commands": [{"name": "go-unit", "return_code": 0, "status": "FINISHED"}]}
    return result, manifest, go, cases


@pytest.mark.case("Q00-ci-diagnostic-secret-boundary")
def test_public_projection_never_copies_private_output_or_unregistered_names(qa_scratch):
    result, manifest, go, cases = fixture()
    secret = "PRIVATE_FIXTURE_CREDENTIAL"
    result.update(stdout=secret, stderr=secret, database_url=secret, run_id=secret)
    result["commands"][0].update(stdout=secret, stderr=secret, args=[secret], env={"PASSWORD": secret})
    result["commands"].append({"name": secret, "return_code": 1})
    result["cases"].append({"id": secret, "status": "FAIL", "assertion": secret})
    cases.append({"package": secret, "name": secret, "status": "fail"})
    report = publish_diagnostic(qa_scratch, result, manifest, go, cases)
    assert secret not in json.dumps(report)
    assert secret not in (qa_scratch / "ci-diagnostic.json").read_text()
    assert report["status"] == "FAIL_OR_INCOMPLETE"
    assert report["unexpected_check_count"] == report["go_unregistered_count"] == 1
    with pytest.raises(FileExistsError):
        publish_diagnostic(qa_scratch, result, manifest, go, cases)


@pytest.mark.case("Q00-ci-diagnostic-exact-evidence")
def test_missing_failed_skipped_duplicate_stale_and_unregistered_cases_cannot_pass():
    result, manifest, go, cases = fixture()
    assert build_diagnostic(result, manifest, go, cases)["status"] == "PASS_BOOTSTRAP_ONLY"
    for status in ("FAIL", "ERROR", "SKIPPED", "BLOCKED", "NOT_RUN"):
        result["cases"][0]["status"] = status
        report = build_diagnostic(result, manifest, go, cases)
        assert report["status"] == "FAIL_OR_INCOMPLETE" and report["failed_checks"][0]["status"] == status
    result, manifest, go, cases = fixture()
    for changed in ([], cases * 2, [{**cases[0], "status": "skip"}], cases + [{**cases[0], "name": "TestNew"}]):
        assert build_diagnostic(result, manifest, go, changed)["status"] == "FAIL_OR_INCOMPLETE"
    result["tested_source_hash"] = "c" * 64
    assert build_diagnostic(result, manifest, go, cases)["status"] == "FAIL_OR_INCOMPLETE"
