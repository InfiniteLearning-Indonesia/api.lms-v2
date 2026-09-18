import uuid

import pytest


def create(lab, admin, csrf, name):
    response = lab["mutate"](admin, csrf, "/v3/classes", {"name": name})
    assert response.status == 200
    return response.json()


def add(lab, client, csrf, cls, subject, role):
    response = lab["mutate"](client, csrf, f"/v3/classes/{cls['id']}/participants",
                             {"user_id": lab["actors"][subject]["id"], "role": role})
    assert response.status == 200
    return response.json()


def classroom(lab, name="M05 API"):
    admin, admin_csrf = lab["login"]("admin")
    teacher, teacher_csrf = lab["login"]("teacher")
    cls = create(lab, admin, admin_csrf, name)
    add(lab, admin, admin_csrf, cls, "teacher", "teacher")
    return admin, admin_csrf, teacher, teacher_csrf, cls


@pytest.mark.case("M05-CP03-group-ownership")
def test_group_is_class_owned_and_optional(lab):
    admin, acsrf, teacher, tcsrf, cls = classroom(lab, "Groups")
    add(lab, admin, acsrf, cls, "student", "student")
    add(lab, admin, acsrf, cls, "student2", "student")
    group_a = lab["mutate"](teacher, tcsrf, f"/v3/classes/{cls['id']}/groups", {"name": "Alpha"})
    group_b = lab["mutate"](teacher, tcsrf, f"/v3/classes/{cls['id']}/groups", {"name": "Beta"})
    assert group_a.status == group_b.status == 200
    assert lab["mutate"](teacher, tcsrf, f"/v3/classes/{cls['id']}/groups", {"name": " alpha "}).status == 409
    for group in (group_a.json(), group_b.json()):
        assigned = lab["mutate"](teacher, tcsrf,
            f"/v3/classes/{cls['id']}/groups/{group['id']}/members",
            {"user_id": lab["actors"]["student"]["id"]})
        assert assigned.status == 200 and assigned.json()["group_id"] == group["id"]

    foreign = create(lab, admin, acsrf, "Foreign group")
    add(lab, admin, acsrf, foreign, "teacher", "teacher")
    foreign_group = lab["mutate"](teacher, tcsrf, f"/v3/classes/{foreign['id']}/groups", {"name": "Foreign"}).json()
    rejected = lab["mutate"](teacher, tcsrf,
        f"/v3/classes/{cls['id']}/groups/{foreign_group['id']}/members",
        {"user_id": lab["actors"]["student"]["id"]})
    assert rejected.status == 403


@pytest.mark.case("M05-CP03-mentor-history")
def test_personal_mentor_requires_current_class_teacher(lab):
    admin, acsrf, teacher, tcsrf, cls = classroom(lab, "Mentors")
    add(lab, admin, acsrf, cls, "teacher2", "teacher")
    add(lab, admin, acsrf, cls, "teacher2", "student")
    add(lab, admin, acsrf, cls, "student", "student")
    path = f"/v3/classes/{cls['id']}/mentors"
    first = lab["mutate"](teacher, tcsrf, path, {
        "student_user_id": lab["actors"]["student"]["id"],
        "teacher_user_id": lab["actors"]["teacher"]["id"],
    })
    second = lab["mutate"](teacher, tcsrf, path, {
        "student_user_id": lab["actors"]["student"]["id"],
        "teacher_user_id": lab["actors"]["teacher2"]["id"],
    })
    assert first.status == second.status == 200
    assert first.json()["id"] != second.json()["id"]
    assert lab["mutate"](teacher, tcsrf, path, {
        "student_user_id": lab["actors"]["student"]["id"],
        "teacher_user_id": lab["actors"]["outsider"]["id"],
    }).status == 403


@pytest.mark.case("M05-CP03-authority-validation")
def test_group_and_mentor_authority_boundary(lab):
    admin, acsrf, teacher, tcsrf, cls = classroom(lab, "Authority")
    student, scsrf = lab["login"]("student")
    add(lab, admin, acsrf, cls, "student", "student")
    assert lab["mutate"](student, scsrf, f"/v3/classes/{cls['id']}/groups", {"name": "Intrusion"}).status == 403
    assert lab["mutate"](teacher, tcsrf, f"/v3/classes/{cls['id']}/mentors", {
        "student_user_id": lab["actors"]["student"]["id"],
        "teacher_user_id": lab["actors"]["student"]["id"],
    }).status == 400
    assert lab["mutate"](teacher, tcsrf, f"/v3/classes/{cls['id']}/groups", {"name": "x", "class_id": cls["id"]}).status == 400


@pytest.mark.case("M05-CP04-invitation-acceptance")
def test_invitation_is_target_bound_and_idempotent(lab):
    admin, acsrf, teacher, tcsrf, cls = classroom(lab, "Invitations")
    student, scsrf = lab["login"]("student")
    outsider, ocsrf = lab["login"]("outsider")
    invitation = lab["mutate"](teacher, tcsrf, f"/v3/classes/{cls['id']}/invitations", {
        "user_id": lab["actors"]["student"]["id"], "role": "student",
    })
    assert invitation.status == 200 and invitation.json()["status"] == "INVITED"
    accept_path = f"/v3/classes/{cls['id']}/invitations/{invitation.json()['id']}/accept"
    assert lab["mutate"](outsider, ocsrf, accept_path, {}).status == 403
    key = uuid.uuid4().hex
    accepted = lab["mutate"](student, scsrf, accept_path, {}, key=key)
    retried = lab["mutate"](student, scsrf, accept_path, {}, key=key)
    assert accepted.status == retried.status == 200 and accepted.json() == retried.json()
    assert accepted.json()["episode_id"] and accepted.json()["status"] == "ACCEPTED"
    assert lab["mutate"](teacher, tcsrf, f"/v3/classes/{cls['id']}/invitations", {
        "user_id": lab["actors"]["teacher2"]["id"], "role": "teacher",
    }).status == 403


@pytest.mark.case("M05-CP04-retake-provenance")
def test_retake_requires_ended_episode_and_explicit_origin(lab):
    admin, acsrf, teacher, tcsrf, cls = classroom(lab, "Retake")
    student, scsrf = lab["login"]("student")
    invitation = lab["mutate"](teacher, tcsrf, f"/v3/classes/{cls['id']}/invitations", {
        "user_id": lab["actors"]["student"]["id"], "role": "student",
    }).json()
    accepted = lab["mutate"](student, scsrf,
        f"/v3/classes/{cls['id']}/invitations/{invitation['id']}/accept", {}).json()
    episode_path = f"/v3/classes/{cls['id']}/participants/{lab['actors']['student']['id']}/episodes"
    body = {"kind": "RETAKE", "origin_episode_id": accepted["episode_id"], "recognition_policy": "SEPARATE_HISTORY"}
    assert lab["mutate"](teacher, tcsrf, episode_path, body).status == 409
    assert lab["mutate"](teacher, tcsrf,
        f"/v3/classes/{cls['id']}/participants/{lab['actors']['student']['id']}/end", {}).status == 200
    retake = lab["mutate"](teacher, tcsrf, episode_path, body)
    assert retake.status == 200
    assert retake.json()["kind"] == "RETAKE" and retake.json()["origin_episode_id"] == accepted["episode_id"]
    assert lab["mutate"](teacher, tcsrf, episode_path, {
        "kind": "RETAKE", "origin_episode_id": accepted["episode_id"], "recognition_policy": "REVIEW_REQUIRED",
    }).status == 400


@pytest.mark.case("M05-CP04-transfer-equivalence")
def test_transfer_and_equivalence_are_reference_only_policies(lab):
    admin, acsrf, teacher, tcsrf, target = classroom(lab, "Transfer target")
    student, scsrf = lab["login"]("student")
    target_invite = lab["mutate"](teacher, tcsrf, f"/v3/classes/{target['id']}/invitations", {
        "user_id": lab["actors"]["student"]["id"], "role": "student",
    }).json()
    target_accept = lab["mutate"](student, scsrf,
        f"/v3/classes/{target['id']}/invitations/{target_invite['id']}/accept", {}).json()

    source = create(lab, admin, acsrf, "Transfer source")
    add(lab, admin, acsrf, source, "teacher", "teacher")
    source_invite = lab["mutate"](teacher, tcsrf, f"/v3/classes/{source['id']}/invitations", {
        "user_id": lab["actors"]["student"]["id"], "role": "student",
    }).json()
    source_accept = lab["mutate"](student, scsrf,
        f"/v3/classes/{source['id']}/invitations/{source_invite['id']}/accept", {}).json()
    assert lab["mutate"](teacher, tcsrf,
        f"/v3/classes/{source['id']}/participants/{lab['actors']['student']['id']}/end", {}).status == 200
    target_participant = f"/v3/classes/{target['id']}/participants/{lab['actors']['student']['id']}"
    assert lab["mutate"](teacher, tcsrf, target_participant + "/end", {}).status == 200
    transfer = lab["mutate"](teacher, tcsrf, target_participant + "/episodes", {
        "kind": "TRANSFER", "origin_episode_id": source_accept["episode_id"], "recognition_policy": "REVIEW_REQUIRED",
    })
    assert transfer.status == 200 and transfer.json()["recognition_policy"] == "REVIEW_REQUIRED"
    assert transfer.json()["origin_episode_id"] == source_accept["episode_id"]
    assert target_accept["episode_id"] != transfer.json()["episode_id"]
    assert lab["mutate"](teacher, tcsrf, target_participant + "/end", {}).status == 200
    equivalent = lab["mutate"](teacher, tcsrf, target_participant + "/episodes", {
        "kind": "EQUIVALENCE", "origin_episode_id": source_accept["episode_id"], "recognition_policy": "REFERENCE_ONLY",
    })
    assert equivalent.status == 200 and equivalent.json()["recognition_policy"] == "REFERENCE_ONLY"
