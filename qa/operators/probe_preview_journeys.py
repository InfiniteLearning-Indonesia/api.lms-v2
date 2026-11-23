"""Drive three LMS journeys through the actual frontend of an owner-run preview."""

import hashlib
import json
from pathlib import Path
import re
import secrets

from playwright.sync_api import Error as PlaywrightError, expect, sync_playwright

from qa.operators.restore_legacy_baseline import write_new

FLOW_NAMES = ('admin_invitation_class', 'assignment_submission_grade', 'contextual_class_role')
FIELD_LABELS = {
    'classId': 'ID Class',
    'invitationId': 'ID undangan',
    'activityId': 'ID aktivitas',
    'revisionId': 'ID revisi konten',
    'submissionId': 'ID pengumpulan',
    'gradeItemId': 'ID item nilai',
    'definitionId': 'ID versi definisi',
}


class JourneyFailure(Exception):
    """Only put stable, non-secret evidence into the failure description."""


def require(condition, reason):
    if not condition:
        raise JourneyFailure(reason)


def dist_fingerprint(dist):
    dist = Path(dist)
    digest = hashlib.sha256()
    files = [dist / 'index.html', *sorted((dist / 'assets').glob('*'))]
    require(files[0].is_file() and len(files) > 1, 'FE_TEST_BUILD_REQUIRED')
    for file in files:
        if file.is_file():
            digest.update(file.relative_to(dist).as_posix().encode())
            digest.update(b'\x00')
            digest.update(file.read_bytes())
            digest.update(b'\x00')
    return digest.hexdigest()


class Journeys:
    def __init__(self, browser, origin, mailbox, admin_email, password, headless):
        self.browser, self.origin, self.mailbox = browser, origin, mailbox
        self.admin_email, self.password = admin_email, password
        self.headless = headless
        self.sample_password = 'Preview fixture ' + secrets.token_urlsafe(21)
        suffix = secrets.token_hex(5)
        self.mentor_email = f'qa-mentor-{suffix}@preview.invalid'
        self.mentee_email = f'qa-mentee-{suffix}@preview.invalid'
        self.sample_names = (f'QA Class A {suffix}', f'QA Class B {suffix}')
        self.page = None
        self.step = 'setup'
        self.operation = None
        self.ui_stage = None
        self.steps = []

    def page_for(self):
        # The parent supervisor checks readiness with its pinned certificate.
        # Only the verified preview origin is reachable from these contexts.
        context = self.browser.new_context(ignore_https_errors=True, service_workers='block', reduced_motion='reduce')
        context.route('**/*', lambda route: route.continue_() if route.request.url.startswith(self.origin + '/') else route.abort())
        return context.new_page()

    def mark(self, name):
        self.step = name
        self.operation = None
        self.ui_stage = None

    def focus(self, page):
        if not self.headless and self.page is not page:
            page.bring_to_front()
        self.page = page

    def passed(self):
        self.steps.append(self.step)

    def response(self, page, method, path, click, status):
        self.focus(page)
        with page.expect_response(lambda r: r.url == self.origin + path and r.request.method == method, timeout=15000) as waited:
            click()
        response = waited.value
        require(response.status == status, f'EXPECTED_HTTP_{status}_GOT_{response.status}')
        return response.json()

    def login(self, page, email, password, role):
        self.focus(page)
        page.goto(self.origin + '/login', wait_until='domcontentloaded')
        page.get_by_label('Email', exact=True).fill(email)
        page.get_by_label('Kata Sandi', exact=True).fill(password)
        self.response(page, 'POST', '/v3/auth/password/login', page.get_by_role('button', name='Masuk', exact=True).click, 200)
        expect(page.get_by_role('heading', name='Panel uji LMS')).to_be_visible(timeout=15000)
        expect(page.get_by_text(role, exact=True)).to_be_visible(timeout=15000)

    def action(self, page, title, path, method='POST', status=200, body=None, fields=None):
        self.operation, self.ui_stage = title, 'select_action'
        self.focus(page)
        page.get_by_role('searchbox', name=re.compile('Cari di')).fill(title)
        name = re.compile(r'^' + re.escape(title) + r'(?:\s+(?:Lulus|Gagal))?$')
        page.get_by_role('complementary', name='Katalog aksi').get_by_role('button', name=name).click()
        panel = page.get_by_role('region', name='Detail aksi')
        expect(panel.get_by_role('heading', name=title, exact=True)).to_be_visible()
        self.ui_stage = 'fill_parameters'
        for field, value in (fields or {}).items():
            panel.get_by_label(FIELD_LABELS[field], exact=True).fill(value)
        if body is not None:
            self.ui_stage = 'fill_body'
            editor = panel.get_by_role('button', name='Edit body JSON', exact=True)
            if editor.is_visible():
                editor.click()
            panel.locator('#test-request-body').fill(json.dumps(body))
        if status >= 400:
            self.ui_stage = 'set_expected_status'
            panel.get_by_text('Opsi dan uji penolakan', exact=True).click()
            panel.get_by_label('HTTP yang diharapkan').fill(str(status))
        self.ui_stage = 'wait_for_api_response'
        data = self.response(page, method, path, panel.get_by_role('button', name='Jalankan aksi', exact=True).click, status)
        self.ui_stage = 'verify_panel_result'
        expect(panel.get_by_text('Sesuai ekspektasi', exact=True)).to_be_visible()
        return data

    def make_class(self, name):
        return self.action(self.admin, 'Buat Class', '/v3/classes', body={'name': name, 'program_label': 'Preview QA', 'cohort_label': 'synthetic'})

    def invite(self, class_id, email, role):
        action = 'Undang Mentor lewat email' if role == 'teacher' else 'Undang Mentee lewat email'
        data = self.action(self.admin, action, f'/v3/classes/{class_id}/account-invitations',
                           body={'email': email, 'role': role}, fields={'classId': class_id})
        require(data.get('role') == role and data.get('status') == 'INVITED', 'INVITATION_ROLE_OR_STATE')
        return data

    def activate(self, page, email):
        self.focus(page)
        messages = [m for m in self.mailbox() if m['email'] == email and m['purpose'] == 'invite']
        require(len(messages) == 1, 'EXPECTED_ONE_PENDING_ACTIVATION')
        page.goto(self.origin + '/login?view=invite', wait_until='domcontentloaded')
        page.get_by_label('Token dari mailbox').fill(messages[0]['token'])
        page.get_by_label('Kata sandi baru').fill(self.sample_password)
        self.response(page, 'POST', '/v3/auth/password/invite/accept',
                      page.get_by_role('button', name='Simpan kata sandi').click, 200)
        self.login(page, email, self.sample_password, 'Akun aktif')

    def accept(self, page, invitation):
        self.focus(page)
        class_id, invite_id = invitation['class_id'], invitation['id']
        pending = self.action(page, 'Undangan milik akun aktif', '/v3/me/class-invitations', method='GET')
        require(any(i['id'] == invite_id for i in pending['invitations']), 'INVITATION_MISSING_FROM_RECIPIENT')
        result = self.action(page, 'Terima undangan Class', f'/v3/classes/{class_id}/invitations/{invite_id}/accept',
                             body={}, fields={'classId': class_id, 'invitationId': invite_id})
        require(result['status'] == 'ACCEPTED' and result['role'] == invitation['role'], 'ACCEPTANCE_ROLE_OR_STATE')
        after = self.action(page, 'Undangan milik akun aktif', '/v3/me/class-invitations', method='GET')
        require(all(i['id'] != invite_id for i in after['invitations']), 'CONSUMED_INVITATION_STILL_PENDING')

    def publish_class(self, actor, created):
        self.action(actor, 'Publikasikan Class', f"/v3/classes/{created['id']}/publish",
                    fields={'classId': created['id']}, body={'version': created['version']})

    def check_role(self, page, class_id, role):
        visible = self.action(page, 'Daftar Class akun aktif', '/v3/me/classes', method='GET')
        require(any(c['id'] == class_id and c['roles'] == [role] for c in visible['classes']), 'CLASS_ROLE_NOT_VISIBLE')

    def invitations(self):
        self.admin, self.mentor, self.mentee = (self.page_for() for _ in range(3))
        self.mark('admin_login_password_denial')
        self.focus(self.admin)
        self.admin.goto(self.origin + '/login', wait_until='domcontentloaded')
        self.admin.get_by_label('Email', exact=True).fill(self.admin_email)
        self.admin.get_by_label('Kata Sandi', exact=True).fill('Incorrect synthetic password')
        self.response(self.admin, 'POST', '/v3/auth/password/login', self.admin.get_by_role('button', name='Masuk', exact=True).click, 401)
        expect(self.admin.get_by_role('alert')).to_contain_text('tidak valid')
        self.login(self.admin, self.admin_email, self.password, 'Site Admin')
        self.passed()

        self.mark('admin_creates_class_and_invites_both_roles')
        self.class_a = self.make_class(self.sample_names[0])
        class_id = self.class_a['id']
        mentor_invite = self.invite(class_id, self.mentor_email, 'teacher')
        mentee_invite = self.invite(class_id, self.mentee_email, 'student')
        self.passed()

        self.mark('both_recipients_activate_and_accept')
        self.activate(self.mentor, self.mentor_email)
        self.activate(self.mentee, self.mentee_email)
        visible = self.action(self.mentor, 'Daftar Class akun aktif', '/v3/me/classes', method='GET')
        require(all(c['id'] != class_id for c in visible['classes']), 'PENDING_INVITATION_GRANTED_ACCESS')
        self.accept(self.mentor, mentor_invite)
        self.accept(self.mentee, mentee_invite)
        self.publish_class(self.mentor, self.class_a)
        self.check_role(self.mentor, class_id, 'teacher')
        self.check_role(self.mentee, class_id, 'student')
        self.passed()

    def grades(self):
        class_id = self.class_a['id']
        base = f'/v3/classes/{class_id}'
        self.mark('mentor_publishes_assignment')
        fields = {'classId': class_id}
        section = self.action(self.mentor, 'Buat bagian konten', base + '/sections', status=201, fields=fields,
                              body={'title': 'Instruksi QA', 'position': 1})
        activity = self.action(self.mentor, 'Buat aktivitas', base + '/activities', status=201, fields=fields,
                               body={'section_id': section['id'], 'activity_type': 'ASSIGNMENT', 'title': 'Tugas QA', 'position': 1})
        activity_path = base + f"/activities/{activity['id']}"
        fields = {**fields, 'activityId': activity['id']}
        revision = self.action(self.mentor, 'Buat revisi konten', activity_path + '/revisions', status=201, fields=fields,
                               body={'raw_body': '<p>Jawab tugas QA.</p>', 'timezone': 'Asia/Jakarta',
                                     'assignment': {'submission_mode': 'TEXT', 'max_attempts': 2, 'max_files': 0,
                                                    'max_file_bytes': 0, 'late_policy': 'REJECT_AFTER_CUTOFF'}})
        revision_path = activity_path + f"/revisions/{revision['id']}"
        revision_fields = {**fields, 'revisionId': revision['id']}
        self.action(self.mentor, 'Publikasikan revisi', revision_path + '/publish', fields=revision_fields,
                    body={'version': revision['version']})
        self.passed()

        self.mark('mentee_submits_exact_revision')
        submission = self.action(self.mentee, 'Buat pengumpulan', revision_path + '/submissions', fields=revision_fields, body={})
        sub_path = base + f"/submissions/{submission['id']}"
        sub_fields = {'classId': class_id, 'submissionId': submission['id']}
        answer = self.action(self.mentee, 'Tambah revisi jawaban', sub_path + '/revisions', fields=sub_fields,
                             body={'version': submission['version'], 'body': 'Jawaban sintetis untuk QA.', 'file_asset_ids': []})
        receipt = self.action(self.mentee, 'Kumpulkan tugas', sub_path + '/handin', fields=sub_fields,
                              body={'version': submission['version'], 'attempt_version': 1})
        require(receipt['revision_id'] == answer['id'], 'RECEIPT_REVISION_MISMATCH')
        self.passed()

        self.mark('grade_private_until_mentor_releases_it')
        grade = self.action(self.mentor, 'Buat item nilai', base + '/grade-items', status=201, fields=fields,
                            body={'activity_id': activity['id'], 'title': 'Nilai tugas QA', 'weight_bps': 10000,
                                  'scale': {'name': 'Skala 100', 'minimum_milli': 0, 'maximum_milli': 100000, 'display_decimals': 1},
                                  'criteria': [{'key': 'answer', 'title': 'Jawaban', 'weight_bps': 10000,
                                                'maximum_milli': 100000, 'position': 1}]})
        item_path = base + f"/grade-items/{grade['grade_item_id']}"
        grade_fields = {'classId': class_id, 'gradeItemId': grade['grade_item_id']}
        definition_id = grade['definition_version_id']
        self.action(self.mentor, 'Publikasikan versi definisi', item_path + f'/versions/{definition_id}/publish',
                    fields={**grade_fields, 'definitionId': definition_id}, body={'expected_publication_revision': 0})
        scored = self.action(self.mentor, 'Catat nilai', item_path + '/grades', fields=grade_fields,
                             body={'aggregate_id': submission['id'], 'submission_revision_id': answer['id'],
                                   'expected_version': 0, 'value_milli': 85000})
        read = item_path + f"/grades/{submission['id']}"
        read_fields = {**grade_fields, 'submissionId': submission['id']}
        self.action(self.mentee, 'Baca nilai pengumpulan', read, method='GET', status=404, fields=read_fields)
        self.action(self.mentor, 'Rilis nilai', read + '/release', fields=read_fields,
                    body={'expected_version': scored['version']})
        visible = self.action(self.mentee, 'Baca nilai pengumpulan', read, method='GET', fields=read_fields)
        require(visible['value_milli'] == 85000, 'RELEASED_SCORE_MISMATCH')
        self.passed()

        self.mark('released_grade_visible_in_mini_class')
        self.focus(self.mentee)
        self.mentee.goto(self.origin + '/login?view=class&class=' + class_id, wait_until='domcontentloaded')
        expect(self.mentee.get_by_role('heading', name=self.sample_names[0])).to_be_visible()
        self.mentee.get_by_role('button', name='Nilai', exact=True).click()
        released = self.mentee.get_by_role('region', name='Nilai yang dirilis')
        expect(released.get_by_text('Nilai tugas QA', exact=True)).to_be_visible()
        expect(released.get_by_text('85', exact=True)).to_be_visible()
        self.mentee.get_by_role('link', name='Panel uji', exact=True).click()
        expect(self.mentee.get_by_role('heading', name='Panel uji LMS')).to_be_visible()
        self.passed()

    def contextual(self):
        self.mark('same_account_has_two_class_roles')
        # A failure in the grading flow may have left the student on the mini page.
        self.focus(self.mentee)
        self.mentee.goto(self.origin + '/login?view=lab', wait_until='domcontentloaded')
        expect(self.mentee.get_by_role('heading', name='Panel uji LMS')).to_be_visible()
        class_b = self.make_class(self.sample_names[1])
        b = class_b['id']
        student, teacher = self.invite(b, self.mentor_email, 'student'), self.invite(b, self.mentee_email, 'teacher')
        self.accept(self.mentor, student)
        self.accept(self.mentee, teacher)
        self.publish_class(self.mentee, class_b)
        a = self.class_a['id']
        self.check_role(self.mentor, a, 'teacher')
        self.check_role(self.mentor, b, 'student')
        self.passed()

        self.mark('same_action_allowed_only_in_teacher_class')
        name = 'Uji batas Class'
        self.action(self.mentor, 'Buat bagian konten', f'/v3/classes/{a}/sections', status=201, fields={'classId': a},
                    body={'title': name, 'position': 2})
        self.action(self.mentor, 'Buat bagian konten', f'/v3/classes/{b}/sections', status=403, fields={'classId': b},
                    body={'title': name, 'position': 2})
        content = self.action(self.mentor, 'Daftar konten', f'/v3/classes/{b}/content', method='GET', fields={'classId': b})
        require(not any(s['title'] == name for s in content['sections']), 'DENIED_WRITE_MUTATED_CLASS')
        self.passed()

        self.mark('mini_class_list_shows_contextual_roles')
        self.focus(self.mentor)
        self.mentor.get_by_role('link', name='Daftar Class', exact=True).click()
        expect(self.mentor.get_by_role('heading', name='Daftar Class', exact=True)).to_be_visible()
        for name, role in zip(self.sample_names, ('Mentor', 'Mentee')):
            card = self.mentor.get_by_role('link', name=re.compile(re.escape(name)))
            expect(card.get_by_text(role, exact=True)).to_be_visible()
        self.passed()


def run_browser_journeys(origin, mailbox, admin_email, admin_password, root, dist, *, headless=False):
    require(re.fullmatch(r'https://127\.0\.0\.1:[0-9]{4,5}', origin), 'EXPECTED_OWNER_LOOPBACK_PREVIEW')
    destination = Path(root) / ('journeys-' + secrets.token_hex(6))
    destination.mkdir()
    report = {'scope': 'OWNER_LOCAL_SYNTHETIC_BROWSER', 'browser_mode': 'headless' if headless else 'visible',
              'flows': [], 'status': 'BLOCKED_BROWSER_STARTUP'}
    try:
        report['fe_build_sha256'] = dist_fingerprint(dist)
        with sync_playwright() as playwright:
            browser = playwright.chromium.launch(channel='chrome', headless=headless, slow_mo=0 if headless else 250)
            try:
                test = Journeys(browser, origin, mailbox, admin_email, admin_password, headless)
                for index, (name, action) in enumerate(zip(FLOW_NAMES, (test.invitations, test.grades, test.contextual))):
                    if index and report['flows'][0]['status'] != 'PASS':
                        report['flows'].append({'id': name, 'status': 'BLOCKED_BY_INVITATION_FLOW'})
                        continue
                    test.steps = []
                    test.mark('prepare')
                    print(f'[fe-qa] {name}', flush=True)
                    try:
                        action()
                        report['flows'].append({'id': name, 'status': 'PASS', 'steps': test.steps})
                    except (JourneyFailure, PlaywrightError, AssertionError, KeyError, ValueError) as error:
                        result = {'id': name, 'status': 'FAIL', 'failed_step': test.step, 'steps': test.steps,
                                  'reason': str(error) if isinstance(error, JourneyFailure) else type(error).__name__}
                        if test.operation:
                            result.update(operation=test.operation, ui_stage=test.ui_stage)
                        report['flows'].append(result)
                        if test.page and '/login?view=lab' in test.page.url:
                            try:
                                filename = name + '.png'
                                test.page.screenshot(path=str(destination / filename), timeout=5000, animations='disabled')
                                result['screenshot'] = filename
                            except PlaywrightError:
                                pass
                report['status'] = 'PASS_FE_TEST_JOURNEYS' if all(f['status'] == 'PASS' for f in report['flows']) else 'FAIL_FE_TEST_JOURNEYS'
                require(report['fe_build_sha256'] == dist_fingerprint(dist), 'FE_TEST_BUILD_CHANGED_DURING_QA')
            finally:
                browser.close()
    except (JourneyFailure, OSError, PlaywrightError) as error:
        if report['flows']:
            report['status'] = 'FAIL_FE_TEST_JOURNEYS'
        report['reason'] = str(error) if isinstance(error, JourneyFailure) else type(error).__name__
    for flow in report['flows']:
        print(f"[fe-qa] {flow['id']} {flow['status']}" +
              (f" at {flow['failed_step']} ({flow['reason']})" if flow.get('failed_step') else '') +
              (f" {flow['operation']} / {flow['ui_stage']}" if flow.get('operation') else ''), flush=True)
    write_new(destination / 'summary.json', json.dumps(report, indent=2).encode())
    print(json.dumps({'status': report['status'], 'report': str(destination / 'summary.json'),
                      'passed': sum(f['status'] == 'PASS' for f in report['flows']), 'expected': len(FLOW_NAMES)}), flush=True)
    return report
