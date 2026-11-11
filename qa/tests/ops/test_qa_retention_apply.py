"""Filesystem fixtures only: real unlink, mocked pg_controldata; no real cluster."""
import json
from pathlib import Path
from types import SimpleNamespace
import uuid
import zipfile

import pytest
from qa import retention,retention_apply as apply,storage
from qa.evidence import source_fingerprint
from qa.run import run_lock
from qa.tests.ops.test_qa_retention import make_run,row


@pytest.fixture
def lab(monkeypatch):
    actual=Path(__file__).resolve().parents[3]
    base=actual.parent.parent/'lms-v3-qa-artifacts'/('retention-apply-fixture-'+uuid.uuid4().hex)
    service=base/'backend-v3/backend-go';service.mkdir(parents=True)
    (service/'fixture.py').write_text('# synthetic source, not app code\n')
    private=service.parent/'private-backup';private.mkdir()
    pins=service.parent/'qa-retention-pins.json';storage.dump_new(pins,{'version':1,'pins':[]})
    loc=storage.project_locations(service);roots={'workspace':loc[0],'legacy':loc[1]}
    for p in roots.values():p.mkdir(parents=True)
    monkeypatch.setattr(storage.subprocess,'run',lambda *a,**k:SimpleNamespace(returncode=0,stdout='Database cluster state: shut down\n'))
    runs=[make_run(roots,day,'workspace' if day%2 else 'legacy') for day in (1,2,3,4)]
    plan=retention.build_plan(roots,{},Path('.'))
    plan.update(planner_source_hash=source_fingerprint(service),pins_sha256=storage.file_hash(pins))
    planfile=private/'plan.json';storage.dump_new(planfile,plan)
    return SimpleNamespace(service=service,private=private,pins=pins,roots=roots,runs=runs,
                           plan=plan,planfile=planfile,sha=storage.file_hash(planfile),source=source_fingerprint(service))


def job(lab):
    path=lab.private/('qa-retention-apply-'+uuid.uuid4().hex);path.mkdir()
    archive=path/'source-and-context.zip'
    with zipfile.ZipFile(archive,'x') as bundle:
        bundle.write(lab.service/'fixture.py','fixture.py')
    storage.dump_new(path/'backup-receipt.json',{'sha256':storage.file_hash(archive),'source_hash':lab.source,
                                              'database_access':'NONE','reopened_byte_verification':'PASS'})
    return path


def execute(lab,run=None,**kw):
    return apply.apply_one(lab.service,lab.planfile,lab.sha,(run or lab.runs[0]).name,Path('.'),
                           kw.pop('job',None) or job(lab),**kw)


def evidence(run):
    return {p.name:storage.file_hash(p) for p in run.iterdir() if p.is_file() and not p.name.startswith('retention-')}


@pytest.mark.case('Q00-retention-apply-preview')
def test_default_preview_and_explicit_ack(lab):
    run=lab.runs[0];before=storage.tree_digest(run)
    assert execute(lab)['status']=='PREVIEW_ONLY'
    with pytest.raises(retention.Protected,match='ACK_REQUIRED'):execute(lab,apply=True)
    assert before==storage.tree_digest(run)
    assert not (run/'retention-intent.json').exists()


@pytest.mark.case('Q00-retention-apply-preservation')
def test_actual_fixture_disposal_preserves_evidence_backups_and_global_quota(lab):
    preserved={r.name:evidence(r) for r in lab.runs};j=job(lab)
    before=storage.measure(lab.runs[0]/'postgres')
    result=execute(lab,apply=True,ack=apply.ACK,job=j)
    assert result['status']==apply.STATUS and result['before']==before
    assert result['removed_payload_allocated_bytes']==before['allocated_bytes']>0
    assert not (lab.runs[0]/'postgres').exists()
    assert result['after']['allocated_bytes']==0
    assert preserved=={r.name:evidence(r) for r in lab.runs}
    assert all((r/'postgres').exists() for r in lab.runs[1:])
    events=[json.loads(s)['event'] for s in (j/'journal.jsonl').read_text().splitlines()]
    assert events==['INTENT','DELETE_START','COMPLETE']
    assert retention.disposed_receipt(lab.runs[0])['plan_sha256']==lab.sha
    plan=retention.build_plan(lab.roots,{},Path('.'))
    assert plan['decision_counts']=={'DISPOSED':1,'KEEP':2,'CANDIDATE':1}
    assert execute(lab,apply=True,ack=apply.ACK)['status']=='ALREADY_DISPOSED_NO_ACTION'
    assert preserved=={r.name:evidence(r) for r in lab.runs}


@pytest.mark.parametrize('change',[
    pytest.param('source',marks=pytest.mark.case('Q00-retention-apply-stale-source')),
    pytest.param('pins',marks=pytest.mark.case('Q00-retention-apply-stale-pins')),
    pytest.param('plan-hash',marks=pytest.mark.case('Q00-retention-apply-plan-hash')),
    pytest.param('contract',marks=pytest.mark.case('Q00-retention-apply-contract')),
    pytest.param('time',marks=pytest.mark.case('Q00-retention-apply-expired-plan')),
    pytest.param('path',marks=pytest.mark.case('Q00-retention-apply-path')),
    pytest.param('control',marks=pytest.mark.case('Q00-retention-apply-control')),
    pytest.param('cases',marks=pytest.mark.case('Q00-retention-apply-cases')),
    pytest.param('backup',marks=pytest.mark.case('Q00-retention-apply-backup')),
    pytest.param('new-pin',marks=pytest.mark.case('Q00-retention-apply-new-pin')),
    pytest.param('active',marks=pytest.mark.case('Q00-retention-apply-active')),
    pytest.param('scope',marks=pytest.mark.case('Q00-retention-apply-scope')),
    pytest.param('quota',marks=pytest.mark.case('Q00-retention-apply-quota'))])
def test_changes_after_review_refuse_before_intent(lab,change):
    run=lab.runs[0]
    if change=='source':(lab.service/'fixture.py').write_text('# changed\n')
    elif change=='pins':lab.pins.write_text('{"version":1,"pins":[]}\n')
    elif change=='plan-hash':lab.planfile.write_text(lab.planfile.read_text()+'\n')
    elif change in ('contract','path','time'):
        if change=='contract':lab.plan['version']=1
        elif change=='time':lab.plan['created_at']='2020-01-01T00:00:00+00:00'
        else:row(lab.plan,run)['cluster_path']=str(lab.private)
        lab.planfile.write_text(json.dumps(lab.plan));lab.sha=storage.file_hash(lab.planfile)
    elif change=='control':(run/'postgres/global/pg_control').write_bytes(b'changed')
    elif change=='cases':(run/'unit-cases.json').write_text('[{"status":"skip"}]')
    elif change=='backup':(run/'fixture.dump').write_bytes(b'corrupt')
    elif change=='new-pin':storage.dump_new(run/'retention-pin.json',{'version':1,'reason':'New investigation'})
    elif change=='active':(run/'postgres/postmaster.pid').write_text('synthetic fixture')
    elif change=='scope':
        summary=retention.read_json(run/'summary.json');summary['shared_database_access']='PRODUCTION'
        (run/'summary.json').write_text(json.dumps(summary))
    elif change=='quota':
        # New holds remove newer runs from ordinary quota: target becomes KEEP.
        for r in lab.runs[1:]:storage.dump_new(r/'retention-pin.json',{'version':1,'reason':'Investigate'})
    with pytest.raises(retention.Protected):execute(lab,apply=True,ack=apply.ACK)
    assert (run/'postgres').exists() and not (run/'retention-intent.json').exists()


@pytest.mark.case('Q00-retention-apply-backup-lock-path')
def test_bad_context_backup_lock_and_reparse_are_protected(lab,monkeypatch):
    j=job(lab);(j/'source-and-context.zip').write_bytes(b'broken')
    with pytest.raises(retention.Protected,match='BACKUP'):execute(lab,apply=True,ack=apply.ACK,job=j)
    with run_lock(lab.roots['workspace']/'storage.lock'):
        with pytest.raises(OSError):execute(lab,apply=True,ack=apply.ACK)
    original=storage.reparse
    bad=lab.runs[0]/'postgres'
    monkeypatch.setattr(storage,'reparse',lambda p:True if Path(p)==bad else original(p))
    with pytest.raises(retention.Protected):execute(lab,apply=True,ack=apply.ACK)
    assert bad.exists() and not (lab.runs[0]/'retention-intent.json').exists()


@pytest.mark.parametrize('partial',[
    pytest.param(False,marks=pytest.mark.case('Q00-retention-apply-interruption-before-delete')),
    pytest.param(True,marks=pytest.mark.case('Q00-retention-apply-interruption-partial-delete'))])
def test_interruption_requires_review_and_never_blindly_resumes(lab,monkeypatch,partial):
    run=lab.runs[0];before=evidence(run);j=job(lab)
    def interrupted(path):
        assert Path(path).resolve()==(run/'postgres').resolve()
        assert (run/'retention-intent.json').exists() and 'DELETE_START' in (j/'journal.jsonl').read_text()
        if partial:(Path(path)/'page').unlink()
        raise KeyboardInterrupt('simulated fixture interruption')
    monkeypatch.setattr(apply.shutil,'rmtree',interrupted)
    with pytest.raises(KeyboardInterrupt):execute(lab,apply=True,ack=apply.ACK,job=j)
    assert 'NEEDS_OPERATOR_REVIEW' in (j/'journal.jsonl').read_text()
    assert (run/'postgres').exists() and not (run/'retention-disposal.json').exists()
    assert before==evidence(run)
    plan=retention.build_plan(lab.roots,{},Path('.'))
    assert row(plan,run)['reason']=='INCOMPLETE_DISPOSAL_REQUIRES_REVIEW'
    with pytest.raises(retention.Protected,match='REQUIRES_REVIEW'):execute(lab,apply=True,ack=apply.ACK)


@pytest.mark.case('Q00-retention-apply-protected-retry')
def test_keep_duplicate_and_reappeared_cluster_refuse(lab):
    with pytest.raises(retention.Protected,match='CANDIDATE'):execute(lab,lab.runs[-1],apply=True,ack=apply.ACK)
    execute(lab,apply=True,ack=apply.ACK)
    (lab.runs[0]/'postgres').mkdir()
    with pytest.raises(retention.Protected,match='REAPPEARED'):execute(lab,apply=True,ack=apply.ACK)


@pytest.mark.case('Q00-retention-apply-job-boundary')
def test_job_outside_fixed_private_backup_is_refused(lab):
    j=lab.roots['workspace']/('qa-retention-apply-'+uuid.uuid4().hex);j.mkdir()
    with pytest.raises(retention.Protected,match='JOB_OUTSIDE'):execute(lab,apply=True,ack=apply.ACK,job=j)
    assert all((r/'postgres').exists() for r in lab.runs)


@pytest.mark.case('Q00-retention-apply-last-minute-pin')
def test_pin_added_during_context_verification_refuses(lab,monkeypatch):
    original=apply.context_backup;run=lab.runs[0]
    def add_pin(job,source):
        value=original(job,source)
        storage.dump_new(run/'retention-pin.json',{'version':1,'reason':'Late investigation'})
        return value
    monkeypatch.setattr(apply,'context_backup',add_pin)
    with pytest.raises(retention.Protected,match='PRE_ACTION'):execute(lab,apply=True,ack=apply.ACK)
    assert (run/'postgres').exists() and not (run/'retention-intent.json').exists()


@pytest.mark.case('Q00-retention-apply-replaced-directory')
def test_identical_control_bytes_in_replaced_directory_still_refused(lab):
    run=lab.runs[0];data=(run/'postgres').resolve();saved=run/'fixture-saved-cluster'
    # Verified fixture-local rename, no shell or recursive move.
    assert data.parent==run and not saved.exists()
    data.rename(saved);(data/'global').mkdir(parents=True)
    for source,_,_ in storage.files(saved):
        dest=data/source.relative_to(saved);dest.parent.mkdir(parents=True,exist_ok=True);dest.write_bytes(source.read_bytes())
    with pytest.raises(retention.Protected,match='CLUSTER_CHANGED'):execute(lab,apply=True,ack=apply.ACK)
    assert data.exists() and saved.exists() and not (run/'retention-intent.json').exists()


@pytest.mark.case('Q00-retention-apply-tampered-disposal-receipt')
def test_completed_marker_does_not_hide_changed_receipt(lab):
    j=job(lab);execute(lab,apply=True,ack=apply.ACK,job=j)
    receipt=j/'receipt.json';receipt.write_text(receipt.read_text()+'\n')
    plan=retention.build_plan(lab.roots,{},Path('.'))
    assert row(plan,lab.runs[0])['reason']=='INVALID_DISPOSAL_RECEIPT'
    with pytest.raises(retention.Protected):execute(lab,apply=True,ack=apply.ACK)


@pytest.mark.case('Q00-retention-apply-interruption-before-receipt')
def test_interruption_after_raw_removal_does_not_claim_disposed(lab,monkeypatch):
    run=lab.runs[0];before=evidence(run);real_remove=apply.shutil.rmtree
    def interrupted(path):
        assert Path(path).resolve()==(run/'postgres').resolve()
        real_remove(path)
        raise KeyboardInterrupt('simulated crash before receipt')
    monkeypatch.setattr(apply.shutil,'rmtree',interrupted)
    with pytest.raises(KeyboardInterrupt):execute(lab,apply=True,ack=apply.ACK)
    assert not (run/'postgres').exists() and before==evidence(run)
    assert row(retention.build_plan(lab.roots,{},Path('.')),run)['reason']=='INCOMPLETE_DISPOSAL_REQUIRES_REVIEW'
    with pytest.raises(retention.Protected,match='REQUIRES_REVIEW'):execute(lab,apply=True,ack=apply.ACK)


@pytest.mark.case('Q00-retention-apply-malformed-marker')
def test_non_object_disposal_marker_holds_instead_of_crashing_scan(lab):
    run=lab.runs[0];(run/'retention-disposal.json').write_text('[]')
    # A raw cluster still present is protected before trusting any marker.
    assert row(retention.build_plan(lab.roots,{},Path('.')),run)['reason']=='DISPOSED_CLUSTER_REAPPEARED'
    with pytest.raises(retention.Protected,match='INVALID_DISPOSAL_MARKER'):retention.disposed_receipt(run)
    with pytest.raises(retention.Protected):execute(lab,apply=True,ack=apply.ACK)
