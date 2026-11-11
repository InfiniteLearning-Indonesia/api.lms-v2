"""One-cluster retention transaction. No SQL, DB startup, or blind resume.

OS locks coordinate cooperating QA runners, not hostile same-user filesystem
changes. Windows path checks are not a security boundary against that actor.
"""
from contextlib import ExitStack
from datetime import datetime, timezone
import json
import os
from pathlib import Path
import shutil
import zipfile

from qa import retention, storage
from qa.evidence import source_fingerprint
from qa.run import run_lock

ACK='delete-verified-synthetic-qa-cluster-only'
STATUS='DISPOSED_SYNTHETIC_CLUSTER_ONLY'


def durable_json(path,value):
    with path.open('x',encoding='utf-8') as handle:
        json.dump(value,handle,indent=2,sort_keys=True);handle.write('\n')
        handle.flush();os.fsync(handle.fileno())


def append_event(job,event,**values):
    # The job directory is exclusive per invocation; append+fsync before action.
    with (job/'journal.jsonl').open('a',encoding='utf-8') as handle:
        handle.write(json.dumps({'event':event,'at':storage.stamp(),**values},sort_keys=True)+'\n')
        handle.flush();os.fsync(handle.fileno())


def context_backup(job,source):
    """Verify a small source/context archive, independently of logical DB backups."""
    receipt=retention.read_json(job/'backup-receipt.json')
    archive=retention.safe_path(job/'source-and-context.zip')
    if (not isinstance(receipt,dict) or receipt.get('source_hash')!=source or receipt.get('database_access')!='NONE'
            or receipt.get('reopened_byte_verification')!='PASS'
            or receipt.get('sha256')!=storage.file_hash(archive)):
        raise retention.Protected('CONTEXT_BACKUP_NOT_VERIFIED')
    try:
        with zipfile.ZipFile(archive) as bundle:
            if not bundle.namelist() or bundle.testzip() is not None:
                raise retention.Protected('CONTEXT_BACKUP_INVALID')
    except zipfile.BadZipFile:raise retention.Protected('CONTEXT_BACKUP_INVALID')
    return {'archive':str(archive),'sha256':receipt['sha256'],'source_hash':source,
            'database_backup_scope':'Existing per-migration synthetic SQL/custom pairs; NOT a final-state snapshot.'}


def apply_one(service,plan_file,plan_sha256,run_id,pg_bin,job,*,apply=False,ack=None,pre_action_guard=None):
    """Preview by default. Apply deletes at most one reviewed postgres/ payload.

    Callers must create an exclusive job under fixed private-backup and provide
    verified source/context backup there before apply. No arbitrary roots/paths.
    An interrupted intent requires manual review; successful retries are no-op.
    """
    service=retention.safe_path(service)
    locations=storage.project_locations(service)
    roots={'workspace':locations[0],'legacy':locations[1]}
    roots={k:retention.safe_path(v) for k,v in roots.items()}
    private=retention.safe_path(service.parent/'private-backup')
    job=retention.safe_path(job)
    import re
    if job.parent!=private or not re.fullmatch(r'qa-retention-apply-[a-f0-9]{32}',job.name):
        raise retention.Protected('JOB_OUTSIDE_FIXED_ROOT')
    plan_file=retention.safe_path(plan_file)
    if not plan_file.is_relative_to(private) or not retention.HASH.fullmatch(plan_sha256):
        raise retention.Protected('PLAN_OUTSIDE_FIXED_ROOT_OR_HASH_INVALID')
    if apply and ack!=ACK:raise retention.Protected('EXPLICIT_SYNTHETIC_DISPOSAL_ACK_REQUIRED')
    if not storage.RUN_NAME.fullmatch(run_id):raise retention.Protected('INVALID_RUN_ID')
    with ExitStack() as stack:
        for root in roots.values():
            for name in ('storage.lock','postgres-lab.lock','local-synthetic.lock','release.lock'):
                lock=root/name
                if (lock.exists() or lock.is_symlink()) and storage.reparse(lock):raise retention.Protected('REPARSE_LOCK')
                stack.enter_context(run_lock(lock))
        if storage.file_hash(plan_file)!=plan_sha256:raise retention.Protected('PLAN_HASH_MISMATCH')
        plan=retention.read_json(plan_file)
        source=source_fingerprint(service)
        pins_file=service.parent/'qa-retention-pins.json'
        pins_hash=storage.file_hash(pins_file);pins=retention.load_pins(pins_file)
        if (not isinstance(plan,dict) or plan.get('version')!=2 or plan.get('mode')!='DRY_RUN_ONLY'
                or plan.get('roots')!={k:str(v) for k,v in roots.items()}
                or plan.get('policy')!={'version':3,'keep_pass':2,'keep_failed':1,'scope':'GLOBAL_ACROSS_FIXED_ROOTS'}):
            raise retention.Protected('PLAN_CONTRACT_MISMATCH_REGENERATE')
        try:
            created=datetime.fromisoformat(plan['created_at'])
            age=(datetime.now(timezone.utc)-created).total_seconds()
            if not -120<=age<=86400:raise ValueError()
        except (ValueError,TypeError,KeyError):raise retention.Protected('PLAN_EXPIRED_OR_INVALID_TIME')
        if source!=plan.get('planner_source_hash'):raise retention.Protected('STALE_PLAN_SOURCE')
        if pins_hash!=plan.get('pins_sha256'):raise retention.Protected('STALE_PLAN_PINS')
        matches=[r for r in plan['rows'] if r['run_id']==run_id]
        if len(matches)!=1 or matches[0]['decision']!='CANDIDATE':raise retention.Protected('NOT_ONE_REVIEWED_CANDIDATE')
        reviewed=matches[0];label=reviewed['root']
        run=retention.safe_path(roots[label]/run_id);data=run/'postgres'
        if run.parent!=roots[label] or reviewed['run_path']!=str(run) or reviewed['cluster_path']!=str(data):
            raise retention.Protected('REVIEWED_PATH_MISMATCH')
        # Refresh global quotas and all per-run gates; never trust the old path list.
        fresh=retention.build_plan(roots,pins,pg_bin,measure_candidates=False)
        current=next(r for r in fresh['rows'] if r['root']==label and r['run_id']==run_id)
        if current['decision']=='DISPOSED':
            receipt=retention.disposed_receipt(run)
            if receipt.get('plan_sha256')!=plan_sha256:raise retention.Protected('DISPOSAL_PLAN_MISMATCH')
            return {'status':'ALREADY_DISPOSED_NO_ACTION','run_id':run_id,'deletion_this_invocation':'NONE'}
        if current['decision']!='CANDIDATE':raise retention.Protected('NO_LONGER_CANDIDATE_'+current['reason'])
        fields=('summary_sha256','case_evidence','backup_evidence','cluster_identity','cluster_file_count')
        if any(current.get(k)!=reviewed.get(k) for k in fields):raise retention.Protected('REVIEWED_EVIDENCE_OR_CLUSTER_CHANGED')
        if not apply:return {'status':'PREVIEW_ONLY','run_id':run_id,'cluster_path':str(data),'deletion_this_invocation':'NONE'}
        if (job/'journal.jsonl').exists() or (job/'receipt.json').exists():raise retention.Protected('JOB_ALREADY_USED')
        backup=context_backup(job,source)
        # Last checks after backup verification, before persistent intent/mutation.
        final=retention.inspect_run(label,roots[label],run,pins,pg_bin)
        if final['decision']!='ELIGIBLE' or any(final.get(k)!=reviewed.get(k) for k in fields):
            raise retention.Protected('PRE_ACTION_REVALIDATION_FAILED')
        if source_fingerprint(service)!=source or storage.file_hash(pins_file)!=pins_hash:
            raise retention.Protected('SOURCE_OR_PINS_CHANGED_DURING_APPLY')
        if pre_action_guard is not None:pre_action_guard()
        before=storage.measure(data)
        free_before=shutil.disk_usage(data).free
        identity=retention.cluster_identity(data)
        append_event(job,'INTENT',run_id=run_id,cluster_path=str(data),plan_sha256=plan_sha256,
                     source_hash=source,cluster_identity=identity)
        durable_json(run/'retention-intent.json',{'version':1,'job_path':str(job),'run_id':run_id,
                                                 'plan_sha256':plan_sha256,'scope':'SYNTHETIC_POSTGRES_DIRECTORY_ONLY'})
        try:
            # Verify resolved absolute target is exactly run/postgres in a fixed
            # QA root. Never pass an external or computed-unchecked tree to rmtree.
            if retention.safe_path(data)!=data or data.parent!=run or run.parent!=roots[label]:
                raise retention.Protected('PRE_DELETE_PATH_CHANGED')
            if retention.cluster_identity(data)!=identity or (data/'postmaster.pid').exists():
                raise retention.Protected('PRE_DELETE_CLUSTER_CHANGED_OR_ACTIVE')
            for _ in storage.files(data):pass  # reject nested reparse/special files
            append_event(job,'DELETE_START',run_id=run_id)
            shutil.rmtree(data)
            if data.exists() or data.is_symlink():raise retention.Protected('CLUSTER_STILL_PRESENT')
            if (storage.file_hash(run/'summary.json')!=reviewed['summary_sha256']
                    or retention.backup_evidence(run)!=reviewed['backup_evidence']
                    or retention.verify_cases(run,retention.read_json(run/'summary.json'),reviewed['bucket']=='pass')!=reviewed['case_evidence']):
                raise retention.Protected('PRESERVED_EVIDENCE_CHANGED')
            receipt={'version':1,'status':STATUS,'run_id':run_id,'root':label,'run_path':str(run),
                     'cluster_path':str(data),'plan_sha256':plan_sha256,'source_hash':source,
                     'summary_sha256':reviewed['summary_sha256'],'case_evidence':reviewed['case_evidence'],
                     'backup_evidence':reviewed['backup_evidence'],'context_backup':backup,
                     'before':before,'after':{'logical_bytes':0,'allocated_bytes':0,'files':0},
                     'removed_payload_allocated_bytes':before['allocated_bytes'],
                     'observed_volume_free_delta_bytes':shutil.disk_usage(run).free-free_before,
                     'space_note':'Payload allocation removed; volume delta also includes concurrent disk writes and journal/marker overhead.',
                     'database_connections':'NONE','sql_operations':'NONE','original_qa_status_unchanged':True}
            durable_json(job/'receipt.json',receipt)
            durable_json(run/'retention-disposal.json',{'version':1,'status':STATUS,'run_id':run_id,
                         'receipt_path':str(job/'receipt.json'),'receipt_sha256':storage.file_hash(job/'receipt.json')})
            append_event(job,'COMPLETE',run_id=run_id,receipt_sha256=storage.file_hash(job/'receipt.json'))
            return receipt
        except BaseException as error:
            # This includes interruption; never retry a partially removed tree.
            try:append_event(job,'NEEDS_OPERATOR_REVIEW',run_id=run_id,error_type=type(error).__name__)
            except OSError:pass  # intent already persists even if journal cannot grow
            raise
