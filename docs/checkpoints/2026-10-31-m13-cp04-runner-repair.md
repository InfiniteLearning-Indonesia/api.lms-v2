# M13-CP04 — dependency ordering repair and current evidence

> **Superseding accepted result:** run `20261031T100520Z-postgres-lab-246c640f9cf1` passes on the repaired source. [M13 closure](2026-10-31-m13-local-closure.md) verifies all cases, dependency worker, backup hashes and reconciliation. CP04 is now PASS_LOCAL_SYNTHETIC; the blocked/unverified sections below preserve earlier repair history.

> **Latest requested execution, 31 October:** attempted full `--consumer-closure-journey` on repaired source `12dd482ba4f51db23c49d54c67115159065263b56890a2275b202903dbadacfb`. Run `20261031T094956Z-postgres-lab-e2d1ad481eff` is **BLOCKED_LOCAL_POSTGRES**: `initdb` emitted restricted-token error 87 before backup, migration or API/DB cases. Source matches the passing local-check run below; full CP remains unverified. An ordinary-terminal run on this exact source is still required. No new CP has been opened and no case was waived.

**31 October 2026 — IMPLEMENTED_UNVERIFIED, full CP not closed.** Scope remains M13-CP04 local synthetic consumer closure. No new module, schema, real provider, shared database, deletion, commit or push.

## Confirmed failure and fix

Ordinary-terminal run `20261031T090236Z-postgres-lab-a4653b8fe7a1`, source `7abdb18e4900fc52089040448316716310cc553fc4e58a907b8b5658dfe80a6c`, finished **FAIL**: `M13-CP04 dependency regression missing`. Its summary records M13-CP01, CP02 and CP03 Go cases plus CP03 reconciliation PASS, but no `m03_cp05` result. This does not close CP04 or validate current source.

Actual cause in `qa/postgres_lab.py`: the consumer-closure block read `result['m03_cp05']` before the later worker regression block had called `TestSyntheticWorkerLifecycle`. Moved the consumer block after the worker block, preserving both required checks (`m03_cp05` PASS and `m13_cp03_reconciliation` PASS). Required worker regression, backup+restore, and all dependency API/DB assertions remain enabled.

Added mandatory `M13-CP04-runner-order` regression, described in the dated amendment to [the original frozen scope](2026-10-30-m13-cp04-scope.md). It executes the actual AST worker/closure blocks using external-operation stubs, checks the worker→consumer execution order, and requires denial when worker evidence is absent or AI reconciliation fails. It is a runner test, not a Playwright/DB test. CP04's direct manifest count is now **4**: three existing Go cases plus one new Python runner case. No acceptance assertion was removed.

## Current local verification

Run `20261031T094606Z-m13cp04-local-3fc6c437`, source fingerprint `12dd482ba4f51db23c49d54c67115159065263b56890a2275b202903dbadacfb`, status **PASS_LOCAL_CHECKS_ONLY**, source unchanged during checks. Raw output and summary are under `lms-v3-qa-artifacts/<run-id>/` outside the source tree.

| Check | Result |
|---|---|
| Manifest validation and actual Go/Python case existence | PASS |
| `M13-CP04-runner-order` via pytest evidence plugin | 1/1 PASS |
| `go test -count=1 ./...` | PASS |
| `go vet ./...` | PASS |
| Tagged compilation and execution of CP04's three direct Go cases | 3/3 PASS, exact case-set verification |

These checks exercise four direct CP04 cases locally; **full checkpoint acceptance remains pending** because the fresh PostgreSQL/Playwright dependency run on this source has not been completed. Code coverage is not claimed or measured by this local check. No database was contacted and no backup/migration was attempted during these checks.

## Next required action

Run the existing `--consumer-closure-journey` in ordinary Windows PowerShell. It creates a fresh synthetic cluster and runs full dependencies, including required SQL/custom backups and isolated restore gates before dependency migrations. `backend-v3/portability/RUN_M13_CP04_QA.ps1` is a workspace-relative convenience wrapper; the equivalent Python command is:

```powershell
Set-Location 'D:\Projects\lms-v2\backend-v3\backend-go'
$env:GOCACHE = 'D:\Projects\lms-v2\lms-v3-qa-artifacts\go-cache'
$env:GOTELEMETRY = 'off'
.\.venv\Scripts\python.exe -m qa.postgres_lab --create-local-synthetic-cluster --pg-bin 'C:\Program Files\PostgreSQL\18\bin' --consumer-closure-journey --artifact-parent 'D:\Projects\lms-v2\lms-v3-qa-artifacts'
```

Review final source fingerprint, exact CP04 case sets, M03 worker cases, all M13 API/DB results, backup/restore receipts, reconciliation and final stop outcome before changing CP04 status. Missing/skipped cases, source changes or any required regression failure block closure. M14 is not executed by this CP04-only journey. If all bounded CPs pass, M13 may be described as **4/4 local synthetic CP slices**; real-provider/secret-manager/privacy/recipient/shared-runtime/remote gates still prevent a canonical production-ready M13 claim.

## Context and backup currency

Active evidence/status is maintained in `backend-go/docs`; workspace continuation notes live in `backend-v3/portability/RESUME_CONTEXT_2026-10-31.md` and `backend-v3/START_HERE.md`. Private backup `20261031T092053484506Z` predates this repair. Keep it immutable as a pre-repair recovery point; it does not contain the repaired runner or this note. A future backup must capture these newer working files separately.

**Incremental backup recorded:** `backend-v3/private-backup/cp04-repair-20261031` now contains 29 selected repair/context/local-evidence files (91,340 compressed bytes), with archived-member SHA-256 verification. Apply it after the base snapshot to restore the repaired executable inputs. Later Markdown annotations such as this backup-record paragraph are newer than the incremental snapshot; use the live handoff for current navigation.
