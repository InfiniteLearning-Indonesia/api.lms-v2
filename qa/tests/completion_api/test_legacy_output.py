import hashlib
import uuid

import pytest

from qa.tests.completion_api.test_credential import graded_room, issue_path, release, release_path
from qa.tests.gradebook_api.test_gradebook import mutate


OBSERVE_PATH = "/v3/legacy/completion-observations"


def observation(room, learner_id, state="TRUE", source=None):
    return {
        "source_instance_id": source or "local-synthetic-source",
        "source_snapshot_sha256": "a" * 64,
        "rows": [{
            "source_table": "certificate_projection", "source_pk": uuid.uuid4().hex,
            "source_version": "1", "raw_payload": {"visible": state == "TRUE"},
            "observed_state": state, "candidate_class_id": room["class"]["id"],
            "candidate_learner_user_id": learner_id,
        }],
    }


@pytest.mark.case("M10-CP04-observed-not-issued")
def test_observed_legacy_flag_never_issues_credential(lab):
    room, *_ = graded_room(lab, "M10 observed legacy output")
    learner = lab["actors"]["student"]["id"]
    body = observation(room, learner, source="observed-" + uuid.uuid4().hex)
    response = mutate(lab, room["pairs"]["admin"], OBSERVE_PATH, body)
    assert response.status == 201
    row = response.json()["observations"][0]
    assert row["observed_state"] == "TRUE" and row["disposition"] == "OBSERVED_UNVERIFIED"
    assert row["raw_sha256"] == hashlib.sha256(b'{"visible":true}').hexdigest()
    assert room["pairs"]["student"][0].get(
        f"/v3/classes/{room['class']['id']}/learners/{learner}/transcripts/latest"
    ).status == 404
    assert mutate(lab, room["pairs"]["teacher"], issue_path(room, learner), {
        "transcript_id": row["id"], "privacy": "PUBLIC_MINIMAL",
    }).status == 409


@pytest.mark.case("M10-CP04-provenance-retry")
def test_observation_provenance_and_conflicting_retries(lab):
    room, *_ = graded_room(lab, "M10 legacy provenance")
    learner = lab["actors"]["student"]["id"]
    body = observation(room, learner, state="UNKNOWN", source="unknown-" + uuid.uuid4().hex)
    key = uuid.uuid4().hex
    first = mutate(lab, room["pairs"]["admin"], OBSERVE_PATH, body, key=key)
    replay = mutate(lab, room["pairs"]["admin"], OBSERVE_PATH, body, key=key)
    assert first.status == replay.status == 201
    assert replay.headers["idempotency-replayed"] == "true" and replay.json() == first.json()
    assert first.json()["source_instance_id"] == body["source_instance_id"]
    assert first.json()["observations"][0]["disposition"] == "UNKNOWN_HOLD"
    changed = {**body, "rows": [{**body["rows"][0], "observed_state": "TRUE"}]}
    assert mutate(lab, room["pairs"]["admin"], OBSERVE_PATH, changed, key=key).status == 409
    assert mutate(lab, room["pairs"]["admin"], OBSERVE_PATH, body).status == 409


@pytest.mark.case("M10-CP04-security-stability")
def test_operator_scope_foreign_candidate_and_old_credential_stability(lab):
    room, _, aggregate, revision, grade_path = graded_room(lab, "M10 legacy stable credential", complete=True)
    learner = lab["actors"]["student"]["id"]
    transcript = release(lab, room)
    issued = mutate(lab, room["pairs"]["teacher"], issue_path(room, learner), {
        "transcript_id": transcript["id"], "privacy": "PUBLIC_MINIMAL",
    })
    assert issued.status == 201
    body = observation(room, learner, source="stability-" + uuid.uuid4().hex)
    assert mutate(lab, room["pairs"]["teacher"], OBSERVE_PATH, body).status == 403
    assert mutate(lab, room["pairs"]["student"], OBSERVE_PATH, body).status == 403
    wrong = {**body, "rows": [{**body["rows"][0], "candidate_learner_user_id": lab["actors"]["outsider"]["id"]}]}
    assert mutate(lab, room["pairs"]["admin"], OBSERVE_PATH, wrong).status == 409
    saved = mutate(lab, room["pairs"]["admin"], OBSERVE_PATH, body)
    assert saved.status == 201
    assert mutate(lab, room["pairs"]["teacher"], grade_path, {
        "aggregate_id": aggregate["id"], "submission_revision_id": revision["id"],
        "expected_version": 1, "value_milli": 90000, "reason": "Later correction",
    }).status == 200
    assert mutate(lab, room["pairs"]["teacher"], grade_path + f"/{aggregate['id']}/release", {
        "expected_version": 2,
    }).status == 200
    newer = release(lab, room)
    assert newer["grade_source_hash"] != transcript["grade_source_hash"]
    assert transcript["items"][0]["raw_value_milli"] == 100000
    verified = lab["client"]().get(f"/v3/credentials/{issued.json()['id']}/verify")
    assert verified.status == 200 and verified.json()["status"] == "VALID"
