"""Create private, verified context archives without Git or DB operations.

Archives are not encrypted: keep the entire destination private.
Sources are read only; no overwrite or deletion. Graphify is excluded.
"""
from datetime import datetime, timezone
from hashlib import sha256
import json
import os
from pathlib import Path
import zipfile

SERVICE = Path(__file__).resolve().parents[2]
ROOT = SERVICE.parent.parent
SKIP = {'.git', 'node_modules', '.venv', '.next', 'dist', 'build', '.cache',
        '__pycache__', '.pytest_cache', 'coverage', '.qa-artifacts',
        '.qa-tmp-manifest-20261030a', 'graphify-out', '.agents', '.codex'}
EVIDENCE_SKIP = {'postgres', 'go-cache', 'go-temp', '__pycache__', '.pytest_cache'}


def link(path):
    return path.is_symlink() or getattr(path, 'is_junction', lambda: False)()


def walk(directory, skipped, errors):
    def onerror(exc):
        errors.append({'path': str(Path(exc.filename).relative_to(ROOT)), 'error': type(exc).__name__})
    for folder, dirs, names in os.walk(directory, onerror=onerror, followlinks=False):
        dirs[:] = sorted(name for name in dirs if name not in skipped and not link(Path(folder, name)))
        for name in sorted(names):
            path = Path(folder, name)
            if not link(path):
                yield path


def config(path):
    return path.name.lower().startswith('.env') or path.suffix.lower() in {'.key', '.pem', '.crt', '.pfx', '.p12'}


def archive(destination, name, paths):
    records = []
    target = destination / (name + '.zip')
    with zipfile.ZipFile(target, 'x', compression=zipfile.ZIP_DEFLATED, compresslevel=6) as bundle:
        seen = set()
        for path in paths:
            relative = path.relative_to(ROOT).as_posix()
            if relative in seen:
                continue
            seen.add(relative)
            before = path.stat()
            digest = sha256()
            with path.open('rb') as source, bundle.open(relative, 'w', force_zip64=True) as out:
                for block in iter(lambda: source.read(1024 * 1024), b''):
                    digest.update(block); out.write(block)
            after = path.stat()
            if (before.st_size, before.st_mtime_ns) != (after.st_size, after.st_mtime_ns):
                raise RuntimeError('Source changed during archive: ' + relative)
            records.append({'path': relative, 'bytes': before.st_size, 'sha256': digest.hexdigest()})
            if len(records) % 5000 == 0:
                print(json.dumps({'archive': name, 'files_copied': len(records)}), flush=True)
    # Reopen and read every archived member to prove byte integrity.
    with zipfile.ZipFile(target) as bundle:
        if len(bundle.infolist()) != len(records):
            raise RuntimeError('Archive entry count mismatch')
        for record in records:
            digest = sha256()
            with bundle.open(record['path']) as source:
                for block in iter(lambda: source.read(1024 * 1024), b''):
                    digest.update(block)
            if digest.hexdigest() != record['sha256']:
                raise RuntimeError('Archive digest mismatch')
    with (destination / (name + '-files.json')).open('x', encoding='utf-8') as handle:
        json.dump(records, handle, indent=2)
        handle.write('\n')
    digest = sha256()
    with target.open('rb') as source:
        for block in iter(lambda: source.read(1024 * 1024), b''):
            digest.update(block)
    result = {'archive': target.name, 'files': len(records), 'source_bytes': sum(r['bytes'] for r in records),
              'archive_bytes': target.stat().st_size, 'sha256': digest.hexdigest(), 'reopened_byte_verification': 'PASS'}
    print(json.dumps(result), flush=True)
    return result

