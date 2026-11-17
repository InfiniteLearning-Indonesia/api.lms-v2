# 17 November — local recovery implemented, actual recovery QA pending

**IMPLEMENTED_PENDING_OWNER_NATIVE_QA; full required current suite not accepted.** Supports M01 / DLV-04, no new CP/D or module closure. [Frozen scope](2026-11-17-local-recovery-scope.md), [runbook/command](../baseline/LOCAL_RECOVERY_RUNBOOK.md), [staging configuration reality](../baseline/STAGING_CONFIGURATION_READINESS.md).

## Accepted dependency and delivered work

Before changing code, reviewed owner run `20261116T210959Z-age-stream-09579d7d1bf9`: **7/7 actual protected/crypto/package cases + 79/79 Go + vet PASS** on source `126c217ad43a732e67d5d3b32473233ed5bd94385596a78872f0ba9b7b63371b`; backup ZIP/raw cases/tool hashes/source verified. [Bounded acceptance](2026-11-17-protected-package-accepted.md). This closes its old native blocker, not a full module or the changed recovery tree.

Current implementation:

- [Go local transfer](../../internal/migration/backup/package_transfer.go): strict `--seal-only` from private local files with role-capacity cap; explicit `--recover-only` authenticates before new private plaintext creation, verifies second pass + readback/hash/identity/protection, never overwrites/deletes/renames or prints plaintext. No success receipt for retained partial output. Existing integrity/planner modes remain lower proof scope.
- [Small native recovery runner](../../qa/local_recovery_run.py): no DSN/env/existing cluster input; exact nine-case current crypto/81-Go/vet gate, then exact 95 ops before DB creation, managed storage admission, fixed new loopback PG18 cluster/roles/databases, prestate SQL/custom backups + actual isolated proof restore, generated source snapshot, age sealing/auth/recovery, two target restores, schema/row/key/index/sequence/file parity, FK denial and normal-stop evidence. Ten DB cases frozen; **implementation, not execution evidence**.
- [One owner command](../../qa/RUN_LOCAL_RECOVERY_QA.cmd). No tool/dependency network fallback in `--no-download`; missing cached inputs fail. No full LMS dependency journey/retention disposal. New profile is explicitly pinned for review, under counted canonical QA root; after-run storage measurement and growth guard included. Existing ACLs/privileges and policy config unchanged.
- Config/runtime assessed without reading/editing `.env`: `config.Load` still does not load dotenv or support staging modes; lab/source fences remain. Local operator CLI is not a staged product runtime.

## Backup and current verification

Source/context backup before edits: `backend-v3/private-backup/local-recovery-20261117/source-before.zip`, 466 files, 1,246,005 archive bytes. SHA-256 `bf8cfa169c39104c17e21f0e2c14621c6296f8d0d4aeb7583af6f8e70e3b9efe`; reopened byte/hash/CRC review PASS. Not a DB backup; no database migration ran.

Current source **`454187dda3c48e32681a73c9810d47d92fee196e17f00958a2898df608bdfa33`**. Private evidence `local-recovery-20261117/qa-G/` and `review-G.json`. Source unchanged throughout final run/review.

| Scope | Current result |
|---|---|
| Default Go | **81/81 PASS**, exact prior 79 + 2 transfer-boundary/strict-input tests |
| Python ops | **95/95 PASS**, including actual local-transfer CLI refusal and no-ack/failed-crypto/no-external-target runner fences |
| Vet, Windows build and three operator builds | PASS |
| Linux build + tagged-test cross-compilation | PASS_BUILD_ONLY; not Linux runtime/race/remote CI |
| Mandatory current native crypto | **9 required: 4 PASS, 5 FAIL at private-identity fixture setup, AccessDenied**. New recovery assertions not reached. Full suite FAIL/NOT_ACCEPTED, not relabelled/skipped |
| Actual DB recovery profile | **10 cases NOT_RUN**. Dependent DB work stopped before cluster creation; actual_restore_verified=false |
| Backup default statement coverage | **64.7% (712/1100)**; new success/recovery paths unexercised. Not deliverable coverage |
| Current full API/Playwright/DB business/remote CI | NOT_RUN |
| Canonical / real-baseline readiness | M01 0/5, project 0/97 unchanged; DLV-06 NOT_READY |

Read-only review verified old cases/gates preserved, root Go module/config/business runtime and existing schema files unchanged, source backup intact. Only operator/backup/QA code changed. Previous current seven-case owner result stays accepted on its old source; it cannot be reused as a nine-case PASS after new plaintext-recovery behavior.

## Failures and corrections

Run A's new structural unit fixture serialized null arrays, correctly rejected by the strict decoder; the test fixture was populated correctly. Decoder/null policy was not weakened. B fixed that result; C tightened per-role output capacity and owner ops gate; D enforced cache-only dependency behavior; E corrected psql stdin transaction arguments (`--file=-`, quiet mode); F added post-run storage measurement; G uses the declared ciphertext budget instead of assuming a fixed extra MiB covers overhead at every file size. Distinct-source attempts/history retained, not overwritten or portrayed as unchanged retries.

All current five actual-case failures occur at owner-only synthetic identity creation under the restricted process token, matching earlier host limitation. No sandbox SID/broad ACL, elevation, fake crypto result, driver substitution or expected-case removal was used. Further implementation/native PG issues remain possible until the owner run actually passes; code review/compile is insufficient.

## Next action

```powershell
D:\Projects\lms-v2\backend-v3\backend-go\qa\RUN_LOCAL_RECOVERY_QA.cmd
```

Ordinary owner terminal; source must stay frozen. This bundles nine native crypto cases, 81 default Go/vet, 95 ops and the ten small PG recovery stages. If native/storage/tool gates fail, **no DB startup**. If actual recovery fails, stop only its generated cluster and retain evidence. No migration/restore of staging or production, no existing cluster cleanup.

After exact native/DB PASS, review source/cases, backup/target identity, command outcomes and parity/storage/stop receipts before accepting bounded local recovery. Then close residual Playwright/remote gates and actual metadata/baseline/control review; real source adapter, mapping, independent custody, product runtime composition and frontend/pilot remain separate. `.env` editing is not a staging activation step.

This session: no shared DB/cluster created, no actual restore/export/migration, env edits, existing ACL/privilege changes, existing artifact deletion, legacy/frontend edits, commit or push. New source/QA is local and not covered by the old GitHub commit. PostgreSQL skill guidance informed fixture keys/FK indexes/privileges; [native PG runbook](../baseline/LOCAL_RECOVERY_RUNBOOK.md) links the primary tool semantics.
