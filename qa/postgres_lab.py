"""Opt-in fresh loopback PostgreSQL lab. Keeps rows, backups and evidence.

NEVER accepts a DSN, env file, existing cluster or shared database. Requires an
explicit creation acknowledgement. No DROP/DELETE/TRUNCATE/restore --clean.
"""
import argparse
from datetime import datetime, timezone
from hashlib import sha256
import json
import os
from pathlib import Path
import secrets
import socket
import subprocess
import uuid
import sys
import threading
import time
from qa.evidence import source_fingerprint, write_new_json, verify_go_cases
from qa.run import ROOT, clean_environment, run_lock
from qa import storage, retention_automation

INITDB_SAFETY_OPTIONS = ("--auth-host=scram-sha-256", "--auth-local=scram-sha-256", "--encoding=UTF8", "--no-locale", "--no-clean")

# Synthetic schema only: definitions as well as names must survive restore.
CATALOG_QUERIES = (
    "SELECT n.nspname,c.relname,c.relkind FROM pg_catalog.pg_class c JOIN pg_catalog.pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname IN ('public','qa_cp05') ORDER BY 1,2,3",
    "SELECT table_schema,table_name,column_name,ordinal_position,data_type,is_nullable,column_default FROM information_schema.columns WHERE table_schema IN ('public','qa_cp05') ORDER BY 1,2,4",
    "SELECT n.nspname,c.relname,k.conname,pg_catalog.pg_get_constraintdef(k.oid) FROM pg_catalog.pg_constraint k JOIN pg_catalog.pg_class c ON c.oid=k.conrelid JOIN pg_catalog.pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname IN ('public','qa_cp05') ORDER BY 1,2,3",
)


def require_outcome(code, expect_failure=False):
    if (code != 0) != expect_failure:
        raise ValueError("synthetic command outcome mismatch; see retained evidence")


def require_digest(archive, expected):
    if archive.is_symlink() or not archive.is_file() or sha256(archive.read_bytes()).hexdigest() != expected:
        raise ValueError("synthetic archive integrity gate rejected")


def require_coverage(profile):
    """A successful covdata exit alone does not prove any data was captured."""
    lines = profile.read_text(encoding="utf-8").splitlines()
    required = {"lmsv3/internal/classes/", "lmsv3/internal/identity/", "lmsv3/internal/platform/database/"}
    observed = set()
    executed = 0
    for line in lines[1:]:
        location, statements, count = line.rsplit(" ", 2)
        if int(statements) < 0 or int(count) < 0:
            raise ValueError("invalid coverage counter")
        executed += int(statements) if int(count) else 0
        observed.update(prefix for prefix in required if location.startswith(prefix))
    if not lines or lines[0] not in ("mode: set", "mode: count", "mode: atomic") or observed != required or not executed:
        raise ValueError("API coverage missing, empty or incomplete")


def run_to_files(args, directory, counter, env, timeout):
    """File handles avoid Windows communicate() hanging on inherited pipes.

    A pg_ctl child may retain the handles after pg_ctl exits. Never wait for
    pipe EOF from the database server. All raw output is private synthetic QA.
    """
    out = directory / f"command-{counter:02}.stdout"
    err = directory / f"command-{counter:02}.stderr"
    with out.open("xb") as stdout, err.open("xb") as stderr:
        proc = subprocess.run([str(a) for a in args], cwd=ROOT, env=env,
                              stdout=stdout, stderr=stderr, timeout=timeout)
    return proc.returncode, out.read_bytes(), err.read_bytes()


def fixture_sql(marker):
    if len(marker) != 64 or any(c not in "abcdef0123456789" for c in marker):
        raise ValueError("invalid synthetic marker")
    return ("CREATE SCHEMA qa_cp05; CREATE TABLE qa_cp05.marker(value text NOT NULL); "
            f"INSERT INTO qa_cp05.marker VALUES ('{marker}'); " +
            " ".join(f"CREATE TABLE qa_cp05.{table}(id text PRIMARY KEY); INSERT INTO qa_cp05.{table} VALUES ('seed');" for table in ("domain", "audit", "outbox")) +
            " CREATE ROLE qa_reader NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOBYPASSRLS;"
            " GRANT USAGE ON SCHEMA qa_cp05 TO qa_reader; GRANT SELECT ON qa_cp05.marker TO qa_reader;")


def validate_tools(directory):
    directory = directory.resolve(strict=True)
    ext = ".exe" if os.name == "nt" else ""
    return {name: directory / (name + ext) for name in ("initdb", "pg_ctl", "pg_dump", "pg_restore", "psql", "createdb")
            if (directory / (name + ext)).is_file()}


@retention_automation.lab_lifecycle
def run_lab(pg_bin, class_journey=False, identity_journey=False, runtime_journey=False,
            worker_journey=False, participation_journey=False, file_journey=False,
            content_journey=False, content_safety_journey=False, content_contract_journey=False,
            submission_journey=False, migration_journey=False, migration_backfill_journey=False,
            migration_completion_journey=False, logbook_period_journey=False,
            logbook_lifecycle_journey=False, notification_journey=False, gradebook_journey=False,
            completion_journey=False, completion_correction_journey=False,
            credential_journey=False, legacy_output_journey=False, class_academic_journey=False,
            content_closure_journey=False, ai_suggestion_journey=False, artifact_parent=None,
            consumer_closure_journey=False, reporting_journey=False, reporting_contract_journey=False,
            reporting_pagination_journey=False, reporting_performance_journey=False):
    tools = validate_tools(pg_bin)
    if len(tools) != 6:
        raise ValueError("PostgreSQL tools unavailable")
    # One canonical local root; legacy .qa-artifacts is retained for history.
    default_parent = ROOT.parent.parent / "lms-v3-qa-artifacts"
    parent = Path(artifact_parent).resolve() if artifact_parent else default_parent
    if artifact_parent and (not parent.is_absolute() or parent.parent == parent or parent.name != "lms-v3-qa-artifacts"):
        raise ValueError("external artifact parent rejected")
    parent.mkdir(exist_ok=True)
    with run_lock(parent / "storage.lock"), run_lock(parent / "postgres-lab.lock"):
        storage_state = storage.preflight(ROOT, parent)
        run_id = datetime.now(timezone.utc).strftime("%Y%m%dT%H%M%SZ") + "-postgres-lab-" + uuid.uuid4().hex[:12]
        directory = parent / run_id
        directory.mkdir()
        write_new_json(directory / "storage-preflight.json", storage_state)
        source = source_fingerprint(ROOT)
        data = directory / "postgres"
        password = secrets.token_hex(32)
        marker = secrets.token_hex(32)
        password_file = directory / "synthetic-password.private"
        with password_file.open("x", encoding="utf-8") as f:
            f.write(password)
        env = clean_environment(ROOT, os.environ)
        # Database creation/restore can exceed five seconds on a cold Windows
        # filesystem. The subprocess timeout remains the outer bound; runtime
        # roles still enforce their own 3-5 second statement limits.
        env.update(PGPASSWORD=password, PGCONNECT_TIMEOUT="10", PGSSLMODE="disable", PGOPTIONS="-c statement_timeout=30000")
        with socket.socket() as sock:
            sock.bind(("127.0.0.1", 0))
            port = sock.getsockname()[1]
        connection = ["--host=127.0.0.1", "--port=" + str(port), "--username=qa_fixture", "--no-password"]
        result = dict(run_id=run_id, source_hash=source, status="FAIL", profile="new-local-synthetic",
                      shared_database_access="NONE", database_deletion="NONE", cleanup="STOP_PROCESS_RETAIN_CLUSTER",
                      production_backup="NOT_EXECUTED", migration_authority=False)
        started = False
        storage_monitoring = True
        counter = 0

        def command(args, timeout=45, expect_failure=False):
            nonlocal counter
            counter += 1
            heartbeat_stop = threading.Event()
            heartbeat = None
            if 'qa/tests/reporting_performance_api' in args:
                def progress_heartbeat():
                    began = time.monotonic()
                    while not heartbeat_stop.wait(30):
                        progress = directory / 'm14cp04-progress.jsonl'
                        label = 'starting'
                        if progress.is_file():
                            try:
                                label = json.loads(progress.read_text(encoding='utf-8').splitlines()[-1])['stage']
                            except (ValueError, IndexError, KeyError):
                                label = 'progress pending'
                        print(f'[qa] M14-CP04 elapsed {int(time.monotonic()-began)}s; {label}; run {run_id}', flush=True)
                heartbeat = threading.Thread(target=progress_heartbeat, daemon=True)
                heartbeat.start()
            try:
                code, output, error_output = run_to_files(args, directory, counter, env, timeout)
                # Fixture-only output; never record passwords, DSNs or args.
                stderr = error_output.decode("utf-8", "replace").replace(password, "[REDACTED]")
                write_new_json(directory / f"command-{counter:02}.json", {"tool": Path(str(args[0])).name,
                               "exit_code": code, "expected_failure": expect_failure, "stderr": stderr})
                require_outcome(code, expect_failure)
                # On some restricted Windows sessions initdb emits a fatal
                # restricted-token error while incorrectly returning zero.
                # Do not treat a partial data directory as a started cluster.
                if Path(str(args[0])).stem.lower() == "initdb" and "initdb: error:" in stderr.lower():
                    result["status"] = "BLOCKED_LOCAL_POSTGRES"
                    raise ValueError("initdb reported failure despite exit code zero")
                if counter % 100 == 0:
                    if storage_monitoring:
                        storage.guard_growth(directory)
                    print(f"[qa] {counter} synthetic commands completed; run {run_id}", flush=True)
                return output
            except subprocess.TimeoutExpired:
                write_new_json(directory / f"timeout-{counter:02}.json", {"status": "TIMEOUT"})
                raise ValueError("bounded synthetic command timed out")
            finally:
                heartbeat_stop.set()
                if heartbeat is not None:
                    heartbeat.join(timeout=1)

        def sql(database, text):
            return command([tools["psql"], *connection, "--dbname=" + database, "-X", "-A", "-t", "-v", "ON_ERROR_STOP=1", "-c", text]).strip()

        def snapshot(database, name):
            print(f"[qa] backup and isolated restore: {name}; run {run_id}", flush=True)
            archive, rendered = directory / (name + ".dump"), directory / (name + ".sql")
            if archive.exists() or rendered.exists():
                raise ValueError("artifact overwrite refused")
            command([tools["pg_dump"], *connection, "--dbname=" + database, "--format=custom", "--no-owner", "--no-acl", "--file=" + str(archive)])
            command([tools["pg_restore"], "--list", archive])
            rendered_bytes = command([tools["pg_restore"], "--no-owner", "--no-acl", "--file=-", archive])
            with rendered.open("xb") as handle:
                handle.write(rendered_bytes)
            proof = name + "_proof"
            command([tools["createdb"], *connection, "--maintenance-db=postgres", proof])
            command([tools["pg_restore"], *connection, "--dbname=" + proof, "--exit-on-error", "--no-owner", "--no-acl", archive])
            for compare in CATALOG_QUERIES:
                if sql(database, compare) != sql(proof, compare):
                    raise ValueError("synthetic restore catalog mismatch")
            if name != "prestate":
                for text in ["SELECT value FROM qa_cp05.marker"] + [f"SELECT id FROM qa_cp05.{t} ORDER BY id" for t in ("domain", "audit", "outbox")]:
                    if sql(database, text) != sql(proof, text):
                        raise ValueError("synthetic restore data mismatch")
            if name in ("pre_cp11", "pre_cp12", "pre_m03_cp05", "pre_m05_cp03", "pre_m05_cp04", "pre_m05_cp05", "pre_m06_cp01", "pre_m06_cp02", "pre_m07_cp01", "pre_m07_cp02", "pre_m07_cp03", "pre_m07_cp04", "pre_m08_cp01", "pre_m08_cp02", "pre_m08_cp03", "pre_m08_cp04", "pre_m15_cp01", "pre_m15_cp02", "pre_m15_cp03", "pre_m15_cp04", "pre_m15_cp05", "pre_m11_cp01", "pre_m12_cp01", "pre_m12_cp02", "pre_m12_cp03", "pre_m13_cp01", "pre_m13_cp02", "pre_m13_cp03", "pre_m09_cp01", "pre_m09_cp02", "pre_m09_cp03", "pre_m09_cp04", "pre_m10_cp01", "pre_m10_cp02", "pre_m10_cp03", "pre_m10_cp04", "pre_m14_cp04"):
                for compare in CATALOG_QUERIES:
                    compare = compare.replace("('public','qa_cp05')", "('public','qa_cp05','v3','v3ops','v3identity','v3migration')")
                    if sql(database, compare) != sql(proof, compare):
                        raise ValueError("lifecycle restore catalog mismatch")
                tables = sql(database, "SELECT schemaname||'.'||tablename FROM pg_tables WHERE schemaname IN ('v3','v3ops','v3identity','v3migration') ORDER BY 1").decode().splitlines()
                import re
                for table in tables:
                    if not re.fullmatch(r"v3(?:ops|identity|migration)?\.[a-z_]+", table):
                        raise ValueError("unreviewed backup table name")
                    compare = f"SELECT to_jsonb(r)::text FROM {table} r ORDER BY 1"
                    if sql(database, compare) != sql(proof, compare):
                        raise ValueError("lifecycle restore rows mismatch")
            receipt = {"scope": "SYNTHETIC_ONLY", "restore": "VERIFIED_FIXTURE_SCOPE", "source_database": database,
                       "proof_database": proof, "artifacts": {p.name: sha256(p.read_bytes()).hexdigest() for p in (archive, rendered)}}
            write_new_json(directory / (name + "-receipt.json"), receipt)
            return receipt

        def negative_backup_checks():
            original = directory / "fixture.dump"
            original_bytes = original.read_bytes()
            original_hash = sha256(original_bytes).hexdigest()
            checks = []
            # New derivatives only; never damage or overwrite the verified backup.
            corrupt = directory / "invalid-header.dump"
            with corrupt.open("xb") as handle:
                handle.write(b"INVALID" + original_bytes[7:])
            try:
                require_digest(corrupt, original_hash)
            except ValueError:
                checks.append({"id": "archive-digest-mismatch", "status": "PASS", "restore_attempted": False})
            else:
                raise ValueError("changed archive passed integrity gate")
            command([tools["pg_restore"], "--list", corrupt], expect_failure=True)
            checks.append({"id": "invalid-archive-header", "status": "PASS"})
            truncated = directory / "truncated-data.dump"
            with truncated.open("xb") as handle:
                handle.write(original_bytes[:-64])
            # A readable TOC alone does not prove that table payloads are intact.
            command([tools["pg_restore"], "--list", truncated])
            command([tools["pg_restore"], "--no-owner", "--no-acl", "--file=-", truncated], expect_failure=True)
            checks.append({"id": "truncated-data-stream", "status": "PASS"})
            for name, exclusion in (("missing_object", "--exclude-table=qa_cp05.outbox"),
                                    ("missing_rows", "--exclude-table-data=qa_cp05.outbox")):
                archive = directory / (name + ".dump")
                if archive.exists():
                    raise ValueError("artifact overwrite refused")
                command([tools["pg_dump"], *connection, "--dbname=qa_cp05", "--format=custom", "--no-owner", "--no-acl", exclusion, "--file=" + str(archive)])
                proof = name + "_proof"
                command([tools["createdb"], *connection, "--maintenance-db=postgres", proof])
                command([tools["pg_restore"], *connection, "--dbname=" + proof, "--exit-on-error", "--no-owner", "--no-acl", archive])
                same_catalog = all(sql("qa_cp05", q) == sql(proof, q) for q in CATALOG_QUERIES)
                if name == "missing_object":
                    if same_catalog:
                        raise ValueError("missing object escaped reconciliation")
                else:
                    if not same_catalog or sql("qa_cp05", "SELECT id FROM qa_cp05.outbox ORDER BY id") == sql(proof, "SELECT id FROM qa_cp05.outbox ORDER BY id"):
                        raise ValueError("missing rows escaped reconciliation")
                checks.append({"id": name, "status": "PASS", "restore_completed": True,
                               "reconciliation": "REJECTED", "proof_database": proof,
                               "archive_sha256": sha256(archive.read_bytes()).hexdigest()})
            require_digest(original, original_hash)
            return checks

        try:
            # Always a NEW cluster initialized by this invocation, bound to loopback.
            command([tools["initdb"], "-D", data, "--username=qa_fixture", "--pwfile=" + str(password_file),
                     *INITDB_SAFETY_OPTIONS], 60)
            if not (data / "PG_VERSION").is_file() or not (data / "pg_hba.conf").is_file():
                raise ValueError("initdb returned without a complete cluster; retained, not started")
            # pg_ctl executes postgres; data/options contain no user SQL/credentials.
            started = True  # also stop after partial start/timeout
            command([tools["pg_ctl"], "-D", data, "-l", directory / "postgres.log", "-o",
                     f"-h 127.0.0.1 -p {port} -c ssl=off -c max_connections=10", "-w", "-t", "15", "start"], 25)
            command([tools["createdb"], *connection, "--maintenance-db=postgres", "qa_cp05"])
            # Backup + actual isolated restore proof BEFORE fixture DDL.
            result["prestate_backup"] = snapshot("qa_cp05", "prestate")
            sql("qa_cp05", fixture_sql(marker))
            result["fixture_backup"] = snapshot("qa_cp05", "fixture")
            result["negative_backup_checks"] = negative_backup_checks()
            env.update(QA_PG_ACK="new-local-synthetic-cluster", QA_PG_PORT=str(port), QA_PG_PASSWORD=password, QA_PG_MARKER=marker)
            output = command(["go", "test", "-json", "-count=1", "-tags=qaintegration",
                              "-coverprofile=" + str(directory / "integration-coverage.out"), "./internal/platform/transaction"], 120)
            with (directory / "go-test.jsonl").open("xb") as handle:
                handle.write(output)
            cases = []
            for line in output.splitlines():
                event = json.loads(line)
                if event.get("Test") and event.get("Action") in ("pass", "fail", "skip"):
                    cases.append({"package": event["Package"], "name": event["Test"], "status": event["Action"]})
            expected = ["lmsv3/internal/platform/transaction:" + name for name in
                        ("TestTransactionBoundaries", "TestCancellationAndPanicRollback", "TestSyntheticPostgresTransactions", "TestSyntheticPostgresDenials")]
            if not verify_go_cases(expected, cases):
                raise ValueError("integration cases missing/failed/skipped")
            result["cases"] = cases
            result["instrumented_coverage"] = command(["go", "tool", "cover", "-func=" + str(directory / "integration-coverage.out")]).decode().strip()
            if class_journey:
                # Fresh backup + isolated restore BEFORE v3 schema/provisioning.
                result["pre_v3_backup"] = snapshot("qa_cp05", "pre_v3")
                env.update(QA_LAB_DIR=str(directory), QA_RUNTIME_PASSWORD=secrets.token_hex(32),
                           QA_WORKER_PASSWORD=secrets.token_hex(32))
                target = sha256(("127.0.0.1\0" + str(port) + "\0qa_cp05\0" + marker).encode()).hexdigest()
                write_new_json(directory / "pre-v3-proof.json", dict(run_id=run_id, target=target,
                    created_at=datetime.now(timezone.utc).isoformat(), restored=True,
                    archive_hash=sha256((directory / "pre_v3.dump").read_bytes()).hexdigest()))
                labels = ("admin", "teacher", "teacher2", "student", "outsider", "disabled", "expired", "revoked", "student2")
                write_new_json(directory / "actors.private.json", {label: dict(id=f"{i:032x}", token=secrets.token_hex(32)) for i, label in enumerate(labels, 1)})
                manifest = json.loads((ROOT / "qa/manifests/CP08-CP10.json").read_text())
                manifest.update(json.loads((ROOT / "qa/manifests/M03-CP03-CP04.json").read_text()))
                if worker_journey:
                    manifest.update(json.loads((ROOT / "qa/manifests/M03-CP05-CP06.json").read_text()))
                if participation_journey:
                    manifest.update(json.loads((ROOT / "qa/manifests/M05-CP03-CP04.json").read_text()))
                if file_journey:
                    manifest.update(json.loads((ROOT / "qa/manifests/M06-CP01-CP02.json").read_text()))
                if content_journey:
                    manifest.update(json.loads((ROOT / "qa/manifests/M06-CP03-M07-CP01.json").read_text()))
                if content_safety_journey:
                    manifest.update(json.loads((ROOT / "qa/manifests/M07-CP02.json").read_text()))
                if content_contract_journey:
                    manifest.update(json.loads((ROOT / "qa/manifests/M07-CP03.json").read_text()))
                if submission_journey:
                    manifest.update(json.loads((ROOT / "qa/manifests/M08-CP01-CP02.json").read_text()))
                    manifest.update(json.loads((ROOT / "qa/manifests/M08-CP03-CP04.json").read_text()))
                if migration_journey:
                    manifest.update(json.loads((ROOT / "qa/manifests/M15-CP01-CP02.json").read_text()))
                if migration_backfill_journey:
                    manifest.update(json.loads((ROOT / "qa/manifests/M15-CP03-CP04.json").read_text()))
                if migration_completion_journey:
                    manifest.update(json.loads((ROOT / "qa/manifests/M15-CP05-M11-CP01.json").read_text()))
                if logbook_period_journey:
                    manifest.update(json.loads((ROOT / "qa/manifests/M12-CP01.json").read_text()))
                if logbook_lifecycle_journey:
                    manifest.update(json.loads((ROOT / "qa/manifests/M12-CP02-CP03.json").read_text()))
                if notification_journey:
                    manifest.update(json.loads((ROOT / "qa/manifests/M13-CP01-CP02.json").read_text()))
                if gradebook_journey:
                    manifest.update(json.loads((ROOT / "qa/manifests/M09-CP01-CP02.json").read_text()))
                    manifest.update(json.loads((ROOT / "qa/manifests/M09-CP03.json").read_text()))
                    manifest.update(json.loads((ROOT / "qa/manifests/M09-CP04.json").read_text()))
                if completion_journey:
                    manifest.update(json.loads((ROOT / "qa/manifests/M10-CP01.json").read_text()))
                if completion_correction_journey:
                    manifest.update(json.loads((ROOT / "qa/manifests/M10-CP02.json").read_text()))
                if credential_journey:
                    manifest.update(json.loads((ROOT / "qa/manifests/M10-CP03.json").read_text()))
                if legacy_output_journey:
                    manifest.update(json.loads((ROOT / "qa/manifests/M10-CP04.json").read_text()))
                if class_academic_journey:
                    manifest.update(json.loads((ROOT / "qa/manifests/M05-CP05.json").read_text()))
                if content_closure_journey:
                    manifest.update(json.loads((ROOT / "qa/manifests/M07-CP04.json").read_text()))
                if ai_suggestion_journey:
                    manifest.update(json.loads((ROOT / "qa/manifests/M13-CP03.json").read_text()))
                if consumer_closure_journey:
                    manifest.update(json.loads((ROOT / "qa/manifests/M13-CP04.json").read_text()))
                if reporting_journey:
                    manifest.update(json.loads((ROOT / "qa/manifests/M14-CP01.json").read_text()))
                if reporting_contract_journey:
                    manifest.update(json.loads((ROOT / "qa/manifests/M14-CP02.json").read_text()))
                if reporting_pagination_journey:
                    manifest.update(json.loads((ROOT / "qa/manifests/M14-CP03.json").read_text()))
                if reporting_performance_journey:
                    manifest.update(json.loads((ROOT / "qa/manifests/M14-CP04.json").read_text()))

                def slice_go(checkpoint, package, test, race=False):
                    go_args = ["go", "test"]
                    if race:
                        go_args.append("-race")
                    output = command([*go_args, "-json", "-count=1", "-tags=qaintegration", "-run=^"+test+"$",
                        "-coverprofile="+str(directory / (checkpoint+"-coverage.out")), package], 180 if race else 120)
                    events = [json.loads(line) for line in output.splitlines()]
                    cases = [dict(package=e["Package"], name=e["Test"], status=e["Action"]) for e in events if e.get("Test") and e.get("Action") in ("pass", "fail", "skip")]
                    if not verify_go_cases(manifest[checkpoint+"_go"], cases):
                        raise ValueError("checkpoint integration incomplete")
                    write_new_json(directory / (checkpoint+"-cases.json"), cases)
                    result[checkpoint] = dict(status="PASS_SYNTHETIC_ONLY", cases=cases)

                # Keep older journeys pinned to the schema they exercise. File
                # and content journeys install M06/M07 incrementally only after
                # their checkpoint-specific backup and restore proof exists.
                env["QA_CATALOGUE_SCOPE"] = "pre-m06" if file_journey else "pre-m07"
                slice_go("m03_cp03", "./internal/migration/runner", "TestSyntheticForwardRunner")
                env.pop("QA_CATALOGUE_SCOPE", None)
                migrate_binary = directory / ("migrate.exe" if os.name == "nt" else "migrate")
                command(["go", "build", "-trimpath", "-o", migrate_binary, "./cmd/migrate"], 120)
                env["LMS_MIGRATION_MODE"] = "local-synthetic"
                if not file_journey:
                    env["QA_CATALOGUE_SCOPE"] = "pre-m07"
                    migration_output = command([migrate_binary], 45)
                    env.pop("QA_CATALOGUE_SCOPE", None)
                    migration_result = json.loads(migration_output)
                    expected_replay = {"class-slice-1","cp11-1","cp12-1","m03-cp05-1","m05-cp03-1","m05-cp04-1","m06-cp01-1","m06-cp02-1"}
                    if migration_result.get("status") != "APPLIED" or migration_result.get("applied") or set(migration_result.get("replayed",[])) != expected_replay:
                        raise ValueError("operator migration replay evidence invalid")
                    result["m03_cp03"]["operator_cli"] = "PASS_REPLAY_SYNTHETIC_ONLY"
                result["m03_cp03"]["binary_sha256"] = sha256(migrate_binary.read_bytes()).hexdigest()
                slice_go("cp08", "./internal/migration/backup", "TestSyntheticBackupProtection")
                recovered = directory / "recovered.dump"
                command([tools["pg_restore"], "--list", recovered])
                command([tools["createdb"], *connection, "--maintenance-db=postgres", "recovered_proof"])
                command([tools["pg_restore"], *connection, "--dbname=recovered_proof", "--exit-on-error", "--no-owner", "--no-acl", recovered])
                for query in CATALOG_QUERIES + tuple(f"SELECT id FROM qa_cp05.{t} ORDER BY id" for t in ("domain", "audit", "outbox")):
                    if sql("qa_cp05", query) != sql("recovered_proof", query):
                        raise ValueError("encrypted recovery restore mismatch")
                result["cp08"]["recovered_restore"] = "VERIFIED_FIXTURE_SCOPE"
                slice_go("cp09", "./internal/classes", "TestSyntheticRuntimeFoundation")
                # Synthetic role provisioning is operator-owned and separate
                # from schema DDL. M05 tables have no DELETE/TRUNCATE grants.
                sql("qa_cp05", "GRANT SELECT,INSERT,UPDATE ON v3.class_groups,v3.group_memberships,v3.mentor_assignments,v3.invitations TO qa_runtime")
                binary = directory / ("class-lab.exe" if os.name == "nt" else "class-lab")
                command(["go", "build", "-cover", "-coverpkg=./cmd/class-lab,./internal/classes,./internal/identity,./internal/platform/database", "-trimpath", "-o", binary, "./cmd/class-lab"], 120)
                coverdir = directory / "cp10-go-cover"
                coverdir.mkdir()
                env.update(QA_CLASS_BINARY=str(binary), QA_ARTIFACT_ROOT=str(directory), QA_CASE_RESULTS=str(directory / "cp10-cases.json"), GOCOVERDIR=str(coverdir))
                command([sys.executable, "-m", "pytest", "-p", "qa.plugin", "qa/tests/class_api", "-q"], 120)
                cases = json.loads((directory / "cp10-cases.json").read_text())
                expected = set(manifest["cp10_python"])
                if cases["exit_status"] != 0 or set(cases["collected_ids"]) != expected or len(cases["cases"]) != len(expected) or any(c["status"] != "PASS" for c in cases["cases"]):
                    raise ValueError("Class Playwright cases incomplete")
                result["cp10"] = dict(status="PASS_SYNTHETIC_ONLY", cases=cases["cases"], binary_sha256=sha256(binary.read_bytes()).hexdigest())
                command(["go", "tool", "covdata", "textfmt", "-i="+str(coverdir), "-o="+str(directory / "cp10-coverage.out")])
                require_coverage(directory / "cp10-coverage.out")
                result["cp10"]["instrumented_go_coverage"] = command(["go", "tool", "cover", "-func="+str(directory / "cp10-coverage.out")]).decode().strip()
                for query in (
                    "SELECT count(*) FROM v3.audit a LEFT JOIN v3.outbox o ON o.event_id=a.id WHERE o.event_id IS NULL",
                    "SELECT count(*) FROM v3.episodes p JOIN v3.enrollments e ON e.id=p.enrollment_id WHERE p.ended_at IS NULL AND e.status<>'ACTIVE'",
                    "SELECT count(*) FROM v3.roles r JOIN v3.enrollments e ON e.id=r.enrollment_id WHERE r.revoked_at IS NULL AND e.status<>'ACTIVE'",
                ):
                    if sql("qa_cp05", query) != b"0":
                        raise ValueError("Class persistence reconciliation failed")
                result["cp10"]["database_reconciliation"] = "PASS"
                if identity_journey:
                    manifest.update(json.loads((ROOT / "qa/manifests/CP11-CP12.json").read_text()))
                    env["QA_IDENTITY_PASSWORD"] = secrets.token_hex(32)
                    issuer_binary = directory / ("bridge-fixture.exe" if os.name == "nt" else "bridge-fixture")
                    command(["go", "build", "-trimpath", "-o", issuer_binary, "./cmd/bridge-fixture"],120)
                    identity_binary = directory / ("identity-lab.exe" if os.name == "nt" else "identity-lab")
                    command(["go", "build", "-cover", "-coverpkg=./cmd/identity-lab,./internal/identity,./internal/classes,./internal/files,./internal/egress,./internal/content,./internal/submissions,./internal/attendance,./internal/logbook,./internal/notifications,./internal/aisuggestions,./internal/gradebook,./internal/completion,./internal/reporting,./internal/access,./internal/platform/database", "-trimpath", "-o", identity_binary, "./cmd/identity-lab"],120)
                    for cp, package, test in (("cp11", "./internal/identity", "TestSyntheticIdentityLifecycle"), ("cp12", "./internal/classes", "TestSyntheticContextLifecycle")):
                        result[cp+"_backup"] = snapshot("qa_cp05", "pre_"+cp)
                        write_new_json(directory / (cp+"-proof.json"),dict(run_id=run_id,target=target,created_at=datetime.now(timezone.utc).isoformat(),restored=True,archive_hash=sha256((directory/("pre_"+cp+".dump")).read_bytes()).hexdigest()))
                        slice_go(cp, package, test)
                        identity_state = json.loads(sql("qa_cp05", "SELECT jsonb_object_agg(l.subject,jsonb_build_object('id',l.user_id,'version',l.version,'active',l.active,'site_role_version',r.version)) FROM v3identity.links l JOIN v3identity.site_roles r ON r.user_id=l.user_id"))
                        write_new_json(directory / (cp+"-state.json"),identity_state)
                        cp_cover=directory/(cp+"-api-cover");cp_cover.mkdir()
                        env.update(QA_CHECKPOINT=cp, QA_IDENTITY_BINARY=str(identity_binary),QA_ISSUER_BINARY=str(issuer_binary),QA_BRIDGE_PUBLIC=(directory/"issuer-public.txt").read_text(),QA_CASE_RESULTS=str(directory/(cp+"-api-cases.json")),GOCOVERDIR=str(cp_cover))
                        command([sys.executable,"-m","pytest","-p","qa.plugin","qa/tests/identity_api","-q"],120)
                        api_cases=json.loads((directory/(cp+"-api-cases.json")).read_text())
                        expected=set(manifest[cp+"_python"])
                        if api_cases["exit_status"]!=0 or set(api_cases["collected_ids"])!=expected or len(api_cases["cases"])!=len(expected) or any(c["status"]!="PASS" for c in api_cases["cases"]):
                            raise ValueError("lifecycle API required cases incomplete")
                        result[cp]["api_cases"]=api_cases["cases"]
                        profile=directory/(cp+"-api-coverage.out")
                        command(["go","tool","covdata","textfmt","-i="+str(cp_cover),"-o="+str(profile)])
                        if not profile.is_file() or "lmsv3/internal/identity/" not in profile.read_text() or not any(line.endswith(" 1") for line in profile.read_text().splitlines()):
                            raise ValueError("lifecycle API coverage missing")
                        result[cp]["api_coverage"]=command(["go","tool","cover","-func="+str(profile)]).decode().strip()
                        result[cp]["binary_sha256"]=sha256(identity_binary.read_bytes()).hexdigest()
                    for query in ("SELECT count(*) FROM v3.audit a LEFT JOIN v3.outbox o ON o.event_id=a.id WHERE o.event_id IS NULL", "SELECT count(*) FROM v3.enrollment_access x JOIN v3.enrollments e ON e.id=x.enrollment_id WHERE x.suspended_at IS NOT NULL AND e.status<>'ACTIVE'", "SELECT count(*) FROM v3.episodes p JOIN v3.enrollments e ON e.id=p.enrollment_id WHERE p.ended_at IS NULL AND e.status<>'ACTIVE'", "SELECT count(*) FROM v3.roles r JOIN v3.enrollments e ON e.id=r.enrollment_id WHERE r.revoked_at IS NULL AND e.status<>'ACTIVE'"):
                        if sql("qa_cp05",query)!=b"0":raise ValueError("lifecycle persistence reconciliation failed")
                    for query in ("SELECT EXISTS(SELECT 1 FROM v3.transition_events WHERE action='end' AND previous_state='SUSPENDED' AND next_state='ENDED')", "SELECT EXISTS(SELECT 1 FROM v3.transition_events t WHERE action='add' AND previous_state='ENDED' AND (SELECT count(*) FROM v3.episodes p WHERE p.enrollment_id=t.enrollment_id)>=2)", "SELECT EXISTS(SELECT 1 FROM v3.transition_events WHERE action='close' AND previous_state='PUBLISHED' AND next_state='CLOSED')", "SELECT EXISTS(SELECT 1 FROM v3.transition_events WHERE action='reactivate' AND previous_state='SUSPENDED' AND next_state='ACTIVE')"):
                        if sql("qa_cp05",query)!=b"t":raise ValueError("lifecycle history evidence missing")
                    result["lifecycle_reconciliation"]="PASS"
                    def submission_api(checkpoint, expected_key):
                        state = json.loads(sql("qa_cp05", "SELECT jsonb_object_agg(l.subject,jsonb_build_object('id',l.user_id,'version',l.version,'active',l.active,'site_role_version',r.version)) FROM v3identity.links l JOIN v3identity.site_roles r ON r.user_id=l.user_id"))
                        write_new_json(directory / (checkpoint+"-state.json"), state)
                        cover = directory / (checkpoint+"-api-cover")
                        cover.mkdir()
                        env.update(QA_CHECKPOINT=checkpoint, QA_IDENTITY_BINARY=str(identity_binary),
                                   QA_ISSUER_BINARY=str(issuer_binary), QA_BRIDGE_PUBLIC=(directory/"issuer-public.txt").read_text(),
                                   QA_FILE_ROOT=str(directory/(checkpoint+"-private-objects")),
                                   QA_CASE_RESULTS=str(directory/(checkpoint+"-api-cases.json")), GOCOVERDIR=str(cover))
                        command([sys.executable,"-m","pytest","-p","qa.plugin","qa/tests/submission_api","-q"],210)
                        cases=json.loads((directory/(checkpoint+"-api-cases.json")).read_text())
                        expected=set(manifest[expected_key])
                        if cases["exit_status"]!=0 or set(cases["collected_ids"])!=expected or len(cases["cases"])!=len(expected) or any(case["status"]!="PASS" for case in cases["cases"]):
                            raise ValueError(checkpoint+" Playwright cases incomplete")
                        profile=directory/(checkpoint+"-api-coverage.out")
                        command(["go","tool","covdata","textfmt","-i="+str(cover),"-o="+str(profile)])
                        require_coverage(profile)
                        if "lmsv3/internal/submissions/" not in profile.read_text(encoding="utf-8"):
                            raise ValueError(checkpoint+" submission coverage missing")
                        result[checkpoint+"_api"]={"status":"PASS_SYNTHETIC_ONLY","cases":cases["cases"],
                            "instrumented_go_coverage":command(["go","tool","cover","-func="+str(profile)]).decode().strip()}
                    if file_journey:
                        # Install current-source M06 schema before checking the
                        # current API binary's readiness. Every installment is
                        # still preceded by its own verified snapshot/restore.
                        for checkpoint, test in (("m06_cp01", "TestSyntheticPrivateFileOwnership"),
                                                 ("m06_cp02", "TestSyntheticFileDeliveryLifecycle")):
                            snapshot_name = "pre_" + checkpoint
                            result[checkpoint + "_backup"] = snapshot("qa_cp05", snapshot_name)
                            write_new_json(directory / (checkpoint.replace("_", "-") + "-proof.json"), {
                                "run_id": run_id,
                                "target": target,
                                "created_at": datetime.now(timezone.utc).isoformat(),
                                "restored": True,
                                "archive_hash": sha256((directory / (snapshot_name + ".dump")).read_bytes()).hexdigest(),
                            })
                            slice_go(checkpoint, "./internal/files", test, race=os.name != "nt")
                        env["QA_CATALOGUE_SCOPE"] = "pre-m07"
                        migration_output = command([migrate_binary], 45)
                        env.pop("QA_CATALOGUE_SCOPE", None)
                        migration_result = json.loads(migration_output)
                        expected_replay = {"class-slice-1","cp11-1","cp12-1","m03-cp05-1","m05-cp03-1","m05-cp04-1","m06-cp01-1","m06-cp02-1"}
                        if migration_result.get("status") != "APPLIED" or migration_result.get("applied") or set(migration_result.get("replayed",[])) != expected_replay:
                            raise ValueError("M06 operator migration replay evidence invalid")
                        result["m03_cp03"]["operator_cli"] = "PASS_REPLAY_SYNTHETIC_ONLY"
                        if content_journey:
                            slice_go("m06_cp03", "./internal/files", "TestSyntheticBoundedRemoteImport", race=os.name != "nt")
                            result["m07_cp01_backup"] = snapshot("qa_cp05", "pre_m07_cp01")
                            write_new_json(directory / "m07-cp01-proof.json", {
                                "run_id": run_id, "target": target,
                                "created_at": datetime.now(timezone.utc).isoformat(), "restored": True,
                                "archive_hash": sha256((directory / "pre_m07_cp01.dump").read_bytes()).hexdigest(),
                            })
                            slice_go("m07_cp01", "./internal/content", "TestSyntheticContentStructure", race=os.name != "nt")
                            env["QA_CATALOGUE_SCOPE"] = "pre-m07-cp02"
                            migration_output = command([migrate_binary], 45)
                            env.pop("QA_CATALOGUE_SCOPE", None)
                            migration_result = json.loads(migration_output)
                            expected_replay = {"class-slice-1","cp11-1","cp12-1","m03-cp05-1","m05-cp03-1","m05-cp04-1","m06-cp01-1","m06-cp02-1","m07-cp01-1"}
                            if migration_result.get("status") != "APPLIED" or migration_result.get("applied") or set(migration_result.get("replayed",[])) != expected_replay:
                                raise ValueError("M07 operator migration replay evidence invalid")
                            if content_safety_journey:
                                result["m07_cp02_backup"] = snapshot("qa_cp05", "pre_m07_cp02")
                                write_new_json(directory / "m07-cp02-proof.json", {
                                    "run_id": run_id, "target": target,
                                    "created_at": datetime.now(timezone.utc).isoformat(), "restored": True,
                                    "archive_hash": sha256((directory / "pre_m07_cp02.dump").read_bytes()).hexdigest(),
                                })
                                slice_go("m07_cp02", "./internal/content", "TestSyntheticSafeContentAvailability", race=os.name != "nt")
                                env["QA_CATALOGUE_SCOPE"] = "pre-m07-cp03"
                                migration_output = command([migrate_binary], 45)
                                env.pop("QA_CATALOGUE_SCOPE", None)
                                migration_result = json.loads(migration_output)
                                expected_replay = {"class-slice-1","cp11-1","cp12-1","m03-cp05-1","m05-cp03-1","m05-cp04-1","m06-cp01-1","m06-cp02-1","m07-cp01-1","m07-cp02-1"}
                                if migration_result.get("status") != "APPLIED" or migration_result.get("applied") or set(migration_result.get("replayed",[])) != expected_replay:
                                    raise ValueError("M07-CP02 operator migration replay evidence invalid")
                                if content_contract_journey:
                                    result["m07_cp03_backup"] = snapshot("qa_cp05", "pre_m07_cp03")
                                    write_new_json(directory / "m07-cp03-proof.json", {
                                        "run_id": run_id, "target": target,
                                        "created_at": datetime.now(timezone.utc).isoformat(), "restored": True,
                                        "archive_hash": sha256((directory / "pre_m07_cp03.dump").read_bytes()).hexdigest(),
                                    })
                                    slice_go("m07_cp03", "./internal/content", "TestSyntheticAssignmentRevisionStability", race=os.name != "nt")
                                    env["QA_CATALOGUE_SCOPE"] = "pre-m08-cp01"
                                    migration_output = command([migrate_binary], 45)
                                    env.pop("QA_CATALOGUE_SCOPE", None)
                                    migration_result = json.loads(migration_output)
                                    expected_replay = {"class-slice-1","cp11-1","cp12-1","m03-cp05-1","m05-cp03-1","m05-cp04-1","m06-cp01-1","m06-cp02-1","m07-cp01-1","m07-cp02-1","m07-cp03-1"}
                                    if migration_result.get("status") != "APPLIED" or migration_result.get("applied") or set(migration_result.get("replayed",[])) != expected_replay:
                                        raise ValueError("M07-CP03 operator migration replay evidence invalid")
                                    if submission_journey:
                                        result["m08_cp01_backup"] = snapshot("qa_cp05", "pre_m08_cp01")
                                        write_new_json(directory / "m08-cp01-proof.json", {
                                            "run_id": run_id, "target": target,
                                            "created_at": datetime.now(timezone.utc).isoformat(), "restored": True,
                                            "archive_hash": sha256((directory / "pre_m08_cp01.dump").read_bytes()).hexdigest(),
                                        })
                                        slice_go("m08_cp01", "./internal/submissions", "TestSyntheticSubmissionFoundation", race=os.name != "nt")
                                        submission_api("m08cp01", "m08_cp01_python")
                                        env["QA_CATALOGUE_SCOPE"] = "pre-m08-cp02"
                                        migration_output = command([migrate_binary], 45)
                                        env.pop("QA_CATALOGUE_SCOPE", None)
                                        migration_result = json.loads(migration_output)
                                        expected_replay = {"class-slice-1","cp11-1","cp12-1","m03-cp05-1","m05-cp03-1","m05-cp04-1","m06-cp01-1","m06-cp02-1","m07-cp01-1","m07-cp02-1","m07-cp03-1","m08-cp01-1"}
                                        if migration_result.get("status") != "APPLIED" or migration_result.get("applied") or set(migration_result.get("replayed",[])) != expected_replay:
                                            raise ValueError("M08-CP01 operator migration replay evidence invalid")
                                        result["m08_cp02_backup"] = snapshot("qa_cp05", "pre_m08_cp02")
                                        write_new_json(directory / "m08-cp02-proof.json", {
                                            "run_id": run_id, "target": target,
                                            "created_at": datetime.now(timezone.utc).isoformat(), "restored": True,
                                            "archive_hash": sha256((directory / "pre_m08_cp02.dump").read_bytes()).hexdigest(),
                                        })
                                        slice_go("m08_cp02", "./internal/submissions", "TestSyntheticSubmissionRevisionLifecycle", race=os.name != "nt")
                                        env["QA_CATALOGUE_SCOPE"] = "pre-m08-cp03"
                                        migration_output = command([migrate_binary], 45)
                                        env.pop("QA_CATALOGUE_SCOPE", None)
                                        migration_result = json.loads(migration_output)
                                        expected_replay.add("m08-cp02-1")
                                        if migration_result.get("status") != "APPLIED" or migration_result.get("applied") or set(migration_result.get("replayed",[])) != expected_replay:
                                            raise ValueError("M08-CP02 operator migration replay evidence invalid")
                                        submission_api("m08cp02", "m08_cp02_python")
                                        for query in (
                                            "SELECT count(*) FROM v3.submission_aggregates s LEFT JOIN v3.enrollments e ON e.id=s.enrollment_id AND e.class_id=s.class_id AND e.user_id=s.owner_user_id LEFT JOIN v3.episodes p ON p.id=s.episode_id AND p.enrollment_id=s.enrollment_id LEFT JOIN v3.content_revisions r ON r.id=s.assignment_revision_id AND r.class_id=s.class_id AND r.activity_id=s.activity_id WHERE e.id IS NULL OR p.id IS NULL OR r.id IS NULL",
                                            "SELECT count(*) FROM v3.submission_revisions r LEFT JOIN v3.submission_attempts a ON a.id=r.attempt_id AND a.class_id=r.class_id AND a.aggregate_id=r.aggregate_id WHERE a.id IS NULL",
                                            "SELECT count(*) FROM v3.submission_revision_files f LEFT JOIN v3.file_assets a ON a.id=f.file_asset_id AND a.class_id=f.class_id WHERE a.id IS NULL",
                                            "SELECT count(*) FROM v3.audit a LEFT JOIN v3.outbox o ON o.event_id=a.id WHERE a.action LIKE 'submission.%' AND o.event_id IS NULL",
                                        ):
                                            rows=sql("qa_cp05",query).decode().splitlines()
                                            if rows and any(value!="0" for value in rows): raise ValueError("M08 submission reconciliation failed")
                                        result["m08_reconciliation"]="PASS"
                                        result["m08_cp03_backup"] = snapshot("qa_cp05", "pre_m08_cp03")
                                        write_new_json(directory / "m08-cp03-proof.json", {
                                            "run_id": run_id, "target": target,
                                            "created_at": datetime.now(timezone.utc).isoformat(), "restored": True,
                                            "archive_hash": sha256((directory / "pre_m08_cp03.dump").read_bytes()).hexdigest(),
                                        })
                                        slice_go("m08_cp03", "./internal/submissions", "TestSyntheticSubmissionHandinReceipt", race=os.name != "nt")
                                        submission_api("m08cp03", "m08_cp03_python")
                                        env["QA_CATALOGUE_SCOPE"] = "pre-m08-cp04"
                                        migration_output = command([migrate_binary], 45)
                                        env.pop("QA_CATALOGUE_SCOPE", None)
                                        migration_result = json.loads(migration_output)
                                        expected_replay.add("m08-cp03-1")
                                        if migration_result.get("status") != "APPLIED" or migration_result.get("applied") or set(migration_result.get("replayed",[])) != expected_replay:
                                            raise ValueError("M08-CP03 operator migration replay evidence invalid before CP04")
                                        result["m08_cp04_backup"] = snapshot("qa_cp05", "pre_m08_cp04")
                                        write_new_json(directory / "m08-cp04-proof.json", {
                                            "run_id": run_id, "target": target,
                                            "created_at": datetime.now(timezone.utc).isoformat(), "restored": True,
                                            "archive_hash": sha256((directory / "pre_m08_cp04.dump").read_bytes()).hexdigest(),
                                        })
                                        slice_go("m08_cp04", "./internal/submissions", "TestSyntheticLegacyChronologyLanding", race=os.name != "nt")
                                        env["QA_CATALOGUE_SCOPE"] = "pre-m11-cp01"
                                        migration_output = command([migrate_binary], 45)
                                        env.pop("QA_CATALOGUE_SCOPE", None)
                                        migration_result = json.loads(migration_output)
                                        expected_replay.add("m08-cp04-1")
                                        if migration_result.get("status") != "APPLIED" or migration_result.get("applied") or set(migration_result.get("replayed",[])) != expected_replay:
                                            raise ValueError("M08-CP04 operator migration replay evidence invalid")
                                        submission_api("m08cp04", "m08_cp04_python")
                                        for query in (
                                            "SELECT count(*) FROM v3.submission_receipts r LEFT JOIN v3.submission_attempts a ON a.id=r.attempt_id AND a.class_id=r.class_id AND a.aggregate_id=r.aggregate_id LEFT JOIN v3.submission_revisions s ON s.id=r.revision_id AND s.class_id=r.class_id AND s.attempt_id=r.attempt_id WHERE a.id IS NULL OR s.id IS NULL",
                                            "SELECT count(*) FROM v3.submission_receipts r JOIN v3.submission_attempts a ON a.id=r.attempt_id WHERE a.state='DRAFT'",
                                            "SELECT count(*) FROM v3.legacy_submission_evidence WHERE chronology_status<>'UNKNOWN'",
                                        ):
                                            rows=sql("qa_cp05",query).decode().splitlines()
                                            if rows and any(value!="0" for value in rows): raise ValueError("M08 receipt/chronology reconciliation failed")
                                        result["m08_receipt_reconciliation"]="PASS"
                                        if migration_journey:
                                            def m15_api(checkpoint):
                                                state = json.loads(sql("qa_cp05", "SELECT jsonb_object_agg(l.subject,jsonb_build_object('id',l.user_id,'version',l.version,'active',l.active,'site_role_version',r.version)) FROM v3identity.links l JOIN v3identity.site_roles r ON r.user_id=l.user_id"))
                                                write_new_json(directory / (checkpoint + "-state.json"), state)
                                                cover = directory / (checkpoint + "-api-cover")
                                                cover.mkdir()
                                                env.update(QA_CHECKPOINT=checkpoint, QA_IDENTITY_BINARY=str(identity_binary),
                                                    QA_ISSUER_BINARY=str(issuer_binary), QA_BRIDGE_PUBLIC=(directory / "issuer-public.txt").read_text(),
                                                    QA_CASE_RESULTS=str(directory / (checkpoint + "-api-cases.json")), GOCOVERDIR=str(cover))
                                                command([sys.executable, "-m", "pytest", "-p", "qa.plugin", "qa/tests/migration_api", "-q"], 120)
                                                cases = json.loads((directory / (checkpoint + "-api-cases.json")).read_text())
                                                expected_key = {"m15cp01":"m15_python", "m15cp02":"m15_python", "m15cp03":"m15_cp03_python", "m15cp04":"m15_cp04_python", "m15cp05":"m15_cp05_python"}[checkpoint]
                                                expected = {name for name in manifest[expected_key] if name.startswith("M15-CP" + checkpoint[-2:])}
                                                if cases["exit_status"] != 0 or set(cases["collected_ids"]) != expected or len(cases["cases"]) != len(expected) or any(case["status"] != "PASS" for case in cases["cases"]):
                                                    raise ValueError(checkpoint + " Playwright cases incomplete")
                                                result[checkpoint + "_api"] = {"status": "PASS_SYNTHETIC_ONLY", "cases": cases["cases"]}

                                            result["m15_cp01_backup"] = snapshot("qa_cp05", "pre_m15_cp01")
                                            write_new_json(directory / "m15-cp01-proof.json", {
                                                "run_id": run_id, "target": target, "created_at": datetime.now(timezone.utc).isoformat(),
                                                "restored": True, "archive_hash": sha256((directory / "pre_m15_cp01.dump").read_bytes()).hexdigest(),
                                            })
                                            slice_go("m15_cp01", "./internal/migration/foundation", "TestSyntheticPreflight", race=os.name != "nt")
                                            m15_api("m15cp01")
                                            result["m15_cp02_backup"] = snapshot("qa_cp05", "pre_m15_cp02")
                                            write_new_json(directory / "m15-cp02-proof.json", {
                                                "run_id": run_id, "target": target, "created_at": datetime.now(timezone.utc).isoformat(),
                                                "restored": True, "archive_hash": sha256((directory / "pre_m15_cp02.dump").read_bytes()).hexdigest(),
                                            })
                                            slice_go("m15_cp02", "./internal/migration/foundation", "TestSyntheticProvenance", race=os.name != "nt")
                                            m15_api("m15cp02")
                                            for query in (
                                                "SELECT count(*) FROM v3migration.landing_rows l LEFT JOIN v3migration.origin_keys o ON o.source_environment=l.source_environment AND o.source_instance_id=l.source_instance_id AND o.source_table=l.source_table AND o.source_pk=l.source_pk AND o.target_namespace=l.target_namespace WHERE o.source_pk IS NULL",
                                                "SELECT count(*) FROM v3migration.landing_rows l LEFT JOIN v3migration.scope_objects s ON s.snapshot_id=l.snapshot_id AND s.kind='TABLE' AND s.name=l.source_table WHERE s.name IS NULL",
                                                "SELECT count(*) FROM v3migration.scope_objects WHERE snapshot_id='cp02-landing' AND kind='TABLE' AND name IN ('users','session') AND expected_count<>1",
                                            ):
                                                if sql("qa_cp05", query) != b"0":
                                                    raise ValueError("M15 provenance reconciliation failed")
                                            result["m15_reconciliation"] = "PASS"
                                            if migration_backfill_journey:
                                                def m15_backup(checkpoint):
                                                    snapshot_name = "pre_" + checkpoint
                                                    result[checkpoint + "_backup"] = snapshot("qa_cp05", snapshot_name)
                                                    write_new_json(directory / (checkpoint.replace("_", "-") + "-proof.json"), {
                                                        "run_id": run_id, "target": target, "created_at": datetime.now(timezone.utc).isoformat(),
                                                        "restored": True, "archive_hash": sha256((directory / (snapshot_name + ".dump")).read_bytes()).hexdigest(),
                                                    })

                                                m15_backup("m15_cp03")
                                                slice_go("m15_cp03", "./internal/migration/foundation", "TestSyntheticMapping(Decisions|Conflict)", race=os.name != "nt")
                                                m15_api("m15cp03")
                                                m15_backup("m15_cp04")
                                                slice_go("m15_cp04", "./internal/migration/foundation", "TestSynthetic(AtomicBackfill|OwnerFenceReplay)", race=os.name != "nt")
                                                target_id = sql("qa_cp05", "SELECT target_id FROM v3migration.shadow_classes WHERE mapped_run_id='cp03-run-a' ORDER BY target_id LIMIT 1").decode()
                                                if len(target_id) != 32 or any(char not in "0123456789abcdef" for char in target_id):
                                                    raise ValueError("M15 shadow identity missing")
                                                write_new_json(directory / "m15cp04-shadow-id.json", {"target_id":target_id})
                                                m15_api("m15cp04")
                                                for query in (
                                                    "SELECT count(*) FROM v3migration.origin_mappings m LEFT JOIN v3migration.import_receipts r ON r.run_id=m.run_id AND r.source_table=m.source_table AND r.source_pk=m.source_pk AND r.source_version=m.source_version AND r.target_id=m.target_id WHERE r.run_id IS NULL",
                                                    "SELECT count(*) FROM v3migration.shadow_classes s LEFT JOIN v3migration.origin_mappings m ON m.target_id=s.target_id AND m.run_id=s.mapped_run_id WHERE m.target_id IS NULL",
                                                    "SELECT count(*) FROM v3migration.import_checkpoints WHERE applied_count<>planned_count OR backup_sha256 !~ '^[a-f0-9]{64}$'",
                                                    "SELECT count(*) FROM v3migration.shadow_classes WHERE publish_state<>'UNPUBLISHED'",
                                                ):
                                                    if sql("qa_cp05", query) != b"0":
                                                        raise ValueError("M15 backfill reconciliation failed")
                                                result["m15_backfill_reconciliation"] = "PASS"
                                                if migration_completion_journey:
                                                    m15_backup("m15_cp05")
                                                    slice_go("m15_cp05", "./internal/migration/foundation", "TestSyntheticReconciliation(Complete|RejectsIncomplete)", race=os.name != "nt")
                                                    m15_api("m15cp05")
                                                    result["m15_cp05_read_only"] = "PASS"

                                                    result["m11_cp01_backup"] = snapshot("qa_cp05", "pre_m11_cp01")
                                                    write_new_json(directory / "m11-cp01-proof.json", {
                                                        "run_id": run_id, "target": target, "created_at": datetime.now(timezone.utc).isoformat(),
                                                        "restored": True, "archive_hash": sha256((directory / "pre_m11_cp01.dump").read_bytes()).hexdigest(),
                                                    })
                                                    slice_go("m11_cp01", "./internal/attendance", "TestSynthetic(ClassMeetingCalendar|MeetingExceptionBoundary)", race=os.name != "nt")
                                                    state = json.loads(sql("qa_cp05", "SELECT jsonb_object_agg(l.subject,jsonb_build_object('id',l.user_id,'version',l.version,'active',l.active,'site_role_version',r.version)) FROM v3identity.links l JOIN v3identity.site_roles r ON r.user_id=l.user_id"))
                                                    write_new_json(directory / "m11cp01-state.json", state)
                                                    cover = directory / "m11cp01-api-cover"
                                                    cover.mkdir()
                                                    env.update(QA_CHECKPOINT="m11cp01", QA_IDENTITY_BINARY=str(identity_binary),
                                                        QA_ISSUER_BINARY=str(issuer_binary), QA_BRIDGE_PUBLIC=(directory / "issuer-public.txt").read_text(),
                                                        QA_CASE_RESULTS=str(directory / "m11cp01-api-cases.json"), GOCOVERDIR=str(cover))
                                                    command([sys.executable, "-m", "pytest", "-p", "qa.plugin", "qa/tests/attendance_api", "-q"], 150)
                                                    cases = json.loads((directory / "m11cp01-api-cases.json").read_text())
                                                    expected = set(manifest["m11_cp01_python"])
                                                    if cases["exit_status"] != 0 or set(cases["collected_ids"]) != expected or len(cases["cases"]) != len(expected) or any(c["status"] != "PASS" for c in cases["cases"]):
                                                        raise ValueError("M11-CP01 Playwright cases incomplete")
                                                    result["m11cp01_api"] = {"status": "PASS_SYNTHETIC_ONLY", "cases": cases["cases"]}
                                                    for query in (
                                                        "SELECT count(*) FROM v3.meeting_exceptions e LEFT JOIN v3.class_meetings m ON m.id=e.meeting_id AND m.class_id=e.class_id WHERE m.id IS NULL",
                                                        "SELECT count(*) FROM v3.class_meetings m WHERE m.local_date<>(m.starts_at AT TIME ZONE m.timezone)::date",
                                                        "SELECT count(*) FROM v3.meeting_exceptions e WHERE e.revision<> (SELECT count(*) FROM v3.meeting_exceptions x WHERE x.meeting_id=e.meeting_id AND x.revision<=e.revision)",
                                                    ):
                                                        if sql("qa_cp05", query) != b"0":
                                                            raise ValueError("M11 meeting persistence reconciliation failed")
                                                    result["m11_cp01_reconciliation"] = "PASS"
                                                    if logbook_period_journey:
                                                        result["m12_cp01_backup"] = snapshot("qa_cp05", "pre_m12_cp01")
                                                        write_new_json(directory / "m12-cp01-proof.json", {
                                                            "run_id": run_id, "target": target, "created_at": datetime.now(timezone.utc).isoformat(),
                                                            "restored": True, "archive_hash": sha256((directory / "pre_m12_cp01.dump").read_bytes()).hexdigest(),
                                                        })
                                                        slice_go("m12_cp01", "./internal/logbook", "TestSynthetic(ClassPeriodLifecycle|PeriodBoundary)", race=os.name != "nt")
                                                        state = json.loads(sql("qa_cp05", "SELECT jsonb_object_agg(l.subject,jsonb_build_object('id',l.user_id,'version',l.version,'active',l.active,'site_role_version',r.version)) FROM v3identity.links l JOIN v3identity.site_roles r ON r.user_id=l.user_id"))
                                                        write_new_json(directory / "m12cp01-state.json", state)
                                                        cover = directory / "m12cp01-api-cover"
                                                        cover.mkdir()
                                                        env.update(QA_CHECKPOINT="m12cp01", QA_IDENTITY_BINARY=str(identity_binary),
                                                            QA_ISSUER_BINARY=str(issuer_binary), QA_BRIDGE_PUBLIC=(directory / "issuer-public.txt").read_text(),
                                                            QA_CASE_RESULTS=str(directory / "m12cp01-api-cases.json"), GOCOVERDIR=str(cover))
                                                        command([sys.executable, "-m", "pytest", "-p", "qa.plugin", "qa/tests/logbook_api", "-q"], 150)
                                                        cases = json.loads((directory / "m12cp01-api-cases.json").read_text())
                                                        expected = set(manifest["m12_cp01_python"])
                                                        if cases["exit_status"] != 0 or set(cases["collected_ids"]) != expected or len(cases["cases"]) != len(expected) or any(c["status"] != "PASS" for c in cases["cases"]):
                                                            raise ValueError("M12-CP01 Playwright cases incomplete")
                                                        result["m12cp01_api"] = {"status": "PASS_SYNTHETIC_ONLY", "cases": cases["cases"]}
                                                        for query in (
                                                            "SELECT count(*) FROM v3.logbook_periods p LEFT JOIN v3.logbook_period_sets s ON s.id=p.set_id AND s.class_id=p.class_id WHERE s.id IS NULL",
                                                            "SELECT count(*) FROM v3.logbook_period_publications p LEFT JOIN v3.logbook_period_sets s ON s.id=p.set_id AND s.class_id=p.class_id WHERE s.id IS NULL",
                                                            "SELECT count(*) FROM v3.logbook_period_publications p WHERE p.revision<>(SELECT count(*) FROM v3.logbook_period_publications x WHERE x.class_id=p.class_id AND x.revision<=p.revision)",
                                                        ):
                                                            if sql("qa_cp05", query) != b"0":
                                                                raise ValueError("M12 period persistence reconciliation failed")
                                                        result["m12_cp01_reconciliation"] = "PASS"
                                                        if logbook_lifecycle_journey:
                                                            result["m12_cp02_backup"] = snapshot("qa_cp05", "pre_m12_cp02")
                                                            write_new_json(directory / "m12-cp02-proof.json", {
                                                                "run_id": run_id, "target": target, "created_at": datetime.now(timezone.utc).isoformat(),
                                                                "restored": True, "archive_hash": sha256((directory / "pre_m12_cp02.dump").read_bytes()).hexdigest(),
                                                            })
                                                            slice_go("m12_cp02", "./internal/logbook", "TestSyntheticLogbookEntry(Lifecycle|Boundary)", race=os.name != "nt")
                                                            state = json.loads(sql("qa_cp05", "SELECT jsonb_object_agg(l.subject,jsonb_build_object('id',l.user_id,'version',l.version,'active',l.active,'site_role_version',r.version)) FROM v3identity.links l JOIN v3identity.site_roles r ON r.user_id=l.user_id"))
                                                            write_new_json(directory / "m12cp02-state.json", state)
                                                            cover = directory / "m12cp02-api-cover"
                                                            cover.mkdir()
                                                            env.update(QA_CHECKPOINT="m12cp02", QA_IDENTITY_BINARY=str(identity_binary),
                                                                QA_ISSUER_BINARY=str(issuer_binary), QA_BRIDGE_PUBLIC=(directory / "issuer-public.txt").read_text(),
                                                                QA_CASE_RESULTS=str(directory / "m12cp02-api-cases.json"), GOCOVERDIR=str(cover))
                                                            command([sys.executable, "-m", "pytest", "-p", "qa.plugin", "qa/tests/logbook_api", "-q"], 180)
                                                            cases = json.loads((directory / "m12cp02-api-cases.json").read_text())
                                                            expected = set(manifest["m12_cp02_python"])
                                                            if cases["exit_status"] != 0 or set(cases["collected_ids"]) != expected or len(cases["cases"]) != len(expected) or any(c["status"] != "PASS" for c in cases["cases"]):
                                                                raise ValueError("M12-CP02 Playwright cases incomplete")
                                                            profile = directory / "m12cp02-api-coverage.out"
                                                            command(["go", "tool", "covdata", "textfmt", "-i=" + str(cover), "-o=" + str(profile)])
                                                            require_coverage(profile)
                                                            if "lmsv3/internal/logbook/" not in profile.read_text(encoding="utf-8"):
                                                                raise ValueError("M12-CP02 logbook coverage missing")
                                                            result["m12cp02_api"] = {"status": "PASS_SYNTHETIC_ONLY", "cases": cases["cases"],
                                                                "instrumented_go_coverage": command(["go", "tool", "cover", "-func=" + str(profile)]).decode().strip()}
                                                            for query in (
                                                                "SELECT count(*) FROM v3.logbook_entries e LEFT JOIN v3.logbook_periods p ON p.id=e.period_id AND p.class_id=e.class_id LEFT JOIN v3.enrollments n ON n.id=e.enrollment_id AND n.class_id=e.class_id AND n.user_id=e.owner_user_id LEFT JOIN v3.episodes ep ON ep.id=e.episode_id AND ep.enrollment_id=e.enrollment_id WHERE p.id IS NULL OR n.id IS NULL OR ep.id IS NULL",
                                                                "SELECT count(*) FROM v3.logbook_entry_revisions r WHERE r.revision_no<>(SELECT count(*) FROM v3.logbook_entry_revisions x WHERE x.entry_id=r.entry_id AND x.revision_no<=r.revision_no)",
                                                                "SELECT count(*) FROM v3.logbook_entry_events e WHERE e.version<>(SELECT count(*) FROM v3.logbook_entry_events x WHERE x.entry_id=e.entry_id AND x.version<=e.version)",
                                                                "SELECT count(*) FROM v3.logbook_entry_events WHERE state IN ('REVISION_REQUIRED','ACCEPTED')",
                                                            ):
                                                                if sql("qa_cp05", query) != b"0":
                                                                    raise ValueError("M12 entry persistence reconciliation failed")
                                                            result["m12_cp02_reconciliation"] = "PASS"

                                                            result["m12_cp03_backup"] = snapshot("qa_cp05", "pre_m12_cp03")
                                                            write_new_json(directory / "m12-cp03-proof.json", {
                                                                "run_id": run_id, "target": target, "created_at": datetime.now(timezone.utc).isoformat(),
                                                                "restored": True, "archive_hash": sha256((directory / "pre_m12_cp03.dump").read_bytes()).hexdigest(),
                                                            })
                                                            slice_go("m12_cp03", "./internal/logbook", "TestSyntheticLogbookReview(Lifecycle|Boundary)", race=os.name != "nt")
                                                            state = json.loads(sql("qa_cp05", "SELECT jsonb_object_agg(l.subject,jsonb_build_object('id',l.user_id,'version',l.version,'active',l.active,'site_role_version',r.version)) FROM v3identity.links l JOIN v3identity.site_roles r ON r.user_id=l.user_id"))
                                                            write_new_json(directory / "m12cp03-state.json", state)
                                                            cover = directory / "m12cp03-api-cover"
                                                            cover.mkdir()
                                                            env.update(QA_CHECKPOINT="m12cp03", QA_IDENTITY_BINARY=str(identity_binary),
                                                                QA_ISSUER_BINARY=str(issuer_binary), QA_BRIDGE_PUBLIC=(directory / "issuer-public.txt").read_text(),
                                                                QA_CASE_RESULTS=str(directory / "m12cp03-api-cases.json"), GOCOVERDIR=str(cover))
                                                            command([sys.executable, "-m", "pytest", "-p", "qa.plugin", "qa/tests/logbook_api", "-q"], 180)
                                                            cases = json.loads((directory / "m12cp03-api-cases.json").read_text())
                                                            expected = set(manifest["m12_cp03_python"])
                                                            if cases["exit_status"] != 0 or set(cases["collected_ids"]) != expected or len(cases["cases"]) != len(expected) or any(c["status"] != "PASS" for c in cases["cases"]):
                                                                raise ValueError("M12-CP03 Playwright cases incomplete")
                                                            profile = directory / "m12cp03-api-coverage.out"
                                                            command(["go", "tool", "covdata", "textfmt", "-i=" + str(cover), "-o=" + str(profile)])
                                                            require_coverage(profile)
                                                            if "lmsv3/internal/logbook/" not in profile.read_text(encoding="utf-8"):
                                                                raise ValueError("M12-CP03 logbook coverage missing")
                                                            result["m12cp03_api"] = {"status": "PASS_SYNTHETIC_ONLY", "cases": cases["cases"],
                                                                "instrumented_go_coverage": command(["go", "tool", "cover", "-func=" + str(profile)]).decode().strip()}
                                                            for query in (
                                                                "SELECT count(*) FROM v3.logbook_entry_events e LEFT JOIN v3.logbook_entry_revisions r ON r.id=e.revision_id AND r.entry_id=e.entry_id AND r.class_id=e.class_id WHERE r.id IS NULL",
                                                                "SELECT count(*) FROM v3.logbook_entry_events e LEFT JOIN v3.mentor_assignments m ON m.id=e.mentor_assignment_id AND m.class_id=e.class_id WHERE e.state IN ('REVISION_REQUIRED','ACCEPTED') AND e.mentor_assignment_id IS NOT NULL AND m.id IS NULL",
                                                                "SELECT count(*) FROM v3.logbook_entry_events e WHERE e.version<>(SELECT count(*) FROM v3.logbook_entry_events x WHERE x.entry_id=e.entry_id AND x.version<=e.version)",
                                                            ):
                                                                if sql("qa_cp05", query) != b"0":
                                                                    raise ValueError("M12 review persistence reconciliation failed")
                                                            result["m12_cp03_reconciliation"] = "PASS"
                                                            if notification_journey:
                                                                def m13_proof(checkpoint):
                                                                    snapshot_name = "pre_" + checkpoint
                                                                    result[checkpoint + "_backup"] = snapshot("qa_cp05", snapshot_name)
                                                                    write_new_json(directory / (checkpoint.replace("_", "-") + "-proof.json"), {
                                                                        "run_id": run_id, "target": target, "created_at": datetime.now(timezone.utc).isoformat(),
                                                                        "restored": True, "archive_hash": sha256((directory / (snapshot_name + ".dump")).read_bytes()).hexdigest(),
                                                                    })

                                                                def m13_api(checkpoint, expected_key):
                                                                    state = json.loads(sql("qa_cp05", "SELECT jsonb_object_agg(l.subject,jsonb_build_object('id',l.user_id,'version',l.version,'active',l.active,'site_role_version',r.version)) FROM v3identity.links l JOIN v3identity.site_roles r ON r.user_id=l.user_id"))
                                                                    write_new_json(directory / (checkpoint + "-state.json"), state)
                                                                    cover = directory / (checkpoint + "-api-cover")
                                                                    cover.mkdir()
                                                                    env.update(QA_CHECKPOINT=checkpoint, QA_IDENTITY_BINARY=str(identity_binary),
                                                                        QA_ISSUER_BINARY=str(issuer_binary), QA_BRIDGE_PUBLIC=(directory / "issuer-public.txt").read_text(),
                                                                        QA_CASE_RESULTS=str(directory / (checkpoint + "-api-cases.json")), GOCOVERDIR=str(cover))
                                                                    command([sys.executable, "-m", "pytest", "-p", "qa.plugin", "qa/tests/notification_api", "-q"], 180)
                                                                    cases = json.loads((directory / (checkpoint + "-api-cases.json")).read_text())
                                                                    expected = set(manifest[expected_key])
                                                                    if cases["exit_status"] != 0 or set(cases["collected_ids"]) != expected or len(cases["cases"]) != len(expected) or any(c["status"] != "PASS" for c in cases["cases"]):
                                                                        raise ValueError(checkpoint + " Playwright cases incomplete")
                                                                    profile = directory / (checkpoint + "-api-coverage.out")
                                                                    command(["go", "tool", "covdata", "textfmt", "-i=" + str(cover), "-o=" + str(profile)])
                                                                    if "lmsv3/internal/notifications/" not in profile.read_text(encoding="utf-8") or not any(line.endswith(" 1") for line in profile.read_text(encoding="utf-8").splitlines()):
                                                                        raise ValueError(checkpoint + " notification coverage missing")
                                                                    result[checkpoint + "_api"] = {"status":"PASS_SYNTHETIC_ONLY", "cases":cases["cases"],
                                                                        "instrumented_go_coverage":command(["go","tool","cover","-func="+str(profile)]).decode().strip()}

                                                                m13_proof("m13_cp01")
                                                                slice_go("m13_cp01", "./internal/notifications", "TestSyntheticDurableDelivery(Lifecycle|Boundary)", race=os.name != "nt")
                                                                m13_api("m13cp01", "m13_cp01_python")
                                                                for query in (
                                                                    "SELECT count(*) FROM v3ops.outbox_provider_receipts r LEFT JOIN v3.outbox o ON o.event_id=r.event_id WHERE o.event_id IS NULL",
                                                                    "SELECT count(*) FROM v3ops.outbox_provider_receipts r WHERE r.attempt_number>(SELECT attempts FROM v3.outbox o WHERE o.event_id=r.event_id)",
                                                                ):
                                                                    if sql("qa_cp05", query) != b"0": raise ValueError("M13 durable-delivery reconciliation failed")
                                                                result["m13_cp01_reconciliation"] = "PASS"

                                                                m13_proof("m13_cp02")
                                                                slice_go("m13_cp02", "./internal/notifications", "TestSyntheticNotification(Lifecycle|Boundary)", race=os.name != "nt")
                                                                m13_api("m13cp02", "m13_cp02_python")
                                                                for query in (
                                                                    "SELECT count(*) FROM v3.notification_deliveries WHERE source='HISTORICAL_IMPORT' AND (status<>'SUPPRESSED' OR suppression_code<>'HISTORICAL_IMPORT')",
                                                                    "SELECT count(*) FROM v3.notification_preference_events p WHERE p.version<>(SELECT count(*) FROM v3.notification_preference_events x WHERE x.user_id=p.user_id AND x.channel=p.channel AND x.version<=p.version)",
                                                                    "SELECT count(*) FROM v3ops.provider_binding_events WHERE secret_ref !~ '^secretref://'",
                                                                ):
                                                                    if sql("qa_cp05", query) != b"0": raise ValueError("M13 notification reconciliation failed")
                                                                result["m13_cp02_reconciliation"] = "PASS"
                                                                if gradebook_journey:
                                                                    def m09_api(checkpoint, expected_key):
                                                                        state = json.loads(sql("qa_cp05", "SELECT jsonb_object_agg(l.subject,jsonb_build_object('id',l.user_id,'version',l.version,'active',l.active,'site_role_version',r.version)) FROM v3identity.links l JOIN v3identity.site_roles r ON r.user_id=l.user_id"))
                                                                        write_new_json(directory / (checkpoint + "-state.json"), state)
                                                                        cover = directory / (checkpoint + "-api-cover")
                                                                        cover.mkdir()
                                                                        env.update(QA_CHECKPOINT=checkpoint, QA_IDENTITY_BINARY=str(identity_binary),
                                                                            QA_ISSUER_BINARY=str(issuer_binary), QA_BRIDGE_PUBLIC=(directory / "issuer-public.txt").read_text(),
                                                                            QA_CASE_RESULTS=str(directory / (checkpoint + "-api-cases.json")), GOCOVERDIR=str(cover))
                                                                        command([sys.executable, "-m", "pytest", "-p", "qa.plugin", "qa/tests/gradebook_api", "-q"], 180)
                                                                        cases = json.loads((directory / (checkpoint + "-api-cases.json")).read_text())
                                                                        expected = set(manifest[expected_key])
                                                                        if cases["exit_status"] != 0 or set(cases["collected_ids"]) != expected or len(cases["cases"]) != len(expected) or any(c["status"] != "PASS" for c in cases["cases"]):
                                                                            raise ValueError(checkpoint + " Playwright cases incomplete")
                                                                        profile = directory / (checkpoint + "-api-coverage.out")
                                                                        command(["go", "tool", "covdata", "textfmt", "-i=" + str(cover), "-o=" + str(profile)])
                                                                        if "lmsv3/internal/gradebook/" not in profile.read_text(encoding="utf-8") or not any(line.endswith(" 1") for line in profile.read_text(encoding="utf-8").splitlines()):
                                                                            raise ValueError(checkpoint + " gradebook coverage missing")
                                                                        result[checkpoint + "_api"] = {"status":"PASS_SYNTHETIC_ONLY", "cases":cases["cases"],
                                                                            "instrumented_go_coverage":command(["go","tool","cover","-func="+str(profile)]).decode().strip()}

                                                                    m13_proof("m09_cp01")
                                                                    slice_go("m09_cp01", "./internal/gradebook", "TestSyntheticGradeDefinition(Lifecycle|Boundary)", race=os.name != "nt")
                                                                    m09_api("m09cp01", "m09_cp01_python")
                                                                    for query in (
                                                                        "SELECT count(*) FROM v3.grade_definition_versions d LEFT JOIN v3.grade_items i ON i.id=d.grade_item_id AND i.class_id=d.class_id LEFT JOIN v3.grade_scales s ON s.id=d.scale_id AND s.class_id=d.class_id WHERE i.id IS NULL OR s.id IS NULL",
                                                                        "SELECT count(*) FROM v3.grade_definition_publications p WHERE p.revision<>(SELECT count(*) FROM v3.grade_definition_publications x WHERE x.grade_item_id=p.grade_item_id AND x.revision<=p.revision)",
                                                                        "SELECT count(*) FROM v3.grade_definition_publications p WHERE (SELECT coalesce(sum(c.weight_bps),0) FROM v3.rubric_criteria c WHERE c.definition_version_id=p.definition_version_id AND NOT EXISTS(SELECT 1 FROM v3.rubric_criteria child WHERE child.definition_version_id=c.definition_version_id AND child.parent_criterion_id=c.id))<>10000",
                                                                    ):
                                                                        if sql("qa_cp05", query) != b"0": raise ValueError("M09 definition reconciliation failed")
                                                                    result["m09_cp01_reconciliation"] = "PASS"

                                                                    m13_proof("m09_cp02")
                                                                    slice_go("m09_cp02", "./internal/gradebook", "TestSyntheticScopedGrade(Lifecycle|Boundary)", race=os.name != "nt")
                                                                    m09_api("m09cp02", "m09_cp02_python")
                                                                    for query in (
                                                                        "SELECT count(*) FROM v3.grade_current c LEFT JOIN v3.grade_revisions r ON r.id=c.revision_id AND r.class_id=c.class_id AND r.grade_item_id=c.grade_item_id AND r.aggregate_id=c.aggregate_id WHERE r.id IS NULL OR r.version<>c.version",
                                                                        "SELECT count(*) FROM v3.grade_revisions r LEFT JOIN v3.submission_receipts s ON s.aggregate_id=r.aggregate_id AND s.attempt_id=r.attempt_id AND s.revision_id=r.submission_revision_id WHERE s.id IS NULL",
                                                                        "SELECT count(*) FROM v3.grade_revisions r WHERE r.version<>(SELECT count(*) FROM v3.grade_revisions x WHERE x.grade_item_id=r.grade_item_id AND x.aggregate_id=r.aggregate_id AND x.version<=r.version)",
                                                                    ):
                                                                        if sql("qa_cp05", query) != b"0": raise ValueError("M09 scoped-grade reconciliation failed")
                                                                    if sql("qa_cp05", "SELECT bool_or(has_table_privilege('qa_runtime',format('%I.%I',schemaname,tablename),'DELETE')) FROM pg_tables WHERE schemaname='v3' AND tablename LIKE 'grade%'") != b"f":
                                                                        raise ValueError("M09 runtime delete privilege detected")
                                                                    result["m09_cp02_reconciliation"] = "PASS"

                                                                    m13_proof("m09_cp03")
                                                                    slice_go("m09_cp03", "./internal/gradebook", "TestSynthetic(Import|Release)(Lifecycle|Boundary)", race=os.name != "nt")
                                                                    m09_api("m09cp03", "m09_cp03_python")
                                                                    for query in (
                                                                        "SELECT count(*) FROM v3.grade_revisions r LEFT JOIN v3.grade_import_previews p ON p.id=r.source_preview_id WHERE r.source_preview_id IS NOT NULL AND p.id IS NULL",
                                                                        "SELECT count(*) FROM v3.grade_import_confirms c LEFT JOIN v3.grade_import_previews p ON p.id=c.preview_id WHERE p.id IS NULL OR p.actor_id<>c.actor_id",
                                                                        "SELECT count(*) FROM v3.grade_import_confirms c JOIN v3.grade_import_previews p ON p.id=c.preview_id WHERE jsonb_array_length(p.entries)<>(SELECT count(*) FROM v3.grade_revisions r WHERE r.source_preview_id=p.id)",
                                                                        "SELECT count(*) FROM v3.grade_revisions r LEFT JOIN v3.grade_import_confirms c ON c.preview_id=r.source_preview_id WHERE r.source_preview_id IS NOT NULL AND c.preview_id IS NULL",
                                                                        "SELECT count(*) FROM v3.grade_releases l JOIN v3.grade_revisions r ON r.id=l.revision_id WHERE l.learner_user_id<>r.learner_user_id OR l.grade_version<>r.version OR l.class_id<>r.class_id OR l.grade_item_id<>r.grade_item_id OR l.aggregate_id<>r.aggregate_id",
                                                                    ):
                                                                        if sql("qa_cp05", query) != b"0": raise ValueError("M09 import/release reconciliation failed")
                                                                    if sql("qa_cp05", "SELECT bool_or(has_table_privilege('qa_runtime',format('%I.%I',schemaname,tablename),'DELETE')) FROM pg_tables WHERE schemaname='v3' AND tablename LIKE 'grade%'") != b"f":
                                                                        raise ValueError("M09 CP03 runtime delete privilege detected")
                                                                    result["m09_cp03_reconciliation"] = "PASS"

                                                                    m13_proof("m09_cp04")
                                                                    slice_go("m09_cp04", "./internal/gradebook", "TestSyntheticHistorical(Semantics|ScopedMatch|Boundary)", race=os.name != "nt")
                                                                    m09_api("m09cp04", "m09_cp04_python")
                                                                    for query in (
                                                                        "SELECT count(*) FROM v3.legacy_grade_reconciliation_runs x WHERE jsonb_array_length(x.response->'observations')<>(SELECT count(*) FROM v3.legacy_grade_observations o WHERE o.run_id=x.id)",
                                                                        "SELECT count(*) FROM v3.legacy_grade_observations o LEFT JOIN v3.grade_releases l ON l.id=o.canonical_release_id LEFT JOIN v3.grade_revisions r ON r.id=l.revision_id WHERE o.canonical_release_id IS NOT NULL AND (l.id IS NULL OR l.class_id<>o.candidate_class_id OR l.grade_item_id<>o.candidate_grade_item_id OR l.aggregate_id<>o.candidate_aggregate_id OR r.value_milli<>o.canonical_score_milli)",
                                                                        "SELECT count(*) FROM v3.legacy_grade_observations WHERE (origin='LEGACY_MINIMUM_65' AND (observed_score_milli<>65000 OR outcome<>'POLICY_DERIVED_HOLD')) OR (origin='AI_FALLBACK_75' AND (observed_score_milli<>75000 OR outcome<>'AI_UNREVIEWED_HOLD'))",
                                                                        "SELECT count(*) FROM v3.legacy_grade_observations WHERE source_scope='PROGRAM_GLOBAL' AND (outcome<>'AMBIGUOUS_HOLD' OR candidate_class_id IS NOT NULL OR canonical_release_id IS NOT NULL)",
                                                                        "SELECT count(*) FROM v3.legacy_grade_observations WHERE outcome='MATCHED' AND (raw_state<>'VALUE' OR raw_score_milli IS DISTINCT FROM canonical_score_milli OR observed_score_milli IS DISTINCT FROM canonical_score_milli)",
                                                                    ):
                                                                        if sql("qa_cp05", query) != b"0": raise ValueError("M09 historical reconciliation failed")
                                                                    if sql("qa_cp05", "SELECT bool_or(has_table_privilege('qa_runtime',format('%I.%I',schemaname,tablename),'DELETE') OR has_table_privilege('qa_runtime',format('%I.%I',schemaname,tablename),'UPDATE')) FROM pg_tables WHERE schemaname='v3' AND tablename LIKE 'legacy_grade%'") != b"f":
                                                                        raise ValueError("M09 CP04 runtime mutation privilege detected")
                                                                    result["m09_cp04_reconciliation"] = "PASS"
                                                                    if completion_journey:
                                                                        m13_proof("m10_cp01")
                                                                        slice_go("m10_cp01", "./internal/completion", "TestSyntheticCompletion(RuleSnapshot|OutcomeSeparation|Boundary)", race=os.name != "nt")
                                                                        state = json.loads(sql("qa_cp05", "SELECT jsonb_object_agg(l.subject,jsonb_build_object('id',l.user_id,'version',l.version,'active',l.active,'site_role_version',r.version)) FROM v3identity.links l JOIN v3identity.site_roles r ON r.user_id=l.user_id"))
                                                                        write_new_json(directory / "m10cp01-state.json", state)
                                                                        cover = directory / "m10cp01-api-cover"
                                                                        cover.mkdir()
                                                                        env.update(QA_CHECKPOINT="m10cp01", QA_IDENTITY_BINARY=str(identity_binary),
                                                                            QA_ISSUER_BINARY=str(issuer_binary), QA_BRIDGE_PUBLIC=(directory / "issuer-public.txt").read_text(),
                                                                            QA_CASE_RESULTS=str(directory / "m10cp01-api-cases.json"), GOCOVERDIR=str(cover))
                                                                        command([sys.executable, "-m", "pytest", "-p", "qa.plugin", "qa/tests/completion_api", "-q"], 210)
                                                                        cases = json.loads((directory / "m10cp01-api-cases.json").read_text())
                                                                        expected = set(manifest["m10_cp01_python"])
                                                                        if cases["exit_status"] != 0 or set(cases["collected_ids"]) != expected or len(cases["cases"]) != len(expected) or any(c["status"] != "PASS" for c in cases["cases"]):
                                                                            raise ValueError("m10cp01 Playwright cases incomplete")
                                                                        profile = directory / "m10cp01-api-coverage.out"
                                                                        command(["go", "tool", "covdata", "textfmt", "-i=" + str(cover), "-o=" + str(profile)])
                                                                        if not any(line.startswith("lmsv3/internal/completion/") and int(line.rsplit(" ", 1)[1]) > 0 for line in profile.read_text(encoding="utf-8").splitlines()[1:]):
                                                                            raise ValueError("m10cp01 completion coverage missing")
                                                                        result["m10cp01_api"] = {"status":"PASS_SYNTHETIC_ONLY", "cases":cases["cases"],
                                                                            "instrumented_go_coverage":command(["go","tool","cover","-func="+str(profile)]).decode().strip()}
                                                                        for query in (
                                                                            "SELECT count(*) FROM v3.learning_evidence e LEFT JOIN v3.completion_rule_versions r ON r.id=e.rule_version_id AND r.class_id=e.class_id AND r.activity_id=e.activity_id AND r.content_revision_id=e.content_revision_id WHERE r.id IS NULL",
                                                                            "SELECT count(*) FROM v3.activity_completion_outcomes o LEFT JOIN v3.learning_evidence e ON e.id=o.evidence_id AND e.class_id=o.class_id AND e.activity_id=o.activity_id AND e.rule_version_id=o.rule_version_id AND e.learner_user_id=o.learner_user_id WHERE o.state='COMPLETED' AND e.id IS NULL",
                                                                            "SELECT count(*) FROM v3.completion_rule_versions r WHERE r.version<>(SELECT count(*) FROM v3.completion_rule_versions x WHERE x.activity_id=r.activity_id AND x.version<=r.version)",
                                                                            "SELECT count(*) FROM v3.audit a LEFT JOIN v3.outbox o ON o.event_id=a.id WHERE a.action LIKE 'completion.%' AND o.event_id IS NULL",
                                                                        ):
                                                                            if sql("qa_cp05", query) != b"0": raise ValueError("M10 completion reconciliation failed")
                                                                        if sql("qa_cp05", "SELECT bool_or(has_table_privilege('qa_runtime',format('%I.%I',schemaname,tablename),'DELETE') OR has_table_privilege('qa_runtime',format('%I.%I',schemaname,tablename),'UPDATE')) FROM pg_tables WHERE schemaname='v3' AND tablename IN ('completion_rule_versions','learning_evidence','activity_completion_outcomes','completion_commands')") != b"f":
                                                                            raise ValueError("M10 runtime mutation privilege detected")
                                                                        result["m10_cp01_reconciliation"] = "PASS"
                                                                        if completion_correction_journey:
                                                                            m13_proof("m10_cp02")
                                                                            slice_go("m10_cp02", "./internal/completion", "TestSyntheticCompletion(ProgressProjection|OverrideReopen|CorrectionBoundary)", race=os.name != "nt")
                                                                            state = json.loads(sql("qa_cp05", "SELECT jsonb_object_agg(l.subject,jsonb_build_object('id',l.user_id,'version',l.version,'active',l.active,'site_role_version',r.version)) FROM v3identity.links l JOIN v3identity.site_roles r ON r.user_id=l.user_id"))
                                                                            write_new_json(directory / "m10cp02-state.json", state)
                                                                            cover = directory / "m10cp02-api-cover"
                                                                            cover.mkdir()
                                                                            env.update(QA_CHECKPOINT="m10cp02", QA_IDENTITY_BINARY=str(identity_binary),
                                                                                QA_ISSUER_BINARY=str(issuer_binary), QA_BRIDGE_PUBLIC=(directory / "issuer-public.txt").read_text(),
                                                                                QA_CASE_RESULTS=str(directory / "m10cp02-api-cases.json"), GOCOVERDIR=str(cover))
                                                                            command([sys.executable, "-m", "pytest", "-p", "qa.plugin", "qa/tests/completion_api", "-q"], 210)
                                                                            cases = json.loads((directory / "m10cp02-api-cases.json").read_text())
                                                                            expected = set(manifest["m10_cp02_python"])
                                                                            if cases["exit_status"] != 0 or set(cases["collected_ids"]) != expected or len(cases["cases"]) != len(expected) or any(c["status"] != "PASS" for c in cases["cases"]):
                                                                                raise ValueError("m10cp02 Playwright cases incomplete")
                                                                            profile = directory / "m10cp02-api-coverage.out"
                                                                            command(["go", "tool", "covdata", "textfmt", "-i=" + str(cover), "-o=" + str(profile)])
                                                                            if not any(line.startswith("lmsv3/internal/completion/progress.go:") and int(line.rsplit(" ", 1)[1]) > 0 for line in profile.read_text(encoding="utf-8").splitlines()[1:]):
                                                                                raise ValueError("m10cp02 progress coverage missing")
                                                                            result["m10cp02_api"] = {"status":"PASS_SYNTHETIC_ONLY", "cases":cases["cases"],
                                                                                "instrumented_go_coverage":command(["go","tool","cover","-func="+str(profile)]).decode().strip()}
                                                                            for query in (
                                                                                "SELECT count(*) FROM v3.completion_corrections c LEFT JOIN v3.completion_rule_versions r ON r.id=c.rule_version_id AND r.class_id=c.class_id AND r.activity_id=c.activity_id AND r.content_revision_id=c.content_revision_id WHERE r.id IS NULL",
                                                                                "SELECT count(*) FROM v3.completion_corrections c WHERE c.version<>c.base_outcome_count+(SELECT count(*) FROM v3.completion_corrections x WHERE x.rule_version_id=c.rule_version_id AND x.episode_id=c.episode_id AND x.version<=c.version)",
                                                                                "SELECT count(*) FROM v3.audit a LEFT JOIN v3.outbox o ON o.event_id=a.id WHERE a.action='completion.correct' AND o.event_id IS NULL",
                                                                            ):
                                                                                if sql("qa_cp05", query) != b"0": raise ValueError("M10 correction reconciliation failed")
                                                                            if sql("qa_cp05", "SELECT bool_or(has_table_privilege('qa_runtime',format('%I.%I',schemaname,tablename),'DELETE') OR has_table_privilege('qa_runtime',format('%I.%I',schemaname,tablename),'UPDATE')) FROM pg_tables WHERE schemaname='v3' AND tablename IN ('completion_corrections','completion_correction_commands')") != b"f":
                                                                                raise ValueError("M10 correction history mutation privilege detected")
                                                                            result["m10_cp02_reconciliation"] = "PASS"
                                                                            if credential_journey:
                                                                                m13_proof("m10_cp03")
                                                                                slice_go("m10_cp03", "./internal/completion", "TestSynthetic(TranscriptProvenance|CredentialLifecycle|CredentialBoundary)", race=os.name != "nt")
                                                                                state = json.loads(sql("qa_cp05", "SELECT jsonb_object_agg(l.subject,jsonb_build_object('id',l.user_id,'version',l.version,'active',l.active,'site_role_version',r.version)) FROM v3identity.links l JOIN v3identity.site_roles r ON r.user_id=l.user_id"))
                                                                                write_new_json(directory / "m10cp03-state.json", state)
                                                                                cover = directory / "m10cp03-api-cover"
                                                                                cover.mkdir()
                                                                                env.update(QA_CHECKPOINT="m10cp03", QA_IDENTITY_BINARY=str(identity_binary),
                                                                                    QA_ISSUER_BINARY=str(issuer_binary), QA_BRIDGE_PUBLIC=(directory / "issuer-public.txt").read_text(),
                                                                                    QA_CASE_RESULTS=str(directory / "m10cp03-api-cases.json"), GOCOVERDIR=str(cover))
                                                                                command([sys.executable, "-m", "pytest", "-p", "qa.plugin", "qa/tests/completion_api", "-q"], 240)
                                                                                cases = json.loads((directory / "m10cp03-api-cases.json").read_text())
                                                                                expected = set(manifest["m10_cp03_python"])
                                                                                if cases["exit_status"] != 0 or set(cases["collected_ids"]) != expected or len(cases["cases"]) != len(expected) or any(c["status"] != "PASS" for c in cases["cases"]):
                                                                                    raise ValueError("m10cp03 Playwright cases incomplete")
                                                                                profile = directory / "m10cp03-api-coverage.out"
                                                                                command(["go", "tool", "covdata", "textfmt", "-i=" + str(cover), "-o=" + str(profile)])
                                                                                if not any(line.startswith("lmsv3/internal/completion/credential.go:") and int(line.rsplit(" ", 1)[1]) > 0 for line in profile.read_text(encoding="utf-8").splitlines()[1:]):
                                                                                    raise ValueError("m10cp03 credential coverage missing")
                                                                                result["m10cp03_api"] = {"status":"PASS_SYNTHETIC_ONLY", "cases":cases["cases"],
                                                                                    "instrumented_go_coverage":command(["go","tool","cover","-func="+str(profile)]).decode().strip()}
                                                                                for query in (
                                                                                    "SELECT count(*) FROM v3.transcript_grade_items i LEFT JOIN v3.transcript_releases t ON t.id=i.transcript_id AND t.class_id=i.class_id AND t.learner_user_id=i.learner_user_id WHERE t.id IS NULL",
                                                                                    "SELECT count(*) FROM v3.transcript_releases t WHERE t.version<>(SELECT count(*) FROM v3.transcript_releases x WHERE x.class_id=t.class_id AND x.episode_id=t.episode_id AND x.version<=t.version)",
                                                                                    "SELECT count(*) FROM v3.credential_records c WHERE (SELECT count(*) FROM v3.credential_events e WHERE e.credential_id=c.id AND e.action='ISSUED')<>1",
                                                                                    "SELECT count(*) FROM v3.credential_events e WHERE e.version<>(SELECT count(*) FROM v3.credential_events x WHERE x.credential_id=e.credential_id AND x.version<=e.version)",
                                                                                    "SELECT count(*) FROM v3.audit a LEFT JOIN v3.outbox o ON o.event_id=a.id WHERE (a.action='transcript.release' OR a.action LIKE 'credential.%') AND o.event_id IS NULL",
                                                                                ):
                                                                                    if sql("qa_cp05", query) != b"0": raise ValueError("M10 transcript/credential reconciliation failed")
                                                                                if sql("qa_cp05", "SELECT bool_or(has_table_privilege('qa_runtime',format('%I.%I',schemaname,tablename),'DELETE') OR has_table_privilege('qa_runtime',format('%I.%I',schemaname,tablename),'UPDATE')) FROM pg_tables WHERE schemaname='v3' AND tablename IN ('transcript_releases','transcript_grade_items','credential_records','credential_events','credential_commands')") != b"f":
                                                                                    raise ValueError("M10 credential history mutation privilege detected")
                                                                                result["m10_cp03_reconciliation"] = "PASS"
                                                                                if legacy_output_journey:
                                                                                    m13_proof("m10_cp04")
                                                                                    slice_go("m10_cp04", "./internal/completion", "TestSyntheticLegacyOutput(ObservedOnly|IdempotencyBoundary|CorrectionStability)", race=os.name != "nt")
                                                                                    sql("qa_cp05", "GRANT SELECT,INSERT ON v3.legacy_completion_runs,v3.legacy_completion_observations TO qa_runtime")
                                                                                    state = json.loads(sql("qa_cp05", "SELECT jsonb_object_agg(l.subject,jsonb_build_object('id',l.user_id,'version',l.version,'active',l.active,'site_role_version',r.version)) FROM v3identity.links l JOIN v3identity.site_roles r ON r.user_id=l.user_id"))
                                                                                    write_new_json(directory / "m10cp04-state.json", state)
                                                                                    cover = directory / "m10cp04-api-cover"
                                                                                    cover.mkdir()
                                                                                    env.update(QA_CHECKPOINT="m10cp04", QA_IDENTITY_BINARY=str(identity_binary),
                                                                                        QA_ISSUER_BINARY=str(issuer_binary), QA_BRIDGE_PUBLIC=(directory / "issuer-public.txt").read_text(),
                                                                                        QA_CASE_RESULTS=str(directory / "m10cp04-api-cases.json"), GOCOVERDIR=str(cover))
                                                                                    command([sys.executable, "-m", "pytest", "-p", "qa.plugin", "qa/tests/completion_api", "-q"], 240)
                                                                                    cases = json.loads((directory / "m10cp04-api-cases.json").read_text())
                                                                                    expected = set(manifest["m10_cp04_python"])
                                                                                    if cases["exit_status"] != 0 or set(cases["collected_ids"]) != expected or len(cases["cases"]) != len(expected) or any(c["status"] != "PASS" for c in cases["cases"]):
                                                                                        raise ValueError("m10cp04 Playwright cases incomplete")
                                                                                    profile = directory / "m10cp04-api-coverage.out"
                                                                                    command(["go", "tool", "covdata", "textfmt", "-i=" + str(cover), "-o=" + str(profile)])
                                                                                    if not any(line.startswith("lmsv3/internal/completion/legacy_output.go:") and int(line.rsplit(" ", 1)[1]) > 0 for line in profile.read_text(encoding="utf-8").splitlines()[1:]):
                                                                                        raise ValueError("m10cp04 observation coverage missing")
                                                                                    result["m10cp04_api"] = {"status": "PASS_SYNTHETIC_ONLY", "cases": cases["cases"],
                                                                                        "instrumented_go_coverage": command(["go", "tool", "cover", "-func=" + str(profile)]).decode().strip()}
                                                                                    for query in (
                                                                                        "SELECT count(*) FROM v3.legacy_completion_observations o LEFT JOIN v3.legacy_completion_runs r ON r.id=o.run_id WHERE r.id IS NULL OR r.source_instance_id<>o.source_instance_id",
                                                                                        "SELECT count(*) FROM v3.legacy_completion_runs r WHERE jsonb_array_length(r.response->'observations')<>(SELECT count(*) FROM v3.legacy_completion_observations o WHERE o.run_id=r.id)",
                                                                                        "SELECT count(*) FROM v3.legacy_completion_observations WHERE (observed_state IN ('TRUE','FALSE') AND disposition<>'OBSERVED_UNVERIFIED') OR (observed_state IN ('UNKNOWN','INVALID') AND disposition<>'UNKNOWN_HOLD')",
                                                                                        "SELECT count(*) FROM v3.legacy_completion_observations o WHERE o.candidate_class_id IS NOT NULL AND NOT EXISTS(SELECT 1 FROM v3.enrollments e WHERE e.class_id=o.candidate_class_id AND e.user_id=o.candidate_learner_user_id)",
                                                                                        "SELECT count(*) FROM v3.audit WHERE action='completion.legacy.observe'",
                                                                                    ):
                                                                                        if sql("qa_cp05", query) != b"0": raise ValueError("M10 legacy output reconciliation failed")
                                                                                    if sql("qa_cp05", "SELECT bool_or(has_table_privilege('qa_runtime',format('%I.%I',schemaname,tablename),'DELETE') OR has_table_privilege('qa_runtime',format('%I.%I',schemaname,tablename),'UPDATE')) FROM pg_tables WHERE schemaname='v3' AND tablename IN ('legacy_completion_runs','legacy_completion_observations')") != b"f":
                                                                                        raise ValueError("M10 legacy observation mutation privilege detected")
                                                                                    result["m10_cp04_reconciliation"] = "PASS"
                                                                                    if class_academic_journey:
                                                                                        m13_proof("m05_cp05")
                                                                                        state = json.loads(sql("qa_cp05", "SELECT jsonb_object_agg(l.subject,jsonb_build_object('id',l.user_id,'version',l.version,'active',l.active,'site_role_version',r.version)) FROM v3identity.links l JOIN v3identity.site_roles r ON r.user_id=l.user_id"))
                                                                                        write_new_json(directory / "m05cp05-state.json", state)
                                                                                        cover = directory / "m05cp05-api-cover"
                                                                                        cover.mkdir()
                                                                                        env.update(QA_CHECKPOINT="m05cp05", QA_IDENTITY_BINARY=str(identity_binary),
                                                                                            QA_ISSUER_BINARY=str(issuer_binary), QA_BRIDGE_PUBLIC=(directory / "issuer-public.txt").read_text(),
                                                                                            QA_CASE_RESULTS=str(directory / "m05cp05-api-cases.json"), GOCOVERDIR=str(cover))
                                                                                        command([sys.executable, "-m", "pytest", "-p", "qa.plugin", "qa/tests/class_academic_api", "-q"], 240)
                                                                                        cases = json.loads((directory / "m05cp05-api-cases.json").read_text())
                                                                                        expected = set(manifest["m05_cp05_python"])
                                                                                        if cases["exit_status"] != 0 or set(cases["collected_ids"]) != expected or len(cases["cases"]) != len(expected) or any(c["status"] != "PASS" for c in cases["cases"]):
                                                                                            raise ValueError("m05cp05 Playwright cases incomplete")
                                                                                        slice_go("m05_cp05", "./internal/classes", "TestSyntheticClassAcademic(Ancestry|History)", race=os.name != "nt")
                                                                                        profile = directory / "m05cp05-api-coverage.out"
                                                                                        command(["go", "tool", "covdata", "textfmt", "-i=" + str(cover), "-o=" + str(profile)])
                                                                                        if not any(line.startswith("lmsv3/internal/classes/") and int(line.rsplit(" ", 1)[1]) > 0 for line in profile.read_text(encoding="utf-8").splitlines()[1:]):
                                                                                            raise ValueError("m05cp05 Class coverage missing")
                                                                                        result["m05cp05_api"] = {"status": "PASS_SYNTHETIC_ONLY", "cases": cases["cases"],
                                                                                            "instrumented_go_coverage": command(["go", "tool", "cover", "-func=" + str(profile)]).decode().strip()}
                                                                                        for query in (
                                                                                            "SELECT count(*) FROM v3.submission_aggregates s LEFT JOIN v3.episodes ep ON ep.id=s.episode_id AND ep.enrollment_id=s.enrollment_id LEFT JOIN v3.enrollments e ON e.id=s.enrollment_id AND e.class_id=s.class_id AND e.user_id=s.owner_user_id WHERE ep.id IS NULL OR e.id IS NULL",
                                                                                            "SELECT count(*) FROM v3.grade_revisions gr JOIN v3.submission_aggregates s ON s.id=gr.aggregate_id WHERE gr.class_id<>s.class_id OR gr.learner_user_id<>s.owner_user_id",
                                                                                            "SELECT count(*) FROM v3.grade_releases rel JOIN v3.grade_revisions gr ON gr.id=rel.revision_id WHERE rel.learner_user_id<>gr.learner_user_id OR rel.class_id<>gr.class_id",
                                                                                            "SELECT count(*) FROM v3.episodes ep JOIN v3.episodes old ON old.id=ep.origin_episode_id WHERE ep.kind='RETAKE' AND (ep.enrollment_id<>old.enrollment_id OR old.ended_at IS NULL)",
                                                                                        ):
                                                                                            if sql("qa_cp05", query) != b"0": raise ValueError("M05 academic ancestry reconciliation failed")
                                                                                        result["m05_cp05_reconciliation"] = "PASS"
                                                                                        if content_closure_journey:
                                                                                            m13_proof("m07_cp04")
                                                                                            state = json.loads(sql("qa_cp05", "SELECT jsonb_object_agg(l.subject,jsonb_build_object('id',l.user_id,'version',l.version,'active',l.active,'site_role_version',r.version)) FROM v3identity.links l JOIN v3identity.site_roles r ON r.user_id=l.user_id"))
                                                                                            write_new_json(directory / "m07cp04-state.json", state)
                                                                                            cover = directory / "m07cp04-api-cover"
                                                                                            cover.mkdir()
                                                                                            env.update(QA_CHECKPOINT="m07cp04", QA_IDENTITY_BINARY=str(identity_binary),
                                                                                                QA_ISSUER_BINARY=str(issuer_binary), QA_BRIDGE_PUBLIC=(directory / "issuer-public.txt").read_text(),
                                                                                                QA_CASE_RESULTS=str(directory / "m07cp04-api-cases.json"), GOCOVERDIR=str(cover))
                                                                                            command([sys.executable, "-m", "pytest", "-p", "qa.plugin", "qa/tests/content_closure_api", "-q"], 300)
                                                                                            cases = json.loads((directory / "m07cp04-api-cases.json").read_text())
                                                                                            expected = set(manifest["m07_cp04_python"])
                                                                                            if cases["exit_status"] != 0 or set(cases["collected_ids"]) != expected or len(cases["cases"]) != len(expected) or any(c["status"] != "PASS" for c in cases["cases"]):
                                                                                                raise ValueError("m07cp04 Playwright cases incomplete")
                                                                                            slice_go("m07_cp04", "./internal/content", "Test(SyntheticContentClosure(Ownership|Clone)|LegacyImportantLinksPreview)", race=os.name != "nt")
                                                                                            profile = directory / "m07cp04-api-coverage.out"
                                                                                            command(["go", "tool", "covdata", "textfmt", "-i=" + str(cover), "-o=" + str(profile)])
                                                                                            if not any(line.startswith("lmsv3/internal/content/clone.go:") and int(line.rsplit(" ", 1)[1]) > 0 for line in profile.read_text(encoding="utf-8").splitlines()[1:]):
                                                                                                raise ValueError("m07cp04 clone API coverage missing")
                                                                                            result["m07cp04_api"] = {"status": "PASS_SYNTHETIC_ONLY", "cases": cases["cases"],
                                                                                                "instrumented_go_coverage": command(["go", "tool", "cover", "-func=" + str(profile)]).decode().strip()}
                                                                                            for query in (
                                                                                                "SELECT count(*) FROM v3.submission_aggregates s LEFT JOIN v3.content_revisions r ON r.id=s.assignment_revision_id AND r.class_id=s.class_id AND r.activity_id=s.activity_id WHERE r.id IS NULL",
                                                                                                "SELECT count(*) FROM v3.audit a LEFT JOIN v3.outbox o ON o.event_id=a.id WHERE a.action='content.clone_structure' AND o.event_id IS NULL",
                                                                                                "SELECT count(*) FROM v3.content_commands c JOIN v3.classes target ON target.id=c.response->>'target_class_id' WHERE c.response ? 'source_class_id' AND (EXISTS(SELECT 1 FROM v3.content_revisions r WHERE r.class_id=target.id) OR (SELECT count(*) FROM v3.content_sections s WHERE s.class_id=target.id)<>jsonb_array_length(c.response->'sections'))",
                                                                                            ):
                                                                                                if sql("qa_cp05", query) != b"0": raise ValueError("M07 content closure reconciliation failed")
                                                                                            result["m07_cp04_reconciliation"] = "PASS"
                    if ai_suggestion_journey:
                        m13_proof("m13_cp03")
                        command(["go", "test", "-count=1", "-tags=qaintegration", "-run=^TestSyntheticAISuggestionInstall$", "./internal/aisuggestions"], 120)
                        state = json.loads(sql("qa_cp05", "SELECT jsonb_object_agg(l.subject,jsonb_build_object('id',l.user_id,'version',l.version,'active',l.active,'site_role_version',r.version)) FROM v3identity.links l JOIN v3identity.site_roles r ON r.user_id=l.user_id"))
                        write_new_json(directory / "m13cp03-state.json", state)
                        cover = directory / "m13cp03-api-cover"
                        cover.mkdir()
                        env.update(QA_CHECKPOINT="m13cp03", QA_IDENTITY_BINARY=str(identity_binary),
                                   QA_ISSUER_BINARY=str(issuer_binary), QA_BRIDGE_PUBLIC=(directory / "issuer-public.txt").read_text(),
                                   QA_CASE_RESULTS=str(directory / "m13cp03-api-cases.json"), GOCOVERDIR=str(cover))
                        command([sys.executable, "-m", "pytest", "-p", "qa.plugin", "qa/tests/ai_suggestion_api", "-q"], 300)
                        cases = json.loads((directory / "m13cp03-api-cases.json").read_text())
                        expected = set(manifest["m13_cp03_python"])
                        if cases["exit_status"] != 0 or set(cases["collected_ids"]) != expected or len(cases["cases"]) != len(expected) or any(c["status"] != "PASS" for c in cases["cases"]):
                            raise ValueError("m13cp03 Playwright cases incomplete")
                        slice_go("m13_cp03", "./internal/aisuggestions", "Test(SyntheticAISuggestion(Provenance|Boundary)|AISuggestionValidation)", race=os.name != "nt")
                        profile = directory / "m13cp03-api-coverage.out"
                        command(["go", "tool", "covdata", "textfmt", "-i=" + str(cover), "-o=" + str(profile)])
                        if not any(line.startswith("lmsv3/internal/aisuggestions/") and int(line.rsplit(" ", 1)[1]) > 0 for line in profile.read_text(encoding="utf-8").splitlines()[1:]):
                            raise ValueError("m13cp03 API coverage missing")
                        result["m13cp03_api"] = {"status": "PASS_SYNTHETIC_ONLY", "cases": cases["cases"],
                            "instrumented_go_coverage": command(["go", "tool", "cover", "-func=" + str(profile)]).decode().strip()}
                        for query in (
                            "SELECT count(*) FROM v3.ai_suggestions s LEFT JOIN v3.submission_revisions r ON r.id=s.submission_revision_id AND r.class_id=s.class_id WHERE r.id IS NULL OR r.body_sha256<>s.submission_sha256",
                            "SELECT count(*) FROM v3.grade_revisions g JOIN v3.ai_suggestions s ON s.id=g.ai_suggestion_id WHERE s.status<>'SUCCEEDED' OR s.class_id<>g.class_id OR s.grade_item_id<>g.grade_item_id OR s.aggregate_id<>g.aggregate_id OR s.submission_revision_id<>g.submission_revision_id OR s.definition_version_id<>g.definition_version_id",
                            "SELECT count(*) FROM v3.ai_suggestions WHERE status='FAILED' AND (proposed_value_milli IS NOT NULL OR failure_code IS NULL)",
                            "SELECT count(*) FROM v3.audit a LEFT JOIN v3.outbox o ON o.event_id=a.id WHERE a.action='ai.suggestion.created' AND o.event_id IS NULL",
                        ):
                            if sql("qa_cp05", query) != b"0": raise ValueError("M13 AI suggestion reconciliation failed")
                        if sql("qa_cp05", "SELECT count(*) FROM v3.grade_revisions WHERE ai_suggestion_id IS NOT NULL") == b"0":
                            raise ValueError("M13 human grade link missing")
                        result["m13_cp03_reconciliation"] = "PASS"
                    if reporting_journey:
                        slice_go("m14_cp01", "./internal/reporting", "Test(SyntheticReportIntegrity|ReportingContract)", race=os.name != "nt")
                        for cp, key in (("m14cp01", "m14_cp01_python"), ("m14cp02", "m14_cp02_python"), ("m14cp03", "m14_cp03_python")):
                            if cp == "m14cp02" and not reporting_contract_journey:
                                continue
                            if cp == "m14cp03" and not reporting_pagination_journey:
                                continue
                            state = json.loads(sql("qa_cp05", "SELECT jsonb_object_agg(l.subject,jsonb_build_object('id',l.user_id,'version',l.version,'active',l.active,'site_role_version',r.version)) FROM v3identity.links l JOIN v3identity.site_roles r ON r.user_id=l.user_id"))
                            write_new_json(directory / (cp+"-state.json"), state)
                            cover = directory / (cp+"-api-cover")
                            cover.mkdir()
                            env.update(QA_CHECKPOINT=cp, QA_IDENTITY_BINARY=str(identity_binary),
                                       QA_ISSUER_BINARY=str(issuer_binary), QA_BRIDGE_PUBLIC=(directory / "issuer-public.txt").read_text(),
                                       QA_CASE_RESULTS=str(directory / (cp+"-api-cases.json")), GOCOVERDIR=str(cover))
                            command([sys.executable, "-m", "pytest", "-p", "qa.plugin", "qa/tests/reporting_api", "-q"], 300)
                            cases = json.loads((directory / (cp+"-api-cases.json")).read_text())
                            expected = set(manifest[key])
                            if cases["exit_status"] != 0 or set(cases["collected_ids"]) != expected or len(cases["cases"]) != len(expected) or any(c["status"] != "PASS" for c in cases["cases"]):
                                raise ValueError(cp+" Playwright cases incomplete")
                            profile = directory / (cp+"-api-coverage.out")
                            command(["go", "tool", "covdata", "textfmt", "-i="+str(cover), "-o="+str(profile)])
                            if not any(line.startswith("lmsv3/internal/reporting/") and int(line.rsplit(" ", 1)[1]) > 0 for line in profile.read_text(encoding="utf-8").splitlines()[1:]):
                                raise ValueError(cp+" reporting API coverage missing")
                            result[cp+"_api"] = {"status":"PASS_SYNTHETIC_ONLY", "cases":cases["cases"],
                                "instrumented_go_coverage":command(["go", "tool", "cover", "-func="+str(profile)]).decode().strip()}
                        if reporting_contract_journey:
                            slice_go("m14_cp02", "./internal/reporting", "TestReportingContract", race=os.name != "nt")
                        if reporting_pagination_journey:
                            slice_go("m14_cp03", "./internal/reporting", "Test(Report(PageCursorSnapshot|ExportEncodingBounds)|SyntheticReportExportAudit)", race=os.name != "nt")
                            if sql("qa_cp05", "SELECT count(*) FROM v3.audit a LEFT JOIN v3.outbox o ON o.event_id=a.id WHERE a.action LIKE 'reporting.grades.export/%' AND (o.event_id IS NULL OR o.topic<>'reporting.audit.v1')") != b"0":
                                raise ValueError("M14 private export audit reconciliation failed")
                            result["m14_cp03_reconciliation"] = "PASS"
                        if reporting_performance_journey:
                            # Append-only synthetic fixtures are preceded by a verified full restore.
                            m13_proof("m14_cp04")
                            native_binary = directory / ("report-load.exe" if os.name == "nt" else "report-load")
                            command(["go", "build", "-trimpath", "-o", native_binary, "./cmd/report-load"], 120)
                            slice_go("m14_cp04_load", "./internal/reportbench", "Test(LoadConfigurationBoundary|NearestRankPercentile|LoadMetricsAndContract)", race=os.name != "nt")
                            state = json.loads(sql("qa_cp05", "SELECT jsonb_object_agg(l.subject,jsonb_build_object('id',l.user_id,'version',l.version,'active',l.active,'site_role_version',r.version)) FROM v3identity.links l JOIN v3identity.site_roles r ON r.user_id=l.user_id"))
                            write_new_json(directory / "m14cp04-state.json", state)
                            cover = directory / "m14cp04-api-cover"
                            cover.mkdir()
                            env.update(QA_CHECKPOINT="m14cp04", QA_IDENTITY_BINARY=str(identity_binary),
                                       QA_ISSUER_BINARY=str(issuer_binary), QA_BRIDGE_PUBLIC=(directory / "issuer-public.txt").read_text(),
                                       QA_CASE_RESULTS=str(directory / "m14cp04-api-cases.json"), GOCOVERDIR=str(cover),
                                       QA_REPORT_LOAD_BINARY=str(native_binary))
                            command([sys.executable, "-m", "pytest", "-p", "qa.plugin", "qa/tests/reporting_performance_api",
                                     "qa/tests/ops/test_reporting_performance.py", "-q", "-s"], 1200)
                            cases = json.loads((directory / "m14cp04-api-cases.json").read_text())
                            expected = set(manifest["m14_cp04_python"])
                            if cases["exit_status"] != 0 or set(cases["collected_ids"]) != expected or len(cases["cases"]) != len(expected) or any(c["status"] != "PASS" for c in cases["cases"]):
                                raise ValueError("M14-CP04 Python evidence incomplete")
                            from qa.reporting_perf import review as review_load
                            accepted = review_load(json.loads((directory / "m14cp04-load-raw.json").read_text()),
                                                   json.loads((ROOT / "qa/manifests/M14-CP04-budget.json").read_text()))
                            if accepted != json.loads((directory / "m14cp04-load-acceptance.json").read_text()):
                                raise ValueError("load acceptance differs from raw samples")
                            profile = directory / "m14cp04-api-coverage.out"
                            command(["go", "tool", "covdata", "textfmt", "-i="+str(cover), "-o="+str(profile)])
                            for source_file in ("report.go", "pages.go"):
                                if not any(line.startswith("lmsv3/internal/reporting/"+source_file+":") and int(line.rsplit(" ",1)[1])>0 for line in profile.read_text(encoding="utf-8").splitlines()[1:]):
                                    raise ValueError("M14-CP04 actual API source coverage missing")
                            result["m14cp04_api"] = {"status":"PASS_SYNTHETIC_ONLY", "cases":cases["cases"],
                                "instrumented_go_coverage":command(["go", "tool", "cover", "-func="+str(profile)]).decode().strip()}
                            result["m14_cp04_performance"] = accepted
                            slice_go("m14_cp04_db", "./internal/reporting", "TestSyntheticReportPerformancePlanAndPool", race=os.name != "nt")
                            result["m14_cp04_reconciliation"] = "PASS"
                    if migration_completion_journey:
                        if gradebook_journey:
                            if ai_suggestion_journey:
                                env.pop("QA_CATALOGUE_SCOPE", None)
                            else:
                                env["QA_CATALOGUE_SCOPE"] = "pre-m13-cp03"
                        elif notification_journey:
                            env["QA_CATALOGUE_SCOPE"] = "pre-m09-cp01"
                        elif logbook_lifecycle_journey:
                            env["QA_CATALOGUE_SCOPE"] = "pre-m13-cp01"
                        elif logbook_period_journey:
                            env["QA_CATALOGUE_SCOPE"] = "pre-m12-cp02"
                        else:
                            env["QA_CATALOGUE_SCOPE"] = "pre-m12-cp01"
                    elif submission_journey:
                        env["QA_CATALOGUE_SCOPE"] = "pre-m11-cp01"
                    elif content_contract_journey:
                        env["QA_CATALOGUE_SCOPE"] = "pre-m08-cp01"
                    elif content_safety_journey:
                        env["QA_CATALOGUE_SCOPE"] = "pre-m07-cp03"
                    elif content_journey:
                        env["QA_CATALOGUE_SCOPE"] = "pre-m07-cp02"
                    else:
                        env["QA_CATALOGUE_SCOPE"] = "pre-m07"
                    if runtime_journey:
                        tls_binary=directory/("tls-fixture.exe" if os.name=="nt" else "tls-fixture")
                        command(["go","build","-trimpath","-o",tls_binary,"./cmd/tls-fixture"],120)
                        command([tls_binary],30)
                        api_binary=directory/("api-runtime.exe" if os.name=="nt" else "api-runtime")
                        command(["go","build","-cover","-coverpkg=./cmd/api,./internal/identity,./internal/classes,./internal/access,./internal/platform/httpserver,./internal/platform/database,./internal/migration/runner","-trimpath","-o",api_binary,"./cmd/api"],120)
                        with socket.socket() as runtime_socket:
                            runtime_socket.bind(("127.0.0.1",0));runtime_port=runtime_socket.getsockname()[1]
                        runtime_state=json.loads(sql("qa_cp05", "SELECT jsonb_object_agg(l.subject,jsonb_build_object('id',l.user_id,'version',l.version,'active',l.active,'site_role_version',r.version)) FROM v3identity.links l JOIN v3identity.site_roles r ON r.user_id=l.user_id"))
                        write_new_json(directory/"runtime-state.json",runtime_state)
                        runtime_cover=directory/"m03-cp04-api-cover";runtime_cover.mkdir()
                        env.update(QA_CHECKPOINT="runtime",QA_RUNTIME_KIND="api",QA_IDENTITY_BINARY=str(api_binary),QA_ISSUER_BINARY=str(issuer_binary),QA_BRIDGE_PUBLIC=(directory/"issuer-public.txt").read_text(),QA_CASE_RESULTS=str(directory/"m03-cp04-api-cases.json"),GOCOVERDIR=str(runtime_cover),LMS_MODE="v3-local-synthetic",LMS_HTTP_ADDR=f"127.0.0.1:{runtime_port}",LMS_ORIGIN=f"https://127.0.0.1:{runtime_port}",LMS_TLS_CERT_FILE=str((directory/"runtime-tls.crt").resolve()),LMS_TLS_KEY_FILE=str((directory/"runtime-tls.key").resolve()),LMS_BRIDGE_PUBLIC_KEY=(directory/"issuer-public.txt").read_text())
                        command([sys.executable,"-m","pytest","-p","qa.plugin","qa/tests/identity_api","-q"],180)
                        api_cases=json.loads((directory/"m03-cp04-api-cases.json").read_text())
                        expected=set(manifest["m03_cp04_python"])
                        if api_cases["exit_status"]!=0 or set(api_cases["collected_ids"])!=expected or len(api_cases["cases"])!=len(expected) or any(c["status"]!="PASS" for c in api_cases["cases"]):
                            raise ValueError("runtime API required cases incomplete")
                        profile=directory/"m03-cp04-api-coverage.out"
                        command(["go","tool","covdata","textfmt","-i="+str(runtime_cover),"-o="+str(profile)])
                        require_coverage(profile)
                        result["m03_cp04"]=dict(status="PASS_SYNTHETIC_ONLY",cases=api_cases["cases"],binary_sha256=sha256(api_binary.read_bytes()).hexdigest(),instrumented_go_coverage=command(["go","tool","cover","-func="+str(profile)]).decode().strip(),startup_ddl="NONE",direct_tls=True,graceful_shutdown="PASS")
                    if worker_journey:
                        result["m03_cp05_backup"] = snapshot("qa_cp05", "pre_m03_cp05")
                        slice_go("m03_cp05", "./internal/outbox", "TestSyntheticWorkerLifecycle", race=os.name != "nt")
                        slice_go("m03_cp06", "./internal/outbox", "TestSyntheticWorkerConcurrentReplay", race=os.name != "nt")
                        result["m03_cp06"] = {
                            **result.get("m03_cp06", {}),
                            "status": "PASS_SYNTHETIC_ONLY",
                            "tagged_db_race": "PASS_NATIVE" if os.name != "nt" else "NOT_RUN_WINDOWS_COMPILER",
                            "pool_pressure": "PASS_IN_M03_CP05_CASE",
                            "commit_uncertainty_recovery": "PASS_IN_M03_CP05_CASE",
                            "container_execution": "NOT_RUN",
                            "remote_ci_execution": "NOT_RUN"
                        }
                    # Closure consumes completed worker regression evidence.
                    # Checking above the worker block rejects a healthy run
                    # before TestSyntheticWorkerLifecycle has been executed.
                    if consumer_closure_journey:
                        if result.get("m03_cp05", {}).get("status") != "PASS_SYNTHETIC_ONLY" or result.get("m13_cp03_reconciliation") != "PASS":
                            raise ValueError("M13-CP04 dependency regression missing")
                        slice_go("m13_cp04", "./internal/outbox", "TestWorker(ReceiptDeadline|UnknownOutcomeRetry|RetryConcurrencyAndShutdown)", race=os.name != "nt")
                        env["QA_CASE_RESULTS"] = str(directory / "m13cp04-runner-cases.json")
                        command([sys.executable, "-m", "pytest", "-p", "qa.plugin", "qa/tests/ops/test_consumer_closure_order.py", "-q"], 60)
                        runner_cases = json.loads((directory / "m13cp04-runner-cases.json").read_text())
                        if (runner_cases["exit_status"] != 0 or set(runner_cases["collected_ids"]) != set(manifest["m13_cp04_python"])
                                or len(runner_cases["cases"]) != len(manifest["m13_cp04_python"])
                                or any(case["status"] != "PASS" for case in runner_cases["cases"])):
                            raise ValueError("M13-CP04 runner regression incomplete")
                        result["m13_cp04_runner"] = {"status":"PASS_SYNTHETIC_ONLY", "cases":runner_cases["cases"]}
                        result["m13_cp04_consumer"] = {"status":"PASS_SYNTHETIC_ONLY", "dependencies":["m03_cp05", "m13_cp03_reconciliation"]}
                    if participation_journey:
                        for checkpoint, test in (("m05_cp03", "TestSyntheticGroupingAndMentor"),
                                                 ("m05_cp04", "TestSyntheticInvitationRetakeAndProvenance")):
                            snapshot_name = "pre_" + checkpoint
                            result[checkpoint + "_backup"] = snapshot("qa_cp05", snapshot_name)
                            write_new_json(directory / (checkpoint.replace("_", "-") + "-proof.json"), {
                                "run_id": run_id,
                                "target": target,
                                "created_at": datetime.now(timezone.utc).isoformat(),
                                "restored": True,
                                "archive_hash": sha256((directory / (snapshot_name + ".dump")).read_bytes()).hexdigest(),
                            })
                            slice_go(checkpoint, "./internal/classes", test, race=os.name != "nt")
                        participation_state = json.loads(sql("qa_cp05", "SELECT jsonb_object_agg(l.subject,jsonb_build_object('id',l.user_id,'version',l.version,'active',l.active,'site_role_version',r.version)) FROM v3identity.links l JOIN v3identity.site_roles r ON r.user_id=l.user_id"))
                        write_new_json(directory / "m05-state.json", participation_state)
                        participation_cover = directory / "m05-api-cover"
                        participation_cover.mkdir()
                        # The prior cmd/api regression uses signal-based process
                        # control. M05 intentionally starts identity-lab, whose
                        # bounded stdin shutdown contract is different.
                        env.pop("QA_RUNTIME_KIND", None)
                        env.update(QA_CHECKPOINT="m05", QA_IDENTITY_BINARY=str(identity_binary),
                                   QA_ISSUER_BINARY=str(issuer_binary),
                                   QA_BRIDGE_PUBLIC=(directory / "issuer-public.txt").read_text(),
                                   QA_CASE_RESULTS=str(directory / "m05-api-cases.json"),
                                   GOCOVERDIR=str(participation_cover))
                        command([sys.executable, "-m", "pytest", "-p", "qa.plugin", "qa/tests/participation_api", "-q"], 180)
                        participation_cases = json.loads((directory / "m05-api-cases.json").read_text())
                        expected = set(manifest["m05_python"])
                        if (participation_cases["exit_status"] != 0 or
                                set(participation_cases["collected_ids"]) != expected or
                                len(participation_cases["cases"]) != len(expected) or
                                any(case["status"] != "PASS" for case in participation_cases["cases"])):
                            raise ValueError("M05 Playwright cases incomplete")
                        profile = directory / "m05-api-coverage.out"
                        command(["go", "tool", "covdata", "textfmt", "-i=" + str(participation_cover), "-o=" + str(profile)])
                        require_coverage(profile)
                        coverage = command(["go", "tool", "cover", "-func=" + str(profile)]).decode().strip()
                        result["m05_api"] = {
                            "status": "PASS_SYNTHETIC_ONLY",
                            "cases": participation_cases["cases"],
                            "binary_sha256": sha256(identity_binary.read_bytes()).hexdigest(),
                            "instrumented_go_coverage": coverage,
                        }
                        for query in (
                            "SELECT count(*) FROM v3.group_memberships WHERE ended_at IS NULL GROUP BY enrollment_id HAVING count(*)>1",
                            "SELECT count(*) FROM v3.mentor_assignments WHERE ended_at IS NULL GROUP BY student_enrollment_id HAVING count(*)>1",
                            "SELECT count(*) FROM v3.invitations i LEFT JOIN v3.enrollments e ON e.class_id=i.class_id AND e.user_id=i.user_id WHERE i.status='ACCEPTED' AND e.id IS NULL",
                            "SELECT count(*) FROM v3.episodes WHERE kind<>'INITIAL' AND origin_episode_id IS NULL",
                            "SELECT count(*) FROM v3.audit a LEFT JOIN v3.outbox o ON o.event_id=a.id WHERE o.event_id IS NULL",
                        ):
                            rows = sql("qa_cp05", query).decode().splitlines()
                            if rows and any(value != "0" for value in rows):
                                raise ValueError("M05 persistence reconciliation failed")
                        result["m05_reconciliation"] = "PASS"
                    if file_journey:
                        file_state = json.loads(sql("qa_cp05", "SELECT jsonb_object_agg(l.subject,jsonb_build_object('id',l.user_id,'version',l.version,'active',l.active,'site_role_version',r.version)) FROM v3identity.links l JOIN v3identity.site_roles r ON r.user_id=l.user_id"))
                        write_new_json(directory / "m06-state.json", file_state)
                        file_cover = directory / "m06-api-cover"
                        file_cover.mkdir()
                        file_root = directory / "m06-private-objects"
                        env.pop("QA_RUNTIME_KIND", None)
                        env.update(QA_CHECKPOINT="m06", QA_FILE_ROOT=str(file_root),
                                   QA_IDENTITY_BINARY=str(identity_binary), QA_ISSUER_BINARY=str(issuer_binary),
                                   QA_BRIDGE_PUBLIC=(directory / "issuer-public.txt").read_text(),
                                   QA_CASE_RESULTS=str(directory / "m06-api-cases.json"), GOCOVERDIR=str(file_cover))
                        command([sys.executable, "-m", "pytest", "-p", "qa.plugin", "qa/tests/file_api", "-q"], 210)
                        file_cases = json.loads((directory / "m06-api-cases.json").read_text())
                        expected = set(manifest["m06_python"])
                        if (file_cases["exit_status"] != 0 or set(file_cases["collected_ids"]) != expected or
                                len(file_cases["cases"]) != len(expected) or any(case["status"] != "PASS" for case in file_cases["cases"])):
                            raise ValueError("M06 Playwright cases incomplete")
                        profile = directory / "m06-api-coverage.out"
                        command(["go", "tool", "covdata", "textfmt", "-i=" + str(file_cover), "-o=" + str(profile)])
                        require_coverage(profile)
                        if "lmsv3/internal/files/" not in profile.read_text(encoding="utf-8"):
                            raise ValueError("M06 file coverage missing")
                        result["m06_api"] = {"status": "PASS_SYNTHETIC_ONLY", "cases": file_cases["cases"],
                            "binary_sha256": sha256(identity_binary.read_bytes()).hexdigest(),
                            "instrumented_go_coverage": command(["go", "tool", "cover", "-func=" + str(profile)]).decode().strip()}
                        for query in (
                            "SELECT count(*) FROM v3.file_assets WHERE object_key=sha256 OR object_key !~ '^[a-f0-9]{64}$' OR sha256 !~ '^[a-f0-9]{64}$' OR state='AVAILABLE' AND (scan_status<>'CLEAN' OR available_at IS NULL)",
                            "SELECT count(*) FROM v3.file_access_events e LEFT JOIN v3.file_assets a ON a.id=e.asset_id WHERE a.id IS NULL",
                            "SELECT count(*) FROM v3.audit a LEFT JOIN v3.outbox o ON o.event_id=a.id WHERE a.action LIKE 'file.%' AND o.event_id IS NULL",
                        ):
                            rows = sql("qa_cp05", query).decode().splitlines()
                            if rows and any(value != "0" for value in rows):
                                raise ValueError("M06 persistence reconciliation failed")
                        result["m06_reconciliation"] = "PASS"
                    if content_journey:
                        base_env = dict(QA_IDENTITY_BINARY=str(identity_binary), QA_ISSUER_BINARY=str(issuer_binary),
                            QA_BRIDGE_PUBLIC=(directory / "issuer-public.txt").read_text())
                        # API denial checks use an actual resolver and a localhost-only
                        # allowlist; policy must reject before any internal connection.
                        egress_state = json.loads(sql("qa_cp05", "SELECT jsonb_object_agg(l.subject,jsonb_build_object('id',l.user_id,'version',l.version,'active',l.active,'site_role_version',r.version)) FROM v3identity.links l JOIN v3identity.site_roles r ON r.user_id=l.user_id"))
                        write_new_json(directory / "m06cp03-state.json", egress_state)
                        egress_cover = directory / "m06cp03-api-cover"
                        egress_cover.mkdir()
                        env.update(base_env, QA_CHECKPOINT="m06cp03", QA_FILE_ROOT=str(directory / "m06cp03-private-objects"),
                            QA_CASE_RESULTS=str(directory / "m06cp03-api-cases.json"), GOCOVERDIR=str(egress_cover))
                        command([sys.executable, "-m", "pytest", "-p", "qa.plugin", "qa/tests/egress_api", "-q"], 120)
                        egress_cases = json.loads((directory / "m06cp03-api-cases.json").read_text())
                        expected = set(manifest["m06_cp03_python"])
                        if (egress_cases["exit_status"] != 0 or set(egress_cases["collected_ids"]) != expected or len(egress_cases["cases"]) != len(expected) or any(case["status"] != "PASS" for case in egress_cases["cases"])):
                            raise ValueError("M06-CP03 Playwright cases incomplete")
                        egress_profile = directory / "m06cp03-api-coverage.out"
                        command(["go", "tool", "covdata", "textfmt", "-i=" + str(egress_cover), "-o=" + str(egress_profile)])
                        require_coverage(egress_profile)
                        result["m06_cp03_api"] = {"status": "PASS_SYNTHETIC_ONLY", "cases": egress_cases["cases"],
                            "instrumented_go_coverage": command(["go", "tool", "cover", "-func=" + str(egress_profile)]).decode().strip()}

                        sql("qa_cp05", "GRANT SELECT,INSERT,UPDATE ON v3.content_sections,v3.content_activities TO qa_runtime; GRANT SELECT,INSERT ON v3.content_commands TO qa_runtime")
                        content_state = json.loads(sql("qa_cp05", "SELECT jsonb_object_agg(l.subject,jsonb_build_object('id',l.user_id,'version',l.version,'active',l.active,'site_role_version',r.version)) FROM v3identity.links l JOIN v3identity.site_roles r ON r.user_id=l.user_id"))
                        write_new_json(directory / "m07-state.json", content_state)
                        content_cover = directory / "m07-api-cover"
                        content_cover.mkdir()
                        env.update(base_env, QA_CHECKPOINT="m07", QA_CASE_RESULTS=str(directory / "m07-api-cases.json"), GOCOVERDIR=str(content_cover))
                        command([sys.executable, "-m", "pytest", "-p", "qa.plugin", "qa/tests/content_api", "-q"], 150)
                        content_cases = json.loads((directory / "m07-api-cases.json").read_text())
                        expected = set(manifest["m07_cp01_python"])
                        if (content_cases["exit_status"] != 0 or set(content_cases["collected_ids"]) != expected or len(content_cases["cases"]) != len(expected) or any(case["status"] != "PASS" for case in content_cases["cases"])):
                            raise ValueError("M07-CP01 Playwright cases incomplete")
                        content_profile = directory / "m07-api-coverage.out"
                        command(["go", "tool", "covdata", "textfmt", "-i=" + str(content_cover), "-o=" + str(content_profile)])
                        require_coverage(content_profile)
                        if "lmsv3/internal/content/" not in content_profile.read_text(encoding="utf-8"):
                            raise ValueError("M07 content coverage missing")
                        result["m07_api"] = {"status": "PASS_SYNTHETIC_ONLY", "cases": content_cases["cases"],
                            "instrumented_go_coverage": command(["go", "tool", "cover", "-func=" + str(content_profile)]).decode().strip()}
                        for query in (
                            "SELECT count(*) FROM v3.content_sections GROUP BY class_id,position HAVING count(*)>1",
                            "SELECT count(*) FROM v3.content_activities GROUP BY section_id,position HAVING count(*)>1",
                            "SELECT count(*) FROM v3.content_activities a LEFT JOIN v3.content_sections s ON s.id=a.section_id AND s.class_id=a.class_id WHERE s.id IS NULL",
                            "SELECT count(*) FROM v3.audit a LEFT JOIN v3.outbox o ON o.event_id=a.id WHERE a.action LIKE 'content.%' AND o.event_id IS NULL",
                        ):
                            rows = sql("qa_cp05", query).decode().splitlines()
                            if rows and any(value != "0" for value in rows):
                                raise ValueError("M07 persistence reconciliation failed")
                        result["m07_reconciliation"] = "PASS"
                        if content_safety_journey:
                            if content_contract_journey:
                                sql("qa_cp05", "REVOKE UPDATE ON v3.content_revisions FROM qa_runtime; GRANT SELECT,INSERT ON v3.content_revisions TO qa_runtime; GRANT UPDATE(canonical_body,timezone,available_from,due_at,cutoff_at,state,version,updated_at) ON v3.content_revisions TO qa_runtime; REVOKE UPDATE ON v3.content_revision_sources FROM qa_runtime; GRANT INSERT ON v3.content_revision_sources TO qa_runtime; GRANT SELECT(revision_id) ON v3.content_revision_sources TO qa_runtime; GRANT UPDATE(raw_body,raw_sha256,updated_at) ON v3.content_revision_sources TO qa_runtime; GRANT SELECT,INSERT ON v3.assignment_revision_rules TO qa_runtime; GRANT UPDATE(submission_mode,max_attempts,max_files,max_file_bytes,late_policy,version,updated_at) ON v3.assignment_revision_rules TO qa_runtime")
                                env["QA_CONTENT_CONTRACTS"] = "required"
                            else:
                                sql("qa_cp05", "GRANT SELECT,INSERT,UPDATE ON v3.content_revisions TO qa_runtime; GRANT INSERT,UPDATE ON v3.content_revision_sources TO qa_runtime")
                            safety_state = json.loads(sql("qa_cp05", "SELECT jsonb_object_agg(l.subject,jsonb_build_object('id',l.user_id,'version',l.version,'active',l.active,'site_role_version',r.version)) FROM v3identity.links l JOIN v3identity.site_roles r ON r.user_id=l.user_id"))
                            write_new_json(directory / "m07cp02-state.json", safety_state)
                            safety_cover = directory / "m07cp02-api-cover"
                            safety_cover.mkdir()
                            env.update(base_env, QA_CHECKPOINT="m07cp02", QA_CASE_RESULTS=str(directory / "m07cp02-api-cases.json"), GOCOVERDIR=str(safety_cover))
                            command([sys.executable, "-m", "pytest", "-p", "qa.plugin", "qa/tests/content_safety_api", "-q"], 180)
                            safety_cases = json.loads((directory / "m07cp02-api-cases.json").read_text())
                            expected = set(manifest["m07_cp02_python"])
                            if (safety_cases["exit_status"] != 0 or set(safety_cases["collected_ids"]) != expected or len(safety_cases["cases"]) != len(expected) or any(case["status"] != "PASS" for case in safety_cases["cases"])):
                                raise ValueError("M07-CP02 Playwright cases incomplete")
                            safety_profile = directory / "m07cp02-api-coverage.out"
                            command(["go", "tool", "covdata", "textfmt", "-i=" + str(safety_cover), "-o=" + str(safety_profile)])
                            require_coverage(safety_profile)
                            if "lmsv3/internal/content/" not in safety_profile.read_text(encoding="utf-8"):
                                raise ValueError("M07-CP02 content coverage missing")
                            result["m07_cp02_api"] = {"status": "PASS_SYNTHETIC_ONLY", "cases": safety_cases["cases"],
                                "instrumented_go_coverage": command(["go", "tool", "cover", "-func=" + str(safety_profile)]).decode().strip()}
                            for query in (
                                "SELECT count(*) FROM v3.content_revisions r LEFT JOIN v3.content_revision_sources s ON s.revision_id=r.id WHERE s.revision_id IS NULL",
                                "SELECT count(*) FROM v3.content_revisions WHERE due_at<available_from OR cutoff_at<due_at OR cutoff_at<available_from",
                                "SELECT count(*) FROM v3.audit a LEFT JOIN v3.outbox o ON o.event_id=a.id WHERE a.action LIKE 'content.%revision' AND o.event_id IS NULL",
                            ):
                                rows = sql("qa_cp05", query).decode().splitlines()
                                if rows and any(value != "0" for value in rows):
                                    raise ValueError("M07-CP02 persistence reconciliation failed")
                            result["m07_cp02_reconciliation"] = "PASS"
                            if content_contract_journey:
                                contract_state = json.loads(sql("qa_cp05", "SELECT jsonb_object_agg(l.subject,jsonb_build_object('id',l.user_id,'version',l.version,'active',l.active,'site_role_version',r.version)) FROM v3identity.links l JOIN v3identity.site_roles r ON r.user_id=l.user_id"))
                                write_new_json(directory / "m07cp03-state.json", contract_state)
                                contract_cover = directory / "m07cp03-api-cover"
                                contract_cover.mkdir()
                                env.update(base_env, QA_CHECKPOINT="m07cp03", QA_CONTENT_CONTRACTS="required",
                                           QA_CASE_RESULTS=str(directory / "m07cp03-api-cases.json"), GOCOVERDIR=str(contract_cover))
                                command([sys.executable, "-m", "pytest", "-p", "qa.plugin", "qa/tests/content_contract_api", "-q"], 210)
                                contract_cases = json.loads((directory / "m07cp03-api-cases.json").read_text())
                                expected = set(manifest["m07_cp03_python"])
                                if (contract_cases["exit_status"] != 0 or set(contract_cases["collected_ids"]) != expected or
                                        len(contract_cases["cases"]) != len(expected) or any(case["status"] != "PASS" for case in contract_cases["cases"])):
                                    raise ValueError("M07-CP03 Playwright cases incomplete")
                                contract_profile = directory / "m07cp03-api-coverage.out"
                                command(["go", "tool", "covdata", "textfmt", "-i=" + str(contract_cover), "-o=" + str(contract_profile)])
                                require_coverage(contract_profile)
                                if "lmsv3/internal/content/" not in contract_profile.read_text(encoding="utf-8"):
                                    raise ValueError("M07-CP03 content coverage missing")
                                result["m07_cp03_api"] = {"status": "PASS_SYNTHETIC_ONLY", "cases": contract_cases["cases"],
                                    "instrumented_go_coverage": command(["go", "tool", "cover", "-func=" + str(contract_profile)]).decode().strip()}
                                for query in (
                                    "SELECT count(*) FROM v3.assignment_revision_rules ar JOIN v3.content_activities a ON a.id=ar.activity_id AND a.class_id=ar.class_id WHERE a.activity_type<>'ASSIGNMENT'",
                                    "SELECT count(*) FROM v3.assignment_revision_rules ar LEFT JOIN v3.content_revisions cr ON cr.id=ar.revision_id AND cr.class_id=ar.class_id AND cr.activity_id=ar.activity_id WHERE cr.id IS NULL",
                                    "SELECT count(*) FROM v3.content_revisions cr JOIN v3.content_activities a ON a.id=cr.activity_id AND a.class_id=cr.class_id LEFT JOIN v3.assignment_revision_rules ar ON ar.revision_id=cr.id WHERE a.activity_type='ASSIGNMENT' AND cr.created_at>=(SELECT applied_at FROM v3ops.migrations WHERE version='m07-cp03-1') AND ar.revision_id IS NULL",
                                    "SELECT count(*) FROM v3.audit a LEFT JOIN v3.outbox o ON o.event_id=a.id WHERE a.action LIKE 'content.%revision' AND o.event_id IS NULL",
                                ):
                                    rows = sql("qa_cp05", query).decode().splitlines()
                                    if rows and any(value != "0" for value in rows):
                                        raise ValueError("M07-CP03 persistence reconciliation failed")
                                result["m07_cp03_reconciliation"] = "PASS"
            if source != source_fingerprint(ROOT):
                raise ValueError("source changed during integration")
            result["status"] = "PASS_SYNTHETIC_ONLY"
        except (OSError, ValueError) as error:
            result["failure"] = str(error).replace(password, "[REDACTED]")
        finally:
            # A storage refusal must never prevent the bounded database stop.
            storage_monitoring = False
            if started:
                try:
                    # The retained multi-database fixture can need more than
                    # 15 seconds for its final checkpoint on Windows disks.
                    # Keep a bounded fast stop; do not force-kill or hide failure.
                    command([tools["pg_ctl"], "-D", data, "-m", "fast", "-w", "-t", "45", "stop"], 55)
                except (OSError, ValueError):
                    result["status"] = "FAIL_STOP_REQUIRES_OPERATOR"
            write_new_json(directory / "summary.json", result)
            # Keep all raw bytes, but avoid retaining each stopped cluster at
            # its uncompressed size. Failures stay visible in a separate receipt;
            # storage housekeeping cannot turn a failed QA run into PASS.
            if started and result["status"] != "FAIL_STOP_REQUIRES_OPERATOR" and os.name == "nt":
                print(f"[qa] lossless storage compaction after stop; run {run_id}", flush=True)
                try:
                    storage.compact_run(directory, [parent], pg_bin, parent / "storage-receipts")
                except (OSError, ValueError, subprocess.TimeoutExpired) as error:
                    write_new_json(directory / "storage-retained.json", {"action":"RETAINED_REQUIRES_REVIEW",
                                   "error_type":type(error).__name__, "database_deletion":"NONE"})
        print(json.dumps({"run_id": run_id, "status": result["status"], "cluster_retained": True}))
        return 0 if result["status"] == "PASS_SYNTHETIC_ONLY" else 1


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--create-local-synthetic-cluster", action="store_true", required=True)
    parser.add_argument("--pg-bin", type=Path, required=True)
    parser.add_argument("--class-journey", action="store_true", help="Also execute CP08-CP10 on the new synthetic cluster only")
    parser.add_argument("--identity-journey", action="store_true", help="Execute CP11 then CP12 plus CP08-CP10 regression; new local cluster only")
    parser.add_argument("--runtime-journey", action="store_true", help="Execute M03-CP03/CP04 plus all identity/Class regressions on a new local cluster")
    parser.add_argument("--worker-journey", action="store_true", help="Execute M03-CP05/CP06 local scope plus all prior regressions on a new local cluster")
    parser.add_argument("--participation-journey", action="store_true", help="Execute M05-CP03/CP04 plus all backend dependency regressions on a new local cluster")
    parser.add_argument("--file-journey", action="store_true", help="Execute M06-CP01/CP02 plus all backend dependency regressions on a new local cluster")
    parser.add_argument("--content-journey", action="store_true", help="Execute M06-CP03 and M07-CP01 plus all backend dependency regressions on a new local cluster")
    parser.add_argument("--content-safety-journey", action="store_true", help="Execute M07-CP02 plus the full content and backend dependency chain on a new local cluster")
    parser.add_argument("--content-contract-journey", action="store_true", help="Execute M07-CP03 plus all content and backend dependency regressions on a new local cluster")
    parser.add_argument("--submission-journey", action="store_true", help="Execute M08-CP01/CP04 plus the full backend dependency chain on a new local cluster")
    parser.add_argument("--migration-foundation-journey", action="store_true", help="Execute synthetic M15-CP01/CP02 after M08 and the full dependency chain on a new local cluster")
    parser.add_argument("--migration-backfill-journey", action="store_true", help="Execute synthetic M15-CP01/CP04 after M08 and the full dependency chain on a new local cluster")
    parser.add_argument("--migration-completion-journey", action="store_true", help="Execute synthetic M15-CP01/CP05 then M11-CP01 with full backend dependency chain on a new local cluster")
    parser.add_argument("--logbook-period-journey", action="store_true", help="Execute synthetic M12-CP01 after M11 and all backend dependency regressions on a new local cluster")
    parser.add_argument("--logbook-lifecycle-journey", action="store_true", help="Execute synthetic M12-CP01 through M12-CP03 after M11 and all backend dependency regressions on a new local cluster")
    parser.add_argument("--notification-journey", action="store_true", help="Execute synthetic M13-CP01/CP02 after the complete backend dependency chain")
    parser.add_argument("--gradebook-journey", action="store_true", help="Execute synthetic M09-CP01 through CP04 after the complete backend dependency chain")
    parser.add_argument("--completion-journey", action="store_true", help="Execute synthetic M10-CP01 after M09 and the complete backend dependency chain")
    parser.add_argument("--completion-correction-journey", action="store_true", help="Execute synthetic M10-CP01/CP02 with full dependency regression")
    parser.add_argument("--credential-journey", action="store_true", help="Execute synthetic M10-CP01/CP03 with full dependency regression")
    parser.add_argument("--legacy-output-journey", action="store_true", help="Execute synthetic M10-CP01/CP04 with full dependency regression")
    parser.add_argument("--class-academic-journey", action="store_true", help="Execute synthetic M05-CP05 integration after M10-CP04 and full dependency regression")
    parser.add_argument("--content-closure-journey", action="store_true", help="Execute synthetic M07-CP04 after M05-CP05 and full dependency regression")
    parser.add_argument("--ai-suggestion-journey", action="store_true", help="Execute synthetic M13-CP03 after M07-CP04 and full dependency regression")
    parser.add_argument("--consumer-closure-journey", action="store_true", help="Execute synthetic M13-CP04 after M13-CP03")
    parser.add_argument("--reporting-journey", action="store_true", help="Execute synthetic M14-CP01 after M13-CP04")
    parser.add_argument("--reporting-contract-journey", action="store_true", help="Execute synthetic M14-CP02 after M14-CP01")
    parser.add_argument("--reporting-pagination-journey", action="store_true", help="Execute synthetic M14-CP03 after CP01/02 and all dependency regressions")
    parser.add_argument("--reporting-performance-journey", action="store_true", help="Execute frozen M14-CP04 native load/plan/pool checks plus all backend dependencies; new local cluster only")
    parser.add_argument("--artifact-parent", type=Path, help="Optional local output directory named lms-v3-qa-artifacts; never a database input")
    args = parser.parse_args()
    if args.reporting_performance_journey:
        args.reporting_pagination_journey = True
    if args.reporting_pagination_journey:
        args.reporting_contract_journey = True
    if args.reporting_contract_journey:
        args.reporting_journey = True
    if args.reporting_journey:
        args.consumer_closure_journey = True
    if args.consumer_closure_journey:
        args.ai_suggestion_journey = True
    if args.ai_suggestion_journey:
        args.content_closure_journey = True
    if args.content_closure_journey:
        args.class_academic_journey = True
    if args.class_academic_journey:
        args.legacy_output_journey = True
    if args.legacy_output_journey:
        args.credential_journey = True
    if args.credential_journey:
        args.completion_correction_journey = True
    if args.completion_correction_journey:
        args.completion_journey = True
    migration = args.migration_foundation_journey or args.migration_backfill_journey or args.migration_completion_journey or args.logbook_period_journey or args.logbook_lifecycle_journey or args.notification_journey or args.gradebook_journey or args.completion_journey
    return run_lab(args.pg_bin,
        args.class_journey or args.identity_journey or args.runtime_journey or args.worker_journey or args.participation_journey or args.file_journey or args.content_journey or args.content_safety_journey or args.content_contract_journey or args.submission_journey or migration,
        args.identity_journey or args.runtime_journey or args.worker_journey or args.participation_journey or args.file_journey or args.content_journey or args.content_safety_journey or args.content_contract_journey or args.submission_journey or migration,
        args.runtime_journey or args.worker_journey or args.participation_journey or args.file_journey or args.content_journey or args.content_safety_journey or args.content_contract_journey or args.submission_journey or migration,
        args.worker_journey or args.participation_journey or args.file_journey or args.content_journey or args.content_safety_journey or args.content_contract_journey or args.submission_journey or migration,
        args.participation_journey or args.file_journey or args.content_journey or args.content_safety_journey or args.content_contract_journey or args.submission_journey or migration,
        args.file_journey or args.content_journey or args.content_safety_journey or args.content_contract_journey or args.submission_journey or migration,
        args.content_journey or args.content_safety_journey or args.content_contract_journey or args.submission_journey or migration,
        args.content_safety_journey or args.content_contract_journey or args.submission_journey or migration,
        args.content_contract_journey or args.submission_journey or migration,
        args.submission_journey or migration,
        migration,
        args.migration_backfill_journey or args.migration_completion_journey or args.logbook_period_journey or args.logbook_lifecycle_journey or args.notification_journey or args.gradebook_journey or args.completion_journey,
        args.migration_completion_journey or args.logbook_period_journey or args.logbook_lifecycle_journey or args.notification_journey or args.gradebook_journey or args.completion_journey,
        args.logbook_period_journey or args.logbook_lifecycle_journey or args.notification_journey or args.gradebook_journey or args.completion_journey,
        args.logbook_lifecycle_journey or args.notification_journey or args.gradebook_journey or args.completion_journey,
        args.notification_journey or args.gradebook_journey or args.completion_journey,
        args.gradebook_journey or args.completion_journey,
        args.completion_journey,
        args.completion_correction_journey,
        args.credential_journey,
        args.legacy_output_journey,
        args.class_academic_journey,
        args.content_closure_journey,
        args.ai_suggestion_journey,
        args.artifact_parent,
        args.consumer_closure_journey,
        args.reporting_journey,
        args.reporting_contract_journey,
        args.reporting_pagination_journey,
        args.reporting_performance_journey)


if __name__ == "__main__":
    raise SystemExit(main())
