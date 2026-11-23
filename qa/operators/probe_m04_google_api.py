"""Playwright API contract smoke for the ephemeral Google gateway."""

import json
import re
import sys
from urllib.parse import parse_qs, urlsplit

from playwright.sync_api import sync_playwright


def main():
    origin = json.load(sys.stdin)['origin']
    if not re.fullmatch(r'https://127\.0\.0\.1:[1-9][0-9]{0,4}', origin):
        raise ValueError('GOOGLE_API_FIXTURE_ORIGIN')
    with sync_playwright() as playwright:
        request = playwright.request.new_context(base_url=origin, ignore_https_errors=True, timeout=5000)
        try:
            cases = (
                ('start', 'https://foreign.invalid', {'mode': 'login'}, 403, 'FORBIDDEN'),
                ('start', origin, {'mode': 'login', 'redirect_uri': 'https://foreign.invalid'}, 400, 'INVALID_INPUT'),
                ('start', origin, {'mode': 'link', 'email': 'unknown@example.invalid'}, 403, 'FORBIDDEN'),
                ('complete', origin, {'state': 'a' * 64, 'code': 'unbound'}, 401, 'UNAUTHENTICATED'),
            )
            for path, observed_origin, body, status, error in cases:
                response = request.post('/v3/auth/google/' + path, data=body, headers={'Origin': observed_origin})
                try:
                    if response.status != status or response.json() != {'error': error}:
                        raise ValueError('GOOGLE_API_SMOKE_MISMATCH')
                finally:
                    response.dispose()
            response = request.post('/v3/auth/google/start', data={'mode': 'login'}, headers={'Origin': origin})
            try:
                url = urlsplit(response.json()['authorization_url'])
                query = parse_qs(url.query)
                cookies = [entry for entry in request.storage_state()['cookies'] if entry['name'] == '__Host-lms-v3-google']
                if (response.status != 200 or response.headers.get('cache-control') != 'no-store' or
                        url.scheme != 'https' or url.netloc != 'accounts.google.com' or
                        query.get('redirect_uri') != [origin + '/auth/google/callback'] or
                        query.get('code_challenge_method') != ['S256'] or 'client_secret' in query or
                        len(cookies) != 1 or not cookies[0]['secure'] or not cookies[0]['httpOnly'] or
                        cookies[0]['sameSite'] != 'Strict'):
                    raise ValueError('GOOGLE_API_FLOW_COOKIE_MISMATCH')
            finally:
                response.dispose()
        finally:
            request.dispose()
    print('PASS_M04_GOOGLE_PLAYWRIGHT_5')


if __name__ == '__main__':
    main()
