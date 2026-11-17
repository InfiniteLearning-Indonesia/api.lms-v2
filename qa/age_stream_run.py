"""Unattended synthetic age QA, no database, cleanup, proxy changes or publish.

Run from an ordinary operator terminal. Tool acquisition uses standard Go
checksum verification and only a workspace-local GOBIN/cache. No secret or
raw crypto output is printed; missing actual crypto evidence stays BLOCKED.
"""
import argparse
from datetime import datetime, timezone
import json
import os
from pathlib import Path
import re
import subprocess
import sys
import time
import uuid

from qa import storage
from qa.evidence import source_fingerprint, verify_go_cases
from qa.operators.context_archive import archive, walk, config, SKIP
from qa.run import ROOT, clean_environment, run_lock

VERSION = 'v1.3.2'
EXPECTED = [
    'lmsv3/internal/migration/backup:TestAgeActualLargeRoundTrip',
    'lmsv3/internal/migration/backup:TestAgeActualTamperAndWrongKey',
    'lmsv3/internal/migration/backup:TestAgeActualBindingAndShortInput',
    'lmsv3/internal/migration/backup:TestAgeActualCancellationAndWriterFailure',
    'lmsv3/internal/migration/backup:TestAgeActualProtectedOutput',
    'lmsv3/internal/migration/backup:TestAgeActualAuthenticatedPackage',
    'lmsv3/internal/migration/backup:TestAgeActualPackageTamperAndBinding',
    'lmsv3/internal/migration/backup:TestAgeActualProtectedRecoveryAndCLI',
    'lmsv3/internal/migration/backup:TestAgeActualRecoveryRejectsCorruptionBeforeCreation',
]


def manifest(service):
    value = json.loads((service / 'qa/manifests/M01-AGE-STREAM.json').read_text(encoding='utf-8'))
    if (value.get('scope') != 'LOCAL_SYNTHETIC_AGE_CLI_ONLY' or value.get('tool_version') != VERSION
            or value.get('go_tag') != 'qaage' or value.get('full_module_verified') is not False
            or value.get('actual_restore_verified') is not False or value.get('expected_tests') != EXPECTED):
        raise ValueError('AGE_MANIFEST_INVALID')
    return value


def tool_paths(service):
    extension = '.exe' if os.name == 'nt' else ''
    root = service / '.cache/qa-tools' / ('age-' + VERSION)
    return root, root / ('age' + extension), root / ('age-keygen' + extension)


def probe(service):
    root, age, keygen = tool_paths(service)
    present = all(p.is_file() and not storage.reparse(p) for p in (age, keygen))
    return {'scope': 'LOCAL_SYNTHETIC_AGE_CLI_ONLY', 'tool_version': VERSION,
            'status': 'TOOLS_PRESENT_NOT_CRYPTO_VERIFIED' if present else 'BLOCKED_AGE_TOOLS_UNAVAILABLE',
            'crypto_suite': 'NOT_RUN', 'authorizes_execution': False, 'actual_restore_verified': False,
            'database_access': 'NONE', 'download_attempted': False, 'proxy_changes': 'NONE'}


def review_cases(cases, expected, before, after):
    if before != after or not re.fullmatch(r'[a-f0-9]{64}', before) or not verify_go_cases(expected, cases):
        raise ValueError('AGE_EVIDENCE_NOT_PASS')


def go_cases(stdout):
    cases = []
    for line in stdout.splitlines():
        try:
            value = json.loads(line)
        except ValueError:
            continue
        if value.get('Test') and value.get('Action') in {'pass', 'fail', 'skip'}:
            cases.append({'package': value['Package'], 'name': value['Test'], 'status': value['Action']})
    return cases


def download_environment(service):
    env = clean_environment(service, os.environ)
    # Keep the operator's normal proxies. Never unset a denied proxy, disable
    # TLS, use GOPRIVATE/GONOSUMDB, or set GOSUMDB=off to get a green result.
    for key, value in os.environ.items():
        if key.upper() in {'HTTP_PROXY', 'HTTPS_PROXY', 'ALL_PROXY', 'NO_PROXY', 'GOPROXY'}:
            env[key] = value
    if 'GOPROXY' not in os.environ:
        env.pop('GOPROXY', None)
    env['GOSUMDB'] = 'sum.golang.org'
    env['GOPATH'] = str(service / '.cache/qa-go-path')
    return env


def acquire_tools(service, destination, command, allow_install):
    root, age, keygen = tool_paths(service)
    proof_path = root / 'verified-install.json'
    if proof_path.is_file():
        proof = json.loads(proof_path.read_text(encoding='utf-8'))
        if (storage.reparse(proof_path) or proof.get('version') != VERSION
                or proof.get('checksum_verification') != 'GO_DEFAULT_SUMDB_ENABLED'
                or not re.fullmatch(r'h1:[A-Za-z0-9+/]{43}=', proof.get('module_sum', ''))):
            raise ValueError('AGE_INSTALL_PROOF_INVALID')
        if any(not p.is_file() or storage.reparse(p) or storage.file_hash(p) != proof.get(p.name) for p in (age, keygen)):
            raise ValueError('AGE_INSTALL_PROOF_MISMATCH')
        return age, keygen, proof
    if not allow_install:
        raise ValueError('AGE_TOOLS_UNAVAILABLE_DOWNLOAD_NOT_ALLOWED')
    if age.exists() or keygen.exists():
        raise ValueError('AGE_PARTIAL_INSTALL_REQUIRES_REVIEW')
    if any(storage.reparse(p) for p in [root, *root.parents] if p.exists()):
        raise ValueError('AGE_LINKED_TOOL_CACHE')
    root.mkdir(parents=True, exist_ok=True)
    env = download_environment(service)
    result = command('age-download', ['go', 'mod', 'download', '-json', 'filippo.io/age@' + VERSION], env, 300)
    module = json.loads(result.stdout)
    if module.get('Path') != 'filippo.io/age' or module.get('Version') != VERSION or not re.fullmatch(r'h1:[A-Za-z0-9+/]{43}=', module.get('Sum', '')):
        raise ValueError('AGE_MODULE_CHECKSUM_UNAVAILABLE')
    env['GOBIN'] = str(root)
    command('age-install', ['go', 'install', '-trimpath', 'filippo.io/age/cmd/age@' + VERSION,
                          'filippo.io/age/cmd/age-keygen@' + VERSION], env, 600)
    proof = {'version': VERSION, 'module_sum': module['Sum'], 'checksum_verification': 'GO_DEFAULT_SUMDB_ENABLED'}
    for p in (age, keygen):
        if not p.is_file() or storage.reparse(p):
            raise ValueError('AGE_TOOL_INSTALL_INCOMPLETE')
        proof[p.name] = storage.file_hash(p)
    storage.dump_new(proof_path, proof)
    return age, keygen, proof


def execute(service=ROOT, allow_install=False, max_minutes=60):
    if max_minutes < 1 or max_minutes > 60:
        raise ValueError('AGE_RUN_TIME_BUDGET_INVALID')
    frozen = manifest(service)
    source = source_fingerprint(service)
    parent = storage.project_locations(service)[0]
    parent.mkdir(exist_ok=True)
    # No retention hook: this task does not authorize deleting any old payload.
    with run_lock(parent / 'storage.lock'), run_lock(parent / 'age-stream.lock'):
        print('[age-qa] measuring managed storage; read-only, no cleanup; may take several minutes', flush=True)
        admitted = storage.preflight(service, parent)
        run_id = datetime.now(timezone.utc).strftime('%Y%m%dT%H%M%SZ') + '-age-stream-' + uuid.uuid4().hex[:12]
        destination = service.parent / 'private-backup' / run_id
        destination.mkdir(parents=True)
        storage.dump_new(destination / 'storage-preflight.json', admitted)
        errors = []
        print('[age-qa] source backup and reopened-byte verification', flush=True)
        receipt = archive(destination, 'source-before', [p for p in walk(service, SKIP, errors) if not config(p)])
        if errors or source != source_fingerprint(service):
            raise ValueError('AGE_SOURCE_BACKUP_UNSTABLE')
        storage.dump_new(destination / 'backup-receipt.json', {'source_hash': source, 'archive': receipt, 'database_access': 'NONE'})
        deadline = time.monotonic() + max_minutes * 60
        env = clean_environment(service, os.environ)
        calls = []

        def command(name, args, child_env=env, timeout=180):
            remaining = int(deadline - time.monotonic())
            if remaining < 1:
                raise ValueError('AGE_TIME_BUDGET_EXHAUSTED')
            print('[age-qa] ' + name, flush=True)
            result = subprocess.run(args, cwd=service, env=child_env, capture_output=True,
                                    text=True, encoding='utf-8', errors='replace', timeout=min(timeout, remaining))
            with (destination / (name + '.log')).open('x', encoding='utf-8') as handle:
                handle.write(result.stdout); handle.write(result.stderr)
            calls.append({'name': name, 'exit_code': result.returncode})
            storage.guard_growth(destination)
            if result.returncode:
                raise ValueError('AGE_COMMAND_FAILED_' + name.upper().replace('-', '_'))
            if source != source_fingerprint(service):
                raise ValueError('AGE_SOURCE_CHANGED')
            return result

        result = {'run_id': run_id, 'source_hash': source, 'scope': frozen['scope'], 'status': 'BLOCKED',
                  'crypto_suite': 'NOT_RUN', 'full_module_verified': False, 'actual_restore_verified': False,
                  'database_access': 'NONE', 'deletion': 'NONE', 'commit_push': 'NONE', 'proxy_changes': 'NONE'}
        try:
            age, keygen, proof = acquire_tools(service, destination, command, allow_install)
            env.update(QA_AGE_BINARY=str(age), QA_AGE_SHA256=proof[age.name], QA_AGE_MODULE_SUM=proof['module_sum'],
                       QA_AGE_KEYGEN_BINARY=str(keygen), QA_AGE_KEYGEN_SHA256=proof[keygen.name])
            if allow_install:
                command('project-download', ['go', 'mod', 'download'], download_environment(service), 300)
            # Public CLI is built from this same frozen tree, never a stale PATH tool.
            binaries = destination / 'bin'
            binaries.mkdir()
            package_binary = binaries / ('baseline-package.exe' if os.name == 'nt' else 'baseline-package')
            command('baseline-package-build', ['go', 'build', '-trimpath', '-o', str(package_binary), './cmd/baseline-package'])
            env['QA_BASELINE_PACKAGE_BINARY'] = str(package_binary)
            crypto = command('crypto-actual', ['go', 'test', '-json', '-count=1', '-tags=qaage', '-run=^TestAgeActual',
                                                './internal/migration/backup'], timeout=600)
            cases = go_cases(crypto.stdout)
            storage.dump_new(destination / 'crypto-cases.json', {'cases': cases})
            review_cases(cases, frozen['expected_tests'], source, source_fingerprint(service))
            result['crypto_suite'] = 'PASS_ACTUAL_PINNED_AGE_SYNTHETIC_ONLY'
            unit = command('go-unit', ['go', 'test', '-json', '-count=1', './...'], timeout=300)
            expected = json.loads((service / 'qa/manifests/go-tests.json').read_text())['expected_tests']
            review_cases(go_cases(unit.stdout), expected, source, source_fingerprint(service))
            command('go-vet', ['go', 'vet', './...'])
            result.update(status='PASS_LOCAL_CRYPTO_SLICE_ONLY', actual_crypto_case_count=len(cases),
                          default_go_case_count=len(expected), source_unchanged=True)
        except (OSError, ValueError, KeyError, subprocess.TimeoutExpired) as error:
            # A fixed reason code only. Logs stay private; key values never enter args.
            reason = str(error) if isinstance(error, ValueError) and re.fullmatch(r'[A-Z0-9_]{1,100}', str(error)) else type(error).__name__
            result.update(status='BLOCKED_OR_FAILED', reason=reason)
        result['commands'] = calls
        storage.dump_new(destination / 'summary.json', result)
        print(json.dumps({'status': result['status'], 'crypto_suite': result['crypto_suite'], 'report': str(destination),
                          'full_module_verified': False, 'actual_restore_verified': False}), flush=True)
        return 0 if result['status'] == 'PASS_LOCAL_CRYPTO_SLICE_ONLY' else 2


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--probe-only', action='store_true')
    group = parser.add_mutually_exclusive_group()
    group.add_argument('--install-tools', action='store_true')
    group.add_argument('--no-download', action='store_true')
    parser.add_argument('--max-minutes', type=int, default=60)
    args = parser.parse_args()
    try:
        manifest(ROOT)
        if args.probe_only:
            print(json.dumps(probe(ROOT)), flush=True)
            return 2
        return execute(allow_install=args.install_tools, max_minutes=args.max_minutes)
    except (OSError, ValueError, KeyError) as error:
        prefix = str(error).split(':', 1)[0] if isinstance(error, ValueError) else ''
        reason = prefix if re.fullmatch(r'[A-Z][A-Z0-9_]{0,99}', prefix) else type(error).__name__
        print(json.dumps({'status': 'BLOCKED_BEFORE_CRYPTO_QA', 'error_type': type(error).__name__,
                          'reason': reason,
                          'crypto_suite': 'NOT_RUN', 'database_access': 'NONE', 'deletion': 'NONE'}), flush=True)
        return 2


if __name__ == '__main__':
    raise SystemExit(main())
