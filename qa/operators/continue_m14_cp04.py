"""Owner-terminal inventory then existing M14 QA, only when admission passes.

Runs with the caller's permissions. Never elevates, changes ACL, bypasses a gate,
or supplies a DSN/env file. --inventory-only never launches QA.
"""
import argparse
import json
from pathlib import Path
import subprocess
import sys

from qa.operators.review_qa_storage_owner import review_storage

SERVICE = Path(__file__).resolve().parents[2]
BASE = SERVICE.parent
WORKSPACE = BASE.parent


def continue_qa(pg_bin,inventory_only=False,prepare_only=False):
    state=review_storage()
    if state['status']!='PASS_STORAGE_ADMISSION_ONLY':
        print(json.dumps({'status':'BLOCKED_BEFORE_QA','inventory':state,'qa_started':False}),flush=True)
        return 1
    if inventory_only:
        print(json.dumps({'status':'PASS_STORAGE_ADMISSION_ONLY','inventory':state,'qa_started':False}),flush=True)
        return 0
    command=[sys.executable,'-m','qa.operators.run_m14_cp04','--pg-bin',str(pg_bin)]
    if prepare_only:command.append('--prepare-only')
    print('[continue-M14] inventory passed; existing QA runner rechecks storage and writes verified backups.',flush=True)
    return subprocess.run(command,cwd=SERVICE).returncode


def main():
    parser=argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--pg-bin',type=Path,default=Path(r'C:\Program Files\PostgreSQL\18\bin'))
    parser.add_argument('--inventory-only',action='store_true')
    parser.add_argument('--prepare-only',action='store_true')
    args=parser.parse_args()
    return continue_qa(args.pg_bin,args.inventory_only,args.prepare_only)


if __name__=='__main__':raise SystemExit(main())
