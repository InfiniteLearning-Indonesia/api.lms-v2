import struct
from pathlib import Path
import pytest
from qa.release import static_amd64


@pytest.mark.case("Q00-release-contract")
def test_release_contract():
    data = bytearray(120)
    data[:6] = b"\x7fELF\x02\x01"
    struct.pack_into("<H", data, 18, 62)
    struct.pack_into("<Q", data, 32, 64)
    struct.pack_into("<HH", data, 54, 56, 1)
    struct.pack_into("<I", data, 64, 1)
    assert static_amd64(bytes(data))
    for kind in (2, 3):
        struct.pack_into("<I", data, 64, kind)
        assert not static_amd64(bytes(data))
    assert not static_amd64(b"not ELF")
    docker = (Path(__file__).resolve().parents[3] / "deploy/Dockerfile").read_text()
    instructions = [line for line in docker.splitlines() if line and not line.startswith("#")]
    assert instructions == ["FROM scratch", "COPY --chown=65532:65532 --chmod=0555 api /api", "USER 65532:65532", 'ENTRYPOINT ["/api"]']
