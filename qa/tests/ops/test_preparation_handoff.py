"""Real small pytest subprocesses and owner-handoff rejection contracts."""
import ast
import importlib.util
import json
import os
from pathlib import Path
import sys
from types import SimpleNamespace
import uuid
import zipfile

import pytest
from qa import preparation,storage

SERVICE=Path(__file__).resolve().parents[3]


@pytest.fixture
def lab():
    base=SERVICE.parent.parent/'lms-v3-qa-artifacts'/('preparation-unit-'+uuid.uuid4().hex)
    service=base/'project/backend-v3/backend-go'
    (service/'qa/tests/ops').mkdir(parents=True);(service/'qa/manifests').mkdir()
    (service/'pytest.ini').write_text('[pytest]\nmarkers =\n    case(id): stable evidence ID\n')
    dest=base/'evidence';dest.mkdir()
    return SimpleNamespace(base=base,service=service,dest=dest)


def tiny_suite(lab,skip=False,expected=None):
    script='import pytest\n@pytest.mark.case("FIXTURE-pass")\ndef test_pass(): assert True\n'
    if skip:script+='@pytest.mark.case("FIXTURE-skip")\ndef test_skip(): pytest.skip("fixture")\n'
    (lab.service/'qa/tests/ops/test_tiny.py').write_text(script)
    value={'version':1,'profile':'LOCAL_FILESYSTEM_PREPARATION_ONLY','full_module_verified':False,
           'tests':['qa/tests/ops/test_tiny.py'],'expected_checks':expected or ['FIXTURE-pass']+(['FIXTURE-skip'] if skip else [])}
    storage.dump_new(lab.service/'qa/manifests/M14-CP04-preparation.json',value)
    return dict(os.environ,PYTHONPATH=str(SERVICE),PYTEST_DISABLE_PLUGIN_AUTOLOAD='1',PYTHONDONTWRITEBYTECODE='1')


def operator(name):
    path=SERVICE.parent/'portability'/name
    sys.path.insert(0,str(path.parent))
    spec=importlib.util.spec_from_file_location('preparation_fixture_'+path.stem,path)
    module=importlib.util.module_from_spec(spec);spec.loader.exec_module(module)
    return module


@pytest.mark.case('Q00-preflight-case-evidence')
def test_real_pytest_writes_exact_evidence_without_inheriting_old_output(lab):
    env=tiny_suite(lab);old=lab.base/'older-cases.json';old.write_text('preserve older evidence')
    env.update(QA_CASE_RESULTS=str(old),QA_ARTIFACT_ROOT=str(lab.base/'wrong-root'))
    result=preparation.run_ops_checks(lab.service,lab.dest,env)
    assert result['verified_case_count']==1 and result['status']=='PASS_LOCAL_PREPARATION_ONLY'
    assert old.read_text()=='preserve older evidence' and not (lab.base/'wrong-root').exists()
    evidence=json.loads((lab.dest/'ops-cases.json').read_text())
    assert evidence['collected_ids']==['FIXTURE-pass'] and evidence['cases'][0]['status']=='PASS'


@pytest.mark.case('Q00-preflight-nonpass-green-exit')
def test_real_pytest_exit_zero_with_skip_does_not_pass_preparation(lab):
    env=tiny_suite(lab,skip=True)
    with pytest.raises(ValueError,match='OPS_CASE_NONPASS'):preparation.run_ops_checks(lab.service,lab.dest,env)
    report=json.loads((lab.dest/'ops-check.json').read_text())
    assert report['exit_code']==0 and report['status']=='REJECTED'


@pytest.mark.case('Q00-preflight-frozen-case-set')
def test_real_pytest_green_exit_with_incomplete_manifest_set_is_rejected(lab):
    env=tiny_suite(lab,expected=['FIXTURE-pass','FIXTURE-missing'])
    with pytest.raises(ValueError,match='COLLECTED_CASES_MISMATCH'):preparation.run_ops_checks(lab.service,lab.dest,env)
    assert json.loads((lab.dest/'ops-check.json').read_text())['exit_code']==0


@pytest.mark.case('Q00-owner-handoff-blocked')
def test_unknown_inventory_never_launches_qa(monkeypatch):
    tool=operator('continue_m14_cp04.py')
    monkeypatch.setattr(tool,'review_storage',lambda:{'status':'BLOCKED','reasons':['UNKNOWN_BYTES']})
    monkeypatch.setattr(tool.subprocess,'run',lambda *a,**k:pytest.fail('QA launched without storage admission'))
    assert tool.continue_qa(Path('.'))==1


@pytest.mark.case('Q00-owner-handoff-admitted')
def test_inventory_only_does_not_launch_and_qa_failure_is_preserved(monkeypatch):
    tool=operator('continue_m14_cp04.py');calls=[]
    monkeypatch.setattr(tool,'review_storage',lambda:{'status':'PASS_STORAGE_ADMISSION_ONLY','reasons':[]})
    def launch(command,**kw):calls.append((command,kw));return SimpleNamespace(returncode=1)
    monkeypatch.setattr(tool.subprocess,'run',launch)
    assert tool.continue_qa(Path('.'),inventory_only=True)==0 and not calls
    assert tool.continue_qa(Path('.'),prepare_only=True)==1
    assert len(calls)==1 and '--prepare-only' in calls[0][0] and '--pg-bin' in calls[0][0]
    assert calls[0][1]['cwd']==SERVICE


@pytest.mark.case('Q00-owner-inventory-real-gate')
def test_inventory_unknown_is_recorded_without_zero_byte_admission(lab,monkeypatch):
    tool=operator('review_qa_storage_owner.py')
    (lab.service.parent/'private-backup').mkdir();root=storage.project_locations(lab.service)[0];root.mkdir()
    monkeypatch.setattr(tool,'SERVICE',lab.service);monkeypatch.setattr(tool,'BASE',lab.service.parent)
    monkeypatch.setattr(storage,'inventory',lambda *a:{'complete':False,'unreadable':[{'path':'fixture-unknown','error':'PermissionError'}],
                                                   'allocated_bytes':0,'logical_bytes':0})
    monkeypatch.setattr(tool.shutil,'disk_usage',lambda *a:SimpleNamespace(free=100*storage.GIB))
    result=tool.review_storage()
    assert result['status']=='BLOCKED' and result['reasons']==['UNKNOWN_BYTES']
    assert json.loads((Path(result['report'])/'inventory.json').read_text())['acl_changes']=='NONE'


@pytest.mark.case('Q00-preflight-secret-archive-exclusion')
def test_operator_context_archive_excludes_dotenv_hyphen_and_uppercase_variants(lab):
    tool=operator('collect_important_context.py')
    names=['.env-staging','.ENV','.env.example','fixture.key','public.py']
    files=[]
    for name in names:
        p=lab.base/name;p.write_text('PRIVATE_TEST_FIXTURE' if name!='public.py' else '# public source\n');files.append(p)
    tool.archive(lab.dest,'filtered-context',[p for p in files if not tool.config(p)])
    with zipfile.ZipFile(lab.dest/'filtered-context.zip') as z:
        assert len(z.namelist())==1 and z.namelist()[0].endswith('/public.py')
        assert b'PRIVATE_TEST_FIXTURE' not in z.read(z.namelist()[0])


@pytest.mark.case('Q00-preflight-manifest-wiring')
def test_fixed_preparation_manifest_covers_selected_ops_and_launcher_verifies_evidence():
    manifest=preparation.load_ops_manifest(SERVICE);ids=set()
    for name in manifest['tests']:
        for node in ast.walk(ast.parse((SERVICE/name).read_text(encoding='utf-8'))):
            if isinstance(node,ast.Call) and isinstance(node.func,ast.Attribute) and node.func.attr=='case' and node.args:
                if isinstance(node.args[0],ast.Constant):ids.add(node.args[0].value)
    assert ids==set(manifest['expected_checks'])
    tree=ast.parse((SERVICE.parent/'portability/run_m14_cp04.py').read_text(encoding='utf-8'))
    assert any(isinstance(n,ast.Call) and isinstance(n.func,ast.Name) and n.func.id=='run_ops_checks' for n in ast.walk(tree))
