"""Playwright request-context smoke checks on the ephemeral HTTPS fixture."""

import json
import re
import sys

from playwright.sync_api import sync_playwright


def main():
    origin = json.load(sys.stdin)['origin']
    if not re.fullmatch(r'https://127\.0\.0\.1:[1-9][0-9]{0,4}', origin):
        raise ValueError('PASSWORD_API_FIXTURE_ORIGIN')
    with sync_playwright() as playwright:
        request = playwright.request.new_context(base_url=origin, ignore_https_errors=True, timeout=5000)
        try:
            cases = (
                ('accounts/invite', origin, {'email': 'probe@example.invalid'}, 401, 'UNAUTHENTICATED'),
                ('accounts/invite', 'https://foreign.invalid', {'email': 'probe@example.invalid'}, 403, 'FORBIDDEN'),
                ('password/login', origin, {'email': 'absent-probe@example.invalid',
                                             'password': 'synthetic password only'}, 401, 'UNAUTHENTICATED'),
                ('password/invite/accept', origin, {'password': 'synthetic password only'}, 400, 'INVALID_INPUT'),
                ('accounts/invite', origin, {'email': 'probe@example.invalid', 'site_admin': True}, 400, 'INVALID_INPUT'),
            )
            for path, observed_origin, body, status, error in cases:
                response = request.post('/v3/auth/' + path, data=body, headers={'Origin': observed_origin})
                try:
                    if response.status != status or response.json() != {'error': error}:
                        raise ValueError('PASSWORD_API_SMOKE_MISMATCH')
                finally:
                    response.dispose()
        finally:
            request.dispose()
    print('PASS_M04_PASSWORD_PLAYWRIGHT_5')


if __name__ == '__main__':
    main()
