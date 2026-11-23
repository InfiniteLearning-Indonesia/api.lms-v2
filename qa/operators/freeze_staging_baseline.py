"""Explicit owner-terminal export of the legacy application's public schema.

Never run by QA/CI. No restore, migration, remote DDL, R2, cleanup or Git calls.
Credentials remain in memory/child environment; stdout contains fixed status only.
The two plaintext backups remain in a newly created owner/SYSTEM-only directory.
"""
import argparse
from datetime import datetime, timezone
from hashlib import sha256
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import time
import uuid

from qa import storage
from qa.age_stream_run import acquire_tools
from qa.local_recovery_run import private_mkdir, write_new
from qa.run import run_lock

SERVICE = Path(__file__).resolve().parents[2]
PG_BIN = Path(r'C:\Program Files\PostgreSQL\18\bin')
MIB = 1024 ** 2
GIB = 1024 ** 3
MAX_DATABASE = 256 * MIB
MAX_FILE = 256 * MIB
MAX_RUN = 512 * MIB
FREE_FLOOR = 4 * GIB
SCOPE = 'LEGACY_APPLICATION_PUBLIC_SCHEMA_ONLY'


class Stop(Exception):
    """Only fixed, non-secret reason codes may cross the console boundary."""


def now():
    return datetime.now(timezone.utc).isoformat()


def read_config(path):
    if not path.is_file() or path.stat().st_size > MIB:
        raise Stop('CONFIG_UNAVAILABLE_OR_TOO_LARGE')
    if path.stat().st_nlink != 1 or any(storage.reparse(p) for p in (path, *path.parents)):
        raise Stop('LINKED_CONFIG_REJECTED')
    return parse_config(path.read_text(encoding='utf-8-sig'))


def parse_config(text):
    if len(text.encode('utf-8')) > MIB:
        raise Stop('CONFIG_UNAVAILABLE_OR_TOO_LARGE')
    if '\x00' in text:
        raise Stop('CONFIG_NUL_REJECTED')
    allowed = {'DB_HOST', 'DB_PORT', 'DB_USER', 'DB_USERNAME', 'DB_PASSWORD',
               'DB_NAME', 'DB_DATABASE', 'DB_SSL', 'DB_SSL_ROOT_CERT'}
    values = {}
    for line in text.splitlines():
        match = re.match(r'^\s*(?:export\s+)?([A-Za-z_][A-Za-z0-9_]*)\s*=(.*)$', line)
        if not match or match[1] not in allowed:
            continue
        key, value = match[1], match[2].strip()
        if key in values:
            raise Stop('DUPLICATE_DATABASE_CONFIG_KEY')
        if value.startswith(('"', "'")):
            if len(value) < 2 or value[-1] != value[0]:
                raise Stop('CONFIG_INVALID_QUOTING')
            value = value[1:-1]
        else:
            value = value.split(' #', 1)[0].rstrip()
        values[key] = value
    for primary, alias in [('DB_USER', 'DB_USERNAME'), ('DB_NAME', 'DB_DATABASE')]:
        if values.get(primary) and values.get(alias) and values[primary] != values[alias]:
            raise Stop('CONFLICTING_DATABASE_CONFIG_ALIASES')
        values[primary] = values.get(primary) or values.get(alias, '')
    if any(not values.get(k) for k in ['DB_HOST', 'DB_PORT', 'DB_USER', 'DB_PASSWORD', 'DB_NAME']):
        raise Stop('MISSING_DATABASE_CONFIG')
    if not values['DB_PORT'].isdigit() or not 1 <= int(values['DB_PORT']) <= 65535:
        raise Stop('INVALID_DATABASE_PORT')
    if any(c in values['DB_HOST'] for c in '/\\@, \t\r\n'):
        raise Stop('INVALID_DATABASE_HOST')
    if any(c in values['DB_USER'] + values['DB_NAME'] for c in '\r\n\x00'):
        raise Stop('INVALID_DATABASE_IDENTITY')
    if values.get('DB_SSL', 'true').lower() != 'true':
        raise Stop('TLS_REQUIRED')
    return values


def pg_environment(values, ca, inherited):
    # Replace libpq overrides, including PGHOSTADDR/PGSERVICE/PGOPTIONS. Keep
    # normal network/proxy settings unchanged. Never import other dotenv keys.
    env = {k: v for k, v in inherited.items() if not k.upper().startswith('PG')}
    env.update(PGHOST=values['DB_HOST'], PGPORT=values['DB_PORT'],
               PGUSER=values['DB_USER'], PGPASSWORD=values['DB_PASSWORD'],
               PGDATABASE=values['DB_NAME'], PGSSLMODE='verify-full',
               PGSSLROOTCERT=str(ca), PGCONNECT_TIMEOUT='10',
               PGSSLMINPROTOCOLVERSION='TLSv1.2', PGGSSENCMODE='disable',
               PGAPPNAME='lmsv3-owner-readonly-baseline',
               PGOPTIONS='-c default_transaction_read_only=on -c statement_timeout=60000 '
                         '-c lock_timeout=2000 -c idle_in_transaction_session_timeout=60000',
               PGCLIENTENCODING='UTF8', LC_ALL='C')
    return env


def custom_args(pg_bin):
    return [str(pg_bin / 'pg_dump.exe'), '--no-password', '--format=custom',
            '--schema=public', '--no-owner', '--no-privileges', '--no-tablespaces',
            '--lock-wait-timeout=2000']


PROBE_SQL = """
BEGIN TRANSACTION ISOLATION LEVEL REPEATABLE READ READ ONLY;
SELECT json_build_object(
 'server_version',current_setting('server_version_num')::integer,
 'read_only',current_setting('transaction_read_only')='on',
 'server_observed_tls',COALESCE((SELECT ssl FROM pg_catalog.pg_stat_ssl WHERE pid=pg_backend_pid()),false),
 'database_bytes',pg_catalog.pg_database_size(current_database()),
 'role_flags',(SELECT json_build_object('superuser',rolsuper,'bypass_rls',rolbypassrls,
     'create_database',rolcreatedb,'create_role',rolcreaterole)
     FROM pg_catalog.pg_roles WHERE rolname=current_user),
 'public_tables',COALESCE((SELECT json_agg(json_build_object('name',c.relname,'kind',c.relkind,
     'can_select',pg_catalog.has_table_privilege(c.oid,'SELECT')) ORDER BY c.relname)
     FROM pg_catalog.pg_class c JOIN pg_catalog.pg_namespace n ON n.oid=c.relnamespace
     WHERE n.nspname='public' AND c.relkind IN ('r','p')),'[]'::json),
 'cross_schema_fk_count',(SELECT count(*) FROM pg_catalog.pg_constraint k
     JOIN pg_catalog.pg_class c ON c.oid=k.conrelid
     JOIN pg_catalog.pg_namespace n ON n.oid=c.relnamespace
     JOIN pg_catalog.pg_class rc ON rc.oid=k.confrelid
     JOIN pg_catalog.pg_namespace rn ON rn.oid=rc.relnamespace
     WHERE k.contype='f' AND n.nspname='public' AND rn.nspname<>'public'),
 'extensions',COALESCE((SELECT json_agg(json_build_object('name',extname,'version',extversion)
     ORDER BY extname) FROM pg_catalog.pg_extension),'[]'::json));
ROLLBACK;
"""


def probe_args(pg_bin):
    # Both commands use ONE libpq connection. conninfo reads the client's
    # transport state, whereas pg_stat_ssl describes the backend-facing hop.
    return [str(pg_bin / 'psql.exe'), '-X', '-q', '-A', '-t', '-w',
            '-v', 'ON_ERROR_STOP=1', '--command', PROBE_SQL,
            '--command', r'\conninfo']


def parse_preflight_output(raw):
    try:
        text = raw.decode('utf-8').lstrip()
        observation, end = json.JSONDecoder().raw_decode(text)
    except (UnicodeError, ValueError):
        raise Stop('PREFLIGHT_OUTPUT_INVALID') from None
    if not isinstance(observation, dict):
        raise Stop('PREFLIGHT_OUTPUT_INVALID')
    # PostgreSQL 18 conninfo uses unaligned key|value rows with -A -t.
    # Host/user/database rows are never copied into reports or console output.
    fields = {}
    for line in text[end:].splitlines():
        name, separator, value = line.partition('|')
        if separator and name in {'SSL Connection', 'SSL Protocol'}:
            if name in fields:
                raise Stop('AMBIGUOUS_CLIENT_TLS_EVIDENCE')
            fields[name] = value.strip()
    if fields.get('SSL Connection') != 'true':
        raise Stop('CLIENT_TLS_NOT_VERIFIED')
    if fields.get('SSL Protocol') not in {'TLSv1.2', 'TLSv1.3'}:
        raise Stop('CLIENT_TLS_PROTOCOL_NOT_VERIFIED')
    observation.update(client_tls=True, client_tls_protocol=fields['SSL Protocol'],
                       client_tls_evidence='PSQL18_LIBPQ_CONNINFO_SAME_CONNECTION')
    return observation


def validate_observation(obs, free_bytes):
    if obs.get('client_tls') is not True:
        raise Stop('CLIENT_TLS_NOT_VERIFIED')
    if obs.get('read_only') is not True:
        raise Stop('READ_ONLY_TRANSACTION_NOT_VERIFIED')
    if not 140000 <= obs.get('server_version', 0) < 190000:
        raise Stop('UNSUPPORTED_SOURCE_POSTGRES_VERSION')
    size = obs.get('database_bytes', 0)
    if not isinstance(size, int) or not 0 < size <= MAX_DATABASE:
        raise Stop('SOURCE_SIZE_OUTSIDE_BOUNDED_EXPORT_SCOPE')
    if free_bytes < FREE_FLOOR + max(GIB, size * 16):
        raise Stop('INSUFFICIENT_DISK_FOR_BASELINE')
    tables = obs.get('public_tables', [])
    if not tables or any(not t.get('can_select') or t.get('kind') != 'r' for t in tables):
        raise Stop('TABLE_PERMISSION_OR_PARTITION_REQUIRES_REVIEW')
    if obs.get('cross_schema_fk_count') != 0:
        raise Stop('CROSS_SCHEMA_FOREIGN_KEY_REQUIRES_SCOPE_REVIEW')


def reason_from_stderr(raw, default):
    lower = raw.lower()
    if b'certificate' in lower or b'ssl' in lower or b'tls' in lower:
        return 'TLS_OR_CERTIFICATE_REJECTED'
    if b'password authentication failed' in lower:
        return 'DATABASE_AUTHENTICATION_FAILED'
    if b'permission denied' in lower:
        return 'DATABASE_PERMISSION_DENIED'
    return default


def captured(args, env, label, timeout=60):
    result = subprocess.run(args, cwd=SERVICE, env=env, capture_output=True, timeout=timeout)
    if result.returncode:
        raise Stop(reason_from_stderr(result.stderr, label + '_FAILED'))
    return result


def file_command(args, env, path, root, label, timeout=180):
    # All destinations are exclusively created inside a fresh private root.
    with path.open('xb') as out:
        child = subprocess.Popen(args, cwd=SERVICE, env=env, stdout=out, stderr=subprocess.PIPE)
        deadline = time.monotonic() + timeout
        try:
            while child.poll() is None:
                size = sum(p.stat().st_size for p in root.iterdir() if p.is_file())
                if path.stat().st_size > MAX_FILE or size > MAX_RUN:
                    raise Stop('BASELINE_GROWTH_LIMIT_EXCEEDED')
                if shutil.disk_usage(root).free < FREE_FLOOR:
                    raise Stop('BASELINE_DISK_FLOOR_REACHED')
                if time.monotonic() >= deadline:
                    raise Stop(label + '_TIMEOUT')
                time.sleep(0.2)
            _, error = child.communicate(timeout=5)
            if child.returncode:
                raise Stop(reason_from_stderr(error, label + '_FAILED'))
            if error.strip():
                raise Stop(label + '_WARNINGS_REQUIRE_REVIEW')
        finally:
            if child.poll() is None:
                child.kill()
                child.communicate(timeout=5)
        out.flush()
        os.fsync(out.fileno())
    if not 0 < path.stat().st_size <= MAX_FILE:
        raise Stop(label + '_SIZE_REJECTED')
    if sum(p.stat().st_size for p in root.iterdir() if p.is_file()) > MAX_RUN:
        raise Stop('BASELINE_GROWTH_LIMIT_EXCEEDED')
    if shutil.disk_usage(root).free < FREE_FLOOR:
        raise Stop('BASELINE_DISK_FLOOR_REACHED')


def verify_toc(toc, observation):
    tables = re.findall(r'\bTABLE DATA public (\S+) \S+\s*$', toc, re.MULTILINE)
    expected = [t['name'] for t in observation['public_tables']]
    if len(tables) != len(expected) or set(tables) != set(expected):
        raise Stop('ARCHIVE_TABLE_SCOPE_MISMATCH')
    return len(tables)


def run(pg_bin=PG_BIN, service=SERVICE):
    parent = service.parent / 'private-backup'
    parent.mkdir(exist_ok=True)
    run_id = datetime.now(timezone.utc).strftime('%Y%m%dT%H%M%SZ') + '-staging-baseline-' + uuid.uuid4().hex[:12]
    root, keys = parent / run_id, parent / (run_id + '-keys')
    result = {'run_id': run_id, 'status': 'BLOCKED_BEFORE_EXPORT', 'scope': SCOPE,
              'operator_sha256': storage.file_hash(Path(__file__)),
              'database_mutations': 'NONE', 'database_rows_exported': False,
              'source_connection_attempted': False, 'export_attempted': False,
              'snapshot_complete': False, 'partial_artifacts_possible': False,
              'actual_restore_verified': False, 'r2_access': 'NONE',
              'deletion': 'NONE', 'commit_push': 'NONE', 'independent_key_custody': False}
    try:
        # Deliberately before credentials, connections or key generation.
        private_mkdir(root)
        write_new(root / 'destination-check.txt', b'Private destination write check; no data.\n')
        private_mkdir(keys)
        write_new(keys / 'destination-check.txt', b'Private key destination write check.\n')
        with run_lock(parent / 'staging-baseline.lock'):
            print('[baseline] private destinations writable; checking cached tools', flush=True)
            def no_download(*args, **kwargs):
                raise Stop('TOOL_DOWNLOAD_NOT_ALLOWED')
            age, keygen, proof = acquire_tools(service, root, no_download, False)
            ca = service.parent / 'prod-ca-2021.crt'
            if not ca.is_file() or any(storage.reparse(p) for p in (ca, *ca.parents)):
                raise Stop('TRUSTED_CA_UNAVAILABLE')
            tool_versions = {}
            for name in ['psql.exe', 'pg_dump.exe', 'pg_restore.exe']:
                path = pg_bin / name
                if not path.is_file() or storage.reparse(path):
                    raise Stop('POSTGRES_TOOL_UNAVAILABLE')
                version = captured([str(path), '--version'], os.environ.copy(), 'PG_TOOL_VERSION')
                if not re.search(rb'\(PostgreSQL\) 18\.', version.stdout):
                    raise Stop('EXPECTED_POSTGRES18_CLIENT_REQUIRED')
                tool_versions[name] = version.stdout.decode('utf-8').strip()
            result['postgres_tool_versions'] = tool_versions
            values = read_config(service / '.env.staging')
            if values.get('DB_SSL_ROOT_CERT'):
                raise Stop('EXISTING_CA_SETTING_REQUIRES_REVIEW')
            env = pg_environment(values, ca, os.environ)
            print('[baseline] staging metadata only; verify-full TLS; read-only sessions', flush=True)
            result['source_connection_attempted'] = True
            observed = captured(probe_args(pg_bin), env, 'READONLY_PREFLIGHT')
            obs = parse_preflight_output(observed.stdout)
            result['preflight_transport'] = {k: obs.get(k) for k in
                ['read_only', 'client_tls', 'client_tls_protocol', 'server_observed_tls', 'client_tls_evidence']}
            # Preserve safe metadata even when a later gate blocks the export.
            write_new(root / 'preflight.json', json.dumps(obs, indent=2).encode())
            validate_observation(obs, shutil.disk_usage(root).free)
            fingerprint = sha256('\x00'.join(values[k] for k in ['DB_HOST', 'DB_PORT', 'DB_USER', 'DB_NAME']).encode()).hexdigest()
            result.update(source_identity_sha256=fingerprint, source_environment='USER_IDENTIFIED_STAGING',
                          source_server_version=obs['server_version'], source_database_bytes=obs['database_bytes'],
                          source_role_flags=obs['role_flags'], least_privilege_verified=False,
                          exclusions=['NON_PUBLIC_SCHEMAS', 'GLOBAL_ROLES_TABLESPACES', 'R2_OBJECT_BYTES'],
                          inspection_is_preflight_not_shared_snapshot=True)
            identity = keys / 'identity.txt'
            generated = captured([str(keygen)], os.environ.copy(), 'AGE_KEYGEN')
            write_new(identity, generated.stdout)
            recipient = captured([str(keygen), '-y', str(identity)], os.environ.copy(), 'AGE_RECIPIENT').stdout.decode().strip()
            if not re.fullmatch(r'age1[0-9a-z]{58}', recipient):
                raise Stop('AGE_RECIPIENT_INVALID')
            dump, sql = root / 'database-public.dump', root / 'database-public.sql'
            result['export_started_at'] = now()
            result.update(export_attempted=True, partial_artifacts_possible=True)
            print('[baseline] taking ONE public-schema custom snapshot; no remote writes', flush=True)
            file_command(custom_args(pg_bin), env, dump, root, 'PG_DUMP')
            result['database_rows_exported'] = True
            result['export_finished_at'] = now()
            toc = captured([str(pg_bin / 'pg_restore.exe'), '--list', str(dump)], os.environ.copy(), 'ARCHIVE_LIST')
            result['table_data_entries'] = verify_toc(toc.stdout.decode('utf-8'), obs)
            write_new(root / 'archive-list.txt', toc.stdout)
            print('[baseline] deriving SQL from that same archive; NOT executing SQL', flush=True)
            file_command([str(pg_bin / 'pg_restore.exe'), '--no-owner', '--no-privileges', '--file=-', str(dump)],
                         os.environ.copy(), sql, root, 'DERIVE_SQL')
            artifacts = []
            for plain in [dump, sql]:
                cipher = root / (plain.name + '.age')
                print('[baseline] encrypting and authenticating ' + plain.name, flush=True)
                file_command([str(age), '--encrypt', '--recipient', recipient, str(plain)],
                             os.environ.copy(), cipher, root, 'AGE_ENCRYPT')
                recovered = captured([str(age), '--decrypt', '--identity', str(identity), str(cipher)],
                                     os.environ.copy(), 'AGE_AUTHENTICATE', 120)
                if len(recovered.stdout) != plain.stat().st_size or sha256(recovered.stdout).hexdigest() != storage.file_hash(plain):
                    raise Stop('ENCRYPTED_ROUNDTRIP_MISMATCH')
                del recovered
                artifacts.append({'name': plain.name, 'bytes': plain.stat().st_size,
                                  'sha256': storage.file_hash(plain), 'cipher_name': cipher.name,
                                  'cipher_bytes': cipher.stat().st_size, 'cipher_sha256': storage.file_hash(cipher),
                                  'authenticated_roundtrip': True})
            result.update(status='PASS_PUBLIC_SNAPSHOT_EXPORT_ONLY', artifacts=artifacts,
                          snapshot_complete=True, partial_artifacts_possible=False,
                          age_version=proof['version'], age_binary_sha256=proof[age.name],
                          key_directory_name=keys.name, plaintext_retained_in_private_directory=True,
                          restore_status='NOT_RUN_REVIEW_DUMP_AND_EXTENSION_DEPENDENCIES_FIRST')
    except (OSError, Stop, ValueError, KeyError, subprocess.TimeoutExpired) as exc:
        if result['export_attempted']:
            result['status'] = 'FAILED_OR_INCOMPLETE_EXPORT'
        result['reason'] = str(exc) if isinstance(exc, Stop) else type(exc).__name__
    result['finished_at'] = now()
    try:
        write_new(root / 'summary.json', json.dumps(result, indent=2).encode())
        result['private_summary_written'] = True
    except OSError:
        result['private_summary_written'] = False
    print(json.dumps({'status': result['status'], 'reason': result.get('reason'), 'scope': SCOPE,
                      'report': str(root / 'summary.json'), 'key_directory': str(keys),
                      'database_rows_exported': result['database_rows_exported'],
                      'export_attempted': result['export_attempted'],
                      'partial_artifacts_possible': result['partial_artifacts_possible'],
                      'private_summary_written': result['private_summary_written'],
                      'actual_restore_verified': False, 'database_mutations': 'NONE'}), flush=True)
    return 0 if result['status'] == 'PASS_PUBLIC_SNAPSHOT_EXPORT_ONLY' else 2


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--staging-readonly', action='store_true', required=True)
    parser.add_argument('--pg-bin', type=Path, default=PG_BIN)
    args = parser.parse_args()
    if not args.staging_readonly:
        parser.error('Explicit staging read-only acknowledgement required')
    return run(pg_bin=args.pg_bin)


if __name__ == '__main__':
    raise SystemExit(main())
