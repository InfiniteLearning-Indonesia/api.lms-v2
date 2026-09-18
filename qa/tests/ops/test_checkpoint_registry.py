import copy
from pathlib import Path
import pytest

from qa.checkpoint_registry import load, reconcile, validate


ROOT = Path(__file__).resolve().parents[3]


def catalog_cases():
    cases = set(load(ROOT / "qa/manifests/M03.json")["expected_checks"])
    cases.update(load(ROOT / "qa/manifests/go-tests.json")["expected_tests"])
    for name in ("M03-CP03-CP04.json", "M03-CP05-CP06.json", "M05-CP03-CP04.json", "M06-CP01-CP02.json", "M06-CP03-M07-CP01.json", "M07-CP02.json", "M07-CP03.json", "M08-CP01-CP02.json", "M08-CP03-CP04.json", "M15-CP01-CP02.json", "M15-CP03-CP04.json", "M15-CP05-M11-CP01.json", "M12-CP01.json", "M12-CP02-CP03.json", "M13-CP01-CP02.json", "M09-CP01-CP02.json"):
        manifest = load(ROOT / "qa/manifests" / name)
        for key, value in manifest.items():
            if key.endswith("_go") or key.endswith("_python"):
                cases.update(value)
    return cases


@pytest.mark.case("M03-CP06-checkpoint-registry")
def test_checkpoint_registry_rejects_missing_duplicate_and_stale_evidence(tmp_path):
    registry_path = ROOT / "qa/manifests/M03-CP05-CP06.json"
    registry = load(registry_path)
    registry["module"] = "M03"
    module = load(ROOT / "qa/manifests/M03.json")
    deliverables = {item["id"] for item in module["deliverables"]}
    validate(registry, deliverables, catalog_cases())
    expected = next(item["cases"] for item in registry["checkpoints"] if item["id"] == "M03-CP05")
    evidence = [{"id": case, "status": "PASS"} for case in expected]
    assert reconcile(registry, "M03-CP05", evidence, "same", "same")["status"] == "PASS"
    assert reconcile(registry, "M03-CP05", evidence[:-1], "same", "same")["status"] == "BLOCKED"
    assert reconcile(registry, "M03-CP05", evidence, "new", "old")["status"] == "BLOCKED"
    with pytest.raises(ValueError):
        reconcile(registry, "M03-CP05", evidence + evidence[:1], "same", "same")
    broken = copy.deepcopy(registry)
    broken["checkpoints"][0]["cases"].append("UNREGISTERED")
    with pytest.raises(ValueError):
        validate(broken, deliverables, catalog_cases())
    duplicate = tmp_path / "duplicate.json"
    duplicate.write_text('{"version":1,"version":1}', encoding="utf-8")
    with pytest.raises(ValueError):
        load(duplicate)

    m05 = load(ROOT / "qa/manifests/M05-CP03-CP04.json")
    m05_cases = set(m05["m05_cp03_go"] + m05["m05_cp04_go"] + m05["m05_python"])
    validate(m05, {f"M05-D0{i}" for i in range(1, 6)}, m05_cases)

    m06 = load(ROOT / "qa/manifests/M06-CP01-CP02.json")
    m06_cases = set(m06["m06_cp01_go"] + m06["m06_cp02_go"] + m06["m06_python"])
    validate(m06, {f"M06-D0{i}" for i in range(1, 6)}, m06_cases)

    next_scope = load(ROOT / "qa/manifests/M06-CP03-M07-CP01.json")
    all_cases = catalog_cases()
    for checkpoint, module in (("M06-CP03", "M06"), ("M07-CP01", "M07")):
        item = next(item for item in next_scope["checkpoints"] if item["id"] == checkpoint)
        scoped = {**next_scope, "module": module, "checkpoints": [item]}
        validate(scoped, {f"{module}-D0{i}" for i in range(1, 6)}, all_cases)

    m07_cp02 = load(ROOT / "qa/manifests/M07-CP02.json")
    m07_cp02["module"] = "M07"
    validate(m07_cp02, {f"M07-D0{i}" for i in range(1, 6)}, all_cases)

    m07_cp03 = load(ROOT / "qa/manifests/M07-CP03.json")
    m07_cp03["module"] = "M07"
    validate(m07_cp03, {f"M07-D0{i}" for i in range(1, 6)}, all_cases)

    m08 = load(ROOT / "qa/manifests/M08-CP01-CP02.json")
    m08["module"] = "M08"
    validate(m08, {f"M08-D0{i}" for i in range(1, 6)}, all_cases)
    for checkpoint in m08["checkpoints"]:
        evidence = [{"id": case, "status": "PASS"} for case in checkpoint["cases"]]
        assert reconcile(m08, checkpoint["id"], evidence, "current", "current")["status"] == "PASS"
        assert reconcile(m08, checkpoint["id"], evidence, "changed", "current")["status"] == "BLOCKED"

    m08_next = load(ROOT / "qa/manifests/M08-CP03-CP04.json")
    m08_next["module"] = "M08"
    validate(m08_next, {f"M08-D0{i}" for i in range(1, 6)}, catalog_cases())
    for checkpoint in m08_next["checkpoints"]:
        evidence = [{"id": case, "status": "PASS"} for case in checkpoint["cases"]]
        assert reconcile(m08_next, checkpoint["id"], evidence, "current", "current")["status"] == "PASS"
        assert reconcile(m08_next, checkpoint["id"], evidence, "changed", "current")["status"] == "BLOCKED"

    m15 = load(ROOT / "qa/manifests/M15-CP01-CP02.json")
    validate(m15, {f"M15-D0{i}" for i in range(1, 7)}, catalog_cases())
    for checkpoint in m15["checkpoints"]:
        evidence = [{"id": case, "status": "PASS"} for case in checkpoint["cases"]]
        assert reconcile(m15, checkpoint["id"], evidence, "current", "current")["status"] == "PASS"
        assert reconcile(m15, checkpoint["id"], evidence, "changed", "current")["status"] == "BLOCKED"

    m15_next = load(ROOT / "qa/manifests/M15-CP03-CP04.json")
    validate(m15_next, {f"M15-D0{i}" for i in range(1, 7)}, catalog_cases())
    for checkpoint in m15_next["checkpoints"]:
        evidence = [{"id": case, "status": "PASS"} for case in checkpoint["cases"]]
        assert reconcile(m15_next, checkpoint["id"], evidence, "current", "current")["status"] == "PASS"
        assert reconcile(m15_next, checkpoint["id"], evidence, "changed", "current")["status"] == "BLOCKED"

    mixed = load(ROOT / "qa/manifests/M15-CP05-M11-CP01.json")
    for module, checkpoint_id in (("M15", "M15-CP05"), ("M11", "M11-CP01")):
        item = next(item for item in mixed["checkpoints"] if item["id"] == checkpoint_id)
        scoped = {**mixed, "module": module, "checkpoints": [item]}
        validate(scoped, {f"{module}-D0{i}" for i in range(1, 7 if module == "M15" else 6)}, catalog_cases())
        evidence = [{"id": case, "status": "PASS"} for case in item["cases"]]
        assert reconcile(scoped, checkpoint_id, evidence, "current", "current")["status"] == "PASS"
        assert reconcile(scoped, checkpoint_id, evidence, "stale", "current")["status"] == "BLOCKED"

    m12 = load(ROOT / "qa/manifests/M12-CP01.json")
    m12["module"] = "M12"
    validate(m12, {f"M12-D0{i}" for i in range(1, 6)}, catalog_cases())
    frozen = m12["checkpoints"][0]["cases"]
    evidence = [{"id": case, "status": "PASS"} for case in frozen]
    assert reconcile(m12, "M12-CP01", evidence, "current", "current")["status"] == "PASS"
    assert reconcile(m12, "M12-CP01", evidence[:-1], "current", "current")["status"] == "BLOCKED"

    m12_next = load(ROOT / "qa/manifests/M12-CP02-CP03.json")
    m12_next["module"] = "M12"
    validate(m12_next, {f"M12-D0{i}" for i in range(1, 6)}, catalog_cases())
    for checkpoint in m12_next["checkpoints"]:
        evidence = [{"id": case, "status": "PASS"} for case in checkpoint["cases"]]
        assert reconcile(m12_next, checkpoint["id"], evidence, "current", "current")["status"] == "PASS"
        assert reconcile(m12_next, checkpoint["id"], evidence, "stale", "current")["status"] == "BLOCKED"

    m13 = load(ROOT / "qa/manifests/M13-CP01-CP02.json")
    m13["module"] = "M13"
    validate(m13, {f"M13-D0{i}" for i in range(1, 6)}, catalog_cases())
    for checkpoint in m13["checkpoints"]:
        evidence = [{"id": case, "status": "PASS"} for case in checkpoint["cases"]]
        assert reconcile(m13, checkpoint["id"], evidence, "current", "current")["status"] == "PASS"
        assert reconcile(m13, checkpoint["id"], evidence, "stale", "current")["status"] == "BLOCKED"

    m09 = load(ROOT / "qa/manifests/M09-CP01-CP02.json")
    m09["module"] = "M09"
    validate(m09, {f"M09-D0{i}" for i in range(1, 6)}, catalog_cases())
    for checkpoint in m09["checkpoints"]:
        evidence = [{"id": case, "status": "PASS"} for case in checkpoint["cases"]]
        assert reconcile(m09, checkpoint["id"], evidence, "current", "current")["status"] == "PASS"
        assert reconcile(m09, checkpoint["id"], evidence, "stale", "current")["status"] == "BLOCKED"
