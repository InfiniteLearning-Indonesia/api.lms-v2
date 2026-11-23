"""Run the M04 password, Google, or Class invitation journey locally."""

from datetime import datetime, timezone
import json
import os
import secrets
import shutil
import socket
import subprocess
import sys
import uuid

from qa import storage
from qa.evidence import source_fingerprint
from qa.operators.restore_legacy_baseline import (
    Cluster, PG_BIN, SERVICE, Stop, bounded, filtered_toc, local_environment,
    private_mkdir, write_new,
)
from qa.run import clean_environment, run_lock

TARGET = 'password_qa'
ROLE = 'm04_password_writer'
PROFILES = {
    'password': ('CP03', {'TestM04PasswordPostgresJourney'}),
    'google': ('CP04', {'TestGoogleProviderVerification', 'TestM04GooglePostgresJourney'}),
    'class-invitations': ('CP05', {'TestM04ClassInvitationPostgresJourney'}),
}


class PasswordFixtureCluster(Cluster):
    target = TARGET

    def env(self, database='postgres', role='admin'):
        if database not in {'postgres', self.target, 'empty_proof'} or role != 'admin':
            raise Stop('PASSWORD_FIXTURE_SCOPE')
        env = local_environment(os.environ, self.port, self.passwords['admin'], 'lms_baseline_admin', 'postgres')
        env['PGDATABASE'] = database
        return env

    def sql_admin(self, database, query):
        return self.sql(database, query, 'admin')


def prepare_empty_target(root, cluster, receipt):
    target = cluster.target
    for tool in ('initdb', 'pg_ctl', 'pg_controldata', 'createdb', 'pg_dump', 'pg_restore', 'psql'):
        if b'(PostgreSQL) 18.' not in cluster.command(tool, ['--version']):
            raise Stop('PASSWORD_FIXTURE_PG18_REQUIRED')
    password = root / 'bootstrap.pw'
    write_new(password, cluster.passwords['admin'].encode())
    cluster.command('initdb', ['-D', cluster.data, '--username=lms_baseline_admin',
                    '--pwfile=' + str(password), '--auth-host=scram-sha-256', '--auth-local=scram-sha-256',
                    '--encoding=UTF8', '--no-locale', '--no-clean'])
    if not (cluster.data / 'global/pg_control').is_file():
        raise Stop('PASSWORD_FIXTURE_INITDB_INCOMPLETE')
    with (cluster.data / 'postgresql.conf').open('a', encoding='utf-8') as config:
        config.write(f"\nlisten_addresses='127.0.0.1'\nport={cluster.port}\nssl=off\n"
                     "max_connections=12\nshared_buffers='16MB'\nmin_wal_size='32MB'\nmax_wal_size='64MB'\n"
                     "shared_preload_libraries=''\nlog_statement='none'\nlog_min_error_statement='panic'\n")
    cluster.start_attempted = True
    cluster.command('pg_ctl', ['-D', cluster.data, '-l', root / 'postgres.log', '-w', '-t', '20', 'start'])
    cluster.identity()
    for db in (target, 'empty_proof'):
        cluster.command('createdb', ['--template=template0', '--encoding=UTF8', '--no-password', db])

    print('[m04] backing up and restoring empty target before installing fixture schema', flush=True)
    archive = root / 'empty.dump'
    write_new(archive, cluster.command('pg_dump', ['--format=custom', '--schema=public', '--no-owner',
             '--no-privileges', '--no-password'], target))
    sql = root / 'empty.sql'
    write_new(sql, cluster.command('pg_restore', ['--no-owner', '--no-privileges', '--file=-', archive]))
    toc = root / 'empty.list'
    write_new(toc, filtered_toc(cluster.command('pg_restore', ['--list', archive]))[0])
    cluster.command('pg_restore', ['--dbname=empty_proof', '--single-transaction', '--exit-on-error',
                    '--no-owner', '--no-privileges', '--no-password', '--use-list='+str(toc), archive], 'empty_proof')
    probe = "SELECT count(*) FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='public'"
    if cluster.sql_admin(target, probe).strip() != b'0' or cluster.sql_admin('empty_proof', probe).strip() != b'0':
        raise Stop('PASSWORD_FIXTURE_EMPTY_RESTORE_MISMATCH')
    receipt['empty_target_backup'] = {'custom_sha256': storage.file_hash(archive),
                                      'sql_sha256': storage.file_hash(sql), 'restore_verified': True}


def exercise(root, cluster, receipt, profile='password'):
    prepare_empty_target(root, cluster, receipt)
    marker, writer = secrets.token_hex(32), secrets.token_hex(32)
    cluster.sql_admin(TARGET, "CREATE SCHEMA qa_password_guard;"
        "CREATE TABLE qa_password_guard.target(marker text PRIMARY KEY);"
        f"INSERT INTO qa_password_guard.target VALUES ('{marker}');"
        f"CREATE ROLE {ROLE} LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOBYPASSRLS PASSWORD '{writer}';")
    env = clean_environment(SERVICE, os.environ)
    env.update(GOTOOLCHAIN='local', QA_M04_PORT=str(cluster.port), QA_M04_ADMIN=cluster.passwords['admin'],
               QA_M04_WRITER=writer, QA_M04_MARKER=marker, QA_M04_CLUSTER=str(cluster.data),
               QA_M04_PYTHON=sys.executable, QA_M04_API_PROBE=str(SERVICE / 'qa/operators/probe_m04_password_api.py'))
    _, expected = PROFILES[profile]
    if profile == 'google':
        env['QA_M04_GOOGLE_PROBE'] = str(SERVICE / 'qa/operators/probe_m04_google_api.py')
    elif profile == 'class-invitations':
        env['QA_M04_CLASS_PROBE'] = str(SERVICE / 'qa/operators/probe_m04_class_api.py')
    print(f'[m04] testing {profile} over HTTPS with restricted database role', flush=True)
    run = subprocess.run(['go', 'test', '-buildvcs=false', '-count=1', '-tags=password_rehearsal',
                          '-run=^('+'|'.join(sorted(expected))+')$', '-json', './internal/identity'],
                         cwd=SERVICE, env=env, capture_output=True, timeout=220)
    write_new(root / 'test-output.jsonl', run.stdout)
    events = [json.loads(line) for line in run.stdout.splitlines() if line.startswith(b'{')]
    passed = [e['Test'] for e in events if e['Action'] == 'pass' and 'Test' in e and '/' not in e['Test']]
    if run.returncode or len(passed) != len(expected) or set(passed) != expected:
        raise Stop(profile.upper().replace('-', '_') + '_FIXTURE_INTEGRATION_FAILED')
    receipt['cases'] = passed
    receipt['playwright_request_cases'] = 5


def main():
    options = {(): 'password', ('--google',): 'google', ('--class-invitations',): 'class-invitations'}
    profile = options.get(tuple(sys.argv[1:]))
    if profile is None:
        return 2
    label = profile.upper().replace('-', '_')
    checkpoint, _ = PROFILES[profile]
    parent = SERVICE.parent / 'private-backup'
    run_id = datetime.now(timezone.utc).strftime('%Y%m%dT%H%M%SZ')+f'-m04-{profile}-'+uuid.uuid4().hex[:12]
    root = parent / run_id
    receipt = {'status': f'BLOCKED_BEFORE_{label}_QA', 'run_id': run_id, 'cases': []}
    cluster = None
    try:
        private_mkdir(root)
        write_new(root / 'directory-check.txt', f'Synthetic {profile} fixture.\n'.encode())
        with run_lock(parent / 'm04-password.lock'):
            if shutil.disk_usage(root).free < 6 * 1024**3:
                raise Stop('PASSWORD_FIXTURE_DISK_SPACE')
            for known in PROFILES:
                for prior in parent.glob(f'*-m04-{known}-*'):
                    if (prior / 'postgres/postmaster.pid').exists():
                        raise Stop('M04_PRIOR_CLUSTER_NEEDS_REVIEW')
            with socket.socket() as listener:
                listener.bind(('127.0.0.1', 0))
                port = listener.getsockname()[1]
            cluster = PasswordFixtureCluster(root, PG_BIN, port, {'admin': secrets.token_hex(32)})
            receipt['status'] = f'FAILED_{label}_QA'
            try:
                receipt['source_hash'] = source_fingerprint(SERVICE)
                exercise(root, cluster, receipt, profile=profile)
            finally:
                if cluster.start_attempted:
                    receipt['normal_stop_verified'] = cluster.stop()
            if not receipt.get('normal_stop_verified'):
                raise Stop('PASSWORD_FIXTURE_STOP_NOT_VERIFIED')
            if source_fingerprint(SERVICE) != receipt['source_hash']:
                raise Stop('M04_SOURCE_CHANGED_DURING_QA')
            receipt['run_bytes'] = bounded(root)
            receipt['status'] = f'PASS_M04_{checkpoint}_LOCAL_SYNTHETIC_ONLY'
    except (OSError, Stop, ValueError, KeyError, subprocess.TimeoutExpired, KeyboardInterrupt) as error:
        receipt['reason'] = str(error) if isinstance(error, Stop) else type(error).__name__
    try:
        write_new(root / 'summary.json', json.dumps(receipt, indent=2).encode())
    except OSError:
        if receipt['status'].startswith('PASS'):
            receipt.update(status=f'FAILED_{label}_QA', reason='RECEIPT_WRITE_FAILED')
    print(json.dumps(receipt), flush=True)
    return 0 if receipt['status'].startswith('PASS') else 2


if __name__ == '__main__':
    raise SystemExit(main())
