import json
import uuid
from pathlib import Path
from types import SimpleNamespace
import pytest
from qa import storage


@pytest.fixture(name='tmp_path')
def retained_fixture_directory():
    # pytest's chmod(0700) basetemp can be unreadable to the restricted Windows
    # token. Inherit workspace ACLs normally and retain these synthetic files.
    service=Path(__file__).resolve().parents[3]
    path=service.parent.parent/'lms-v3-qa-artifacts'/('storage-unit-'+uuid.uuid4().hex)
    path.mkdir(parents=True)
    return path


def fixture(tmp_path):
    root=tmp_path/'lms-v3-qa-artifacts';root.mkdir()
    run=root/'20261111T000000Z-postgres-lab-123456abcdef';run.mkdir()
    data=run/'postgres';data.mkdir();(data/'PG_VERSION').write_text('18\n')
    (data/'global').mkdir();(data/'global'/'pg_control').write_bytes(b'synthetic control')
    (data/'relation').write_bytes(b'append-only synthetic rows'*10)
    summary={'run_id':run.name,'profile':'new-local-synthetic','status':'PASS_SYNTHETIC_ONLY',
             'database_deletion':'NONE','shared_database_access':'NONE'}
    (run/'summary.json').write_text(json.dumps(summary))
    (run/'fixture.dump').write_bytes(b'fixture')
    (run/'fixture-receipt.json').write_text(json.dumps({'restore':'VERIFIED_FIXTURE_SCOPE',
                 'artifacts':{'fixture.dump':storage.file_hash(run/'fixture.dump')}}))
    return root,run,data


@pytest.mark.case('Q00-storage-synthetic-scope')
def test_only_closed_marker_bound_run_is_eligible(tmp_path,monkeypatch):
    root,run,data=fixture(tmp_path)
    monkeypatch.setattr(storage.subprocess,'run',lambda *a,**k:SimpleNamespace(returncode=0,stdout='Database cluster state:               shut down\n'))
    assert storage.closed_run(run,[root],tmp_path)[0]==run
    with pytest.raises(ValueError):storage.closed_run(run,[tmp_path],tmp_path)
    (data/'postmaster.pid').write_text('123')
    with pytest.raises(ValueError):storage.closed_run(run,[root],tmp_path)
    # No deletion even in the test fixture: use another independent case tree.
    second=tmp_path/'second';second.mkdir();root2,run2,data2=fixture(second)
    monkeypatch.setattr(storage.subprocess,'run',lambda *a,**k:SimpleNamespace(returncode=0,stdout='Database cluster state:               in production\n'))
    with pytest.raises(ValueError):storage.closed_run(run2,[root2],tmp_path)
    summary=json.loads((run2/'summary.json').read_text());summary['shared_database_access']='PRODUCTION'
    (run2/'summary.json').write_text(json.dumps(summary))
    with pytest.raises(ValueError):storage.closed_run(run2,[root2],tmp_path)


@pytest.mark.case('Q00-storage-evidence-integrity')
def test_backup_integrity_and_cluster_digest_detect_mutation(tmp_path):
    _,run,data=fixture(tmp_path)
    before=storage.tree_digest(data)
    index=storage.tree_index(run)
    assert storage.verify_backups(run)==['fixture-receipt.json']
    original=(data/'relation').read_bytes()
    (data/'relation').write_bytes(b'x'*len(original))
    assert storage.tree_index(run)==index  # Metadata mode must not claim full byte verification.
    assert storage.tree_digest(data)!=before
    (data/'relation').write_bytes(b'changed synthetic rows')
    assert storage.tree_index(run)!=index
    assert storage.tree_digest(data)!=before
    (run/'fixture.dump').write_bytes(b'corrupt')
    with pytest.raises(ValueError):storage.verify_backups(run)
    (run/'fixture-receipt.json').write_text(json.dumps({'restore':'VERIFIED_FIXTURE_SCOPE','artifacts':{'../outside':'x'}}))
    with pytest.raises(ValueError):storage.verify_backups(run)


@pytest.mark.case('Q00-storage-budget')
def test_budget_unknown_and_free_space_are_fail_closed(tmp_path,monkeypatch):
    state={'allocated_bytes':0,'logical_bytes':0,'complete':True,'unreadable':[]}
    monkeypatch.setattr(storage,'project_locations',lambda *a:[])
    monkeypatch.setattr(storage,'inventory',lambda *a:dict(state))
    monkeypatch.setattr(storage.shutil,'disk_usage',lambda *a:SimpleNamespace(free=100*storage.GIB))
    assert storage.preflight(tmp_path,tmp_path)['policy']['database_deletion']=='NEVER'
    state['allocated_bytes']=31*storage.GIB
    with pytest.raises(ValueError):storage.preflight(tmp_path,tmp_path)
    state.update(allocated_bytes=0,complete=False,unreadable=[{'path':'unknown'}])
    with pytest.raises(ValueError):storage.preflight(tmp_path,tmp_path)
    state.update(complete=True,unreadable=[])
    monkeypatch.setattr(storage.shutil,'disk_usage',lambda *a:SimpleNamespace(free=9*storage.GIB))
    with pytest.raises(ValueError):storage.preflight(tmp_path,tmp_path)
    monkeypatch.setattr(storage.shutil,'disk_usage',lambda *a:SimpleNamespace(free=100*storage.GIB))
    monkeypatch.setattr(storage,'measure',lambda *a:{'allocated_bytes':3*storage.GIB})
    with pytest.raises(ValueError):storage.guard_growth(tmp_path)


@pytest.mark.case('Q00-storage-selective-compression')
def test_compression_skips_empty_small_files_and_bounds_native_batches(tmp_path,monkeypatch):
    (tmp_path/'empty').write_bytes(b'')
    (tmp_path/'tiny').write_bytes(b'not worth a compression operation')
    for n in range(50):(tmp_path/f'page-{n}').write_bytes(b'x'*8192)
    calls=[]
    def compact(args,**kwargs):
        calls.append((args,kwargs));return SimpleNamespace(returncode=0)
    monkeypatch.setattr(storage.subprocess,'run',compact)
    code,completed,selected=storage.compress_selected(Path('compact.exe'),tmp_path)
    assert code==0 and completed==selected==50 and len(calls)==2
    for args,options in calls:
        assert not any(str(a).startswith('/S') for a in args)
        assert 1<=len(args[3:])<=48
        assert all(Path(a).name.startswith('page-') for a in args[3:])
        assert options['stdin']==storage.subprocess.DEVNULL and options['timeout']==45


@pytest.mark.case('Q00-storage-all-entrypoints')
def test_every_qa_entrypoint_refuses_storage_before_build_or_database_start(tmp_path,monkeypatch):
    from qa import run,release,postgres_lab
    service=tmp_path/'backend-v3'/'backend-go';manifests=service/'qa'/'manifests';manifests.mkdir(parents=True)
    (manifests/'M03.json').write_text(json.dumps({'package':'M03','profile':'local-synthetic'}))
    (manifests/'go-tests.json').write_text(json.dumps({'expected_tests':[]}))
    for module in (run,release,postgres_lab):monkeypatch.setattr(module,'ROOT',service)
    def refused(*a,**k):raise ValueError('STORAGE_TEST_REFUSAL')
    monkeypatch.setattr(storage,'preflight',refused)
    monkeypatch.setattr(postgres_lab,'validate_tools',lambda *a:{key:'not-executed' for key in ('initdb','pg_ctl','pg_dump','pg_restore','createdb','psql')})
    def unexpected(*a,**k):pytest.fail('subprocess started despite rejected storage admission')
    monkeypatch.setattr(storage.subprocess,'run',unexpected)
    for entry in (lambda:run.execute('M03',None,True),release.main,lambda:postgres_lab.run_lab(tmp_path)):
        with pytest.raises(ValueError,match='STORAGE_TEST_REFUSAL'):entry()
