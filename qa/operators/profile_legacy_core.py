"""Offline dry run of the pinned legacy snapshot. Reports aggregate metadata only."""

from collections import Counter, defaultdict
from datetime import datetime
from hashlib import sha256
import json
import re
from uuid import UUID

from qa.operators.restore_legacy_baseline import PIN, SERVICE, SNAPSHOT, Stop, copy_ledger


FIELDS = {
    'programs': ('id', 'name'),
    'batches': ('id', 'name', 'programId', 'includedProgramIds', 'logbookSchedule'),
    'classes': ('id', 'programId', 'batchId', 'mentorId', 'createdAt',
                'isTranscriptReleased', 'isCertificateReleased'),
    'users': ('id', 'email', 'googleId', 'roles', 'status', 'createdAt'),
    'enrollments': ('id', 'studentId', 'classId', 'createdAt'),
    'program_heads': ('programId', 'userId'),
}
CORE = ('programs', 'batches', 'classes', 'users', 'enrollments')
STATUSES = {'invited', 'active', 'suspended', 'inactive', 'graduated'}
ROLES = {'admin', 'facilitator', 'mentor', 'student'}
DECISION_READY = 'candidate'
DECISION_REVIEW = 'hold_for_review'
DECISION_DEFERRED = 'pending_domain_mapping'


def decode_copy(field):
    if field == b'\\N':
        return None
    # pg_dump text COPY escapes bytes before pg_restore renders the SQL.
    controls = {ord(code): value for code, value in zip('bfnrtv', b'\b\f\n\r\t\v')}
    def escape(match):
        token = match[1]
        if token[:1] == b'x' and len(token) > 1:
            return bytes([int(token[1:], 16)])
        if token[:1] in b'01234567':
            return bytes([int(token, 8) & 255])
        return bytes([controls.get(token[0], token[0])])
    return re.sub(rb'\\(x[0-9A-Fa-f]{1,2}|[0-7]{1,3}|.)', escape, field).decode('utf-8')


def selected_rows(raw, ledger):
    selected = {table: [] for table in FIELDS}
    indexes = None
    table = None
    for line in raw.splitlines():
        if table is not None:
            if line == b'\\.':
                table, indexes = None, None
            elif indexes is not None:
                parts = line.split(b'\t')
                selected[table].append({k: decode_copy(parts[i]) for k, i in indexes.items()})
            continue
        header = re.match(rb'^COPY public\.([a-z_]+) \(', line)
        if header:
            table = header[1].decode('ascii')
            if table in FIELDS:
                columns = ledger[table]['columns']
                indexes = {k: columns.index(k) for k in FIELDS[table]}
    if any(len(rows) != ledger[table]['rows'] for table, rows in selected.items()):
        raise Stop('PROFILE_ROW_SCOPE_MISMATCH')
    return selected


def keyed(rows):
    result = {}
    for row in rows:
        key = row['id']
        if not isinstance(key, str) or str(UUID(key)) != key or key in result:
            raise Stop('PROFILE_PRIMARY_KEY_MISMATCH')
        result[key] = row
    return result


def normalize(value):
    return value.strip().casefold() if value else ''


def collisions(rows, field, transform):
    groups = defaultdict(set)
    for row in rows.values():
        name = transform(row[field])
        if name:
            groups[name].add(row['id'])
    return [group for group in groups.values() if len(group) > 1]


def describe_groups(groups):
    return {'groups': len(groups), 'rows': sum(len(group) for group in groups)}


def account_roles(value):
    try:
        roles = json.loads(value)
    except (TypeError, ValueError):
        return None
    if not isinstance(roles, list) or not all(isinstance(role, str) and role in ROLES for role in roles):
        return None
    if len(roles) != len(set(roles)):
        return None
    return set(roles)


def profile(raw, accepted_ledger):
    ledger = copy_ledger(raw)
    if ledger != accepted_ledger or len(ledger) != 21 or sum(t['rows'] for t in ledger.values()) != 7843:
        raise Stop('PROFILE_ARCHIVE_DIFFERS_FROM_ACCEPTED_RESTORE')
    source = selected_rows(raw, ledger)
    programs, batches, classes, users, enrollments = (keyed(source[name]) for name in CORE)
    decisions = {name: Counter() for name in CORE}
    details = {name: Counter() for name in CORE}
    account = Counter()
    legacy_roles = Counter()
    mentor = Counter()
    roster = Counter()

    def record(table, reason=None):
        decisions[table][DECISION_REVIEW if reason else DECISION_READY] += 1
        if reason:
            details[table][reason] += 1
        return reason is None

    email_dupes = collisions(users, 'email', normalize)
    google_dupes = collisions(users, 'googleId', lambda v: v.strip() if v else '')
    conflicting_accounts = set().union(*email_dupes, *google_dupes)
    roles_by_user = {}
    eligible_users = set()
    for user_id, row in users.items():
        status = row['status'] if row['status'] in STATUSES else 'unknown'
        roles = account_roles(row['roles'])
        account[status] += 1
        legacy_roles.update(roles if roles is not None else ['invalid'])
        roles_by_user[user_id] = roles or set()
        reason = None
        if status == 'unknown' or roles is None or not normalize(row['email']):
            reason = 'unknown_status_invalid_roles_or_missing_email'
        elif user_id in conflicting_accounts:
            reason = 'identity_collision'
        if record('users', reason):
            eligible_users.add(user_id)

    label_groups = defaultdict(set)
    for row in programs.values():
        record('programs', None if normalize(row['name']) else 'blank_program_label')
    for row in batches.values():
        record('batches', None if normalize(row['name']) else 'blank_cohort_label')
    eligible_classes = set()
    program_batch_groups = Counter()
    batch_primary_mismatch = 0
    released = Counter()
    for class_id, row in classes.items():
        program = programs.get(row['programId'])
        batch = batches.get(row['batchId'])
        program_batch_groups[row['programId'], row['batchId']] += 1
        if batch and batch['programId'] and batch['programId'] != row['programId']:
            batch_primary_mismatch += 1
        reason = None
        if not program or not batch:
            reason = 'missing_program_or_batch_reference'
        elif not normalize(program['name']) or not normalize(batch['name']):
            reason = 'blank_source_label'
        else:
            proposed_name = program['name'].strip() + ' — ' + batch['name'].strip()
            if len(proposed_name) > 200:
                reason = 'class_name_over_200_characters'
            label_groups[normalize(proposed_name)].add(class_id)
        if record('classes', reason):
            eligible_classes.add(class_id)
        for field in ('isTranscriptReleased', 'isCertificateReleased'):
            released[field] += row[field] == 't'
        if not row['mentorId']:
            mentor['unassigned'] += 1
        elif row['mentorId'] not in users:
            mentor['missing_user'] += 1
        elif class_id not in eligible_classes or row['mentorId'] not in eligible_users:
            mentor['dependency_held'] += 1
        elif 'mentor' not in roles_by_user[row['mentorId']]:
            mentor['legacy_mentor_role_disagrees'] += 1
        else:
            mentor['assignment_candidate'] += 1
            if users[row['mentorId']]['status'] != 'active':
                mentor['candidate_with_nonactive_account'] += 1

    pairs = Counter((row['classId'], row['studentId']) for row in enrollments.values())
    for row in enrollments.values():
        user = users.get(row['studentId'])
        course = classes.get(row['classId'])
        reason = None
        if not user or not course:
            reason = 'missing_user_or_class_reference'
        elif row['studentId'] not in eligible_users or row['classId'] not in eligible_classes:
            reason = 'dependency_held'
        elif pairs[row['classId'], row['studentId']] > 1:
            reason = 'duplicate_user_class_pair'
        elif 'student' not in roles_by_user[row['studentId']]:
            reason = 'legacy_student_role_disagrees'
        precedes_class = False
        if user and course:
            if course['mentorId'] == row['studentId']:
                roster['same_user_mentor_and_enrolled_in_class'] += 1
            recorded = datetime.fromisoformat(row['createdAt'])
            for key, origin in [('class', course), ('user', user)]:
                if recorded < datetime.fromisoformat(origin['createdAt']):
                    roster['enrollment_precedes_' + key] += 1
                    precedes_class |= key == 'class'
        if record('enrollments', reason):
            if user['status'] != 'active':
                roster['candidate_with_nonactive_account'] += 1
            if precedes_class:
                roster['candidate_with_chronology_to_review'] += 1

    inventory = {}
    for table, entry in sorted(ledger.items()):
        counts = dict(decisions[table]) if table in CORE else {DECISION_DEFERRED: entry['rows']}
        if sum(counts.values()) != entry['rows']:
            raise Stop('PROFILE_ACCOUNTING_MISMATCH')
        inventory[table] = {'rows': entry['rows'], 'decisions': counts}
        if details.get(table):
            inventory[table]['hold_reasons'] = dict(details[table])
    totals = Counter()
    for entry in inventory.values():
        totals.update(entry['decisions'])
    return {
        'status': 'PASS_OFFLINE_MAPPING_PROFILE_ONLY',
        'snapshot_run': SNAPSHOT,
        'source_sql_sha256': sha256(raw).hexdigest(),
        'ruleset': 'legacy-core-preview-v1',
        'inventory': inventory,
        'decision_totals': dict(totals),
        'account_statuses': dict(account),
        'legacy_global_role_memberships': dict(legacy_roles),
        'normalized_email_collisions': describe_groups(email_dupes),
        'google_subject_collisions': describe_groups(google_dupes),
        'class_label_collisions': describe_groups([v for v in label_groups.values() if len(v) > 1]),
        'repeated_program_batch_pairs': {
            'groups': sum(count > 1 for count in program_batch_groups.values()),
            'rows': sum(count for count in program_batch_groups.values() if count > 1),
        },
        'class_batch_primary_program_differs': batch_primary_mismatch,
        'class_release_flags': dict(released),
        'teacher_assignments': dict(mentor),
        'enrollment_observations': dict(roster),
        'program_head_references': {
            'rows': len(source['program_heads']),
            'missing_program_or_user': sum(r['programId'] not in programs or r['userId'] not in users
                                           for r in source['program_heads']),
        },
        'batch_metadata': {
            'rows_with_schedule': sum(bool(r['logbookSchedule']) and r['logbookSchedule'] not in {'[]', '{}'} for r in batches.values()),
            'rows_with_included_programs': sum(bool(r['includedProgramIds']) and r['includedProgramIds'] not in {'[]', '{}'} for r in batches.values()),
        },
    }


def main():
    backup = SERVICE.parent / 'private-backup'
    run = backup / '20261119T093357Z-legacy-local-restore-debaab74a513'
    try:
        size, digest = PIN['database-public.sql']
        sql = backup / SNAPSHOT / 'database-public.sql'
        if sql.stat().st_size != size:
            raise Stop('PROFILE_PINNED_INPUT_SIZE_MISMATCH')
        raw = sql.read_bytes()
        if sha256(raw).hexdigest() != digest:
            raise Stop('PROFILE_PINNED_INPUT_HASH_MISMATCH')
        receipt = json.loads((run / 'summary.json').read_text(encoding='utf-8'))
        if (receipt.get('source_run') != SNAPSHOT or receipt.get('actual_restore_verified') is not True
                or receipt.get('status') != 'PASS_REVIEWED_LOCAL_BASELINE_RESTORE_ONLY'):
            raise Stop('PROFILE_ACCEPTED_RESTORE_REQUIRED')
        ledger = json.loads((run / 'source-copy-ledger.json').read_text(encoding='utf-8'))
        print(json.dumps(profile(raw, ledger), sort_keys=True, indent=2))
    except (Stop, KeyError, TypeError, OSError, ValueError):
        # Parsing exceptions may include values from the snapshot.
        print(json.dumps({'status': 'PROFILE_FAILED', 'reason': 'INPUT_OR_ACCOUNTING_CHECK_FAILED'}))
        return 2
    return 0


if __name__ == '__main__':
    raise SystemExit(main())
