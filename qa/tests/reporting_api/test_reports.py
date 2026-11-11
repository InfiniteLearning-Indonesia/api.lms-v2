import json
from pathlib import Path

import pytest

from qa.tests.gradebook_api.test_gradebook import classroom, create_definition, publish, handin, mutate

CONTRACT = Path(__file__).resolve().parents[3] / "api" / "openapi" / "reporting-v1.json"

def room_with_grade(lab, title):
    room = classroom(lab, title, second_teacher=True)
    definition = create_definition(lab, room)
    assert publish(lab, room, definition).status == 200
    aggregate, revision = handin(lab, room)
    grade_path = f"/v3/classes/{room['class']['id']}/grade-items/{definition['grade_item_id']}/grades"
    report_path = f"/v3/classes/{room['class']['id']}/reports/grades/{lab['actors']['student']['id']}"
    return room, definition, aggregate, revision, grade_path, report_path

@pytest.mark.case("M14-CP01-release-read-your-writes")
def test_release_read_your_writes(lab):
    room, definition, aggregate, revision, grades, report = room_with_grade(lab, "M14 report release")
    student = room["pairs"]["student"][0]
    empty = student.get(report)
    assert empty.status == 200 and empty.json()["grades"] == [] and empty.json()["watermark"] is None
    body = {"aggregate_id": aggregate["id"], "submission_revision_id": revision["id"], "expected_version": 0, "value_milli": 70000}
    assert mutate(lab, room["pairs"]["teacher"], grades, body).status == 200
    assert student.get(report).json()["grades"] == []
    first = mutate(lab, room["pairs"]["teacher"], grades+f"/{aggregate['id']}/release", {"expected_version": 1})
    assert first.status == 200
    visible = student.get(report)
    assert visible.status == 200 and visible.json()["grades"][0]["value_milli"] == 70000
    assert visible.json()["grades"][0]["grade_version"] == 1 and visible.json()["watermark"]["release_id"] == first.json()["id"]
    assert mutate(lab, room["pairs"]["teacher2"], grades, {**body, "expected_version": 1, "value_milli": 82000, "reason": "Moderated evidence"}).status == 200
    assert student.get(report).json()["grades"][0]["value_milli"] == 70000
    second = mutate(lab, room["pairs"]["teacher2"], grades+f"/{aggregate['id']}/release", {"expected_version": 2})
    assert second.status == 200
    latest = student.get(report).json()
    assert latest["grades"][0]["value_milli"] == 82000 and latest["grades"][0]["grade_version"] == 2
    assert latest["watermark"]["release_id"] == second.json()["id"]

@pytest.mark.case("M14-CP01-actor-boundary")
def test_actor_and_class_boundary(lab):
    room, _, _, _, _, report = room_with_grade(lab, "M14 report boundary")
    assert room["pairs"]["teacher"][0].get(report).status == 200
    assert room["pairs"]["teacher2"][0].get(report).status == 200
    assert room["pairs"]["admin"][0].get(report).status == 200
    assert room["pairs"]["outsider"][0].get(report).status == 403
    other = classroom(lab, "M14 foreign class")
    foreign = f"/v3/classes/{other['class']['id']}/reports/grades/{lab['actors']['student']['id']}"
    assert room["pairs"]["teacher2"][0].get(foreign).status in (403, 404)
    target = f"/v3/classes/{room['class']['id']}/reports/grades/{lab['actors']['outsider']['id']}"
    assert room["pairs"]["teacher"][0].get(target).status == 404

@pytest.mark.case("M14-CP01-minimal-fields")
def test_only_released_minimal_fields(lab):
    room, _, _, _, _, report = room_with_grade(lab, "M14 minimal fields")
    payload = room["pairs"]["student"][0].get(report).json()
    assert set(payload) == {"contract_version", "class_id", "learner_user_id", "watermark", "grades"}
    assert payload["contract_version"] == 1
    assert "email" not in json.dumps(payload).lower() and "submission" not in json.dumps(payload).lower()

@pytest.mark.case("M14-CP02-contract-response")
def test_contract_matches_response(lab):
    contract = json.loads(CONTRACT.read_text(encoding="utf-8"))
    operation = contract["paths"]["/v3/classes/{class_id}/reports/grades/{learner_user_id}"]["get"]
    assert operation["responses"]["200"]["content"]["application/json"]["schema"]["$ref"] == "#/components/schemas/GradeReport"
    room, definition, aggregate, revision, grades, report = room_with_grade(lab, "M14 contract")
    assert mutate(lab, room["pairs"]["teacher"], grades, {"aggregate_id":aggregate["id"],"submission_revision_id":revision["id"],"expected_version":0,"value_milli":65000}).status == 200
    assert mutate(lab, room["pairs"]["teacher"], grades+f"/{aggregate['id']}/release", {"expected_version":1}).status == 200
    response = room["pairs"]["student"][0].get(report)
    assert response.status == 200 and response.headers["cache-control"] == "no-store"
    schema = contract["components"]["schemas"]["GradeReport"]
    payload = response.json()
    assert set(payload) == set(schema["required"]) == set(schema["properties"])
    assert payload["contract_version"] == schema["properties"]["contract_version"]["const"]
    assert set(payload["watermark"]) == set(contract["components"]["schemas"]["Watermark"]["required"])
    assert len(payload["grades"]) == 1
    grade = payload["grades"][0]
    assert set(grade) == set(contract["components"]["schemas"]["ReleasedGrade"]["required"])
    assert grade["grade_item_id"] == definition["grade_item_id"] and isinstance(grade["value_milli"], int)

@pytest.mark.case("M14-CP02-contract-errors")
def test_invalid_query_and_unknown_route(lab):
    room, _, _, _, _, report = room_with_grade(lab, "M14 contract errors")
    client = room["pairs"]["student"][0]
    invalid = client.get(report+"?learner_user_id=anything")
    missing = client.get(report+"/export")
    malformed = client.get(report.rsplit("/", 1)[0]+"/not-an-id")
    # The shared identity gateway rejects all query strings before routing.
    assert invalid.status == 401 and invalid.json() == {"error": "UNAUTHENTICATED"}
    assert missing.status == 404 and missing.json() == {"error": "NOT_FOUND"}
    assert malformed.status == 400 and malformed.json() == {"error": "INVALID_INPUT"}
    contract = json.loads(CONTRACT.read_text(encoding="utf-8"))
    responses = contract["paths"]["/v3/classes/{class_id}/reports/grades/{learner_user_id}"]["get"]["responses"]
    assert "400" in responses and "401" in responses and "404" in responses and "403" in responses
