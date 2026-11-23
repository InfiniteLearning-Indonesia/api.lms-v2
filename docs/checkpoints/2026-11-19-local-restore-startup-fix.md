# Windows local restore startup supervision fix — 19 November 2026

## Observed incident

Owner runs `20261118T134618Z-legacy-local-restore-ce15f83aa6c4` and
`20261119T074035Z-legacy-local-restore-caf691c7fec0` stopped producing progress
after the new-cluster startup line. The latest cluster's own log and pid marker
reported ready; the operator had not reached target-empty backup or row parity.

An authorized **read-only loopback query** to the latest cluster independently
verified its exact data_directory and recorded port. It found **zero user-created
databases** and no `lms_baseline_owner` role. The wrapper was still waiting before
provisioning/restore; the 7,843-record application baseline had not been loaded.
This was not a staging/production connection or an R2 operation.

Root cause: `Cluster.command` used `subprocess.run(capture_output=True)` for
`pg_ctl start`. On Windows a background postgres descendant can keep inherited
pipe handles open after the control process exits. Waiting for EOF (including
timeout cleanup via communicate) can stall the Python supervisor indefinitely.
The old command budget therefore did not reliably bound this pipe-wait path.

## Fix and verification

- [control_process](../../qa/operators/restore_legacy_baseline.py) now redirects
  pg_ctl stdout/stderr to exclusive **regular files inside the private run**,
  with stdin DEVNULL. It waits for the direct CLI with a timeout; it does not wait
  for background-descendant pipe EOF. Other data commands retain their existing
  protected capture behavior.
- A monotonically increasing control-command sequence prevents output filename
  collisions when startup fails and the finally block attempts stop.
- Console emits explicit pg_ctl start/stop progress. Raw diagnostic contents,
  credentials, data rows and queries are not printed.
- `prior_cluster_activity` blocks a new clone when a previous recorded loopback
  port is still listening. Unknown liveness/invalid directory evidence fails
  closed. Stale PID/no-listener directories are preserved and recorded; **no PID
  deletion, restart, process kill or automatic cleanup** is performed.
- **38/38 offline regressions PASS**: previous 34 plus regular-file control I/O,
  timeout propagation, prior-live-cluster guard and stale-marker preservation.
- Actual native **synthetic process-I/O reproduction PASS**: ordinary pipe capture
  waited **3.125 seconds** for a synthetic descendant; fixed file capture returned
  in **0.078 seconds** after the direct parent. No DB/key/source data was involved.
- A targeted graceful pg_ctl stop was attempted against **only** the independently
  verified empty latest local cluster; it returned exit 1, could not send the stop
  signal, and did not prove shutdown. No escalation, broad process termination,
  Windows service change or permission workaround was used. The owner terminal
  must finish/interrupt its own older process and establish shutdown.

The original baseline export/crypto acceptance remains valid. Actual database
restore is **NOT_VERIFIED**, and 11-case native restore completion remains open.
No Go runtime/schema/migration fence, `.env`, source database, R2, module count,
delete, commit or push was changed.

## Owner handoff

The currently running Python process loaded the old wrapper; editing the file
does not replace that process. If it is still stuck, **press Ctrl+C once**, allow
the old finally block to stop its own cluster, and wait for the PowerShell prompt.
An interruption/failure result for that old run is expected, not PASS evidence.
Then run the same updated launcher:

```powershell
D:\Projects\lms-v2\backend-v3\backend-go\qa\RESTORE_STAGING_BASELINE_LOCAL.cmd
```

If `PRIOR_LOCAL_CLUSTER_STILL_LISTENING_STOP_OWNER_RUN_FIRST` appears, stop and
review that specific owned run; do not repeatedly create new clusters or use
global PostgreSQL/taskkill commands. A failed or stale directory is retained.
Do not remove old PID markers or accepted receipts to force progress.

New full result still requires **11/11 native cases** including matching all
schema/row hashes and proved normal stop. Unit or synthetic process-I/O PASS is
not actual restore acceptance.

Ignored evidence: `.qa-artifacts/20261119T074347Z-restore-startup-fix-f05c83d7/`
contains reopened/verified public-source backup, local identity result, targeted
stop result, and native synthetic process-I/O regression. No raw source records
or key values are in public documentation.

References: [Python subprocess pipe/timeout semantics](https://docs.python.org/3.12/library/subprocess.html),
[PostgreSQL pg_ctl lifecycle](https://www.postgresql.org/docs/18/app-pg-ctl.html).
