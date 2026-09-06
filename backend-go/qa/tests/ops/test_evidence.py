import pytest
import os
from pathlib import Path
from qa.evidence import summarize, render_section, replace_if_unchanged, publish_checkpoint, source_fingerprint, verify_go_cases


def contract():
    return {"expected_checks": ["a", "b"], "deliverables": [
        {"id": "D1", "checks": ["a", "b"], "pending_evidence": []}],
        "required_gates": [{"id": "gate", "status": "PASS"}], "full_suite_implemented": True}


@pytest.mark.case("Q00-evidence-missing-cases")
def test_missing_skipped_flaky_failures_never_complete():
    for status in ("FAIL", "ERROR", "SKIPPED", "NOT_RUN", "BLOCKED", "INTERRUPTED", "FLAKY"):
        result = summarize(contract(), [{"id": "a", "status": "PASS"}, {"id": "b", "status": status}], "x", "x", True)
        assert result["deliverable_coverage"]["verified"] == 0
        assert result["package_status"] == "BLOCKED"
    result = summarize(contract(), [{"id": "a", "status": "PASS"}], "x", "x", True)
    assert result["missing_check_ids"] == ["b"]
    assert result["full_package_test_pass_rate"] == 50


@pytest.mark.case("Q00-evidence-stale-and-gates")
def test_stale_manual_and_gate_evidence_veto_completion():
    cases = [{"id": value, "status": "PASS"} for value in ("a", "b")]
    assert summarize(contract(), cases, "x", "x", True)["package_status"] == "BACKEND_VERIFIED"
    assert summarize(contract(), cases, "x", "changed", True)["evidence_currency"] == "STALE"
    c = contract(); c["deliverables"][0]["pending_evidence"] = ["REVIEW_NOT_AVAILABLE"]
    assert summarize(c, cases, "x", "x", True)["deliverable_coverage"]["verified"] == 0
    c = contract(); c["required_gates"][0]["status"] = "BLOCKED"
    assert summarize(c, cases, "x", "x", True)["package_status"] == "BLOCKED"
    assert summarize(contract(), cases, "x", "x", False)["package_status"] == "BLOCKED"


@pytest.mark.case("Q00-evidence-duplicates-zero")
def test_zero_duplicate_unexpected_cases():
    with pytest.raises(ValueError):
        summarize({**contract(), "expected_checks": []}, [], "x", "x", True)
    with pytest.raises(ValueError):
        summarize(contract(), [{"id": "a", "status": "PASS"}] * 2, "x", "x", True)
    extra = [{"id": v, "status": "PASS"} for v in ("a", "b", "extra")]
    assert summarize(contract(), extra, "x", "x", True)["automated_checks"] == "FAIL"
    go_case = {"package": "synthetic", "name": "TestOne", "status": "pass"}
    assert verify_go_cases(["synthetic:TestOne"], [go_case])
    assert not verify_go_cases([], [])
    assert not verify_go_cases(["synthetic:TestOne", "synthetic:TestMissing"], [go_case])
    assert not verify_go_cases(["synthetic:TestOne"], [go_case, go_case])
    assert not verify_go_cases(["synthetic:TestOne"], [{**go_case, "status": "skip"}])
    with pytest.raises(ValueError):
        summarize({**contract(), "deliverables": []}, [], "x", "x", True)
    partial = {**contract(), "full_suite_implemented": False}
    assert summarize(partial, [{"id": v, "status": "PASS"} for v in ("a", "b")], "x", "x", True)["package_status"] == "BLOCKED"


@pytest.mark.case("Q00-render-preserves-human-section")
def test_report_renderer_is_idempotent_and_preserves_manual_scope():
    result = summarize(contract(), [], "x", "x", False)
    original = "# Human scope\n\n- [ ] Never delete data.\n"
    output = render_section(original, "M00", result, "synthetic-run")
    assert output.startswith(original)
    assert render_section(output, "M00", result, "synthetic-run") == output
    with pytest.raises(ValueError):
        render_section(output + "<!-- QA:M00:START -->", "M00", result, "synthetic-run")
    with pytest.raises(ValueError):
        render_section("<!-- QA:M00:END --><!-- QA:M00:START -->", "M00", result, "synthetic-run")
    root = Path(os.environ["QA_ARTIFACT_ROOT"]) / "renderer-selftest"
    (root / "docs" / "modules").mkdir(parents=True)
    module, dashboard = root / "docs/modules/M00.md", root / "docs/IMPLEMENTATION_STATUS.md"
    module.write_text(original, encoding="utf-8")
    dashboard.write_text("# Human dashboard\n", encoding="utf-8")
    result.update(package="M00", run_id="synthetic-run", source_hash=source_fingerprint(root))
    receipt = publish_checkpoint(root, "M00", result)
    assert receipt["publication"] == "COMPLETE"
    assert module.read_text(encoding="utf-8").startswith(original)
    assert "synthetic-run" in dashboard.read_text(encoding="utf-8")
    saved = module.read_text(encoding="utf-8")
    assert publish_checkpoint(root, "M00", result)["publication"] == "COMPLETE"
    assert module.read_text(encoding="utf-8") == saved
    with pytest.raises(ValueError):
        replace_if_unchanged(module, "outdated text", "should not replace human changes")
    assert module.read_text(encoding="utf-8") == saved
    with pytest.raises(ValueError):
        publish_checkpoint(root, "M00", {**result, "source_hash": "stale"})
