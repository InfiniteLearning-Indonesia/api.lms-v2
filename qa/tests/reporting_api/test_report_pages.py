import csv
from datetime import datetime, timedelta, timezone
import io

import pytest

from qa.tests.gradebook_api.test_gradebook import classroom, create_definition, definition_body, publish, handin, mutate


def released(lab, room, title='Essay grade', value=71000):
    body = definition_body()
    body['title'] = title
    definition = create_definition(lab, room, body)
    assert publish(lab, room, definition).status == 200
    aggregate, revision = handin(lab, room)
    grades = f"/v3/classes/{room['class']['id']}/grade-items/{definition['grade_item_id']}/grades"
    command = {'aggregate_id': aggregate['id'], 'submission_revision_id': revision['id'], 'expected_version': 0, 'value_milli': value}
    assert mutate(lab, room['pairs']['teacher'], grades, command).status == 200
    assert mutate(lab, room['pairs']['teacher'], grades+f"/{aggregate['id']}/release", {'expected_version': 1}).status == 200
    return definition, aggregate, revision, grades


def two_items(lab, name):
    room = classroom(lab, name, second_teacher=True)
    first = released(lab, room)
    base = f"/v3/classes/{room['class']['id']}"
    section = mutate(lab, room['pairs']['teacher'], base+'/sections', {'title': 'Second work', 'position': 2}).json()
    activity = mutate(lab, room['pairs']['teacher'], base+'/activities', {'section_id': section['id'], 'activity_type': 'ASSIGNMENT', 'title': 'Second essay', 'position': 1}).json()
    now = datetime.now(timezone.utc)
    revision = mutate(lab, room['pairs']['teacher'], base+f"/activities/{activity['id']}/revisions", {
        'raw_body': 'Write second essay', 'timezone': 'Asia/Jakarta',
        'available_from': (now-timedelta(hours=1)).isoformat(), 'due_at': (now+timedelta(hours=1)).isoformat(),
        'cutoff_at': (now+timedelta(hours=2)).isoformat(),
        'assignment': {'submission_mode': 'TEXT', 'max_attempts': 2, 'max_files': 0, 'max_file_bytes': 0, 'late_policy': 'REJECT_AFTER_CUTOFF'},
    }).json()
    revision = mutate(lab, room['pairs']['teacher'], base+f"/activities/{activity['id']}/revisions/{revision['id']}/publish", {'version': revision['version']}).json()
    second_room = {**room, 'activity': activity, 'revision': revision}
    second = released(lab, second_room, 'Second grade', 82000)
    report = base+f"/reports/grades/{lab['actors']['student']['id']}"
    return room, first, second, report


@pytest.mark.case('M14-CP03-pages')
def test_multiple_pages_sort_filter_and_snapshot(lab):
    room, first, second, report = two_items(lab, 'M14 paging')
    pair = room['pairs']['student']
    initial = mutate(lab, pair, report+'/pages', {'page_size': 1})
    assert initial.status == 200 and initial.headers['cache-control'] == 'no-store'
    a = initial.json()
    assert len(a['grades']) == 1 and a['next_cursor'] and len(a['snapshot']) == 64
    following = mutate(lab, pair, report+'/pages', {'page_size': 1, 'cursor': a['next_cursor']})
    assert following.status == 200
    b = following.json()
    ids = [a['grades'][0]['grade_item_id'], b['grades'][0]['grade_item_id']]
    assert ids == sorted({first[0]['grade_item_id'], second[0]['grade_item_id']})
    assert a['snapshot'] == b['snapshot'] and b['next_cursor'] == ''
    filtered = mutate(lab, pair, report+'/pages', {'page_size': 50, 'sort': 'grade_item_id_asc', 'grade_item_id': first[0]['grade_item_id']})
    assert filtered.status == 200 and [g['grade_item_id'] for g in filtered.json()['grades']] == [first[0]['grade_item_id']]


@pytest.mark.case('M14-CP03-boundary')
def test_cursor_authority_and_invalid_requests(lab):
    room, _, _, report = two_items(lab, 'M14 paging boundary')
    teacher = room['pairs']['teacher']
    first = mutate(lab, teacher, report+'/pages', {'page_size': 1})
    assert first.status == 200
    cursor = first.json()['next_cursor']
    assert mutate(lab, room['pairs']['teacher2'], report+'/pages', {'page_size': 1, 'cursor': cursor}).status == 403
    assert mutate(lab, room['pairs']['outsider'], report+'/pages', {'page_size': 1}).status == 403
    for body in ({'page_size': 51}, {'page_size': 0}, {'page_size': 1, 'sort': 'title'},
                 {'page_size': 1, 'learner_user_id': lab['actors']['outsider']['id']},
                 {'page_size': 1, 'cursor': 'x'+cursor[1:]}, {'page_size': 2, 'cursor': cursor}):
        assert mutate(lab, teacher, report+'/pages', body).status == 400
    assert teacher[0].post(report+'/pages', data={'page_size': 1}).status == 403
    foreign = classroom(lab, 'M14 paging foreign')
    path = f"/v3/classes/{foreign['class']['id']}/reports/grades/{lab['actors']['student']['id']}/pages"
    assert mutate(lab, teacher, path, {'page_size': 1, 'cursor': cursor}).status == 403


@pytest.mark.case('M14-CP03-stale')
def test_release_between_pages_requires_restart(lab):
    room, first, _, report = two_items(lab, 'M14 stale snapshot')
    pair = room['pairs']['student']
    initial = mutate(lab, pair, report+'/pages', {'page_size': 1}).json()
    definition, aggregate, revision, grades = first
    assert mutate(lab, room['pairs']['teacher'], grades, {'aggregate_id': aggregate['id'], 'submission_revision_id': revision['id'], 'expected_version': 1, 'value_milli': 99000, 'reason': 'Reviewed regrade'}).status == 200
    assert mutate(lab, pair, report+'/pages', {'page_size': 1, 'cursor': initial['next_cursor']}).status == 200
    assert mutate(lab, room['pairs']['teacher'], grades+f"/{aggregate['id']}/release", {'expected_version': 2}).status == 200
    stale = mutate(lab, pair, report+'/pages', {'page_size': 1, 'cursor': initial['next_cursor']})
    assert stale.status == 409 and stale.json()['error'] == 'STALE_SNAPSHOT'
    restarted = mutate(lab, pair, report+'/pages', {'page_size': 50})
    assert restarted.status == 200 and restarted.json()['snapshot'] != initial['snapshot']
    assert next(g for g in restarted.json()['grades'] if g['grade_item_id'] == definition['grade_item_id'])['value_milli'] == 99000


@pytest.mark.case('M14-CP03-export')
def test_private_formula_safe_export_preserves_grades(lab):
    room = classroom(lab, 'M14 CSV export')
    released(lab, room, '=HYPERLINK("https://invalid.example")')
    report = f"/v3/classes/{room['class']['id']}/reports/grades/{lab['actors']['student']['id']}"
    before = room['pairs']['student'][0].get(report).json()
    for subject in ('teacher', 'student'):
        response = mutate(lab, room['pairs'][subject], report+'/exports', {'format': 'csv'})
        assert response.status == 200 and response.headers['content-type'].startswith('text/csv')
        assert response.headers['cache-control'] == 'no-store' and 'attachment' in response.headers['content-disposition']
        assert len(response.headers['x-export-audit-id']) == 64 and len(response.headers['x-report-snapshot']) == 64
        rows = list(csv.reader(io.StringIO(response.text())))
        assert len(rows) == 2 and rows[1][3].startswith("'=HYPERLINK")
        assert 'email' not in rows[0] and 'submission' not in rows[0]
    assert room['pairs']['student'][0].get(report).json() == before
    assert mutate(lab, room['pairs']['outsider'], report+'/exports', {'format': 'csv'}).status == 403
    assert mutate(lab, room['pairs']['teacher'], report+'/exports', {'format': 'xlsx'}).status == 400
    assert mutate(lab, room['pairs']['teacher'], report+'/exports', {'format': 'csv', 'learner_user_id': 'forged'}).status == 400
    assert room['pairs']['teacher'][0].post(report+'/exports', data={'format': 'csv'}).status == 403
