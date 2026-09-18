import time
import uuid

import pytest


def classroom(lab, name):
    admin, admin_csrf = lab["login"]("admin")
    teacher, teacher_csrf = lab["login"]("teacher")
    student, student_csrf = lab["login"]("student")
    student2, student2_csrf = lab["login"]("student2")
    outsider, outsider_csrf = lab["login"]("outsider")
    created = lab["mutate"](admin, admin_csrf, "/v3/classes", {"name": name})
    assert created.status == 200
    cls = created.json()
    for subject, role in (("teacher", "teacher"), ("student", "student"), ("student2", "student")):
        response = lab["mutate"](admin, admin_csrf, f"/v3/classes/{cls['id']}/participants", {
            "user_id": lab["actors"][subject]["id"], "role": role,
        })
        assert response.status == 200
    published = lab["mutate"](admin, admin_csrf, f"/v3/classes/{cls['id']}/publish", {"version": 1})
    assert published.status == 200
    return {
        "class": cls, "admin": (admin, admin_csrf), "teacher": (teacher, teacher_csrf),
        "student": (student, student_csrf), "student2": (student2, student2_csrf),
        "outsider": (outsider, outsider_csrf),
    }


def upload(lab, actor, csrf, cls, owner, purpose, name, media_type, data, key=None):
    return actor.post(f"/v3/classes/{cls['id']}/files", data=data, headers={
        "Content-Type": media_type,
        "X-CSRF-Token": csrf,
        "Idempotency-Key": key or uuid.uuid4().hex,
        "X-File-Owner": owner,
        "X-File-Purpose": purpose,
        "X-File-Name": name,
    })


@pytest.mark.case("M06-CP01-private-asset")
def test_private_asset_hides_object_key(lab):
    room = classroom(lab, "M06 private asset")
    teacher, csrf = room["teacher"]
    response = upload(lab, teacher, csrf, room["class"], lab["actors"]["teacher"]["id"],
                      "MATERIAL", "lesson.txt", "text/plain", b"private lesson\n")
    assert response.status == 201
    asset = response.json()
    assert asset["state"] == "AVAILABLE" and "object_key" not in asset
    student, _ = room["student"]
    metadata = student.get(f"/v3/files/{asset['id']}")
    assert metadata.status == 200 and metadata.json()["sha256"] == asset["sha256"]


@pytest.mark.case("M06-CP01-class-ownership")
def test_evidence_is_owner_and_class_scoped(lab):
    room = classroom(lab, "M06 ownership")
    student, csrf = room["student"]
    asset = upload(lab, student, csrf, room["class"], lab["actors"]["student"]["id"],
                   "EVIDENCE", "proof.txt", "text/plain", b"student proof\n").json()
    student2, _ = room["student2"]
    outsider, _ = room["outsider"]
    assert student2.get(f"/v3/files/{asset['id']}").status == 403
    assert outsider.get(f"/v3/files/{asset['id']}").status == 403
    teacher, _ = room["teacher"]
    assert teacher.get(f"/v3/files/{asset['id']}").status == 200


@pytest.mark.case("M06-CP01-idempotency-authority")
def test_upload_retry_and_owner_authority(lab):
    room = classroom(lab, "M06 idempotency")
    student, csrf = room["student"]
    key = uuid.uuid4().hex
    args = (lab, student, csrf, room["class"], lab["actors"]["student"]["id"],
            "EVIDENCE", "retry.txt", "text/plain", b"same bytes\n")
    first = upload(*args, key=key)
    second = upload(*args, key=key)
    assert first.status == 201 and second.status == 200 and first.json() == second.json()
    foreign = upload(lab, student, csrf, room["class"], lab["actors"]["student2"]["id"],
                     "EVIDENCE", "foreign.txt", "text/plain", b"foreign owner\n")
    assert foreign.status == 403


@pytest.mark.case("M06-CP02-upload-validation")
def test_upload_magic_mime_size_and_name_validation(lab):
    room = classroom(lab, "M06 validation")
    teacher, csrf = room["teacher"]
    owner = lab["actors"]["teacher"]["id"]
    for name, media, data in (
        ("spoof.pdf", "application/pdf", b"plain text"),
        ("active.txt", "text/plain", b"<html><script>alert(1)</script></html>"),
        ("../escape.txt", "text/plain", b"safe text"),
        ("large.txt", "text/plain", b"x" * ((2 << 20) + 1)),
    ):
        assert upload(lab, teacher, csrf, room["class"], owner, "MATERIAL", name, media, data).status == 400


@pytest.mark.case("M06-CP02-quarantine")
def test_scanner_unavailable_and_threat_fail_closed(lab):
    room = classroom(lab, "M06 quarantine")
    teacher, csrf = room["teacher"]
    owner = lab["actors"]["teacher"]["id"]
    unavailable = upload(lab, teacher, csrf, room["class"], owner, "MATERIAL", "pending.txt",
                         "text/plain", b"SCAN_UNAVAILABLE local fixture")
    threat = upload(lab, teacher, csrf, room["class"], owner, "MATERIAL", "threat.txt", "text/plain",
                    b"X5O!P%@AP[4\\PZX54(P^)7CC)7}$EICAR")
    assert unavailable.status == 202 and unavailable.json()["state"] == "QUARANTINED"
    assert threat.status == 202 and threat.json()["state"] == "REJECTED"
    for asset in (unavailable.json(), threat.json()):
        grant = lab["mutate"](teacher, csrf, f"/v3/files/{asset['id']}/download-grants", {})
        assert grant.status == 409


@pytest.mark.case("M06-CP02-download-authority")
def test_download_grant_is_target_bound(lab):
    room = classroom(lab, "M06 download authority")
    student, csrf = room["student"]
    asset = upload(lab, student, csrf, room["class"], lab["actors"]["student"]["id"],
                   "EVIDENCE", "private.txt", "text/plain", b"private evidence\n").json()
    grant = lab["mutate"](student, csrf, f"/v3/files/{asset['id']}/download-grants", {})
    assert grant.status == 201
    student2, _ = room["student2"]
    assert student2.get(f"/v3/file-downloads/{grant.json()['token']}").status == 403
    delivered = student.get(f"/v3/file-downloads/{grant.json()['token']}")
    assert delivered.status == 200 and delivered.body() == b"private evidence\n"
    assert delivered.headers["cache-control"] == "no-store"
    assert delivered.headers["x-content-type-options"] == "nosniff"
    assert delivered.headers["content-disposition"].startswith("attachment")


@pytest.mark.case("M06-CP02-download-expiry-retry")
def test_download_grant_retries_then_expires(lab):
    room = classroom(lab, "M06 grant expiry")
    teacher, csrf = room["teacher"]
    asset = upload(lab, teacher, csrf, room["class"], lab["actors"]["teacher"]["id"],
                   "MATERIAL", "retry.txt", "text/plain", b"download retry\n").json()
    grant = lab["mutate"](teacher, csrf, f"/v3/files/{asset['id']}/download-grants", {}).json()
    path = f"/v3/file-downloads/{grant['token']}"
    assert teacher.get(path).status == 200
    assert teacher.get(path).status == 200
    time.sleep(2.2)
    assert teacher.get(path).status == 404
