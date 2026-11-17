"""Fixed local-synthetic QA; no shared DB/Git. Scoped QA retention is separate."""
from datetime import datetime, timezone
from hashlib import sha256
import argparse
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys

SERVICE = Path(__file__).resolve().parents[2]
BASE = SERVICE.parent
WORKSPACE = BASE.parent
from qa.evidence import source_fingerprint, write_new_json
from qa.run import run_lock
from qa import storage,retention_automation
from qa.preparation import run_ops_checks
from qa.operators.context_archive import archive, walk, config, SKIP


def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--pg-bin',type=Path,default=Path(r'C:\Program Files\PostgreSQL\18\bin'))
    parser.add_argument('--prepare-only',action='store_true',help='Backup/build/unit checks only; never start PostgreSQL')
    args=parser.parse_args()
    pg=args.pg_bin.resolve(strict=True)
    if not (pg/('initdb.exe' if os.name=='nt' else 'initdb')).is_file():
        raise ValueError('PostgreSQL tools missing')
    parent=WORKSPACE/'lms-v3-qa-artifacts';parent.mkdir(exist_ok=True)
    # The lock file is retained after a run. Probe the OS lock, not file existence.
    with run_lock(parent/'postgres-lab.lock'):
        pass
    try:
        retention_automation.before_run(SERVICE,pg)
        storage_state=storage.preflight(SERVICE,parent)
    except (OSError,ValueError) as error:
        print('[M14-CP04] STOP before database startup: '+str(error),flush=True)
        print('Inspect with python -m qa.operators.review_qa_storage_owner; no gate bypass or ACL change.',flush=True)
        return 1
    stamp=datetime.now(timezone.utc).strftime('%Y%m%dT%H%M%S%fZ')
    destination=BASE/'private-backup'/('m14-cp04-'+stamp);destination.mkdir(parents=True,exist_ok=False)
    write_new_json(destination/'storage-preflight.json',storage_state)
    errors=[]
    sources=[p for p in walk(SERVICE,SKIP,errors) if not config(p)]
    sources += [p for p in (walk(BASE/'portability',SKIP,errors) if (BASE/'portability').is_dir() else [])
                if p.suffix.lower() in {'.py','.ps1','.cmd','.md','.json'} and not config(p)]
    # Private policies are optional inputs, never fabricated by a checkout.
    sources += [p for p in (BASE/'START_HERE.md',BASE/'qa-retention-pins.json',BASE/'qa-retention-automation.json') if p.is_file()]
    if errors: raise ValueError('Source backup contains inaccessible files')
    source=source_fingerprint(SERVICE)
    print('[M14-CP04] 1/5 verified source/context backup',flush=True)
    receipt=archive(destination,'source-and-context',sources)
    if source != source_fingerprint(SERVICE):raise ValueError('Source changed during backup')
    write_new_json(destination/'backup-receipt.json',{'source_hash':source,'archive':receipt,'encrypted':False,
                    'configuration_secrets_excluded':True,'database_access':'NONE','git_operations':'NONE'})
    # Set environment values directly; avoid trailing-space CMD assignment traps.
    env=dict(os.environ);env.update(GOCACHE=str(parent/'go-cache'),GOTELEMETRY='off',GOPROXY='off',PYTHONUNBUFFERED='1')
    checks=[]
    def check(name,command,timeout):
        print('[M14-CP04] '+name,flush=True)
        completed=subprocess.run(command,cwd=SERVICE,env=env,capture_output=True,text=True,timeout=timeout)
        log=destination/(name+'.log')
        with log.open('x',encoding='utf-8') as f:f.write(completed.stdout);f.write(completed.stderr)
        checks.append({'check':name,'exit_code':completed.returncode,'log_sha256':sha256(log.read_bytes()).hexdigest()})
        if completed.returncode:
            print('FAILED: '+name+'; log: '+str(log),flush=True)
            raise ValueError('Preparation failed; retained diagnostic logs')
        print('PASS: '+name,flush=True)
    try:
        check('manifest-review',[sys.executable,'-m','qa.review_m13_m14_manifests'],60)
        print('[M14-CP04] exact per-case ops preflight',flush=True)
        checks.append(run_ops_checks(SERVICE,destination,env))
        print('PASS: ops-targeted-regression; exact cases and evidence verified',flush=True)
        check('go-unit',['go','test','./...'],300)
        check('go-vet',['go','vet','./...'],180)
        check('db-probe-compile',['go','test','-tags=qaintegration','-run=^$','./internal/reporting'],180)
        check('load-build',['go','build','-trimpath','-o',str(destination/'report-load.exe' if os.name=='nt' else destination/'report-load'),'./cmd/report-load'],180)
        if source != source_fingerprint(SERVICE):raise ValueError('Source changed during preparation')
        write_new_json(destination/'preparation.json',{'status':'PASS_LOCAL_PREPARATION_ONLY','source_hash':source,'checks':checks,
                                                      'full_database_qa':'NOT_RUN','canonical_module_verified':False})
        if args.prepare_only:
            print(json.dumps({'status':'PASS_LOCAL_PREPARATION_ONLY','backup':str(destination),'source_hash':source}),flush=True)
            return 0
        print('[M14-CP04] 2/5 full dependency journey + SQL/custom backups and actual isolated restores',flush=True)
        print('[M14-CP04] 3/5 seed/load/EXPLAIN/pool; 4/5 dependent regressions and normal stop; 5/5 read-only evidence review',flush=True)
        print('[M14-CP04] Keep terminal open; do not edit source while this job runs.',flush=True)
        command=[sys.executable,'-m','qa.postgres_lab','--create-local-synthetic-cluster','--pg-bin',str(pg),
                 '--reporting-performance-journey','--artifact-parent',str(parent)]
        last=None
        with (destination/'full-qa-console.log').open('x',encoding='utf-8') as log:
            process=subprocess.Popen(command,cwd=SERVICE,env=env,stdout=subprocess.PIPE,stderr=subprocess.STDOUT,text=True,encoding='utf-8',errors='replace')
            for line in process.stdout:
                print(line,end='',flush=True);log.write(line);log.flush()
                try:
                    event=json.loads(line)
                    if isinstance(event,dict) and 'run_id' in event and 'cluster_retained' in event:last=event
                except ValueError:pass
            process.stdout.close();code=process.wait()
        if code or not last or last['status']!='PASS_SYNTHETIC_ONLY':
            write_new_json(destination/'outcome.json',{'status':'FAIL_OR_INTERRUPTED','last_summary':last,'qa_exit_code':code,'source_hash':source})
            print('[M14-CP04] FAIL: evidence retained; no completion claim. Backup/log: '+str(destination),flush=True)
            return 1
        check('full-evidence-review',[sys.executable,'-m','qa.operators.review_m14_contract_run',last['run_id'],'--include-cp04'],180)
        write_new_json(destination/'outcome.json',{'status':'PASS_REVIEWED_LOCAL_SYNTHETIC','run_id':last['run_id'],'source_hash':source,
                       'canonical_module_verified':False,'documentation_closure':'PENDING_HUMAN_READ_OF_REVIEW','git_operations':'NONE'})
        print(json.dumps({'status':'PASS_REVIEWED_LOCAL_SYNTHETIC','run_id':last['run_id'],'backup':str(destination),
                          'next':'Return this result for documentation closure; no production/SLA/full-module claim.'}),flush=True)
        return 0
    except (OSError,ValueError,subprocess.TimeoutExpired) as error:
        write_new_json(destination/'launcher-failure.json',{'error_type':type(error).__name__,'checks':checks,'source_hash':source})
        print('[M14-CP04] STOP: '+str(error)+'; evidence: '+str(destination),flush=True)
        return 1


if __name__=='__main__':raise SystemExit(main())
