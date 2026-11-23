"""Restore one reviewed real baseline into a NEW private local cluster only.

No dotenv/DSN/existing-server input, remote connections, R2, deletes or Git calls.
This is deliberately separate from synthetic QA and the Go migration runtime.
"""
import argparse
from datetime import datetime, timezone
from hashlib import sha256
import ipaddress
import json
import os
from pathlib import Path
import re
import secrets
import shutil
import socket
import subprocess
import time
import uuid

from qa import storage
from qa.age_stream_run import acquire_tools
from qa.local_recovery_run import private_mkdir, write_new
from qa.operators.freeze_staging_baseline import Stop
from qa.operators.windows_local_state import cluster_process_and_listener_state
from qa.run import run_lock

SERVICE = Path(__file__).resolve().parents[2]
PG_BIN = Path(r'C:\Program Files\PostgreSQL\18\bin')
SNAPSHOT = '20261118T131839Z-staging-baseline-3fe3bf70b4d4'
REVIEWED_DDL_SHA256 = 'a3f484735830590d8cd529817145ea33ebc3d6fc05628bab32cbf9fa9cd02f31'
PIN = {
    'database-public.dump': (528662, '60519c713393da6b92a01d6b793793fe47de6bc4871704278b5643eeaaa67058'),
    'database-public.sql': (2121156, 'fd35763e23e0d300b15288383076ba78e116d48fd7b14a609acba0ec453a4eca'),
    'database-public.dump.age': (528990, '8a4447e8af6b8aaefbad7f10d4c9b4a382d4107ab52f44c1b5d887413496b13e'),
    'database-public.sql.age': (2121868, 'aeb8ecb7b23fdfe28857d1237512afec1a9ce4721ba48f274c7db6a3ff2ea10e'),
}
DATABASES = {'postgres', 'legacy_frozen', 'empty_proof'}
TOOLS = ('initdb', 'pg_ctl', 'pg_controldata', 'createdb', 'pg_dump', 'pg_restore', 'psql')
CASES = ['encrypted-input-authentication', 'schema-and-copy-review', 'new-cluster-identity',
         'empty-target-backup-restore', 'minimal-extension-provisioning', 'atomic-custom-restore',
         'schema-constraints-parity', 'all-table-row-parity', 'reader-and-sequence-safety',
         'original-baseline-unchanged', 'normal-stop']
MAX_RUN = 1024 ** 3
FREE_FLOOR = 4 * 1024 ** 3


def identifier(value):
    if not re.fullmatch(r'[A-Za-z_][A-Za-z0-9_]*', value):
        raise Stop('UNREVIEWED_IDENTIFIER')
    return '"' + value + '"'


def fields(text):
    """Split generated SQL fields at top-level commas, preserving literals."""
    result, current, quote, depth, i = [], [], None, 0, 0
    while i < len(text):
        c = text[i]
        if quote:
            current.append(c)
            if c == quote:
                if i + 1 < len(text) and text[i + 1] == quote:
                    current.append(text[i + 1]); i += 1
                else:
                    quote = None
        elif c in "'\"":
            quote = c; current.append(c)
        elif c == '(':
            depth += 1; current.append(c)
        elif c == ')':
            depth -= 1; current.append(c)
            if depth < 0:
                raise Stop('UNBALANCED_SQL_FIELDS')
        elif c == ',' and depth == 0:
            result.append(''.join(current).strip()); current = []
        else:
            current.append(c)
        i += 1
    if quote or depth:
        raise Stop('UNBALANCED_SQL_FIELDS')
    result.append(''.join(current).strip())
    return result


def statements(raw):
    """Small fail-closed lexer for this pinned schema, not arbitrary SQL."""
    lines, directives = [], []
    for line in raw.decode('utf-8').splitlines():
        if line.startswith('\\'):
            match = re.fullmatch(r'\\(restrict|unrestrict) ([A-Za-z0-9]{1,128})', line)
            if not match:
                raise Stop('UNREVIEWED_PSQL_DIRECTIVE')
            directives.append(match.groups())
        else:
            lines.append(line)
    if (len(directives) != 2 or [d[0] for d in directives] != ['restrict', 'unrestrict']
            or directives[0][1] != directives[1][1]):
        raise Stop('INVALID_RESTRICT_DIRECTIVE_PAIR')
    text = '\n'.join(lines)
    output, buf, quote, i = [], [], None, 0
    while i < len(text):
        c = text[i]
        if quote:
            buf.append(c)
            if c == quote:
                if i + 1 < len(text) and text[i + 1] == quote:
                    buf.append(text[i + 1]); i += 1
                else:
                    quote = None
        elif text[i:i + 2] == '--':
            end = text.find('\n', i)
            i = len(text) if end < 0 else end
            if buf and buf[-1] != ' ':
                buf.append(' ')
            continue
        elif text[i:i + 2] == '/*' or c in '$\\':
            raise Stop('UNREVIEWED_SCHEMA_LEXEME')
        elif c in "'\"":
            quote = c; buf.append(c)
        elif c == ';':
            value = ''.join(buf).strip()
            if value:
                output.append(value)
            buf = []
        elif c.isspace():
            if buf and buf[-1] != ' ':
                buf.append(' ')
        else:
            buf.append(c)
        i += 1
    if quote or ''.join(buf).strip():
        raise Stop('INCOMPLETE_SCHEMA_SQL')
    return output


def review_schema(raw):
    commands = statements(raw)
    tables, enums, defaults, fk, constraints, indexes = {}, [], set(), 0, 0, 0
    for cmd in commands:
        table = re.fullmatch(r'CREATE TABLE public\.([a-z_]+) \((.*)\)', cmd)
        enum = re.fullmatch(r'CREATE TYPE public\.([a-z_]+) AS ENUM \((.*)\)', cmd)
        if table:
            name, body = table.groups()
            columns = []
            for column in fields(body):
                m = re.match(r'^("[A-Za-z_][A-Za-z0-9_]*"|[A-Za-z_][A-Za-z0-9_]*)\s+', column)
                if not m:
                    raise Stop('UNREVIEWED_COLUMN_DEFINITION')
                columns.append(m[1].strip('"'))
                if ' DEFAULT ' in column:
                    expr = column.split(' DEFAULT ', 1)[1]
                    defaults.update(re.findall(r'\b([A-Za-z_][A-Za-z0-9_.]*)\s*\(', expr))
            if name in tables:
                raise Stop('DUPLICATE_SCHEMA_TABLE')
            tables[name] = columns
        elif enum:
            enums.append(enum[1])
        elif re.match(r'^ALTER TABLE ONLY public\.[a-z_]+ ADD CONSTRAINT ', cmd):
            if ' FOREIGN KEY ' in cmd:
                if not re.search(r'REFERENCES public\.[a-z_]+\(', cmd):
                    raise Stop('UNREVIEWED_FOREIGN_KEY')
                fk += 1
            elif ' PRIMARY KEY ' not in cmd and ' UNIQUE ' not in cmd:
                raise Stop('UNREVIEWED_CONSTRAINT')
            constraints += 1
        elif re.fullmatch(r'CREATE INDEX "IDX_session_expire" ON public\.session USING btree \(expire\)', cmd):
            indexes += 1
        elif cmd in {'CREATE SCHEMA public', "COMMENT ON SCHEMA public IS 'standard public schema'",
                     "SELECT pg_catalog.set_config('search_path', '', false)"}:
            pass
        elif re.fullmatch(r"SET (statement_timeout|lock_timeout|idle_in_transaction_session_timeout|transaction_timeout) = 0", cmd):
            pass
        elif cmd in {"SET client_encoding = 'UTF8'", 'SET standard_conforming_strings = on',
                     'SET check_function_bodies = false', 'SET xmloption = content',
                     'SET client_min_messages = warning', 'SET row_security = off',
                     "SET default_tablespace = ''", 'SET default_table_access_method = heap'}:
            pass
        else:
            raise Stop('UNREVIEWED_SCHEMA_STATEMENT')
    if defaults - {'extensions.uuid_generate_v4', 'now', 'gen_random_uuid'}:
        raise Stop('UNREVIEWED_DEFAULT_FUNCTION')
    if len(tables) != 21 or len(enums) != 4 or fk != 27 or indexes != 1:
        raise Stop('SCHEMA_OUTSIDE_REVIEWED_BASELINE')
    ddl = sorted(c for c in commands if not c.startswith(('SET ', 'SELECT pg_catalog.set_config(')))
    return {'tables': tables, 'enum_count': len(enums), 'foreign_key_count': fk,
            'constraint_count': constraints, 'extra_index_count': indexes,
            'sequence_count': 0, 'required_extension': 'uuid-ossp',
            'ddl_sha256': sha256(json.dumps(ddl, ensure_ascii=True).encode()).hexdigest()}


def copy_ledger(raw):
    result, active, rows = {}, None, []
    for line in raw.splitlines():
        if active:
            if line == b'\\.':
                result[active]['rows'] = len(rows)
                result[active]['copy_multiset_sha256'] = rows_hash(rows)
                active, rows = None, []
            else:
                if len(line.split(b'\t')) != len(result[active]['columns']):
                    raise Stop('SOURCE_COPY_FIELD_COUNT_MISMATCH')
                rows.append(line)
            continue
        match = re.fullmatch(rb'COPY public\.([a-z_]+) \((.*)\) FROM stdin;', line)
        if match:
            name = match[1].decode('ascii')
            if name in result:
                raise Stop('DUPLICATE_COPY_BLOCK')
            columns = [c.strip('"') for c in fields(match[2].decode('ascii'))]
            for column in columns:
                identifier(column)
            result[name] = {'columns': columns}
            active = name
    if active:
        raise Stop('INCOMPLETE_COPY_BLOCK')
    return result


def rows_hash(rows):
    # Sorting preserves duplicates; values/rows never enter console or public docs.
    return sha256(b'\n'.join(sorted(rows))).hexdigest()


def filtered_toc(raw):
    lines, skipped = [], 0
    for line in raw.decode('utf-8').splitlines():
        if re.match(r'^\d+; \d+ \d+ (?:SCHEMA - public |COMMENT - SCHEMA public )', line):
            lines.append('; existing public schema metadata omitted: ' + line)
            skipped += 1
        else:
            lines.append(line)
    if skipped not in {0, 2}:
        raise Stop('PUBLIC_SCHEMA_TOC_PAIR_MISMATCH')
    return ('\n'.join(lines) + '\n').encode(), skipped


def local_environment(inherited, port, password, user, database):
    if database not in DATABASES or not 1 <= port <= 65535:
        raise Stop('LOCAL_TARGET_OUTSIDE_FIXED_SCOPE')
    env = {k: v for k, v in inherited.items() if not k.upper().startswith('PG')}
    env.update(PGHOST='127.0.0.1', PGPORT=str(port), PGUSER=user,
               PGPASSWORD=password, PGDATABASE=database, PGSSLMODE='disable',
               PGGSSENCMODE='disable', PGCONNECT_TIMEOUT='5',
               PGAPPNAME='lmsv3-new-local-baseline-restore', LC_ALL='C',
               PGOPTIONS='-c statement_timeout=30000 -c lock_timeout=2000',
               PGCLIENTENCODING='UTF8')
    return env


def restore_args(database, selection, dump):
    if database not in DATABASES - {'postgres'}:
        raise Stop('LOCAL_DATABASE_NOT_ALLOWED')
    # pg_restore requires --dbname explicitly; PGDATABASE alone selects SCRIPT
    # output mode, not a database restore. No --clean/--create is ever accepted.
    return ['--dbname=' + database, '--single-transaction', '--exit-on-error',
            '--no-owner', '--no-privileges', '--no-tablespaces',
            '--use-list=' + str(selection), dump]


def complete_cases(cases):
    return cases == CASES


def cluster_identity_checks(observed, directory, port):
    """Require the exact host address, including a host-sized inet mask.

    PostgreSQL's explicit inet::text cast retains /32 even for IPv4 hosts.
    Accept its host representation without accepting a wider subnet, another
    loopback address, an IPv6 connection, or a different target directory/port.
    """
    checks = {'directory': False, 'address': False, 'port': False, 'version': False}
    if not isinstance(observed, dict):
        return checks
    try:
        if isinstance(observed.get('directory'), str):
            checks['directory'] = Path(observed['directory']).resolve() == directory.resolve()
        if isinstance(observed.get('address'), str):
            interface = ipaddress.ip_interface(observed['address'])
            checks['address'] = (interface.version == 4 and interface.network.prefixlen == 32
                                 and str(interface.ip) == '127.0.0.1')
    except (ValueError, OSError):
        pass
    checks['port'] = type(observed.get('port')) is int and observed['port'] == port
    checks['version'] = type(observed.get('version')) is int and 180000 <= observed['version'] < 190000
    return checks


def control_process(args, env, cwd, timeout, output_path, error_path):
    """Daemon descendants must never inherit capture_output PIPE handles.

    On Windows a postgres descendant can keep pg_ctl's pipes open after pg_ctl
    exits. communicate(), including subprocess.run's timeout cleanup, then waits
    for EOF indefinitely. Regular files let us wait for the direct CLI only.
    """
    with output_path.open('xb+') as out, error_path.open('xb+') as error:
        result = subprocess.run(args, env=env, cwd=cwd, stdin=subprocess.DEVNULL,
                                stdout=out, stderr=error, timeout=timeout)
        out.flush(); error.flush()
        out.seek(0); error.seek(0)
        return subprocess.CompletedProcess(args, result.returncode, out.read(), error.read())


def prior_cluster_activity(parent, current):
    """Do not clone again while another interrupted local cluster listens.

    Inspect recorded PIDs and IPv4 listeners directly via read-only Windows APIs;
    never remove a PID file or signal a process based on an old PID.
    """
    observed = []
    for prior in sorted(parent.glob('*-legacy-local-restore-*')):
        if prior == current or not prior.is_dir() or storage.reparse(prior):
            continue
        pid_file = prior / 'postgres/postmaster.pid'
        if not pid_file.is_file():
            continue
        lines = pid_file.read_text(encoding='utf-8').splitlines()
        if len(lines) < 4 or not lines[0].isdigit() or not lines[3].isdigit() or not 1 <= int(lines[3]) <= 65535:
            raise Stop('PRIOR_LOCAL_CLUSTER_METADATA_REQUIRES_REVIEW')
        if Path(lines[1]).resolve() != (prior / 'postgres').resolve():
            raise Stop('PRIOR_LOCAL_CLUSTER_DIRECTORY_MISMATCH')
        state = cluster_process_and_listener_state(int(lines[0]), int(lines[3]))
        if state['listener_pids']:
            raise Stop('PRIOR_LOCAL_CLUSTER_STILL_LISTENING_STOP_OWNER_RUN_FIRST')
        if state['recorded_process_exists']:
            raise Stop('PRIOR_LOCAL_CLUSTER_PROCESS_STILL_EXISTS_STOP_OWNER_RUN_FIRST')
        observed.append({'run_id': prior.name, 'status': 'STALE_PID_NO_PROCESS_NO_LISTENER_PRESERVED',
                         'probe': state['probe'], 'clean_shutdown_verified': False})
    return observed


def validate_manifest(value):
    expected = {'version': 1, 'scope': 'ONE_PINNED_REAL_PUBLIC_BASELINE_NEW_LOCAL_CLUSTER_ONLY',
                'source_run': SNAPSHOT, 'tables': 21, 'rows': 7843, 'enums': 4,
                'constraints': 52, 'foreign_keys': 27, 'sequences': 0,
                'cases': CASES, 'source_connection': 'NONE', 'r2_access': 'NONE',
                'full_module_verified': False, 'migration_authority': False,
                'automatic_deletion': 'NONE'}
    if value != expected or value.get('full_module_verified') is not False or value.get('migration_authority') is not False:
        raise Stop('REAL_BASELINE_MANIFEST_MISMATCH')


def bounded(root):
    if shutil.disk_usage(root).free < FREE_FLOOR:
        raise Stop('LOCAL_RESTORE_DISK_FLOOR_REACHED')
    total = 0
    def unreadable(error):
        raise error
    for folder, dirs, names in os.walk(root, followlinks=False, onerror=unreadable):
        if any(storage.reparse(Path(folder) / name) for name in dirs):
            raise Stop('LINKED_RESTORE_OUTPUT')
        for name in names:
            p = Path(folder) / name
            if storage.reparse(p):
                raise Stop('LINKED_RESTORE_OUTPUT')
            total += p.stat().st_size
            if total > MAX_RUN:
                raise Stop('LOCAL_RESTORE_GROWTH_LIMIT')
    return total


class Cluster:
    def __init__(self, root, pg_bin, port, passwords):
        self.root, self.data, self.pg_bin = root, root / 'postgres', pg_bin
        self.port, self.passwords = port, passwords
        self.deadline = time.monotonic() + 15 * 60
        self.calls = []
        self.start_attempted = False
        self.command_sequence = 0
        self.identity_sequence = 0
        self.identity_reports = []

    def env(self, database='postgres', role='admin'):
        users = {'admin': 'lms_baseline_admin', 'owner': 'lms_baseline_owner', 'reader': 'lms_baseline_reader'}
        return local_environment(os.environ, self.port, self.passwords[role], users[role], database)

    def command(self, tool, args, database='postgres', role='admin', input_bytes=None, cleanup=False):
        remaining = self.deadline - time.monotonic()
        if remaining <= 0 and not cleanup:
            raise Stop('LOCAL_RESTORE_TIME_BUDGET_EXHAUSTED')
        self.command_sequence += 1
        command = [str(self.pg_bin / (tool + '.exe')), *map(str, args)]
        timeout = 40 if cleanup else min(120, max(1, remaining))
        if tool == 'pg_ctl':
            if input_bytes is not None:
                raise Stop('CONTROL_PROCESS_STDIN_NOT_ALLOWED')
            if args and str(args[-1]) in {'start', 'stop'}:
                print('[restore] pg_ctl ' + str(args[-1]) + '; bounded wait, private file output', flush=True)
            stem = self.root / ('pg-ctl-' + str(self.command_sequence).zfill(3))
            result = control_process(command, self.env(database, role), SERVICE, timeout,
                                     stem.with_suffix('.stdout'), stem.with_suffix('.stderr'))
        else:
            result = subprocess.run(command, env=self.env(database, role), cwd=SERVICE,
                                    input=input_bytes, capture_output=True, timeout=timeout)
        self.calls.append({'tool': tool, 'exit_code': result.returncode})
        if result.returncode:
            raise Stop('LOCAL_' + tool.upper() + '_FAILED')
        if tool == 'initdb' and b'initdb: error:' in result.stderr.lower():
            raise Stop('INITDB_FATAL_DESPITE_EXIT_ZERO')
        if len(result.stdout) > 64 * 1024 ** 2:
            raise Stop('LOCAL_COMMAND_OUTPUT_LIMIT')
        if not cleanup and shutil.disk_usage(self.root).free < FREE_FLOOR:
            raise Stop('LOCAL_RESTORE_DISK_FLOOR_REACHED')
        return result.stdout

    def sql(self, db, query, role='owner'):
        return self.command('psql', ['-X', '-q', '-A', '-t', '-w', '--file=-',
            '--single-transaction', '-v', 'ON_ERROR_STOP=1'], db, role, query.encode())

    def create_db(self, name):
        if name not in DATABASES - {'postgres'}:
            raise Stop('LOCAL_DATABASE_NOT_ALLOWED')
        self.command('createdb', ['--no-password', '--maintenance-db=postgres', '--template=template0',
                                '--encoding=UTF8', '--owner=lms_baseline_owner', name])

    def empty_state(self, database):
        value = json.loads(self.sql(database, "SELECT json_build_object('relations',"
            "(SELECT count(*) FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='public'),"
            "'types',(SELECT count(*) FROM pg_type t JOIN pg_namespace n ON n.oid=t.typnamespace WHERE n.nspname='public'),"
            "'constraints',(SELECT count(*) FROM pg_constraint k JOIN pg_namespace n ON n.oid=k.connamespace WHERE n.nspname='public'),"
            "'schema_comment',(SELECT obj_description(oid,'pg_namespace') FROM pg_namespace WHERE nspname='public'))"))
        if any(value[k] != 0 for k in ('relations', 'types', 'constraints')):
            raise Stop('PUBLIC_PRESTATE_NOT_EMPTY')
        return value

    def identity(self):
        raw = self.sql('postgres', "SELECT json_build_object('directory',current_setting('data_directory'),"
                       "'version',current_setting('server_version_num')::int,'address',inet_server_addr()::text,"
                       "'port',inet_server_port())", 'admin')
        v = json.loads(raw)
        checks = cluster_identity_checks(v, self.data, self.port)
        self.identity_sequence += 1
        name = 'identity-' + str(self.identity_sequence).zfill(3) + '.json'
        write_new(self.root / name, json.dumps({'checks': checks, 'observed': v,
            'expected': {'directory': str(self.data), 'address': '127.0.0.1/32',
                         'port': self.port, 'server_major': 18}}, indent=2).encode())
        self.identity_reports.append({'file': name, 'checks': checks})
        if not all(checks.values()):
            raise Stop('LOCAL_CLUSTER_IDENTITY_MISMATCH')

    def stop(self):
        if not self.start_attempted:
            return False
        if (self.data / 'postmaster.pid').exists():
            self.command('pg_ctl', ['-D', self.data, '-m', 'fast', '-w', '-t', '30', 'stop'], cleanup=True)
        control = self.command('pg_controldata', ['-D', self.data], cleanup=True)
        if (self.data / 'postmaster.pid').exists() or not re.search(rb'Database cluster state:\s+shut down\s*(?:\r?\n|$)', control):
            raise Stop('LOCAL_STOP_NOT_PROVEN')
        return True


def run(pg_bin=PG_BIN, service=SERVICE):
    parent = service.parent / 'private-backup'
    source = parent / SNAPSHOT
    accepted = parent / ('accepted-local-baseline-' + PIN['database-public.dump'][1] + '.json')
    run_id = datetime.now(timezone.utc).strftime('%Y%m%dT%H%M%SZ') + '-legacy-local-restore-' + uuid.uuid4().hex[:12]
    root = parent / run_id
    result = {'run_id': run_id, 'source_run': SNAPSHOT, 'status': 'BLOCKED_BEFORE_RESTORE',
              'operator_sha256': storage.file_hash(Path(__file__)), 'source_access': 'LOCAL_FILES_ONLY',
              'source_database_connection': 'NONE', 'r2_access': 'NONE', 'deletion': 'NONE',
              'actual_restore_verified': False, 'cluster_retained': False, 'cases': [],
              'full_module_verified': False, 'migration_authority': False}
    cluster = None
    lock = None
    lock_acquired = False
    try:
        validate_manifest(json.loads((service / 'qa/manifests/REAL-LEGACY-LOCAL-RESTORE.json').read_text(encoding='utf-8')))
        private_mkdir(root)
        write_new(root / 'destination-check.txt', b'Private real-baseline restore; new directory only.\n')
        lock = run_lock(parent / 'legacy-baseline-restore.lock')
        lock.__enter__()
        lock_acquired = True
        if accepted.exists():
            raise Stop('BASELINE_ALREADY_RESTORED_REVIEW_ACCEPTED_RECEIPT')
        prior = prior_cluster_activity(parent, root)
        write_new(root / 'prior-cluster-review.json', json.dumps(prior, indent=2).encode())
        if shutil.disk_usage(root).free < FREE_FLOOR + 2 * 1024 ** 3:
            raise Stop('INSUFFICIENT_LOCAL_RESTORE_SPACE')
        for sub in ('keys', 'recovered', 'prestate'):
            private_mkdir(root / sub)
        print('[restore] checking exact accepted snapshot; no source database connection', flush=True)
        for name, (size, digest) in PIN.items():
            path = source / name
            if (not path.is_file() or path.stat().st_size != size or path.stat().st_nlink != 1
                    or any(storage.reparse(p) for p in (path, *path.parents)) or storage.file_hash(path) != digest):
                raise Stop('PINNED_SNAPSHOT_MISMATCH')
        def no_download(*args, **kwargs):
            raise Stop('DOWNLOAD_NOT_ALLOWED')
        age, _, _ = acquire_tools(service, root, no_download, False)
        key = source.with_name(source.name + '-keys') / 'identity.txt'
        if not key.is_file() or key.stat().st_nlink != 1 or any(storage.reparse(p) for p in (key, *key.parents)):
            raise Stop('SNAPSHOT_KEY_UNAVAILABLE')
        for name in ('database-public.dump', 'database-public.sql'):
            recovered = subprocess.run([str(age), '--decrypt', '--identity', str(key), str(source / (name + '.age'))],
                                       env=os.environ.copy(), capture_output=True, timeout=30)
            if recovered.returncode or len(recovered.stdout) != PIN[name][0] or sha256(recovered.stdout).hexdigest() != PIN[name][1]:
                raise Stop('SNAPSHOT_AUTHENTICATION_MISMATCH')
            write_new(root / 'recovered' / name, recovered.stdout)
            del recovered
        result['cases'].append('encrypted-input-authentication')
        passwords = {role: secrets.token_hex(32) for role in ('admin', 'owner', 'reader')}
        write_new(root / 'keys/credentials.json', json.dumps(passwords).encode())
        with socket.socket() as sock:
            sock.bind(('127.0.0.1', 0)); port = sock.getsockname()[1]
        cluster = Cluster(root, pg_bin, port, passwords)
        for name in TOOLS:
            path = pg_bin / (name + '.exe')
            if not path.is_file() or any(storage.reparse(p) for p in (path, *path.parents)):
                raise Stop('LOCAL_PG_TOOL_UNAVAILABLE')
            version = cluster.command(name, ['--version'])
            if not re.search(rb'\(PostgreSQL\) 18\.', version):
                raise Stop('LOCAL_POSTGRES18_REQUIRED')
        dump = root / 'recovered/database-public.dump'
        sql = root / 'recovered/database-public.sql'
        schema = cluster.command('pg_restore', ['--schema-only', '--no-owner', '--no-privileges', '--file=-', dump])
        review = review_schema(schema)
        ledger = copy_ledger(sql.read_bytes())
        if (review['ddl_sha256'] != REVIEWED_DDL_SHA256 or review['constraint_count'] != 52
                or sum(table['rows'] for table in ledger.values()) != 7843):
            raise Stop('INPUT_DIFFERS_FROM_REVIEWED_SCHEMA_OR_ROW_SCOPE')
        if set(ledger) != set(review['tables']) or any(ledger[t]['columns'] != review['tables'][t] for t in ledger):
            raise Stop('SCHEMA_COPY_COLUMN_SCOPE_MISMATCH')
        write_new(root / 'schema-review.json', json.dumps(review, indent=2).encode())
        write_new(root / 'source-copy-ledger.json', json.dumps(ledger, indent=2).encode())
        toc = cluster.command('pg_restore', ['--list', dump])
        selected, skipped = filtered_toc(toc)
        if skipped != 2:
            raise Stop('REVIEWED_PUBLIC_SCHEMA_METADATA_MISSING')
        write_new(root / 'restore-toc.list', selected)
        result['cases'].append('schema-and-copy-review')
        print('[restore] creating NEW loopback cluster; no existing PostgreSQL service changes', flush=True)
        password_file = root / 'keys/bootstrap.pw'
        write_new(password_file, passwords['admin'].encode())
        cluster.command('initdb', ['-D', cluster.data, '--username=lms_baseline_admin', '--pwfile=' + str(password_file),
                                  '--auth-host=scram-sha-256', '--auth-local=scram-sha-256',
                                  '--encoding=UTF8', '--no-locale', '--no-clean'])
        if not all((cluster.data / f).is_file() for f in ['PG_VERSION', 'pg_hba.conf', 'postgresql.conf', 'global/pg_control']):
            raise Stop('LOCAL_CLUSTER_INCOMPLETE')
        with (cluster.data / 'postgresql.conf').open('a', encoding='utf-8') as handle:
            handle.write("\n# Dedicated frozen baseline; no provider/application services.\n"
                "listen_addresses='127.0.0.1'\nport=" + str(port) + "\nssl=off\nmax_connections=8\n"
                "shared_buffers='16MB'\nmax_wal_size='64MB'\nmin_wal_size='32MB'\n"
                "shared_preload_libraries=''\nlocal_preload_libraries=''\nsession_preload_libraries=''\n"
                "log_statement='none'\nlog_min_error_statement='panic'\n")
        result['cluster_retained'] = True
        cluster.start_attempted = True
        cluster.command('pg_ctl', ['-D', cluster.data, '-l', root / 'postgres.log', '-w', '-t', '20', 'start'])
        cluster.identity()
        for role in ('owner', 'reader'):
            cluster.sql('postgres', 'CREATE ROLE lms_baseline_' + role +
                        " LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOBYPASSRLS PASSWORD '" + passwords[role] + "'", 'admin')
        cluster.create_db('legacy_frozen'); cluster.create_db('empty_proof')
        result['cases'].append('new-cluster-identity')
        print('[restore] backing up EMPTY target and proving its restore before loading real data', flush=True)
        empty_state = cluster.empty_state('legacy_frozen')
        empty = root / 'prestate/empty.dump'
        write_new(empty, cluster.command('pg_dump', ['--format=custom', '--schema=public', '--no-owner', '--no-privileges', '--no-password'], 'legacy_frozen', 'owner'))
        write_new(root / 'prestate/empty.sql', cluster.command('pg_restore', ['--no-owner', '--no-privileges', '--file=-', empty]))
        empty_toc, _ = filtered_toc(cluster.command('pg_restore', ['--list', empty]))
        write_new(root / 'prestate/empty-toc.list', empty_toc)
        cluster.command('pg_restore', restore_args('empty_proof', root / 'prestate/empty-toc.list', empty), 'empty_proof', 'owner')
        if cluster.empty_state('empty_proof') != empty_state or cluster.empty_state('legacy_frozen') != empty_state:
            raise Stop('EMPTY_PRESTATE_RESTORE_MISMATCH')
        write_new(root / 'prestate/receipt.json', json.dumps({'status': 'PASS_EMPTY_PUBLIC_SCOPE_RESTORE',
            'dump_sha256': storage.file_hash(empty), 'sql_sha256': storage.file_hash(root / 'prestate/empty.sql'),
            'catalog': empty_state,
            'scope': 'NEW_EMPTY_PUBLIC_SCHEMA_NOT_GLOBAL_ROLES'}).encode())
        result['cases'].append('empty-target-backup-restore')
        if not (pg_bin.parent / 'share/extension/uuid-ossp.control').is_file():
            raise Stop('UUID_EXTENSION_NOT_INSTALLED_LOCALLY')
        print('[restore] provisioning local uuid-ossp dependency and restricted reader', flush=True)
        cluster.sql('legacy_frozen', "CREATE SCHEMA extensions; CREATE EXTENSION \"uuid-ossp\" WITH SCHEMA extensions;"
            " GRANT USAGE ON SCHEMA public,extensions TO lms_baseline_owner,lms_baseline_reader;"
            " ALTER DEFAULT PRIVILEGES FOR ROLE lms_baseline_owner IN SCHEMA public GRANT SELECT ON TABLES TO lms_baseline_reader;", 'admin')
        # Default privileges must be set FOR the restoring owner, not the bootstrap admin.
        result['cases'].append('minimal-extension-provisioning')
        cluster.identity()
        bounded(root)
        print('[restore] restoring reviewed custom archive atomically as restricted owner', flush=True)
        cluster.command('pg_restore', restore_args('legacy_frozen', root / 'restore-toc.list', dump), 'legacy_frozen', 'owner')
        result['cases'].append('atomic-custom-restore')
        target_schema = cluster.command('pg_dump', ['--schema-only', '--schema=public', '--no-owner', '--no-privileges', '--no-password'], 'legacy_frozen', 'owner')
        target_review = review_schema(target_schema)
        if target_review != review:
            write_new(root / 'restored-schema-review.json', json.dumps(target_review, indent=2).encode())
            raise Stop('RESTORED_SCHEMA_SIGNATURE_MISMATCH')
        result['cases'].append('schema-constraints-parity')
        print('[restore] reconciling all 21 COPY row multisets; no record contents printed', flush=True)
        parity = []
        for table, source_table in ledger.items():
            columns = ','.join(identifier(c) for c in source_table['columns'])
            query = ("SET LOCAL TIME ZONE 'UTC'; SET LOCAL DateStyle='ISO'; SET LOCAL IntervalStyle='postgres';"
                     " SET LOCAL extra_float_digits=3; SET LOCAL bytea_output='hex';"
                     ' COPY (SELECT ' + columns + ' FROM public.' + identifier(table) + ') TO STDOUT WITH (FORMAT text);')
            output = cluster.sql('legacy_frozen', query, 'reader')
            rows = output.splitlines()
            if len(rows) != source_table['rows'] or rows_hash(rows) != source_table['copy_multiset_sha256']:
                raise Stop('RESTORED_ROW_MULTISET_MISMATCH')
            parity.append({'table': table, 'rows': len(rows), 'copy_multiset_sha256': rows_hash(rows)})
        write_new(root / 'row-parity.json', json.dumps(parity, indent=2).encode())
        result.update(table_count=len(parity), total_rows=sum(t['rows'] for t in parity))
        result['cases'].append('all-table-row-parity')
        safety = json.loads(cluster.sql('legacy_frozen', "SELECT json_build_object('sequences',"
            "(SELECT count(*) FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='public' AND c.relkind='S'),"
            "'unvalidated_fk',(SELECT count(*) FROM pg_constraint k JOIN pg_namespace n ON n.oid=k.connamespace WHERE n.nspname='public' AND k.contype='f' AND NOT k.convalidated),"
            "'superuser',(SELECT rolsuper FROM pg_roles WHERE rolname=current_user),"
            "'bypass',(SELECT rolbypassrls FROM pg_roles WHERE rolname=current_user),"
            "'create_database',(SELECT rolcreatedb FROM pg_roles WHERE rolname=current_user),"
            "'create_role',(SELECT rolcreaterole FROM pg_roles WHERE rolname=current_user),"
            "'can_write',(SELECT bool_or(has_table_privilege(c.oid,'INSERT,UPDATE,DELETE,TRUNCATE')) FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='public' AND c.relkind='r'))", 'reader'))
        if safety != {'sequences': 0, 'unvalidated_fk': 0, 'superuser': False, 'bypass': False,
                      'create_database': False, 'create_role': False, 'can_write': False}:
            raise Stop('READER_OR_CONSTRAINT_SAFETY_MISMATCH')
        write_new(root / 'reader-safety.json', json.dumps(safety).encode())
        cluster.sql('postgres', 'ALTER ROLE lms_baseline_reader SET default_transaction_read_only=on', 'admin')
        if cluster.sql('legacy_frozen', "SELECT current_setting('transaction_read_only')", 'reader').strip() != b'on':
            raise Stop('READER_READONLY_NOT_VERIFIED')
        result['cases'].append('reader-and-sequence-safety')
        write_new(root / 'keys/local-reader.json', json.dumps({'host': '127.0.0.1', 'port': port,
            'database': 'legacy_frozen', 'user': 'lms_baseline_reader', 'password': passwords['reader'],
            'scope': 'THIS_NEW_LOCAL_CLUSTER_ONLY', 'server_stopped_after_run': True}).encode())
        for name, (_, digest) in PIN.items():
            if storage.file_hash(source / name) != digest:
                raise Stop('ORIGINAL_BASELINE_CHANGED')
        result['cases'].append('original-baseline-unchanged')
    except (Exception, KeyboardInterrupt) as exc:
        if cluster and cluster.start_attempted:
            result['status'] = 'FAILED_OR_INCOMPLETE_LOCAL_RESTORE'
        result['reason'] = str(exc) if isinstance(exc, Stop) else type(exc).__name__
    finally:
        if cluster and cluster.start_attempted:
            try:
                cluster.stop(); result['cases'].append('normal-stop')
            except (Exception, KeyboardInterrupt):
                result['stop_failure'] = 'LOCAL_STOP_REQUIRES_OPERATOR'
                result.setdefault('reason', 'LOCAL_STOP_REQUIRES_OPERATOR')
        try:
            result['run_bytes'] = bounded(root)
        except (OSError, Stop):
            result.setdefault('reason', 'LOCAL_STORAGE_OR_PERMISSION_REVIEW_REQUIRED')
        if complete_cases(result['cases']) and not result.get('reason'):
            result.update(status='PASS_REVIEWED_LOCAL_BASELINE_RESTORE_ONLY', actual_restore_verified=True)
        result['commands'] = cluster.calls if cluster else []
        result['identity_checks'] = cluster.identity_reports if cluster else []
        result['finished_at'] = datetime.now(timezone.utc).isoformat()
        try:
            write_new(root / 'summary.json', json.dumps(result, indent=2).encode())
            if result['actual_restore_verified']:
                write_new(accepted, json.dumps({'run_id': run_id, 'report': str(root / 'summary.json'),
                    'source_sha256': PIN['database-public.dump'][1], 'status': result['status']}).encode())
        except OSError:
            result['private_summary_written'] = False
            if result['actual_restore_verified']:
                result.update(status='RESTORED_EVIDENCE_WRITE_FAILED', actual_restore_verified=False,
                              reason='LOCAL_RECEIPT_WRITE_FAILED')
        if lock_acquired:
            lock.__exit__(None, None, None)
    print(json.dumps({'status': result['status'], 'reason': result.get('reason'), 'run_id': run_id,
        'report': str(root / 'summary.json'), 'actual_restore_verified': result['actual_restore_verified'],
        'cluster_retained': result['cluster_retained'], 'table_count': result.get('table_count'),
        'total_rows': result.get('total_rows'), 'source_database_connection': 'NONE', 'r2_access': 'NONE',
        'completed_cases': len(result['cases']), 'expected_cases': len(CASES),
        'stop_failure': result.get('stop_failure'), 'private_summary_written': result.get('private_summary_written', True),
        'deletion': 'NONE', 'full_module_verified': False}), flush=True)
    return 0 if result['actual_restore_verified'] else 2


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--new-local-restore', action='store_true', required=True)
    parser.add_argument('--pg-bin', type=Path, default=PG_BIN)
    args = parser.parse_args()
    if not args.new_local_restore:
        parser.error('New local cluster acknowledgement required')
    return run(pg_bin=args.pg_bin)


if __name__ == '__main__':
    raise SystemExit(main())
