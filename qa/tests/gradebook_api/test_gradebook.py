from datetime import datetime, timedelta, timezone
import uuid

import pytest


def mutate(lab, pair, path, body, method="post", key=None):
    return lab["mutate"](pair[0], pair[1], path, body, method=method, key=key or uuid.uuid4().hex)


def classroom(lab, name, second_teacher=False, second_student=False):
    pairs = {subject: lab["login"](subject) for subject in ("admin", "teacher", "teacher2", "student", "outsider")}
    created = mutate(lab, pairs["admin"], "/v3/classes", {"name": name}).json()
    roles = [("teacher", "teacher"), ("student", "student")]
    if second_teacher:
        roles.append(("teacher2", "teacher"))
    if second_student:
        roles.append(("outsider", "student"))
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


def handin(lab, room, subject="student"):
    base = f"/v3/classes/{room['class']['id']}"
    aggregate = mutate(lab, room["pairs"][subject], base+f"/activities/{room['activity']['id']}/revisions/{room['revision']['id']}/submissions", {}).json()
    revision = mutate(lab, room["pairs"][subject], base+f"/submissions/{aggregate['id']}/revisions", {"version": 1, "body": "final answer", "file_asset_ids": []}).json()
    receipt = mutate(lab, room["pairs"][subject], base+f"/submissions/{aggregate['id']}/handin", {"version": 1, "attempt_version": 1})
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


def import_paths(room, definition):
    base = f"/v3/classes/{room['class']['id']}/grade-items/{definition['grade_item_id']}"
    return base+"/imports/preview", base+"/imports/confirm", base+"/grades"


@pytest.mark.case("M09-CP03-import-lifecycle")
def test_import_preview_confirm_atomic_replay(lab):
    room = classroom(lab, "M09 import lifecycle", second_teacher=True)
    definition = create_definition(lab, room); assert publish(lab, room, definition).status == 200
    aggregate, revision = handin(lab, room)
    preview_path, confirm_path, grades = import_paths(room, definition)
    entries = [{"aggregate_id": aggregate["id"], "submission_revision_id": revision["id"], "expected_version": 0, "value_milli": 80000}]
    key = uuid.uuid4().hex
    preview = mutate(lab, room["pairs"]["teacher"], preview_path, {"entries": entries}, key=key)
    replay = mutate(lab, room["pairs"]["teacher"], preview_path, {"entries": entries}, key=key)
    assert preview.status == replay.status == 200 and replay.headers["idempotency-replayed"] == "true"
    assert preview.json()["id"] == replay.json()["id"] and preview.json()["publication_revision"] == 1
    assert room["pairs"]["teacher"][0].get(grades+"/"+aggregate["id"]).status == 404
    body = {"preview_id": preview.json()["id"], "entries": entries}
    key = uuid.uuid4().hex
    confirmed = mutate(lab, room["pairs"]["teacher"], confirm_path, body, key=key)
    replay = mutate(lab, room["pairs"]["teacher"], confirm_path, body, key=key)
    assert confirmed.status == replay.status == 200 and replay.headers["idempotency-replayed"] == "true"
    assert confirmed.json() == replay.json() and len(confirmed.json()) == 1
    assert confirmed.json()[0]["reason"] == "IMPORT_INITIAL" and confirmed.json()[0]["version"] == 1


@pytest.mark.case("M09-CP03-import-boundary")
def test_import_changed_actor_stale_and_invalid_denied(lab):
    room = classroom(lab, "M09 import boundary", second_teacher=True)
    definition = create_definition(lab, room); assert publish(lab, room, definition).status == 200
    aggregate, revision = handin(lab, room)
    preview_path, confirm_path, grades = import_paths(room, definition)
    entry = {"aggregate_id": aggregate["id"], "submission_revision_id": revision["id"], "expected_version": 0, "value_milli": 80000}
    assert mutate(lab, room["pairs"]["outsider"], preview_path, {"entries": [entry]}).status == 403
    assert mutate(lab, room["pairs"]["teacher"], preview_path, {"entries": [entry, entry]}).status == 400
    preview = mutate(lab, room["pairs"]["teacher"], preview_path, {"entries": [entry]})
    assert preview.status == 200
    body = {"preview_id": preview.json()["id"], "entries": [entry]}
    assert mutate(lab, room["pairs"]["teacher"], confirm_path, {**body, "entries": [{**entry, "value_milli": 90000}]}).status == 409
    assert mutate(lab, room["pairs"]["teacher2"], confirm_path, body).status == 404
    assert mutate(lab, room["pairs"]["teacher"], grades, {**entry, "reason": ""}).status == 200
    assert mutate(lab, room["pairs"]["teacher"], confirm_path, body).status == 409
    assert room["pairs"]["teacher"][0].get(grades+"/"+aggregate["id"]).json()["value_milli"] == 80000
    # A later row goes stale after preview; the earlier row must not commit.
    atomic = classroom(lab, "M09 atomic import", second_student=True)
    atomic_definition = create_definition(lab, atomic); assert publish(lab, atomic, atomic_definition).status == 200
    first_aggregate, first_revision = handin(lab, atomic)
    second_aggregate, second_revision = handin(lab, atomic, "outsider")
    atomic_preview_path, atomic_confirm_path, atomic_grades = import_paths(atomic, atomic_definition)
    entries = sorted([
        {"aggregate_id": first_aggregate["id"], "submission_revision_id": first_revision["id"], "expected_version": 0, "value_milli": 81000},
        {"aggregate_id": second_aggregate["id"], "submission_revision_id": second_revision["id"], "expected_version": 0, "value_milli": 82000},
    ], key=lambda entry: entry["aggregate_id"])
    first, second = entries
    pending = mutate(lab, atomic["pairs"]["teacher"], atomic_preview_path, {"entries": [first, second]})
    assert pending.status == 200
    assert mutate(lab, atomic["pairs"]["teacher"], atomic_grades, {**second, "value_milli": 70000}).status == 200
    rejected = mutate(lab, atomic["pairs"]["teacher"], atomic_confirm_path, {"preview_id": pending.json()["id"], "entries": [first, second]})
    assert rejected.status == 409
    assert atomic["pairs"]["teacher"][0].get(atomic_grades+"/"+first["aggregate_id"]).status == 404


@pytest.mark.case("M09-CP03-release-lifecycle")
def test_release_only_exposes_explicit_revision(lab):
    room = classroom(lab, "M09 release lifecycle", second_teacher=True)
    definition = create_definition(lab, room); assert publish(lab, room, definition).status == 200
    aggregate, revision = handin(lab, room)
    _, _, grades = import_paths(room, definition)
    body = {"aggregate_id": aggregate["id"], "submission_revision_id": revision["id"], "expected_version": 0, "value_milli": 75000, "reason": ""}
    assert mutate(lab, room["pairs"]["teacher"], grades, body).status == 200
    read = grades+"/"+aggregate["id"]
    assert room["pairs"]["student"][0].get(read).status == 404
    assert mutate(lab, room["pairs"]["teacher"], read+"/release", {"expected_version": 1}).status == 200
    assert room["pairs"]["student"][0].get(read).json()["value_milli"] == 75000
    assert room["pairs"]["outsider"][0].get(read).status == 403
    assert mutate(lab, room["pairs"]["teacher2"], grades, {**body, "expected_version": 1, "value_milli": 90000, "reason": "Moderated"}).status == 200
    assert room["pairs"]["student"][0].get(read).json()["value_milli"] == 75000
    assert mutate(lab, room["pairs"]["teacher"], read+"/release", {"expected_version": 2}).status == 200
    assert room["pairs"]["student"][0].get(read).json()["value_milli"] == 90000


@pytest.mark.case("M09-CP03-release-boundary")
def test_release_permissions_stale_duplicate_and_replay(lab):
    room = classroom(lab, "M09 release boundary", second_teacher=True)
    definition = create_definition(lab, room); assert publish(lab, room, definition).status == 200
    aggregate, revision = handin(lab, room)
    _, _, grades = import_paths(room, definition)
    assert mutate(lab, room["pairs"]["teacher"], grades, {"aggregate_id": aggregate["id"], "submission_revision_id": revision["id"], "expected_version": 0, "value_milli": 75000}).status == 200
    path = grades+"/"+aggregate["id"]+"/release"
    assert mutate(lab, room["pairs"]["student"], path, {"expected_version": 1}).status == 403
    assert mutate(lab, room["pairs"]["outsider"], path, {"expected_version": 1}).status == 403
    assert mutate(lab, room["pairs"]["teacher"], path, {"expected_version": 2}).status == 409
    key = uuid.uuid4().hex
    first = mutate(lab, room["pairs"]["teacher"], path, {"expected_version": 1}, key=key)
    replay = mutate(lab, room["pairs"]["teacher"], path, {"expected_version": 1}, key=key)
    assert first.status == replay.status == 200 and replay.headers["idempotency-replayed"] == "true"
    assert first.json() == replay.json()
    assert mutate(lab, room["pairs"]["teacher2"], path, {"expected_version": 1}).status == 409


def historical_room(lab, name, release=True):
    room = classroom(lab, name, second_teacher=True)
    definition = create_definition(lab, room); assert publish(lab, room, definition).status == 200
    aggregate, revision = handin(lab, room)
    grades = f"/v3/classes/{room['class']['id']}/grade-items/{definition['grade_item_id']}/grades"
    assert mutate(lab, room["pairs"]["teacher"], grades, {"aggregate_id": aggregate["id"], "submission_revision_id": revision["id"], "expected_version": 0, "value_milli": 80000}).status == 200
    if release:
        assert mutate(lab, room["pairs"]["teacher"], grades+f"/{aggregate['id']}/release", {"expected_version": 1}).status == 200
    return room, definition, aggregate


def historical_row(room, definition, aggregate, source_row_id="match"):
    return {"source_table": "submissions", "source_row_id": source_row_id, "source_scope": "CLASS", "raw_state": "VALUE",
            "raw_value_text": "80", "raw_score_milli": 80000, "observed_score_milli": 80000, "origin": "RECORDED",
            "candidate_class_id": room["class"]["id"], "candidate_grade_item_id": definition["grade_item_id"], "candidate_aggregate_id": aggregate["id"]}


def historical_payload(rows, marker):
    import hashlib
    return {"source_snapshot_sha256": hashlib.sha256(marker.encode()).hexdigest(), "policy_reference": "SYNTHETIC-M09-CP04-V1", "rows": rows}


@pytest.mark.case("M09-CP04-historical-semantics")
def test_historical_raw_observed_semantics_are_preserved(lab):
    room, definition, aggregate = historical_room(lab, "M09 historical semantics")
    mapped = {"candidate_class_id": room["class"]["id"], "candidate_grade_item_id": definition["grade_item_id"], "candidate_aggregate_id": aggregate["id"]}
    rows = [
        {"source_table": "transcript_projection", "source_row_id": "missing", "source_scope": "CLASS", "raw_state": "MISSING_ROW", "observed_score_milli": 65000, "origin": "LEGACY_MINIMUM_65", **mapped},
        {"source_table": "transcript_projection", "source_row_id": "null", "source_scope": "CLASS", "raw_state": "EXPLICIT_NULL", "raw_value_text": "null", "observed_score_milli": 65000, "origin": "LEGACY_MINIMUM_65", **mapped},
        {"source_table": "submissions", "source_row_id": "zero", "source_scope": "CLASS", "raw_state": "VALUE", "raw_value_text": "0", "raw_score_milli": 0, "observed_score_milli": 0, "origin": "RECORDED", **mapped},
        {"source_table": "ai_evaluation", "source_row_id": "fallback75", "source_scope": "CLASS", "raw_state": "INVALID", "raw_value_text": "provider_error", "observed_score_milli": 75000, "origin": "AI_FALLBACK_75", **mapped},
        {"source_table": "competency_scores", "source_row_id": "global", "source_scope": "PROGRAM_GLOBAL", "raw_state": "VALUE", "raw_value_text": "90", "raw_score_milli": 90000, "observed_score_milli": 90000, "origin": "RECORDED"},
    ]
    response = mutate(lab, room["pairs"]["admin"], "/v3/grade-reconciliations", historical_payload(rows, "semantics"))
    assert response.status == 200
    observations = response.json()["observations"]
    assert [item["outcome"] for item in observations] == ["POLICY_DERIVED_HOLD", "POLICY_DERIVED_HOLD", "DIFFERENCE_HOLD", "AI_UNREVIEWED_HOLD", "AMBIGUOUS_HOLD"]
    assert observations[2]["raw_score_milli"] == 0 and observations[0]["raw_state"] == "MISSING_ROW" and observations[1]["raw_state"] == "EXPLICIT_NULL"


@pytest.mark.case("M09-CP04-scoped-match")
def test_historical_exact_match_approval_and_unreleased_hold(lab):
    room, definition, aggregate = historical_room(lab, "M09 historical match")
    match = historical_row(room, definition, aggregate)
    difference = {**historical_row(room, definition, aggregate, "approved"), "raw_value_text": "70", "raw_score_milli": 70000, "observed_score_milli": 70000,
                  "resolution": "APPROVED_DIFFERENCE", "reason": "Academic review accepted preserved legacy outcome"}
    payload = historical_payload([match, difference], "match")
    key = uuid.uuid4().hex
    first = mutate(lab, room["pairs"]["admin"], "/v3/grade-reconciliations", payload, key=key)
    replay = mutate(lab, room["pairs"]["admin"], "/v3/grade-reconciliations", payload, key=key)
    assert first.status == replay.status == 200 and replay.headers["idempotency-replayed"] == "true"
    assert [item["outcome"] for item in first.json()["observations"]] == ["MATCHED", "APPROVED_DIFFERENCE"]
    assert first.json()["observations"][0]["canonical_score_milli"] == 80000
    pending_room, pending_definition, pending_aggregate = historical_room(lab, "M09 historical unreleased", release=False)
    pending = mutate(lab, pending_room["pairs"]["admin"], "/v3/grade-reconciliations", historical_payload([historical_row(pending_room, pending_definition, pending_aggregate, "pending")], "pending"))
    assert pending.status == 200 and pending.json()["observations"][0]["outcome"] == "NO_CANONICAL_HOLD"


@pytest.mark.case("M09-CP04-security-boundary")
def test_historical_admin_ancestry_approval_and_atomic_boundary(lab):
    room, definition, aggregate = historical_room(lab, "M09 historical boundary")
    valid = historical_row(room, definition, aggregate)
    path = "/v3/grade-reconciliations"
    assert mutate(lab, room["pairs"]["teacher"], path, historical_payload([valid], "teacher")).status == 403
    assert mutate(lab, room["pairs"]["student"], path, historical_payload([valid], "student")).status == 403
    assert mutate(lab, room["pairs"]["admin"], path, historical_payload([valid, valid], "duplicate")).status == 400
    global_approval = {"source_table": "competency_scores", "source_row_id": "global-approval", "source_scope": "PROGRAM_GLOBAL", "raw_state": "VALUE", "raw_value_text": "90", "raw_score_milli": 90000, "observed_score_milli": 90000, "origin": "RECORDED", "resolution": "APPROVED_DIFFERENCE", "reason": "Forged approval for ambiguous global row"}
    assert mutate(lab, room["pairs"]["admin"], path, historical_payload([global_approval], "global")).status == 400
    foreign = {**historical_row(room, definition, aggregate, "foreign"), "candidate_aggregate_id": "missing-aggregate"}
    key = uuid.uuid4().hex
    rejected = mutate(lab, room["pairs"]["admin"], path, historical_payload([valid, foreign], "foreign"), key=key)
    assert rejected.status == 404
    accepted = mutate(lab, room["pairs"]["admin"], path, historical_payload([valid], "valid-after-rollback"), key=key)
    assert accepted.status == 200
    changed = mutate(lab, room["pairs"]["admin"], path, historical_payload([{**valid, "observed_score_milli": 79000}], "changed"), key=key)
    assert changed.status == 409
