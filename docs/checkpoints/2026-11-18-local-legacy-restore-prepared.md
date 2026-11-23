# Local legacy restore preparation — 18 November 2026

User authorized snapshot schema/dependency review, a new isolated local restore,
and data verification. Source is the accepted local snapshot, not staging/live.
No database deletion, shared mutations, R2, commit or push is authorized here.

Completed:

- Reviewed actual complete schema commands and COPY blocks privately:
  21 tables, **7,843 rows**, 4 enums, 52 constraints/27 FK, 1 additional index,
  zero sequences. Found uuid-ossp/`extensions.uuid_generate_v4()` dependency;
  installed control file exists. No programmable/foreign/provider DDL in this
  reviewed snapshot. Pinned schema DDL SHA-256
  `a3f484735830590d8cd529817145ea33ebc3d6fc05628bab32cbf9fa9cd02f31`.
- Defined two metadata-only public schema TOC omissions for an existing empty
  public schema; no application table/data exclusion and no DROP workaround.
- Added [new-local-only operator/runbook](../baseline/LOCAL_LEGACY_RESTORE.md),
  [11-case contract](../../qa/manifests/REAL-LEGACY-LOCAL-RESTORE.json), and owner
  CMD launcher. No synthetic lab fence, Go application or migration code changed.
- **34/34 offline regressions PASS**, including 15 new restore safety/parser
  cases. Actual pinned-input schema/COPY review PASS. This is not a native
  database/full Go/Playwright acceptance claim.
- Actual runner attempt `20261118T133831Z-legacy-local-restore-ac6886594191`:
  **BLOCKED_BEFORE_RESTORE / PermissionError**, 0/11 native cases, no source
  connection, input recovery, initdb, new running cluster, or restore.

Ignored local evidence:
`.qa-artifacts/20261118T132606Z-baseline-restore-review-d722133c/`
contains source review, schema signature/column inventory, private counts/hashes,
offline test output, actual blocked attempt and handoff-verification receipt.
No private row contents, passwords or raw keys are printed to public docs.

Status: **IMPLEMENTED_PENDING_OWNER_NATIVE_RESTORE**. Normal Windows owner
terminal is necessary for protected output; this is the OS permission limitation,
not a request for a new authorization or a reason to weaken ACLs. Source archive
remains accepted and untouched; staging/R2 not reconnected. No module/CP/DLV-06
closure, commit or push. Next command and exit criteria are in the runbook.
