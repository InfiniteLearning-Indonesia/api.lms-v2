from datetime import datetime, timedelta, timezone
import uuid

import pytest


def mutate(lab, pair, path, body, method="post", key=None):
    actor, csrf = pair
    return lab["mutate"](actor, csrf, path, body, method=method, key=key or uuid.uuid4().hex)


def rule(mode="TEXT", attempts=1, files=0, file_bytes=0, late="REJECT_AFTER_CUTOFF"):
    return {
        "submission_mode": mode,
        "max_attempts": attempts,
        "max_files": files,
        "max_file_bytes": file_bytes,
        "late_policy": late,
    }


def revision(raw, assignment=None):
    now = datetime.now(timezone.utc)
    return {
        "raw_body": raw,
        "timezone": "Asia/Jakarta",
        "available_from": (now - timedelta(hours=1)).isoformat().replace("+00:00", "Z"),
        "due_at": (now + timedelta(hours=1)).isoformat().replace("+00:00", "Z"),
        "cutoff_at": (now + timedelta(hours=2)).isoformat().replace("+00:00", "Z"),
        **({"assignment": assignment} if assignment is not None else {}),
    }


def classroom(lab, name="M07 CP03 contract"):
    admin, admin_csrf = lab["login"]("admin")
    teacher, teacher_csrf = lab["login"]("teacher")
    student, student_csrf = lab["login"]("student")
    outsider, outsider_csrf = lab["login"]("outsider")
    created = lab["mutate"](admin, admin_csrf, "/v3/classes", {"name": name})
    assert created.status == 200
    cls = created.json()
    for subject, role_name in (("teacher", "teacher"), ("student", "student")):
        assert lab["mutate"](admin, admin_csrf, f"/v3/classes/{cls['id']}/participants", {
            "user_id": lab["actors"][subject]["id"], "role": role_name,
        }).status == 200
    assert lab["mutate"](admin, admin_csrf, f"/v3/classes/{cls['id']}/publish", {"version": 1}).status == 200
    teacher_pair = (teacher, teacher_csrf)
    section = mutate(lab, teacher_pair, f"/v3/classes/{cls['id']}/sections", {"title": "Week 1", "position": 1}).json()
    activity = mutate(lab, teacher_pair, f"/v3/classes/{cls['id']}/activities", {
        "section_id": section["id"], "activity_type": "ASSIGNMENT", "title": "Stable assignment", "position": 1,
    }).json()
    return cls, activity, teacher_pair, (student, student_csrf), (outsider, outsider_csrf)


@pytest.mark.case("M07-CP03-assignment-contract-validation")
def test_assignment_rule_is_explicit_and_bounded(lab):
    cls, activity, teacher, _, _ = classroom(lab)
    base = f"/v3/classes/{cls['id']}/activities/{activity['id']}/revisions"
    assert mutate(lab, teacher, base, revision("missing")).status == 400
    assert mutate(lab, teacher, base, revision("invalid", rule("FILES", 1, 0, 0))).status == 400
    created = mutate(lab, teacher, base, revision("valid", rule("TEXT_AND_FILES", 3, 2, 2 << 20, "ALLOW_MARKED_LATE")))
    assert created.status == 201
    assert created.json()["assignment"]["max_attempts"] == 3
    assert created.json()["assignment"]["contract_version"] == 1


@pytest.mark.case("M07-CP03-published-revision-immutable")
def test_published_revision_requires_new_identity(lab):
    cls, activity, teacher, _, _ = classroom(lab)
    base = f"/v3/classes/{cls['id']}/activities/{activity['id']}/revisions"
    first = mutate(lab, teacher, base, revision("v1", rule())).json()
    first = mutate(lab, teacher, base + f"/{first['id']}/publish", {"version": first["version"]}).json()
    changed = revision("mutated", rule("TEXT", 2))
    changed["version"] = first["version"]
    assert mutate(lab, teacher, base + f"/{first['id']}", changed, method="patch").status == 409
    second = mutate(lab, teacher, base, revision("v2", rule("FILES", 4, 2, 1 << 20))).json()
    assert second["id"] != first["id"] and second["revision_no"] == first["revision_no"] + 1


@pytest.mark.case("M07-CP03-exact-historical-reference")
def test_old_reference_does_not_follow_new_publication(lab):
    cls, activity, teacher, student, _ = classroom(lab)
    base = f"/v3/classes/{cls['id']}/activities/{activity['id']}/revisions"
    first = mutate(lab, teacher, base, revision("old contract", rule("TEXT_AND_FILES", 2, 1, 1024))).json()
    first = mutate(lab, teacher, base + f"/{first['id']}/publish", {"version": first["version"]}).json()
    second = mutate(lab, teacher, base, revision("new contract", rule("FILES", 5, 3, 2048))).json()
    second = mutate(lab, teacher, base + f"/{second['id']}/publish", {"version": second["version"]}).json()
    old = student[0].get(base + f"/{first['id']}")
    assert old.status == 200
    assert old.json()["revision"]["id"] == first["id"]
    assert old.json()["revision"]["canonical_body"] == "old contract"
    assert old.json()["rule"]["max_attempts"] == 2
    assert student[0].get(base + f"/{second['id']}").json()["rule"]["max_attempts"] == 5
    first = mutate(lab, teacher, base + f"/{first['id']}/withdraw", {"version": first["version"]}).json()
    assert student[0].get(base + f"/{first['id']}").status == 404
    retained = teacher[0].get(base + f"/{first['id']}")
    assert retained.status == 200 and retained.json()["revision"]["state"] == "WITHDRAWN"


@pytest.mark.case("M07-CP03-reference-context-boundary")
def test_draft_and_foreign_contracts_fail_closed(lab):
    cls, activity, teacher, student, outsider = classroom(lab)
    base = f"/v3/classes/{cls['id']}/activities/{activity['id']}/revisions"
    draft = mutate(lab, teacher, base, revision("draft", rule())).json()
    assert student[0].get(base + f"/{draft['id']}").status == 404
    assert outsider[0].get(base + f"/{draft['id']}").status == 403
    foreign_cls, foreign_activity, foreign_teacher, _, _ = classroom(lab, "M07 CP03 foreign")
    foreign_base = f"/v3/classes/{foreign_cls['id']}/activities/{foreign_activity['id']}/revisions"
    foreign = mutate(lab, foreign_teacher, foreign_base, revision("foreign", rule())).json()
    foreign = mutate(lab, foreign_teacher, foreign_base + f"/{foreign['id']}/publish", {"version": foreign["version"]}).json()
    assert student[0].get(base + f"/{foreign['id']}").status == 404
