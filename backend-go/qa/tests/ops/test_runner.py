import json
import os
from pathlib import Path
import subprocess
import sys
import pytest
from qa.run import ROOT, clean_environment, run_lock


@pytest.mark.case("Q00-runner-lock-and-env")
def test_lock_releases_and_child_environment_excludes_credentials():
    path = Path(os.environ["QA_ARTIFACT_ROOT"]) / "lock-selftest.lock"
    with run_lock(path):
        with pytest.raises(OSError):
            with run_lock(path):
                pytest.fail("second run acquired an occupied lock")
    with run_lock(path):
        assert path.exists()
    clean = clean_environment(ROOT, {"PATH": "safe-path", "DB_PASSWORD": "DO_NOT_EMIT",
                                    "DATABASE_URL": "DO_NOT_EMIT", "SUPABASE_SERVICE_ROLE_KEY": "DO_NOT_EMIT",
                                    "PYTEST_ADDOPTS": "--ignore=qa/tests", "GOFLAGS": "-skip-tests"})
    assert "DO_NOT_EMIT" not in json.dumps(clean)
    assert "PYTEST_ADDOPTS" not in clean and "GOFLAGS" not in clean
    assert clean["GOPROXY"] == "off" and clean["GOTOOLCHAIN"] == "go1.27.1"


@pytest.mark.case("Q00-runner-plugin-evidence")
def test_real_pytest_plugin_records_fail_skip_error_and_duplicate_collection():
    artifacts = Path(os.environ["QA_ARTIFACT_ROOT"]) / "plugin-selftest"
    artifacts.mkdir()
    fixture = artifacts / "test_fixture.py"
    # These are generated synthetic QA inputs, never business/database records.
    fixture.write_text('''import pytest
@pytest.mark.case("pass")
def test_pass(): assert True
@pytest.mark.case("fail")
def test_fail(): assert False
@pytest.mark.case("skip")
def test_skip(): pytest.skip("synthetic skip")
@pytest.fixture
def broken(): raise RuntimeError("synthetic setup error")
@pytest.mark.case("error")
def test_error(broken): pass
@pytest.mark.case("xpass")
@pytest.mark.xfail(reason="synthetic unexpected pass", strict=False)
def test_xpass(): pass
''', encoding="utf-8")
    env = clean_environment(ROOT, os.environ)
    destination = artifacts / "cases.json"
    env["QA_CASE_RESULTS"] = str(destination)
    command = [sys.executable, "-m", "pytest", "-p", "qa.plugin", "-q", str(fixture)]
    run = subprocess.run(command, cwd=ROOT, env=env, capture_output=True, text=True, timeout=20)
    assert run.returncode == 1
    evidence = json.loads(destination.read_text(encoding="utf-8"))
    assert {r["id"]: r["status"] for r in evidence["cases"]} == {
        "pass": "PASS", "fail": "FAIL", "skip": "SKIPPED", "error": "ERROR", "xpass": "FAIL"}
    duplicate = artifacts / "test_duplicate.py"
    duplicate.write_text('''import pytest
@pytest.mark.case("duplicate")
def test_one(): pass
@pytest.mark.case("duplicate")
def test_two(): pass
''', encoding="utf-8")
    env["QA_CASE_RESULTS"] = str(artifacts / "duplicate.json")
    run = subprocess.run(command[:-1] + [str(duplicate)], cwd=ROOT, env=env,
                         capture_output=True, text=True, timeout=20)
    assert run.returncode != 0
    assert "duplicate QA case ID" in run.stderr
