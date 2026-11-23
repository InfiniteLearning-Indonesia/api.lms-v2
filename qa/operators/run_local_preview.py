"""Start the actual Go LMS runtime with an isolated synthetic PG18 database."""

import argparse
from datetime import datetime, timezone
import hashlib
import json
import os
from pathlib import Path
import secrets
import shutil
import socket
import ssl
import subprocess
import sys
import time
import urllib.error
import urllib.request
import uuid

from qa.evidence import source_fingerprint
from qa.operators.rehearse_m04_password import PasswordFixtureCluster, prepare_empty_target
from qa.operators.restore_legacy_baseline import PG_BIN, SERVICE, Stop, bounded, private_mkdir, write_new
from qa.run import clean_environment, run_lock

ADMIN_ID = '00000000000000000000000000000001'
ADMIN_EMAIL = 'admin@preview.invalid'


class PreviewCluster(PasswordFixtureCluster):
    target = 'preview_qa'


def free_port():
    with socket.socket() as probe:
        probe.bind(('127.0.0.1', 0))
        return probe.getsockname()[1]


def checked(command, env, root, label, timeout):
    result = subprocess.run(command, cwd=SERVICE, env=env, capture_output=True, timeout=timeout)
    if result.returncode:
        write_new(root / (label + '.stderr'), result.stderr)
        raise Stop(label.upper().replace('-', '_') + '_FAILED')
    return result.stdout


def build(root):
    env = clean_environment(SERVICE, os.environ)
    env.update(GOTOOLCHAIN='local', LMS_MODE='v3-local-preview', QA_LAB_DIR=str(root),
               QA_PG_ACK='new-local-synthetic-cluster')
    for cmd, executable in [('api', 'api'), ('preview-fixture', 'fixture'), ('tls-fixture', 'tls')]:
        checked(['go', 'build', '-buildvcs=false', '-o', str(root / (executable + '.exe')), './cmd/' + cmd],
                env, root, 'build-' + executable, 180)
    checked([str(root / 'tls.exe')], env, root, 'generate-tls', 15)
    return env


def install_fixture(cluster, schema, marker, writer, admin_password):
    salt = secrets.token_bytes(16)
    hashed = hashlib.pbkdf2_hmac('sha256', admin_password.encode(), salt, 600000, dklen=32)
    # Only fixed SQL and freshly generated hex values are interpolated here.
    cluster.sql_admin(cluster.target, f"""CREATE SCHEMA preview_guard;
CREATE TABLE preview_guard.target(marker text PRIMARY KEY);
INSERT INTO preview_guard.target VALUES ('{marker}');
CREATE ROLE preview_runtime LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS PASSWORD '{writer}';
{schema}
INSERT INTO v3.users(id,active,site_admin) VALUES('{ADMIN_ID}',true,false);
INSERT INTO v3identity.links(user_id,issuer,subject,version,active,valid_until)
 VALUES('{ADMIN_ID}','urn:lms-v3:password','{ADMIN_ID}',1,true,'infinity');
INSERT INTO v3identity.site_roles(user_id,admin) VALUES('{ADMIN_ID}',true);
INSERT INTO v3password.accounts(user_id,email,activated_at) VALUES('{ADMIN_ID}','{ADMIN_EMAIL}',clock_timestamp());
INSERT INTO v3password.credentials(user_id,salt,password_hash) VALUES('{ADMIN_ID}',decode('{salt.hex()}','hex'),decode('{hashed.hex()}','hex'));
""")
    return cluster.sql_admin(cluster.target, 'SELECT (pg_control_system()).system_identifier::text').decode().strip()


def wait_ready(process, origin, root):
    ctx = ssl.create_default_context(cafile=str(root / 'runtime-tls.crt'))
    deadline = time.monotonic() + 18
    while process.poll() is None and time.monotonic() < deadline:
        try:
            with urllib.request.urlopen(origin + '/health/ready', context=ctx, timeout=2) as response:
                if response.status == 200 and json.load(response) == {'status': 'ready'}:
                    return
        except (OSError, urllib.error.URLError, ValueError):
            time.sleep(0.3)
    raise Stop('PREVIEW_API_NOT_READY')


def stop_api(process):
    if process.stdin and not process.stdin.closed:
        process.stdin.close()
    try:
        return process.wait(timeout=12) == 0
    except subprocess.TimeoutExpired:
        process.terminate()
        process.wait(timeout=10)
        return False


def run(args):
    parent = SERVICE.parent / 'private-backup'
    run_id = datetime.now(timezone.utc).strftime('%Y%m%dT%H%M%SZ') + '-local-preview-' + uuid.uuid4().hex[:12]
    root = parent / run_id
    receipt = {'status': 'BLOCKED_BEFORE_LOCAL_PREVIEW', 'run_id': run_id, 'cases': [], 'phase': 'private_directory'}
    try:
        private_mkdir(root)
        write_new(root / 'directory-check.txt', b'Synthetic preview fixture.\n')
        with run_lock(parent / 'local-preview.lock'):
            if shutil.disk_usage(root).free < 6 * 1024**3:
                raise Stop('PREVIEW_DISK_FLOOR')
            if any((p / 'postgres/postmaster.pid').exists() for p in parent.glob('*-local-preview-*')):
                raise Stop('PREVIEW_PRIOR_CLUSTER_NEEDS_REVIEW')
            receipt['status'] = 'FAILED_LOCAL_PREVIEW'
            receipt['source_hash'] = source_fingerprint(SERVICE)
            cluster = PreviewCluster(root, PG_BIN, free_port(), {'admin': secrets.token_hex(32)})
            process = None
            try:
                receipt['phase'] = 'build_tools'
                print('[preview] building cmd/api and local tools', flush=True)
                env = build(root)
                schema = checked([str(root / 'fixture.exe'), 'schema'], env, root, 'fixture-schema', 15).decode()
                receipt['phase'] = 'backup_and_restore_empty_target'
                prepare_empty_target(root, cluster, receipt)
                marker, writer, admin_password = secrets.token_hex(32), secrets.token_hex(32), secrets.token_urlsafe(24)
                receipt['phase'] = 'install_schema'
                print('[preview] installing the synthetic schema and restricted role', flush=True)
                system_id = install_fixture(cluster, schema, marker, writer, admin_password)
                port = args.port or free_port()
                origin = f'https://127.0.0.1:{port}'
                env.update(LMS_HTTP_ADDR=f'127.0.0.1:{port}', LMS_ORIGIN=origin, LMS_PREVIEW_ROOT=str(root),
                           LMS_TLS_CERT_FILE=str(root / 'runtime-tls.crt'), LMS_TLS_KEY_FILE=str(root / 'runtime-tls.key'),
                           LMS_PREVIEW_DB_PORT=str(cluster.port), LMS_PREVIEW_DB_PASSWORD=writer,
                           LMS_PREVIEW_DB_SYSTEM_ID=system_id, LMS_PREVIEW_MARKER=marker,
                           LMS_PREVIEW_DELIVERY_KEY=secrets.token_hex(32), LMS_PREVIEW_ATTACHED='1')
                if args.google:
                    for k in ('LMS_PREVIEW_GOOGLE_CLIENT_ID', 'LMS_PREVIEW_GOOGLE_CLIENT_SECRET'):
                        if not os.environ.get(k):
                            raise Stop('PREVIEW_GOOGLE_CONFIGURATION_REQUIRED')
                        env[k] = os.environ[k]
                    env['LMS_PREVIEW_GOOGLE_FLOW_KEY'] = secrets.token_hex(32)
                if args.qa:
                    web = root / 'web-fixture'
                    web.mkdir()
                    write_new(web / 'index.html', b'<!doctype html><title>LMS preview fixture</title>')
                else:
                    web = SERVICE.parents[1] / 'fe-test' / 'dist'
                if (web / 'index.html').is_file():
                    env['LMS_PREVIEW_WEB_ROOT'] = str(web)
                receipt['phase'] = 'api_readiness'
                with (root / 'api.log').open('xb') as log:
                    process = subprocess.Popen([str(root / 'api.exe')], cwd=SERVICE, env=env, stdin=subprocess.PIPE,
                                               stdout=log, stderr=subprocess.STDOUT, creationflags=subprocess.CREATE_NO_WINDOW)
                    wait_ready(process, origin, root)
                    def mailbox():
                        return json.loads(checked([str(root / 'fixture.exe'), 'mailbox'], env, root, 'mailbox', 10))
                    print(f'[preview] ready {origin}', flush=True)
                    if args.qa:
                        receipt['phase'] = 'playwright_api'
                        from playwright.sync_api import Error as PlaywrightError
                        from qa.operators.probe_local_preview import smoke
                        try:
                            receipt['cases'] = smoke(origin, admin_password, mailbox)
                        except PlaywrightError as error:
                            raise Stop('PREVIEW_PLAYWRIGHT_FAILED') from error
                        if len(receipt['cases']) != 7:
                            raise Stop('PREVIEW_EXPECTED_CASE_COUNT_MISMATCH')
                    else:
                        receipt['phase'] = 'manual_session'
                        credentials = root / 'preview-admin.private.json'
                        write_new(credentials, json.dumps({'origin': origin, 'email': ADMIN_EMAIL, 'password': admin_password}, indent=2).encode())
                        print(f'[preview] synthetic admin credentials in {credentials}', flush=True)
                        print('[preview] Type qa to watch the browser journeys, qa headless to run in the background, mail for the private mailbox, or stop.', flush=True)
                        while process.poll() is None:
                            try:
                                action = input('preview> ').strip()
                            except EOFError:
                                break
                            if action == 'stop':
                                break
                            if action in ('qa', 'qa headless'):
                                from qa.operators.probe_preview_journeys import run_browser_journeys
                                try:
                                    wait_ready(process, origin, root)
                                    run_browser_journeys(origin, mailbox, ADMIN_EMAIL, admin_password, root, web, headless=action == 'qa headless')
                                except (OSError, Stop, RuntimeError) as error:
                                    reason = str(error) if isinstance(error, Stop) else type(error).__name__
                                    print(json.dumps({'status': 'BLOCKED_FE_TEST_JOURNEYS', 'reason': reason}), flush=True)
                            if action == 'mail':
                                destination = root / ('mailbox-' + uuid.uuid4().hex[:12] + '.private.json')
                                write_new(destination, json.dumps(mailbox(), indent=2).encode())
                                print(f'[preview] private mailbox {destination}', flush=True)
            finally:
                try:
                    if process is not None:
                        receipt['api_shutdown_verified'] = stop_api(process)
                finally:
                    if cluster.start_attempted:
                        receipt['normal_stop_verified'] = cluster.stop()
            if not receipt.get('api_shutdown_verified') or not receipt.get('normal_stop_verified'):
                raise Stop('PREVIEW_SHUTDOWN_NOT_VERIFIED')
            if args.qa and receipt['source_hash'] != source_fingerprint(SERVICE):
                raise Stop('PREVIEW_SOURCE_CHANGED_DURING_QA')
            receipt['run_bytes'] = bounded(root)
            receipt['phase'] = 'completed'
            receipt['status'] = 'PASS_LOCAL_PREVIEW_SYNTHETIC_ONLY' if args.qa else 'LOCAL_PREVIEW_STOPPED'
    except (OSError, Stop, ValueError, KeyError, subprocess.TimeoutExpired, KeyboardInterrupt) as error:
        receipt['reason'] = str(error) if isinstance(error, Stop) else type(error).__name__
    try:
        write_new(root / 'summary.json', json.dumps(receipt, indent=2).encode())
    except OSError:
        if receipt['status'] in {'PASS_LOCAL_PREVIEW_SYNTHETIC_ONLY', 'LOCAL_PREVIEW_STOPPED'}:
            receipt.update(status='FAILED_LOCAL_PREVIEW', reason='RECEIPT_WRITE_FAILED')
    print(json.dumps(receipt), flush=True)
    return 0 if receipt['status'] in {'PASS_LOCAL_PREVIEW_SYNTHETIC_ONLY', 'LOCAL_PREVIEW_STOPPED'} else 2


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--qa', action='store_true')
    parser.add_argument('--port', type=int)
    parser.add_argument('--google', action='store_true')
    args = parser.parse_args()
    if args.port is not None and not 1024 <= args.port <= 65535:
        parser.error('port outside loopback preview range')
    if args.google and (args.qa or args.port is None):
        parser.error('Google preview requires an explicit port and manual mode')
    if not args.qa and args.port is None:
        args.port = 9443
    return run(args)


if __name__ == '__main__':
    sys.exit(main())
