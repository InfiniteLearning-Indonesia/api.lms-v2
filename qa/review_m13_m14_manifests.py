"""Read-only manifest review for the three M13/M14 synthetic checkpoints."""

import ast
from pathlib import Path

from qa.checkpoint_registry import load, validate

ROOT = Path(__file__).resolve().parents[1]
MANIFESTS = ("M13-CP04.json", "M14-CP01.json", "M14-CP02.json", "M14-CP03.json", "M14-CP04.json")


def python_cases(path: Path) -> set[str]:
    tree = ast.parse(path.read_text(encoding="utf-8"))
    found = set()
    for node in tree.body:
        if not isinstance(node, (ast.FunctionDef, ast.AsyncFunctionDef)):
            continue
        for decorator in node.decorator_list:
            if (isinstance(decorator, ast.Call) and isinstance(decorator.func, ast.Attribute)
                    and decorator.func.attr == "case" and decorator.args
                    and isinstance(decorator.args[0], ast.Constant)):
                found.add(decorator.args[0].value)
    return found


def go_cases(path: Path) -> set[str]:
    tree = path.read_text(encoding="utf-8")
    import re
    return set(re.findall(r"(?m)^func (Test[A-Za-z0-9_]+)\(", tree))


def main() -> None:
    python_found = (python_cases(ROOT / "qa/tests/reporting_api/test_reports.py")
                    | python_cases(ROOT / "qa/tests/reporting_api/test_report_pages.py")
                    | python_cases(ROOT / "qa/tests/ops/test_consumer_closure_order.py")
                    | python_cases(ROOT / "qa/tests/ops/test_reporting_performance.py")
                    | python_cases(ROOT / "qa/tests/reporting_performance_api/test_load.py"))
    go_found = {
        "lmsv3/internal/outbox": go_cases(ROOT / "internal/outbox/worker_test.go"),
        "lmsv3/internal/reportbench": go_cases(ROOT / "internal/reportbench/load_test.go"),
        "lmsv3/internal/reporting": go_cases(ROOT / "internal/reporting/report_test.go")
        | go_cases(ROOT / "internal/reporting/pages_test.go")
        | go_cases(ROOT / "internal/reporting/integration_test.go")
        | go_cases(ROOT / "internal/reporting/performance_integration_test.go"),
    }
    for name in MANIFESTS:
        manifest = load(ROOT / "qa/manifests" / name)
        module = manifest["module"]
        declared = set(manifest.get("m13_cp04_go", []) + manifest.get("m13_cp04_python", []) + manifest.get("m14_cp01_go", [])
                       + manifest.get("m14_cp01_python", []) + manifest.get("m14_cp02_go", [])
                       + manifest.get("m14_cp02_python", []) + manifest.get("m14_cp03_go", [])
                       + manifest.get("m14_cp03_python", []) + manifest.get("m14_cp04_python", [])
                       + manifest.get("m14_cp04_load_go", []) + manifest.get("m14_cp04_db_go", []))
        checkpoint = manifest["checkpoints"][0]
        validate(manifest, {f"{module}-D0{i}" for i in range(1, 6)}, declared)
        if set(checkpoint["cases"]) != declared:
            raise ValueError(name + ": case mapping differs from runner lists")
        for case in declared:
            if case.startswith("lmsv3/"):
                package, test = case.split(":", 1)
                if test not in go_found.get(package, set()):
                    raise ValueError(name + ": missing Go test " + case)
            elif case not in python_found:
                raise ValueError(name + ": missing Playwright case " + case)
        print(f"{checkpoint['id']}: manifest valid; {len(declared)} mapped cases exist")


if __name__ == "__main__":
    main()
