"""Local-only implementation checkpoint. No database/provider/deployment commands.

Exit 0 = full module verified; 1 = automated check failed; 2 = remaining gates.
--checks-only changes ONLY the exit code for a bootstrap CI job, never status.
"""
import argparse
from contextlib import contextmanager
from datetime import datetime, timezone
from hashlib import sha256
import json
import os
from pathlib import Path
import subprocess
import sys
import uuid

from qa.evidence import publish_checkpoint, render_section, source_fingerprint, summarize, verify_go_cases, write_new_json
from qa.observations import reconcile

ROOT = Path(__file__).resolve().parents[1]


def clean_environment(root: Path, inherited: dict) -> dict:
    allowed = {"PATH", "SYSTEMROOT", "WINDIR", "COMSPEC", "PATHEXT", "TEMP", "TMP",
               "TMPDIR", "USERPROFILE", "HOME", "LOCALAPPDATA", "APPDATA", "PROGRAMFILES"}
    env = {k: v for k, v in inherited.items() if k.upper() in allowed}
    env.update(GOTOOLCHAIN="go1.27.1", GOMODCACHE=str(root / ".cache" / "go-mod"),
               GOCACHE=str(root / ".cache" / "go-build"), GOPROXY="off", GOENV="off",
               GOWORK="off", PYTHONPATH=str(root), PYTHONDONTWRITEBYTECODE="1",
               PYTEST_DISABLE_PLUGIN_AUTOLOAD="1", GIT_CONFIG_NOSYSTEM="1",
               GIT_CONFIG_GLOBAL=os.devnull, GIT_CONFIG_COUNT="1",
               GIT_CONFIG_KEY_0="safe.directory", GIT_CONFIG_VALUE_0=root.parent.as_posix())
    return env


@contextmanager
def run_lock(path: Path):
    """OS-released exclusive lock, retained on disk (no stale PID or deletion)."""
    with path.open("a+b") as handle:
        handle.seek(0, os.SEEK_END)
        if handle.tell() == 0:
            handle.write(b"0")
            handle.flush()
        handle.seek(0)
        acquired = False
        try:
            if os.name == "nt":
                import msvcrt
                msvcrt.locking(handle.fileno(), msvcrt.LK_NBLCK, 1)
            else:
                import fcntl
                fcntl.flock(handle.fileno(), fcntl.LOCK_EX | fcntl.LOCK_NB)
            acquired = True
            yield
        finally:
            if acquired:
                handle.seek(0)
                if os.name == "nt":
                    msvcrt.locking(handle.fileno(), msvcrt.LK_UNLCK, 1)
                else:
                    fcntl.flock(handle.fileno(), fcntl.LOCK_UN)


def run_command(args: list[str], env: dict, destination: Path, name: str,
                timeout: int = 180) -> dict:
    try:
        completed = subprocess.run(args, cwd=ROOT, env=env, capture_output=True,
                                   text=True, encoding="utf-8", errors="replace", timeout=timeout)
        record = {"name": name, "return_code": completed.returncode,
                  "stdout": completed.stdout, "stderr": completed.stderr, "status": "FINISHED"}
    except subprocess.TimeoutExpired:
        record = {"name": name, "return_code": None, "status": "TIMEOUT", "stdout": "", "stderr": ""}
    except OSError:
        record = {"name": name, "return_code": None, "status": "COMMAND_UNAVAILABLE", "stdout": "", "stderr": ""}
    write_new_json(destination / f"command-{name}.json", record)
    return record


def execute(package: str, workspace: Path | None, checks_only: bool, update_docs: bool = False,
            observation: Path | None = None, expected_target: str | None = None) -> int:
    if bool(observation) != bool(expected_target):
        raise ValueError("observation requires explicit target fingerprint")
    manifest_path = ROOT / "qa" / "manifests" / f"{package}.json"
    manifest = json.loads(manifest_path.read_text(encoding="utf-8"))
    go_manifest = json.loads((ROOT / "qa/manifests/go-tests.json").read_text(encoding="utf-8"))
    if manifest["package"] != package or manifest["profile"] != "local-synthetic":
        raise ValueError("manifest scope mismatch")
    if package == "M00" and (workspace is None or not workspace.is_absolute() or not workspace.is_dir()):
        raise ValueError("M00 requires explicit absolute local workspace")
    started = datetime.now(timezone.utc)
    run_id = started.strftime("%Y%m%dT%H%M%SZ") + "-" + package + "-" + uuid.uuid4().hex[:12]
    parent = ROOT / ".qa-artifacts"
    parent.mkdir(exist_ok=True)
    with run_lock(parent / "local-synthetic.lock"):
        directory = parent / run_id
        directory.mkdir()
        (directory / "bin").mkdir()
        tested_hash = source_fingerprint(ROOT)
        env = clean_environment(ROOT, os.environ)
        extension = ".exe" if os.name == "nt" else ""
        api_binary = directory / "bin" / ("api" + extension)
        baseline_binary = directory / "bin" / ("baseline" + extension)
        db_inspect_binary = directory / "bin" / ("db-inspect" + extension)
        env.update(QA_API_BINARY=str(api_binary), QA_ARTIFACT_ROOT=str(directory),
                   QA_DB_INSPECT_BINARY=str(db_inspect_binary),
                   QA_CASE_RESULTS=str(directory / "pytest-cases.json"),
                   QA_BASELINE_REPORT=str(directory / "baseline.json"))
        commands = []
        def command(args, name, timeout=180):
            record = run_command(args, env, directory, name, timeout)
            commands.append({k: v for k, v in record.items() if k not in {"stdout", "stderr"}})
            return record

        version = command(["go", "version"], "go-version")
        goroot = command(["go", "env", "GOROOT"], "go-root")
        fmt = {"return_code": None, "stdout": ""}
        if goroot["return_code"] == 0:
            gofiles = sorted(str(p) for base in (ROOT / "cmd", ROOT / "internal") for p in base.rglob("*.go"))
            fmt = command([str(Path(goroot["stdout"].strip()) / "bin" / ("gofmt" + extension)), "-l", *gofiles], "go-format")
        api_build = command(["go", "build", "-trimpath", "-o", str(api_binary), "./cmd/api"], "go-build-api")
        baseline_build = command(["go", "build", "-trimpath", "-o", str(baseline_binary), "./cmd/baseline"], "go-build-baseline")
        db_inspect_build = command(["go", "build", "-trimpath", "-o", str(db_inspect_binary), "./cmd/db-inspect"], "go-build-db-inspect")
        backup_build = command(["go", "build", "-trimpath", "-o", str(directory / "bin" / ("backup-plan" + extension)), "./cmd/backup-plan"], "go-build-backup-plan")
        unit = command(["go", "test", "-json", "-count=1", "-coverprofile=" + str(directory / "go-coverage.out"), "./..."], "go-unit")
        vet = command(["go", "vet", "./..."], "go-vet")
        go_results = []
        for line in unit["stdout"].splitlines():
            try:
                event = json.loads(line)
            except json.JSONDecodeError:
                continue
            if event.get("Test") and event.get("Action") in {"pass", "fail", "skip"}:
                go_results.append({"package": event["Package"], "name": event["Test"], "status": event["Action"]})
        write_new_json(directory / "go-cases.json", {"cases": go_results})
        coverage = command(["go", "tool", "cover", "-func=" + str(directory / "go-coverage.out")], "go-coverage")
        baseline_ok = True
        if package == "M00":
            inventory_result = command([str(baseline_binary), "--workspace", str(workspace)], "source-inventory", 45)
            baseline_ok = inventory_result["return_code"] == 0
            if baseline_ok:
                write_new_json(directory / "baseline.json", json.loads(inventory_result["stdout"]))
        pytest = command([sys.executable, "-m", "pytest", "-p", "qa.plugin", "-q",
                          "--junitxml=" + str(directory / "pytest-junit.xml"), *manifest["pytest_paths"]], "pytest", 120)
        report_path = directory / "pytest-cases.json"
        case_report = json.loads(report_path.read_text(encoding="utf-8")) if report_path.exists() else {}
        checks = [
            ("Q00-go-format", fmt["return_code"] == 0 and not fmt["stdout"].strip()),
            ("Q00-go-build", api_build["return_code"] == 0 and baseline_build["return_code"] == 0 and db_inspect_build["return_code"] == 0 and backup_build["return_code"] == 0),
            ("Q00-go-unit", unit["return_code"] == 0 and verify_go_cases(go_manifest["expected_tests"], go_results)),
            ("Q00-go-vet", vet["return_code"] == 0),
        ]
        cases = [{"id": key, "status": "PASS" if passed else "FAIL"} for key, passed in checks]
        cases += case_report.get("cases", [])
        final_hash = source_fingerprint(ROOT)
        command_ok = (pytest["return_code"] == 0 and case_report.get("exit_status") == 0 and baseline_ok
                      and coverage["return_code"] == 0 and version["return_code"] == 0
                      and "go1.27.1 " in version["stdout"])
        result = summarize(manifest, cases, final_hash, tested_hash, command_ok)
        result.update(schema_version=1, run_id=run_id, package=package, profile="local-synthetic",
                      started_at=started.isoformat(), ended_at=datetime.now(timezone.utc).isoformat(),
                      source_hash=final_hash, tested_source_hash=tested_hash,
                      manifest_sha256=sha256(manifest_path.read_bytes()).hexdigest(),
                      commands=commands, cases=cases, required_gates=manifest["required_gates"],
                      toolchain=version["stdout"].strip(), python=sys.version.split()[0],
                      go_test_case_count=len(go_results), go_statement_coverage=coverage["stdout"].strip().splitlines()[-1:] or ["NOT_MEASURED"],
                      binary_sha256={p.name: sha256(p.read_bytes()).hexdigest() for p in (api_binary, baseline_binary, db_inspect_binary) if p.exists()},
                      database_access="NOT_EXECUTED", backup="NOT_EXECUTED", migration="NOT_EXECUTED",
                      deployment="NOT_DEPLOYED", source_commit="UNCOMMITTED_WORKTREE_HASH_USED",
                      approval="NOT_REQUESTED_OR_GRANTED_BY_THIS_RUN")
        if observation:
            result["operational_observation"] = reconcile(observation, final_hash, expected_target)
        write_new_json(directory / "summary.json", result)
        # Preview only: an interrupted or unreviewed run cannot overwrite human status.
        module = ROOT / "docs" / "modules" / f"{package}.md"
        preview = render_section(module.read_text(encoding="utf-8"), package, result, run_id)
        with (directory / f"{package}.preview.md").open("x", encoding="utf-8") as handle:
            handle.write(preview)
        if update_docs:
            receipt = publish_checkpoint(ROOT, package, result)
            write_new_json(directory / "publication.json", receipt)
        print(json.dumps({k: result[k] for k in ("run_id", "automated_checks", "package_status", "deliverable_coverage")}, indent=2))
        print("Evidence: " + str(directory / "summary.json"))
        if result["automated_checks"] != "PASS":
            return 1
        if checks_only:
            print("BOOTSTRAP CHECKS ONLY: module, release, migration and deployment gates remain blocked.")
            return 0
        return 0 if result["package_status"] == "BACKEND_VERIFIED" else 2


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--package", choices=("M00", "M03"), required=True)
    parser.add_argument("--profile", choices=("local-synthetic",), default="local-synthetic")
    parser.add_argument("--workspace", type=Path)
    parser.add_argument("--checks-only", action="store_true")
    parser.add_argument("--update-docs", action="store_true", help="publish generated local checkpoint sections, never operational approvals")
    parser.add_argument("--observation", type=Path, help="existing explicit operator receipt, never triggers DB access")
    parser.add_argument("--expected-target-fingerprint", help="independently selected target fingerprint; not proof of production identity")
    args = parser.parse_args()
    try:
        return execute(args.package, args.workspace, args.checks_only, args.update_docs,
                       args.observation, args.expected_target_fingerprint)
    except (OSError, ValueError, KeyError):
        print("Checkpoint refused/interrupted: invalid scope, unavailable evidence or occupied local lock. No database action performed.", file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
