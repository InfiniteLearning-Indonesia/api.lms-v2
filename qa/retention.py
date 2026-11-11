"""Read-only retention inspection/selection; mutations live in retention_apply."""
from collections import Counter
from datetime import datetime, timezone
from hashlib import sha256
import json
from pathlib import Path
import re
import subprocess

from qa import storage

HASH=re.compile(r'^[a-f0-9]{64}$')
ROOT_LABELS={'workspace','legacy'}


class Protected(ValueError):
    pass


def safe_path(path):
    path=Path(path).absolute()
    for part in (path,*path.parents):
        if part.exists() and storage.reparse(part):raise Protected('REPARSE_PATH')
    return path.resolve(strict=True)


def read_json(path):
    if storage.reparse(path):raise Protected('REPARSE_EVIDENCE')
    if path.stat().st_size>8*1024*1024:raise Protected('EVIDENCE_SIZE_BOUND')
    def unique(pairs):
        value={}
        for key,item in pairs:
            if key in value:raise Protected('DUPLICATE_JSON_KEY')
            value[key]=item
        return value
    return json.loads(path.read_text(encoding='utf-8'),object_pairs_hook=unique)


def load_pins(path):
    value=read_json(path)
    if not isinstance(value,dict) or set(value)!={'version','pins'} or value['version']!=1 or not isinstance(value['pins'],list):
        raise Protected('INVALID_PIN_REGISTRY')
    pins={}
    for item in value['pins']:
        if (not isinstance(item,dict) or set(item)!={'root','run_id','reason'} or item['root'] not in ROOT_LABELS
                or not storage.RUN_NAME.fullmatch(item['run_id']) or not isinstance(item['reason'],str)
                or not item['reason'].strip() or len(item['reason'])>500):raise Protected('INVALID_PIN_ENTRY')
        key=(item['root'],item['run_id'])
        if key in pins:raise Protected('DUPLICATE_PIN')
        pins[key]=item['reason']
    return pins


def run_time(value,run_id):
    fallback=datetime.strptime(run_id[:16],'%Y%m%dT%H%M%SZ').replace(tzinfo=timezone.utc)
    if 'started_at' not in value:return fallback
    try:
        started=datetime.fromisoformat(value['started_at'].replace('Z','+00:00'))
        if started.tzinfo is None or abs((started-fallback).total_seconds())>120:raise ValueError()
        return started.astimezone(timezone.utc)
    except (ValueError,TypeError,AttributeError):raise Protected('INVALID_OR_CONFLICTING_RUN_TIME')


def case_records(value):
    if isinstance(value,list):return value
    if isinstance(value,dict) and isinstance(value.get('cases'),list):
        if 'collected_ids' in value:
            ids=[c.get('id') for c in value['cases'] if isinstance(c,dict)]
            if (len(ids)!=len(set(ids)) or set(ids)!=set(value['collected_ids'])
                    or not ids):raise Protected('CASE_ID_MISMATCH')
        return value['cases']
    raise Protected('CASE_FORMAT_UNKNOWN')


def verify_cases(run,summary,is_pass):
    groups=[summary.get('cases')]
    groups.extend(v['cases'] for v in summary.values() if isinstance(v,dict) and 'cases' in v)
    if not isinstance(groups[0],list) or not groups[0]:raise Protected('MISSING_CASE_EVIDENCE')
    hashes={}
    for path in sorted(run.glob('*cases.json')):
        value=read_json(path)
        records=case_records(value)
        groups.append(records)
        if is_pass and isinstance(value,dict) and value.get('exit_status',0)!=0:raise Protected('PASS_HAS_FAILED_CASE_FILE')
        hashes[path.name]=storage.file_hash(path)
    if not hashes:raise Protected('MISSING_CASE_FILES')
    known={'pass','fail','error','skipped','skip','blocked','interrupted','flaky'}
    count=0
    for records in groups:
        if not isinstance(records,list) or not records:raise Protected('MISSING_CASE_EVIDENCE')
        for case in records:
            if not isinstance(case,dict) or not isinstance(case.get('status'),str):raise Protected('CASE_FORMAT_UNKNOWN')
            status=case['status'].lower()
            if status not in known:raise Protected('UNKNOWN_CASE_STATUS')
            if is_pass and status!='pass':raise Protected('PASS_HAS_NONPASS_CASE')
            count+=1
    if is_pass:
        if summary.get('failure'):raise Protected('PASS_HAS_FAILURE_FIELD')
        for name,value in summary.items():
            if 'reconciliation' in name and value!='PASS':raise Protected('PASS_RECONCILIATION_FAILED')
            if isinstance(value,dict) and 'status' in value and value['status']!='PASS_SYNTHETIC_ONLY':
                raise Protected('PASS_GROUP_FAILED')
    return {'case_file_hashes':hashes,'observed_case_records':count,
            'case_count_note':'Includes repeated summary/file records; not a deduplicated coverage percentage.'}


def backup_evidence(run):
    receipts={}
    for path in sorted(run.glob('*-receipt.json')):
        value=read_json(path)
        if not isinstance(value,dict):raise Protected('BACKUP_RECEIPT_FORMAT')
        if 'restore' not in value and 'artifacts' not in value:continue
        artifacts=value.get('artifacts')
        if (value.get('scope')!='SYNTHETIC_ONLY' or value.get('restore')!='VERIFIED_FIXTURE_SCOPE'
                or value.get('source_database')!='qa_cp05' or not isinstance(value.get('proof_database'),str)
                or not value['proof_database'].endswith('_proof') or not isinstance(artifacts,dict) or not artifacts):
            raise Protected('BACKUP_RESTORE_NOT_VERIFIED')
        suffixes=set()
        for name,digest in artifacts.items():
            if (not isinstance(name,str) or '/' in name or '\\' in name or Path(name).name!=name
                    or not isinstance(digest,str) or not HASH.fullmatch(digest)):raise Protected('BACKUP_PATH_OR_HASH_INVALID')
            artifact=run/name
            if storage.reparse(artifact) or not artifact.is_file() or storage.file_hash(artifact)!=digest:
                raise Protected('BACKUP_HASH_MISMATCH')
            suffixes.add(artifact.suffix)
        if not {'.sql','.dump'}<=suffixes:raise Protected('BACKUP_SQL_CUSTOM_PAIR_MISSING')
        receipts[path.name]={'receipt_sha256':storage.file_hash(path),'artifacts':artifacts,
                              'source_database':value['source_database'],'proof_database':value['proof_database']}
    if not receipts:raise Protected('BACKUP_NOT_VERIFIED')
    return receipts


def cluster_identity(data):
    """Bind plans to this physical directory and its stopped control metadata."""
    data=safe_path(data);info=data.stat()
    return {'device':info.st_dev,'inode':info.st_ino,
            'control_sha256':storage.file_hash(data/'global'/'pg_control'),
            'version_sha256':storage.file_hash(data/'PG_VERSION')}


def disposed_receipt(run):
    """Separate disposal evidence; never rewrite or grant new QA acceptance."""
    marker=read_json(run/'retention-disposal.json')
    if (not isinstance(marker,dict) or marker.get('version')!=1 or marker.get('run_id')!=run.name
            or marker.get('status')!='DISPOSED_SYNTHETIC_CLUSTER_ONLY'
            or not HASH.fullmatch(marker.get('receipt_sha256',''))):raise Protected('INVALID_DISPOSAL_MARKER')
    receipt=safe_path(Path(marker['receipt_path']))
    # Receipt is a direct file in this project's fixed private-backup job.
    if (receipt.name!='receipt.json' or not re.fullmatch(r'qa-retention-apply-[a-f0-9]{32}',receipt.parent.name)
            or receipt.parent.parent.name!='private-backup'
            or storage.file_hash(receipt)!=marker['receipt_sha256']):raise Protected('INVALID_DISPOSAL_RECEIPT')
    value=read_json(receipt)
    service=receipt.parent.parent.parent/'backend-go'
    known=storage.project_locations(service)
    roots={'workspace':known[0],'legacy':known[1]}
    if (not isinstance(value,dict) or value.get('status')!=marker['status'] or value.get('run_id')!=run.name
            or value.get('root') not in roots or run.parent!=roots.get(value.get('root'))
            or value.get('run_path')!=str(run) or value.get('cluster_path')!=str(run/'postgres')
            or value.get('summary_sha256')!=storage.file_hash(run/'summary.json')
            or value.get('backup_evidence')!=backup_evidence(run)
            or value.get('case_evidence')!=verify_cases(run,read_json(run/'summary.json'),
                              read_json(run/'summary.json')['status']=='PASS_SYNTHETIC_ONLY')):
        raise Protected('DISPOSAL_EVIDENCE_CHANGED')
    return value


def inspect_run(root_label,root,run,pins,pg_bin):
    row={'root':root_label,'run_id':run.name,'run_path':str(run.absolute()),
         'cluster_path':str((run/'postgres').absolute()),'decision':'HOLD','reason':'UNINSPECTED',
         'cluster_size':None,'size_status':'NOT_MEASURED_PROTECTED'}
    try:
        resolved=safe_path(run)
        if resolved.parent!=root:raise Protected('PATH_OUTSIDE_ROOT')
        row['run_path']=str(resolved);row['cluster_path']=str(resolved/'postgres')
        key=(root_label,run.name)
        local_pin=resolved/'retention-pin.json'
        if key in pins:
            row.update(reason='PINNED',pin_reason=pins[key]);return row
        if local_pin.exists() or local_pin.is_symlink():
            pin=read_json(local_pin)
            if not isinstance(pin,dict) or set(pin)!={'version','reason'} or pin['version']!=1 or not isinstance(pin['reason'],str) or not pin['reason'].strip():
                raise Protected('INVALID_LOCAL_PIN')
            row.update(reason='PINNED',pin_reason=pin['reason']);return row
        value=read_json(resolved/'summary.json')
        if not isinstance(value,dict):raise Protected('SUMMARY_FORMAT_UNKNOWN')
        row['reported_status']=value.get('status')
        if (value.get('run_id')!=run.name or value.get('profile')!='new-local-synthetic'
                or value.get('shared_database_access')!='NONE' or value.get('database_deletion')!='NONE'):
            raise Protected('SYNTHETIC_IDENTITY_NOT_PROVEN')
        if not isinstance(value.get('source_hash'),str) or not HASH.fullmatch(value['source_hash']):raise Protected('SOURCE_HASH_MISSING')
        row.update(source_hash=value['source_hash'],summary_sha256=storage.file_hash(resolved/'summary.json'),
                   source_currency='HISTORICAL_NOT_COMPARED_TO_CURRENT_TREE')
        row['run_time_utc']=run_time(value,run.name).isoformat()
        if value.get('status')=='FAIL_STOP_REQUIRES_OPERATOR':raise Protected('OPERATOR_REVIEW_REQUIRED')
        if value.get('status') not in ('PASS_SYNTHETIC_ONLY','FAIL'):raise Protected('UNKNOWN_OR_BLOCKED_RUN_STATUS')
        if (resolved/'retention-disposal.json').exists() or (resolved/'retention-disposal.json').is_symlink():
            if (resolved/'postgres').exists():raise Protected('DISPOSED_CLUSTER_REAPPEARED')
            disposed_receipt(resolved)
            row.update(decision='DISPOSED',reason='VERIFIED_SEPARATE_DISPOSAL_RECEIPT');return row
        if (resolved/'retention-intent.json').exists() or (resolved/'retention-intent.json').is_symlink():raise Protected('INCOMPLETE_DISPOSAL_REQUIRES_REVIEW')
        data=safe_path(resolved/'postgres')
        if data.parent!=resolved:raise Protected('CLUSTER_OUTSIDE_RUN')
        storage.closed_run(resolved,[root],pg_bin)
        row['case_evidence']=verify_cases(resolved,value,value['status']=='PASS_SYNTHETIC_ONLY')
        row['backup_evidence']=backup_evidence(resolved)
        # Scan metadata to reject nested links before producing any candidate.
        count=sum(1 for _ in storage.files(data))
        if count==0:raise Protected('EMPTY_CLUSTER')
        row.update(decision='ELIGIBLE',reason='VERIFIED_SYNTHETIC_CLOSED',cluster_file_count=count,
                   cluster_identity=cluster_identity(data),
                   bucket='pass' if value['status']=='PASS_SYNTHETIC_ONLY' else 'failed')
    except (OSError,ValueError,KeyError,TypeError,subprocess.TimeoutExpired) as error:
        row['reason']=str(error) if isinstance(error,Protected) else 'INSPECTION_FAILED'
        row['error_type']=type(error).__name__
    return row


def select(rows):
    """Pure global 2+1 selection; protected rows never consume the ordinary quota."""
    duplicates=Counter(r['run_id'] for r in rows)
    for row in rows:
        if duplicates[row['run_id']]>1:row.update(decision='HOLD',reason='DUPLICATE_RUN_ID')
    for bucket,keep in (('pass',2),('failed',1)):
        ordered=sorted((r for r in rows if r['decision']=='ELIGIBLE' and r['bucket']==bucket),
                       key=lambda r:(r['run_time_utc'],r['run_id'],r['root']),reverse=True)
        for i,row in enumerate(ordered):
            row.update(decision='KEEP' if i<keep else 'CANDIDATE',
                       reason='LATEST_'+bucket.upper() if i<keep else 'OUTSIDE_GLOBAL_'+bucket.upper()+'_QUOTA')
    return sorted(rows,key=lambda r:(r['root'],r['run_id']))


def build_plan(roots,pins,pg_bin,measure_candidates=True,progress=None):
    if set(roots)!=ROOT_LABELS:raise Protected('TWO_FIXED_ROOT_LABELS_REQUIRED')
    roots={label:safe_path(path) for label,path in roots.items()}
    if len(set(roots.values()))!=2:raise Protected('DISTINCT_ROOTS_REQUIRED')
    rows=[];exceptions=[];ignored=0
    for label,root in sorted(roots.items()):
        for entry in sorted(root.iterdir()):
            if storage.RUN_NAME.fullmatch(entry.name):
                if progress:progress('inspect '+label+'/'+entry.name)
                rows.append(inspect_run(label,root,entry,pins,pg_bin));continue
            try:
                if storage.reparse(entry):raise Protected('REPARSE_NON_RUN_ENTRY')
                if entry.is_dir():
                    # Probe access only; do not recursively scan caches or scratch.
                    import os
                    with os.scandir(entry) as scan:next(scan,None)
                    if (entry/'postgres').exists():raise Protected('UNRECOGNIZED_CLUSTER_DIRECTORY')
                ignored+=1
            except (OSError,ValueError) as error:
                exceptions.append({'root':label,'path':str(entry),'reason':str(error) if isinstance(error,Protected) else 'INACCESSIBLE_NON_RUN_ENTRY',
                                   'error_type':type(error).__name__,'bytes':None})
    rows=select(rows)
    for row in rows:
        if row['decision']!='CANDIDATE':continue
        try:
            if measure_candidates:
                if progress:progress('measure '+row['root']+'/'+row['run_id'])
                row['cluster_size']=storage.measure(Path(row['cluster_path']));row['size_status']='MEASURED'
            else:row['size_status']='NOT_REQUESTED'
        except (OSError,ValueError) as error:row.update(decision='HOLD',reason='CANDIDATE_SIZE_UNREADABLE',error_type=type(error).__name__)
    seen={(r['root'],r['run_id']) for r in rows}
    unresolved=[{'root':k[0],'run_id':k[1],'reason':'PIN_TARGET_NOT_DISCOVERED'} for k in sorted(pins) if k not in seen]
    counts=dict(Counter(r['decision'] for r in rows))
    return {'version':2,'mode':'DRY_RUN_ONLY','status':'PLAN_WITH_PROTECTED_EXCEPTIONS' if exceptions or unresolved or counts.get('HOLD') else 'PLAN_READY_FOR_REVIEW',
            'policy':{'version':3,'keep_pass':2,'keep_failed':1,'scope':'GLOBAL_ACROSS_FIXED_ROOTS'},
            'created_at':datetime.now(timezone.utc).isoformat(),'roots':{k:str(v) for k,v in roots.items()},
            'rows':rows,'decision_counts':counts,'non_run_exceptions':exceptions,'unresolved_pins':unresolved,
            'ignored_non_cluster_entries':ignored,'estimated_candidate_allocated_bytes':sum(r['cluster_size']['allocated_bytes'] for r in rows if r['decision']=='CANDIDATE' and r['cluster_size']),
            'estimate_note':'Cluster subfolders only; metadata/shared extents are not a guaranteed amount freed. Protected/unmeasured bytes are not zero.',
            'database_connections':'NONE','database_deletion':'NONE','compression':'NONE','apply_implemented':True,
            'authority':'REVIEW_PLAN_ONLY_NOT_DISPOSAL_AUTHORIZATION'}
