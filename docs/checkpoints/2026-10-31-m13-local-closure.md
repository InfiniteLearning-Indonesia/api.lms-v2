# M13 — local synthetic closure

**31 October 2026: M13-CP04 PASS_LOCAL_SYNTHETIC; M13 4/4 bounded local CP slices verified. Canonical M13 remains 0/5 fully verified deliverables.**

## Accepted evidence

- Run: `20261031T100520Z-postgres-lab-246c640f9cf1`, final `PASS_SYNTHETIC_ONLY`, retained cluster, ordinary Windows terminal.
- Tested/current executable source fingerprint: `12dd482ba4f51db23c49d54c67115159065263b56890a2275b202903dbadacfb`.
- Raw summary SHA-256: `3c963cffe1c2e595b6b6401bb233c5d5eb9de8431282055f0bc0949dbae1c39e`.
- Raw evidence directory: workspace `lms-v3-qa-artifacts/<run-id>/`.
- Read-only review receipt: workspace `backend-v3/portability/cp04-review-20261031T101821197640Z.json`. It validates exact manifest case sets, current source, worker dependency, reconciliation, backup file hashes and final run outcome. No database was contacted during review.

| Scope | Go cases | Playwright API cases | Python runner cases | Result |
|---|---:|---:|---:|---|
| M13-CP01 durable delivery | 2/2 | 2/2 | — | PASS |
| M13-CP02 notification safety | 2/2 | 2/2 | — | PASS |
| M13-CP03 AI suggestion | 3/3 | 4/4 | — | PASS |
| M13-CP04 consumer closure | 3/3 | Dependency API suites above | 1/1 | PASS |

M13 has **19/19 direct mandatory cases PASS** across CP01–04: 10 Go, 8 Playwright API and 1 runner case. This is the bounded manifest case count, not a global backend coverage percentage. M03 `TestSyntheticWorkerLifecycle` also passed as a separate mandatory dependency, with its exact manifest cases checked. No missing/skipped required M13 case was accepted.

The CP04 direct Go coverage profile covers 61/73 statements of `internal/outbox/worker.go` (**83.6%**). Its mock-queue tests cover 0/71 `store.go` statements in that particular profile; database behavior is exercised by separate dependency integration suites. Do not present 83.6% as total outbox, M13, or backend coverage.

## Backup, restore and data boundaries

- **39** retained backup/isolated-restore receipts were reviewed. Each reports `VERIFIED_FIXTURE_SCOPE`; referenced SQL/custom artifacts exist and match their recorded SHA-256.
- All 27 top-level reconciliation results are PASS, including M13 CP01–03 and academic/migration dependencies.
- Negative backup checks are PASS. Final run outcome is PASS, including the runner's stop handling and unchanged-source check.
- All schema/data operations in this journey were confined to a newly created local synthetic cluster, with dependency backup/restore gates. CP04 itself adds no schema.
- Shared staging/production access NONE; production backup NOT_EXECUTED; database deletion NONE; cluster retained. No deployment, Git commit or push.

## What closed

The bounded worker/consumer lifecycle, receipt deadline and unknown-outcome handling, retry/shutdown regression, rehearsal notification isolation and non-authoritative AI contract now have a successful combined local run. The earlier failure `M13-CP04 dependency regression missing` was caused by runner ordering and was repaired with an additive mandatory regression. Prior failed runs and [repair evidence](2026-10-31-m13-cp04-runner-repair.md) remain historical records; they are not relabeled PASS.

## What remains open for canonical M13

Actual provider adapters and provider-specific idempotency, actual secret-manager handling, privacy/data-export and recipient policy, shared-runtime wiring, remote CI/container execution and applicable shared quality gates still need evidence in their intended environments. Local CP closure is not authorization for staging/live copy, production traffic, real outbound delivery or deployment. M00–M18 canonical deliverable count stays 0/97 under the existing full acceptance contract.

## Next dependency lane

M14-CP01 and CP02 already have implementation but no accepted full-run evidence. Verify those slices using `--reporting-contract-journey` before advancing to M14-CP03. This M13-only run does not execute or close M14. Do not start another CP merely by publishing this closure note; wait for the user's next implementation scope.

## Context and backup currency

Current reading entry is workspace `backend-v3/START_HERE.md`; continuation instructions are `backend-v3/portability/RESUME_CONTEXT_2026-10-31.md`. Existing base backup and CP04-repair incremental backup predate this accepted run and closure note. They remain immutable historical snapshots; raw new run evidence and these newer Markdown updates must be retained separately or included in a future incremental backup before transfer.
