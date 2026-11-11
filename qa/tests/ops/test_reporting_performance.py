import copy
import json
from pathlib import Path
import pytest
from qa.reporting_perf import review


@pytest.mark.case('M14-CP04-load-acceptance')
def test_frozen_acceptance_rejects_incomplete_or_fabricated_metrics():
    budget = json.loads((Path(__file__).resolve().parents[2]/'manifests/M14-CP04-budget.json').read_text())
    raw = {'phases':[]}
    for w in ('small','boundary'):
        for k in budget['kinds']:
            for c in budget[w]['concurrency']:
                chunk = dict(kind=k,items=budget[w]['items'],grade_version=budget[w]['grade_version'],concurrency=c,
                             requests=30,warmups=2,errors=0,warmup_errors=0,error_codes={},latencies_ms=[1.0]*30,
                             duration_ms=30.,requests_per_second=1000.,p50_ms=1.,p95_ms=1.,p99_ms=1.,
                             max_response_bytes=512,os='synthetic',arch='test',go_version='test',cpus=1)
                raw['phases'].append(dict(workload=w,kind=k,concurrency=c,wall_ms=150.,chunks=[copy.deepcopy(chunk) for _ in range(4)]))
    assert review(raw,budget)['measured_requests'] == 1080
    for field, value in [('errors',1),('warmup_errors',1),('latencies_ms',[float('nan')]*30),
                         ('p95_ms',0),('requests',29),('items',201),('duration_ms',0),('max_response_bytes',262145)]:
        bad=copy.deepcopy(raw);bad['phases'][0]['chunks'][0][field]=value
        with pytest.raises(ValueError): review(bad,budget)
    for phases in (raw['phases'][:-1],raw['phases']+[raw['phases'][0]]):
        with pytest.raises(ValueError): review({'phases':phases},budget)
    bad=copy.deepcopy(raw)
    for chunk in bad['phases'][0]['chunks']:
        chunk.update(latencies_ms=[2000.]*30,p50_ms=2000.,p95_ms=2000.,p99_ms=2000.,duration_ms=60000.,requests_per_second=.5)
    bad['phases'][0]['wall_ms']=300000.
    with pytest.raises(ValueError):review(bad,budget)
