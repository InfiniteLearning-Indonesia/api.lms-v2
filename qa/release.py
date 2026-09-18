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
        binaries = {}
        for target in ("api", "worker"):
            copies = []
            for name in (target, target + "-repeat"):
                command = run_command(["go", "build", "-trimpath", "-buildvcs=false", "-o", str(directory / name), "./cmd/" + target], env, directory, name)
                if command["return_code"] != 0:
                    write_new_json(directory / "summary.json", {"run_id": run_id, "status": "FAIL_BUILD", "source_hash": source})
                    return 1
                copies.append((directory / name).read_bytes())
            binaries[target] = copies
        dockerfiles = {"Dockerfile": (ROOT / "deploy/Dockerfile").read_bytes(),
                       "Dockerfile.worker": (ROOT / "deploy/Dockerfile.worker").read_bytes()}
        passed = (all(copies[0] == copies[1] and static_amd64(copies[0]) for copies in binaries.values())
                  and source == source_fingerprint(ROOT))
        for name, data in dockerfiles.items():
            with (directory / name).open("xb") as handle:
                handle.write(data)
        # Explicit allowlist; deterministic UID/mode/mtime, no env/CA/source tree.
        with tarfile.open(directory / "backend-bootstrap-linux-amd64.tar", "x", format=tarfile.USTAR_FORMAT) as archive:
            contents = (("api", binaries["api"][0], 0o555),
                        ("worker", binaries["worker"][0], 0o555),
                        ("Dockerfile", dockerfiles["Dockerfile"], 0o444),
                        ("Dockerfile.worker", dockerfiles["Dockerfile.worker"], 0o444))
            for name, data, mode in contents:
                info = tarfile.TarInfo(name)
                info.size, info.mode, info.uid, info.gid, info.mtime = len(data), mode, 65532, 65532, 0
                archive.addfile(info, io.BytesIO(data))
        result = dict(run_id=run_id, status="PASS_BUILD_ONLY" if passed else "FAIL", source_hash=source,
                      binary_sha256={name: sha256(copies[0]).hexdigest() for name, copies in binaries.items()},
                      deterministic_repeat=all(copies[0] == copies[1] for copies in binaries.values()),
                      static_linux_amd64=all(static_amd64(copies[0]) for copies in binaries.values()), container_execution="NOT_RUN",
                      non_root_runtime="NOT_VERIFIED", deployment="NOT_EXECUTED")
        write_new_json(directory / "summary.json", result)
        print(result)
        return 0 if passed else 1


if __name__ == "__main__":
    raise SystemExit(main())
