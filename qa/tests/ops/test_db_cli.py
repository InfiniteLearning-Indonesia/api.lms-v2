import os
from pathlib import Path
import subprocess
import pytest
from qa.run import ROOT, clean_environment


@pytest.mark.case("M00-T02-db-cli-offline-gates")
def test_db_inspection_requires_explicit_ack_and_existing_absolute_config():
    env = clean_environment(ROOT, os.environ)
    tracked = subprocess.run(["git", "-c", "core.excludesFile=", "ls-files", "--", ".env.staging", ".env.example"],
                             cwd=ROOT, env=env, capture_output=True, text=True, timeout=5)
    assert tracked.returncode == 0 and tracked.stdout.strip() == "", "private env must not be tracked"
    ignored = subprocess.run(["git", "-c", "core.excludesFile=", "check-ignore", "--", ".env.staging", ".env.example"],
                             cwd=ROOT, env=env, capture_output=True, text=True, timeout=5)
    assert ignored.returncode == 0 and set(ignored.stdout.splitlines()) == {".env.staging", ".env.example"}
    binary = os.environ["QA_DB_INSPECT_BINARY"]
    # Never supply the actual staging config in synthetic QA.
    missing = str(Path(os.environ["QA_ARTIFACT_ROOT"]) / "does-not-exist.env")
    cases = [([], "STAGING_READONLY_ACK_REQUIRED"),
             (["--staging-readonly", "--env-file", "relative.env"], "CONFIG_ABSOLUTE_PATH_REQUIRED"),
             (["--staging-readonly", "--env-file", missing], "CONFIG_FILE_UNAVAILABLE")]
    for args, expected in cases:
        result = subprocess.run([binary, *args], env=env, capture_output=True, text=True, timeout=5)
        assert result.returncode == 2
        assert result.stdout == ""
        assert result.stderr.strip() == expected
