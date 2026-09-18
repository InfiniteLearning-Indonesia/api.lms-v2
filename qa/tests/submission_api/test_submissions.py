from datetime import datetime, timedelta, timezone
import uuid

import pytest


def mutate(lab, pair, path, body, method="post", key=None):
    actor, csrf = pair
    return lab["mutate"](actor, csrf, path, body, method=method, key=key or uuid.uuid4().hex)


def classroom(lab, name, attempts=2, available_hours=-1, due_hours=1, cutoff_hours=2):
    pairs = {subject: lab["login"](subject) for subject in ("admin", "teacher", "student", "outsider")}
    created = mutate(lab, pairs["admin"], "/v3/classes", {"name": name})
    assert created.status == 200
    cls = created.json()
    enrollments = {}
    for subject, role in (("teacher", "teacher"), ("student", "student")):
        response = mutate(lab, pairs["admin"], f"/v3/classes/{cls['id']}/participants", {
            "user_id": lab["actors"][subject]["id"], "role": role,
        })
        assert response.status == 200
        enrollments[subject] = response.json()
    assert mutate(lab, pairs["admin"], f"/v3/classes/{cls['id']}/publish", {"version": 1}).status == 200
    section = mutate(lab, pairs["teacher"], f"/v3/classes/{cls['id']}/sections", {"title": "Work", "position": 1}).json()
    activity = mutate(lab, pairs["teacher"], f"/v3/classes/{cls['id']}/activities", {
        "section_id": section["id"], "activity_type": "ASSIGNMENT", "title": "Assignment", "position": 1,
    }).json()
    now = datetime.now(timezone.utc)
    revision = mutate(lab, pairs["teacher"], f"/v3/classes/{cls['id']}/activities/{activity['id']}/revisions", {
        "raw_body": "Do the work", "timezone": "Asia/Jakarta",
        "available_from": (now+timedelta(hours=available_hours)).isoformat().replace("+00:00", "Z"),
        "due_at": (now+timedelta(hours=due_hours)).isoformat().replace("+00:00", "Z"),
        "cutoff_at": (now+timedelta(hours=cutoff_hours)).isoformat().replace("+00:00", "Z"),
        "assignment": {"submission_mode": "TEXT_AND_FILES", "max_attempts": attempts, "max_files": 2,
                       "max_file_bytes": 2 << 20, "late_policy": "REJECT_AFTER_CUTOFF"},
    }).json()
    revision = mutate(lab, pairs["teacher"], f"/v3/classes/{cls['id']}/activities/{activity['id']}/revisions/{revision['id']}/publish", {"version": revision["version"]}).json()
    return {"class": cls, "activity": activity, "revision": revision, "pairs": pairs, "enrollments": enrollments}


def create_submission(lab, room, key=None):
    path = f"/v3/classes/{room['class']['id']}/activities/{room['activity']['id']}/revisions/{room['revision']['id']}/submissions"
    return mutate(lab, room["pairs"]["student"], path, {}, key=key)


@pytest.mark.case("M08-CP01-server-derived-context")
def test_submission_context_is_server_derived_and_idempotent(lab):
    room = classroom(lab, "M08 CP01 owner")
    key = uuid.uuid4().hex
    first, replay = create_submission(lab, room, key), create_submission(lab, room, key)
    assert first.status == 200 and replay.status == 200 and replay.headers["idempotency-replayed"] == "true"
    assert first.json() == replay.json()
    value = first.json()
    assert value["owner_user_id"] == lab["actors"]["student"]["id"]
    assert value["enrollment_id"] == room["enrollments"]["student"]["enrollment_id"]
    assert value["episode_id"] and value["attempts"][0]["attempt_no"] == 1


@pytest.mark.case("M08-CP01-cross-context-denial")
def test_foreign_revision_outsider_and_ended_participation_are_denied(lab):
    room = classroom(lab, "M08 CP01 context")
    foreign = classroom(lab, "M08 CP01 foreign")
    path = f"/v3/classes/{room['class']['id']}/activities/{room['activity']['id']}/revisions/{foreign['revision']['id']}/submissions"
    assert mutate(lab, room["pairs"]["student"], path, {}).status == 404
    own_path = f"/v3/classes/{room['class']['id']}/activities/{room['activity']['id']}/revisions/{room['revision']['id']}/submissions"
    assert mutate(lab, room["pairs"]["outsider"], own_path, {}).status == 403
    ended = mutate(lab, room["pairs"]["admin"], f"/v3/classes/{room['class']['id']}/participants/{lab['actors']['student']['id']}/end", {})
    assert ended.status == 200
    assert create_submission(lab, room).status == 403


def upload_evidence(lab, room, data=b"first evidence\n"):
    student, csrf = room["pairs"]["student"]
    return student.post(f"/v3/classes/{room['class']['id']}/files", data=data, headers={
        "Content-Type": "text/plain", "X-CSRF-Token": csrf, "Idempotency-Key": uuid.uuid4().hex,
        "X-File-Owner": lab["actors"]["student"]["id"], "X-File-Purpose": "EVIDENCE", "X-File-Name": "work.txt",
    })


@pytest.mark.case("M08-CP02-retained-revisions-files")
def test_append_preserves_old_revision_and_attachment(lab):
    room = classroom(lab, "M08 CP02 revisions")
    aggregate = create_submission(lab, room).json()
    asset = upload_evidence(lab, room).json()
    base = f"/v3/classes/{room['class']['id']}/submissions/{aggregate['id']}"
    first = mutate(lab, room["pairs"]["student"], base+"/revisions", {"version": aggregate["version"], "body": "first", "file_asset_ids": [asset["id"]]})
    second = mutate(lab, room["pairs"]["student"], base+"/revisions", {"version": aggregate["version"], "body": "second", "file_asset_ids": []})
    assert first.status == 200 and second.status == 200 and first.json()["id"] != second.json()["id"]
    history = room["pairs"]["student"][0].get(base).json()
    revisions = history["attempts"][0]["revisions"]
    assert [item["body"] for item in revisions] == ["first", "second"]
    assert revisions[0]["files"][0]["asset_id"] == asset["id"] and revisions[1]["files"] == []


@pytest.mark.case("M08-CP02-lock-return-resubmit")
def test_transition_authority_and_resubmit_retains_attempt(lab):
    room = classroom(lab, "M08 CP02 lifecycle")
    aggregate = create_submission(lab, room).json()
    base = f"/v3/classes/{room['class']['id']}/submissions/{aggregate['id']}"
    assert mutate(lab, room["pairs"]["student"], base+"/revisions", {"version": 1, "body": "attempt one", "file_asset_ids": []}).status == 200
    assert mutate(lab, room["pairs"]["outsider"], base+"/return", {"attempt_version": 1}).status == 403
    locked = mutate(lab, room["pairs"]["student"], base+"/lock", {"attempt_version": 1})
    assert locked.status == 200 and locked.json()["state"] == "LOCKED"
    assert mutate(lab, room["pairs"]["student"], base+"/revisions", {"version": 1, "body": "overwrite", "file_asset_ids": []}).status == 409
    returned = mutate(lab, room["pairs"]["teacher"], base+"/return", {"attempt_version": locked.json()["version"]})
    assert returned.status == 200 and returned.json()["state"] == "RETURNED"
    resubmitted = mutate(lab, room["pairs"]["student"], base+"/resubmit", {"version": 1, "attempt_version": returned.json()["version"]})
    assert resubmitted.status == 200 and resubmitted.json()["current_attempt_no"] == 2
    history = room["pairs"]["teacher"][0].get(base).json()
    assert len(history["attempts"]) == 2 and history["attempts"][0]["state"] == "RETURNED" and history["attempts"][1]["state"] == "DRAFT"


@pytest.mark.case("M08-CP02-optimistic-conflict")
def test_stale_versions_and_foreign_assets_fail_closed(lab):
    room = classroom(lab, "M08 CP02 conflict")
    other = classroom(lab, "M08 CP02 other")
    aggregate = create_submission(lab, room).json()
    foreign_asset = upload_evidence(lab, other).json()
    base = f"/v3/classes/{room['class']['id']}/submissions/{aggregate['id']}"
    assert mutate(lab, room["pairs"]["student"], base+"/revisions", {"version": 1, "body": "foreign", "file_asset_ids": [foreign_asset["id"]]}).status == 403
    locked = mutate(lab, room["pairs"]["student"], base+"/lock", {"attempt_version": 1}).json()
    assert mutate(lab, room["pairs"]["teacher"], base+"/return", {"attempt_version": 1}).status == 409
    returned = mutate(lab, room["pairs"]["teacher"], base+"/return", {"attempt_version": locked["version"]}).json()
    assert mutate(lab, room["pairs"]["student"], base+"/resubmit", {"version": 999, "attempt_version": returned["version"]}).status == 409


def prepare_work(lab, room, text="final work"):
    aggregate = create_submission(lab, room).json()
    base = f"/v3/classes/{room['class']['id']}/submissions/{aggregate['id']}"
    revision = mutate(lab, room["pairs"]["student"], base + "/revisions", {
        "version": aggregate["version"], "body": text, "file_asset_ids": [],
    })
    assert revision.status == 200
    return aggregate, base, revision.json()


def handin(lab, room, base, version=1, attempt_version=1, key=None):
    return mutate(lab, room["pairs"]["student"], base + "/handin", {
        "version": version, "attempt_version": attempt_version,
    }, key=key)


@pytest.mark.case("M08-CP03-durable-idempotent-receipt")
def test_durable_receipt_replay_is_exact_and_server_derived(lab):
    room = classroom(lab, "M08 CP03 receipt")
    aggregate, base, revision = prepare_work(lab, room)
    key = uuid.uuid4().hex
    first, replay = handin(lab, room, base, key=key), handin(lab, room, base, key=key)
    assert first.status == 200 and replay.status == 200
    assert replay.headers["idempotency-replayed"] == "true" and first.json() == replay.json()
    receipt = first.json()
    assert receipt["revision_id"] == revision["id"] and receipt["submitted_by"] == lab["actors"]["student"]["id"]
    assert receipt["timezone"] == "Asia/Jakarta" and receipt["submitted_local"].endswith("+07:00")
    assert receipt["late"] is False and receipt["attempt_id"] == aggregate["attempts"][0]["id"]
    history = room["pairs"]["student"][0].get(base).json()
    assert history["attempts"][0]["receipt"]["id"] == receipt["id"]


@pytest.mark.case("M08-CP03-deadline-policy")
def test_due_marks_late_and_cutoff_rejects_atomically(lab):
    late = classroom(lab, "M08 CP03 late", due_hours=-1, cutoff_hours=1)
    _, base, _ = prepare_work(lab, late)
    response = handin(lab, late, base)
    assert response.status == 200 and response.json()["late"] is True
    closed = classroom(lab, "M08 CP03 cutoff", available_hours=-3, due_hours=-2, cutoff_hours=-1)
    _, base, _ = prepare_work(lab, closed)
    response = handin(lab, closed, base)
    assert response.status == 409 and response.json()["error"] == "DEADLINE_PASSED"
    history = closed["pairs"]["student"][0].get(base).json()
    assert history["attempts"][0]["state"] == "DRAFT" and "receipt" not in history["attempts"][0]


@pytest.mark.case("M08-CP03-concurrent-conflict")
def test_conflicting_retry_and_second_key_cannot_duplicate_receipt(lab):
    room = classroom(lab, "M08 CP03 conflict")
    _, base, _ = prepare_work(lab, room)
    key = uuid.uuid4().hex
    assert handin(lab, room, base, key=key).status == 200
    assert handin(lab, room, base, attempt_version=2, key=key).status == 409
    assert handin(lab, room, base).status == 409
    history = room["pairs"]["teacher"][0].get(base).json()
    assert len([a for a in history["attempts"] if a.get("receipt")]) == 1
    draft_room = classroom(lab, "M08 CP03 lock is not handin")
    _, draft_base, _ = prepare_work(lab, draft_room)
    locked = mutate(lab, draft_room["pairs"]["student"], draft_base + "/lock", {"attempt_version": 1})
    assert locked.status == 200
    assert mutate(lab, draft_room["pairs"]["teacher"], draft_base + "/return", {"attempt_version": 2}).status == 409
    assert handin(lab, draft_room, draft_base, attempt_version=2).status == 200
    assert mutate(lab, draft_room["pairs"]["teacher"], draft_base + "/return", {"attempt_version": 2}).status == 200


@pytest.mark.case("M08-CP04-vertical-journey")
def test_create_content_handin_return_resubmit_has_two_receipts(lab):
    room = classroom(lab, "M08 CP04 vertical")
    aggregate, base, first_revision = prepare_work(lab, room, "attempt one")
    first = handin(lab, room, base).json()
    returned = mutate(lab, room["pairs"]["teacher"], base + "/return", {"attempt_version": 2})
    assert returned.status == 200
    resubmitted = mutate(lab, room["pairs"]["student"], base + "/resubmit", {
        "version": aggregate["version"], "attempt_version": returned.json()["version"],
    })
    assert resubmitted.status == 200 and resubmitted.json()["current_attempt_no"] == 2
    second_revision = mutate(lab, room["pairs"]["student"], base + "/revisions", {
        "version": resubmitted.json()["version"], "body": "attempt two", "file_asset_ids": [],
    })
    assert second_revision.status == 200
    second = handin(lab, room, base, version=resubmitted.json()["version"]).json()
    history = room["pairs"]["teacher"][0].get(base).json()
    assert [a["receipt"]["revision_id"] for a in history["attempts"]] == [first_revision["id"], second_revision.json()["id"]]
    assert [a["receipt"]["id"] for a in history["attempts"]] == [first["id"], second["id"]]


@pytest.mark.case("M08-CP04-receipt-history-authority")
def test_owner_history_remains_readable_after_participation_end(lab):
    room = classroom(lab, "M08 CP04 history")
    _, base, revision = prepare_work(lab, room)
    receipt = handin(lab, room, base).json()
    ended = mutate(lab, room["pairs"]["admin"],
                   f"/v3/classes/{room['class']['id']}/participants/{lab['actors']['student']['id']}/end", {})
    assert ended.status == 200
    history = room["pairs"]["student"][0].get(base)
    assert history.status == 200 and history.json()["attempts"][0]["receipt"]["id"] == receipt["id"]
    assert history.json()["attempts"][0]["revisions"][0]["id"] == revision["id"]
    assert room["pairs"]["outsider"][0].get(base).status == 403
