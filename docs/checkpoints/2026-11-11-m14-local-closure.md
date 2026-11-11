# M14 — local synthetic closure accepted

**11 November 2026: M14-CP04 PASS_LOCAL_SYNTHETIC; M14 4/4 bounded local CP slices accepted. Canonical M14 remains 0/5 fully verified deliverables.** This closes the current backend local lane; production, real-source rehearsal, frontend and shared quality gates remain separate.

## Accepted evidence and completion boundary

- Run: `20261111T093737Z-postgres-lab-6e4bc487c4d6`, final `PASS_SYNTHETIC_ONLY`; launcher final `PASS_REVIEWED_LOCAL_SYNTHETIC`.
- Tested and current source: `cb0a9af604011f90e6bc14844f637e5e0bb094104b509ff03905407feae0a7bc`.
- Summary SHA256: `b6dde965a1d5976b2279e1ac50c71802353742d726cfadff4ed50dbecea6df26`.
- [Launcher outcome](../../../private-backup/m14-cp04-20261111T093419788503Z/outcome.json), [full evidence review log](../../../private-backup/m14-cp04-20261111T093419788503Z/full-evidence-review.log), [read-only review receipt](../../../portability/m14-review-20261111T095404196248Z.json).
- [Closure review](../../../private-backup/m14-local-closure-20261111T120923245572Z/closure-review.json): source/summary match; **47 CP04 artifacts** and **80 logical backup files** from **40 restore receipts** independently rehashed. No database was contacted for documentation closure.
- Watcher independently reached [READY_FOR_ACCEPTANCE_REVIEW](../../../private-backup/m14-watch-20261111T094057770398Z/result.json); its draft was not an automatic status acceptance. The implementation/evidence/remaining gates have now been read and local closure recorded here.

| Scope on accepted run | Go cases | Playwright API cases | Result |
|---|---:|---:|---|
| M14-CP01 projections/integrity | 2/2 | 3/3 | PASS |
| M14-CP02 API contracts | 1/1 | 2/2 | PASS |
| M14-CP03 pagination/export | 3/3 | 4/4 | PASS |
| M14-CP04 measured performance/closure | 4/4 | 2/2 | PASS |
| **Direct frozen M14 cases** | **10/10** | **11/11** | **21/21 PASS** |

CP04's four Go cases are the three native load-client contract/metrics cases and `TestSyntheticReportPerformancePlanAndPool`; its two API cases are benchmark seed/auth/execution and independent load acceptance. The extra database-free parser regression is covered by Go unit preflight and is not counted as another frozen CP04 deliverable.

## Measured performance

The unchanged workload is `LOCAL_SYNTHETIC_INSTRUMENTED_CLOSED_LOOP`: small **2 items / grade version 1**, boundary **200 items / grade version 3**, report/page/export, concurrency 1 and boundary concurrency 4. Nine phases have 120 measured requests each, **1,080 total**, plus warmups. No missing/failed request is accepted; frozen latency/response-size/error gates passed.

| Workload / endpoint / concurrency | p95 ms | p99 ms |
|---|---:|---:|
| small / report / 1 | 3.4242 | 3.8550 |
| small / page / 1 | 3.3698 | 3.8806 |
| small / export / 1 | 5.5032 | 6.2994 |
| boundary / report / 1 | 21.6662 | 22.9047 |
| boundary / report / 4 | 40.1687 | 100.1018 |
| boundary / page / 1 | 4.7250 | 5.1616 |
| boundary / page / 4 | 12.2567 | 14.9416 |
| boundary / export / 1 | 5.4898 | 5.9951 |
| boundary / export / 4 | 12.7041 | 17.5818 |

These are observed local, instrumented Windows/amd64 Go 1.27.1 results on an 8-CPU host. They are not a production SLA, arbitrary-scale capacity proof, browser UX score, or total-module code coverage. No query/index tuning or threshold relaxation was needed or claimed.

## Native DB, pool, audit and dependency evidence

- The actual shared `releasedGradesSQL` query was explained under the synthetic runtime role. Root rows are **2.00** and **200.00**; execution times **0.184 ms** and **1.478 ms**, both within the unchanged **500 ms** budget. Raw small/boundary plans and the combined DB/pool receipt are retained.
- Runtime pool cap **4**. Fifth acquisition cancelled at observed **50.0523 ms** for the 50 ms deadline; one cancelled acquisition recorded. Recovery succeeded within the 1,000 ms bound; observed recovery 0 ms, acquired connections after release **0**.
- **384 export audit/outbox pairs** matched the measured and warmup exports; release/revision history checks passed.
- All **29** top-level reconciliation results PASS, including M14-CP04. Required M03 worker and M13 consumer/closure/runner regressions PASS after CP04, as verified by the final reviewer.
- [Owner preparation](../../../private-backup/m14-cp04-20261111T093419788503Z/preparation.json) passed **68/68 ops cases**, Go unit, go vet, tagged DB compile, load build and manifest validation. Those 68 checks are separate from the 21 frozen M14 cases.
- Normal PostgreSQL stop succeeded; no PID marker remained. Lossless post-stop compression and the serial retention hook executed; the hook performed no additional disposal on this run. Native lifecycle hook execution is now evidenced on the fixed source, beyond the earlier fixture-only tests.

## Backup, history and data safety

Forty per-installment receipts report actual isolated restore verification. Referenced SQL/custom files still match SHA256; together the 80 backup files are **70,490,440 logical bytes**. These are synthetic migration-stage snapshots, not a final-state raw cluster or production backup. Keep the original run, logical backups and receipt chain.

The earlier run `20261111T084515Z-postgres-lab-dc2f748e9472` stays FAIL. [PG18 parser repair](2026-11-11-m14-cp04-plan-parser-fix.md) was followed by this fresh run; neither its original summary nor the acceptance budget was rewritten to obtain PASS.

Shared staging/production access NONE; no real data copy, rollout, database deletion, Git commit or push from closure. Schema and source were not changed during acceptance review. QA snapshot after stop: **1 DISPOSED, 53 CANDIDATE, 3 KEEP, 14 HOLD**. Historical candidates remain outside the future-only automation cutoff; local closure does not authorize a backlog purge.

## What is closed and what remains open

All four M14 CPs now have passing source-bound evidence for their local backend scope: actor boundaries, minimized/versioned contract, read-your-writes, pagination/stale cursors/private export, and frozen measured performance with query/pool/audit checks.

Backend handoff entry for the later frontend alignment is the existing [reporting OpenAPI contract](../../api/openapi/reporting-v1.json), supported by the CP02 contract and CP01/03/04 request fixtures. No frontend UI or browser usability acceptance is added by this closure.

Canonical M14 remains **0/5**, project M00-M18 **0/97** under the existing full contract. Applicable shared-runtime/key/identity policy, legacy/source inventory and parity decisions, remote CI/container/operational evidence, frontend integration/usability/accessibility, production capacity/SLA and reviewer/academic/ops quality gates still require their own evidence. Do not convert 4/4 CPs or 21/21 cases into a global implementation percentage or production-readiness claim.

## Next dependency decision

No new CP is started by recording this closure. Review the remaining M11 attendance/permit policy and M12 legacy-mapping gates, or M00/M01/M02 real-baseline readiness before selecting the next authorized lane. M16 rehearsal requires verified source/target identity, real backup/restore proof and reviewed M15 mapping; M17 additionally depends on rehearsal, ownership and frontend readiness. Completing M14 does not automatically satisfy those inputs.

Milestone source/context and accepted evidence/SQL/custom backups are retained through the [closure backup manifest](../../../private-backup/m14-local-closure-20261111T120923245572Z/manifest.json). It excludes physical clusters, executables, caches, Git and secrets; it does not replace a machine or production backup.
