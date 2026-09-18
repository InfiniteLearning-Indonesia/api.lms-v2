from datetime import datetime, timedelta, timezone
import uuid

import pytest


def room(lab, name, teacher2=False, student=True):
    admin, admin_csrf = lab["login"]("admin")
    teacher, teacher_csrf = lab["login"]("teacher")
    learner, learner_csrf = lab["login"]("student")
    created = lab["mutate"](admin, admin_csrf, "/v3/classes", {"name": name})
    assert created.status == 200
    class_id = created.json()["id"]
    for subject, role in [("teacher", "teacher")] + ([("teacher2", "teacher")] if teacher2 else []) + ([("student", "student")] if student else []):
        assert lab["mutate"](admin, admin_csrf, f"/v3/classes/{class_id}/participants", {
            "user_id": lab["actors"][subject]["id"], "role": role,
        }).status == 200
    assert lab["mutate"](admin, admin_csrf, f"/v3/classes/{class_id}/publish", {"version": 1}).status == 200
    return class_id, (admin, admin_csrf), (teacher, teacher_csrf), (learner, learner_csrf)


def iso(value):
    return value.isoformat().replace("+00:00", "Z")


def make_period(lab, teacher, class_id, expired=False):
    now = datetime.now(timezone.utc)
    if expired:
        start, end, due = now - timedelta(days=3), now - timedelta(days=2), now - timedelta(days=1)
    else:
        start, end, due = now - timedelta(hours=1), now + timedelta(hours=1), now + timedelta(days=1)
    base = f"/v3/classes/{class_id}/period-sets"
    draft = lab["mutate"](teacher[0], teacher[1], base, {
        "title": "Entry window", "timezone": "UTC", "periods": [{
            "name": "Current", "starts_at": iso(start), "ends_at": iso(end), "due_at": iso(due),
        }],
    })
    assert draft.status == 201
    set_ = draft.json()["set"]
    current = teacher[0].get(base)
    expected = current.json()["id"] if current.status == 200 else ""
    published = lab["mutate"](teacher[0], teacher[1], f"{base}/{set_['id']}/publish", {"expected_active_id": expected})
    assert published.status == 200
    return set_["periods"][0]["id"]


def save(lab, student, class_id, period_id, body="Reflection", key=None):
    return lab["mutate"](student[0], student[1], f"/v3/classes/{class_id}/logbook-entries", {
        "period_id": period_id, "body": body, "file_asset_ids": [],
    }, key=key or uuid.uuid4().hex)


def submit(lab, student, class_id, entry):
    return lab["mutate"](student[0], student[1], f"/v3/classes/{class_id}/logbook-entries/{entry['id']}/submit", {
        "revision_id": entry["revision_id"], "expected_version": entry["version"],
    })


def mentor(lab, actor, class_id, teacher_subject):
    return lab["mutate"](actor[0], actor[1], f"/v3/classes/{class_id}/mentors", {
        "student_user_id": lab["actors"]["student"]["id"],
        "teacher_user_id": lab["actors"][teacher_subject]["id"],
    })


def review(lab, actor, class_id, entry, decision="ACCEPTED", feedback="Complete"):
    return lab["mutate"](actor[0], actor[1], f"/v3/classes/{class_id}/logbook-entries/{entry['id']}/review", {
        "revision_id": entry["revision_id"], "expected_version": entry["version"],
        "decision": decision, "feedback": feedback,
    })


@pytest.mark.case("M12-CP02-entry-lifecycle")
def test_entry_revisions_submission_and_replay(lab):
    class_id, _, teacher, student = room(lab, "M12 CP02 lifecycle")
    period = make_period(lab, teacher, class_id)
    key = uuid.uuid4().hex
    first = save(lab, student, class_id, period, "First", key)
    replay = save(lab, student, class_id, period, "First", key)
    assert first.status == 201 and replay.status == 200
    assert replay.headers.get("idempotent-replay") == "true" and replay.json() == first.json()
    first_entry = first.json()["entry"]
    second = save(lab, student, class_id, period, "Second")
    assert second.status == 201
    second_entry = second.json()["entry"]
    assert second_entry["id"] == first_entry["id"] and second_entry["revision_no"] == 2 and second_entry["revision_count"] == 2
    sent = submit(lab, student, class_id, second_entry)
    assert sent.status == 200 and sent.json()["entry"]["state"] == "SUBMITTED"
    assert student[0].get(f"/v3/classes/{class_id}/logbook-entries/{second_entry['id']}").json()["body"] == "Second"
    assert save(lab, student, class_id, period, "Silent overwrite").status == 409


@pytest.mark.case("M12-CP02-entry-boundary")
def test_entry_context_and_period_boundary(lab):
    class_a, _, teacher, student = room(lab, "M12 CP02 A")
    class_b, _, teacher_b, _ = room(lab, "M12 CP02 B", student=False)
    period = make_period(lab, teacher, class_a)
    assert save(lab, teacher, class_a, period).status == 403
    made = save(lab, student, class_a, period)
    assert made.status == 201
    entry = made.json()["entry"]
    assert teacher[0].get(f"/v3/classes/{class_a}/logbook-entries/{entry['id']}").status == 403
    assert teacher_b[0].get(f"/v3/classes/{class_b}/logbook-entries/{entry['id']}").status == 404
    expired = make_period(lab, teacher, class_a, expired=True)
    assert save(lab, student, class_a, expired).status == 409
    invalid = lab["mutate"](student[0], student[1], f"/v3/classes/{class_a}/logbook-entries", {
        "period_id": period, "body": "x", "file_asset_ids": [], "owner_user_id": lab["actors"]["outsider"]["id"],
    })
    assert invalid.status == 400


@pytest.mark.case("M12-CP03-review-lifecycle")
def test_review_revision_and_mentor_reassignment(lab):
    class_id, admin, teacher, student = room(lab, "M12 CP03 lifecycle", teacher2=True)
    teacher2, teacher2_csrf = lab["login"]("teacher2")
    assert mentor(lab, admin, class_id, "teacher").status == 200
    period = make_period(lab, teacher, class_id)
    draft = save(lab, student, class_id, period).json()["entry"]
    sent = submit(lab, student, class_id, draft).json()["entry"]
    returned = review(lab, teacher, class_id, sent, "REVISION_REQUIRED", "Add detail")
    assert returned.status == 200 and returned.json()["entry"]["state"] == "REVISION_REQUIRED"
    revised = save(lab, student, class_id, period, "Expanded").json()["entry"]
    assert revised["revision_no"] == 2 and revised["state"] == "DRAFT"
    resent = submit(lab, student, class_id, revised).json()["entry"]
    assert mentor(lab, admin, class_id, "teacher2").status == 200
    assert review(lab, teacher, class_id, resent).status == 403
    accepted = review(lab, (teacher2, teacher2_csrf), class_id, resent)
    assert accepted.status == 200 and accepted.json()["entry"]["state"] == "ACCEPTED"
    assert teacher[0].get(f"/v3/classes/{class_id}/logbook-entries/{resent['id']}").status == 403
    assert teacher2.get(f"/v3/classes/{class_id}/logbook-entries/{resent['id']}").status == 200


@pytest.mark.case("M12-CP03-review-boundary")
def test_review_authority_and_stale_version(lab):
    class_id, admin, teacher, student = room(lab, "M12 CP03 boundary", teacher2=True)
    teacher2, teacher2_csrf = lab["login"]("teacher2")
    assert mentor(lab, admin, class_id, "teacher").status == 200
    period = make_period(lab, teacher, class_id)
    sent = submit(lab, student, class_id, save(lab, student, class_id, period).json()["entry"]).json()["entry"]
    assert review(lab, (teacher2, teacher2_csrf), class_id, sent).status == 403
    assert review(lab, student, class_id, sent).status == 403
    accepted = review(lab, teacher, class_id, sent)
    assert accepted.status == 200
    assert review(lab, teacher, class_id, sent, "REVISION_REQUIRED", "Stale").status == 409
    foreign, _, foreign_teacher, _ = room(lab, "M12 CP03 foreign", student=False)
    assert review(lab, foreign_teacher, foreign, sent).status == 404
