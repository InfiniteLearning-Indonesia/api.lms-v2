import pytest
from qa.checkpoint_report import exact_cases, union_coverage, deliverables


@pytest.mark.case("Q00-checkpoint-report")
def test_checkpoint_report_rejects_false_completion():
    case={"id":"a","status":"PASS"}
    assert exact_cases(["a"],[case])
    for cases in ([],[case,case],[{"id":"a","status":"SKIPPED"}],[case,{"id":"b","status":"PASS"}]):
        assert not exact_cases(["a"],cases)
    manifest={"deliverables":{"D1":["a","b"]}}
    assert deliverables(manifest,{"a":True,"b":True},True)["D1"]["status"]=="VERIFIED_LOCAL"
    assert deliverables(manifest,{"a":True,"b":True},False)["D1"]["status"]=="PENDING"
    assert deliverables(manifest,{"a":True},True)["D1"]["status"]=="PENDING"
    profiles=["mode: set\npkg/file.go:1.1,2.1 2 1\npkg/file.go:3.1,4.1 8 0\n", "mode: atomic\npkg/file.go:1.1,2.1 2 10\npkg/file.go:3.1,4.1 8 1\n"]
    assert union_coverage(profiles)["pkg"]==dict(covered=10,statements=10,percent=100.0)
    with pytest.raises(ValueError):union_coverage([profiles[0],profiles[1].replace("2 10","3 10")])
    with pytest.raises(ValueError):union_coverage(["mode: set\n"])
