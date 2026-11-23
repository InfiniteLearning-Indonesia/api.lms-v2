"""Offline safety regressions; no staging, database, real keys or R2 access."""
import contextlib
import io
import json
from pathlib import Path
import unittest
from unittest.mock import patch

from qa.operators import freeze_staging_baseline as baseline


class FreezeBaselineTests(unittest.TestCase):
    def config(self, extra=''):
        return ('DB_HOST=fixture.invalid\nDB_PORT=5432\nDB_USER=fixture\n'
                'DB_USERNAME=fixture\nDB_NAME=fixture\nDB_PASSWORD="fake-only"\n'
                'DB_SSL=true\n' + extra)

    def observation(self):
        return {'client_tls': True, 'server_observed_tls': False,
                'read_only': True, 'server_version': 170006,
                'database_bytes': 15 * baseline.MIB, 'cross_schema_fk_count': 0,
                'public_tables': [{'name': 'users', 'kind': 'r', 'can_select': True}]}

    def test_only_database_keys_loaded(self):
        values = baseline.parse_config(self.config('R2_SECRET_ACCESS_KEY=fake-r2\nSMTP_PASS=fake-smtp\n'))
        self.assertEqual(values['DB_PASSWORD'], 'fake-only')
        self.assertNotIn('R2_SECRET_ACCESS_KEY', values)
        self.assertNotIn('SMTP_PASS', values)

    def test_conflicting_aliases_fail_without_values(self):
        with self.assertRaisesRegex(baseline.Stop, '^CONFLICTING_DATABASE_CONFIG_ALIASES$'):
            baseline.parse_config(self.config().replace('DB_USERNAME=fixture', 'DB_USERNAME=fake-private'))

    def test_duplicate_database_key_rejected(self):
        with self.assertRaisesRegex(baseline.Stop, '^DUPLICATE_DATABASE_CONFIG_KEY$'):
            baseline.parse_config(self.config('DB_PASSWORD=fake-second\n'))

    def test_tls_disabled_rejected(self):
        with self.assertRaisesRegex(baseline.Stop, '^TLS_REQUIRED$'):
            baseline.parse_config(self.config().replace('DB_SSL=true', 'DB_SSL=false'))

    def test_libpq_overrides_cannot_redirect_connection_and_proxies_preserved(self):
        values = baseline.parse_config(self.config())
        env = baseline.pg_environment(values, Path('fixture-ca.crt'),
                                      {'PGSERVICE': 'wrong', 'PGHOSTADDR': 'wrong',
                                       'PGOPTIONS': 'wrong', 'HTTPS_PROXY': 'fixture-proxy', 'PATH': 'fixture-path'})
        self.assertNotIn('PGSERVICE', env)
        self.assertNotIn('PGHOSTADDR', env)
        self.assertEqual(env['PGHOST'], 'fixture.invalid')
        self.assertEqual(env['PGSSLMODE'], 'verify-full')
        self.assertEqual(env['PGSSLMINPROTOCOLVERSION'], 'TLSv1.2')
        self.assertEqual(env['PGGSSENCMODE'], 'disable')
        self.assertEqual(env['LC_ALL'], 'C')
        self.assertIn('default_transaction_read_only=on', env['PGOPTIONS'])
        self.assertEqual(env['HTTPS_PROXY'], 'fixture-proxy')
        self.assertEqual(env['PATH'], 'fixture-path')

    def test_observation_requires_tls_and_readonly(self):
        for field, reason in [('client_tls', 'CLIENT_TLS_NOT_VERIFIED'),
                              ('read_only', 'READ_ONLY_TRANSACTION_NOT_VERIFIED')]:
            obs = self.observation()
            obs[field] = False
            with self.subTest(field=field), self.assertRaisesRegex(baseline.Stop, '^' + reason + '$'):
                baseline.validate_observation(obs, 10 * baseline.GIB)

    def test_client_tls_survives_backend_hop_without_ssl(self):
        value = self.observation()
        value.pop('client_tls')
        raw = (json.dumps(value) + '\nDatabase|fixture-private-db\n'
               'SSL Connection|true\nSSL Protocol|TLSv1.3\n').encode()
        observed = baseline.parse_preflight_output(raw)
        baseline.validate_observation(observed, 10 * baseline.GIB)
        self.assertIs(observed['server_observed_tls'], False)
        self.assertIs(observed['client_tls'], True)
        self.assertNotIn('fixture-private-db', json.dumps(observed))

    def test_backend_ssl_cannot_replace_missing_client_tls(self):
        for info in ['', '\nSSL Connection|false\nSSL Protocol|TLSv1.3\n']:
            raw = (json.dumps({'server_observed_tls': True, 'read_only': True}) + info).encode()
            with self.subTest(info=info), self.assertRaisesRegex(baseline.Stop, '^CLIENT_TLS_NOT_VERIFIED$'):
                baseline.parse_preflight_output(raw)

    def test_weak_or_missing_client_tls_protocol_rejected(self):
        for info in ['', 'SSL Protocol|TLSv1.0\n']:
            raw = ('{}\nSSL Connection|true\n' + info).encode()
            with self.subTest(info=info), self.assertRaisesRegex(baseline.Stop, '^CLIENT_TLS_PROTOCOL_NOT_VERIFIED$'):
                baseline.parse_preflight_output(raw)

    def test_duplicate_transport_evidence_rejected(self):
        raw = b'{}\nSSL Connection|true\nSSL Protocol|TLSv1.3\nSSL Connection|false\n'
        with self.assertRaisesRegex(baseline.Stop, '^AMBIGUOUS_CLIENT_TLS_EVIDENCE$'):
            baseline.parse_preflight_output(raw)

    def test_invalid_probe_json_is_safe_failure(self):
        for raw in [b'not-json fake-password', b'[]\nSSL Connection|true\nSSL Protocol|TLSv1.3\n']:
            with self.subTest(raw=raw), self.assertRaisesRegex(baseline.Stop, '^PREFLIGHT_OUTPUT_INVALID$'):
                baseline.parse_preflight_output(raw)

    def test_client_status_and_readonly_must_be_boolean_true(self):
        for field in ['client_tls', 'read_only']:
            obs = self.observation()
            obs[field] = 'true'
            with self.subTest(field=field), self.assertRaises(baseline.Stop):
                baseline.validate_observation(obs, 10 * baseline.GIB)

    def test_metadata_and_conninfo_share_one_client_process(self):
        args = baseline.probe_args(Path('fixture-bin'))
        self.assertEqual(args.count('--command'), 2)
        self.assertEqual(args[-1], r'\conninfo')
        self.assertIn('READ ONLY', args[args.index('--command') + 1])

    def test_capacity_floor_and_database_ceiling(self):
        with self.assertRaisesRegex(baseline.Stop, '^INSUFFICIENT_DISK_FOR_BASELINE$'):
            baseline.validate_observation(self.observation(), baseline.FREE_FLOOR)
        obs = self.observation()
        obs['database_bytes'] = baseline.MAX_DATABASE + 1
        with self.assertRaisesRegex(baseline.Stop, '^SOURCE_SIZE_OUTSIDE_BOUNDED_EXPORT_SCOPE$'):
            baseline.validate_observation(obs, 100 * baseline.GIB)

    def test_cross_schema_or_unreadable_table_requires_review(self):
        obs = self.observation()
        obs['cross_schema_fk_count'] = 1
        with self.assertRaisesRegex(baseline.Stop, '^CROSS_SCHEMA_FOREIGN_KEY_REQUIRES_SCOPE_REVIEW$'):
            baseline.validate_observation(obs, 10 * baseline.GIB)
        obs = self.observation()
        obs['public_tables'][0]['can_select'] = False
        with self.assertRaisesRegex(baseline.Stop, '^TABLE_PERMISSION_OR_PARTITION_REQUIRES_REVIEW$'):
            baseline.validate_observation(obs, 10 * baseline.GIB)

    def test_one_snapshot_archive_scope_and_no_connection_secrets_in_args(self):
        args = baseline.custom_args(Path('fixture-bin'))
        self.assertIn('--format=custom', args)
        self.assertIn('--schema=public', args)
        self.assertIn('--no-password', args)
        self.assertIn('--no-privileges', args)
        self.assertFalse(any('postgres://' in a or 'fake-only' in a for a in args))
        self.assertFalse(any(a in args for a in ['--clean', '--create', '--enable-row-security']))

    def test_archive_table_accounting_rejects_missing_or_duplicate(self):
        toc = '12; 0 100 TABLE DATA public users fixture\n'
        self.assertEqual(baseline.verify_toc(toc, self.observation()), 1)
        for bad in ['', toc + toc, '12; 0 100 TABLE DATA public other fixture\n']:
            with self.subTest(toc=bad), self.assertRaisesRegex(baseline.Stop, '^ARCHIVE_TABLE_SCOPE_MISMATCH$'):
                baseline.verify_toc(bad, self.observation())

    def test_errors_are_fixed_codes_without_secret_diagnostics(self):
        raw = b'password authentication failed for fake-user fake-password'
        self.assertEqual(baseline.reason_from_stderr(raw, 'FAILED'), 'DATABASE_AUTHENTICATION_FAILED')
        self.assertEqual(baseline.reason_from_stderr(b'private detail', 'FAILED'), 'FAILED')

    def test_private_destination_failure_stops_before_credentials_and_connection(self):
        output = io.StringIO()
        with patch.object(Path, 'mkdir'), \
             patch.object(baseline, 'private_mkdir', side_effect=PermissionError()), \
             patch.object(baseline, 'write_new', side_effect=PermissionError()), \
             patch.object(baseline, 'read_config') as read_config, \
             patch.object(baseline, 'captured') as captured, \
             contextlib.redirect_stdout(output):
            status = baseline.run(service=Path('fixture-service'))
        self.assertEqual(status, 2)
        read_config.assert_not_called()
        captured.assert_not_called()
        self.assertIn('BLOCKED_BEFORE_EXPORT', output.getvalue())
        self.assertIn('"database_rows_exported": false', output.getvalue())


if __name__ == '__main__':
    unittest.main()
