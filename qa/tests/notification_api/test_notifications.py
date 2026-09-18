import uuid

import pytest


@pytest.mark.case("M13-CP01-delivery-lifecycle")
def test_admin_observes_receipts_and_duplicate_semantics(lab):
    admin, _ = lab["login"]("admin")
    response = admin.get("/v3/operations/deliveries")
    assert response.status == 200
    rows = response.json()["deliveries"]
    receipts = [row for row in rows if row.get("provider_key") == "rehearsal.email"]
    assert receipts and {row["provider_outcome"] for row in receipts} >= {"CONFIRMED", "DUPLICATE"}
    assert all(len(row["reference_hash"]) == 64 for row in receipts)
    assert "secretref://" not in response.text() and "password" not in response.text().lower()


@pytest.mark.case("M13-CP01-delivery-boundary")
def test_delivery_visibility_is_admin_only_and_cookie_only(lab):
    student, _ = lab["login"]("student")
    assert student.get("/v3/operations/deliveries").status == 403
    assert student.get("/v3/operations/deliveries", headers={"Authorization": "Bearer forged"}).status == 401


@pytest.mark.case("M13-CP02-notification-lifecycle")
def test_user_controls_own_email_preference_idempotently(lab):
    student, csrf = lab["login"]("student")
    current = student.get("/v3/notification-preferences/email")
    assert current.status == 200 and current.json()["channel"] == "EMAIL"
    key = uuid.uuid4().hex
    body = {"enabled": True, "expected_version": current.json()["version"]}
    first = lab["mutate"](student, csrf, "/v3/notification-preferences/email", body, key=key, method="PUT")
    replay = lab["mutate"](student, csrf, "/v3/notification-preferences/email", body, key=key, method="PUT")
    assert first.status == 200 and replay.status == 200
    assert replay.headers.get("idempotent-replay") == "true" and replay.json() == first.json()
    assert student.get("/v3/notification-preferences/email").json()["enabled"] is True


@pytest.mark.case("M13-CP02-notification-boundary")
def test_import_suppression_and_operator_redaction(lab):
    admin, _ = lab["login"]("admin")
    student, csrf = lab["login"]("student")
    assert student.get("/v3/operations/notifications").status == 403
    response = admin.get("/v3/operations/notifications")
    assert response.status == 200
    imports = [row for row in response.json()["notifications"] if row["source"] == "HISTORICAL_IMPORT"]
    assert imports and all(row["status"] == "SUPPRESSED" and row["suppression_code"] == "HISTORICAL_IMPORT" for row in imports)
    assert "secretref://" not in response.text() and "smtp-password" not in response.text()
    invalid = lab["mutate"](student, csrf, "/v3/notification-preferences/email", {
        "enabled": False, "expected_version": 0, "user_id": lab["actors"]["admin"]["id"],
    }, method="PUT")
    assert invalid.status == 400
