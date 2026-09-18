import uuid

import pytest


def classroom(lab):
    admin, admin_csrf = lab["login"]("admin")
    teacher, teacher_csrf = lab["login"]("teacher")
    outsider, outsider_csrf = lab["login"]("outsider")
    created = lab["mutate"](admin, admin_csrf, "/v3/classes", {"name": "M06 bounded egress"})
    assert created.status == 200
    cls = created.json()
    assigned = lab["mutate"](admin, admin_csrf, f"/v3/classes/{cls['id']}/participants", {
        "user_id": lab["actors"]["teacher"]["id"], "role": "teacher",
    })
    assert assigned.status == 200
    published = lab["mutate"](admin, admin_csrf, f"/v3/classes/{cls['id']}/publish", {"version": 1})
    assert published.status == 200
    return cls, (teacher, teacher_csrf), (outsider, outsider_csrf)


def remote(lab, actor, csrf, cls, url, **extra):
    body = {
        "url": url,
        "owner_user_id": lab["actors"]["teacher"]["id"],
        "purpose": "MATERIAL",
        "original_name": "remote.txt",
        "declared_media_type": "text/plain",
        **extra,
    }
    return actor.post(f"/v3/classes/{cls['id']}/file-imports", data=body, headers={
        "X-CSRF-Token": csrf, "Idempotency-Key": uuid.uuid4().hex,
    })


@pytest.mark.case("M06-CP03-private-address")
def test_private_and_nonstandard_targets_fail_closed(lab):
    cls, (teacher, csrf), _ = classroom(lab)
    for url in (
        "https://localhost/internal",
        "https://127.0.0.1/internal",
        "https://[::1]/internal",
        "https://localhost:8443/internal",
    ):
        assert remote(lab, teacher, csrf, cls, url).status == 400


@pytest.mark.case("M06-CP03-allowlist-url")
def test_scheme_userinfo_and_arbitrary_provider_are_rejected(lab):
    cls, (teacher, csrf), _ = classroom(lab)
    for url in (
        "http://localhost/file.txt",
        "https://user@localhost/file.txt",
        "https://arbitrary-ai.example/file.txt",
    ):
        assert remote(lab, teacher, csrf, cls, url).status == 400


@pytest.mark.case("M06-CP03-authority-failclosed")
def test_authority_and_request_shape_precede_egress(lab):
    cls, _, (outsider, csrf) = classroom(lab)
    assert remote(lab, outsider, csrf, cls, "https://localhost/internal").status == 403
    teacher, teacher_csrf = lab["login"]("teacher")
    malformed = remote(lab, teacher, teacher_csrf, cls, "https://localhost/internal", unexpected=True)
    assert malformed.status == 400
