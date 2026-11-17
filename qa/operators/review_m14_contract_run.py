"""Read-only acceptance review for an actual M14 CP01/02 synthetic run."""
from datetime import datetime, timezone
from hashlib import sha256
import json
from pathlib import Path
import sys

SERVICE = Path(__file__).resolve().parents[2]
ROOT = SERVICE.parent.parent
from qa.evidence import source_fingerprint, verify_go_cases


def load(path):
    return json.loads(path.read_text(encoding='utf-8'))


def main():
    parent = ROOT / 'lms-v3-qa-artifacts'
    run = (parent / sys.argv[1]).resolve(strict=True)
    if run.parent != parent.resolve(strict=True):
        raise ValueError('Run path escaped artifact parent')
    summary = load(run / 'summary.json')
    if summary.get('status') != 'PASS_SYNTHETIC_ONLY':
        raise ValueError('Full journey did not pass')
    current = source_fingerprint(SERVICE)
    if current != summary['source_hash']:
        raise ValueError('Current source differs from tested source')
    include_cp04 = len(sys.argv) == 3 and sys.argv[2] == '--include-cp04'
    include_cp03 = include_cp04 or len(sys.argv) == 3 and sys.argv[2] == '--include-cp03'
    if len(sys.argv) not in (2, 3) or len(sys.argv) == 3 and not include_cp03:
        raise ValueError('Use <run-id> [--include-cp03|--include-cp04]')
    results = {}
    checkpoints = ('m14_cp01', 'm14_cp02', 'm14_cp03') if include_cp03 else ('m14_cp01', 'm14_cp02')
    for checkpoint in checkpoints:
        name = checkpoint.upper().replace('_', '-')
        manifest = load(SERVICE / 'qa/manifests' / (name + '.json'))
        go_cases = load(run / (checkpoint + '-cases.json'))
        if not verify_go_cases(manifest[checkpoint + '_go'], go_cases):
            raise ValueError(name + ' Go cases incomplete')
        api = load(run / (checkpoint.replace('_cp', 'cp') + '-api-cases.json'))
        expected = set(manifest[checkpoint + '_python'])
        if (api['exit_status'] != 0 or set(api['collected_ids']) != expected
                or len(api['cases']) != len(expected)
                or {case['id'] for case in api['cases']} != expected
                or any(case['status'] != 'PASS' for case in api['cases'])):
            raise ValueError(name + ' API cases incomplete')
        results[name] = {'go_pass': len(go_cases), 'api_pass': len(expected), 'status': 'PASS_LOCAL_SYNTHETIC'}
    if include_cp04:
        from qa.reporting_perf import review, number
        manifest = load(SERVICE / 'qa/manifests/M14-CP04.json')
        go_count = 0
        for group in ('m14_cp04_load', 'm14_cp04_db'):
            cases = load(run / (group + '-cases.json'))
            if not verify_go_cases(manifest[group+'_go'], cases):
                raise ValueError('CP04 Go load/DB evidence incomplete')
            go_count += len(cases)
        api = load(run / 'm14cp04-api-cases.json')
        expected = set(manifest['m14_cp04_python'])
        if (api['exit_status'] != 0 or set(api['collected_ids']) != expected or len(api['cases']) != len(expected)
                or {case['id'] for case in api['cases']} != expected or any(c['status'] != 'PASS' for c in api['cases'])):
            raise ValueError('CP04 Python evidence incomplete')
        budget = load(SERVICE / 'qa/manifests/M14-CP04-budget.json')
        accepted = review(load(run/'m14cp04-load-raw.json'), budget)
        if accepted != load(run/'m14cp04-load-acceptance.json') or accepted != summary.get('m14_cp04_performance'):
            raise ValueError('CP04 load acceptance/raw/summary mismatch')
        db = load(run/'m14cp04-db-plan-pool.json')
        if db.get('status') != 'PASS_SYNTHETIC_ONLY' or db.get('query') != 'releasedGradesSQL' or db.get('export_audits') != 384:
            raise ValueError('CP04 DB evidence incomplete')
        for w in ('small','boundary'):
            plan = db['plans'][w]
            if len(plan) != 1 or plan[0]['Plan']['Actual Rows'] != budget[w]['items'] or number(plan[0]['Execution Time']) > budget['db_execution_ms']:
                raise ValueError('CP04 actual query plan bound failed')
        pool = db['pool']
        if (pool['max_connections'] != budget['pool_max_connections'] or pool['cancel_deadline_ms'] != budget['pool_cancel_ms']
                or number(pool['recovery_ms']) > budget['pool_recovery_ms'] or pool['canceled_acquires'] < 1
                or pool['acquired_after_release'] != 0 or number(pool['observed_cancel_ms']) < budget['pool_cancel_ms']*.8):
            raise ValueError('CP04 pool cancellation/recovery failed')
        if summary.get('m14_cp04_reconciliation') != 'PASS':
            raise ValueError('CP04 reconciliation missing')
        results['M14-CP04'] = {'go_pass':go_count,'api_pass':len(expected),'status':'PASS_LOCAL_SYNTHETIC',
                                  'measured_requests':accepted['measured_requests'],'production_sla_verified':False}
    for dependency in ('m03_cp05', 'm13_cp04', 'm13_cp04_runner', 'm13_cp04_consumer'):
        if summary.get(dependency, {}).get('status') != 'PASS_SYNTHETIC_ONLY':
            raise ValueError('Required dependency missing: ' + dependency)
    if include_cp03 and summary.get('m14_cp03_reconciliation') != 'PASS':
        raise ValueError('CP03 export audit reconciliation missing')
    for name, value in summary.items():
        if 'reconciliation' in name and value != 'PASS':
            raise ValueError('Reconciliation failed: ' + name)
        if isinstance(value, dict) and 'status' in value and value['status'] != 'PASS_SYNTHETIC_ONLY':
            raise ValueError('Required journey slice not passing: ' + name)
    receipts = []
    for path in sorted(run.glob('*-receipt.json')):
        receipt = load(path)
        if 'restore' not in receipt or 'artifacts' not in receipt:
            continue
        if receipt['restore'] != 'VERIFIED_FIXTURE_SCOPE':
            raise ValueError('Restore receipt not verified')
        for filename, digest in receipt['artifacts'].items():
            artifact = run / filename
            if artifact.parent != run or sha256(artifact.read_bytes()).hexdigest() != digest:
                raise ValueError('Backup artifact mismatch')
        receipts.append(path.name)
    required = {'prestate-receipt.json', 'pre_m03_cp05-receipt.json', 'pre_m13_cp03-receipt.json'}
    if include_cp04:
        required.add('pre_m14_cp04-receipt.json')
    if not required <= set(receipts):
        raise ValueError('Dependency backup proof missing')
    if summary['shared_database_access'] != 'NONE' or summary['database_deletion'] != 'NONE':
        raise ValueError('Data safety boundary mismatch')
    result = {'run_id': summary['run_id'], 'source_hash': current,
              'review': 'PASS_M14_CP01_CP02_CP03_CP04_LOCAL_SYNTHETIC' if include_cp04 else 'PASS_M14_CP01_CP02_CP03_LOCAL_SYNTHETIC' if include_cp03 else 'PASS_M14_CP01_CP02_LOCAL_SYNTHETIC', 'checkpoints': results,
              'verified_backup_restore_receipts': receipts,
              'summary_sha256': sha256((run / 'summary.json').read_bytes()).hexdigest(),
              'canonical_module_verified': False, 'database_access_during_review': 'NONE',
              'cp03_acceptance': 'PASS_LOCAL_SYNTHETIC' if include_cp03 else 'NOT_ASSESSED', 'git_operations': 'NONE'}
    result['cp04_acceptance'] = 'PASS_LOCAL_SYNTHETIC' if include_cp04 else 'NOT_ASSESSED'
    if include_cp04:
        # Bind key performance artifacts as well as raw command and backup evidence.
        result['cp04_artifact_hashes'] = {p.name:sha256(p.read_bytes()).hexdigest() for p in sorted(run.glob('m14cp04-*')) if p.is_file()}
    destination = SERVICE.parent / 'private-backup' / ('m14-review-' + datetime.now(timezone.utc).strftime('%Y%m%dT%H%M%S%fZ') + '.json')
    destination.parent.mkdir(parents=True, exist_ok=True)
    with destination.open('x', encoding='utf-8') as handle:
        json.dump(result, handle, indent=2); handle.write('\n')
    print(json.dumps({'review_file': str(destination), **result}))


if __name__ == '__main__':
    main()
