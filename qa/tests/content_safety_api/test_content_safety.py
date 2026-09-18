from datetime import datetime, timedelta, timezone
import os
import uuid

import pytest


def mutate(lab, pair, path, body, method="post", key=None):
    actor, csrf = pair
    return lab["mutate"](actor, csrf, path, body, method=method, key=key or uuid.uuid4().hex)


def classroom(lab, activity_type="ASSIGNMENT"):
    admin, admin_csrf = lab["login"]("admin")
    teacher, teacher_csrf = lab["login"]("teacher")
    student, student_csrf = lab["login"]("student")
    outsider, outsider_csrf = lab["login"]("outsider")
    created = lab["mutate"](admin, admin_csrf, "/v3/classes", {"name": "M07 CP02 safe content"})
    assert created.status == 200
    cls = created.json()
    for subject, role in (("teacher", "teacher"), ("student", "student")):
        assert lab["mutate"](admin, admin_csrf, f"/v3/classes/{cls['id']}/participants", {
            "user_id": lab["actors"][subject]["id"], "role": role,
        }).status == 200
    assert lab["mutate"](admin, admin_csrf, f"/v3/classes/{cls['id']}/publish", {"version": 1}).status == 200
    teacher_pair = (teacher, teacher_csrf)
    section = mutate(lab, teacher_pair, f"/v3/classes/{cls['id']}/sections", {"title": "Week 1", "position": 1}).json()
    activity = mutate(lab, teacher_pair, f"/v3/classes/{cls['id']}/activities", {
        "section_id": section["id"], "activity_type": activity_type, "title": "Activity", "position": 1,
    }).json()
    return cls, activity, teacher_pair, (student, student_csrf), (outsider, outsider_csrf)


def revision_body(raw="Hello", available=None, due=None, cutoff=None):
    body = {"raw_body": raw, "timezone": "Asia/Jakarta"}
    if os.environ.get("QA_CONTENT_CONTRACTS") == "required":
        body["assignment"] = {"submission_mode": "TEXT", "max_attempts": 1, "max_files": 0,
                              "max_file_bytes": 0, "late_policy": "REJECT_AFTER_CUTOFF"}
    if available is not None:
        body["available_from"] = available.isoformat().replace("+00:00", "Z")
    if due is not None:
        body["due_at"] = due.isoformat().replace("+00:00", "Z")
    if cutoff is not None:
        body["cutoff_at"] = cutoff.isoformat().replace("+00:00", "Z")
    return body


@pytest.mark.case("M07-CP02-canonical-source-boundary")
def test_canonical_body_never_returns_raw_active_markup(lab):
    cls, activity, teacher, _, _ = classroom(lab)
    path = f"/v3/classes/{cls['id']}/activities/{activity['id']}/revisions"
    response = mutate(lab, teacher, path, revision_body("Read <script>alert(1)</script>\r\nnext"))
    assert response.status == 201
    value = response.json()
    assert value["content_format"] == "SAFE_HTML"
    assert "<script>" not in value["canonical_body"]
    assert "&lt;script&gt;" in value["canonical_body"] and "<br>" in value["canonical_body"]
    assert "raw_body" not in value


@pytest.mark.case("M07-CP02-lifecycle-authority")
def test_only_class_author_can_publish_and_withdraw(lab):
    cls, activity, teacher, student, outsider = classroom(lab)
    base = f"/v3/classes/{cls['id']}/activities/{activity['id']}/revisions"
    revision = mutate(lab, teacher, base, revision_body()).json()
    publish = base + f"/{revision['id']}/publish"
    assert mutate(lab, student, publish, {"version": revision["version"]}).status == 403
    published = mutate(lab, teacher, publish, {"version": revision["version"]})
    assert published.status == 200 and published.json()["state"] == "PUBLISHED"
    assert mutate(lab, outsider, base + f"/{revision['id']}/withdraw", {"version": published.json()["version"]}).status == 403
    withdrawn = mutate(lab, teacher, base + f"/{revision['id']}/withdraw", {"version": published.json()["version"]})
    assert withdrawn.status == 200 and withdrawn.json()["state"] == "WITHDRAWN"


@pytest.mark.case("M07-CP02-availability-boundary")
def test_draft_future_and_withdrawn_are_hidden_from_student(lab):
    cls, activity, teacher, student, _ = classroom(lab)
    base = f"/v3/classes/{cls['id']}/activities/{activity['id']}/revisions"
    now = datetime.now(timezone.utc)
    future = mutate(lab, teacher, base, revision_body("Future", now + timedelta(hours=1))).json()
    future = mutate(lab, teacher, base + f"/{future['id']}/publish", {"version": future["version"]}).json()
    snapshot = student[0].get(f"/v3/classes/{cls['id']}/content")
    assert snapshot.status == 200 and snapshot.json()["revisions"] == []
    visible = mutate(lab, teacher, base, revision_body("Visible", now - timedelta(hours=1))).json()
    visible = mutate(lab, teacher, base + f"/{visible['id']}/publish", {"version": visible["version"]}).json()
    snapshot = student[0].get(f"/v3/classes/{cls['id']}/content")
    assert [item["id"] for item in snapshot.json()["revisions"]] == [visible["id"]]
    assert mutate(lab, teacher, base + f"/{visible['id']}/withdraw", {"version": visible["version"]}).status == 200
    assert student[0].get(f"/v3/classes/{cls['id']}/content").json()["revisions"] == []


@pytest.mark.case("M07-CP02-schedule-validation")
def test_timezone_deadline_and_activity_rules_fail_closed(lab):
    cls, activity, teacher, _, _ = classroom(lab)
    base = f"/v3/classes/{cls['id']}/activities/{activity['id']}/revisions"
    now = datetime.now(timezone.utc)
    invalid_zone = revision_body()
    invalid_zone["timezone"] = "Not/A_Zone"
    assert mutate(lab, teacher, base, invalid_zone).status == 400
    assert mutate(lab, teacher, base, revision_body("Reverse", now, now + timedelta(hours=2), now + timedelta(hours=1))).status == 400
    resource_cls, resource, resource_teacher, _, _ = classroom(lab, "RESOURCE")
    resource_base = f"/v3/classes/{resource_cls['id']}/activities/{resource['id']}/revisions"
    assert mutate(lab, resource_teacher, resource_base, revision_body("No due", now, now + timedelta(hours=1))).status == 400
