# CP06–CP10 — scope dan status pilihan

Continuation of [CP01–CP05](2026-09-06-five-checkpoints.md). These slices do not replace canonical Mxx acceptance. This file retains the original broad scope; local acceptance is narrower and explicitly recorded below.

Latest 7 September: [CP08–CP10 local implementation and final QA](2026-09-07-cp08-cp10.md) **PASS_SYNTHETIC_ONLY** after authorization. CP08 operational backup remains BLOCKED; CP09 non-lab runtime/worker and CP10 full identity/lifecycle remain PARTIAL. CP07 stays PARTIAL/BLOCKED_INPUT: both available env configurations are staging; production/provider inputs remain unavailable. Previous [CP06–07 result](2026-09-07-cp06-cp07.md) retained. **Next selectable work: [CP11–CP15 brief](CP11-CP15_BRIEF.md), not yet started.**

## CP06 — Close isolated QA and evidence gaps (M01/M03)

- Outcome: execute/review fresh synthetic PostgreSQL lab, backup/restore proof and transaction integration; close remaining non-root/container proof where tooling is available.
- Work: if CP05 lab remained unexecuted, authorize/run it first; strengthen archive corruption/missing-object tests, coverage collection and denial assertions. Avoid repeating already successful proof unnecessarily; verify source hashes.
- QA: actual rollback/no-partial-write, pool timeout/cancel/recovery, failed backup gate, bounded shutdown. Missing environment is BLOCKED, not skipped PASS.
- Dependency: local-cluster/container authority and tools; no production required. Retain all data/backup artifacts.
- Complexity: Medium. Exit: scoped real integration evidence, not full M01/M03 acceptance.

## CP07 — Close environment identity and security disposition (M00; M02 handoff)

- Outcome: verified live/staging identities and clear disposition of public-table API grants.
- Work: actual deployment secret references/SHA, approved metadata checks on each target, least-privilege account provisioning plan, Data API exposed-schema review, full jobs/storage/capacity inventory and owner/reviewer handoff.
- QA: different actual targets, correct CA/role, no sensitive values in reports; missing provider settings cannot be inferred from SQL grants alone.
- Dependency: production reference/access and owner decisions. Privilege changes/legacy patches need separately scoped approval and safe change procedure; this brief is not that authorization.
- Complexity: Medium with external dependencies. No public exploit tests or automatic revoke/RLS changes.

## CP08 — Real backup/restore and operational gate (M01)

- Outcome: approved source/target snapshots with protected SQL/custom artifacts and isolated restore proof.
- Work: replace declaration-only preflight with actual capacity/ACL/encryption/tool checks, complete backup scope including object bytes as applicable, snapshot-consistent reconciliation, verified key recovery and run-bound receipts.
- QA: corrupt/truncated/missing data, wrong target, stale receipt, restore mismatch and unsafe artifact access block progress. Test first on synthetic fixtures before real export.
- Dependency: CP07 for the environment accessed, authorized export/destination/isolated restore. Never overwrite staging, delete rows/schema/files, or replay active session/provider credentials into development.
- Complexity: Large. Exit: full M01 evidence only when every mandatory D is satisfied.

## CP09 — Actual runtime DB/audit/outbox/migration foundation (M03)

- Outcome: Go runtime uses restricted schema/role and atomically persists domain+audit+outbox; migration ledger has backup/checksum/lock guards.
- Work: production-grade TLS/pool configuration, explicit repositories, no runtime DDL/DELETE, safe commit-uncertainty behavior, outbox worker protocol, version/lock/idempotency handling and full integration evidence.
- QA: concurrency, pool pressure, privilege denial, atomicity failures, stale backup and checksum conflicts. Current synthetic three-table transaction is a fixture, not the durable business implementation.
- Dependency: verified M01 before migration; domain/security decisions. Independent code/tests can precede shared staging operations.
- Complexity: Large. No automatic deployment or production data use.

## CP10 — First real Class journey (M04 + M05)

- Outcome: Admin creates name-only Class, adds multiple Teachers/Students; access follows the resource's Class, not global Mentor/program/batch.
- Work: scoped slice of identity/session/capability and Class/enrollment commands; enrollment separate from role, cohort/program optional, archive/revoke history retained, no hard deletion.
- QA: real actor×Class matrix, foreign/nested IDs, mass assignment, disabled account, retry/duplicate enrollment and transaction failures. M04 fixtures alone cannot close M04↔M05 integration.
- Dependency: CP09 foundations and reviewed identity authority/bridge policy. No frontend work; no claim that this slice completes all M04/M05 criteria.
- Complexity: Large. Exit: first backend business journey, not full LMS or production readiness.

Execution always ends with per-checkpoint QA, cumulative regression, current hashes, verified/partial/blocked status and next input. M02 legacy containment remains a parallel authorized security workstream; Go progress never substitutes for deployed legacy mitigation.
