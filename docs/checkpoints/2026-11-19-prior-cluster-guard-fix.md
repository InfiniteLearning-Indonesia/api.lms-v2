# Prior local cluster liveness evidence — 19 November 2026

Owner retry `20261119T075610Z-legacy-local-restore-961ceb3b6683` stopped at
`BLOCKED_BEFORE_RESTORE / PRIOR_LOCAL_CLUSTER_STATUS_UNKNOWN`, 0/11 cases,
without creating a new cluster or touching staging/R2.

## Actual cause and distinction

- The preceding 19 November run was interrupted by its owner. Its saved receipt
  confirms `KeyboardInterrupt`, normal-stop case present, `stop_failure=null`,
  and no actual restore. Its pid marker is gone.
- The remaining marker belongs to older run
  `20261118T134618Z-legacy-local-restore-ce15f83aa6c4`, recorded PID 6672 and
  local port 58273. The one-second socket check timed out, so the guard correctly
  refused to infer inactivity but lacked a reliable way to resolve this state.
- Direct read-only Windows process snapshot confirms PID 6672 absent; the Windows
  IPv4 listener table confirms no listener at that port. Older pg_control still
  says `in production`, evidence of unclean interruption, **not proof of an active
  process or of a clean shutdown**. The old directory is quarantined/preserved.

## Fix

[windows_local_state](../../qa/operators/windows_local_state.py) queries
Toolhelp process IDs before and after `GetExtendedTcpTable` for IPv4 listener
owner PIDs. No network connection, signal, process kill, permission change,
PID-file deletion or service action is involved.

[prior_cluster_activity](../../qa/operators/restore_legacy_baseline.py) now
requires **both** no recorded process and no matching listener before recording
`STALE_PID_NO_PROCESS_NO_LISTENER_PRESERVED`, with
`clean_shutdown_verified=false`. Any listener blocks; an existing recorded PID
without a listener also blocks. API failure, incomplete/malformed buffers and
unstable enumeration fail closed. PID reuse can conservatively block, never
authorize killing that process. Existing records and source archives are untouched.

## Evidence

- **43/43 offline regressions PASS**: five new cases plus previous 38. Coverage
  includes network-order port decoding, malformed/nonlistener table refusal,
  process presence on either side of the listener snapshot, enumeration failure,
  and live recorded PID without listener. Previous liveness tests now use kernel
  metadata; they no longer fake socket refusal as the current implementation.
- **Actual read-only prior guard PASS** on the real stale marker, preserving the
  directory and proving no process/listener. This does not certify old data clean.
- **Actual native positive detection PASS** using a new synthetic loopback
  listener owned by the test process; PID/listener both detected. The socket
  closed normally afterward; no DB/file/user data was created for that test.
- Accepted snapshot hashes unchanged. No actual new restore, source database/R2
  connection, Go application change, module/CP closure, commit or push.

Ignored local evidence:
`.qa-artifacts/20261119T075908Z-prior-cluster-guard-24277867/`, including verified
public-source-before backup, actual negative/positive kernel checks and handoff.

## Next owner run

The actual guard now passes on the current prior-run state. Run the same updated
`qa/RESTORE_STAGING_BASELINE_LOCAL.cmd` once in the normal owner terminal. Final
acceptance still requires exact 11/11 native DB cases, row/schema parity and proved
normal stop. **Actual restore remains NOT_VERIFIED** until that run is reviewed.

Primary references: [GetExtendedTcpTable](https://learn.microsoft.com/en-us/windows/win32/api/iphlpapi/nf-iphlpapi-getextendedtcptable),
[Toolhelp process snapshot](https://learn.microsoft.com/en-us/windows/win32/api/tlhelp32/nf-tlhelp32-createtoolhelp32snapshot).
