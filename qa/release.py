"""Build a static Linux artifact twice; no container daemon, push or deployment."""
from datetime import datetime, timezone
from hashlib import sha256
import io
import os
import struct
import tarfile
import uuid
from qa.evidence import source_fingerprint, write_new_json
from qa.run import ROOT, clean_environment, run_command, run_lock


def static_amd64(data: bytes) -> bool:
    if len(data) < 64 or data[:6] != b"\x7fELF\x02\x01" or struct.unpack_from("<H", data, 18)[0] != 62:
        return False
    offset = struct.unpack_from("<Q", data, 32)[0]
    size, count = struct.unpack_from("<HH", data, 54)
    if size < 56 or count == 0 or offset + size * count > len(data):
        return False
    return all(struct.unpack_from("<I", data, offset + i * size)[0] not in (2, 3) for i in range(count))


def main():
    parent = ROOT / ".qa-artifacts"
    parent.mkdir(exist_ok=True)
    with run_lock(parent / "release.lock"):
        run_id = datetime.now(timezone.utc).strftime("%Y%m%dT%H%M%SZ") + "-release-" + uuid.uuid4().hex[:12]
        directory = parent / run_id
        directory.mkdir()
        source = source_fingerprint(ROOT)
        env = clean_environment(ROOT, os.environ)
        env.update(GOOS="linux", GOARCH="amd64", CGO_ENABLED="0")
        binaries = []
        for name in ("api", "api-repeat"):
            command = run_command(["go", "build", "-trimpath", "-buildvcs=false", "-o", str(directory / name), "./cmd/api"], env, directory, name)
            if command["return_code"] != 0:
                write_new_json(directory / "summary.json", {"run_id": run_id, "status": "FAIL_BUILD", "source_hash": source})
                return 1
            binaries.append((directory / name).read_bytes())
        dockerfile = (ROOT / "deploy/Dockerfile").read_bytes()
        passed = binaries[0] == binaries[1] and static_amd64(binaries[0]) and source == source_fingerprint(ROOT)
        with (directory / "Dockerfile").open("xb") as handle:
            handle.write(dockerfile)
        # Explicit allowlist; deterministic UID/mode/mtime, no env/CA/source tree.
        with tarfile.open(directory / "backend-bootstrap-linux-amd64.tar", "x", format=tarfile.USTAR_FORMAT) as archive:
            for name, data, mode in (("api", binaries[0], 0o555), ("Dockerfile", dockerfile, 0o444)):
                info = tarfile.TarInfo(name)
                info.size, info.mode, info.uid, info.gid, info.mtime = len(data), mode, 65532, 65532, 0
                archive.addfile(info, io.BytesIO(data))
        result = dict(run_id=run_id, status="PASS_BUILD_ONLY" if passed else "FAIL", source_hash=source,
                      binary_sha256=sha256(binaries[0]).hexdigest(), deterministic_repeat=binaries[0] == binaries[1],
                      static_linux_amd64=static_amd64(binaries[0]), container_execution="NOT_RUN",
                      non_root_runtime="NOT_VERIFIED", deployment="NOT_EXECUTED")
        write_new_json(directory / "summary.json", result)
        print(result)
        return 0 if passed else 1


if __name__ == "__main__":
    raise SystemExit(main())
