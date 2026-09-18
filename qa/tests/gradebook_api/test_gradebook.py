from datetime import datetime, timedelta, timezone
import uuid

import pytest


def mutate(lab, pair, path, body, method="post", key=None):
    return lab["mutate"](pair[0], pair[1], path, body, method=method, key=key or uuid.uuid4().hex)


def classroom(lab, name, second_teacher=False):
    pairs = {subject: lab["login"](subject) for subject in ("admin", "teacher", "teacher2", "student", "outsider")}
    created = mutate(lab, pairs["admin"], "/v3/classes", {"name": name}).json()
    roles = [("teacher", "teacher"), ("student", "student")]
    if second_teacher:
        roles.append(("teacher2", "teacher"))
    for subject, role in roles:
        response = mutate(lab, pairs["admin"], f"/v3/classes/{created['id']}/participants", {"user_id": lab["actors"][subject]["id"], "role": role})
        assert response.status == 200
    assert mutate(lab, pairs["admin"], f"/v3/classes/{created['id']}/publish", {"version": 1}).status == 200
    section = mutate(lab, pairs["teacher"], f"/v3/classes/{created['id']}/sections", {"title": "Work", "position": 1}).json()
    activity = mutate(lab, pairs["teacher"], f"/v3/classes/{created['id']}/activities", {"section_id": section["id"], "activity_type": "ASSIGNMENT", "title": "Essay", "position": 1}).json()
    now = datetime.now(timezone.utc)
    revision = mutate(lab, pairs["teacher"], f"/v3/classes/{created['id']}/activities/{activity['id']}/revisions", {
        "raw_body": "Write an essay", "timezone": "Asia/Jakarta", "available_from": (now-timedelta(hours=1)).isoformat(),
        "due_at": (now+timedelta(hours=1)).isoformat(), "cutoff_at": (now+timedelta(hours=2)).isoformat(),
        "assignment": {"submission_mode": "TEXT", "max_attempts": 2, "max_files": 0, "max_file_bytes": 0, "late_policy": "REJECT_AFTER_CUTOFF"},
    }).json()
    revision = mutate(lab, pairs["teacher"], f"/v3/classes/{created['id']}/activities/{activity['id']}/revisions/{revision['id']}/publish", {"version": revision["version"]}).json()
    return {"class": created, "activity": activity, "revision": revision, "pairs": pairs}


def definition_body(weights=(6000, 4000), parents=("", "")):
    return {"activity_id": None, "title": "Essay grade", "weight_bps": 10000,
            "scale": {"name": "Points", "minimum_milli": 0, "maximum_milli": 100000, "display_decimals": 3},
            "criteria": [
                {"key": "analysis", "title": "Analysis", "parent_key": parents[0], "weight_bps": weights[0], "maximum_milli": 100000, "position": 1},
                {"key": "clarity", "title": "Clarity", "parent_key": parents[1], "weight_bps": weights[1], "maximum_milli": 100000, "position": 2},
            ]}


def create_definition(lab, room, body=None):
    body = body or definition_body()
    body["activity_id"] = room["activity"]["id"]
    response = mutate(lab, room["pairs"]["teacher"], f"/v3/classes/{room['class']['id']}/grade-items", body)
    assert response.status == 201
    return response.json()


def publish(lab, room, definition, expected=0):
    return mutate(lab, room["pairs"]["teacher"], f"/v3/classes/{room['class']['id']}/grade-items/{definition['grade_item_id']}/versions/{definition['definition_version_id']}/publish", {"expected_publication_revision": expected})


def handin(lab, room):
    base = f"/v3/classes/{room['class']['id']}"
    aggregate = mutate(lab, room["pairs"]["student"], base+f"/activities/{room['activity']['id']}/revisions/{room['revision']['id']}/submissions", {}).json()
    revision = mutate(lab, room["pairs"]["student"], base+f"/submissions/{aggregate['id']}/revisions", {"version": 1, "body": "final answer", "file_asset_ids": []}).json()
    receipt = mutate(lab, room["pairs"]["student"], base+f"/submissions/{aggregate['id']}/handin", {"version": 1, "attempt_version": 1})
    assert receipt.status == 200 and receipt.json()["revision_id"] == revision["id"]
    return aggregate, revision


@pytest.mark.case("M09-CP01-definition-lifecycle")
def test_definition_versions_publish_and_replay(lab):
    room = classroom(lab, "M09 definition lifecycle")
    definition = create_definition(lab, room)
    key = uuid.uuid4().hex
    path = f"/v3/classes/{room['class']['id']}/grade-items/{definition['grade_item_id']}/versions/{definition['definition_version_id']}/publish"
    first = mutate(lab, room["pairs"]["teacher"], path, {"expected_publication_revision": 0}, key=key)
    replay = mutate(lab, room["pairs"]["teacher"], path, {"expected_publication_revision": 0}, key=key)
    assert first.status == 200 and replay.status == 200 and replay.headers["idempotency-replayed"] == "true"
    versions = room["pairs"]["teacher"][0].get(f"/v3/classes/{room['class']['id']}/grade-items/{definition['grade_item_id']}")
    assert versions.status == 200 and len(versions.json()) == 1 and len(versions.json()[0]["criteria"]) == 2


@pytest.mark.case("M09-CP01-definition-boundary")
def test_invalid_rubric_cycle_and_foreign_actor_fail_closed(lab):
    bad_room = classroom(lab, "M09 invalid weights")
    bad = create_definition(lab, bad_room, definition_body((6000, 3000)))
    assert publish(lab, bad_room, bad).status == 409
    cycle_room = classroom(lab, "M09 rubric cycle")
    cyclic = create_definition(lab, cycle_room, definition_body((5000, 5000), ("clarity", "analysis")))
    assert publish(lab, cycle_room, cyclic).status == 409
    body = definition_body(); body["activity_id"] = cycle_room["activity"]["id"]
    denied = mutate(lab, cycle_room["pairs"]["outsider"], f"/v3/classes/{cycle_room['class']['id']}/grade-items", body)
    assert denied.status == 403


@pytest.mark.case("M09-CP02-grade-lifecycle")
def test_grade_and_reasoned_regrade_are_versioned(lab):
    room = classroom(lab, "M09 grade lifecycle", second_teacher=True)
    definition = create_definition(lab, room); assert publish(lab, room, definition).status == 200
    aggregate, revision = handin(lab, room)
    path = f"/v3/classes/{room['class']['id']}/grade-items/{definition['grade_item_id']}/grades"
    initial = mutate(lab, room["pairs"]["teacher"], path, {"aggregate_id": aggregate["id"], "submission_revision_id": revision["id"], "expected_version": 0, "value_milli": 80000, "reason": ""})
    assert initial.status == 200 and initial.json()["version"] == 1 and initial.json()["reason"] == "INITIAL"
    regrade = mutate(lab, room["pairs"]["teacher2"], path, {"aggregate_id": aggregate["id"], "submission_revision_id": revision["id"], "expected_version": 1, "value_milli": 85000, "reason": "Moderated evidence"})
    assert regrade.status == 200 and regrade.json()["version"] == 2
    current = room["pairs"]["teacher"][0].get(path+f"/{aggregate['id']}")
    assert current.status == 200 and current.json()["value_milli"] == 85000 and current.json()["submission_revision_id"] == revision["id"]


@pytest.mark.case("M09-CP02-grade-boundary")
def test_unreleased_foreign_and_stale_grade_access_fails_closed(lab):
    room = classroom(lab, "M09 grade boundary", second_teacher=True)
    definition = create_definition(lab, room); assert publish(lab, room, definition).status == 200
    aggregate, revision = handin(lab, room)
    foreign = classroom(lab, "M09 foreign submission"); _, foreign_revision = handin(lab, foreign)
    path = f"/v3/classes/{room['class']['id']}/grade-items/{definition['grade_item_id']}/grades"
    body = {"aggregate_id": aggregate["id"], "submission_revision_id": revision["id"], "expected_version": 0, "value_milli": 70000, "reason": ""}
    assert mutate(lab, room["pairs"]["teacher"], path, {**body, "submission_revision_id": foreign_revision["id"]}).status == 404
    assert mutate(lab, room["pairs"]["teacher"], path, body).status == 200
    assert mutate(lab, room["pairs"]["teacher2"], path, {**body, "value_milli": 71000, "reason": "stale"}).status == 409
    assert room["pairs"]["student"][0].get(path+f"/{aggregate['id']}").status == 403
