import json
import os
from pathlib import Path
import subprocess
import time
import uuid
import pytest
from playwright.sync_api import sync_playwright


def pytest_ignore_collect(collection_path, config):
    return collection_path.name == "test_context.py" and os.environ.get("QA_CHECKPOINT") == "cp11"


def pytest_collection_modifyitems(items):
    # Rate-limit exhaustion follows all session-dependent journeys.
    items.sort(key=lambda item: item.name == "test_jti_single_use_across_challenges_and_rate_limit")


@pytest.fixture(scope="session")
def lab():
    root = Path(os.environ["QA_ARTIFACT_ROOT"]).resolve(strict=True)
    binary = Path(os.environ["QA_IDENTITY_BINARY"]).resolve(strict=True)
    signer = Path(os.environ["QA_ISSUER_BINARY"]).resolve(strict=True)
    assert binary.is_relative_to(root) and signer.is_relative_to(root)
    assert os.environ["QA_PG_ACK"] == "new-local-synthetic-cluster"
    cp = os.environ["QA_CHECKPOINT"]
    env = {k:v for k,v in os.environ.items() if k.upper() in {"PATH","SYSTEMROOT","WINDIR","TEMP","TMP","COMSPEC","QA_PG_ACK","QA_PG_PORT","QA_PG_MARKER","QA_RUNTIME_PASSWORD","QA_IDENTITY_PASSWORD","QA_BRIDGE_PUBLIC","QA_CHECKPOINT","GOCOVERDIR"}}
    logfile = root / (cp+"-identity-runtime.jsonl")
    with logfile.open("x",encoding="utf-8") as log:
        proc = subprocess.Popen([str(binary)],env=env,stdin=subprocess.PIPE,stdout=log,stderr=log)
        try:
            deadline=time.monotonic()+10
            address=None
            while time.monotonic()<deadline:
                assert proc.poll() is None, "identity runtime startup gate rejected"
                lines=logfile.read_text().splitlines()
                if lines:
                    event=json.loads(lines[0]); assert event["msg"]=="identity_lab_started"
                    address=event["address"];assert address.startswith("https://127.0.0.1:")
                    break
                time.sleep(.05)
            assert address
            actors=json.loads((root/(cp+"-state.json")).read_text())
            secrets=[]
            with sync_playwright() as p:
                contexts=[]
                def client():
                    c=p.request.new_context(base_url=address,ignore_https_errors=True,timeout=5000,extra_http_headers={"Origin":address})
                    contexts.append(c);return c
                def sign(subject,nonce="",**overrides):
                    claims={"sub":subject,"nonce":nonce,"jti":uuid.uuid4().hex+uuid.uuid4().hex,"version":actors.get(subject,{}).get("version",1),**overrides}
                    signer_env={k:v for k,v in env.items() if k in {"PATH","SYSTEMROOT","WINDIR","TEMP","TMP","COMSPEC","QA_PG_ACK"}}
                    signer_env["QA_LAB_DIR"]=str(root)
                    result=subprocess.run([str(signer)],input=json.dumps(claims),text=True,capture_output=True,env=signer_env,timeout=5)
                    assert result.returncode==0,"synthetic issuer failed"
                    token=json.loads(result.stdout)["assertion"];secrets.append(token);return token
                def login(subject):
                    c=client();r=c.post("/v3/auth/challenge",data={});assert r.status==200
                    r=c.post("/v3/auth/exchange",data={"assertion":sign(subject,r.json()["nonce"])});assert r.status==200
                    csrf=r.json()["csrf_token"];secrets.append(csrf)
                    cookie=[v for v in c.storage_state()["cookies"] if v["name"]=="__Host-lms-v3"][0]
                    secrets.append(cookie["value"]);return c,csrf
                def mutate(c,csrf,path,body,method="post",key=None):
                    return getattr(c,method)(path,data=body,headers={"X-CSRF-Token":csrf,"Idempotency-Key":key or uuid.uuid4().hex})
                try:
                    yield dict(client=client,sign=sign,login=login,actors=actors,mutate=mutate,origin=address)
                finally:
                    for c in contexts:c.dispose()
                    contents=logfile.read_text()
                    assert all(s not in contents for s in secrets),"credential in runtime log"
                    assert env["QA_IDENTITY_PASSWORD"] not in contents and env["QA_RUNTIME_PASSWORD"] not in contents
        finally:
            if proc.poll() is None:proc.stdin.write(b"shutdown\n");proc.stdin.flush()
            try:proc.wait(timeout=8)
            except subprocess.TimeoutExpired:proc.kill();proc.wait(timeout=3);pytest.fail("identity shutdown timed out")
            finally:proc.stdin.close()
            assert proc.returncode==0
