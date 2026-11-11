import uuid

import pytest

from qa.tests.gradebook_api.test_gradebook import classroom, create_definition, handin, mutate, publish


def rule_path(room):
    return f"/v3/classes/{room['class']['id']}/activities/{room['activity']['id']}/completion-rules"


def evaluation_path(room):
    return f"/v3/classes/{room['class']['id']}/activities/{room['activity']['id']}/completion/evaluate"


def rule_body(room, expected=0):
    return {
        "content_revision_id": room["revision"]["id"],
        "expected_version": expected,
        "requirement": "EVIDENCE_PRESENT",
        "evidence_kind": "ACTIVITY_ACKNOWLEDGED",
    }


def create_rule(lab, room, expected=0):
    response = mutate(lab, room["pairs"]["teacher"], rule_path(room), rule_body(room, expected))
    assert response.status == 201
    return response.json()


@pytest.mark.case("M10-CP01-rule-snapshot")
def test_rule_snapshot_is_versioned_immutable_and_idempotent(lab):
    room = classroom(lab, "M10 completion rules")
    key = uuid.uuid4().hex
    first = mutate(lab, room["pairs"]["teacher"], rule_path(room), rule_body(room), key=key)
    replay = mutate(lab, room["pairs"]["teacher"], rule_path(room), rule_body(room), key=key)
    assert first.status == replay.status == 201
    assert replay.headers["idempotency-replayed"] == "true"
    assert first.json() == replay.json() and first.json()["version"] == 1
    second = create_rule(lab, room, 1)
    assert second["version"] == 2 and second["id"] != first.json()["id"]
    stale = mutate(lab, room["pairs"]["teacher"], rule_path(room), rule_body(room, 0))
    assert stale.status == 409
    unknown = {**rule_body(room, 2), "frontend_can_complete": True}
    assert mutate(lab, room["pairs"]["teacher"], rule_path(room), unknown).status == 400


@pytest.mark.case("M10-CP01-grade-separation")
def test_explicit_evidence_completes_without_grade_and_grade_alone_does_not(lab):
    no_grade = classroom(lab, "M10 no grade completion")
    rule = create_rule(lab, no_grade)
    body = {"content_revision_id": no_grade["revision"]["id"], "rule_version_id": rule["id"], "acknowledge": True}
    completed = mutate(lab, no_grade["pairs"]["student"], evaluation_path(no_grade), body)
    assert completed.status == 200
    assert completed.json()["state"] == "COMPLETED" and completed.json()["evidence_id"]

    high_grade = classroom(lab, "M10 high grade incomplete", second_teacher=True)
    definition = create_definition(lab, high_grade)
    assert publish(lab, high_grade, definition).status == 200
    aggregate, revision = handin(lab, high_grade)
    grades = f"/v3/classes/{high_grade['class']['id']}/grade-items/{definition['grade_item_id']}/grades"
    assert mutate(lab, high_grade["pairs"]["teacher"], grades, {
        "aggregate_id": aggregate["id"], "submission_revision_id": revision["id"],
        "expected_version": 0, "value_milli": 100000, "reason": "High grade",
    }).status == 200
    assert mutate(lab, high_grade["pairs"]["teacher"], grades + f"/{aggregate['id']}/release", {"expected_version": 1}).status == 200
    high_rule = create_rule(lab, high_grade)
    incomplete = mutate(lab, high_grade["pairs"]["student"], evaluation_path(high_grade), {
        "content_revision_id": high_grade["revision"]["id"], "rule_version_id": high_rule["id"], "acknowledge": False,
    })
    assert incomplete.status == 200
    assert incomplete.json()["state"] == "INCOMPLETE"
    assert incomplete.json()["reason"] == "EVIDENCE_MISSING" and "evidence_id" not in incomplete.json()


@pytest.mark.case("M10-CP01-security-boundary")
def test_completion_authority_and_ancestry_fail_closed(lab):
    room = classroom(lab, "M10 completion boundary")
    foreign = classroom(lab, "M10 completion foreign")
    assert mutate(lab, room["pairs"]["student"], rule_path(room), rule_body(room)).status == 403
    assert mutate(lab, room["pairs"]["outsider"], rule_path(room), rule_body(room)).status == 403
    rule = create_rule(lab, room)
    body = {"content_revision_id": room["revision"]["id"], "rule_version_id": rule["id"], "acknowledge": True}
    assert mutate(lab, room["pairs"]["outsider"], evaluation_path(room), body).status == 403
    assert mutate(lab, room["pairs"]["student"], evaluation_path(room), {**body, "content_revision_id": foreign["revision"]["id"]}).status == 404
    key = uuid.uuid4().hex
    first = mutate(lab, room["pairs"]["student"], evaluation_path(room), body, key=key)
    replay = mutate(lab, room["pairs"]["student"], evaluation_path(room), body, key=key)
    assert first.status == replay.status == 200 and replay.headers["idempotency-replayed"] == "true"
    assert first.json() == replay.json()
