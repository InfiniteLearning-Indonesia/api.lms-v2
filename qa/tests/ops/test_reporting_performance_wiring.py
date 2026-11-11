"""Ensure the actual full lab cannot skip the frozen load/DB/backup gates."""
import ast
import json
from pathlib import Path
import pytest

ROOT=Path(__file__).resolve().parents[3]


@pytest.mark.case('M14-CP04-runner-wiring')
def test_full_performance_journey_keeps_real_dependency_backup_and_db_gates():
    tree=ast.parse((ROOT/'qa/postgres_lab.py').read_text())
    branches=[n for n in ast.walk(tree) if isinstance(n,ast.If) and isinstance(n.test,ast.Name)
              and n.test.id=='reporting_performance_journey'
              and any(isinstance(v,ast.Constant) and v.value=='./cmd/report-load' for v in ast.walk(n))]
    assert len(branches)==1
    calls=[n for n in ast.walk(branches[0]) if isinstance(n,ast.Call)]
    proof=[n for n in calls if isinstance(n.func,ast.Name) and n.func.id=='m13_proof']
    assert len(proof)==1 and proof[0].args[0].value=='m14_cp04'
    build=next(n for n in calls if isinstance(n.func,ast.Name) and n.func.id=='command'
               and any(isinstance(v,ast.Constant) and v.value=='./cmd/report-load' for v in ast.walk(n)))
    slices=[n for n in calls if isinstance(n.func,ast.Name) and n.func.id=='slice_go']
    assert [n.args[0].value for n in slices]==['m14_cp04_load','m14_cp04_db']
    api=next(n for n in calls if isinstance(n.func,ast.Name) and n.func.id=='command'
             and any(isinstance(v,ast.Constant) and v.value=='qa/tests/reporting_performance_api' for v in ast.walk(n)))
    assert proof[0].lineno < build.lineno < slices[0].lineno < api.lineno < slices[1].lineno
    constants={n.value for n in ast.walk(tree) if isinstance(n,ast.Constant) and isinstance(n.value,str)}
    assert {'pre_m14_cp04','--reporting-performance-journey','m14_cp04_reconciliation'} <= constants
    flag_branches=[n for n in ast.walk(tree) if isinstance(n,ast.If) and isinstance(n.test,ast.Attribute)
                   and n.test.attr=='reporting_performance_journey']
    assert len(flag_branches)==1
    assigned=[n.targets[0] for n in flag_branches[0].body if isinstance(n,ast.Assign)]
    assert len(assigned)==1 and assigned[0].attr=='reporting_pagination_journey'
    main=(ROOT/'cmd/identity-lab/main.go').read_text()
    assert main.count('== "m14cp04"')==3
    budget=json.loads((ROOT/'qa/manifests/M14-CP04-budget.json').read_text())
    runtime=(ROOT/'internal/platform/database/pool.go').read_text()
    assert '"qa_runtime", "lmsv3-class-runtime", 4)' in runtime and budget['pool_max_connections']==4
    workflow=(ROOT/'.github/workflows/backend-go-qa.yml').read_text()
    assert '--reporting-performance-journey' in workflow
