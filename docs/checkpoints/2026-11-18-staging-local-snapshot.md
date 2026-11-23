# Staging baseline acquisition — 18 November 2026

User meminta mengambil baseline dari `.env.staging` existing setelah menyatakan
staging tersinkron dari live dan media memakai R2. Scope ini terpisah dari
checkpoint domain/M-series: tidak ada CP/module yang ditutup atau dinaikkan coverage.

- Actual Go metadata probe: system-only trust FAIL; explicit existing CA PASS
  `READ_ONLY_OBSERVATION_COMPLETE`, verified TLS and read-only rollback, zero writes.
- Source PG17.6, 15,232,147 bytes, 21 public tables; source credential privileged,
  **least-privilege NOT verified**; production distinction/sync/deployed SHA unknown.
- Private destination write probe and actual owner-runner attempt from Codex:
  `PermissionError`, **BLOCKED_BEFORE_EXPORT**, no user row reads/dump/restore.
- New [owner runner/contract](../baseline/STAGING_LOCAL_SNAPSHOT.md): bounded
  public application custom dump, SQL derived from same archive, private output,
  age roundtrip hashes, explicit exclusions, no remote DDL/R2/deletion/Git.
- Offline regressions: **12/12 unittest PASS**. Initial filesystem-based test
  fixture setup failed in restricted Windows TEMP; parser tests now use in-memory
  synthetic text and mock permission-denial boundary. No native permission gate
  is mocked into acceptance. Actual export/encryption/restore remain NOT_RUN.
- Existing application Go sources, legacy app, schema/migration catalogue, `.env`,
  R2, root module files and synthetic fences unchanged. New Python operator code
  is acquisition tooling, not a TypeScript backend or Go staging runtime.
- No full Go/Playwright/DB/remote CI rerun/acceptance is inferred from the 12 tests.

Private metadata evidence (ignored, source rows/secrets absent):
`.qa-artifacts/20261118T124944Z-staging-readonly-803c93a6ff/`
contains `inspection.json`, `inspection-with-ca.json`, `destination-check.json`,
and `export-attempt.json`. Failed private directories are retained, not deleted.
The native runner must be invoked by the Windows folder owner; no ACL relaxation,
privilege escalation or network/proxy bypass is proposed.

Next: one normal-terminal owner export, review its exact manifest, then separately
review executable dump and restore into a fresh isolated local database. Missing
full restore and DLV-06 gates remain open; a public-only snapshot is not a full
project or R2 backup.
