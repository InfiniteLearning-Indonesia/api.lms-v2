"""Start only the freshly built local-synthetic Go API, never an existing service."""
import json
import os
from pathlib import Path
import subprocess
import time
import pytest
from playwright.sync_api import sync_playwright
from qa.preflight import require_local_api


@pytest.fixture(scope="session")
def local_service():
    binary = Path(os.environ["QA_API_BINARY"]).resolve(strict=True)
    artifacts = Path(os.environ["QA_ARTIFACT_ROOT"]).resolve(strict=True)
    assert binary.is_relative_to(artifacts), "API binary must belong to this run"
    log_path = artifacts / "api-runtime.jsonl"
    env = {k: v for k, v in os.environ.items() if k.upper() in {
        "PATH", "SYSTEMROOT", "WINDIR", "TEMP", "TMP", "TMPDIR", "COMSPEC"}}
    env.update(LMS_MODE="local-synthetic", LMS_HTTP_ADDR="127.0.0.1:0")
    with log_path.open("x", encoding="utf-8") as logs:
        process = subprocess.Popen([str(binary)], env=env, stdout=logs, stderr=logs)
        try:
            deadline = time.monotonic() + 10
            address = None
            while time.monotonic() < deadline:
                assert process.poll() is None, "local API exited before readiness handshake"
                lines = log_path.read_text(encoding="utf-8").splitlines()
                if lines:
                    first = json.loads(lines[0])
                    assert first["msg"] == "local_server_started"
                    assert first["mode"] == "local-synthetic"
                    address = "http://" + first["address"]
                    break
                time.sleep(0.05)
            assert address, "local API did not start within timeout"
            require_local_api(address)
            yield address, log_path
        finally:
            if process.poll() is None:
                process.terminate()  # Only this helper process; no DB/test-data cleanup.
            try:
                process.wait(timeout=5)
            except subprocess.TimeoutExpired:
                process.kill()
                process.wait(timeout=5)


@pytest.fixture(scope="session")
def api(local_service):
    address, _ = local_service
    with sync_playwright() as playwright:
        context = playwright.request.new_context(base_url=address, timeout=3000,
                                                 ignore_https_errors=False)
        try:
            yield context
        finally:
            context.dispose()
