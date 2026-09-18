import uuid

import pytest


def room(lab, name, student=True):
    admin, admin_csrf = lab["login"]("admin")
    teacher, teacher_csrf = lab["login"]("teacher")
    learner, learner_csrf = lab["login"]("student")
    created = lab["mutate"](admin, admin_csrf, "/v3/classes", {"name": name})
    assert created.status == 200
    class_id = created.json()["id"]
    assert lab["mutate"](admin, admin_csrf, f"/v3/classes/{class_id}/participants", {
        "user_id": lab["actors"]["teacher"]["id"], "role": "teacher",
    }).status == 200
    if student:
        assert lab["mutate"](admin, admin_csrf, f"/v3/classes/{class_id}/participants", {
            "user_id": lab["actors"]["student"]["id"], "role": "student",
        }).status == 200
    assert lab["mutate"](admin, admin_csrf, f"/v3/classes/{class_id}/publish", {"version": 1}).status == 200
    return class_id, (admin, admin_csrf), (teacher, teacher_csrf), (learner, learner_csrf)


def payload(count=2):
    periods = []
    for month in range(1, count + 1):
        periods.append({
            "name": f"Period {month}",
            "starts_at": f"2027-{month:02d}-01T00:00:00Z",
            "ends_at": f"2027-{month+1:02d}-01T00:00:00Z",
            "due_at": f"2027-{month+1:02d}-08T00:00:00Z",
        })
    return {"title": "Mentoring plan", "timezone": "Asia/Jakarta", "periods": periods}


def create(lab, actor, class_id, body=None, key=None):
    return lab["mutate"](actor[0], actor[1], f"/v3/classes/{class_id}/period-sets", body or payload(), key=key or uuid.uuid4().hex)


def publish(lab, actor, class_id, set_id, expected=""):
    return lab["mutate"](actor[0], actor[1], f"/v3/classes/{class_id}/period-sets/{set_id}/publish", {"expected_active_id": expected})


@pytest.mark.case("M12-CP01-period-lifecycle")
def test_period_snapshot_publication_and_history(lab):
    class_id, _, teacher, student = room(lab, "M12 period lifecycle")
    path = f"/v3/classes/{class_id}/period-sets"
    key = uuid.uuid4().hex
    first = create(lab, teacher, class_id, key=key)
    replay = create(lab, teacher, class_id, key=key)
    assert first.status == 201 and replay.status == 200
    assert replay.headers.get("idempotent-replay") == "true" and replay.json() == first.json()
    set_a = first.json()["set"]
    assert set_a["state"] == "DRAFT" and len(set_a["periods"]) == 2
    assert student[0].get(path).status == 404
    assert teacher[0].get(f"{path}/{set_a['id']}").status == 200
    assert publish(lab, teacher, class_id, set_a["id"]).status == 200
    assert student[0].get(path).json()["id"] == set_a["id"]
    second = create(lab, teacher, class_id, payload(6))
    assert second.status == 201
    set_b = second.json()["set"]
    assert student[0].get(path).json()["id"] == set_a["id"]
    assert publish(lab, teacher, class_id, set_b["id"], expected=set_a["id"]).status == 200
    assert student[0].get(path).json()["id"] == set_b["id"]
    assert len(student[0].get(path).json()["periods"]) == 6
    assert teacher[0].get(f"{path}/{set_a['id']}").json()["revision"] == 1
    invalid = payload()
    invalid["periods"][1]["starts_at"] = "2027-01-20T00:00:00Z"
    assert create(lab, teacher, class_id, invalid).status == 400


@pytest.mark.case("M12-CP01-context-denied")
def test_period_context_and_cross_class_boundary(lab):
    class_a, _, teacher, student = room(lab, "M12 Class A")
    class_b, _, teacher_b, _ = room(lab, "M12 Class B", student=False)
    made = create(lab, teacher, class_a)
    assert made.status == 201
    set_id = made.json()["set"]["id"]
    assert create(lab, student, class_a).status == 403
    assert student[0].get(f"/v3/classes/{class_a}/period-sets/{set_id}").status == 403
    assert student[0].get(f"/v3/classes/{class_b}/period-sets").status == 403
    outsider, _ = lab["login"]("outsider")
    assert outsider.get(f"/v3/classes/{class_a}/period-sets").status == 403
    assert publish(lab, teacher_b, class_b, set_id).status == 404
    assert publish(lab, teacher, class_a, set_id, expected=uuid.uuid4().hex).status == 409
