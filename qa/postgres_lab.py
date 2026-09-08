"""Opt-in fresh loopback PostgreSQL lab. Retains all clusters, rows and backups.

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
from qa.evidence import source_fingerprint, write_new_json, verify_go_cases
from qa.run import ROOT, clean_environment, run_lock

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


def run_lab(pg_bin, class_journey=False, identity_journey=False):
    tools = validate_tools(pg_bin)
    if len(tools) != 6:
        raise ValueError("PostgreSQL tools unavailable")
    parent = ROOT / ".qa-artifacts"
    parent.mkdir(exist_ok=True)
    with run_lock(parent / "postgres-lab.lock"):
        run_id = datetime.now(timezone.utc).strftime("%Y%m%dT%H%M%SZ") + "-postgres-lab-" + uuid.uuid4().hex[:12]
        directory = parent / run_id
        directory.mkdir()
        source = source_fingerprint(ROOT)
        data = directory / "postgres"
        password = secrets.token_hex(32)
        marker = secrets.token_hex(32)
        password_file = directory / "synthetic-password.private"
        with password_file.open("x", encoding="utf-8") as f:
            f.write(password)
        env = clean_environment(ROOT, os.environ)
        env.update(PGPASSWORD=password, PGCONNECT_TIMEOUT="3", PGSSLMODE="disable", PGOPTIONS="-c statement_timeout=5000")
        with socket.socket() as sock:
            sock.bind(("127.0.0.1", 0))
            port = sock.getsockname()[1]
        connection = ["--host=127.0.0.1", "--port=" + str(port), "--username=qa_fixture", "--no-password"]
        result = dict(run_id=run_id, source_hash=source, status="FAIL", profile="new-local-synthetic",
                      shared_database_access="NONE", database_deletion="NONE", cleanup="STOP_PROCESS_RETAIN_CLUSTER",
                      production_backup="NOT_EXECUTED", migration_authority=False)
        started = False
        counter = 0

        def command(args, timeout=45, expect_failure=False):
            nonlocal counter
            counter += 1
            try:
                code, output, error_output = run_to_files(args, directory, counter, env, timeout)
                # Fixture-only output; never record passwords, DSNs or args.
                stderr = error_output.decode("utf-8", "replace").replace(password, "[REDACTED]")
                write_new_json(directory / f"command-{counter:02}.json", {"tool": Path(str(args[0])).name,
                               "exit_code": code, "expected_failure": expect_failure, "stderr": stderr})
                require_outcome(code, expect_failure)
                return output
            except subprocess.TimeoutExpired:
                write_new_json(directory / f"timeout-{counter:02}.json", {"status": "TIMEOUT"})
                raise ValueError("bounded synthetic command timed out")

        def sql(database, text):
            return command([tools["psql"], *connection, "--dbname=" + database, "-X", "-A", "-t", "-v", "ON_ERROR_STOP=1", "-c", text]).strip()

        def snapshot(database, name):
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
            if name in ("pre_cp11", "pre_cp12"):
                for compare in CATALOG_QUERIES:
                    compare = compare.replace("('public','qa_cp05')", "('public','qa_cp05','v3','v3ops','v3identity')")
                    if sql(database, compare) != sql(proof, compare):
                        raise ValueError("lifecycle restore catalog mismatch")
                tables = sql(database, "SELECT schemaname||'.'||tablename FROM pg_tables WHERE schemaname IN ('v3','v3ops','v3identity') ORDER BY 1").decode().splitlines()
                import re
                for table in tables:
                    if not re.fullmatch(r"v3(?:ops|identity)?\.[a-z_]+", table):
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
                env.update(QA_LAB_DIR=str(directory), QA_RUNTIME_PASSWORD=secrets.token_hex(32))
                target = sha256(("127.0.0.1\0" + str(port) + "\0qa_cp05\0" + marker).encode()).hexdigest()
                write_new_json(directory / "pre-v3-proof.json", dict(run_id=run_id, target=target,
                    created_at=datetime.now(timezone.utc).isoformat(), restored=True,
                    archive_hash=sha256((directory / "pre_v3.dump").read_bytes()).hexdigest()))
                labels = ("admin", "teacher", "teacher2", "student", "outsider", "disabled", "expired", "revoked", "student2")
                write_new_json(directory / "actors.private.json", {label: dict(id=f"{i:032x}", token=secrets.token_hex(32)) for i, label in enumerate(labels, 1)})
                manifest = json.loads((ROOT / "qa/manifests/CP08-CP10.json").read_text())

                def slice_go(checkpoint, package, test):
                    output = command(["go", "test", "-json", "-count=1", "-tags=qaintegration", "-run=^"+test+"$",
                        "-coverprofile="+str(directory / (checkpoint+"-coverage.out")), package], 120)
                    events = [json.loads(line) for line in output.splitlines()]
                    cases = [dict(package=e["Package"], name=e["Test"], status=e["Action"]) for e in events if e.get("Test") and e.get("Action") in ("pass", "fail", "skip")]
                    if not verify_go_cases(manifest[checkpoint+"_go"], cases):
                        raise ValueError("checkpoint integration incomplete")
                    write_new_json(directory / (checkpoint+"-cases.json"), cases)
                    result[checkpoint] = dict(status="PASS_SYNTHETIC_ONLY", cases=cases)

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
                    command(["go", "build", "-cover", "-coverpkg=./cmd/identity-lab,./internal/identity,./internal/classes,./internal/access", "-trimpath", "-o", identity_binary, "./cmd/identity-lab"],120)
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
            if source != source_fingerprint(ROOT):
                raise ValueError("source changed during integration")
            result["status"] = "PASS_SYNTHETIC_ONLY"
        except (OSError, ValueError) as error:
            result["failure"] = str(error).replace(password, "[REDACTED]")
        finally:
            if started:
                try:
                    command([tools["pg_ctl"], "-D", data, "-m", "fast", "-w", "-t", "15", "stop"], 25)
                except (OSError, ValueError):
                    result["status"] = "FAIL_STOP_REQUIRES_OPERATOR"
            write_new_json(directory / "summary.json", result)
        print(json.dumps({"run_id": run_id, "status": result["status"], "cluster_retained": True}))
        return 0 if result["status"] == "PASS_SYNTHETIC_ONLY" else 1


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--create-local-synthetic-cluster", action="store_true", required=True)
    parser.add_argument("--pg-bin", type=Path, required=True)
    parser.add_argument("--class-journey", action="store_true", help="Also execute CP08-CP10 on the new synthetic cluster only")
    parser.add_argument("--identity-journey", action="store_true", help="Execute CP11 then CP12 plus CP08-CP10 regression; new local cluster only")
    args = parser.parse_args()
    return run_lab(args.pg_bin, args.class_journey or args.identity_journey, args.identity_journey)


if __name__ == "__main__":
    raise SystemExit(main())
