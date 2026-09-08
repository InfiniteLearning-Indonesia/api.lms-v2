import time
import uuid
import pytest


@pytest.mark.case("CP11-exchange")
def test_verified_exchange_and_denials(lab):
    c=lab["client"]();nonce=c.post("/v3/auth/challenge",data={}).json()["nonce"]
    for change in ({"iss":"https://foreign.invalid"},{"aud":"wrong"},{"purpose":"reset"},{"exp":int(time.time())-1},{"nbf":int(time.time())+30},{"active":False},{"nonce":"0"*64},{"site_admin":True}):
        claims={"nonce":nonce,**change};raw=lab["sign"]("student",**claims)
        assert c.post("/v3/auth/exchange",data={"assertion":raw}).status in (401,409)
    raw=lab["sign"]("student",nonce)
    assert c.post("/v3/auth/exchange",data={"assertion":raw[:-4]+"abcd"}).status==401
    assert lab["client"]().post("/v3/auth/exchange",data={"assertion":raw}).status==401
    result=c.post("/v3/auth/exchange",data={"assertion":raw});assert result.status==200 and "token" not in result.json()
    assert c.get("/v3/auth/me").json()["id"]==lab["actors"]["student"]["id"]
    assert c.post("/v3/auth/exchange",data={"assertion":raw}).status in (401,409)
    for subject in ("unknown-person","disabled"):
        stranger=lab["client"]();n=stranger.post("/v3/auth/challenge",data={}).json()["nonce"]
        assert stranger.post("/v3/auth/exchange",data={"assertion":lab["sign"](subject,n)}).status==401


@pytest.mark.case("CP11-session")
def test_cookie_rotation_logout_and_revoke(lab):
    c,csrf=lab["login"]("teacher")
    cookies=c.storage_state()["cookies"];session=next(x for x in cookies if x["name"]=="__Host-lms-v3")
    assert session["secure"] and session["httpOnly"] and session["sameSite"]=="Strict" and session["path"]=="/"
    old=session["value"]
    assert lab["mutate"](c,"0"*64,"/v3/auth/rotate",{}).status==403
    rotated=lab["mutate"](c,csrf,"/v3/auth/rotate",{});assert rotated.status==200
    csrf=rotated.json()["csrf_token"]
    assert c.get("/v3/auth/me",headers={"Cookie":"__Host-lms-v3="+old}).status==401
    assert c.get("/v3/auth/me").status==200
    second,_=lab["login"]("teacher")
    assert lab["mutate"](c,csrf,"/v3/auth/revoke-all",{}).status==200
    assert second.get("/v3/auth/me").status==401
    c,csrf=lab["login"]("teacher")
    assert lab["mutate"](c,csrf,"/v3/auth/logout",{}).status==200
    assert c.get("/v3/auth/me").status==401


@pytest.mark.case("CP11-boundary")
def test_owner_cookie_csrf_and_input_boundaries(lab):
    c,csrf=lab["login"]("student")
    assert c.get("/v3/auth/me?sid=anything").status==401
    assert c.get("/v3/auth/me",headers={"Authorization":"Bearer "+"a"*64}).status==401
    assert c.post("/v3/auth/rotate",data={},headers={"Origin":"https://evil.invalid","X-CSRF-Token":csrf}).status==403
    assert c.post("/v3/auth/rotate",data={},headers={"Origin":"","X-CSRF-Token":csrf}).status==403
    assert lab["mutate"](c,csrf,"/v3/auth/rotate",{"site_admin":True}).status==400
    assert c.post("/v3/auth/exchange",data={"email":"student@invalid.test"}).status==400
    assert c.post("/v3/auth/exchange",data={"assertion":"x"*14000}).status==400
    for path in ("setup-password","invite","reset","profile"):
        assert lab["mutate"](c,csrf,"/v3/auth/"+path,{"email":"someone@invalid.test","status":"ACTIVE"}).status==404
    assert c.get("/v3/auth/me",headers={"X-Role":"admin"}).json()["site_admin"] is False
    assert c.get("/v3/auth/me",headers={"Cookie":"__Host-lms-v3="+"a"*64+"; __Host-lms-v3="+"b"*64}).status==401
    assert c.post("/v3/auth/rotate",data={},headers={"X-CSRF-Token":csrf,"Sec-Fetch-Site":"cross-site"}).status==403
    assert c.post("/v3/auth/rotate",data="{}",headers={"X-CSRF-Token":csrf,"Content-Type":"text/plain"}).status==400
    assert c.post("/v3/auth/rotate",data="{} {}",headers={"X-CSRF-Token":csrf,"Content-Type":"application/json"}).status==400


@pytest.mark.case("CP11-projection")
def test_signed_monotonic_account_projection(lab):
    subject="revoked";version=lab["actors"][subject]["version"]
    c,_=lab["login"](subject);issuer=lab["client"]()
    raw=lab["sign"](subject,purpose="account_state",version=version+1,active=False)
    assert issuer.post("/v3/auth/account-state",data={"assertion":raw}).status==200
    assert c.get("/v3/auth/me").status==401
    assert issuer.post("/v3/auth/account-state",data={"assertion":raw}).status==409
    stale=lab["sign"](subject,purpose="account_state",version=version,active=True)
    assert issuer.post("/v3/auth/account-state",data={"assertion":stale}).status==409
    raw=lab["sign"](subject,purpose="account_state",version=version+2,active=True)
    assert issuer.post("/v3/auth/account-state",data={"assertion":raw}).status==200
    lab["actors"][subject]["version"]=version+2
    assert lab["login"](subject)[0].get("/v3/auth/me").status==200
    assert c.get("/v3/auth/me").status==401


@pytest.mark.case("CP11-site-admin")
def test_site_authority_is_not_identity_claim(lab):
    admin,csrf=lab["login"]("admin");student,student_csrf=lab["login"]("student")
    target=lab["actors"]["admin"]["id"]
    version=lab["actors"]["admin"]["site_role_version"]
    assert lab["mutate"](admin,csrf,"/v3/auth/site-role",{"user_id":target,"version":version}).status==400
    # No role changes in this test; the sole local admin must remain available.
    assert lab["mutate"](admin,csrf,"/v3/auth/site-role",{"user_id":target,"admin":False,"version":version}).status==409
    assert lab["mutate"](student,student_csrf,"/v3/auth/site-role",{"user_id":target,"admin":False,"version":1}).status==403
    other=lab["actors"]["teacher2"]
    other_client,_=lab["login"]("teacher2")
    body={"user_id":other["id"],"admin":True,"version":other["site_role_version"]}
    assert lab["mutate"](admin,csrf,"/v3/auth/site-role",body).status==200
    assert other_client.get("/v3/auth/me").json()["site_admin"] is True
    assert lab["mutate"](admin,csrf,"/v3/auth/site-role",body).status==409
    body.update(admin=False,version=body["version"]+1)
    assert lab["mutate"](admin,csrf,"/v3/auth/site-role",body).status==200
    assert other_client.get("/v3/auth/me").json()["site_admin"] is False


@pytest.mark.case("CP11-replay-rate")
def test_jti_single_use_across_challenges_and_rate_limit(lab):
    c=lab["client"]();jti=uuid.uuid4().hex+uuid.uuid4().hex
    n=c.post("/v3/auth/challenge",data={}).json()["nonce"]
    assert c.post("/v3/auth/exchange",data={"assertion":lab["sign"]("student",n,jti=jti)}).status==200
    n=c.post("/v3/auth/challenge",data={}).json()["nonce"]
    assert c.post("/v3/auth/exchange",data={"assertion":lab["sign"]("student",n,jti=jti)}).status==409
    # A failed JTI consume must roll back challenge consumption as well.
    assert c.post("/v3/auth/exchange",data={"assertion":lab["sign"]("student",n)}).status==200
    # Run this last; process-local fixed-window limiter is intentionally coarse.
    limited=False
    for _ in range(121):
        response=c.post("/v3/auth/challenge",data={})
        assert response.status in (200,429)
        if response.status==429:
            limited=True;break
    assert limited
