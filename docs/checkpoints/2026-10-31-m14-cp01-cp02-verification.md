# M14-CP01/02 — current verification gate

> **Latest failed-run diagnosis:** `20261031T103230Z-postgres-lab-e14518526f85` passed all five CP01 Go/API cases but lacked API coverage instrumentation for reporting, then exceeded the 15-second shutdown wait. Both runner issues are repaired; [runtime repair evidence](2026-10-31-m14-qa-runtime-repair.md) supersedes the unchanged-source preflight below. Fresh full QA is still required; CP03 has not started.

31 October 2026. User authorized verification of M14-CP01/02, followed by M14-CP03 **after** their acceptance. CP03 has not been started by this preflight.

## Read-only review and local checks

- Compared actual `internal/reporting/report.go` and `http.go`, OpenAPI reporting-v1 contract, Go tests, Python cases and both original scope files/manifests.
- `qa.review_m13_m14_manifests`: PASS, CP01 5 mapped cases and CP02 3 mapped case entries exist. `TestReportingContract` is shared, so these represent 7 unique test IDs across the two CPs, not 8 independent tests.
- `go test -count=1 ./internal/reporting`: PASS (contract and HTTP boundary tests; no DB).
- `go test -count=1 -tags=qaintegration -run=^TestReportingContract$ ./internal/reporting`: PASS (tagged compilation and contract test; DB integrity test not executed).
- CP01/02 Python collection selects 3 and 2 Playwright API cases respectively. Collection is not test execution or API verification.
- Executable source was not changed during this verification preflight. Current fingerprint remains `12dd482ba4f51db23c49d54c67115159065263b56890a2275b202903dbadacfb`, the source accepted for M13 local closure. Markdown and review utilities outside the Go tree do not change that fingerprint.

## Still required before CP03

A full fresh `--reporting-contract-journey` from ordinary Windows PowerShell. Existing sandbox PostgreSQL restricted-token error 87 prevents that environment from completing `initdb`; repeated unchanged sandbox runs are not a substitute for terminal evidence. The user has been given the full terminal command. Keep source/manifest inputs stable during the run.

Review exact CP01/02 Go/API manifest case sets, source fingerprint, reporting coverage evidence, all dependency suites including M13 CP04, backup/restore hashes, reconciliation and final stop outcome. `backend-v3/portability/review_m14_contract_run.py <run-id>` provides a read-only review helper; it rejects non-PASS, stale source, missing cases or dependency failures and creates its own new review receipt. It does not create a QA PASS without a real run.

Until that review succeeds, CP01/02 remain **IMPLEMENTED_UNVERIFIED**, M14 locally verified count **0/4**, canonical **0/5**. No new migration, export, pagination, staging/live access, deletion, Git operation or deployment occurred in this preflight. Original frozen scope files are unchanged.

## Authorized continuation after the gate

After actual PASS evidence is accepted, update active status and freeze CP03's actor scope, cursor/sort/filter rules, private export policy, row/byte limits, audit/idempotency behavior, migration needs and mandatory QA manifest before implementation. Do not infer CP03 permission to weaken earlier contracts or skip the verification gate.
