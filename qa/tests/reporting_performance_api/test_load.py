"""Actual API seed + native HTTP load. Playwright supplies auth, never timings."""
from datetime import datetime, timedelta, timezone
import json
import os
from pathlib import Path
import subprocess
import time
import pytest
from qa.evidence import write_new_json
from qa.reporting_perf import review
from qa.tests.gradebook_api.test_gradebook import mutate
from qa.tests.reporting_api.test_report_pages import two_items, released


@pytest.mark.case('M14-CP04-benchmark')
def test_native_reporting_load(lab):
    root=Path(os.environ['QA_ARTIFACT_ROOT']).resolve(strict=True)
    binary=Path(os.environ['QA_REPORT_LOAD_BINARY']).resolve(strict=True)
    assert binary.is_relative_to(root) and os.environ['QA_PG_ACK']=='new-local-synthetic-cluster'
    budget=json.loads((Path(__file__).resolve().parents[2]/'manifests/M14-CP04-budget.json').read_text())
    progress=root/'m14cp04-progress.jsonl'
    def note(label):
        with progress.open('a',encoding='utf-8') as f:f.write(json.dumps({'stage':label,'utc':datetime.now(timezone.utc).isoformat()})+'\n')
        print('[qa] M14-CP04 '+label,flush=True)
    rooms={};state={}
    for workload in ('small','boundary'):
        note('seed '+workload)
        room,first,second,_=two_items(lab,'M14 performance '+workload)
        rooms[workload]=room
        refreshed=time.monotonic()
        def refresh():
            nonlocal refreshed
            if time.monotonic()-refreshed>12:
                for subject in ('teacher','student'): room['pairs'][subject]=lab['login'](subject)
                refreshed=time.monotonic()
        base=f"/v3/classes/{room['class']['id']}"
        def versions(item):
            _,aggregate,revision,grades=item
            for version in (2,3):
                refresh()
                assert mutate(lab,room['pairs']['teacher'],grades,{'aggregate_id':aggregate['id'],'submission_revision_id':revision['id'],'expected_version':version-1,'value_milli':70000+version,'reason':'Synthetic benchmark history'}).status==200
                assert mutate(lab,room['pairs']['teacher'],grades+f"/{aggregate['id']}/release",{'expected_version':version}).status==200
        if workload=='boundary':
            versions(first);versions(second)
            for i in range(3,201):
                refresh()
                response=mutate(lab,room['pairs']['teacher'],base+'/activities',{'section_id':room['activity']['section_id'],'activity_type':'ASSIGNMENT','title':f'Load essay {i}','position':i})
                assert response.status==201
                activity=response.json();now=datetime.now(timezone.utc)
                response=mutate(lab,room['pairs']['teacher'],base+f"/activities/{activity['id']}/revisions",{'raw_body':'Synthetic load essay','timezone':'Asia/Jakarta','available_from':(now-timedelta(hours=1)).isoformat(),'due_at':(now+timedelta(hours=2)).isoformat(),'cutoff_at':(now+timedelta(hours=3)).isoformat(),'assignment':{'submission_mode':'TEXT','max_attempts':2,'max_files':0,'max_file_bytes':0,'late_policy':'REJECT_AFTER_CUTOFF'}})
                assert response.status==201
                revision=response.json()
                response=mutate(lab,room['pairs']['teacher'],base+f"/activities/{activity['id']}/revisions/{revision['id']}/publish",{'version':revision['version']})
                assert response.status==200
                refresh()
                item=released(lab,{**room,'activity':activity,'revision':response.json()},f'Load grade {i}')
                versions(item)
                if i%20==0: note(f'seed boundary {i}/200')
        state[workload]={'class_id':room['class']['id'],'learner_id':lab['actors']['student']['id'],'items':budget[workload]['items'],'grade_version':budget[workload]['grade_version']}
    write_new_json(root/'m14cp04-benchmark-state.json',state)
    raw={'phases':[]}
    env={k:v for k,v in os.environ.items() if k.upper() in {'PATH','SYSTEMROOT','WINDIR','TEMP','TMP','COMSPEC','QA_PG_ACK'}}
    for workload in ('small','boundary'):
        for kind in budget['kinds']:
            for concurrency in budget[workload]['concurrency']:
                label=f'{workload}-{kind}-c{concurrency}';note('load '+label)
                start=time.monotonic();chunks=[]
                for i in range(4):
                    pair=lab['login']('student')
                    session=next(c['value'] for c in pair[0].storage_state()['cookies'] if c['name']=='__Host-lms-v3')
                    config={'base_url':lab['origin'],'session':session,'csrf':pair[1],**state[workload],
                            'expected_items':state[workload]['items'],'expected_version':state[workload]['grade_version'],
                            'kind':kind,'requests':30,'concurrency':concurrency,'warmups':2}
                    config.pop('items');config.pop('grade_version')
                    completed=subprocess.run([str(binary)],input=json.dumps(config),text=True,capture_output=True,env=env,timeout=190)
                    # Do not expose stdin, token, arbitrary subprocess output in pytest assertion.
                    if completed.returncode!=0: raise AssertionError('native synthetic load command failed')
                    chunk=json.loads(completed.stdout)
                    write_new_json(root/f'm14cp04-{label}-chunk{i+1}.json',chunk);chunks.append(chunk)
                raw['phases'].append({'workload':workload,'kind':kind,'concurrency':concurrency,'wall_ms':(time.monotonic()-start)*1000,'chunks':chunks})
    write_new_json(root/'m14cp04-load-raw.json',raw)
    accepted=review(raw,budget)
    write_new_json(root/'m14cp04-load-acceptance.json',accepted)
    note('load acceptance PASS 1080 measured requests')
