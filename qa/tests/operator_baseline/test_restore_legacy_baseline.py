"""Offline restore guard/parser regressions. Never accesses real input or DB."""
import contextlib
import io
import json
import subprocess
import socket
import struct
from pathlib import Path
import unittest
from unittest.mock import patch

from qa.operators import restore_legacy_baseline as baseline
from qa.operators import windows_local_state as windows_state


class RestoreLegacyBaselineTests(unittest.TestCase):
    def schema(self, sql):
        return ('-- generated fixture\n\\restrict fixtureKEY123\n' + sql +
                '\n\\unrestrict fixtureKEY123\n').encode()

    def test_schema_lexer_preserves_semicolons_comments_and_spaces_inside_literals(self):
        result = baseline.statements(self.schema("COMMENT ON SCHEMA public IS 'a; b -- c';\nSET lock_timeout = 0;"))
        self.assertEqual(result, ["COMMENT ON SCHEMA public IS 'a; b -- c'", 'SET lock_timeout = 0'])

    def test_schema_restrict_pair_must_match(self):
        raw = self.schema('SET lock_timeout = 0;').replace(b'unrestrict fixtureKEY123', b'unrestrict otherKEY456')
        with self.assertRaisesRegex(baseline.Stop, '^INVALID_RESTRICT_DIRECTIVE_PAIR$'):
            baseline.statements(raw)

    def test_other_psql_commands_never_accepted(self):
        with self.assertRaisesRegex(baseline.Stop, '^UNREVIEWED_PSQL_DIRECTIVE$'):
            baseline.statements(self.schema('\\! fake-command\n'))

    def test_programmatic_schema_never_accepted(self):
        for command in ["CREATE FUNCTION public.f() RETURNS int LANGUAGE sql AS 'SELECT 1';",
                        'DO $$ BEGIN END $$;', 'CREATE FOREIGN TABLE public.f(id int) SERVER foreignserver;']:
            with self.subTest(command=command), self.assertRaises(baseline.Stop):
                baseline.review_schema(self.schema(command))

    def test_unterminated_schema_statement_rejected(self):
        with self.assertRaisesRegex(baseline.Stop, '^INCOMPLETE_SCHEMA_SQL$'):
            baseline.statements(self.schema("SELECT 'unfinished"))

    def test_sql_field_splitting_tracks_parentheses_and_literals(self):
        value = "id uuid DEFAULT extensions.uuid_generate_v4(), tags jsonb DEFAULT '[1,2]'::jsonb, amount numeric(8,2)"
        result = baseline.fields(value)
        self.assertEqual(len(result), 3)
        self.assertIn("'[1,2]'", result[1])

    def test_source_copy_ledger_preserves_null_literal_backslash_and_duplicates(self):
        raw = b'COPY public.example (id, "someValue") FROM stdin;\n1\t\\N\n2\ttext\\tvalue\n1\t\\N\n\\.\n'
        ledger = baseline.copy_ledger(raw)
        self.assertEqual(ledger['example']['columns'], ['id', 'someValue'])
        self.assertEqual(ledger['example']['rows'], 3)
        self.assertEqual(ledger['example']['copy_multiset_sha256'], baseline.rows_hash([b'1\t\\N', b'2\ttext\\tvalue', b'1\t\\N']))

    def test_row_multiset_checks_duplicates_and_ignores_physical_order(self):
        self.assertEqual(baseline.rows_hash([b'a', b'b']), baseline.rows_hash([b'b', b'a']))
        self.assertNotEqual(baseline.rows_hash([b'a', b'b']), baseline.rows_hash([b'a', b'a', b'b']))

    def test_incomplete_duplicate_or_malformed_copy_rejected(self):
        examples = [b'COPY public.example (id) FROM stdin;\n1\n',
                    b'COPY public.example (id) FROM stdin;\n1\t2\n\\.\n',
                    b'COPY public.example (id) FROM stdin;\n\\.\nCOPY public.example (id) FROM stdin;\n\\.\n']
        for raw in examples:
            with self.subTest(raw=raw), self.assertRaises(baseline.Stop):
                baseline.copy_ledger(raw)

    def test_toc_skips_only_existing_public_metadata_and_preserves_data(self):
        raw = b'11; 2615 2200 SCHEMA - public fixture\n12; 0 0 COMMENT - SCHEMA public fixture\n13; 0 123 TABLE DATA public users fixture\n'
        selected, count = baseline.filtered_toc(raw)
        self.assertEqual(count, 2)
        self.assertIn(b'\n13; 0 123 TABLE DATA public users fixture\n', selected)
        self.assertEqual(len([line for line in selected.splitlines() if line and not line.startswith(b';')]), 1)

    def test_incomplete_public_schema_metadata_pair_rejected(self):
        with self.assertRaisesRegex(baseline.Stop, '^PUBLIC_SCHEMA_TOC_PAIR_MISMATCH$'):
            baseline.filtered_toc(b'11; 2615 2200 SCHEMA - public fixture\n')

    def test_restore_selects_fixed_database_explicitly_and_never_cleans(self):
        args = baseline.restore_args('legacy_frozen', Path('fixture.list'), Path('fixture.dump'))
        self.assertIn('--dbname=legacy_frozen', args)
        self.assertIn('--single-transaction', args)
        self.assertIn('--exit-on-error', args)
        self.assertNotIn('--clean', args)
        self.assertNotIn('--create', args)
        with self.assertRaisesRegex(baseline.Stop, '^LOCAL_DATABASE_NOT_ALLOWED$'):
            baseline.restore_args('postgres', Path('fixture.list'), Path('fixture.dump'))

    def test_local_environment_cannot_inherit_remote_server_or_service(self):
        inherited = {'PGHOST': 'remote.invalid', 'PGSERVICE': 'remote', 'PGHOSTADDR': 'remote',
                     'PGPASSWORD': 'fake-source-secret', 'HTTPS_PROXY': 'fixture-proxy'}
        env = baseline.local_environment(inherited, 12345, 'fake-new-local-secret', 'fixture-role', 'legacy_frozen')
        self.assertEqual(env['PGHOST'], '127.0.0.1')
        self.assertEqual(env['PGPORT'], '12345')
        self.assertEqual(env['PGPASSWORD'], 'fake-new-local-secret')
        self.assertEqual(env['HTTPS_PROXY'], 'fixture-proxy')
        self.assertNotIn('PGSERVICE', env)
        self.assertNotIn('PGHOSTADDR', env)
        with self.assertRaisesRegex(baseline.Stop, '^LOCAL_TARGET_OUTSIDE_FIXED_SCOPE$'):
            baseline.local_environment(inherited, 12345, 'fake', 'fixture-role', 'staging')

    def test_acceptance_requires_exact_cases_including_stop(self):
        self.assertTrue(baseline.complete_cases(list(baseline.CASES)))
        self.assertFalse(baseline.complete_cases(list(baseline.CASES[:-1])))
        self.assertFalse(baseline.complete_cases([baseline.CASES[0]] * len(baseline.CASES)))
        manifest = json.loads((baseline.SERVICE / 'qa/manifests/REAL-LEGACY-LOCAL-RESTORE.json').read_text(encoding='utf-8'))
        baseline.validate_manifest(manifest)
        manifest['cases'] = manifest['cases'][:-1]
        with self.assertRaisesRegex(baseline.Stop, '^REAL_BASELINE_MANIFEST_MISMATCH$'):
            baseline.validate_manifest(manifest)

    def test_private_output_failure_prevents_input_recovery_and_cluster_creation(self):
        output = io.StringIO()
        with patch.object(baseline, 'private_mkdir', side_effect=PermissionError()), \
             patch.object(baseline, 'write_new', side_effect=PermissionError()), \
             patch.object(baseline, 'bounded', return_value=0), \
             patch.object(baseline, 'acquire_tools') as tools, \
             patch.object(baseline, 'Cluster') as cluster, contextlib.redirect_stdout(output):
            status = baseline.run()
        self.assertEqual(status, 2)
        tools.assert_not_called()
        cluster.assert_not_called()
        self.assertIn('BLOCKED_BEFORE_RESTORE', output.getvalue())
        self.assertIn('"actual_restore_verified": false', output.getvalue())

    def test_control_process_uses_regular_files_without_inherited_pipe_wait(self):
        class MemoryFile(io.BytesIO):
            def __exit__(self, *args):
                return False
        class Destination:
            def __init__(self):
                self.stream = MemoryFile()
            def open(self, mode):
                if mode != 'xb+':
                    raise AssertionError('exclusive file output required')
                return self.stream
        out, error = Destination(), Destination()
        def native(args, *, env, cwd, stdin, stdout, stderr, timeout):
            self.assertEqual(stdin, subprocess.DEVNULL)
            self.assertIs(stdout, out.stream)
            self.assertIs(stderr, error.stream)
            self.assertEqual(timeout, 20)
            stdout.write(b'waiting for server to start... done\n')
            return subprocess.CompletedProcess(args, 0)
        with patch.object(baseline.subprocess, 'run', side_effect=native):
            result = baseline.control_process(['fixture-pgctl'], {}, Path('fixture'), 20, out, error)
        self.assertEqual(result.returncode, 0)
        self.assertEqual(result.stdout, b'waiting for server to start... done\n')

    def test_control_process_timeout_propagates_without_capture_output_retry(self):
        class MemoryFile(io.BytesIO):
            def __exit__(self, *args):
                return False
        class Destination:
            def open(self, mode):
                return MemoryFile()
        with patch.object(baseline.subprocess, 'run', side_effect=subprocess.TimeoutExpired('fixture', 1)) as native:
            with self.assertRaises(subprocess.TimeoutExpired):
                baseline.control_process(['fixture'], {}, Path('fixture'), 1, Destination(), Destination())
        self.assertEqual(native.call_count, 1)
        self.assertNotIn('capture_output', native.call_args.kwargs)

    def test_prior_live_cluster_blocks_another_clone_without_stopping_processes(self):
        parent, current, prior = Path('fixture-parent'), Path('fixture-current'), Path('fixture-prior')
        pid = prior / 'postgres/postmaster.pid'
        fake = '123\n' + str((prior / 'postgres').resolve()) + '\n0\n12345\n'
        with patch.object(Path, 'glob', return_value=[prior]), patch.object(Path, 'is_dir', return_value=True), \
             patch.object(Path, 'is_file', return_value=True), patch.object(Path, 'read_text', return_value=fake), \
             patch.object(baseline.storage, 'reparse', return_value=False), \
             patch.object(baseline, 'cluster_process_and_listener_state',
                          return_value={'recorded_process_exists': True, 'listener_pids': [123], 'probe': 'fixture-kernel'}) as inspect:
            with self.assertRaisesRegex(baseline.Stop, '^PRIOR_LOCAL_CLUSTER_STILL_LISTENING_STOP_OWNER_RUN_FIRST$'):
                baseline.prior_cluster_activity(parent, current)
        inspect.assert_called_once_with(123, 12345)

    def test_stale_prior_pid_is_preserved_without_start_stop_or_cleanup(self):
        prior = Path('fixture-prior')
        fake = '123\n' + str((prior / 'postgres').resolve()) + '\n0\n12345\n'
        with patch.object(Path, 'glob', return_value=[prior]), patch.object(Path, 'is_dir', return_value=True), \
             patch.object(Path, 'is_file', return_value=True), patch.object(Path, 'read_text', return_value=fake), \
             patch.object(baseline.storage, 'reparse', return_value=False), \
             patch.object(baseline, 'cluster_process_and_listener_state',
                          return_value={'recorded_process_exists': False, 'listener_pids': [], 'probe': 'fixture-kernel'}):
            records = baseline.prior_cluster_activity(Path('fixture-parent'), Path('fixture-current'))
        self.assertEqual(records, [{'run_id': prior.name, 'status': 'STALE_PID_NO_PROCESS_NO_LISTENER_PRESERVED',
                                   'probe': 'fixture-kernel', 'clean_shutdown_verified': False}])

    def test_tcp_listener_table_uses_network_order_port(self):
        raw = struct.pack('<I', 2) + struct.pack('<IIIIII', 2, 0, socket.htons(12345), 0, 0, 777) + \
              struct.pack('<IIIIII', 2, 0, socket.htons(12345), 0, 0, 888)
        self.assertEqual(windows_state.listener_pids_from_table(raw, 12345), [777, 888])
        self.assertEqual(windows_state.listener_pids_from_table(raw, 5432), [])

    def test_partial_or_nonlistener_kernel_table_never_means_absent(self):
        for raw in [b'', struct.pack('<I', 1), struct.pack('<I', 999999),
                    struct.pack('<I', 1) + struct.pack('<IIIIII', 5, 0, socket.htons(12345), 0, 0, 777)]:
            with self.subTest(raw=raw), self.assertRaises(baseline.Stop):
                windows_state.listener_pids_from_table(raw, 12345)

    def test_process_evidence_checks_both_sides_of_listener_snapshot(self):
        for snapshots in [({123}, set()), (set(), {123}), ({123}, {123})]:
            with self.subTest(snapshots=snapshots), \
                 patch.object(windows_state, 'windows_process_ids', side_effect=snapshots), \
                 patch.object(windows_state, 'windows_listener_pids', return_value=[]):
                result = windows_state.cluster_process_and_listener_state(123, 12345)
            self.assertIs(result['recorded_process_exists'], True)

    def test_process_enumeration_failure_is_not_inactive_evidence(self):
        with patch.object(windows_state, 'windows_process_ids', side_effect=baseline.Stop('WINDOWS_PROCESS_SNAPSHOT_UNAVAILABLE')), \
             patch.object(windows_state, 'windows_listener_pids') as listeners:
            with self.assertRaisesRegex(baseline.Stop, '^WINDOWS_PROCESS_SNAPSHOT_UNAVAILABLE$'):
                windows_state.cluster_process_and_listener_state(123, 12345)
        listeners.assert_not_called()

    def test_prior_live_process_without_listener_still_blocks_clone(self):
        prior = Path('fixture-prior')
        fake = '123\n' + str((prior / 'postgres').resolve()) + '\n0\n12345\n'
        with patch.object(Path, 'glob', return_value=[prior]), patch.object(Path, 'is_dir', return_value=True), \
             patch.object(Path, 'is_file', return_value=True), patch.object(Path, 'read_text', return_value=fake), \
             patch.object(baseline.storage, 'reparse', return_value=False), \
             patch.object(baseline, 'cluster_process_and_listener_state',
                          return_value={'recorded_process_exists': True, 'listener_pids': [], 'probe': 'fixture-kernel'}):
            with self.assertRaisesRegex(baseline.Stop, '^PRIOR_LOCAL_CLUSTER_PROCESS_STILL_EXISTS_STOP_OWNER_RUN_FIRST$'):
                baseline.prior_cluster_activity(Path('fixture-parent'), Path('fixture-current'))

    def identity_fixture(self):
        directory = (Path.cwd() / '.qa-artifacts/identity-fixture/postgres').resolve()
        return directory, {'directory': str(directory), 'address': '127.0.0.1/32',
                           'port': 12345, 'version': 180006}

    def test_identity_accepts_exact_ipv4_host_with_or_without_host_mask(self):
        directory, value = self.identity_fixture()
        for address in ['127.0.0.1', '127.0.0.1/32']:
            with self.subTest(address=address):
                self.assertTrue(all(baseline.cluster_identity_checks(dict(value, address=address), directory, 12345).values()))

    def test_identity_rejects_different_host_broad_mask_and_ipv6(self):
        directory, value = self.identity_fixture()
        for address in ['127.0.0.2/32', '127.0.0.1/24', '0.0.0.0/0', '192.0.2.1/32',
                        '::1/128', '::ffff:127.0.0.1/128', 'invalid-address', None, 2130706433]:
            with self.subTest(address=address):
                self.assertIs(baseline.cluster_identity_checks(dict(value, address=address), directory, 12345)['address'], False)

    def test_identity_rejects_wrong_port_or_version_and_wrong_types(self):
        directory, value = self.identity_fixture()
        for field, bad in [('port', 12346), ('port', '12345'), ('port', True),
                           ('version', 170006), ('version', '180006'), ('version', True), ('version', 190000)]:
            with self.subTest(field=field, bad=bad):
                self.assertIs(baseline.cluster_identity_checks(dict(value, **{field: bad}), directory, 12345)[field], False)

    def test_identity_directory_requires_exact_resolved_path(self):
        directory, value = self.identity_fixture()
        value['directory'] = str(directory.parent / 'another-postgres')
        self.assertIs(baseline.cluster_identity_checks(value, directory, 12345)['directory'], False)

    def test_identity_missing_payload_cannot_pass(self):
        directory, value = self.identity_fixture()
        for observed in [None, [], {}, {'address': '127.0.0.1/32'}]:
            with self.subTest(observed=observed):
                self.assertFalse(all(baseline.cluster_identity_checks(observed, directory, 12345).values()))

    def test_identity_method_records_each_comparison_before_rejecting(self):
        directory, value = self.identity_fixture()
        cluster = baseline.Cluster(directory.parent, Path('fixture-bin'), 12345, {})
        bad = dict(value, address='192.0.2.1/32')
        with patch.object(cluster, 'sql', side_effect=[json.dumps(value).encode(), json.dumps(bad).encode()]), \
             patch.object(baseline, 'write_new') as write:
            cluster.identity()
            with self.assertRaisesRegex(baseline.Stop, '^LOCAL_CLUSTER_IDENTITY_MISMATCH$'):
                cluster.identity()
        self.assertEqual([call.args[0].name for call in write.call_args_list], ['identity-001.json', 'identity-002.json'])
        self.assertIs(json.loads(write.call_args_list[0].args[1])['checks']['address'], True)
        self.assertIs(json.loads(write.call_args_list[1].args[1])['checks']['address'], False)


if __name__ == '__main__':
    unittest.main()
