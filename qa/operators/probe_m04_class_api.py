"""Playwright checks with a synthetic recipient and the local HTTPS gateway."""

import json
import re
import sys

from playwright.sync_api import sync_playwright


def main():
    fixture = json.load(sys.stdin)
    origin, path, invitation = (fixture[key] for key in ('origin', 'class_path', 'invitation_id'))
    if (not re.fullmatch(r'https://127\.0\.0\.1:[1-9][0-9]{0,4}', origin) or
            not re.fullmatch(r'/v3/classes/[a-f0-9]{32}', path) or not re.fullmatch(r'[a-f0-9]{32}', invitation) or
            any(not re.fullmatch(r'[a-f0-9]{64}', fixture[key]) for key in ('session', 'csrf'))):
        raise ValueError('CLASS_INVITATION_FIXTURE_SCOPE')
    cookie = {'name': '__Host-lms-v3', 'value': fixture['session'], 'domain': '127.0.0.1',
              'path': '/', 'secure': True, 'httpOnly': True, 'sameSite': 'Strict', 'expires': -1}
    with sync_playwright() as playwright:
        request = playwright.request.new_context(base_url=origin, ignore_https_errors=True, timeout=5000,
                                                 storage_state={'cookies': [cookie], 'origins': []})
        try:
            response = request.get('/v3/me/class-invitations')
            try:
                rows = response.json()['invitations']
                if (response.status != 200 or len(rows) != 1 or rows[0] != {
                        'id': invitation, 'class_id': path.rsplit('/', 1)[-1], 'role': 'student', 'status': 'INVITED'}):
                    raise ValueError('CLASS_INVITATION_LIST_MISMATCH')
            finally:
                response.dispose()
            accept = path + '/invitations/' + invitation + '/accept'
            cases = (
                ('GET', path, origin, '', None, 403),
                ('POST', accept, 'https://foreign.invalid', fixture['csrf'], {}, 403),
                ('POST', accept, origin, '', {}, 403),
                ('POST', accept, origin, fixture['csrf'], {'role': 'teacher'}, 400),
            )
            for method, route, observed_origin, csrf, body, status in cases:
                response = request.fetch(route, method=method, data=body, headers={
                    'Origin': observed_origin, 'X-CSRF-Token': csrf, 'Idempotency-Key': 'playwright-class-probe',
                })
                try:
                    if response.status != status:
                        raise ValueError('CLASS_INVITATION_POLICY_MISMATCH')
                finally:
                    response.dispose()
        finally:
            request.dispose()
    print('PASS_M04_CLASS_PLAYWRIGHT_5')


if __name__ == '__main__':
    main()
