import uuid
import pytest


def mutate(client, path, body, key=None, method="post"):
    return getattr(client, method)(path, data=body, headers={"Idempotency-Key": key or uuid.uuid4().hex})


def create(c, name="Standalone Class"):
    r = mutate(c["admin"], "/v3/classes", {"name": name})
    assert r.status == 200
    return r.json()


def add(c, actors, cls, label, role):
    r = mutate(c["admin"], f"/v3/classes/{cls['id']}/participants", {"user_id": actors[label]["id"], "role": role})
    assert r.status == 200
    return r.json()


@pytest.mark.case("CP10-standalone")
def test_standalone(clients):
    c, actors, _ = clients
    cls = create(c)
    assert cls["state"] == "DRAFT" and cls["cohort_label"] == "" and cls["program_label"] == ""
    assert create(c)["id"] != cls["id"]
    path = f"/v3/classes/{cls['id']}"
    assert mutate(c["admin"], path+"/publish", {"version": 1}).status == 409
    for label, role in (("teacher", "teacher"), ("teacher2", "teacher"), ("student", "student"), ("student2", "student")):
        add(c, actors, cls, label, role)
    assert c["student"].get(path).status == 403
    assert mutate(c["teacher"], path+"/publish", {"version": 1}).status == 200
    edited = mutate(c["teacher2"], path, {"name": "Renamed", "cohort_label": "manual label", "version": 2}, method="patch")
    assert edited.status == 200 and edited.json()["version"] == 3
    renamed = mutate(c["teacher2"], path, {"name": "Rename only", "version": 3}, method="patch")
    assert renamed.status == 200 and renamed.json()["cohort_label"] == "manual label"
    for label in ("student", "student2"):
        assert c[label].get(path).status == 200


@pytest.mark.case("CP10-authority")
def test_class_authority(clients):
    c, actors, _ = clients
    a, b = create(c, "A"), create(c, "B")
    add(c, actors, a, "teacher", "teacher")
    add(c, actors, b, "teacher2", "teacher")
    for label, cls in (("teacher", b), ("teacher2", a), ("outsider", a)):
        path = f"/v3/classes/{cls['id']}"
        assert mutate(c[label], path, {"name": "intrusion", "version": 1}, method="patch").status == 403
    assert mutate(c["teacher"], "/v3/classes", {"name": "unauthorized"}).status == 403
    assert mutate(c["teacher"], f"/v3/classes/{a['id']}/participants", {"user_id": actors["outsider"]["id"], "role": "teacher"}).status == 403
    assert c["outsider"].get(f"/v3/classes/{a['id']}", headers={"X-Role": "admin", "X-User-ID": actors["admin"]["id"]}).status == 403


@pytest.mark.case("CP10-validation")
def test_validation(clients):
    c, actors, _ = clients
    for body in ({"name": "bad", "site_admin": True}, {"name": "bad", "state": "PUBLISHED"}, {"name": ""}, {"name": "x"*17000}):
        assert mutate(c["admin"], "/v3/classes", body).status == 400
    cls = create(c)
    assert mutate(c["admin"], f"/v3/classes/{cls['id']}/participants", {"user_id": actors["student"]["id"], "role": "admin"}).status == 400
    assert c["admin"].post("/v3/classes", data='{"name":"one"} {"name":"two"}', headers={"Content-Type": "application/json", "Idempotency-Key": uuid.uuid4().hex}).status == 400
    assert c["admin"].post("/v3/classes", data="name=x", headers={"Content-Type": "application/x-www-form-urlencoded"}).status == 400
    other = create(c)
    add(c, actors, other, "student2", "student")
    assert mutate(c["admin"], f"/v3/classes/{cls['id']}/participants/{actors['student2']['id']}/end", {}).status == 404


@pytest.mark.case("CP10-session")
def test_session_boundary(clients):
    c, actors, anonymous = clients
    cls = create(c)
    path = f"/v3/classes/{cls['id']}"
    for label in ("disabled", "expired", "revoked"):
        assert c[label].get(path).status == 401
    assert anonymous.get(path, headers={"X-Role": "admin"}).status == 401
    assert anonymous.get(path, headers={"Authorization": "Bearer " + "0"*64}).status == 401
    assert anonymous.get(path, headers={"Cookie": "session="+actors["admin"]["token"]}).status == 401
    assert c["admin"].post("/v3/classes", data={"name": "CSRF"}, headers={"Origin": "https://foreign.invalid", "Idempotency-Key": uuid.uuid4().hex}).status == 403


@pytest.mark.case("CP10-idempotency")
def test_idempotency(clients):
    c, _, _ = clients
    key = uuid.uuid4().hex
    first = mutate(c["admin"], "/v3/classes", {"name": "retry"}, key)
    second = mutate(c["admin"], "/v3/classes", {"name": "retry"}, key)
    assert first.status == second.status == 200 and first.json() == second.json()
    assert mutate(c["admin"], "/v3/classes", {"name": "different"}, key).status == 409
    cls = first.json()
    path = f"/v3/classes/{cls['id']}"
    assert mutate(c["admin"], path, {"name": "updated", "version": 1}, method="patch").status == 200
    assert mutate(c["admin"], path, {"name": "stale", "version": 1}, method="patch").status == 409
    assert c["admin"].post("/v3/classes", data={"name": "missing key"}).status == 400


@pytest.mark.case("CP10-lifecycle")
def test_lifecycle(clients):
    c, actors, _ = clients
    cls = create(c)
    add(c, actors, cls, "teacher", "teacher")
    participation = add(c, actors, cls, "student", "student")
    path = f"/v3/classes/{cls['id']}"
    assert mutate(c["admin"], path+"/publish", {"version": 1}).status == 200
    assert mutate(c["admin"], path+f"/participants/{actors['teacher']['id']}/end", {}).status == 409
    assert mutate(c["admin"], path+f"/participants/{actors['student']['id']}/end", {}).status == 200
    assert c["student"].get(path).status == 403
    rejoined = add(c, actors, cls, "student", "student")
    assert rejoined["enrollment_id"] == participation["enrollment_id"]
    assert c["student"].get(path).status == 200
    assert mutate(c["teacher"], path+"/archive", {"version": 2}).status == 403
    assert mutate(c["admin"], path+"/archive", {"version": 2}).status == 200
    assert c["student"].get(path).json()["state"] == "ARCHIVED"
    assert mutate(c["teacher"], path, {"name": "cannot change", "version": 3}, method="patch").status == 409
