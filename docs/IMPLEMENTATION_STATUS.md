# LMS v3 backend — Implementation status

## Posisi pekerjaan per M — 9 September 2026

**M adalah unit capaian utama; CP adalah bagian di dalam M.** Lihat [peta lengkap M00–M19 → CP](MODULE_CHECKPOINT_MAP.md) dan bagian “Peta pekerjaan” di setiap module brief. Canonical D verification dan raw/generated QA sections di bawah tetap mempunyai denominator acceptance sendiri.

| M yang sudah disentuh | CP rencana | Bagian teruji lokal | Posisi/sisa paling dekat |
|---|---:|---:|---|
| [M00](modules/M00.md) | 4 | 1/4 | Source inventory/evidence lokal ada; production/deployed identity, full inventory/capacity dan handoff belum lengkap |
| [M01](modules/M01.md) | 4 | 1/4 | Synthetic backup/restore teruji; real host/key/object protection, backup nyata dan closure tersisa |
| [M02](modules/M02.md) | 4 | 0/4; CP01 parsial | Security disposition bukan mitigation; perlu scoped legacy fixes dan deployment verification |
| [M03](modules/M03.md) | 6 | 2/6 | **Berikutnya M03-CP03** reusable migration runner, kemudian CP04 API runtime, CP05 worker, CP06 QA/closure |
| [M04](modules/M04.md) | 5 | 2/5 | Bridge/session dan Class fence lokal teruji; real owner/provisioning, refresh/browser dan full ancestry tersisa |
| [M05](modules/M05.md) | 5 | 2/5 | Core Class/participation/lifecycle teruji; grouping/mentor/invited/retake serta content integration tersisa |

M06–M14 masing-masing 0/4, M15 0/5, M16–M17 0/4, M18 0/3; M19 belum frozen (N/A). Ini jumlah potongan scope teruji, **bukan persentase effort atau progress full module**. Tidak menjumlahkan numerator antar M karena shared evidence dan ukuran CP berbeda. Full M00–M18 masih 0/97 D pada evidence terakhir; tidak ada D diubah/diluluskan oleh restrukturisasi ini. Source/test manifests tetap sama; tidak menjalankan QA aplikasi atau operasi DB baru.

**Latest CP11–CP12, 8 September:** [VERIFIED_LOCAL_SLICES](checkpoints/2026-09-08-cp11-cp12.md), **4/4 deliverable lokal masing-masing**. CP11 API 6/6; CP12 + CP11 regression 9/9; CP10 API 6/6; M00 20/20; M03 23/23; 23 default Go cases + Linux race/non-root PASS. Measured coverage union: identity 86.01%, Class 83.85%, access 100%, seluruh profile 70.09%. Full M04/M05 gates tetap belum terverifikasi. Root Go dan CP08–CP10 published pada `edac5af`; user kemudian mengotorisasi publikasi CP11–CP12 ke `be-v3` setelah QA, dengan raw artifact tetap lokal. Ringkasan numerik CP08–CP10 di bawah adalah histori, bukan source terbaru.

[Indeks](README.md) · [QA contract](QA_AUTOMATION_PLAN.md) · [Master plan](../../../ImplementationPlan.MD)

Baseline brief: 5 September 2026; checkpoint implementasi: 6 September 2026 WIB. **M00 inventory source dan M03 Go/QA bootstrap kini executable dan diuji lokal setelah otorisasi pengguna.** Full module, DB identity/backup/migration dan release belum verified. Owner/reviewer operasional belum ditetapkan. Panduan command: [service README](../README.md).

**Histori CP08–CP10:** [laporan lama](checkpoints/2026-09-07-cp08-cp10.md) menyimpan PASS_SYNTHETIC_ONLY sebelum identity/lifecycle berikutnya. Hasil terbaru CP11–CP12 ada di atas. CP07 [security/environment handoff](baseline/CP07_ENVIRONMENT_SECURITY_HANDOFF.md) tetap menunggu actual production/provider/owner inputs. CP13–CP15 yang semula diusulkan sekarang dipetakan ke CP milik M; bukan urutan global baru. Generated blocks di bawah mempertahankan evidence per package.

**7 September update:** [CP05 closed for synthetic scope](checkpoints/2026-09-07-cp05-closure.md): local PostgreSQL backup/restore proof and 3/3 transaction-suite cases PASS; transaction package coverage 100%, not whole-backend coverage. Server stopped; all synthetic data/backups retained. Production/staging backup and full module gates remain pending.

Handoff terperinci, exact tested hashes, failure history dan input berikutnya: [checkpoint 6 September](checkpoints/2026-09-06-bootstrap.md).

Riwayat sebelum CP01–CP05: [staging inspection](checkpoints/2026-09-06-staging-inspection.md), lalu [CA terverifikasi](checkpoints/2026-09-06-staging-ca-verified.md). Saat itu M00 16/16 dan M03 19/19 checks, 13 Go cases dan coverage 67,4%. **CP01–CP05 bootstrap:** M00 19/19, M03 22/22, 17 Go cases, coverage 70,7%; Linux race/non-root proof tersedia. Full modules tetap incomplete; production backup/migration belum dijalankan. CP05 real synthetic DB execution kini PASS, terpisah dari bootstrap counts.

## Dashboard paket

**Git layout correction 8 September:** user authorized backend-only root and push. Worktree root is now `backend-v3/backend-go`; root CI paths and evidence binding updated. [Git handoff and fresh regression](checkpoints/2026-09-08-git-root.md) supersede old layout/publication descriptions. This is not CP11 implementation or CP12 authorization.

**Riwayat kickoff CP11:** semula BLOCKED_DECISION; kelanjutan user mengotorisasi CP11–CP12 dan bridge untuk local slice. [Authority ADR](adr/CP11_IDENTITY_AUTHORITY.md) serta [hasil terbaru](checkpoints/2026-09-08-cp11-cp12.md) menggantikan status kickoff. Real legacy/provider integration tetap belum verified.

Coverage adalah fully verified D / planned mandatory D, bukan jumlah file/tes yang dibuat. Denominator M19 belum frozen. Full-suite pass rate masih N/A; bootstrap checks dan measured Go statement coverage tercatat terpisah pada run evidence. Generated sections di bawah adalah latest checkpoint per paket.

| Paket | Implementation | QA | Capaian D | Next gate/blocker |
|---|---|---|---|---|
| [M00](modules/M00.md) | IN_PROGRESS | PARTIAL_QA; full gate BLOCKED | 0/5 (0%) | Source inventory, staging TLS/public catalog tersedia; deployed SHA/dua DB/least privilege/full inventory/capacity/operator pending. |
| [M01](modules/M01.md) | IN_PROGRESS (synthetic backup/recovery) | PARTIAL_INTEGRATION; full gate BLOCKED | 0/5 (0%) | CP08 actual synthetic dump encryption/decryption/restore verified; real source/target, ACL, key custody and full scope pending. |
| [M02](modules/M02.md) | NOT_STARTED | NOT_RUN | 0/6 (0%) | Live mitigations belum diverifikasi; companion frontend security belum berada dalam scope backend. |
| [M03](modules/M03.md) | IN_PROGRESS | PARTIAL_QA; full gate BLOCKED | 0/5 (0%) | CP09 restricted pool, fixed fixture migration ledger, atomic audit/outbox and lease protocol tested; general migration/runtime wiring, worker, full QG, container/remote CI pending. |
| [M04](modules/M04.md) | IN_PROGRESS (CP11–CP12 slice verified) | LOCAL_IDENTITY_API_DB_PASS | 0/5 full (0%) | Verified synthetic bridge/session/CSRF/projection/local admin/fence; real legacy provisioning/reset/global owner, browser and full QG pending. CP11 slice 4/4. |
| [M05](modules/M05.md) | IN_PROGRESS (CP12 slice verified) | LOCAL_CAPABILITY_API_DB_PASS | 0/5 full (0%) | Teacher manages Students; close/reopen/suspend/reactivate/end/rejoin/history/last-teacher/concurrency verified locally. Grouping/retake/content readiness/operational QG pending. CP12 slice 4/4. |
| [M06](modules/M06.md) | NOT_STARTED | NOT_RUN | 0/5 (0%) | File/context API dan isolated object fixture belum tersedia. |
| [M07](modules/M07.md) | NOT_STARTED | NOT_RUN | 0/5 (0%) | Content/platform dependencies belum implemented. |
| [M08](modules/M08.md) | NOT_STARTED | NOT_RUN | 0/5 (0%) | Assignment/identity/DB implementation belum tersedia. |
| [M09](modules/M09.md) | NOT_STARTED | NOT_RUN | 0/5 (0%) | Grade rules/data-owner decisions dan Go prerequisites belum verified. |
| [M10](modules/M10.md) | NOT_STARTED | NOT_RUN | 0/5 (0%) | Completion policy dan domain prerequisites belum verified. |
| [M11](modules/M11.md) | NOT_STARTED | NOT_RUN | 0/5 (0%) | Attendance policy/mapping dan prerequisite modules belum tersedia. |
| [M12](modules/M12.md) | NOT_STARTED | NOT_RUN | 0/5 (0%) | Period policy dan Go prerequisites belum verified. |
| [M13](modules/M13.md) | NOT_STARTED | NOT_RUN | 0/5 (0%) | No worker/provider test harness or validated integration policy. |
| [M14](modules/M14.md) | NOT_STARTED | NOT_RUN | 0/5 (0%) | Core modules, benchmark workload/threshold dan actual API belum available. |
| [M15](modules/M15.md) | NOT_STARTED | NOT_RUN | 0/6 (0%) | Target schemas/source identity/backup approvals belum verified. |
| [M16](modules/M16.md) | NOT_STARTED | NOT_RUN | 0/5 (0%) | BLOCKED_INPUT: source/target identity, backup/CA/access/data-copy approvals. |
| [M17](modules/M17.md) | NOT_STARTED | NOT_RUN | 0/5 (0%) | Frontend and identity integration, data-owner approvals, rehearsal and deployment readiness pending. |
| [M18](modules/M18.md) | NOT_STARTED | NOT_RUN | 0/5 (0%) | Pilot belum selesai; external release/ops acceptance pending. |
| [M19](modules/M19.md) | DEFERRED | NOT_RUN | N/A | Belum ada optional feature yang dipilih; denominator final coverage belum dibekukan. |

**Total M00–M18:** 0/97 full deliverables verified; M00/M03 bootstrap serta CP08–CP10 real synthetic DB/API slices diuji, full-package acceptance masih incomplete. Numerator tidak dinaikkan dari partial acceptance. Scope baseline/denominator tidak berubah. M19 dikecualikan secara terlihat karena feature belum dipilih, bukan untuk menaikkan score.

## Dashboard gelombang

| Gelombang | Implementation | Cumulative QA | Capaian D | Release |
|---|---|---|---|---|
| [B01](batches/B01.md) | IN_PROGRESS (M00/M01 local portions) | PARTIAL_BOOTSTRAP | 0/16 | NOT_VERIFIED |
| [B02](batches/B02.md) | IN_PROGRESS (local platform/Class slice) | PARTIAL_API_DB | 0/15 | NOT_VERIFIED |
| [B03](batches/B03.md) | NOT_STARTED | NOT_RUN | 0/15 | NOT_VERIFIED |
| [B04](batches/B04.md) | NOT_STARTED | NOT_RUN | 0/10 | NOT_VERIFIED |
| [B05](batches/B05.md) | NOT_STARTED | NOT_RUN | 0/20 | NOT_VERIFIED |
| [B06](batches/B06.md) | NOT_STARTED | NOT_RUN | 0/11 | NOT_VERIFIED |
| [B07](batches/B07.md) | NOT_STARTED | NOT_RUN | 0/10 | NOT_VERIFIED |
| [B08](batches/B08.md) | DEFERRED | NOT_RUN | N/A | NOT_VERIFIED |

## Operasional bukan status kode

| Area | Status aktual checkpoint |
|---|---|
| Root service | Ditetapkan pengguna: `backend-v3/backend-go` |
| Branch/push | Worktree `be-v3`; foundation af1a669/5f24ec8 previously pushed; CP08–CP10 not committed/pushed; user deletions dipertahankan |
| Legacy baseline | Exact backend checkout pair dan semua 64 TS source per checkout diinventori; deployed SHA tetap UNKNOWN |
| Dua DB berbeda | User mengoreksi kedua env yang tersedia menunjuk staging; production reference aktual belum tersedia/terverifikasi |
| Backup valid/restore proof | CP08 synthetic-only SQL/custom + cipher recovery + fresh restore VERIFIED; shared source/target, Windows ACL, independent key custody/full scope pending; failed derivatives bukan backup valid |
| Production→staging import | NOT_EXECUTED |
| DB migration | NOT_EXECUTED |
| Go API / QA runner | LOCAL_BOOTSTRAP_IMPLEMENTED; M00/M03 supported; operator staging TLS/public-catalog read-only observation berhasil; business API belum ada |
| Legacy security deployed | NOT_VERIFIED |
| Pilot/rollout | NOT_EXECUTED; frontend dan ops gates pending |
| Frontend development/testing | Di luar scope; belum dibuat atau diverifikasi |

## External gates yang tetap terlihat

- M02-X01: companion frontend legacy session/XSS/browser compatibility. Backend security PASS tidak meluluskan full web security.
- M14-X01: frontend integration/usability/accessibility menyusul; API contract readiness hanya backend claim.
- M16-X01: actual authorized source snapshot dan data/security/ops review diperlukan, bukan synthetic-only evidence.
- M17-X01: real frontend/session/old-link readiness, maintenance/support approval sebelum pilot.
- M18-X01: actual wave approval/acceptance dan frontend-compatible rollout sebelum full release.
- M19: selected feature/ADR dan granular acceptance manifest belum ada; tetap DEFERRED.

## Checkpoint aktif

| Field | Nilai |
|---|---|
| Pekerjaan saat ini | CP06 synthetic QA selesai; CP07 partial handoff menunggu external inputs; tidak mengubah legacy/frontend |
| Implementation package aktif | M00 dan independent local portion M03; belum ada full module complete |
| Last completed QA run | Lihat generated M00/M03 sections; private `.qa-artifacts/<run_id>/summary.json` |
| Tested commit/tree/schema hash | Source/manifest/binary hashes pada summary; public-catalog hash tersedia di checkpoint CA, bukan full schema/data hash |
| Migration receipt/backup ID | Shared migration/backup NONE; synthetic backup receipts pada CP06 run `20260906T182956Z-postgres-lab-781490e5578f` |
| Next safe action | Tutup CP07: production/deployed refs, dedicated read-only role, Data API settings, full jobs/storage/capacity inventory dan owner/reviewer; CP08 perlu otorisasi baru |
| Stop condition | Tidak copy/migrate shared DB, memperbaiki legacy di checkout lain, push/deploy atau mengasumsikan approvals hanya karena bootstrap PASS |

## Aturan update saat runner tersedia

1. Source of truth QA adalah immutable structured run evidence; Markdown hanya projection sanitised. Human approvals punya identity/scope sendiri.
2. Simpan run ID, source/test/contract/schema hashes, environment/namespace, D/T results, regression/shared gates, backup refs bila applicable dan residual blockers di brief paket.
3. Update dashboard dari run yang sama. Partial/crashed renderer → STALE sampai reconciliation; jangan menunjukkan dua ringkasan yang bertentangan sebagai current.
4. Code/tests/dependency berubah → reverify; latest regression fail mencabut current green confidence, tetapi old run history retained.
5. Commit, push, deployment, actual migration dan traffic owner diperbarui hanya dengan bukti operasional masing-masing.
6. Scope/denominator tidak diedit hanya karena test gagal. SKIP/NOT_RUN/BLOCKED/FLAKY bukan PASS; zero tests atau unavailable environment tidak menjadi paket verified.

## Riwayat checkpoint dokumentasi

| Tanggal | Event | Klaim yang diperbolehkan |
|---|---|---|
| 2026-09-05 | Brief/backend QA plan dibuat | Dokumentasi tersedia; bukan implementasi, backup, QA PASS atau release |
| 2026-09-05 | Validasi struktur dokumentasi | 31 Markdown; 20 modul; 8 gelombang; 102 definisi D/T termasuk 5 gerbang awal M19; tanpa broken local link, duplicate per-module ID, unbalanced fence, atau encoding replacement |
| 2026-09-06 WIB | Implementasi awal setelah trigger pengguna | Go runtime/source inventory + local Python/Playwright runner dan CI definition tersedia; package gates tetap incomplete |
| 2026-09-06 WIB | Source baseline | 8 commit staging ahead; code menambah JSONB `Batch.logbookSchedule`; actual DB schema belum verified |
| 2026-09-06 WIB | Local race attempt | `go test -race` gagal compile runtime/cgo: installed compiler tidak mendukung 64-bit; race proof belum tersedia |

Riwayat 5 September di atas merekam hasil pekerjaan brief terdahulu, bukan kondisi service saat ini. Validasi Markdown tidak menambah numerator D. Raw failure history awal dan run berikutnya retained; tidak ada DB deletion, export, import, backup, migration atau deployment dari implementasi awal ini. [Detail baseline/blocker](baseline/LOCAL_BASELINE.md).

<!-- QA:M03:START -->
## M03: latest automated checkpoint

- Run: `20260908T145630Z-M03-590db667a394`; automated checks: **PASS**.
- Package: **BLOCKED**; evidence: `CURRENT`.
- Verified deliverables: **0/5 (0%)**.
- Bootstrap checks: 23/23 PASS; other outcomes: `{}`.
- Full-package test pass rate: N/A while acceptance suite is incomplete.
- Release/migration/deployment: NOT_VERIFIED / NOT_EXECUTED / NOT_DEPLOYED.
- This is bootstrap evidence, not a completed module. See private run manifest for blockers.
<!-- QA:M03:END -->

<!-- QA:M00:START -->
## M00: latest automated checkpoint

- Run: `20260908T145441Z-M00-ee475c01a1e0`; automated checks: **PASS**.
- Package: **BLOCKED**; evidence: `CURRENT`.
- Verified deliverables: **0/5 (0%)**.
- Bootstrap checks: 20/20 PASS; other outcomes: `{}`.
- Full-package test pass rate: N/A while acceptance suite is incomplete.
- Release/migration/deployment: NOT_VERIFIED / NOT_EXECUTED / NOT_DEPLOYED.
- This is bootstrap evidence, not a completed module. See private run manifest for blockers.
<!-- QA:M00:END -->
