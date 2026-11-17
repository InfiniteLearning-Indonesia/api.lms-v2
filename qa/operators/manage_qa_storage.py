"""Inventory and lossless compression of stopped synthetic QA clusters only."""
import argparse
from collections import Counter
from contextlib import ExitStack
from concurrent.futures import ThreadPoolExecutor,as_completed
import json
from pathlib import Path
import sys
import subprocess
import shutil

SERVICE = Path(__file__).resolve().parents[2]
BASE = SERVICE.parent
WORKSPACE = BASE.parent
from qa import storage,retention
from qa.run import run_lock


def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--compress-closed-clusters',action='store_true')
    parser.add_argument('--run-id',help='Optional single run for a reversible compression pilot')
    parser.add_argument('--workers',type=int,choices=(1,2,3),default=2,help='Bounded disjoint stopped-cluster compression jobs')
    parser.add_argument('--verification',choices=('full','backup-and-metadata'),default='backup-and-metadata')
    parser.add_argument('--pg-bin',type=Path,default=Path(r'C:\Program Files\PostgreSQL\18\bin'))
    args=parser.parse_args()
    locations=storage.project_locations(SERVICE)
    roots=[r for r in locations[:2] if r.is_dir()]
    destination=BASE/'private-backup'/('qa-storage-'+storage.stamp());destination.mkdir(parents=True)
    free_before=shutil.disk_usage(SERVICE).free
    before=storage.inventory(locations);storage.dump_new(destination/'inventory-before.json',before)
    print(json.dumps({'inventory':str(destination/'inventory-before.json'),'allocated_gib':round(before['allocated_bytes']/storage.GIB,2),
                      'logical_gib':round(before['logical_bytes']/storage.GIB,2),'unreadable':len(before['unreadable'])}),flush=True)
    ledger=[];actions=[]
    with ExitStack() as stack:
        for root in roots:
            stack.enter_context(run_lock(root/'storage.lock'))
            stack.enter_context(run_lock(root/'postgres-lab.lock'))
            stack.enter_context(run_lock(root/'local-synthetic.lock'))
            stack.enter_context(run_lock(root/'release.lock'))
        executor=stack.enter_context(ThreadPoolExecutor(max_workers=args.workers))
        jobs={}
        for root in roots:
            for run in sorted(root.iterdir()):
                if not storage.RUN_NAME.fullmatch(run.name) or not run.is_dir():continue
                if args.run_id and run.name!=args.run_id:continue
                summary=run/'summary.json'
                if not summary.is_file():continue
                try:
                    if storage.reparse(run) or storage.reparse(summary):raise ValueError('linked evidence')
                    value=json.loads(summary.read_text(encoding='utf-8'))
                    row={'run_id':run.name,'artifact_root':str(root),'reported_status':value.get('status'),
                         'source_hash':value.get('source_hash'),'summary_sha256':storage.file_hash(summary),
                         'database_access':'NONE','database_deletion':'NONE'}
                    row['case_group_counts']={}
                    if isinstance(value.get('cases'),list):
                        row['case_group_counts']['root']=dict(Counter(c.get('status','UNKNOWN') for c in value['cases'] if isinstance(c,dict)))
                    for name,group in value.items():
                        if isinstance(group,dict) and isinstance(group.get('cases'),list):
                            row['case_group_counts'][name]=dict(Counter(c.get('status','UNKNOWN') for c in group['cases'] if isinstance(c,dict)))
                    try:
                        proofs=storage.verify_backups(run);row['backup_receipt_hashes_verified']=len(proofs)
                    except (OSError,ValueError,KeyError) as e:
                        row['backup_check']='NOT_VERIFIED:'+type(e).__name__
                    # This ledger records historical results; it grants no new PASS.
                    ledger.append(row)
                    if (run/'retention-disposal.json').exists():
                        if (run/'postgres').exists():raise ValueError('disposal marker conflicts with existing raw cluster')
                        retention.disposed_receipt(run)
                        row['current_raw_cluster']='DISPOSED_VERIFIED_SEPARATE_RECEIPT'
                        if args.compress_closed_clusters:
                            actions.append({'run_id':run.name,'action':'ALREADY_DISPOSED_NOT_COMPRESSED'})
                        continue
                    if (run/'retention-intent.json').exists():
                        row['current_raw_cluster']='INCOMPLETE_DISPOSAL_REQUIRES_REVIEW'
                        actions.append({'run_id':run.name,'action':'RETAINED_REQUIRES_REVIEW','reason':'incomplete disposal intent'})
                        continue
                    row['current_raw_cluster']='RETAINED_OR_UNKNOWN'
                    if args.compress_closed_clusters:
                        print('[qa-storage] inspect/compress '+run.name,flush=True)
                        future=executor.submit(storage.compact_run,run,roots,args.pg_bin,destination/'compression-receipts',args.verification)
                        jobs[future]=run.name
                except (OSError,ValueError,KeyError,subprocess.TimeoutExpired) as error:
                    actions.append({'run_id':run.name,'action':'RETAINED_REQUIRES_REVIEW','reason':str(error)})
                    print(json.dumps({'run_id':run.name,'action':'RETAINED_REQUIRES_REVIEW','reason':str(error)}),flush=True)
        for future in as_completed(jobs):
            run_id=jobs[future]
            try:
                outcome=future.result();actions.append(outcome)
                print(json.dumps({'run_id':run_id,'action':outcome['action'],'saved_bytes':outcome.get('saved_allocated_bytes',0)}),flush=True)
            except (OSError,ValueError,KeyError,subprocess.TimeoutExpired) as error:
                actions.append({'run_id':run_id,'action':'RETAINED_REQUIRES_REVIEW','reason':str(error)})
                print(json.dumps({'run_id':run_id,'action':'RETAINED_REQUIRES_REVIEW','reason':str(error)}),flush=True)
    storage.dump_new(destination/'results-ledger.json',ledger)
    storage.dump_new(destination/'actions.json',actions)
    after=storage.inventory(locations);storage.dump_new(destination/'inventory-after.json',after)
    result={'status':'COMPLETED_WITH_EXPLICIT_RETAINED_ITEMS' if any(x['action']=='RETAINED_REQUIRES_REVIEW' for x in actions) else 'COMPLETED',
            'report':str(destination),'recorded_runs':len(ledger),'actions':len(actions),
            'before_allocated_bytes':before['allocated_bytes'],'after_allocated_bytes':after['allocated_bytes'],
            'saved_allocated_bytes':before['allocated_bytes']-after['allocated_bytes'],
            'drive_free_before':free_before,'drive_free_after':shutil.disk_usage(SERVICE).free,
            'database_deletion':'NONE','shared_database_access':'NONE','git_operations':'NONE',
            'incomplete_measurement':not before['complete'] or not after['complete']}
    storage.dump_new(destination/'summary.json',result);print(json.dumps(result),flush=True)
    return 0


if __name__=='__main__':raise SystemExit(main())
