import pytest

from qa import local_recovery_run as recovery
from qa.run import ROOT


@pytest.mark.case('M01-local-recovery-runner-fences')
def test_no_ack_or_failed_crypto_cannot_start_database_or_accept_external_targets(qa_scratch, monkeypatch):
    recovery.manifest(ROOT)
    monkeypatch.setattr(recovery.age_stream_run, 'execute', lambda **kw: pytest.fail('crypto reached without acknowledgement'))
    with pytest.raises(ValueError, match='EXPLICIT_NEW_CLUSTER_ACK'):
        recovery.run(qa_scratch, acknowledged=False)
    monkeypatch.setattr(recovery.age_stream_run, 'execute', lambda **kw: 2)
    monkeypatch.setattr(recovery.subprocess, 'run', lambda *a, **kw: pytest.fail('database/tool process reached after failed crypto'))
    with pytest.raises(ValueError, match='CRYPTO_GATE_NOT_PASS'):
        recovery.run(qa_scratch, acknowledged=True)
    for name in ('postgresql://staging.invalid/live', 'production', 'postgres', 'qa_recovery_source; SELECT 1'):
        with pytest.raises(ValueError): recovery.database_name(name)
    for name in recovery.DATABASES: assert recovery.database_name(name) == name
    occupied=qa_scratch/'occupied';occupied.mkdir();(occupied/'retain.txt').write_text('synthetic')
    with pytest.raises(ValueError,match='EXISTS_OR_LINKED'): recovery.private_mkdir(occupied)
    assert (occupied/'retain.txt').read_text()=='synthetic'
