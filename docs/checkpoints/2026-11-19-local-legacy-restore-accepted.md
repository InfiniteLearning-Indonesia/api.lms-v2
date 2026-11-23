# Accepted local legacy baseline restore — 19 November 2026

**PASS_ACCEPTED_LOCAL_BASELINE_RESTORE_ONLY** after reviewing owner run
`20261119T093357Z-legacy-local-restore-debaab74a513`, which emitted
`PASS_REVIEWED_LOCAL_BASELINE_RESTORE_ONLY`, **11/11 cases**, and
`actual_restore_verified=true`.

This completes the authorized bounded public-baseline local restore. It is not
full M01/M15/M16, DLV-06, staging-v3 runtime, R2 recovery, all-format recovery,
full migration or cutover acceptance. Previous failed runs remain preserved.

## Scope and actual evidence

Input: accepted public-only staging snapshot
`20261118T131839Z-staging-baseline-3fe3bf70b4d4`. No staging/production/R2
connection was made by the restore or this review. Baseline snapshot time remains
18 November; it is not a newly synchronized production snapshot.

Owner run completed at **2026-11-19 09:34:10 UTC / 16:34:10 WIB**. Target is a
new private PostgreSQL18 cluster and database `legacy_frozen`; it is now stopped.
The source archive remains PostgreSQL17.6 logical data. No app/worker/provider
was started; historical sessions and credential-bearing user records were not
activated as v3 authority.

The exact eleven successful cases:

1. Encrypted custom/SQL inputs authenticated and recovered with matching hashes.
2. Pinned schema and all COPY blocks reviewed before execution.
3. Actual new-cluster directory/address/port/version verified.
4. Empty target backed up and actual public-scope restore into a fresh proof DB
   compared before applying dependency/schema/data mutations.
5. Minimal local uuid-ossp dependency provisioned; no Supabase platform clone.
6. Custom archive restored atomically using non-superuser owner, no clean/create.
7. Schema signature matched, including **21 tables, 4 enums, 52 constraints
   containing 27 FK, one additional index and zero sequences**.
8. **All 21 tables / 7,843 rows** reconciled using count and COPY multiset hashes.
   Sorting allows physical row-order differences while preserving duplicates.
9. All FK validated, no sequences, reader has no DML/superuser/bypass/create-role/
   create-DB rights; reader default read-only verified on a new connection.
10. Original accepted snapshot files still match their pinned hashes.
11. Cluster normal-stop proved by pg_controldata and absent postmaster.pid.

The run used approximately **81,100,687 bytes / 77.3 MiB** at its final storage
measurement. This is the successful run's logical footprint, not a total for all
historical artifacts or an OS quota. No additional cluster was created for review.

## Independent local review

Review receipt:
`.qa-artifacts/20261119T093607Z-local-restore-accepted-80b8c723/review.json`.
All twelve review checks passed:

- Exact case sequence, successful status and actual-restore flag.
- Current operator file SHA-256 equals the owner run's recorded hash.
- All four original plaintext/encrypted snapshot file sizes/hashes unchanged.
- Both recovered input payload hashes match pinned originals.
- Saved source COPY ledger reconstructed from the original SQL matches exactly.
- Row-parity receipts have exactly 21 unique tables and 7,843 rows, and each
  table's count/hash matches the reconstructed source ledger.
- Schema review matches pinned DDL signature, enum/FK/constraint/sequence scope.
- Reader safety receipt matches restricted capabilities and validated FK state.
- Both identity evidence files report all four exact comparisons true.
- Empty prestate backup hashes and empty-catalog receipt match saved files.
- **Actual cold pg_controldata** independently reports `shut down`; PID absent.
- Accepted-local-baseline marker references this run and the pinned source hash.

This review rechecked artifact/receipt integrity and actual stopped control
state; it did not restart/query the restored server or repeat the restore.
Native row/schema comparisons were performed by the owner run's hash-matched
operator. Public docs contain no source records, passwords or key values.
No code change or redundant Go/Playwright/full DB rerun was needed to accept
these results. The last operator offline suite remains 49/49 on its source.

## Retained local state and limits

Private root: `backend-v3/private-backup/20261119T093357Z-legacy-local-restore-debaab74a513/`.
It contains stopped `postgres/`, protected recovered input, prestate backups,
row/schema/identity/safety receipts and new local credentials in `keys/`.
The accepted-baseline marker prevents an identical successful restore from being
repeated by the launcher. Keep the legacy clone as a comparison baseline; don't
apply v3 schema transformations into it.

No files or records were deleted. Older failed/stale clusters are preserved,
including the unclean old startup marker; their failed status is not upgraded.
Source age identity remains in its original sibling key folder on the same disk:
independent custody/disaster recovery is still not established.

Public-only input excludes managed non-public schemas, global roles/settings and
R2 bytes. Actual SQL restore as a second full-data target is outside this run;
custom restore and SQL-based source data parity are covered. Target locale C /
PG18 behavior is not claimed identical to production collation or performance.
Source identity/sync/deployed SHA remain user-declared/unverified where noted;
this is a verified local baseline, not certified current production parity.

## Next dependency

Proceed with **read-only real-data analysis and explicit legacy-to-v3 mapping**:
identify Class/program/batch, identity/enrollment/context-role, content/work,
grades/completion/history, and file-reference decisions with provenance and
quarantine for ambiguities. Existing M15 fixture mapping is not a complete real
importer. Build/apply into a **separate new local v3 target**, with verified backup
before migrations and reconciliation against this frozen baseline.

Do not repoint synthetic runners or the Go API at the real legacy database,
enable outbound jobs, or create staging-v2/import shared data as a side effect of
this acceptance. Runtime composition, full-domain mapping, R2 scope, delta/pilot
and production handoff remain separate gates. No commit or push was performed.
