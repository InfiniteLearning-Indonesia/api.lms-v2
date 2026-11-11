import pytest

from qa.tests.gradebook_api.test_gradebook import classroom, create_definition, handin, mutate, publish


def graded_room(lab, name):
    room = classroom(lab, name, second_teacher=True)
    definition = create_definition(lab, room)
    assert publish(lab, room, definition).status == 200
    aggregate, revision = handin(lab, room)
    base = f"/v3/classes/{room['class']['id']}"
    grades = base + f"/grade-items/{definition['grade_item_id']}/grades"
    assert mutate(lab, room["pairs"]["teacher2"], grades, {
        "aggregate_id": aggregate["id"], "submission_revision_id": revision["id"],
        "expected_version": 0, "value_milli": 81000, "reason": "Initial grade",
    }).status == 200
    assert mutate(lab, room["pairs"]["teacher"], grades + f"/{aggregate['id']}/release", {"expected_version": 1}).status == 200
    return room, definition, aggregate, revision, base, grades


@pytest.mark.case("M05-CP05-publish-authority")
def test_publish_and_second_teacher_authority(lab):
    pairs = {name: lab["login"](name) for name in ("admin", "teacher", "teacher2", "student", "outsider")}
    draft = mutate(lab, pairs["admin"], "/v3/classes", {"name": "M05 publish authority", "cohort_label": "free text"}).json()
    base = f"/v3/classes/{draft['id']}"
    assert mutate(lab, pairs["admin"], base + "/publish", {"version": 1}).status == 409
    for name, role in (("teacher", "teacher"), ("teacher2", "teacher"), ("student", "student")):
        assert mutate(lab, pairs["admin"], base + "/participants", {"user_id": lab["actors"][name]["id"], "role": role}).status == 200
    assert mutate(lab, pairs["admin"], base + "/publish", {"version": 1}).status == 200
    section = mutate(lab, pairs["teacher2"], base + "/sections", {"title": "Second teacher", "position": 1})
    assert section.status == 201
    assert mutate(lab, pairs["outsider"], base + "/sections", {"title": "Foreign", "position": 2}).status == 403
    room, definition, aggregate, revision, _, grades = graded_room(lab, "M05 second teacher grades")
    assert room["pairs"]["teacher2"][0].get(grades + f"/{aggregate['id']}").status == 200
    assert mutate(lab, room["pairs"]["outsider"], grades, {
        "aggregate_id": aggregate["id"], "submission_revision_id": revision["id"],
        "expected_version": 1, "value_milli": 90000, "reason": "foreign",
    }).status == 403


@pytest.mark.case("M05-CP05-end-retains-history")
def test_ended_learner_cannot_write_but_teacher_keeps_history(lab):
    room, definition, aggregate, revision, base, grades = graded_room(lab, "M05 ended history")
    student_id = lab["actors"]["student"]["id"]
    read_grade = grades + f"/{aggregate['id']}"
    assert room["pairs"]["student"][0].get(read_grade).status == 200
    ended = mutate(lab, room["pairs"]["teacher"], base + f"/participants/{student_id}/end", {})
    assert ended.status == 200
    assert mutate(lab, room["pairs"]["student"], base + f"/activities/{room['activity']['id']}/revisions/{room['revision']['id']}/submissions", {}).status == 403
    assert room["pairs"]["student"][0].get(read_grade).status == 403
    assert room["pairs"]["teacher2"][0].get(base + f"/submissions/{aggregate['id']}").status == 200
    retained = room["pairs"]["teacher"][0].get(read_grade)
    assert retained.status == 200 and retained.json()["value_milli"] == 81000
    archived = mutate(lab, room["pairs"]["admin"], base + "/archive", {"version": 2})
    assert archived.status == 200
    assert mutate(lab, room["pairs"]["student"], base + f"/activities/{room['activity']['id']}/revisions/{room['revision']['id']}/submissions", {}).status == 403
    assert room["pairs"]["teacher2"][0].get(base + f"/submissions/{aggregate['id']}").status == 200
    assert room["pairs"]["teacher"][0].get(read_grade).status == 403
    assert room["pairs"]["admin"][0].get(read_grade).status == 200


@pytest.mark.case("M05-CP05-retake-separate-history")
def test_retake_does_not_inherit_earlier_work_or_grade(lab):
    room, definition, old_aggregate, old_revision, base, grades = graded_room(lab, "M05 retake isolation")
    student_id = lab["actors"]["student"]["id"]
    assert mutate(lab, room["pairs"]["teacher"], base + f"/participants/{student_id}/end", {}).status == 200
    # The old episode is read through the immutable aggregate, not inferred from the current enrollment state.
    old = room["pairs"]["teacher"][0].get(base + f"/submissions/{old_aggregate['id']}").json()
    old_episode = old["episode_id"]
    retake = mutate(lab, room["pairs"]["teacher"], base + f"/participants/{student_id}/episodes", {
        "kind": "RETAKE", "origin_episode_id": old_episode, "recognition_policy": "SEPARATE_HISTORY",
    })
    assert retake.status == 200 and retake.json()["episode_id"] != old_episode
    fresh = mutate(lab, room["pairs"]["student"], base + f"/activities/{room['activity']['id']}/revisions/{room['revision']['id']}/submissions", {})
    assert fresh.status == 200 and fresh.json()["id"] != old_aggregate["id"]
    assert fresh.json()["episode_id"] == retake.json()["episode_id"]
    assert room["pairs"]["teacher"][0].get(grades + f"/{fresh.json()['id']}").status == 404
    assert room["pairs"]["teacher"][0].get(grades + f"/{old_aggregate['id']}").json()["value_milli"] == 81000
    assert room["pairs"]["teacher"][0].get(base + f"/submissions/{old_aggregate['id']}").json()["episode_id"] == old_episode
