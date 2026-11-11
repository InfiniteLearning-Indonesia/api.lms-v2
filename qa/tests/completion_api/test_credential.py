import uuid

import pytest

from qa.tests.completion_api.test_completion import create_rule, evaluation_path
from qa.tests.gradebook_api.test_gradebook import classroom, create_definition, handin, mutate, publish


def graded_room(lab, name, complete=False):
    room = classroom(lab, name)
    definition = create_definition(lab, room)
    assert publish(lab, room, definition).status == 200
    aggregate, revision = handin(lab, room)
    grade_path = f"/v3/classes/{room['class']['id']}/grade-items/{definition['grade_item_id']}/grades"
    assert mutate(lab, room["pairs"]["teacher"], grade_path, {
        "aggregate_id": aggregate["id"], "submission_revision_id": revision["id"],
        "expected_version": 0, "value_milli": 100000, "reason": "Initial grade",
    }).status == 200
    assert mutate(lab, room["pairs"]["teacher"], grade_path + f"/{aggregate['id']}/release", {
        "expected_version": 1,
    }).status == 200
    rule = create_rule(lab, room)
    if complete:
        assert mutate(lab, room["pairs"]["student"], evaluation_path(room), {
            "content_revision_id": room["revision"]["id"], "rule_version_id": rule["id"], "acknowledge": True,
        }).status == 200
    return room, definition, aggregate, revision, grade_path


def release_path(room, learner_id):
    return f"/v3/classes/{room['class']['id']}/learners/{learner_id}/transcripts/releases"


def issue_path(room, learner_id):
    return f"/v3/classes/{room['class']['id']}/learners/{learner_id}/credentials"


def release(lab, room):
    response = mutate(lab, room["pairs"]["teacher"], release_path(room, lab["actors"]["student"]["id"]), {})
    assert response.status == 201
    return response.json()


@pytest.mark.case("M10-CP03-transcript-snapshot")
def test_explicit_transcript_preserves_grade_provenance(lab):
    room, definition, aggregate, revision, grade_path = graded_room(lab, "M10 transcript provenance")
    learner_id = lab["actors"]["student"]["id"]
    assert mutate(lab, room["pairs"]["student"], release_path(room, learner_id), {}).status == 403
    first = release(lab, room)
    assert first["policy_ref"] == "SYNTHETIC_M10_CP03_V1" and first["version"] == 1
    item = first["items"][0]
    assert item["raw_value_milli"] == 100000 and item["scale_minimum_milli"] == 0
    assert item["scale_maximum_milli"] == 100000 and item["grade_release_id"]
    latest = room["pairs"]["student"][0].get(
        f"/v3/classes/{room['class']['id']}/learners/{learner_id}/transcripts/latest"
    )
    assert latest.status == 200 and latest.json() == first
    assert mutate(lab, room["pairs"]["teacher"], grade_path, {
        "aggregate_id": aggregate["id"], "submission_revision_id": revision["id"],
        "expected_version": 1, "value_milli": 90000, "reason": "Correction",
    }).status == 200
    assert mutate(lab, room["pairs"]["teacher"], grade_path + f"/{aggregate['id']}/release", {
        "expected_version": 2,
    }).status == 200
    second = release(lab, room)
    assert second["version"] == 2 and second["grade_source_hash"] != first["grade_source_hash"]
    assert second["items"][0]["raw_value_milli"] == 90000
    assert first["items"][0]["raw_value_milli"] == 100000


@pytest.mark.case("M10-CP03-credential-lifecycle")
def test_issue_verify_privacy_revoke_and_supersede(lab):
    room, *_ = graded_room(lab, "M10 credential lifecycle", complete=True)
    learner_id = lab["actors"]["student"]["id"]
    first = release(lab, room)
    issue = issue_path(room, learner_id)
    body = {"transcript_id": first["id"], "privacy": "PUBLIC_MINIMAL"}
    key = uuid.uuid4().hex
    created = mutate(lab, room["pairs"]["teacher"], issue, body, key=key)
    replay = mutate(lab, room["pairs"]["teacher"], issue, body, key=key)
    assert created.status == replay.status == 201
    assert replay.headers["idempotency-replayed"] == "true" and created.json() == replay.json()
    old_id = created.json()["id"]
    public = lab["client"]()
    verified = public.get(f"/v3/credentials/{old_id}/verify")
    assert verified.status == 200 and verified.json() == {"id": old_id, "status": "VALID"}
    private_transcript = release(lab, room)
    private = mutate(lab, room["pairs"]["teacher"], issue, {
        "transcript_id": private_transcript["id"], "privacy": "PRIVATE",
    })
    assert private.status == 201
    private_id = private.json()["id"]
    assert public.get(f"/v3/credentials/{private_id}/verify").status == 404
    revoked = mutate(lab, room["pairs"]["teacher"], f"/v3/credentials/{private_id}/revoke", {
        "expected_version": 1, "reason": "Credential withdrawn after review",
    })
    assert revoked.status == 200 and revoked.json()["status"] == "REVOKED"
    successor_transcript = release(lab, room)
    superseded = mutate(lab, room["pairs"]["teacher"], f"/v3/credentials/{old_id}/supersede", {
        "expected_version": 1, "reason": "Reissued with later transcript",
        "successor_transcript_id": successor_transcript["id"], "privacy": "PUBLIC_MINIMAL",
    })
    assert superseded.status == 200 and superseded.json()["status"] == "SUPERSEDED"
    assert public.get(f"/v3/credentials/{old_id}/verify").json() == {"id": old_id, "status": "SUPERSEDED"}
    successor_id = superseded.json()["successor_id"]
    assert public.get(f"/v3/credentials/{successor_id}/verify").json() == {"id": successor_id, "status": "VALID"}


@pytest.mark.case("M10-CP03-security-boundary")
def test_credential_issuance_requires_scope_completion_and_current_transcript(lab):
    room, *_ = graded_room(lab, "M10 credential boundary")
    learner_id = lab["actors"]["student"]["id"]
    transcript = release(lab, room)
    issue = issue_path(room, learner_id)
    body = {"transcript_id": transcript["id"], "privacy": "PUBLIC_MINIMAL"}
    assert mutate(lab, room["pairs"]["teacher"], issue, body).status == 409
    assert mutate(lab, room["pairs"]["student"], issue, body).status == 403
    assert mutate(lab, room["pairs"]["outsider"], issue, body).status == 403
    other, *_ = graded_room(lab, "M10 credential foreign", complete=True)
    foreign = release(lab, other)
    assert mutate(lab, other["pairs"]["teacher"], issue, {
        "transcript_id": foreign["id"], "privacy": "PUBLIC_MINIMAL",
    }).status in (403, 409)
    rule = create_rule(lab, room, 1)
    assert mutate(lab, room["pairs"]["student"], evaluation_path(room), {
        "content_revision_id": room["revision"]["id"], "rule_version_id": rule["id"], "acknowledge": True,
    }).status == 200
    newer = release(lab, room)
    assert mutate(lab, room["pairs"]["teacher"], issue, body).status == 409
    issued = mutate(lab, room["pairs"]["teacher"], issue, {
        "transcript_id": newer["id"], "privacy": "PUBLIC_MINIMAL",
    })
    assert issued.status == 201
    credential_id = issued.json()["id"]
    assert mutate(lab, room["pairs"]["outsider"], f"/v3/credentials/{credential_id}/revoke", {
        "expected_version": 1, "reason": "Unauthorized cancellation",
    }).status == 403
    assert mutate(lab, room["pairs"]["teacher"], f"/v3/credentials/{credential_id}/revoke", {
        "expected_version": 9, "reason": "Stale cancellation request",
    }).status == 409
    verification = lab["client"]().get(f"/v3/credentials/{credential_id}/verify")
    assert verification.status == 200
    assert set(verification.json()) == {"id", "status"}
