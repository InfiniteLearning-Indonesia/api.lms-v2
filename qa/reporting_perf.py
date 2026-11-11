"""Independent fail-closed validation of the frozen M14-CP04 native samples."""
import math


def number(value):
    if isinstance(value, bool) or not isinstance(value, (float, int)) or not math.isfinite(value) or value < 0:
        raise ValueError('invalid finite metric')
    return value


def percentile(values, fraction):
    if not values:
        raise ValueError('missing samples')
    return sorted(number(x) for x in values)[math.ceil(len(values)*fraction)-1]


def phase(chunks, budget, workload, kind, concurrency, wall_ms):
    spec = budget[workload]
    if len(chunks) != budget['chunks_per_phase'] or concurrency not in spec['concurrency'] or kind not in budget['kinds']:
        raise ValueError('phase contract mismatch')
    samples, duration, maximum = [], 0, 0
    machine = None
    for chunk in chunks:
        if any(chunk[key] != expected for key, expected in {
            'kind': kind, 'items': spec['items'], 'grade_version': spec['grade_version'],
            'concurrency': concurrency, 'requests': budget['requests_per_chunk'],
            'warmups': budget['warmups_per_chunk'], 'errors': 0, 'warmup_errors': 0, 'error_codes': {},
        }.items()):
            raise ValueError('chunk correctness/workload failed')
        values = chunk['latencies_ms']
        if len(values) != budget['requests_per_chunk']:
            raise ValueError('chunk sample count mismatch')
        for key, quantile in [('p50_ms', .5), ('p95_ms', .95), ('p99_ms', .99)]:
            if not math.isclose(number(chunk[key]), percentile(values, quantile), rel_tol=1e-9, abs_tol=1e-9):
                raise ValueError('native percentile mismatch')
        elapsed = number(chunk['duration_ms'])
        if elapsed <= 0 or not math.isclose(number(chunk['requests_per_second']), len(values)*1000/elapsed, rel_tol=1e-9):
            raise ValueError('invalid measurement window throughput')
        # Closed-loop sum of service times cannot exceed C times measured wall.
        if sum(values) > elapsed*concurrency*1.02 or max(values) > elapsed*1.02:
            raise ValueError('inconsistent native timing')
        size = number(chunk['max_response_bytes'])
        if size <= 0 or size > budget['max_response_bytes']:
            raise ValueError('response byte budget exceeded')
        current = {key: chunk[key] for key in ('os', 'arch', 'go_version', 'cpus')}
        if not all(current.values()) or number(current['cpus']) < 1 or machine is not None and current != machine:
            raise ValueError('runtime metadata mismatch')
        machine = current
        samples.extend(values); duration += elapsed; maximum = max(maximum, size)
    if len(samples) != budget['samples_per_phase'] or number(wall_ms) < duration:
        raise ValueError('phase totals mismatch')
    p95, p99 = percentile(samples, .95), percentile(samples, .99)
    if p95 > spec['p95_ms'] or p99 > spec['p99_ms']:
        raise ValueError('frozen latency budget exceeded')
    return {'workload': workload, 'kind': kind, 'concurrency': concurrency, 'samples': len(samples),
            'p50_ms': percentile(samples,.5), 'p95_ms': p95, 'p99_ms': p99, 'max_response_bytes': maximum,
            'measurement_ms': duration, 'phase_wall_ms': wall_ms, 'requests_per_second': len(samples)*1000/duration,
            'wall_requests_per_second': len(samples)*1000/wall_ms, 'runtime': machine}


def review(raw, budget):
    required = {(w, k, c) for w in ('small','boundary') for k in budget['kinds'] for c in budget[w]['concurrency']}
    observed = [(p['workload'],p['kind'],p['concurrency']) for p in raw['phases']]
    if len(observed) != len(required) or set(observed) != required:
        raise ValueError('missing/duplicate performance phases')
    phases = [phase(p['chunks'],budget,p['workload'],p['kind'],p['concurrency'],p['wall_ms']) for p in raw['phases']]
    return {'status':'PASS_SYNTHETIC_ONLY','profile':budget['profile'], 'phases':phases,
            'measured_requests':sum(p['samples'] for p in phases), 'production_sla_verified':False}
