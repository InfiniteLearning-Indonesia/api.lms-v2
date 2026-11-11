import copy
import ast
import json
from pathlib import Path
from types import SimpleNamespace
import uuid
import pytest
from qa import retention,storage


@pytest.fixture
def lab(monkeypatch):
    service=Path(__file__).resolve().parents[3]
    base=service.parent.parent/'lms-v3-qa-artifacts'/('retention-unit-'+uuid.uuid4().hex)
    roots={name:base/name for name in ('workspace','legacy')}
    for path in roots.values():path.mkdir(parents=True)
    monkeypatch.setattr(storage.subprocess,'run',lambda *a,**k:SimpleNamespace(returncode=0,stdout='Database cluster state:               shut down\n'))
    return base,roots


def make_run(roots,day,label='workspace',status='PASS_SYNTHETIC_ONLY'):
    name=f'202611{day:02}T000000Z-postgres-lab-{day:012x}'
    run=roots[label]/name;data=run/'postgres';(data/'global').mkdir(parents=True)
    (data/'PG_VERSION').write_text('18\n');(data/'global'/'pg_control').write_bytes(b'fixture control')
    (data/'page').write_bytes(b'preserved page'*1000)
    cases=[{'package':'synthetic/test','name':'TestExample','status':'pass' if status=='PASS_SYNTHETIC_ONLY' else 'fail'}]
    summary={'run_id':name,'profile':'new-local-synthetic','shared_database_access':'NONE','database_deletion':'NONE',
             'source_hash':'a'*64,'status':status,'cases':cases}
    (run/'summary.json').write_text(json.dumps(summary));(run/'unit-cases.json').write_text(json.dumps(cases))
    for suffix in ('sql','dump'):(run/('fixture.'+suffix)).write_bytes(b'synthetic backup')
    receipt={'scope':'SYNTHETIC_ONLY','restore':'VERIFIED_FIXTURE_SCOPE','source_database':'qa_cp05','proof_database':'fixture_proof',
             'artifacts':{p.name:storage.file_hash(p) for p in (run/'fixture.sql',run/'fixture.dump')}}
    (run/'fixture-receipt.json').write_text(json.dumps(receipt))
    return run


def row(plan,run):return next(r for r in plan['rows'] if r['run_id']==run.name)


@pytest.mark.case('Q00-retention-global-selection')
def test_global_two_pass_one_failure_historical_source_and_no_mutation(lab):
    _,roots=lab
    runs=[make_run(roots,day,'legacy' if day%2 else 'workspace') for day in range(1,5)]
    failures=[make_run(roots,day,'legacy' if day%2 else 'workspace','FAIL') for day in (5,6)]
    before={str(p):storage.file_hash(p) for root in roots.values() for p,_,_ in storage.files(root)}
    plan=retention.build_plan(roots,{},Path('.'))
    assert plan['decision_counts']=={'CANDIDATE':3,'KEEP':3}
    assert {r['run_id'] for r in plan['rows'] if r['decision']=='KEEP'}=={runs[2].name,runs[3].name,failures[1].name}
    expected=sum(storage.measure(p/'postgres')['allocated_bytes'] for p in (runs[0],runs[1],failures[0]))
    assert plan['estimated_candidate_allocated_bytes']==expected>0
    assert all(r['cluster_path'].endswith('postgres') for r in plan['rows'])
    assert plan['database_deletion']=='NONE' and plan['apply_implemented'] is True
    assert before=={str(p):storage.file_hash(p) for root in roots.values() for p,_,_ in storage.files(root)}
    assert all(r['source_currency']=='HISTORICAL_NOT_COMPARED_TO_CURRENT_TREE' for r in plan['rows'])
    shuffled=copy.deepcopy(plan['rows'])
    for r in shuffled:r.update(decision='ELIGIBLE',reason='test')
    assert [(r['run_id'],r['decision']) for r in retention.select(list(reversed(shuffled)))]==[(r['run_id'],r['decision']) for r in plan['rows']]


@pytest.mark.case('Q00-retention-protected-exceptions')
def test_pins_active_unknown_and_operator_failure_never_become_candidates(lab):
    _,roots=lab
    ordinary=[make_run(roots,i) for i in (1,2,3)]
    pinned=make_run(roots,4);active=make_run(roots,5)
    (active/'postgres'/'postmaster.pid').write_text('fixture pid')
    operator=make_run(roots,6,status='FAIL_STOP_REQUIRES_OPERATOR')
    unknown=make_run(roots,7,status='UNKNOWN')
    plan=retention.build_plan(roots,{('workspace',pinned.name):'Investigating this exact run'},Path('.'))
    assert row(plan,pinned)['reason']=='PINNED'
    assert row(plan,operator)['reason']=='OPERATOR_REVIEW_REQUIRED'
    assert row(plan,unknown)['reason']=='UNKNOWN_OR_BLOCKED_RUN_STATUS'
    assert row(plan,active)['decision']=='HOLD'
    assert row(plan,ordinary[0])['decision']=='CANDIDATE'
    assert row(plan,ordinary[1])['decision']==row(plan,ordinary[2])['decision']=='KEEP'


@pytest.mark.case('Q00-retention-evidence-gates')
def test_bad_backup_cases_source_scope_and_missing_summary_are_protected(lab):
    _,roots=lab
    broken=make_run(roots,1);(broken/'fixture.dump').write_bytes(b'corrupt')
    casebad=make_run(roots,2);(casebad/'unit-cases.json').write_text('[{"status":"skip"}]')
    scope=make_run(roots,3)
    value=json.loads((scope/'summary.json').read_text());value['shared_database_access']='PRODUCTION';(scope/'summary.json').write_text(json.dumps(value))
    source=make_run(roots,4)
    value=json.loads((source/'summary.json').read_text());value['source_hash']='';(source/'summary.json').write_text(json.dumps(value))
    missing=roots['workspace']/'20261105T000000Z-postgres-lab-000000000005';(missing/'postgres').mkdir(parents=True)
    plan=retention.build_plan(roots,{},Path('.'))
    assert plan['decision_counts']=={'HOLD':5}
    assert row(plan,broken)['reason']=='BACKUP_HASH_MISMATCH'
    assert row(plan,casebad)['reason']=='PASS_HAS_NONPASS_CASE'
    assert row(plan,scope)['reason']=='SYNTHETIC_IDENTITY_NOT_PROVEN'
    assert row(plan,source)['reason']=='SOURCE_HASH_MISSING'
    assert row(plan,missing)['reason']=='INSPECTION_FAILED'


@pytest.mark.case('Q00-retention-time-pins-duplicates')
def test_time_conflicts_malformed_pin_and_duplicate_run_ids_fail_closed(lab):
    base,roots=lab
    duplicate=make_run(roots,1);make_run(roots,1,'legacy')
    conflict=make_run(roots,2)
    v=json.loads((conflict/'summary.json').read_text());v['started_at']='2026-12-01T00:00:00+00:00';(conflict/'summary.json').write_text(json.dumps(v))
    pinbad=make_run(roots,3);(pinbad/'retention-pin.json').write_text('{broken')
    plan=retention.build_plan(roots,{('workspace','20261104T000000Z-postgres-lab-000000000004'):'missing pin'},Path('.'))
    assert plan['decision_counts']=={'HOLD':4} and len(plan['unresolved_pins'])==1
    assert row(plan,duplicate)['reason']=='DUPLICATE_RUN_ID'
    assert row(plan,conflict)['reason']=='INVALID_OR_CONFLICTING_RUN_TIME'
    registry=base/'pins.json';registry.write_text('{"version":1,"version":1,"pins":[]}')
    with pytest.raises(retention.Protected):retention.load_pins(registry)
    registry.write_text(json.dumps({'version':1,'pins':[{'root':'workspace','run_id':duplicate.name,'reason':'debug'}]}))
    assert retention.load_pins(registry)=={('workspace',duplicate.name):'debug'}


@pytest.mark.case('Q00-retention-path-boundaries')
def test_reparse_path_and_unreadable_candidate_size_do_not_escape_plan(lab,monkeypatch):
    _,roots=lab
    bad=make_run(roots,1);first=make_run(roots,2)
    make_run(roots,3);make_run(roots,4)
    original=storage.reparse
    monkeypatch.setattr(storage,'reparse',lambda p:True if Path(p)==bad else original(p))
    monkeypatch.setattr(storage,'measure',lambda p:(_ for _ in ()).throw(PermissionError('fixture protected')))
    plan=retention.build_plan(roots,{},Path('.'))
    assert row(plan,bad)['reason']=='REPARSE_PATH'
    assert row(plan,first)['reason']=='CANDIDATE_SIZE_UNREADABLE'
    assert plan['decision_counts']=={'HOLD':2,'KEEP':2} and plan['estimated_candidate_allocated_bytes']==0


@pytest.mark.case('Q00-retention-registry-coverage')
def test_collected_ops_cases_are_registered_in_bootstrap_contracts():
    service=Path(__file__).resolve().parents[3]
    ids=set()
    for path in (service/'qa/tests/ops').glob('test_*.py'):
        for node in ast.walk(ast.parse(path.read_text(encoding='utf-8'))):
            if (isinstance(node,ast.Call) and isinstance(node.func,ast.Attribute) and node.func.attr=='case'
                    and node.args and isinstance(node.args[0],ast.Constant)):
                ids.add(node.args[0].value)
    for name in ('M00','M03'):
        manifest=json.loads((service/f'qa/manifests/{name}.json').read_text())
        assert ids<=set(manifest['expected_checks'])
        assert len(manifest['expected_checks'])==len(set(manifest['expected_checks']))
        assert manifest['full_suite_implemented'] is False
