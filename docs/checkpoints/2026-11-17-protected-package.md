# 17 November — protected package implementation, native QA pending

> **Later accepted:** [Owner seven-case run reviewed](2026-11-17-protected-package-accepted.md) on source `126c217…`: 7/7 actual + 79/79 Go/vet PASS. Earlier FAIL below is history. [New local recovery](2026-11-17-local-recovery.md) changes the tree and requires nine native cases plus ten actual DB cases; those are not accepted by the old result.

**IMPLEMENTED_PENDING_OWNER_NATIVE_QA. Full required actual suite FAIL / NOT_ACCEPTED; no CP/module closure.** Supports M01-CP02 / DLV-03–04. [Frozen scope](2026-11-17-protected-package-scope.md), [contract and command](../baseline/PROTECTED_PACKAGE_RECOVERY.md), [project handoff](../PROJECT_STATE_BEFORE_LIVE_STAGING.md).

## Work delivered

1. Actual Windows owner/DACL inspection replaces unconditional rejection: narrow current-user/LocalSystem allow policy, no null/broad/unsupported ACE acceptance. Linux owner/mode and regular-file hardlink checks. Read-only checks never set an existing ACL/owner or enable privileges.
2. `SealProtectedAgeFile`: anchored exclusive ciphertext file, actual disk/reserve, private key outside output root (including another local Windows volume), protection/hardlink checks, expected plaintext SHA/size, sync and authenticated readback. Failed/partial output has no success receipt and remains intact. No overwrite/delete/rename. Local building block, not source exporter or independent custody proof.
3. `baseline-package --authenticate-only`: strict bounded input and exact encrypted-artifact receipt coverage; source/snapshot/target/schemas/role/name/plaintext bound inside age; actual authentication to EOF and protected metadata rechecks. Existing integrity-only and static planner retain lower proof flags. No plaintext extraction, SQL execution or DB restore.
4. Bounded anchored identity reads (16 KiB + sentinel) replace an unbounded `ReadFile` after size observation. Core stream format unchanged; new package binding is opt-in, not a conversion of prior receipts.
5. Four default safety cases, three actual native cases (including compiled CLI), one ops CLI refusal case. Previous expected tests/shared gates preserved. Public runner builds the same-source CLI, requires all seven actual cases and prints storage progress. `RUN_PROTECTED_PACKAGE_QA.cmd` reuses verified tools without download.

## Backup and source

Starting hash `469e7d3b57b55a418994173ef73c3a992863bf6f8f72b6b2d8c12176c58ee6ee`. Before code edit: `backend-v3/private-backup/protected-package-20261117/source-before.zip`, 453 files, 1,216,688 archive bytes; SHA-256 `37513c4d4fd57aa455473a52764ab5ad4893664ec9f3dd31aaf361c8daf91bb2`. Reopened byte verification and final hash/ZIP CRC review PASS. Source/context backup, not DB backup.

Current tested executable hash **`126c217ad43a732e67d5d3b32473233ed5bd94385596a78872f0ba9b7b63371b`**. Current evidence: `protected-package-20261117/qa-C/summary.json`, command outputs/raw Go events/coverage/exact ops cases; independent `review.json`. Source unchanged during run C and review.

| Check | Result |
|---|---|
| Default Go | **79/79 PASS**, exact registry, including four new safety tests |
| Python ops | **93/93 PASS**, exact IDs, including actual auth CLI refusal/redaction/no-DB/no-mutation |
| Windows build, vet and operator binary builds | PASS |
| Linux cross-build and tagged-test cross-compilation | PASS_BUILD_ONLY; not runtime/race/remote CI |
| Original four actual crypto cases | **4/4 PASS on current source** |
| Three new native output/package cases | **3/3 FAIL during fixture setup**: `synthetic identity protected copy rejected: Access is denied.`; positive producer/package/CLI assertions not reached |
| Full mandatory actual suite | **4 PASS + 3 FAIL of 7 = FAIL / NOT_ACCEPTED**. No skip/relabel or 4/4 full-suite claim |
| Backup default statement coverage | **69.6% (694/997)**; new protected success paths not exercised; not deliverable coverage |
| Current full Playwright/API/DB/remote CI/actual restore | NOT_RUN |
| Full M01 / DLV-04 / DLV-06 | Partial/open/NOT_READY. Canonical M01 0/5, project 0/97 unchanged |

## Failure diagnosis

Read-only `IsTokenRestricted` observation confirms a **restricted Windows process token**. Native fixture directory creation succeeds, but writing its owner/LocalSystem-only private identity is denied. This is consistent with restricted-token access restrictions, **not proof that the implementation will pass in a normal terminal**. Further host/code issues cannot be ruled out until positive native QA runs.

The ACL was not widened to include sandbox groups, broad inheritance, Administrators or Everyone. No elevation, proxy/ACL repair, driver substitution, fake crypto/DACL success or test removal. Three failures remain on disk. A/B are retained FAIL_OR_INCOMPLETE on their own sources; C followed bounded key reads and cross-volume separation changes, not unchanged blind retry.

Independent review verified prior cases and canonical fields preserved, root `go.mod`/`go.sum` unchanged, backup intact and prior source edits limited to operator/backup scope. Business API/domain code, SQL schemas, legacy/frontend and shared DB are unchanged.

## Owner QA and next dependency

From a normal Windows terminal of the directory/key owner, without changing existing ACLs or editing source:

```powershell
D:\Projects\lms-v2\backend-v3\backend-go\qa\RUN_PROTECTED_PACKAGE_QA.cmd
```

Read-only storage admission → verified source backup → cached tool proof → same-source CLI build → **7/7 mandatory actual cases** → exact 79 default Go → vet → receipt. No download, DB/cluster or retention deletion. Missing tools/storage/permission or any failed case stops acceptance. Current 93 ops evidence is reusable only if source stays identical. Review case/source/backup/tool evidence before accepting `PASS_LOCAL_CRYPTO_SLICE_ONLY` for this bounded slice.

**Actual isolated synthetic SQL/custom/object restore has not started:** it depends on the protected producer/consumer positive gate. Do not rerun a full DB journey to solve this fixture access failure. User metadata/identity/scope preparation can continue; no real-snapshot/staging readiness inferred.

After native PASS: independent custody/public-only source-export design, actual snapshot/restore/reconciliation, long-term archive recovery contract, residual Playwright/remote CI, real identity/mapping and staging runtime remain. Existing 24-hour freshness is not long-term recovery; never retimestamp receipts or remove lab fences to bypass gates.

No DB connection/export/restore/migration, live/staging write, existing ACL/privilege modification, existing artifact deletion, commit or push. New synthetic fixture ACLs are supplied only at initial creation; framework cleanup concerns its own temporary fixtures. No new PostgreSQL cluster or large physical DB payload.
