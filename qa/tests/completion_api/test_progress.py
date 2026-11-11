import uuid

import pytest

from qa.tests.completion_api.test_completion import classroom, create_rule, evaluation_path
from qa.tests.gradebook_api.test_gradebook import mutate


def progress_path(room):
    return f"/v3/classes/{room['class']['id']}/progress"


def correction_path(room):
    return f"/v3/classes/{room['class']['id']}/activities/{room['activity']['id']}/completion/override"


def correction_body(room, rule, learner_id, version=0, action="OVERRIDE_COMPLETE"):
    return {
        "rule_version_id": rule["id"], "learner_user_id": learner_id,
        "action": action, "reason": "Teacher verified offline activity",
        "expected_version": version,
    }


@pytest.mark.case("M10-CP02-progress-projection")
def test_progress_rebuild_is_read_only_and_consistent(lab):
    room = classroom(lab, "M10 progress projection")
    rule = create_rule(lab, room)
    student = room["pairs"]["student"][0]
    initial = student.get(progress_path(room))
    assert initial.status == 200
    assert initial.json()["completed"] == 0 and initial.json()["total"] == 1
    assert initial.json()["activities"][0]["version"] == 0
    result = mutate(lab, room["pairs"]["student"], evaluation_path(room), {
        "content_revision_id": room["revision"]["id"], "rule_version_id": rule["id"], "acknowledge": True,
    })
    assert result.status == 200
    first = student.get(progress_path(room))
    second = student.get(progress_path(room))
    assert first.status == second.status == 200
    assert first.json() == second.json()
    assert first.json()["completed"] == 1 and first.json()["activities"][0]["version"] == 1


@pytest.mark.case("M10-CP02-override-reopen")
def test_teacher_correction_reopen_and_evaluation(lab):
    room = classroom(lab, "M10 correction")
    rule = create_rule(lab, room)
    key = uuid.uuid4().hex
    body = correction_body(room, rule, lab["actors"]["student"]["id"])
    first = mutate(lab, room["pairs"]["teacher"], correction_path(room), body, key=key)
    replay = mutate(lab, room["pairs"]["teacher"], correction_path(room), body, key=key)
    assert first.status == replay.status == 200
    assert replay.headers["idempotency-replayed"] == "true"
    assert first.json() == replay.json() and first.json()["version"] == 1
    student = room["pairs"]["student"][0]
    assert student.get(progress_path(room)).json()["completed"] == 1
    evaluation = {
        "content_revision_id": room["revision"]["id"], "rule_version_id": rule["id"], "acknowledge": True,
    }
    assert mutate(lab, room["pairs"]["student"], evaluation_path(room), evaluation).status == 409
    reopened = mutate(lab, room["pairs"]["teacher"], correction_path(room), {
        **body, "action": "REOPEN", "expected_version": 1, "reason": "Reopen for new learner evidence",
    })
    assert reopened.status == 200 and reopened.json()["version"] == 2
    assert student.get(progress_path(room)).json()["completed"] == 0
    assert mutate(lab, room["pairs"]["student"], evaluation_path(room), evaluation).status == 200
    progress = student.get(progress_path(room)).json()
    assert progress["completed"] == 1 and progress["activities"][0]["version"] == 3


@pytest.mark.case("M10-CP02-security-boundary")
def test_correction_scope_and_get_repair_boundary(lab):
    room = classroom(lab, "M10 correction boundary")
    foreign = classroom(lab, "M10 correction foreign")
    rule = create_rule(lab, room)
    foreign_rule = create_rule(lab, foreign)
    body = correction_body(room, rule, lab["actors"]["student"]["id"])
    path = correction_path(room)
    assert mutate(lab, room["pairs"]["student"], path, body).status == 403
    assert mutate(lab, room["pairs"]["outsider"], path, body).status == 403
    assert mutate(lab, room["pairs"]["teacher"], path, {**body, "rule_version_id": foreign_rule["id"]}).status == 404
    assert mutate(lab, room["pairs"]["teacher"], path, {**body, "expected_version": 9}).status == 409
    assert mutate(lab, room["pairs"]["teacher"], path, {**body, "reason": "x"}).status == 400
    outsider = room["pairs"]["outsider"][0]
    assert outsider.get(progress_path(room)).status == 403
    assert outsider.get(f"/v3/classes/{room['class']['id']}/learners/{lab['actors']['student']['id']}/progress").status == 403
    student = room["pairs"]["student"][0]
    assert student.get(progress_path(room) + "/rebuild").status == 404
    assert student.get(progress_path(room)).json()["completed"] == 0
