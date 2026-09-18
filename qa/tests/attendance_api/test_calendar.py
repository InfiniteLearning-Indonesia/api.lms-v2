import uuid

import pytest


def room(lab, name, student=True):
    admin, admin_csrf = lab["login"]("admin")
    teacher, teacher_csrf = lab["login"]("teacher")
    learner, learner_csrf = lab["login"]("student")
    created = lab["mutate"](admin, admin_csrf, "/v3/classes", {"name": name})
    assert created.status == 200
    class_id = created.json()["id"]
    assigned = lab["mutate"](admin, admin_csrf, f"/v3/classes/{class_id}/participants", {
        "user_id": lab["actors"]["teacher"]["id"], "role": "teacher",
    })
    assert assigned.status == 200
    if student:
        assigned = lab["mutate"](admin, admin_csrf, f"/v3/classes/{class_id}/participants", {
            "user_id": lab["actors"]["student"]["id"], "role": "student",
        })
        assert assigned.status == 200
    assert lab["mutate"](admin, admin_csrf, f"/v3/classes/{class_id}/publish", {"version": 1}).status == 200
    return class_id, (admin, admin_csrf), (teacher, teacher_csrf), (learner, learner_csrf)


def create(lab, teacher, class_id, key=None, start="2026-10-02T03:00:00Z", end="2026-10-02T04:30:00Z", date="2026-10-02"):
    return lab["mutate"](teacher[0], teacher[1], f"/v3/classes/{class_id}/meetings", {
        "title": "Class workshop", "local_date": date, "timezone": "Asia/Jakarta",
        "starts_at": start, "ends_at": end,
    }, key=key or uuid.uuid4().hex)


@pytest.mark.case("M11-CP01-meeting-calendar")
def test_class_meeting_denominator_and_exception_history(lab):
    class_id, admin, teacher, student = room(lab, "M11 calendar")
    key = uuid.uuid4().hex
    first = create(lab, teacher, class_id, key=key)
    replay = create(lab, teacher, class_id, key=key)
    assert first.status == 201 and replay.status == 200
    assert replay.headers.get("idempotent-replay") == "true" and replay.json() == first.json()
    meeting = first.json()["meeting"]
    assert meeting["version"] == 1 and meeting["state"] == "SCHEDULED"
    path = f"/v3/classes/{class_id}/meetings"
    snapshot = student[0].get(path)
    assert snapshot.status == 200 and snapshot.json()["planned_meetings"] == 1 and snapshot.json()["active_meetings"] == 1
    cancelled = lab["mutate"](teacher[0], teacher[1], f"{path}/{meeting['id']}/cancel", {"version": 1, "reason": "Public holiday"})
    assert cancelled.status == 200 and cancelled.json()["meeting"]["version"] == 2
    assert student[0].get(path).json()["active_meetings"] == 0
    stale = lab["mutate"](teacher[0], teacher[1], f"{path}/{meeting['id']}/reinstate", {"version": 1, "reason": "Stale"})
    assert stale.status == 409
    active = lab["mutate"](teacher[0], teacher[1], f"{path}/{meeting['id']}/reinstate", {"version": 2, "reason": "Holiday moved"})
    assert active.status == 200 and active.json()["meeting"]["version"] == 3
    assert student[0].get(path).json()["active_meetings"] == 1
    assert create(lab, teacher, class_id, date="2026-10-01").status == 400
    assert create(lab, teacher, class_id, start="2026-10-02T03:30:00Z", end="2026-10-02T04:00:00Z").status == 409


@pytest.mark.case("M11-CP01-cross-class-denied")
def test_meeting_context_and_cross_class_exception(lab):
    class_a, _, teacher, student = room(lab, "M11 first Class")
    class_b, _, teacher_b, _ = room(lab, "M11 second Class", student=False)
    made = create(lab, teacher, class_a)
    assert made.status == 201
    meeting_id = made.json()["meeting"]["id"]
    assert create(lab, student, class_a, start="2026-10-03T03:00:00Z", end="2026-10-03T04:30:00Z", date="2026-10-03").status == 403
    assert student[0].get(f"/v3/classes/{class_b}/meetings").status == 403
    outsider, _ = lab["login"]("outsider")
    assert outsider.get(f"/v3/classes/{class_a}/meetings").status == 403
    forged = lab["mutate"](teacher_b[0], teacher_b[1], f"/v3/classes/{class_b}/meetings/{meeting_id}/cancel", {"version": 1, "reason": "Cross class"})
    assert forged.status == 404
    assert teacher[0].get(f"/v3/classes/{class_a}/meetings").status == 200
