"""Verify the runner executes the actual worker gate before consumer closure."""
import ast
import json
from pathlib import Path

import pytest


@pytest.mark.case("M13-CP04-runner-order")
def test_worker_dependency_is_executed_before_consumer_closure():
    source = Path(__file__).resolve().parents[2] / "postgres_lab.py"
    tree = ast.parse(source.read_text(encoding="utf-8"))
    def named_if(node, name):
        return isinstance(node, ast.If) and isinstance(node.test, ast.Name) and node.test.id == name
    blocks = []
    for node in ast.walk(tree):
        body = getattr(node, "body", None)
        if (isinstance(body, list) and any(named_if(item, "worker_journey") for item in body)
                and any(named_if(item, "consumer_closure_journey") and any(
                    isinstance(child, ast.Constant) and child.value == "M13-CP04 dependency regression missing"
                    for child in ast.walk(item)) for item in body)):
            blocks.append(body)
    assert len(blocks) == 1, "worker and consumer gates must share the same sequential scope"
    selected = [item for item in blocks[0] if named_if(item, "worker_journey") or named_if(item, "consumer_closure_journey")]
    assert named_if(selected[0], "worker_journey"), "consumer closure precedes required worker execution"
    program = compile(ast.Module(body=selected, type_ignores=[]), str(source), "exec")
    class ArtifactStub:
        def __truediv__(self, name):
            return self
        def read_text(self):
            return json.dumps({"exit_status": 0, "collected_ids": ["M13-CP04-runner-order"],
                               "cases": [{"id": "M13-CP04-runner-order", "status": "PASS"}]})
    for worker, reconciled, expected_pass in ((True, True, True), (False, True, False), (True, False, False)):
        result = {"m13_cp03_reconciliation": "PASS" if reconciled else "FAIL"}
        calls = []
        def slice_go(checkpoint, package, tests, **kwargs):
            calls.append(checkpoint)
            result[checkpoint] = {"status": "PASS_SYNTHETIC_ONLY"}
        env = {"worker_journey": worker, "consumer_closure_journey": True,
               "result": result, "slice_go": slice_go,
               "snapshot": lambda *args: {"restore": "VERIFIED_FIXTURE_SCOPE"},
               "os": __import__("os"), "sys": __import__("sys"), "json": json,
               "env": {}, "directory": ArtifactStub(), "command": lambda *args: None,
               "manifest": {"m13_cp04_python": ["M13-CP04-runner-order"]}}
        if expected_pass:
            exec(program, env)
            assert calls == ["m03_cp05", "m03_cp06", "m13_cp04"]
            assert result["m13_cp04_consumer"]["status"] == "PASS_SYNTHETIC_ONLY"
        else:
            with pytest.raises(ValueError, match="dependency regression missing"):
                exec(program, env)
            assert "m13_cp04_consumer" not in result
            assert "m13_cp04" not in calls
