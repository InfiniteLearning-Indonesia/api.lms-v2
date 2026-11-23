"""Owner-terminal PostgreSQL proof for the pinned Go core importer."""

from datetime import datetime, timezone
import json
import os
import secrets
import shutil
import socket
import subprocess
import uuid

from qa import storage
from qa.operators.restore_legacy_baseline import (
    Cluster, PG_BIN, SERVICE, SNAPSHOT, PIN, Stop, bounded, filtered_toc,
    local_environment, private_mkdir, write_new,
)
from qa.run import clean_environment, run_lock

TARGET = 'legacy_core_v3'
ROLE = 'lms_core_import'
CASES = ('PASS_GO_DRY_RUN_ONLY', 'PASS_LOCAL_CORE_ROLLBACK',
         'PASS_LOCAL_CORE_IMPORTED', 'PASS_LOCAL_CORE_REPLAY')


class RehearsalCluster(Cluster):
    def env(self, database='postgres', role='admin'):
        if database not in {'postgres', TARGET, 'empty_proof'} or role not in {'admin', 'owner'}:
            raise Stop('CORE_TARGET_SCOPE_REJECTED')
        user = 'lms_baseline_admin' if role == 'admin' else ROLE
        env = local_environment(os.environ, self.port, self.passwords[role], user, 'postgres')
        env['PGDATABASE'] = database
        return env

    def create_db(self, name):
        if name not in {TARGET, 'empty_proof'}:
            raise Stop('CORE_TARGET_SCOPE_REJECTED')
        self.command('createdb', ['--no-password', '--maintenance-db=postgres',
                                  '--template=template0', '--encoding=UTF8', '--owner=' + ROLE, name])


def save(path, value):
    write_new(path, json.dumps(value, indent=2).encode())


def go_result(binary, source, env, extra):
    result = subprocess.run([str(binary), '--snapshot', str(source), *extra],
                            env=env, cwd=SERVICE, capture_output=True, timeout=40)
    if result.returncode:
        code = result.stderr.decode('utf-8', errors='replace').strip()
        known_codes = {'CORE_SNAPSHOT_REJECTED', 'CORE_MAPPING_REJECTED', 'CORE_LOCAL_TARGET_REJECTED',
                       'CORE_TARGET_BACKUP_REJECTED', 'CORE_TRANSACTION_FAILED',
                       'CORE_TARGET_RECONCILIATION_FAILED'}
        raise Stop(code if code in known_codes else 'GO_CORE_IMPORT_FAILED')
    return json.loads(result.stdout)


def exercise(root, cluster, env, summary):
    source = SERVICE.parent / 'private-backup' / SNAPSHOT / 'database-public.sql'
    binary = root / 'legacy-core-import.exe'
    build = subprocess.run(['go', 'build', '-buildvcs=false', '-o', str(binary), './cmd/legacy-core-import'],
                           cwd=SERVICE, capture_output=True, env=env, timeout=120)
    if build.returncode:
        raise Stop('GO_CORE_BUILD_FAILED')
    summary['binary_sha256'] = storage.file_hash(binary)
    plan = go_result(binary, source, env, [])
    if plan['status'] != CASES[0]:
        raise Stop('CORE_DRY_RUN_FAILED')
    summary['cases'].append(plan['status'])
    summary['plan'] = plan

    for tool in ('initdb', 'pg_ctl', 'pg_controldata', 'createdb', 'pg_dump', 'pg_restore', 'psql'):
        if b'(PostgreSQL) 18.' not in cluster.command(tool, ['--version']):
            raise Stop('CORE_POSTGRES18_REQUIRED')
    password = root / 'bootstrap.pw'
    write_new(password, cluster.passwords['admin'].encode())
    cluster.command('initdb', ['-D', cluster.data, '--username=lms_baseline_admin',
                    '--pwfile=' + str(password), '--auth-host=scram-sha-256',
                    '--auth-local=scram-sha-256', '--encoding=UTF8', '--no-locale', '--no-clean'])
    if not (cluster.data / 'global/pg_control').is_file():
        raise Stop('CORE_INITDB_INCOMPLETE')
    with (cluster.data / 'postgresql.conf').open('a', encoding='utf-8') as config:
        config.write(f"\nlisten_addresses='127.0.0.1'\nport={cluster.port}\nssl=off\n"
                     "max_connections=8\nshared_buffers='16MB'\nmin_wal_size='32MB'\nmax_wal_size='64MB'\n"
                     "shared_preload_libraries=''\nlog_statement='none'\nlog_min_error_statement='panic'\n")
    cluster.start_attempted = True
    cluster.command('pg_ctl', ['-D', cluster.data, '-l', root / 'postgres.log', '-w', '-t', '20', 'start'])
    cluster.identity()
    cluster.sql('postgres', f"CREATE ROLE {ROLE} LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOBYPASSRLS "
                f"PASSWORD '{cluster.passwords['owner']}';", 'admin')
    cluster.create_db(TARGET)
    cluster.create_db('empty_proof')

    print('[core] backing up and restoring empty public scope before import', flush=True)
    prestate = root / 'prestate'
    prestate.mkdir()
    archive = prestate / 'empty.dump'
    write_new(archive, cluster.command('pg_dump', ['--format=custom', '--schema=public', '--no-owner',
              '--no-privileges', '--no-password'], TARGET, 'owner'))
    sql = prestate / 'empty.sql'
    write_new(sql, cluster.command('pg_restore', ['--no-owner', '--no-privileges', '--file=-', archive]))
    toc = prestate / 'empty.list'
    write_new(toc, filtered_toc(cluster.command('pg_restore', ['--list', archive]))[0])
    cluster.command('pg_restore', ['--dbname=empty_proof', '--single-transaction', '--exit-on-error', '--no-owner',
                    '--no-privileges', '--no-password', '--use-list=' + str(toc), archive], 'empty_proof', 'owner')
    if cluster.empty_state(TARGET) != cluster.empty_state('empty_proof'):
        raise Stop('CORE_EMPTY_RESTORE_MISMATCH')
    backup_hash = storage.file_hash(archive)
    summary['empty_target_backup'] = {'custom_sha256': backup_hash, 'sql_sha256': storage.file_hash(sql),
                                      'restore_verified': True, 'scope': 'EMPTY_PUBLIC'}

    marker = secrets.token_hex(32)
    cluster.sql(TARGET, "CREATE SCHEMA rehearsal_guard;"
                "CREATE TABLE rehearsal_guard.target(marker text PRIMARY KEY,data_directory text NOT NULL);"
                f"INSERT INTO rehearsal_guard.target VALUES('{marker}',current_setting('data_directory'));"
                f"GRANT USAGE ON SCHEMA rehearsal_guard TO {ROLE};"
                f"GRANT SELECT ON rehearsal_guard.target TO {ROLE};", 'admin')
    proof = root / 'target-proof.json'
    save(proof, {'port': cluster.port, 'marker': marker, 'backup_sha256': backup_hash,
                 'verified_at': datetime.now(timezone.utc).isoformat(), 'restored': True})
    go_env = dict(env, LMS_CORE_REHEARSAL_PASSWORD=cluster.passwords['owner'])
    for case, options in zip(CASES[1:], [['--prove-rollback'], [], []], strict=True):
        print('[core] ' + case.removeprefix('PASS_LOCAL_CORE_').lower(), flush=True)
        report = go_result(binary, source, go_env, ['--target-proof', str(proof), *options])
        if report != dict(plan, status=case):
            raise Stop('CORE_PHASE_RESULT_MISMATCH')
        summary['cases'].append(case)
        bounded(root)
    size, digest = PIN['database-public.sql']
    if source.stat().st_size != size or storage.file_hash(source) != digest:
        raise Stop('CORE_SOURCE_SNAPSHOT_CHANGED')


def run():
    parent = SERVICE.parent / 'private-backup'
    run_id = datetime.now(timezone.utc).strftime('%Y%m%dT%H%M%SZ') + '-go-core-rehearsal-' + uuid.uuid4().hex[:12]
    root = parent / run_id
    summary = {'status': 'BLOCKED_BEFORE_IMPORT', 'run_id': run_id, 'cases': []}
    cluster = None
    try:
        private_mkdir(root)
        write_new(root / 'directory-check.txt', b'Private local Go rehearsal.\n')
        with run_lock(parent / 'go-core-rehearsal.lock'):
            if shutil.disk_usage(root).free < 6 * 1024**3:
                raise Stop('CORE_REHEARSAL_DISK_SPACE')
            for prior in parent.glob('*-go-core-rehearsal-*'):
                if prior == root:
                    continue
                if (prior / 'postgres/postmaster.pid').exists():
                    raise Stop('CORE_PRIOR_CLUSTER_NEEDS_REVIEW')
                receipt = prior / 'summary.json'
                if receipt.is_file() and json.loads(receipt.read_text())['status'] == 'PASS_GO_CORE_LOCAL_REHEARSAL_ONLY':
                    raise Stop('CORE_REHEARSAL_ALREADY_ACCEPTED_REVIEW_RECEIPT')
            with socket.socket() as listener:
                listener.bind(('127.0.0.1', 0))
                port = listener.getsockname()[1]
            secrets_by_role = {role: secrets.token_hex(32) for role in ('admin', 'owner')}
            save(root / 'local-credentials.json', {'port': port, 'passwords': secrets_by_role})
            cluster = RehearsalCluster(root, PG_BIN, port, secrets_by_role)
            env = clean_environment(SERVICE, os.environ)
            env['GOTOOLCHAIN'] = 'local'
            summary['status'] = 'FAILED_LOCAL_REHEARSAL'
            try:
                exercise(root, cluster, env, summary)
            finally:
                if cluster.start_attempted:
                    summary['normal_stop_verified'] = cluster.stop()
            if tuple(summary['cases']) != CASES or not summary.get('normal_stop_verified'):
                raise Stop('CORE_REHEARSAL_CASES_INCOMPLETE')
            summary['run_bytes'] = bounded(root)
            summary['status'] = 'PASS_GO_CORE_LOCAL_REHEARSAL_ONLY'
            save(root / 'summary.json', summary)
    except (OSError, Stop, ValueError, KeyError, subprocess.TimeoutExpired, KeyboardInterrupt) as error:
        summary['reason'] = str(error) if isinstance(error, Stop) else type(error).__name__
        if summary['status'] == 'PASS_GO_CORE_LOCAL_REHEARSAL_ONLY':
            summary['status'] = 'FAILED_LOCAL_REHEARSAL'
        try:
            save(root / 'summary.json', summary)
        except OSError:
            pass
    print(json.dumps({key: summary[key] for key in
          ('status', 'run_id', 'cases', 'reason', 'normal_stop_verified', 'run_bytes') if key in summary}), flush=True)
    return 0 if summary['status'] == 'PASS_GO_CORE_LOCAL_REHEARSAL_ONLY' else 2


if __name__ == '__main__':
    raise SystemExit(run())
