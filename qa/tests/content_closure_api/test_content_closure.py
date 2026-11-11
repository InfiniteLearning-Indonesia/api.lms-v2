import uuid

import pytest

from qa.tests.gradebook_api.test_gradebook import classroom, handin, mutate
from qa.tests.content_contract_api.test_content_contract import revision, rule


def draft_class(lab, name, teachers=("teacher",)):
    admin = lab["login"]("admin")
    created = mutate(lab, admin, "/v3/classes", {"name": name})
    assert created.status == 200
    cls = created.json()
    for teacher in teachers:
        assigned = mutate(lab, admin, f"/v3/classes/{cls['id']}/participants", {
            "user_id": lab["actors"][teacher]["id"], "role": "teacher",
        })
        assert assigned.status == 200
    return cls, admin


@pytest.mark.case("M07-CP04-owner-withdrawn")
def test_submission_owner_reads_exact_withdrawn_revision_after_end(lab):
    room = classroom(lab, "M07 CP04 owner withdrawn", second_student=True)
    aggregate, _ = handin(lab, room)
    path = f"/v3/classes/{room['class']['id']}/activities/{room['activity']['id']}/revisions/{room['revision']['id']}"
    before = room["pairs"]["student"][0].get(path)
    assert before.status == 200
    withdrawn = mutate(lab, room["pairs"]["teacher"], path + "/withdraw", {"version": room["revision"]["version"]})
    assert withdrawn.status == 200
    ended = mutate(lab, room["pairs"]["teacher"], f"/v3/classes/{room['class']['id']}/participants/{lab['actors']['student']['id']}/end", {})
    assert ended.status == 200
    retained = room["pairs"]["student"][0].get(path)
    assert retained.status == 200 and retained.json()["revision"]["state"] == "WITHDRAWN"
    assert retained.json()["revision"]["id"] == aggregate["assignment_revision_id"]
    assert retained.json()["revision"]["canonical_body"] == before.json()["revision"]["canonical_body"]
    assert retained.json()["rule"] == before.json()["rule"]
    assert "raw_body" not in retained.text()


@pytest.mark.case("M07-CP04-owner-boundary")
def test_peer_foreign_and_draft_revision_are_not_historical_owners(lab):
    room = classroom(lab, "M07 CP04 owner boundary", second_student=True)
    handin(lab, room)
    base = f"/v3/classes/{room['class']['id']}/activities/{room['activity']['id']}/revisions"
    original = room["revision"]
    assert mutate(lab, room["pairs"]["teacher"], base + f"/{original['id']}/withdraw", {"version": original["version"]}).status == 200
    assert room["pairs"]["outsider"][0].get(base + f"/{original['id']}").status == 404
    draft = mutate(lab, room["pairs"]["teacher"], base, revision("private draft", rule())).json()
    assert room["pairs"]["student"][0].get(base + f"/{draft['id']}").status == 404
    foreign, _ = draft_class(lab, "M07 CP04 foreign")
    assert room["pairs"]["student"][0].get(f"/v3/classes/{foreign['id']}/activities/{room['activity']['id']}/revisions/{original['id']}").status == 403
    assert room["pairs"]["student"][0].get(base + f"/{uuid.uuid4().hex}").status == 404


@pytest.mark.case("M07-CP04-structure-clone")
def test_teacher_clones_only_structure_with_stable_replay(lab):
    source = classroom(lab, "M07 CP04 clone source")
    target, _ = draft_class(lab, "M07 CP04 clone target")
    path = f"/v3/classes/{target['id']}/content/clone"
    key = uuid.uuid4().hex
    first = mutate(lab, source["pairs"]["teacher"], path, {"source_class_id": source["class"]["id"]}, key=key)
    replay = mutate(lab, source["pairs"]["teacher"], path, {"source_class_id": source["class"]["id"]}, key=key)
    assert first.status == replay.status == 200 and first.json() == replay.json()
    mapping = first.json()
    assert mapping["source_class_id"] == source["class"]["id"] and mapping["target_class_id"] == target["id"]
    assert len(mapping["sections"]) == len(mapping["activities"]) == 1
    assert mapping["sections"][0]["source_id"] != mapping["sections"][0]["target_id"]
    view = source["pairs"]["teacher"][0].get(f"/v3/classes/{target['id']}/content")
    assert view.status == 200 and len(view.json()["sections"]) == len(view.json()["activities"]) == 1
    assert view.json()["revisions"] == [] and view.json()["assignment_rules"] == []
    source_view = source["pairs"]["teacher"][0].get(f"/v3/classes/{source['class']['id']}/content")
    assert source_view.status == 200 and source_view.json()["revisions"]


@pytest.mark.case("M07-CP04-clone-boundary")
def test_clone_denies_foreign_actor_nonempty_target_and_oversized_source(lab):
    source = classroom(lab, "M07 CP04 boundary source")
    target, admin = draft_class(lab, "M07 CP04 boundary target", teachers=("teacher", "teacher2"))
    path = f"/v3/classes/{target['id']}/content/clone"
    assert mutate(lab, source["pairs"]["teacher2"], path, {"source_class_id": source["class"]["id"]}).status == 403
    assert mutate(lab, admin, path, {"source_class_id": target["id"]}).status == 400
    assert mutate(lab, admin, path, {"source_class_id": source["class"]["id"], "copy_learners": True}).status == 400
    assert mutate(lab, admin, path, {"source_class_id": source["class"]["id"]}).status == 200
    assert mutate(lab, admin, path, {"source_class_id": source["class"]["id"]}).status == 409
    large = classroom(lab, "M07 CP04 bounded source")
    large_target, _ = draft_class(lab, "M07 CP04 bounded target")
    for index in range(2, 52):
        added = mutate(lab, large["pairs"]["teacher"], f"/v3/classes/{large['class']['id']}/sections", {"title": f"Week {index}", "position": index})
        assert added.status == 201
    denied = mutate(lab, admin, f"/v3/classes/{large_target['id']}/content/clone", {"source_class_id": large["class"]["id"]})
    assert denied.status == 409
    empty = admin[0].get(f"/v3/classes/{large_target['id']}/content")
    assert empty.status == 200 and empty.json()["sections"] == []
