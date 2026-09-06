"""Explicit WSL/Linux proof: Go race suite and non-root loopback API smoke.

No Docker daemon, database, provider credentials, install or deployment.
"""
from datetime import datetime, timezone
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import time
import urllib.request
import uuid
from qa.evidence import source_fingerprint, verify_go_cases, write_new_json
from qa.run import ROOT, clean_environment, run_command, run_lock


def main():
    if sys.platform != "linux":
        raise ValueError("Linux required")
    toolchain = ROOT / ".cache/linux-go-1.27.1/go"
    go = toolchain / "bin/go"
    if not go.is_file():
        raise ValueError("verified local Linux toolchain missing")
    parent = ROOT / ".qa-artifacts"
    parent.mkdir(exist_ok=True)
    with run_lock(parent / "linux-qa.lock"):
        run_id = datetime.now(timezone.utc).strftime("%Y%m%dT%H%M%SZ") + "-linux-qa-" + uuid.uuid4().hex[:12]
        directory = parent / run_id
        directory.mkdir()
        source = source_fingerprint(ROOT)
        env = clean_environment(ROOT, os.environ)
        env.update(GOROOT=str(toolchain), GOTOOLCHAIN="local", GOCACHE=str(ROOT / ".cache/go-build-linux"), CGO_ENABLED="1", PATH="/usr/bin:/bin")
        race = run_command([str(go), "test", "-race", "-json", "-count=1", "./..."], env, directory, "race", 300)
        cases = []
        for line in race["stdout"].splitlines():
            event = json.loads(line)
            if event.get("Test") and event.get("Action") in ("pass", "fail", "skip"):
                cases.append(dict(package=event["Package"], name=event["Test"], status=event["Action"]))
        manifest = json.loads((ROOT / "qa/manifests/go-tests.json").read_text())
        race_ok = race["return_code"] == 0 and verify_go_cases(manifest["expected_tests"], cases)
        env["CGO_ENABLED"] = "0"
        binary = directory / "api"
        build = run_command([str(go), "build", "-trimpath", "-buildvcs=false", "-o", str(binary), "./cmd/api"], env, directory, "api-build", 180)
        runtime_ok = False
        uid = None
        if build["return_code"] == 0:
            prefix = []
            if os.getuid() == 0:
                if not shutil.which("setpriv"):
                    raise ValueError("cannot prove non-root runtime without setpriv")
                prefix = ["setpriv", "--reuid=65532", "--regid=65532", "--clear-groups"]
            runtime_env = {"PATH": "/usr/bin:/bin", "LMS_MODE": "local-synthetic", "LMS_HTTP_ADDR": "127.0.0.1:0"}
            log = directory / "runtime.jsonl"
            with log.open("x") as output:
                process = subprocess.Popen([*prefix, str(binary)], env=runtime_env, stdout=output, stderr=output)
                try:
                    deadline = time.monotonic() + 10
                    address = None
                    while time.monotonic() < deadline and process.poll() is None:
                        lines = log.read_text().splitlines()
                        if lines:
                            event = json.loads(lines[0])
                            if event.get("msg") == "local_server_started":
                                address = event["address"]
                                break
                        time.sleep(0.05)
                    if address and address.startswith("127.0.0.1:"):
                        status = Path(f"/proc/{process.pid}/status").read_text()
                        uid = [int(v) for line in status.splitlines() if line.startswith("Uid:") for v in line.split()[1:]]
                        # Disable ambient proxy handling even for loopback smoke.
                        opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
                        with opener.open("http://" + address + "/health/live", timeout=3) as response:
                            runtime_ok = all(v != 0 for v in uid) and response.status == 200 and json.load(response) == {"status": "alive"}
                finally:
                    if process.poll() is None:
                        process.terminate()
                    try:
                        process.wait(timeout=7)
                    except subprocess.TimeoutExpired:
                        process.kill()
                        process.wait(timeout=3)
                        runtime_ok = False
        passed = race_ok and runtime_ok and source == source_fingerprint(ROOT)
        result = dict(run_id=run_id, source_hash=source, status="PASS" if passed else "FAIL",
                      race="PASS" if race_ok else "FAIL", go_cases=len(cases), non_root_runtime=runtime_ok,
                      runtime_uids=uid, container_execution="NOT_RUN", shared_database_access="NONE", deployment="NOT_EXECUTED")
        write_new_json(directory / "summary.json", result)
        print(json.dumps(result))
        return 0 if passed else 1


if __name__ == "__main__":
    raise SystemExit(main())
