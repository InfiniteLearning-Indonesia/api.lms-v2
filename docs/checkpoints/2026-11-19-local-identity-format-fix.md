# Local cluster identity address format — 19 November 2026

Owner run `20261119T080846Z-legacy-local-restore-bf1ed15f0f91` passed startup
but stopped at `LOCAL_CLUSTER_IDENTITY_MISMATCH`. Its receipt has input
authentication, schema/COPY review and normal-stop cases only (3/11), no
prestate/data restore. `stop_failure=null`; local `pg_controldata` independently
reports **shut down**, PID marker absent, and the log confirms loopback listen on
the configured port and completed shutdown. No source/R2 connection occurred.

## Deterministic validator bug

The SQL query explicitly casts `inet_server_addr()` to `text`. PostgreSQL's
documented inet-to-text conversion retains the netmask, including `/32` for an
IPv4 host. The old Python check compared that value with exactly `127.0.0.1`,
therefore a valid `127.0.0.1/32` could never pass. This is an implementation bug
in the operator, not evidence that it connected to another database.

The old failed run did not save the identity JSON, so its four raw fields cannot
be retrospectively claimed independently verified. The observed loopback log,
configured directory/port, version and the documented cast establish the code
defect; post-fix native acceptance still requires the owner's next run.

## Fix and tests

[cluster_identity_checks](../../qa/operators/restore_legacy_baseline.py) parses
the inet host representation with Python `ipaddress`, accepts only IPv4 address
**127.0.0.1 with prefix /32** (bare host is equivalent), and still requires exact
resolved data_directory, integer expected port, and integer PostgreSQL18 version.
Another loopback host, broader prefix, IPv6/mapped address, remote address,
wrong directory/port/version, missing fields and wrong types remain rejected.

`Cluster.identity` now writes unique private `identity-001.json`,
`identity-002.json`, etc. **before** rejecting a mismatch, including actual/expected
local fields and individual comparisons. A second identity check no longer risks
overwriting an earlier diagnostic; summary links all identity check outcomes.
No credentials, source data or user rows are included in these diagnostics.

- **49/49 offline regressions PASS**: previous 43 plus six identity tests.
- Integration-level method regression feeds the documented `/32` JSON through
  the real `Cluster.identity` method, checks diagnostic preservation, and verifies
  a subsequent remote address still fails. Native database behavior is not mocked
  into a restore acceptance claim.
- Public source-before ZIP reopened/hash verified. Existing loopback shutdown
  proof checked read-only. Original four accepted snapshot artifacts are unchanged.
- Post-fix native identity/restore **NOT_RUN**. No Go application/schema changes,
  new cluster from Codex, source/R2 access, deletion, commit, push or module closure.

Ignored evidence:
`.qa-artifacts/20261119T083355Z-local-identity-fix-b57459ae/`.

## Next

The failed run is stopped; no Ctrl+C or marker removal is needed. Run the same
updated `qa/RESTORE_STAGING_BASELINE_LOCAL.cmd` once in the normal owner terminal.
Expected final acceptance remains exact 11/11 native cases, row/schema parity and
normal stop. Do not interpret these offline tests as full database QA completion.

Primary reference: PostgreSQL [network address functions](https://www.postgresql.org/docs/18/functions-net.html)
documents explicit inet-to-text retaining masks and the separate host format.
