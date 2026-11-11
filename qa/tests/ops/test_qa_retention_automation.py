"""Automation contract: local filesystem fixtures, no native DB creation."""
import ast
import json
from pathlib import Path
import zipfile

import pytest
from qa import retention,retention_automation as auto,storage
from qa.run import run_lock
from qa.tests.ops.test_qa_retention import make_run
from qa.tests.ops.test_qa_retention_apply import lab  # isolated normal-ACL fixture


def enable(lab,cutoff='2026-11-01T00:00:00+00:00'):
    proof=lab.private/'native-pilot-test-stub.json'
    if not proof.exists():storage.dump_new(proof,{'status':'PASS_NATIVE_STOPPED_SYNTHETIC_PILOT','raw_absent':True,
                 'all_preserved_file_hashes_unchanged':True,'keep_clusters_names_sizes_control_summary_unchanged':True})
    policy={'version':1,'enabled':True,'scope':auto.SCOPE,'not_before':cutoff,
            'max_candidates_per_cycle':1,'pilot_review':'private-backup/'+proof.name}
    path=lab.service.parent/'qa-retention-automation.json'
    path.write_text(json.dumps(policy),encoding='utf-8')
    return path,policy


@pytest.mark.case('Q00-retention-auto-default-disabled')
def test_missing_or_disabled_policy_never_mutates(lab):
    assert auto.cycle(lab.service,Path('.'),'TEST')['status']=='DISABLED'
    path,p=enable(lab);p['enabled']=False;path.write_text(json.dumps(p))
    assert auto.cycle(lab.service,Path('.'),'TEST')['status']=='DISABLED'
    assert all((r/'postgres').exists() for r in lab.runs)


@pytest.mark.case('Q00-retention-auto-future-only')
def test_old_backlog_outside_cutoff_untouched_and_eligible_one_removed(lab):
    enable(lab,'2026-11-02T00:00:00+00:00')
    preserved={r.name:storage.file_hash(r/'summary.json') for r in lab.runs}
    result=auto.cycle(lab.service,Path('.'),'TEST')
    assert result['status']=='DISPOSED_SYNTHETIC_CLUSTER_ONLY'
    assert result['run_id']==lab.runs[1].name and result['out_of_automatic_scope_candidates']==1
    assert (lab.runs[0]/'postgres').exists() and not (lab.runs[1]/'postgres').exists()
    assert all((r/'postgres').exists() for r in lab.runs[2:])
    assert preserved=={r.name:storage.file_hash(r/'summary.json') for r in lab.runs}
    again=auto.cycle(lab.service,Path('.'),'TEST')
    assert again['status']=='NO_IN_SCOPE_CANDIDATE' and (lab.runs[0]/'postgres').exists()


@pytest.mark.case('Q00-retention-auto-one-per-cycle')
def test_two_candidates_require_two_serial_cycles(lab):
    enable(lab)
    first=auto.cycle(lab.service,Path('.'),'TEST');assert first['run_id']==lab.runs[0].name
    assert (lab.runs[1]/'postgres').exists()
    second=auto.cycle(lab.service,Path('.'),'TEST');assert second['run_id']==lab.runs[1].name
    assert all((r/'postgres').exists() for r in lab.runs[2:])


@pytest.mark.case('Q00-retention-auto-cutoff-timestamp-boundary')
def test_started_at_tolerance_cannot_pull_pre_activation_run_into_scope(lab):
    enable(lab,'2026-11-01T00:00:30+00:00')
    summary=retention.read_json(lab.runs[0]/'summary.json')
    summary['started_at']='2026-11-01T00:01:00+00:00'
    (lab.runs[0]/'summary.json').write_text(json.dumps(summary))
    result=auto.cycle(lab.service,Path('.'),'TEST')
    assert result['run_id']==lab.runs[1].name and (lab.runs[0]/'postgres').exists()


@pytest.mark.case('Q00-retention-auto-native-proof-policy')
def test_policy_and_native_pilot_prerequisite_are_fail_closed(lab):
    path,p=enable(lab)
    p['max_candidates_per_cycle']=10;path.write_text(json.dumps(p))
    with pytest.raises(retention.Protected,match='POLICY_INVALID'):auto.cycle(lab.service,Path('.'),'TEST')
    path,p=enable(lab);(lab.private/'native-pilot-test-stub.json').write_text('{"status":"UNVERIFIED"}')
    with pytest.raises(retention.Protected,match='NATIVE_PILOT_REQUIRED'):auto.cycle(lab.service,Path('.'),'TEST')
    assert all((r/'postgres').exists() for r in lab.runs)


@pytest.mark.case('Q00-retention-auto-pin-and-current-run')
def test_pin_and_just_finished_run_excluded_without_weakening_quota(lab):
    enable(lab)
    storage.dump_new(lab.runs[0]/'retention-pin.json',{'version':1,'reason':'Investigating'})
    result=auto.cycle(lab.service,Path('.'),'TEST',exclude_run_ids={lab.runs[1].name})
    assert result['status']=='NO_IN_SCOPE_CANDIDATE'
    assert result['global_decisions']['HOLD']==1
    assert all((r/'postgres').exists() for r in lab.runs)


@pytest.mark.case('Q00-retention-auto-revocation')
def test_policy_revoked_during_backup_refuses_before_intent(lab,monkeypatch):
    path,policy=enable(lab);original=auto.create_context_backup
    def revoke(*args):
        original(*args);policy['enabled']=False;path.write_text(json.dumps(policy))
    monkeypatch.setattr(auto,'create_context_backup',revoke)
    result=auto.safe_cycle(lab.service,Path('.'),'TEST')
    assert result['status']=='RETAINED_REQUIRES_REVIEW' and 'AUTHORIZATION_CHANGED' in result['reason']
    assert all((r/'postgres').exists() and not (r/'retention-intent.json').exists() for r in lab.runs)


@pytest.mark.case('Q00-retention-auto-policy-read-race')
def test_policy_and_digest_from_different_revisions_cannot_authorize(lab,monkeypatch):
    path,policy=enable(lab);original=retention.read_json
    def read(p):
        value=original(p)
        if Path(p)==path:
            policy['not_before']='2026-11-03T00:00:00+00:00';path.write_text(json.dumps(policy))
        return value
    monkeypatch.setattr(retention,'read_json',read)
    result=auto.safe_cycle(lab.service,Path('.'),'TEST')
    assert result['reason']=='AUTOMATION_POLICY_CHANGED_DURING_READ'
    assert all((r/'postgres').exists() for r in lab.runs)


@pytest.mark.case('Q00-retention-auto-source-change')
def test_source_change_during_backup_rejects_stale_plan(lab,monkeypatch):
    enable(lab);original=auto.create_context_backup
    def change(*args):
        original(*args);(lab.service/'fixture.py').write_text('# changed after verified backup\n')
    monkeypatch.setattr(auto,'create_context_backup',change)
    result=auto.safe_cycle(lab.service,Path('.'),'TEST')
    assert result['status']=='RETAINED_REQUIRES_REVIEW' and 'STALE_PLAN_SOURCE' in result['reason']
    assert all((r/'postgres').exists() for r in lab.runs)


@pytest.mark.case('Q00-retention-auto-concurrent-lock')
def test_active_runner_lock_blocks_cycle_without_waiting_or_deleting(lab):
    enable(lab)
    with run_lock(lab.roots['workspace']/'storage.lock'):
        result=auto.safe_cycle(lab.service,Path('.'),'TEST')
    assert result['status']=='RETAINED_REQUIRES_REVIEW'
    assert all((r/'postgres').exists() for r in lab.runs)


@pytest.mark.case('Q00-retention-auto-backup-secret-exclusion')
def test_source_archive_reopened_hashes_and_excludes_secrets(lab):
    enable(lab)
    (lab.service/'.env.staging').write_text('PRIVATE_FIXTURE_ONLY=not-a-real-secret\n')
    (lab.service/'private.key').write_text('FAKE PRIVATE KEY\n')
    result=auto.cycle(lab.service,Path('.'),'TEST')
    job=Path(result['apply_job']);records=retention.read_json(job/'source-and-context-files.json')
    assert any(p['path'].endswith('fixture.py') for p in records)
    assert not any('.env' in p['path'] or p['path'].endswith('.key') for p in records)
    with zipfile.ZipFile(job/'source-and-context.zip') as z:
        for p in records:assert auto.sha256(z.read(p['path'])).hexdigest()==p['sha256']
    assert (lab.service/'.env.staging').exists() and (lab.service/'private.key').exists()


@pytest.mark.case('Q00-retention-auto-hook-lock-order')
def test_lab_hooks_acquire_real_locks_only_outside_body_and_preserve_failure(lab,monkeypatch):
    from qa import run
    monkeypatch.setattr(run,'ROOT',lab.service)
    events=[]
    def hook(service,pg_bin,phase,exclude_run_ids=()):
        # Actual lock acquisition proves the wrapper is outside both body locks.
        with run_lock(lab.roots['workspace']/'storage.lock'),run_lock(lab.roots['workspace']/'postgres-lab.lock'):
            events.append((phase,set(exclude_run_ids)))
        return {'status':'NO_IN_SCOPE_CANDIDATE'}
    monkeypatch.setattr(auto,'safe_cycle',hook)
    @auto.lab_lifecycle
    def lab_body(pg_bin,artifact_parent=None):
        with run_lock(lab.roots['workspace']/'storage.lock'),run_lock(lab.roots['workspace']/'postgres-lab.lock'):
            events.append(('BODY_START',set()));make_run(lab.roots,5);events.append(('BODY_STOP',set()))
        return 1
    assert lab_body(Path('.'))==1
    assert [e[0] for e in events]==['BEFORE_ADMISSION','BODY_START','BODY_STOP','AFTER_LAB_STOP']
    assert events[-1][1]=={'20261105T000000Z-postgres-lab-000000000005'}


@pytest.mark.case('Q00-retention-auto-hook-interruption')
def test_interruption_does_not_trigger_new_disposal(lab,monkeypatch):
    from qa import run
    monkeypatch.setattr(run,'ROOT',lab.service);events=[]
    monkeypatch.setattr(auto,'safe_cycle',lambda s,p,phase,*a:events.append(phase))
    @auto.lab_lifecycle
    def interrupted(pg_bin):raise KeyboardInterrupt('simulated stop')
    with pytest.raises(KeyboardInterrupt):interrupted(Path('.'))
    assert events==['BEFORE_ADMISSION']


@pytest.mark.case('Q00-retention-auto-fixed-root')
def test_external_artifact_parent_refused_before_any_hook(lab,monkeypatch):
    from qa import run
    monkeypatch.setattr(run,'ROOT',lab.service)
    monkeypatch.setattr(auto,'before_run',lambda *a:pytest.fail('external root reached retention'))
    @auto.lab_lifecycle
    def lab_body(pg_bin,artifact_parent=None):pytest.fail('external root reached lab')
    with pytest.raises(ValueError,match='fixed workspace'):lab_body(Path('.'),artifact_parent=lab.private)


@pytest.mark.case('Q00-retention-auto-entrypoint-wiring')
def test_all_entrypoints_wire_hooks_before_admission_without_nested_locks():
    service=Path(__file__).resolve().parents[3]
    for file,function in [('run.py','execute'),('release.py','main')]:
        tree=ast.parse((service/'qa'/file).read_text())
        body=next(n for n in tree.body if isinstance(n,ast.FunctionDef) and n.name==function)
        hooks=[n.lineno for n in ast.walk(body) if isinstance(n,ast.Call) and isinstance(n.func,ast.Attribute) and n.func.attr=='before_run']
        locks=[n.lineno for n in ast.walk(body) if isinstance(n,ast.Call) and isinstance(n.func,ast.Name) and n.func.id=='run_lock']
        assert len(hooks)==1 and hooks[0]<min(locks)
    tree=ast.parse((service/'qa/postgres_lab.py').read_text())
    lab=next(n for n in tree.body if isinstance(n,ast.FunctionDef) and n.name=='run_lab')
    assert any(isinstance(n,ast.Attribute) and n.attr=='lab_lifecycle' for n in lab.decorator_list)


@pytest.mark.case('Q00-retention-auto-core-revocation-guard')
def test_revocation_during_core_revalidation_is_checked_before_intent(lab,monkeypatch):
    path,policy=enable(lab);original=retention.inspect_run;calls=0
    def inspect(label,root,run,*args,**kwargs):
        nonlocal calls
        result=original(label,root,run,*args,**kwargs)
        if run.name==lab.runs[0].name:
            calls+=1
            if calls==3:  # cycle snapshot, engine snapshot, final pre-action inspection
                policy['enabled']=False;path.write_text(json.dumps(policy))
        return result
    monkeypatch.setattr(retention,'inspect_run',inspect)
    result=auto.safe_cycle(lab.service,Path('.'),'TEST')
    assert calls==3 and result['reason']=='AUTOMATION_AUTHORIZATION_CHANGED'
    assert (lab.runs[0]/'postgres').exists() and not (lab.runs[0]/'retention-intent.json').exists()


@pytest.mark.case('Q00-retention-auto-compactor-disposal-aware')
def test_old_compaction_tool_recognizes_disposal_receipt(lab,monkeypatch):
    import importlib.util
    import sys
    enable(lab);result=auto.cycle(lab.service,Path('.'),'TEST')
    actual=Path(__file__).resolve().parents[3]
    spec=importlib.util.spec_from_file_location('retention_compactor_fixture',actual.parent/'portability/manage_qa_storage.py')
    module=importlib.util.module_from_spec(spec);spec.loader.exec_module(module)
    monkeypatch.setattr(module,'BASE',lab.service.parent);monkeypatch.setattr(module,'SERVICE',lab.service)
    monkeypatch.setattr(sys,'argv',['manage_qa_storage.py','--compress-closed-clusters','--run-id',result['run_id']])
    monkeypatch.setattr(storage,'inventory',lambda *a:{'allocated_bytes':0,'logical_bytes':0,'complete':True,'unreadable':[]})
    monkeypatch.setattr(storage,'compact_run',lambda *a:pytest.fail('disposed tree scheduled for compaction'))
    assert module.main()==0
    report=next(lab.private.glob('qa-storage-*'))
    assert retention.read_json(report/'actions.json')==[{'run_id':result['run_id'],'action':'ALREADY_DISPOSED_NOT_COMPRESSED'}]
    assert retention.read_json(report/'results-ledger.json')[0]['current_raw_cluster']=='DISPOSED_VERIFIED_SEPARATE_RECEIPT'
