"""Regressions for observed coverage/stop failures in the reporting journey."""
import ast
from pathlib import Path

import pytest


def commands():
    source = Path(__file__).resolve().parents[2] / 'postgres_lab.py'
    tree = ast.parse(source.read_text(encoding='utf-8'))
    return [node for node in ast.walk(tree) if isinstance(node, ast.Call)
            and isinstance(node.func, ast.Name) and node.func.id == 'command'
            and node.args and isinstance(node.args[0], ast.List)]


@pytest.mark.case('M14-CP02-instrumentation-regression')
def test_actual_identity_binary_instruments_reporting():
    builders = [node for node in commands() if any(
        isinstance(arg, ast.Name) and arg.id == 'identity_binary' for arg in node.args[0].elts)
        and any(isinstance(arg, ast.Constant) and arg.value == './cmd/identity-lab' for arg in node.args[0].elts)]
    assert len(builders) == 1
    arguments = [node.value for node in builders[0].args[0].elts if isinstance(node, ast.Constant)]
    assert '-cover' in arguments
    coverage = [value for value in arguments if isinstance(value, str) and value.startswith('-coverpkg=')]
    assert len(coverage) == 1
    assert './internal/reporting' in coverage[0].split('=', 1)[1].split(',')


@pytest.mark.case('Q00-postgres-stop-budget')
def test_fast_stop_has_bounded_checkpoint_budget():
    stops = [node for node in commands() if any(
        isinstance(arg, ast.Constant) and arg.value == 'stop' for arg in node.args[0].elts)]
    assert len(stops) == 1
    values = [node.value if isinstance(node, ast.Constant) else None for node in stops[0].args[0].elts]
    assert values[values.index('-m') + 1] == 'fast'
    assert '-w' in values
    wait = int(values[values.index('-t') + 1])
    timeout = stops[0].args[1].value
    assert 30 <= wait <= 60
    assert wait + 5 <= timeout <= 75
