import json
import os
from pathlib import Path
import subprocess
import time
import pytest
from playwright.sync_api import sync_playwright
from qa.preflight import require_local_api


@pytest.fixture(scope="session")
def class_service():
    root = Path(os.environ["QA_ARTIFACT_ROOT"]).resolve(strict=True)
    binary = Path(os.environ["QA_CLASS_BINARY"]).resolve(strict=True)
    assert binary.is_relative_to(root)
    assert os.environ["QA_PG_ACK"] == "new-local-synthetic-cluster"
    env = {k: v for k, v in os.environ.items() if k.upper() in {
        "PATH", "SYSTEMROOT", "WINDIR", "TEMP", "TMP", "COMSPEC",
        "QA_PG_ACK", "QA_PG_PORT", "QA_PG_MARKER", "QA_RUNTIME_PASSWORD", "GOCOVERDIR"}}
    log = root / "class-runtime.jsonl"
    with log.open("x", encoding="utf-8") as output:
        process = subprocess.Popen([str(binary)], env=env, stdin=subprocess.PIPE, stdout=output, stderr=output)
        try:
            deadline = time.monotonic() + 10
            address = None
            while time.monotonic() < deadline:
                assert process.poll() is None, "Class lab failed its startup gate"
                lines = log.read_text(encoding="utf-8").splitlines()
                if lines:
                    first = json.loads(lines[0])
                    assert first["msg"] == "class_lab_started" and first["mode"] == "local-synthetic"
                    address = "http://" + first["address"]
                    require_local_api(address)
                    break
                time.sleep(0.05)
            assert address, "bounded startup failed"
            yield address, root, log
        finally:
            if process.poll() is None:
                process.stdin.write(b"shutdown\n")
                process.stdin.flush()
            try:
                process.wait(timeout=7)
            except subprocess.TimeoutExpired:
                process.kill()
                process.wait(timeout=3)
                pytest.fail("Class lab graceful shutdown timed out")
            finally:
                process.stdin.close()
            assert process.returncode == 0, "Class lab exited abnormally"


@pytest.fixture(scope="session")
def clients(class_service):
    address, root, log = class_service
    actors = json.loads((root / "actors.private.json").read_text())
    with sync_playwright() as p:
        contexts = {label: p.request.new_context(base_url=address, timeout=5000,
            extra_http_headers={"Authorization": "Bearer " + a["token"]}) for label, a in actors.items()}
        anonymous = p.request.new_context(base_url=address, timeout=5000)
        try:
            yield contexts, actors, anonymous
        finally:
            for c in contexts.values():
                c.dispose()
            anonymous.dispose()
            contents = log.read_text()
            assert all(a["token"] not in contents for a in actors.values()), "session token leaked in runtime log"
            assert os.environ["QA_RUNTIME_PASSWORD"] not in contents, "DB credential leaked"
