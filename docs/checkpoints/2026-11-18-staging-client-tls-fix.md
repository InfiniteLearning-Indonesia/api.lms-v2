# Correct client TLS evidence for staging snapshot — 18 November 2026

## Diagnosis

Owner run `20261118T131356Z-staging-baseline-9dcc44508b30` reached metadata
preflight and returned `BLOCKED_BEFORE_EXPORT / TLS_OR_READONLY_NOT_VERIFIED`.
Its summary confirms no export attempt, source rows, partial dump, or mutations.
Original runner saved preflight only after validation, so failed evidence was
missing; this observability bug is also fixed.

An authorized metadata-only diagnostic reproduced the actual state:

- `read_only=true` in explicit repeatable-read/read-only transaction.
- The selected host is a Supabase pooler (host/account name not published).
- `pg_stat_ssl` for the database-facing backend reports `false`.
- PostgreSQL 18 client `\conninfo` on the **same connection** reports
  `SSL Connection|true`, `SSL Protocol|TLSv1.3`.
- Client environment requires `sslmode=verify-full` and the existing CA.

Root cause is an implementation bug: the old SQL treated backend-hop TLS as
proof of the client-to-endpoint transport. A false backend flag does not negate
the independently observed client TLS. No claim is made about provider-internal
transport security; that hop has not been independently verified.

## Change

[Operator](../../qa/operators/freeze_staging_baseline.py):
`probe_args` executes metadata SQL and client `\conninfo` in one psql process/
connection; `parse_preflight_output` accepts only explicit SSL=true and TLS1.2/1.3,
retains separate `server_observed_tls`, and discards host/user/database conninfo
rows. Duplicate, missing, invalid and weak TLS evidence fails closed.
`pg_environment` retains verify-full/CA, enforces TLS1.2 minimum, disables GSS
alternative encryption, and sets C locale for the PostgreSQL 18 parser.
`validate_observation` still requires boolean-true read-only and client TLS.
Source sessions, not the credential's global privileges, are constrained;
least-privilege certification remains false.

Preflight metadata and safe transport flags are now recorded before later
validation failure. No automatic retries, alternate source, role/ACL changes,
TLS fallback, synthetic fence changes, source mutations or R2 calls were added.
The export command and public-only scope remain unchanged.

## Verification

- **19/19 offline unittest regressions PASS**, including seven new cases for
  split-hop TLS, absent client proof, weak protocol, duplicate evidence, malformed
  output, strict booleans and a single client process for SQL plus conninfo.
- Corrected actual preflight: **PASS_ACTUAL_READONLY_PREFLIGHT_ONLY**. PG17.6,
  database bytes 15,232,147, 21 public tables, zero cross-schema FK count;
  client TLS1.3/verify-full and read-only are true, backend flag remains false.
- Actual dump/encryption/restore **NOT_EXECUTED**. Normal Windows owner terminal
  is still required to write protected output. No CP/module/DLV gate is closed.
- Source-before ZIP reopened/hash-verified; ignored evidence directory:
  `.qa-artifacts/20261118T131557Z-baseline-tls-fix-6d5f45ab/`.
  `actual-preflight.json` contains only sanitized metadata, not source rows,
  raw conninfo, credentials or private keys. Old failed run is preserved.
- No full Go/Playwright/DB journey or remote CI acceptance is inferred.
  No delete, commit, push or `.env` edit.

## Next command

Run the same [owner launcher](../baseline/STAGING_LOCAL_SNAPSHOT.md) once:

```powershell
D:\Projects\lms-v2\backend-v3\backend-go\qa\FREEZE_STAGING_BASELINE.cmd
```

Expected export-only result: `PASS_PUBLIC_SNAPSHOT_EXPORT_ONLY`. Return status/
run ID for archive review before a separately isolated local restore. Do not
delete previous evidence or loosen ACL/TLS if another gate fails.

Primary references: [psql command sequencing and conninfo](https://www.postgresql.org/docs/18/app-psql.html),
[libpq verify-full](https://www.postgresql.org/docs/18/libpq-ssl.html).
