"""Exercise the installed cmd/api preview through its public HTTPS contract."""

from playwright.sync_api import sync_playwright

from qa.operators.restore_legacy_baseline import Stop


def smoke(origin, password, mailbox):
    cases = []
    with sync_playwright() as playwright:
        contexts = []

        def browser():
            # The supervisor already authenticated this loopback server against
            # the certificate generated for this run via /health/ready.
            request = playwright.request.new_context(base_url=origin, ignore_https_errors=True, timeout=8000)
            contexts.append(request)
            return request

        def call(request, method, route, expected, label, *, body=None, csrf='', key='', request_origin=origin, html=False):
            response = request.fetch(route, method=method, data=body, headers={
                'Origin': request_origin, 'X-CSRF-Token': csrf, 'Idempotency-Key': key,
            })
            try:
                if response.status != expected:
                    raise Stop(f'PREVIEW_{label}_{response.status}_EXPECTED_{expected}')
                if html:
                    if 'LMS preview fixture' not in response.text() or response.headers.get('referrer-policy') != 'no-referrer':
                        raise Stop('PREVIEW_WEB_CONTRACT')
                    return None
                return response.json()
            finally:
                response.dispose()

        def post(request, route, body, label, status=200, csrf='', key=''):
            return call(request, 'POST', route, status, label, body=body, csrf=csrf, key=key)

        def get(request, route, label, status=200, **kwargs):
            return call(request, 'GET', route, status, label, **kwargs)

        def verify(value, label):
            if not value:
                raise Stop('PREVIEW_' + label)

        def activate(email, label):
            request = browser()
            matches = [m for m in mailbox() if m['email'] == email and m['purpose'] == 'invite']
            verify(len(matches) == 1, label + '_MAILBOX')
            post(request, '/v3/auth/password/invite/accept', {'token': matches[0]['token'], 'password': password}, label + '_ACTIVATE')
            signed = post(request, '/v3/auth/password/login', {'email': email, 'password': password}, label + '_LOGIN')
            return request, signed['csrf_token']

        try:
            admin, guest = browser(), browser()
            get(guest, '/', 'WEB', html=True)
            get(guest, '/auth/google/callback?code=fixture&state=fixture', 'CALLBACK_LANDING', html=True)
            get(guest, '/v3/me/classes', 'GUEST_BLOCK', 401)
            cases.append('https_static_same_origin_and_api_boundary')

            csrf = post(admin, '/v3/auth/password/login', {'email': 'admin@preview.invalid', 'password': password}, 'ADMIN_LOGIN')['csrf_token']
            verify(get(admin, '/v3/auth/me', 'ADMIN_ME')['site_admin'] is True, 'ADMIN_ROLE')
            get(admin, '/v3/me/classes', 'ADMIN_CLASSES')
            cases.append('password_bootstrap_admin')

            course = post(admin, '/v3/classes', {'name': 'Synthetic Class A', 'cohort_label': '01'}, 'CREATE_CLASS', csrf=csrf, key='preview-class-a')
            foreign = post(admin, '/v3/classes', {'name': 'Synthetic Class B'}, 'CREATE_FOREIGN', csrf=csrf, key='preview-class-b')
            a, b = ('/v3/classes/' + c['id'] for c in (course, foreign))
            mentor_email = 'mentor@preview.invalid'
            mentee_email = 'mentee@preview.invalid'
            mt = post(admin, a + '/account-invitations', {'email': mentor_email, 'role': 'teacher'}, 'INVITE_MENTOR', csrf=csrf, key='preview-invite-mt')
            me = post(admin, a + '/account-invitations', {'email': mentee_email, 'role': 'student'}, 'INVITE_MENTEE', csrf=csrf, key='preview-invite-me')
            cases.append('class_and_account_invitations')

            mentor, mtcsrf = activate(mentor_email, 'MENTOR')
            mentee, mecsrf = activate(mentee_email, 'MENTEE')
            verify(get(mentor, '/v3/me/classes', 'MENTOR_PRE_ACCEPT')['classes'] == [], 'PRE_ACCEPT_MEMBERSHIP')
            verify([i['id'] for i in get(mentee, '/v3/me/class-invitations', 'MENTEE_PENDING')['invitations']] == [me['id']], 'PENDING_SCOPING')
            cases.append('private_mailbox_activation_and_pending_scoping')

            post(mentee, a + '/invitations/' + mt['id'] + '/accept', {}, 'WRONG_RECIPIENT', 403, mecsrf, 'preview-wrong-account')
            post(mentor, a + '/invitations/' + mt['id'] + '/accept', {}, 'ACCEPT_MENTOR', csrf=mtcsrf, key='preview-accept-mt')
            post(mentee, a + '/invitations/' + me['id'] + '/accept', {}, 'ACCEPT_MENTEE', csrf=mecsrf, key='preview-accept-me')
            post(mentor, a + '/publish', {'version': course['version']}, 'PUBLISH_CLASS', csrf=mtcsrf, key='preview-publish-a')
            visible = get(mentee, '/v3/me/classes', 'MENTEE_CLASSES')['classes']
            verify([(item['id'], item['roles']) for item in visible] == [(course['id'], ['student'])], 'CLASS_LIST_SCOPING')
            get(mentor, b, 'FOREIGN_TEACHER_DENIED', 403)
            cases.append('class_acceptance_publish_and_scope')

            get(mentee, a + '/content', 'STUDENT_CONTENT')
            post(mentee, a + '/sections', {'title': 'Rejected', 'position': 1}, 'STUDENT_EDIT_DENIED', 403, mecsrf, 'preview-student-denied')
            post(mentor, a + '/sections', {'title': 'Synthetic section', 'position': 1}, 'MENTOR_EDIT', 201, mtcsrf, 'preview-mentor-edit')
            verify(len(get(mentee, a + '/content', 'STUDENT_CONTENT_AFTER_EDIT')['sections']) == 1, 'CONTENT_VISIBLE')
            get(mentor, b + '/content', 'FOREIGN_CONTENT_DENIED', 403)
            cases.append('existing_content_handler_with_class_authority')

            post(mentor, '/v3/auth/logout', {}, 'MENTOR_LOGOUT', csrf=mtcsrf)
            get(mentor, '/v3/me/classes', 'LOGOUT_ACCESS_DENIED', 401)
            cases.append('cookie_csrf_and_logout')
        finally:
            for request in contexts:
                request.dispose()
    return cases
