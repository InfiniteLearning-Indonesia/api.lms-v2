"""Bounded local QA storage. No database connection, deletion or SQL commands.

Windows compaction is lossless NTFS compression, not archive-and-delete. Failed
or active clusters are retained. Historical summaries are never rewritten.
"""
from datetime import datetime, timezone
from hashlib import sha256
import ctypes
from functools import lru_cache
import json
import os
from pathlib import Path
import re
import shutil
import subprocess

GIB = 1024**3
RUN_NAME = re.compile(r'^\d{8}T\d{6}Z-postgres-lab-[a-f0-9]{12}$')
DEFAULT_POLICY = {'version':2,'total_budget_gib':32,'run_reserve_gib':2,'minimum_free_gib':8,
                  'per_run_budget_gib':2,'database_deletion':'NEVER',
                  'closed_cluster_action':'NTFS_COMPRESSION_ONLY','unknown_or_running':'RETAIN'}


def stamp():
    return datetime.now(timezone.utc).strftime('%Y%m%dT%H%M%S%fZ')


def dump_new(path, value):
    with path.open('x',encoding='utf-8') as handle:
        json.dump(value,handle,indent=2,sort_keys=True);handle.write('\n')


def reparse(path):
    st=path.lstat()
    return path.is_symlink() or bool(getattr(st,'st_file_attributes',0)&0x400)


def files(root):
    """scandir metadata avoids a separate Windows stat syscall per file."""
    root=Path(root)
    if not root.exists():return
    if reparse(root):raise ValueError('storage reparse root rejected')
    stack=[root]
    while stack:
        with os.scandir(stack.pop()) as entries:
            for entry in entries:
                st=entry.stat(follow_symlinks=False)
                if entry.is_symlink() or getattr(st,'st_file_attributes',0)&0x400:
                    raise ValueError('storage reparse entry rejected')
                path=Path(entry.path)
                if entry.is_dir(follow_symlinks=False):stack.append(path)
                elif entry.is_file(follow_symlinks=False):yield path,st.st_size,getattr(st,'st_file_attributes',0)
                else:raise ValueError('storage special file rejected')


@lru_cache(maxsize=1)
def compressed_size_api():
    kernel=ctypes.WinDLL('kernel32',use_last_error=True)
    fn=kernel.GetCompressedFileSizeW
    fn.argtypes=[ctypes.c_wchar_p,ctypes.POINTER(ctypes.c_ulong)]
    fn.restype=ctypes.c_ulong
    return fn


def allocated_bytes(path, logical, attributes=None):
    if os.name!='nt':return path.stat().st_blocks*512 if hasattr(path.stat(),'st_blocks') else logical
    if logical==0:return 0
    if attributes is None:attributes=getattr(path.stat(),'st_file_attributes',0)
    # GetCompressedFileSize returns ordinary length for noncompressed streams.
    # Directory metadata already supplies these attributes; avoid opening every
    # uncompressed PostgreSQL relation just to learn that same length.
    if not attributes & (0x800|0x200):return logical
    fn=compressed_size_api()
    high=ctypes.c_ulong(0);ctypes.set_last_error(0)
    low=fn(str(path),ctypes.byref(high));error=ctypes.get_last_error()
    if low==0xffffffff and error:raise OSError(error,'allocated byte measurement failed')
    return (high.value<<32)|low


def measure(root):
    logical=physical=count=0
    for path,size,attributes in files(root):
        logical+=size;physical+=allocated_bytes(path,size,attributes);count+=1
    return {'logical_bytes':logical,'allocated_bytes':physical,'files':count}


def project_locations(service):
    service=Path(service).resolve(strict=True)
    workspace=service.parent.parent
    return [workspace/'lms-v3-qa-artifacts',service/'.qa-artifacts',service/'.cache',service.parent/'private-backup']


def inventory(locations):
    rows=[];errors=[]
    for root in locations:
        # Inaccessible historical entries are visible, never counted as zero.
        try:
            if not root.exists():continue
            root_total={'logical_bytes':0,'allocated_bytes':0,'files':0}
            for child in root.iterdir():
                try:
                    if reparse(child):raise ValueError('reparse entry')
                    current=measure(child) if child.is_dir() else {'logical_bytes':child.stat().st_size,
                         'allocated_bytes':allocated_bytes(child,child.stat().st_size),'files':1}
                    for key in root_total:root_total[key]+=current[key]
                except (OSError,ValueError) as e:
                    errors.append({'path':str(child),'error':type(e).__name__})
            rows.append({'root':str(root),**root_total})
        except (OSError,ValueError) as e:errors.append({'path':str(root),'error':type(e).__name__})
    return {'roots':rows,'allocated_bytes':sum(x['allocated_bytes'] for x in rows),
            'logical_bytes':sum(x['logical_bytes'] for x in rows),'unreadable':errors,'complete':not errors}


def preflight(service, parent):
    policy=dict(DEFAULT_POLICY)
    report=inventory(project_locations(service));free=shutil.disk_usage(parent).free
    report.update(policy=policy,free_bytes=free)
    if not report['complete']:raise ValueError('QA_STORAGE_UNKNOWN_BYTES: inspect inaccessible paths in inventory; ordinary terminal may have access')
    if report['allocated_bytes']+(policy['run_reserve_gib']*GIB)>policy['total_budget_gib']*GIB:
        raise ValueError('QA_STORAGE_BUDGET: compact stopped synthetic clusters before a new full run')
    if free<(policy['minimum_free_gib']+policy['run_reserve_gib'])*GIB:
        raise ValueError('QA_STORAGE_FREE_SPACE: need minimum free space plus run reserve')
    return report


def guard_growth(directory):
    if shutil.disk_usage(directory).free<DEFAULT_POLICY['minimum_free_gib']*GIB:
        raise ValueError('QA_STORAGE_FREE_SPACE: stop before the drive fills')
    if measure(directory)['allocated_bytes']>DEFAULT_POLICY['per_run_budget_gib']*GIB:
        raise ValueError('QA_STORAGE_RUN_BUDGET: evidence retained, no automatic deletion')


def file_hash(path):
    h=sha256()
    with path.open('rb') as handle:
        for b in iter(lambda:handle.read(1024*1024),b''):h.update(b)
    return h.hexdigest()


def tree_digest(root):
    h=sha256();count=0
    for path,size,_attributes in sorted(files(root),key=lambda pair:pair[0].relative_to(root).as_posix()):
        h.update(path.relative_to(root).as_posix().encode());h.update(b'\0')
        content_hash=file_hash(path) if size else sha256(b'').hexdigest()
        h.update(str(size).encode());h.update(b'\0');h.update(content_hash.encode());h.update(b'\0');count+=1
    return {'sha256':h.hexdigest(),'files':count}


def tree_index(root):
    """All file names/sizes plus control hash; explicitly not a full byte hash."""
    h=sha256();count=0
    for path,size,_attributes in sorted(files(root),key=lambda pair:pair[0].relative_to(root).as_posix()):
        h.update(path.relative_to(root).as_posix().encode());h.update(b'\0')
        h.update(str(size).encode());h.update(b'\0');count+=1
    return {'sha256':h.hexdigest(),'files':count,'control_sha256':file_hash(root/'postgres/global/pg_control')}


def closed_run(run, roots, pg_bin):
    run=Path(run)
    # Inspect the supplied path before resolve so a symlink cannot disappear.
    if reparse(run):raise ValueError('linked run rejected')
    run=run.resolve(strict=True)
    roots=[Path(r).resolve(strict=True) for r in roots]
    if run.parent not in roots or not RUN_NAME.fullmatch(run.name):raise ValueError('run outside fixed QA roots')
    summary=run/'summary.json'
    if reparse(summary):raise ValueError('linked summary rejected')
    value=json.loads(summary.read_text(encoding='utf-8'))
    if (value.get('profile')!='new-local-synthetic' or value.get('shared_database_access')!='NONE'
            or value.get('database_deletion')!='NONE' or value.get('run_id')!=run.name):
        raise ValueError('unproven synthetic scope')
    data=run/'postgres'
    if reparse(data) or (data/'postmaster.pid').exists():raise ValueError('active or ambiguous cluster retained')
    version=(data/'PG_VERSION').read_text().strip()
    if version!='18':raise ValueError('cluster version requires a matching operator tool')
    tool=Path(pg_bin)/('pg_controldata.exe' if os.name=='nt' else 'pg_controldata')
    env={k:v for k,v in os.environ.items() if k.upper() in {'PATH','SYSTEMROOT','WINDIR','COMSPEC','TEMP','TMP'}}
    env.update(LC_ALL='C',LANG='C')
    checked=subprocess.run([str(tool),'-D',str(data)],env=env,capture_output=True,text=True,timeout=15)
    states=[line.split(':',1)[1].strip() for line in checked.stdout.splitlines() if line.startswith('Database cluster state:')]
    if checked.returncode or states!=['shut down']:raise ValueError('clean shutdown not proven; retained untouched')
    return run,value,data


def verify_backups(run):
    receipts=[]
    for p in sorted(run.glob('*-receipt.json')):
        value=json.loads(p.read_text(encoding='utf-8'))
        if 'artifacts' not in value or 'restore' not in value:continue
        if value['restore']!='VERIFIED_FIXTURE_SCOPE':raise ValueError('restore proof not verified')
        for name,digest in value['artifacts'].items():
            if Path(name).name!=name or '/' in name or '\\' in name:raise ValueError('backup path escaped run')
            artifact=run/name
            if reparse(artifact) or file_hash(artifact)!=digest:raise ValueError('backup bytes differ from receipt')
        receipts.append(p.name)
    if not receipts:raise ValueError('no verified logical backup receipts; raw data retained')
    return receipts


def compress_selected(compact, payload):
    """Avoid recursive compact traversing/marking tens of thousands of empty files."""
    selected=[p for p,size,attributes in files(payload) if size>=4096 and not attributes&0x800]
    completed=0
    for offset in range(0,len(selected),48):
        batch=selected[offset:offset+48]
        if sum(len(str(p))+3 for p in batch)>26000:raise ValueError('compact argument bound exceeded')
        checked=subprocess.run([str(compact),'/C','/Q',*[str(p) for p in batch]],cwd=payload,
                               stdin=subprocess.DEVNULL,capture_output=True,text=True,timeout=45)
        if checked.returncode:return checked.returncode,completed,len(selected)
        completed+=len(batch)
        if completed%480==0:print(f'[qa-storage] compressed {completed}/{len(selected)} selected files: {payload.name}',flush=True)
    return 0,completed,len(selected)


def compact_run(run, roots, pg_bin, receipts, verification='backup-and-metadata'):
    if os.name!='nt':raise ValueError('NTFS compaction available only on Windows; no fallback deletion')
    run,value,data=closed_run(run,roots,pg_bin)
    receipts=Path(receipts);receipts.mkdir(parents=True,exist_ok=True)
    record=receipts/(run.name+'-'+stamp()+'.json')
    summary_hash=file_hash(run/'summary.json')
    print('[qa-storage] verify logical backups: '+run.name,flush=True)
    verified=verify_backups(run)
    # Related CSV/SQL/logs/coverage/test executables also accumulate. Compress
    # the finalized run as one byte-preserving unit, not just relation files.
    payload=run
    before=measure(payload)
    # If already compressed well, avoid hashing/recompressing on every future run.
    # Savings alone are not an integrity proof: existing compression is reported only.
    if before['logical_bytes'] and before['allocated_bytes']<before['logical_bytes']*.5:
        result={'run_id':run.name,'action':'ALREADY_SPACE_EFFICIENT_UNCHANGED','before':before,
                'summary_sha256':summary_hash,'logical_backups_verified':len(verified),'database_deletion':'NONE'}
        dump_new(record,result);return result
    if verification not in ('full','backup-and-metadata'):raise ValueError('unknown verification mode')
    fingerprint=tree_digest if verification=='full' else tree_index
    print('[qa-storage] verify run before compression ('+verification+'): '+run.name,flush=True)
    digest=fingerprint(payload)
    dump_new(record.with_suffix('.before.json'),{'run_id':run.name,'tree':digest,'measurement':before,
             'summary_sha256':summary_hash,'logical_backups_verified':verified,'cluster_state':'shut down','verification':verification})
    compact=Path(os.environ.get('SYSTEMROOT',r'C:\Windows'))/'System32/compact.exe'
    # No shell, dynamic SQL, traversal, deletion, archive replacement or forced stop.
    print('[qa-storage] apply NTFS compression: '+run.name,flush=True)
    code,completed,selected=compress_selected(compact,payload)
    after=measure(payload)
    print('[qa-storage] verify run after compression ('+verification+'): '+run.name,flush=True)
    same=fingerprint(payload)==digest and file_hash(run/'summary.json')==summary_hash
    result={'run_id':run.name,'action':'NTFS_COMPRESSED_BYTES_PRESERVED' if same and code==0 else 'REQUIRES_REVIEW',
            'before':before,'after':after,'saved_allocated_bytes':before['allocated_bytes']-after['allocated_bytes'],
            'all_cluster_file_hashes_unchanged':same if verification=='full' else None,'compact_exit_code':code,
            'selected_files':selected,'completed_selected_files':completed,
            'all_run_file_hashes_unchanged':same if verification=='full' else None,
            'verification':verification,'file_names_sizes_control_and_summary_unchanged':same,
            'summary_sha256':summary_hash,'logical_backups_verified':len(verified),
            'database_deletion':'NONE','shared_database_access':'NONE'}
    dump_new(record,result)
    if not same or code:raise ValueError('compaction did not verify; original data retained for review')
    return result
