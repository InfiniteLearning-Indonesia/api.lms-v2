import json
import os
from pathlib import Path
import subprocess
import sys

import pytest

from qa import age_stream_run as runner, storage
from qa.run import ROOT, clean_environment


@pytest.mark.case('M01-age-runner-probe-no-authority')
def test_real_probe_has_no_download_crypto_or_authority():
    result = subprocess.run([sys.executable, '-m', 'qa.age_stream_run', '--probe-only', '--no-download'],
                            cwd=ROOT, env=clean_environment(ROOT, os.environ), capture_output=True, text=True, timeout=30)
    value = json.loads(result.stdout)
    assert result.returncode == 2 and value['crypto_suite'] == 'NOT_RUN'
    assert value['download_attempted'] is value['authorizes_execution'] is value['actual_restore_verified'] is False
    assert value['database_access'] == value['proxy_changes'] == 'NONE'


@pytest.mark.case('M01-age-runner-evidence-and-admission')
def test_missing_skipped_stale_and_unknown_storage_cannot_pass_or_install(qa_scratch, monkeypatch):
    frozen = runner.manifest(ROOT)
    cases = [{'package': i.split(':')[0], 'name': i.split(':')[1], 'status': 'pass'} for i in frozen['expected_tests']]
    runner.review_cases(cases, frozen['expected_tests'], 'a' * 64, 'a' * 64)
    for bad in ([], cases[:-1], cases + cases[:1], [{**c, 'status': 'skip'} for c in cases]):
        with pytest.raises(ValueError): runner.review_cases(bad, frozen['expected_tests'], 'a' * 64, 'a' * 64)
    with pytest.raises(ValueError): runner.review_cases(cases, frozen['expected_tests'], 'a' * 64, 'b' * 64)
    monkeypatch.setattr(storage, 'project_locations', lambda s: [qa_scratch])
    monkeypatch.setattr(storage, 'preflight', lambda *a: (_ for _ in ()).throw(ValueError('UNKNOWN_BYTES')))
    monkeypatch.setattr(runner, 'acquire_tools', lambda *a: pytest.fail('installer reached without storage admission'))
    with pytest.raises(ValueError, match='UNKNOWN_BYTES'): runner.execute(allow_install=True)
    monkeypatch.setenv('HTTPS_PROXY', 'http://normal-proxy.invalid:3128')
    env = runner.download_environment(ROOT)
    assert env['HTTPS_PROXY'] == 'http://normal-proxy.invalid:3128'
    assert env['GOSUMDB'] == 'sum.golang.org' and Path(env['GOPATH']).is_relative_to(ROOT)
