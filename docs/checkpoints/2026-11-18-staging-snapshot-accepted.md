# Accepted public application snapshot — 18 November 2026

**PASS_REVIEWED_PUBLIC_SNAPSHOT_EXPORT_ONLY** for owner run
`20261118T131839Z-staging-baseline-3fe3bf70b4d4`.
This closes the bounded acquisition task; it does not close M01, DLV-06,
actual restore, full migration, staging runtime, or a business-domain checkpoint.

## Observed result and independent local review

The normal Windows owner terminal exported one custom PostgreSQL archive from
the existing user-identified staging source, derived SQL from that archive,
encrypted both, authenticated the encrypted payloads, and emitted
`PASS_PUBLIC_SNAPSHOT_EXPORT_ONLY`. Source PG17.6; client PG18.6; client TLS1.3
with verify-full and explicit read-only metadata transaction. Credential remains
privileged, not a certified least-privilege source role.

Archive capture window: **2026-11-18 13:18:40.455–13:18:43.280 UTC**
(20:18:40–20:18:43 WIB). This is the export window, not an independently recorded
MVCC snapshot identifier or proof of the latest production synchronization.

After the owner run, a separate local-only review verified:

- Exact current operator SHA-256 equals the operator hash recorded by the run.
- Plain custom archive: **528,662 bytes**, SHA-256 matches manifest.
- SQL derived from that archive: **2,121,156 bytes**, SHA-256 matches manifest.
- Encrypted custom/SQL: **528,990 / 2,121,868 bytes**, both SHA-256 values match.
- Actual decryption/authentication of **both** age artifacts completed; plaintext
  bytes and SHA-256 independently match the originals. Secret keys and plaintext
  were not printed or copied into the public repository.
- Actual `pg_restore --list` reads the archive, matches the saved TOC byte-for-
  byte, and accounts for **21 TABLE DATA public entries** against the captured
  preflight inventory. This is table coverage, not a row-count/restore proof.
- Entire artifact directory measured **5,313,829 bytes**, approximately 5.1 MiB.
  No database cluster was created to acquire or review this baseline.

The review did not reconnect to staging, execute dump SQL, write source data,
access R2, delete files, commit or push. Existing application code and schema
were unchanged. No suite was rerun merely to review unchanged exported files.

## Private custody and scope

Artifact directory under `backend-v3/private-backup/`:
`20261118T131839Z-staging-baseline-3fe3bf70b4d4/`.
Key directory is its sibling with suffix `-keys`.
Original private `summary.json`/`preflight.json` remain unchanged.
Independent review receipt:
`.qa-artifacts/20261118T131557Z-baseline-tls-fix-6d5f45ab/export-review.json`.

**Application public-schema snapshot only.** Non-public Supabase schemas,
global roles/tablespaces, service configuration and R2 object bytes are excluded.
Public legacy session records are retained privately as historical baseline;
they are not authorized v3 login sessions. Restoring this snapshot must not start
legacy services, callbacks, jobs or notifications.

Custom/SQL plaintext remains inside the owner-private directory along with
encrypted copies. Key is in a separate private sibling, on the same disk:
**independent key custody is not established**. Keep artifacts and key available
for recovery; never send them to chat/Git. Do not delete or overwrite failed runs.
Avoid repeating the successful export without needing a new dated baseline.

## Next dependency

1. Review executable schema/SQL and extension dependencies without printing data.
2. Prepare a fresh isolated local restore target, identify it unambiguously,
   preserve/verify its empty pre-state, and prevent outbound jobs/providers.
3. Restore using this exact immutable archive, then reconcile schema, record
   counts, constraints, relationships, sequence state and relevant file references.
4. Only after restore proof, define source-to-v3 mappings and migrate into a
   **separate** local v3 database with backups/provenance before each mutation.

Actual restore remains **NOT_RUN**. No proof of full-project backup, R2 snapshot,
full business compatibility or readiness to replace the running LMS is implied.
Existing local synthetic runners must not be repointed at this real baseline.

See [snapshot runbook](../baseline/STAGING_LOCAL_SNAPSHOT.md) and
[intake contract](../baseline/REAL_BASELINE_INTAKE.md).
