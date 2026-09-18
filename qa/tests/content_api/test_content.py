import uuid

import pytest


def classroom(lab, name="M07 content"):
    admin, admin_csrf = lab["login"]("admin")
    teacher, teacher_csrf = lab["login"]("teacher")
    student, student_csrf = lab["login"]("student")
    outsider, outsider_csrf = lab["login"]("outsider")
    created = lab["mutate"](admin, admin_csrf, "/v3/classes", {"name": name})
    assert created.status == 200
    cls = created.json()
    for subject, role in (("teacher", "teacher"), ("student", "student")):
        assigned = lab["mutate"](admin, admin_csrf, f"/v3/classes/{cls['id']}/participants", {
            "user_id": lab["actors"][subject]["id"], "role": role,
        })
        assert assigned.status == 200
    published = lab["mutate"](admin, admin_csrf, f"/v3/classes/{cls['id']}/publish", {"version": 1})
    assert published.status == 200
    return {
        "class": cls, "admin": (admin, admin_csrf), "teacher": (teacher, teacher_csrf),
        "student": (student, student_csrf), "outsider": (outsider, outsider_csrf),
    }


def mutate(lab, pair, path, body, method="post", key=None):
    actor, csrf = pair
    return lab["mutate"](actor, csrf, path, body, method=method, key=key or uuid.uuid4().hex)


@pytest.mark.case("M07-CP01-section-structure")
def test_sections_are_ordered_and_idempotent(lab):
    room = classroom(lab)
    path = f"/v3/classes/{room['class']['id']}/sections"
    key = uuid.uuid4().hex
    first = mutate(lab, room["teacher"], path, {"title": "Orientation", "position": 1}, key=key)
    replay = mutate(lab, room["teacher"], path, {"title": "Orientation", "position": 1}, key=key)
    second = mutate(lab, room["teacher"], path, {"title": "Week 1", "position": 2})
    assert first.status == 201 and replay.status == 200 and replay.json() == first.json()
    assert second.status == 201
    snapshot = room["student"][0].get(f"/v3/classes/{room['class']['id']}/content")
    assert snapshot.status == 200
    assert [item["position"] for item in snapshot.json()["sections"]] == [1, 2]


@pytest.mark.case("M07-CP01-activity-structure")
def test_activity_type_and_section_ancestry(lab):
    room = classroom(lab, "M07 activities")
    base = f"/v3/classes/{room['class']['id']}"
    section = mutate(lab, room["teacher"], base + "/sections", {"title": "Week 1", "position": 1}).json()
    activity = mutate(lab, room["teacher"], base + "/activities", {
        "section_id": section["id"], "activity_type": "ASSIGNMENT", "title": "Task", "position": 1,
    })
    invalid = mutate(lab, room["teacher"], base + "/activities", {
        "section_id": section["id"], "activity_type": "PLUGIN", "title": "Unknown", "position": 2,
    })
    assert activity.status == 201 and activity.json()["section_id"] == section["id"]
    assert invalid.status == 400


@pytest.mark.case("M07-CP01-context-authority")
def test_student_and_outsider_cannot_author(lab):
    room = classroom(lab, "M07 authority")
    path = f"/v3/classes/{room['class']['id']}/sections"
    assert mutate(lab, room["student"], path, {"title": "Escape", "position": 1}).status == 403
    assert room["outsider"][0].get(f"/v3/classes/{room['class']['id']}/content").status == 403
    assert mutate(lab, room["teacher"], path, {"title": "Owned", "position": 1}).status == 201


@pytest.mark.case("M07-CP01-version-order-conflict")
def test_stale_versions_and_duplicate_positions_conflict(lab):
    room = classroom(lab, "M07 conflicts")
    base = f"/v3/classes/{room['class']['id']}"
    first = mutate(lab, room["teacher"], base + "/sections", {"title": "First", "position": 1}).json()
    second = mutate(lab, room["teacher"], base + "/sections", {"title": "Second", "position": 2}).json()
    stale = mutate(lab, room["teacher"], base + f"/sections/{first['id']}", {
        "title": "Stale", "position": 1, "version": 99,
    }, method="patch")
    collision = mutate(lab, room["teacher"], base + f"/sections/{second['id']}", {
        "title": "Collision", "position": 1, "version": second["version"],
    }, method="patch")
    assert stale.status == 409 and collision.status == 409
