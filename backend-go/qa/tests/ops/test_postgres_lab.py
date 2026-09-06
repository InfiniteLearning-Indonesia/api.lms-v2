import pytest
import os
import subprocess
import sys
from qa.postgres_lab import fixture_sql, validate_tools, run_to_files, INITDB_SAFETY_OPTIONS, require_outcome, require_digest, CATALOG_QUERIES
from hashlib import sha256


@pytest.mark.case("Q00-postgres-lab-boundary")
def test_lab_boundary(tmp_path, monkeypatch):
    sql = fixture_sql("a"*64)
    assert "CREATE SCHEMA qa_cp05" in sql
    for verb in ("DELETE", "DROP", "TRUNCATE", "UPDATE"):
        assert verb not in sql
    for marker in ("bad'; SELECT", "", "G"*64):
        with pytest.raises(ValueError):
            fixture_sql(marker)
    assert validate_tools(tmp_path) == {}
    assert "--no-clean" in INITDB_SAFETY_OPTIONS
    assert sql.count("VALUES ('seed')") == 3
    assert "NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT NOBYPASSRLS" in sql
    assert "GRANT SELECT ON qa_cp05.marker TO qa_reader" in sql
    assert len(CATALOG_QUERIES) == 3
    require_outcome(0)
    require_outcome(1, expect_failure=True)
    for code, expected in ((0, True), (1, False)):
        with pytest.raises(ValueError):
            require_outcome(code, expected)
    archive = tmp_path / "test.dump"
    archive.write_bytes(b"synthetic-only")
    require_digest(archive, sha256(b"synthetic-only").hexdigest())
    for path, digest in ((archive, "0"*64), (tmp_path / "missing.dump", "0"*64)):
        with pytest.raises(ValueError):
            require_digest(path, digest)
    original = subprocess.run
    def require_file_handles(*args, **kwargs):
        assert "capture_output" not in kwargs
        assert kwargs["stdout"].fileno() >= 0 and kwargs["stderr"].fileno() >= 0
        return original(*args, **kwargs)
    monkeypatch.setattr(subprocess, "run", require_file_handles)
    code, output, errors = run_to_files([sys.executable, "-c", "print('synthetic-ok')"], tmp_path, 1, dict(os.environ), 5)
    assert code == 0 and output.strip() == b"synthetic-ok" and not errors
    with pytest.raises(FileExistsError):
        run_to_files([sys.executable, "--version"], tmp_path, 1, dict(os.environ), 5)
