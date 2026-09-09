import pytest


@pytest.mark.case("M03-CP04-health")
def test_runtime_health_and_readiness(lab):
    client=lab["client"]()
    live=client.get("/health/live"); ready=client.get("/health/ready")
    assert live.status==200 and live.json()=={"status":"alive"}
    assert ready.status==200 and ready.json()=={"status":"ready"}
    assert client.post("/health/ready",data={}).status==405


@pytest.mark.case("M03-CP04-mounted")
def test_runtime_mounts_cookie_identity_and_contextual_class(lab):
    admin,csrf=lab["login"]("admin")
    created=lab["mutate"](admin,csrf,"/v3/classes",{"name":"Real API runtime"})
    assert created.status==200
    assert admin.get("/v3/classes/"+created.json()["id"]).status==200


@pytest.mark.case("M03-CP04-boundary")
def test_runtime_rejects_bearer_and_untrusted_origin(lab):
    client=lab["client"]()
    assert client.get("/v3/auth/me",headers={"Authorization":"Bearer "+"a"*64}).status==401
    assert client.post("/v3/auth/challenge",data={},headers={"Origin":"https://evil.invalid"}).status==403
