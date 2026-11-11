"""Serialized retention hooks. Explicit policy enables future synthetic runs only.

Hooks run OUTSIDE runner locks. apply_one takes the same locks and revalidates
the plan after the snapshot/backup gap. Old backlog never becomes implicit scope.
"""
from contextlib import ExitStack
from datetime import datetime, timezone
from functools import wraps
from hashlib import sha256
from inspect import signature
import json
from pathlib import Path
import shutil
import uuid
import zipfile

from qa import storage
from qa.evidence import source_fingerprint

SCOPE='FUTURE_LOCAL_SYNTHETIC_RUNS_ONLY'
SKIP={'.git','.venv','.cache','.qa-artifacts','.pytest_cache','__pycache__','node_modules','dist','build',
      '.agents','.codex','.next','graphify-out','coverage','.qa-tmp-manifest-20261030a'}
SECRET_SUFFIXES={'.key','.pem','.crt','.pfx','.p12'}


def read_policy(service):
    from qa import retention
    path=service.parent/'qa-retention-automation.json'
    if not path.exists():return None,None
    retention.safe_path(path)
    before_hash=storage.file_hash(path)
    value=retention.read_json(path)
    keys={'version','enabled','scope','not_before','max_candidates_per_cycle','pilot_review'}
    if (not isinstance(value,dict) or set(value)!=keys or value['version']!=1
            or type(value['enabled']) is not bool or value['scope']!=SCOPE
            or type(value['max_candidates_per_cycle']) is not int or value['max_candidates_per_cycle']!=1):
        raise retention.Protected('AUTOMATION_POLICY_INVALID')
    try:
        at=datetime.fromisoformat(value['not_before'])
        if at.tzinfo is None:raise ValueError()
        if at>datetime.now(timezone.utc):raise ValueError()
    except (ValueError,TypeError):raise retention.Protected('AUTOMATION_CUTOFF_INVALID')
    # A reviewed native one-cluster pilot is the activation prerequisite.
    private=retention.safe_path(service.parent/'private-backup')
    review=retention.safe_path(service.parent/value['pilot_review'])
    if not review.is_relative_to(private):raise retention.Protected('AUTOMATION_PILOT_OUTSIDE_BACKUP')
    proof=retention.read_json(review)
    if (not isinstance(proof,dict) or proof.get('status')!='PASS_NATIVE_STOPPED_SYNTHETIC_PILOT' or proof.get('raw_absent') is not True
            or proof.get('all_preserved_file_hashes_unchanged') is not True
            or proof.get('keep_clusters_names_sizes_control_summary_unchanged') is not True):
        raise retention.Protected('AUTOMATION_NATIVE_PILOT_REQUIRED')
    if storage.file_hash(path)!=before_hash:raise retention.Protected('AUTOMATION_POLICY_CHANGED_DURING_READ')
    return value,before_hash


def create_context_backup(service,job,source,plan_file):
    """Small immutable source/context ZIP; excludes raw DBs, caches and secrets."""
    from qa import retention,retention_apply
    workspace=service.parent.parent
    inputs=[]
    for base in (service,service.parent/'portability'):
        if not base.exists():continue
        stack=[retention.safe_path(base)]
        while stack:
            folder=stack.pop()
            for p in sorted(folder.iterdir()):
                if p.name in SKIP:continue
                if storage.reparse(p):raise retention.Protected('CONTEXT_REPARSE_INPUT')
                if p.is_dir():stack.append(p);continue
                if p.name.startswith('.env') or p.suffix.lower() in SECRET_SUFFIXES:continue
                if not p.is_file():raise retention.Protected('CONTEXT_SPECIAL_INPUT')
                inputs.append(p)
    inputs += [plan_file,service.parent/'qa-retention-automation.json',service.parent/'qa-retention-pins.json']
    if (service.parent/'START_HERE.md').exists():inputs.append(service.parent/'START_HERE.md')
    archive=job/'source-and-context.zip';records=[]
    with zipfile.ZipFile(archive,'x',compression=zipfile.ZIP_DEFLATED,compresslevel=6) as bundle:
        for p in sorted(set(inputs)):
            p=retention.safe_path(p);name=p.relative_to(workspace).as_posix()
            info=p.stat();digest=sha256()
            with p.open('rb') as src,bundle.open(name,'w',force_zip64=True) as out:
                for chunk in iter(lambda:src.read(1024*1024),b''):out.write(chunk);digest.update(chunk)
            after=p.stat()
            if (info.st_size,info.st_mtime_ns)!=(after.st_size,after.st_mtime_ns):raise retention.Protected('CONTEXT_CHANGED_DURING_BACKUP')
            records.append({'path':name,'sha256':digest.hexdigest(),'bytes':info.st_size})
    with zipfile.ZipFile(archive) as bundle:
        for record in records:
            digest=sha256()
            with bundle.open(record['path']) as src:
                for chunk in iter(lambda:src.read(1024*1024),b''):digest.update(chunk)
            if digest.hexdigest()!=record['sha256']:raise retention.Protected('CONTEXT_ARCHIVE_HASH_MISMATCH')
    if source_fingerprint(service)!=source:raise retention.Protected('CONTEXT_SOURCE_CHANGED')
    retention_apply.durable_json(job/'source-and-context-files.json',records)
    retention_apply.durable_json(job/'backup-receipt.json',{'source_hash':source,'sha256':storage.file_hash(archive),
                         'reopened_byte_verification':'PASS','database_access':'NONE','files':len(records),
                         'configuration_secrets_excluded':True})


def cycle(service,pg_bin,phase,exclude_run_ids=()):
    """One fresh snapshot + at most one apply; caller must hold no runner lock."""
    from qa import retention,retention_apply
    from qa.run import run_lock
    service=Path(service).resolve(strict=True)
    policy,policy_hash=read_policy(service)
    if not policy or not policy['enabled']:return {'status':'DISABLED','phase':phase}
    cutoff=datetime.fromisoformat(policy['not_before'])
    locations=storage.project_locations(service)
    roots={'workspace':locations[0],'legacy':locations[1]}
    for root in roots.values():retention.safe_path(root)
    report=service.parent/'private-backup'/('qa-retention-cycle-'+uuid.uuid4().hex);report.mkdir()
    try:
        source=source_fingerprint(service)
        pins_file=service.parent/'qa-retention-pins.json';pins_hash=storage.file_hash(pins_file)
        pins=retention.load_pins(pins_file)
        with ExitStack() as stack:
            for root in roots.values():
                for name in ('storage.lock','postgres-lab.lock','local-synthetic.lock','release.lock'):
                    lock=root/name
                    if (lock.exists() or lock.is_symlink()) and storage.reparse(lock):raise retention.Protected('REPARSE_LOCK')
                    stack.enter_context(run_lock(lock))
            plan=retention.build_plan(roots,pins,pg_bin,measure_candidates=False)
            if source_fingerprint(service)!=source or storage.file_hash(pins_file)!=pins_hash:
                raise retention.Protected('AUTOMATION_SOURCE_OR_PINS_CHANGED')
            plan.update(planner_source_hash=source,pins_sha256=pins_hash)
            retention_apply.durable_json(report/'plan.json',plan)
        candidates=[r for r in plan['rows'] if r['decision']=='CANDIDATE']
        selected=[r for r in candidates if datetime.fromisoformat(r['run_time_utc'])>=cutoff
                  and datetime.strptime(r['run_id'][:16],'%Y%m%dT%H%M%SZ').replace(tzinfo=timezone.utc)>=cutoff
                  and r['run_id'] not in exclude_run_ids]
        selected.sort(key=lambda r:(r['run_time_utc'],r['root'],r['run_id']))
        result={'phase':phase,'source_hash':source,'policy_sha256':policy_hash,'target_scope':SCOPE,
                'not_before':policy['not_before'],'global_decisions':plan['decision_counts'],
                'protected_exceptions':len(plan['non_run_exceptions'])+plan['decision_counts'].get('HOLD',0),
                'out_of_automatic_scope_candidates':len(candidates)-len(selected),
                'plan_sha256':storage.file_hash(report/'plan.json'),'report':str(report)}
        if not selected:result.update(status='NO_IN_SCOPE_CANDIDATE',automatic_deletion='NONE')
        else:
            chosen=selected[0]
            job=service.parent/'private-backup'/('qa-retention-apply-'+uuid.uuid4().hex);job.mkdir()
            create_context_backup(service,job,source,report/'plan.json')
            def guard():
                current,current_hash=read_policy(service)
                if current_hash!=policy_hash or not current['enabled']:
                    raise retention.Protected('AUTOMATION_AUTHORIZATION_CHANGED')
            guard()
            action=retention_apply.apply_one(service,report/'plan.json',result['plan_sha256'],chosen['run_id'],
                            pg_bin,job,apply=True,ack=retention_apply.ACK,pre_action_guard=guard)
            result.update(status=action['status'],run_id=chosen['run_id'],apply_job=str(job))
        retention_apply.durable_json(report/'summary.json',result)
        return result
    except Exception as error:
        # Failed housekeeping is visible separately, never a new business QA PASS.
        retention_apply.durable_json(report/'summary.json',{'status':'RETAINED_REQUIRES_REVIEW','phase':phase,
                     'error_type':type(error).__name__,'reason':str(error),'original_qa_status_changed':False})
        raise


def safe_cycle(service,pg_bin,phase,exclude_run_ids=()):
    try:
        result=cycle(service,pg_bin,phase,exclude_run_ids)
    except Exception as error:
        result={'status':'RETAINED_REQUIRES_REVIEW','phase':phase,'reason':str(error),
                'error_type':type(error).__name__,'original_qa_status_changed':False}
    if result['status']!='DISABLED':print('[qa-retention] '+json.dumps(result),flush=True)
    return result


def before_run(service,pg_bin=None):
    if pg_bin is None:
        found=shutil.which('pg_controldata')
        pg_bin=Path(found).parent if found else Path(r'C:\Program Files\PostgreSQL\18\bin')
    return safe_cycle(service,pg_bin,'BEFORE_ADMISSION')


def lab_lifecycle(function):
    """Keep existing lab signature, native shutdown and QA exit code unchanged."""
    sig=signature(function)
    @wraps(function)
    def wrapped(*args,**kwargs):
        from qa.run import ROOT
        arguments=sig.bind(*args,**kwargs).arguments
        parent=ROOT.parent.parent/'lms-v3-qa-artifacts'
        supplied=arguments.get('artifact_parent')
        if supplied is not None and Path(supplied).resolve()!=parent.resolve():
            raise ValueError('artifact parent must be the fixed workspace QA root')
        parent.mkdir(exist_ok=True)
        before_run(ROOT,arguments['pg_bin'])
        known={p.name for p in parent.iterdir() if storage.RUN_NAME.fullmatch(p.name)}
        code=function(*args,**kwargs)  # original finally stops PostgreSQL under lock
        fresh={p.name for p in parent.iterdir() if storage.RUN_NAME.fullmatch(p.name)}-known
        safe_cycle(ROOT,arguments['pg_bin'],'AFTER_LAB_STOP',fresh)
        return code
    return wrapped
