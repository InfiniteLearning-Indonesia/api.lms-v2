"""Source-bound local ops preflight: green process exit alone is insufficient."""
import json
from pathlib import Path
import subprocess
import sys

from qa import storage


def load_ops_manifest(service):
    value=json.loads((service/'qa/manifests/M14-CP04-preparation.json').read_text(encoding='utf-8'))
    if (value.get('version')!=1 or value.get('profile')!='LOCAL_FILESYSTEM_PREPARATION_ONLY'
            or value.get('full_module_verified') is not False):raise ValueError('OPS_MANIFEST_SCOPE_INVALID')
    tests=value.get('tests');expected=value.get('expected_checks')
    if (not isinstance(tests,list) or not tests or len(tests)!=len(set(tests))
            or not isinstance(expected,list) or not expected or len(expected)!=len(set(expected))
            or any(not isinstance(i,str) or not i for i in expected)):raise ValueError('OPS_MANIFEST_CASES_INVALID')
    for name in tests:
        path=Path(name)
        if (path.parent.as_posix()!='qa/tests/ops' or not path.name.startswith('test_')
                or path.suffix!='.py' or not (service/path).is_file()):raise ValueError('OPS_MANIFEST_PATH_INVALID')
    return value


def verify_ops_cases(value,expected):
    if not isinstance(value,dict) or value.get('exit_status')!=0:raise ValueError('OPS_CASE_EXIT_NOT_PASS')
    collected=value.get('collected_ids');cases=value.get('cases')
    if (not isinstance(collected,list) or len(collected)!=len(set(collected)) or set(collected)!=set(expected)
            or not isinstance(cases,list)):raise ValueError('OPS_COLLECTED_CASES_MISMATCH')
    if any(not isinstance(c,dict) or not isinstance(c.get('id'),str) for c in cases):raise ValueError('OPS_CASE_FORMAT_INVALID')
    ids=[c['id'] for c in cases]
    if len(ids)!=len(set(ids)) or set(ids)!=set(expected):raise ValueError('OPS_EXECUTED_CASES_MISMATCH')
    if any(c.get('status')!='PASS' for c in cases):raise ValueError('OPS_CASE_NONPASS')
    return len(cases)


def run_ops_checks(service,destination,environment,timeout=180):
    service=Path(service);destination=Path(destination)
    manifest=load_ops_manifest(service)
    cases=destination/'ops-cases.json';artifacts=destination/'ops-artifacts';artifacts.mkdir()
    # Never inherit an older QA_CASE_RESULTS/QA_ARTIFACT_ROOT from the shell.
    env=dict(environment,QA_CASE_RESULTS=str(cases),QA_ARTIFACT_ROOT=str(artifacts))
    result=subprocess.run([sys.executable,'-m','pytest','-p','qa.plugin',*manifest['tests'],
                           '-q','-p','no:cacheprovider','--basetemp='+str(destination/'pytest-temp')],
                           cwd=service,env=env,capture_output=True,text=True,timeout=timeout)
    log=destination/'ops-targeted-regression.log'
    with log.open('x',encoding='utf-8') as handle:handle.write(result.stdout);handle.write(result.stderr)
    record={'name':'ops-targeted-regression','exit_code':result.returncode,'log_sha256':storage.file_hash(log),
            'expected_case_count':len(manifest['expected_checks']),'verified_case_count':0,
            'case_evidence_scope':'LOCAL_FILESYSTEM_PREPARATION_ONLY','full_module_verified':False,
            'manifest_sha256':storage.file_hash(service/'qa/manifests/M14-CP04-preparation.json')}
    try:
        if result.returncode:raise ValueError('OPS_PROCESS_FAILED')
        record['verified_case_count']=verify_ops_cases(json.loads(cases.read_text(encoding='utf-8')),manifest['expected_checks'])
        record.update(status='PASS_LOCAL_PREPARATION_ONLY',cases_sha256=storage.file_hash(cases))
    except (OSError,ValueError,TypeError,KeyError) as error:
        record.update(status='REJECTED',reason=str(error))
        storage.dump_new(destination/'ops-check.json',record)
        raise ValueError('OPS_PREFLIGHT_REJECTED: '+str(error)) from error
    storage.dump_new(destination/'ops-check.json',record)
    return record
