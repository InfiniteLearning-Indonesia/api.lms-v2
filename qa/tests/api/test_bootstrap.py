import json
import os
import re
import subprocess
import pytest


@pytest.mark.case("M03-T01-health-contract")
def test_health_is_not_business_readiness(api):
    live = api.get("/health/live")
    assert live.status == 200
    assert live.json() == {"status": "alive"}
    assert live.headers["cache-control"] == "no-store"
    assert live.headers["x-content-type-options"] == "nosniff"
    assert re.fullmatch(r"[0-9a-f]{32}", live.headers["x-request-id"])
    assert api.head("/health/live").body() == b""
    ready = api.get("/health/ready")
    assert ready.status == 503
    assert ready.json() == {"status": "not_ready"}


@pytest.mark.case("M03-T01-request-boundary")
def test_untrusted_request_id_and_credentials_are_not_reflected_or_logged(api, local_service):
    headers = {"Authorization": "Bearer DO_NOT_EMIT", "Cookie": "sid=DO_NOT_EMIT",
               "X-Request-ID": "DO_NOT_EMIT"}
    response = api.get("/DO_NOT_EMIT?secret=DO_NOT_EMIT", headers=headers)
    assert response.status == 404
    assert "DO_NOT_EMIT" not in response.text()
    assert "DO_NOT_EMIT" not in json.dumps(response.headers)
    assert "DO_NOT_EMIT" not in local_service[1].read_text(encoding="utf-8")
    assert api.post("/health/live", data="DO_NOT_EMIT").status == 405
    assert api.get("/health/live", headers=headers).headers["x-request-id"] != response.headers["x-request-id"]


@pytest.mark.case("M03-T01-no-business-authority")
def test_bootstrap_exposes_no_business_or_admin_mutations(api):
    for route in ("/classes", "/users", "/auth/setup-password", "/admin", "/migrations"):
        response = api.post(route, data={"role": "admin"}, headers={"Cookie": "sid=fake"})
        assert response.status == 404
    assert api.get("/health/ready").status == 503


@pytest.mark.case("M03-T01-config-refusal")
def test_binary_refuses_nonlocal_modes_and_bindings():
    for mode, address in (("production", "127.0.0.1:0"), ("staging", "127.0.0.1:0"),
                          ("local-synthetic", "0.0.0.0:0"), ("", "127.0.0.1:0")):
        env = {k: v for k, v in os.environ.items() if k.upper() in {"SYSTEMROOT", "WINDIR", "PATH", "TEMP", "TMP"}}
        env.update(LMS_MODE=mode, LMS_HTTP_ADDR=address)
        result = subprocess.run([os.environ["QA_API_BINARY"]], env=env, capture_output=True,
                                text=True, timeout=5)
        assert result.returncode == 2
        assert "configuration_rejected" in result.stdout
        assert "local_server_started" not in result.stdout
