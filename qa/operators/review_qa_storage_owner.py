"""Read-only complete storage/admission review. No cleanup or ACL changes."""
import json
from pathlib import Path
import shutil
import sys

SERVICE = Path(__file__).resolve().parents[2]
BASE = SERVICE.parent
WORKSPACE = BASE.parent
from qa import storage
from qa.evidence import source_fingerprint
from qa.run import run_lock


def review_storage():
    parent=storage.project_locations(SERVICE)[0];parent.mkdir(exist_ok=True)
    destination=BASE/'private-backup'/('qa-owner-inventory-'+storage.stamp());destination.mkdir(parents=True)
    source=source_fingerprint(SERVICE)
    with run_lock(parent/'storage.lock'):
        print('[qa-owner-review] Measuring managed QA, caches and private backups; no cleanup.',flush=True)
        report=storage.inventory(storage.project_locations(SERVICE))
        free=shutil.disk_usage(SERVICE).free;policy=dict(storage.DEFAULT_POLICY)
        reasons=[]
        if not report['complete']:reasons.append('UNKNOWN_BYTES')
        if report['allocated_bytes']+policy['run_reserve_gib']*storage.GIB>policy['total_budget_gib']*storage.GIB:
            reasons.append('TOTAL_BUDGET')
        if free<(policy['minimum_free_gib']+policy['run_reserve_gib'])*storage.GIB:reasons.append('FREE_SPACE')
        if source_fingerprint(SERVICE)!=source:reasons.append('SOURCE_CHANGED')
        report.update(status='BLOCKED' if reasons else 'PASS_STORAGE_ADMISSION_ONLY',reasons=reasons,
                      policy=policy,source_hash=source,free_bytes=free,database_operations='NONE',acl_changes='NONE',
                      deletion='NONE',full_m14_qa='NOT_RUN')
        storage.dump_new(destination/'inventory.json',report)
    print(json.dumps({'status':report['status'],'report':str(destination),'reasons':reasons,
                      'unreadable':report['unreadable'],'measured_allocated_gib':round(report['allocated_bytes']/storage.GIB,3),
                      'free_gib':round(free/storage.GIB,3)}),flush=True)
    return {'status':report['status'],'reasons':reasons,'report':str(destination),'source_hash':source}


def main():
    return 0 if review_storage()['status']=='PASS_STORAGE_ADMISSION_ONLY' else 1


if __name__=='__main__':raise SystemExit(main())
