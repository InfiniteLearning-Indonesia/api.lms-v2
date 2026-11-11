# LMS v3 backend — Implementation status

> **Accepted 11 November:** [M14 local closure](checkpoints/2026-11-11-m14-local-closure.md), run `20261111T093737Z-postgres-lab-6e4bc487c4d6`, source `cb0a9af604011f90e6bc14844f637e5e0bb094104b509ff03905407feae0a7bc`. **M14 4/4 local CP slices**, 21/21 direct M14 cases (CP04 6/6), 1,080 measured requests, actual query/pool/audit, dependencies and 40 backup/restore receipt sets reviewed. Canonical M14 stays 0/5; project 0/97. No new CP, shared database access, Git or production readiness inferred. Older dated notes are history.

> **Current 2 November:** [M14-CP04 five-step package](checkpoints/2026-11-02-m14-cp04.md) **IMPLEMENTED_PENDING_FULL_QA**. Four of six frozen mandatory cases locally exercised; native actual load and PostgreSQL plan/pool cases pending. Fixed unattended backup/preflight/full-QA/review launcher is ready. Accepted historical counts remain M13 4/4, M14 3/4, canonical unchanged. Current executable source differs from the prior full PASS and must pass the new performance journey. Older status snapshots below are historical.

> **Accepted latest 31 October:** [M14 CP03](checkpoints/2026-10-31-m14-cp03-accepted.md) PASS_LOCAL_SYNTHETIC (7/7 mandatory) on run `20261031T115506Z-postgres-lab-53f7b900538f`; CP01/02/M13 dependencies, source and backup/restore hashes verified. M14 is now 3/4 local slices; CP04 not started, canonical counts unchanged. Older pending notes below are historical.

> **Latest 31 October:** [M14 CP01/02 accepted](checkpoints/2026-10-31-m14-cp01-cp02-accepted.md), M14 2/4 local slices. [CP03 pagination/export implemented](checkpoints/2026-10-31-m14-cp03.md) after scope freeze; Go/local instrumentation checks PASS on new source dc4618a4, full PostgreSQL/Playwright pending. M13 remains 4/4 prior local closure. Canonical counts unchanged; no shared DB or Git operations.

> **Latest M14 verification, 31 October:** [runtime instrumentation and shutdown budget repaired](checkpoints/2026-10-31-m14-qa-runtime-repair.md) after failed run `20261031T103230Z-postgres-lab-e14518526f85`. Six local checks PASS on source `f4819963e757a6afb285a11403459d21c55fa9d80d819803d2ee6eaca33a8303`; fresh full reporting journey pending. M14 stays 0/4 locally verified; CP03 gated. M13's accepted 4/4 local result remains historical evidence on its recorded tested source.

> **Accepted 31 October 2026:** [M13 local closure](checkpoints/2026-10-31-m13-local-closure.md) reviewed on current source; **4/4 CP local synthetic**, direct cases 19/19 PASS, mandatory worker PASS, 39 backup/restore receipts reviewed, all reconciliation PASS. Run `20261031T100520Z-postgres-lab-246c640f9cf1`. M13 canonical stays 0/5; M00–M18 0/97; M14 remains 0/4 local verified. Older failure/unverified snapshots below are history.

> **31 October 2026 — current:** [M13-CP04 runner repair](checkpoints/2026-10-31-m13-cp04-runner-repair.md) passed 5 local checks, including 3 Go cases + 1 new runner regression. Full CP remains unverified until fresh PostgreSQL/Playwright QA on the repaired source. M13 stays 3/4 local slices, M14 0/4; no new schema/shared DB/Git operations. Earlier dated status snapshots below are historical.

> **Update 30 Oktober 2026 — newest work:** M13-CP04 and M14-CP01/02 are [implemented but unverified](checkpoints/2026-10-30-m13-cp04.md). Go unit suite/vet pass. Full synthetic runner stopped before PostgreSQL startup (`initdb` restricted token error 87); M13 stays 3/4 locally verified, M14 stays 0/4, canonical D totals unchanged. Older snapshots below are historical.

## Pembaruan 30 Oktober 2026 — M13-CP03 lulus lokal sintetis

[M13-CP03](checkpoints/2026-10-29-m13-cp03.md) **PASS_LOCAL_SYNTHETIC** pada run `20261030T045432Z-postgres-lab-38ca171bbd9e`, source `d0d9a14313551c0831b1e67ee1437743955c780a1e728f9d06dc03eda2bbf113`: 4/4 Playwright, 3/3 Go, backup SQL/custom plus isolated restore, final DB reconciliation dan dependency regression PASS. M13 kini **3/4 CP lokal teruji**; CP04 belum dikerjakan. Canonical M13 tetap **0/5 D fully verified**, M00–M18 tetap **0/97 D fully verified** karena external/shared gates masih terbuka. Tidak ada akses DB staging/live, delete, commit, push atau deploy. Pembaruan bertanggal sama di bawah merekam run gagal yang sudah digantikan oleh bukti terbaru ini.

## Pembaruan 30 Oktober 2026 — PostgreSQL lokal bekerja, CP03 menunggu ulang QA

Run terminal biasa `20261030T042312Z-postgres-lab-4d9f493c2d96` berhasil memulai cluster sintetis baru, melakukan backup/restore SQL/custom untuk M13-CP03 dan mencapai tes Playwright AI. Tiga dari empat kasus lulus; `M13-CP03-failure-no-grade` gagal karena payload tes memiliki field ekstra yang ditolak endpoint sebelum guard AI diuji. Payload telah diperbaiki dan runner kini mencetak progres berkala. Run tersebut `FAIL` dan source kini berubah; Go/DB mandatory serta final reconciliation CP03 belum dieksekusi. **M13 tetap 2/4 CP lokal teruji, canonical 0/5 D; proyek M00–M18 tetap 0/97 D fully verified.** Jalankan ulang seluruh journey pada cluster sintetis baru sebelum menutup CP03. [Bukti lengkap](checkpoints/2026-10-29-m13-cp03.md). Bagian bertanggal lebih lama di bawah adalah histori.

## Pembaruan 29 Oktober 2026 — M13-CP03 belum ditutup

[M13-CP03](checkpoints/2026-10-29-m13-cp03.md) telah diimplementasikan sebagai suggestion sintetis non-authoritative dengan gradebook tetap human-controlled. `go test ./...`, `go vet ./...`, kompilasi tagged integration dan Playwright collection lulus, tetapi **Playwright/Go-DB/backup/restore/reconciliation tidak dijalankan** karena `initdb`/`pg_ctl` ditolak Windows restricted-token error 87 sebelum migrasi. M13 tetap **2/4 CP lokal teruji**, canonical 0/5 D; M00–M18 tetap **0/97 D fully verified**. Staging/live tidak disentuh; tidak ada delete, commit, push atau deploy. Status bertanggal lebih lama di bawah adalah histori.

## Pembaruan 30 September 2026 — M07-CP04

[M07-CP04](checkpoints/2026-09-30-m07-cp04.md) lulus **4/4 Playwright dan 3/3 Go/DB** pada PostgreSQL lokal sintetis, disertai backup SQL/custom, isolated restore, rekonsiliasi dan dependency regression. M07 kini **4/4 CP slice lokal**, tetapi canonical **0/5 D** dan total M00–M18 **0/97 D fully verified**. Historical owner exact revision dan structure-only clone sudah teruji; real importantLinks import, attachment, frontend/shared/QG dan source mapping belum. Run `20260930T104933Z-postgres-lab-a4ed4b030dc1`, source `52521b0ca62f5846d5af46766bf0e554470fdd103b718c05e33ea17cc75888b6`, `PASS_SYNTHETIC_ONLY`. Staging/live tidak diakses; tidak ada delete, commit, push atau deploy. Status lebih lama di bawah adalah histori.

## Pembaruan 29 September 2026 — M10-CP04 dan M05-CP05

[M10-CP04](checkpoints/2026-09-29-m10-cp04.md) menutup **4/4 CP slice lokal sintetis** M10 (3 Go/DB + 3 Playwright PASS pada run CP04). [M05-CP05](checkpoints/2026-09-29-m05-cp05.md) menutup **5/5 CP slice lokal sintetis** M05 (2 Go/DB + 3 Playwright PASS), dengan Class→content→submission→grade, end/archive dan retake provenance, backup/restore dan reconciliation. Run final M05 `20260929T133707Z-postgres-lab-46f4fba76430`, source `6c0dc6f6a18b62869b056ef7e8c930a29dfed47c699ebe0b4281f549783cb5c9`, `PASS_SYNTHETIC_ONLY`. Canonical M00–M18 tetap **0/97 D fully verified**. Policy Grade historis pasca-archive, academic approval, real source/identity, shared runtime, remote CI, frontend contract dan QG masih terbuka. Tabel/status lama di bawah adalah histori.

## Rekonsiliasi 24 September 2026 — cara membaca status

[Review lintas dokumen](RECONCILIATION_2026-09-24.md) menetapkan M13-CP01/02 **2/4 CP lokal sintetis** setelah memeriksa [manifest dan run QA](checkpoints/2026-09-24-m13-cp01-cp02-evidence-review.md); dashboard lama yang menyebut M13 `NOT_STARTED` tidak berlaku. Run ini adalah regresi pada source CP03 M10, bukan QA baru 24 September. M09 tetap 4/4 CP lokal, M10 3/4, M15 5/5 slice sintetis; seluruh M00–M18 tetap **0/97 D fully verified**. [Baseline database lokal](LOCAL_SYNTHETIC_BASELINE.md) berlaku sampai source live sah tersedia.

## Pembaruan 22 September 2026 — M10-CP03

[Transcript dan credential lifecycle](checkpoints/2026-09-22-m10-cp03.md) lulus **6/6 frozen cases** (3 Go/DB + 3 Playwright), backup SQL/custom + actual isolated restore, reconciliation dan full dependency regression pada PostgreSQL sintetis lokal. Release transcript eksplisit menyimpan provenance grade/policy; credential issue idempotent, revoke/supersede append-only, dan public verification hanya membuka status minimal sesuai visibility. M10 kini **3/4 CP lokal**, canonical **0/5 D**. Run final `20260922T125746Z-postgres-lab-2e3def603dd8`, source `25fb237a09bc2cf81e9e988dfd74646624f6a7485a6ee1da0600aa121db1833c`, `PASS_SYNTHETIC_ONLY`. Staging/production tidak diakses; tidak ada delete, commit, push atau deployment. Berikutnya M10-CP04; academic policy, real mapping, shared runtime dan QG tetap pending.

## Pembaruan 22 September 2026 — M10-CP02

[Progress, override dan reopen](checkpoints/2026-09-22-m10-cp02.md) lulus **6/6 frozen cases** (3 Go/DB + 3 Playwright) dan full dependency regression pada PostgreSQL sintetis lokal, dengan backup SQL/custom serta actual isolated restore sebelum installment. Progress adalah query-time projection dari history; correction append-only, reasoned, versioned, idempotent, scoped, dan audit/outbox atomic. M10 kini **2/4 CP lokal**, canonical **0/5 D**. Run final `20260922T092150Z-postgres-lab-851380abc177`, source `19f1494d5a3448221299747d03110993723388c1e71be051c047f4484b543599`, `PASS_SYNTHETIC_ONLY`. Staging/production tidak diakses; tidak ada delete, commit, push atau deployment. Berikutnya M10-CP03; academic policy dan shared/real gates tetap pending.

## Pembaruan 22 September 2026 — M10-CP01

[Completion-policy foundation](checkpoints/2026-09-22-m10-cp01.md) lulus **6/6 frozen cases** (3 Go/DB + 3 Playwright) setelah backup SQL/custom dan actual isolated restore pada PostgreSQL sintetis lokal. Rule snapshot, explicit learner evidence dan scoped outcome terpisah dari grade; M10 kini **1/4 CP lokal**, canonical **0/5 D**. Run final `20260922T075030Z-postgres-lab-661c9a0a95b9`, source `05fbe4de581eaad03474e2e9b6a9fedde9eecdaf63640d7a905ca0bc6c327a93`, `PASS_SYNTHETIC_ONLY`. Tidak ada staging/production access, delete, commit, push atau deployment. Berikutnya M10-CP02; academic policy dan shared/real gates tetap pending.

## Pembaruan 15 September 2026 — M12-CP02 dan M12-CP03

[M12-CP02/CP03](checkpoints/2026-09-15-m12-cp02-cp03.md) lulus **8/8 frozen cases** pada PostgreSQL sintetis lokal, dengan backup SQL/custom + actual isolated restore terpisah sebelum masing-masing installment. Entry/revision/file history, exact submit, scoped mentor review, reassignment provenance, CAS concurrency, audit/outbox/idempotency dan DB privilege boundaries teruji. M12 kini **3/4 slice lokal**, canonical **0/5 D**; total M00–M18 tetap **0/97 D**. Run final `20260915T113408Z-postgres-lab-c710a6f77c5f`, source `a008d3be4df930256dbb273576fdac775fbdad199054adc40dada63885b9d4e0`, `PASS_SYNTHETIC_ONLY`. Tidak ada staging/production access, delete, commit, push atau deployment. M12-CP04 menunggu reviewed legacy mapping/authority; lane sintetis berikut yang mungkin adalah M13-CP01.

## Pembaruan 14 September 2026 — M12-CP01

[M12-CP01](checkpoints/2026-09-14-m12-cp01.md) lulus **4/4 frozen cases** pada PostgreSQL sintetis lokal dengan backup SQL/custom + actual isolated restore. M12 kini **1/4 slice lokal**, canonical M12 **0/5 D**; total M00–M18 tetap **0/97 D**. Source `25174065464f9b8b1c4496c24fe8fe4268d33e7cdbe01f36bbfb050c0b1d7d84`, run final `20260914T143931Z-postgres-lab-39c96c14ef9a` PASS_SYNTHETIC_ONLY. Tidak ada DB staging/production, delete, commit, push atau deployment. Posisi berikutnya M12-CP02; M11-CP02 menunggu attendance/permit policy.

## Pembaruan 14 September 2026 — M15-CP05 dan M11-CP01

Scope dibekukan pada [manifest M15-CP05/M11-CP01](../qa/manifests/M15-CP05-M11-CP01.json). M15-CP05 menutup **5/5 slice sintetis lokal** dengan rekonsiliasi operator-only; M11-CP01 membuka **1/4 slice lokal** melalui meeting/exception Class. Ini bukan kelulusan canonical: M15 masih **0/6 D**, M11 **0/5 D**, dan total M00–M18 tetap **0/97 D**. Database nyata, cutover, dan frontend tidak disentuh. Bukti run final dan batasnya di [laporan checkpoint](checkpoints/2026-09-14-m15-cp05-m11-cp01.md). Bagian bertanggal 13 September di bawah adalah histori, bukan posisi paling baru.

## Posisi pekerjaan per M — 13 September 2026

**M adalah unit capaian utama; CP adalah bagian di dalam M.** Lihat [peta lengkap M00–M19 → CP](MODULE_CHECKPOINT_MAP.md) dan bagian “Peta pekerjaan” di setiap module brief. Canonical D verification dan raw/generated QA sections di bawah tetap mempunyai denominator acceptance sendiri.

| M yang sudah disentuh | CP rencana | Bagian teruji lokal | Posisi/sisa paling dekat |
|---|---:|---:|---|
| [M00](modules/M00.md) | 4 | 1/4 | Source inventory/evidence lokal ada; production/deployed identity, full inventory/capacity dan handoff belum lengkap |
| [M01](modules/M01.md) | 4 | 1/4 | Synthetic backup/restore teruji; real host/key/object protection, backup nyata dan closure tersisa |
| [M02](modules/M02.md) | 4 | 0/4; CP01 parsial | Security disposition bukan mitigation; perlu scoped legacy fixes dan deployment verification |
| [M03](modules/M03.md) | 6 | 6/6 | Seluruh slice lokal teruji; canonical 0/5 karena shared/container/remote gates |
| [M04](modules/M04.md) | 5 | 2/5 | Bridge/session dan Class fence lokal teruji; real owner/provisioning, refresh/browser dan full ancestry tersisa |
| [M05](modules/M05.md) | 5 | 4/5 | Grouping/mentor/invited/retake/provenance teruji lokal; CP05 content/learner-state closure tersisa |
| [M06](modules/M06.md) | 4 | 3/4 | Private asset/delivery dan bounded egress teruji lokal; CP04 mapping/closure tersisa |
| [M07](modules/M07.md) | 4 | 4/4 sintetis | Structure, safe content, exact assignment reference, historical owner read dan structure-only clone teruji lokal; real import/attachment dan shared gates tersisa |
| [M08](modules/M08.md) | 4 | 3/4; CP04 parsial | Receipt/deadline, exact revision, retained resubmit dan bounded raw landing teruji lokal; CP04 real chronology/closure menunggu M15 |
| [M11](modules/M11.md) | 4 | 1/4 sintetis lokal | CP01 meeting/exception Class lulus; policy, attendance/permit/SP/outcome dan shared runtime masih pending |
| [M12](modules/M12.md) | 4 | 3/4 sintetis lokal | CP01 periods + CP02 entry/revision + CP03 scoped review/reassignment lulus; CP04 legacy mapping/closure dan shared gates pending |
| [M15](modules/M15.md) | 5 | 5/5 sintetis lokal | Reconciliation fixture juga lulus; real-source adapter/CA/backup/academic approval dan file bytes masih tertahan |

M09 4/4, M10 3/4, M13 2/4 (review 24 September), M14 0/4, M11 1/4, M12 3/4, M16–M17 0/4, M18 0/3; M19 belum frozen (N/A). Ini jumlah potongan scope teruji, **bukan persentase effort atau progress full module**. Tidak menjumlahkan numerator antar M karena shared evidence dan ukuran CP berbeda. Full M00–M18 masih 0/97 D; slice lokal tidak menutup klausa canonical yang masih mempunyai provider/shared/QG gate.

**Latest M15-CP03/CP04, 13 September:** [dua slice sintetis lokal lulus](checkpoints/2026-09-13-m15-cp03-cp04.md). Run final `20260913T141341Z-postgres-lab-81b971a0ad85` PASS_SYNTHETIC_ONLY pada source `3252177f…`: backup+actual isolated restore per CP, Go/DB 2/2 dan Playwright 1/1 per CP, deterministic Class mapping/quarantine, append-only decisions, atomic unpublished shadow import, rollback/replay/concurrent-owner fence dan full backend regression PASS. M15 kini 4/5 slice lokal, canonical tetap 0/6; CP05, real source adapter/CA/inventory, shared backup, academic approval dan reconciliation belum ada. Tidak ada staging/live access, database deletion, commit atau push.

**Prior M15-CP01/CP02, 13 September:** [dua slice sintetis lokal lulus](checkpoints/2026-09-13-m15-cp01-cp02.md). Run `20260913T111320Z-postgres-lab-0530ae7319de` PASS_SYNTHETIC_ONLY pada source `407ec8d5…`: backup+actual isolated restore per CP, Go/DB 1/1 dan Playwright 1/1 per CP, 20 baseline table+tambahan+file fixture, negative preflight, private append-only provenance/replay dan full backend regression PASS. Status saat itu M15 2/5 slice lokal, canonical 0/6.

**Latest M08-CP03/CP04, 13 September:** [CP03 local verified; CP04 partial/block M15](checkpoints/2026-09-13-m08-cp03-cp04.md). Run `20260913T043936Z-postgres-lab-d13adcef71bf` PASS_SYNTHETIC_ONLY pada source `c0e483db…`: dua backup+actual isolated restore, Go/DB 2/2, Playwright 5/5, receipt server-clock/idempotency, cutoff rollback, raw chronology retained, dua hand-in/resubmit dan persistence reconciliation PASS. M08 3/4 exit lokal; CP04 real source mapping/reconciliation, historical-owner content read, shared/remote/QG tetap pending. Tidak ada staging/live access, database deletion, commit, atau push.

**M08-CP01/CP02 sebelumnya, 13 September:** [VERIFIED_LOCAL_SLICES](checkpoints/2026-09-13-m08-cp01-cp02.md). Run `20260913T034230Z-postgres-lab-e049f9724d67` PASS_SYNTHETIC_ONLY pada source `a6e090d6…`: backup+restore, Go/DB 2/2, Playwright 5/5, exact revision/episode/Class FK serta retained history PASS. Evidence historis dipertahankan.

**Latest M06-CP03 + M07-CP01, 12 September:** [VERIFIED_LOCAL_SLICES](checkpoints/2026-09-12-m06-cp03-m07-cp01.md). Run `20260912T073128Z-postgres-lab-12615822542d` PASS_SYNTHETIC_ONLY pada source `aeac0f7f…`: verified dump+isolated restore sebelum M07 migration, CP03/CP01 Go-DB 1/1 masing-masing, Playwright 3/3 + 4/4, dan seluruh dependency/persistence regression PASS. Focused statement coverage 34.9%/48.9%; instrumented API 29.1%/30.0%. Tidak ada shared/staging/live access, database deletion, commit, atau push.

**Latest M07-CP02, 12 September:** [VERIFIED_LOCAL_SLICE](checkpoints/2026-09-12-m07-cp02.md). Run `20260912T085032Z-postgres-lab-f65eb8197ca5` PASS_SYNTHETIC_ONLY pada source `9f7dd821…`: backup+actual isolated restore, Go/DB 1/1, Playwright 4/4, restricted raw source, canonical escaping, lifecycle/availability dan seluruh dependency/persistence regression PASS. Focused statements 45.6%; API instrumentation 30.7%. Tidak ada shared/staging/live access, database deletion, commit, atau push.

**M07-CP03 producer, 12 September:** [VERIFIED_LOCAL_PRODUCER_SLICE](checkpoints/2026-09-12-m07-cp03.md). Final producer run `20260912T100518Z-postgres-lab-1bf94dc9948d` PASS_SYNTHETIC_ONLY pada source `b2641049…`: backup+actual isolated restore, Go/DB 1/1, Playwright 4/4, published immutability, exact contract dan dependency regression PASS. Focused statements 40.7%; API instrumentation 30.9%. M08 consumer kini mem-pin exact revision ID; historical-owner content read tetap pending. Tidak ada shared/staging/live access, database deletion, commit, atau push.

**Latest M06-CP01–CP02, 11 September:** [VERIFIED_LOCAL_SLICES](checkpoints/2026-09-11-m06-cp01-cp02.md). Run `20260911T065629Z-postgres-lab-abb87a171a37` PASS_SYNTHETIC_ONLY: backup+actual isolated restore sebelum tiap migrasi, Go/DB 2/2, Playwright API 7/7, DB-enforced locator/checksum separation+uniqueness, quarantine/expiry/context authorization dan persistence reconciliation PASS. Linux race 33 cases serta non-root runtime PASS. M06 kini 2/4 slice lokal; canonical tetap 0/5 karena CP03/CP04, actual scanner/object provider, shared environment, remote CI/container dan QG gates belum lengkap.

**Latest M03-CP05–CP06, 10 September:** [LOCAL_SLICES_COMPLETE](checkpoints/2026-09-10-m03-cp05-cp06.md). Worker lease/fence/retry/dedupe/crash/shutdown/terminal-failure, role terpisah, registry fail-closed, pool pressure dan concurrent replay PASS pada fresh restored-backup PostgreSQL. Run `20260910T081438Z-postgres-lab-6bdee5ba3b3c`; CP05 coverage 75.0%. Bootstrap 24/24, 30 Go cases, Linux race PASS; API+worker static deterministic artifact PASS_BUILD_ONLY. M03 kini 6/6 slice lokal tetapi full canonical tetap 0/5; container, remote CI/tagged-DB-race, shared provider/runtime/operator dan M00/M01/QG gates belum lulus.

**Latest M05-CP03–CP04, 10 September:** [VERIFIED_LOCAL_SLICES](checkpoints/2026-09-10-m05-cp03-cp04.md). Run `20260910T093244Z-postgres-lab-21274ebc3179` PASS_SYNTHETIC_ONLY: CP03/CP04 Go-DB 1/1 masing-masing, Playwright API 6/6, backup+actual isolated restore dua tahap, concurrency dan persistence reconciliation PASS. M05 kini 4/5 slice lokal; canonical tetap 0/5 karena M05-CP05, actual identity/shared environment, remote CI dan QG gates belum lengkap.

**Historical M03-CP03–CP04, 9 September:** [VERIFIED_LOCAL_SLICES](checkpoints/2026-09-09-m03-cp03-cp04.md). Fresh PostgreSQL backup+restore+migration run `20260909T142128Z-postgres-lab-7fa51b57054b` PASS; CP03 runner coverage 76.4%; CP04 real `cmd/api` process 12/12 API cases dan 69.4% instrumented coverage; CP08–CP12 regressions PASS. Linux 27 race cases + non-root UID 65532 PASS. Source fingerprint `a41ae4205c342b9f88791abe87942b3a4e0440f456852fd32eb09d56380e0631`. Pada checkpoint itu M03 mencapai 4/6 bagian, tetapi full canonical D tetap 0/5; shared migration/deploy/container/remote CI tidak diklaim. Commit `52cbc88` dibuat/published pengguna saat sesi berjalan; perubahan sesudah commit tetap harus diperiksa dari worktree/evidence terbaru.

**Latest CP11–CP12, 8 September:** [VERIFIED_LOCAL_SLICES](checkpoints/2026-09-08-cp11-cp12.md), **4/4 deliverable lokal masing-masing**. CP11 API 6/6; CP12 + CP11 regression 9/9; CP10 API 6/6; M00 20/20; M03 23/23; 23 default Go cases + Linux race/non-root PASS. Measured coverage union: identity 86.01%, Class 83.85%, access 100%, seluruh profile 70.09%. Full M04/M05 gates tetap belum terverifikasi. Root Go dan CP08–CP10 published pada `edac5af`; user kemudian mengotorisasi publikasi CP11–CP12 ke `be-v3` setelah QA, dengan raw artifact tetap lokal. Ringkasan numerik CP08–CP10 di bawah adalah histori, bukan source terbaru.

[Indeks](README.md) · [QA contract](QA_AUTOMATION_PLAN.md) · [Master plan](references/ImplementationPlan.MD)

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
| [M03](modules/M03.md) | LOCAL_SLICES_COMPLETE | PARTIAL_QA; full gate BLOCKED | 0/5 (0%) | CP01–CP06 local tested; shared operator/provider/runtime, full QG, container/remote CI pending. |
| [M04](modules/M04.md) | IN_PROGRESS (CP11–CP12 slice verified) | LOCAL_IDENTITY_API_DB_PASS | 0/5 full (0%) | Verified synthetic bridge/session/CSRF/projection/local admin/fence; real legacy provisioning/reset/global owner, browser and full QG pending. CP11 slice 4/4. |
| [M05](modules/M05.md) | IN_PROGRESS (CP12 slice verified) | LOCAL_CAPABILITY_API_DB_PASS | 0/5 full (0%) | Teacher manages Students; close/reopen/suspend/reactivate/end/rejoin/history/last-teacher/concurrency verified locally. Grouping/retake/content readiness/operational QG pending. CP12 slice 4/4. |
| [M06](modules/M06.md) | IN_PROGRESS (CP01–CP03 local slices) | LOCAL_FILE_EGRESS_API_DB_PASS | 0/5 full (0%) | Bounded egress teruji sintetis; CP04 mapping, actual providers/DNS/shared/QG pending. |
| [M07](modules/M07.md) | IN_PROGRESS (CP01–CP04 local synthetic slices) | LOCAL_CONTENT_API_DB_PASS | 0/5 full (0%) | CP04 historical-owner read dan structure-only clone teruji; real import/attachment, frontend/shared/QG gates pending. |
| [M08](modules/M08.md) | IN_PROGRESS (CP01–CP03 local slices; CP04 partial) | LOCAL_SUBMISSION_RECEIPT_API_DB_PASS | 0/5 full (0%) | Receipt/deadline dan retained history teruji; CP04 real chronology/M15, M09 consumer, shared/remote/QG pending. |
| [M09](modules/M09.md) | LOCAL_SYNTHETIC_COMPLETE (CP01–CP04) | PASS_SYNTHETIC_ONLY (`20260921T062257Z-postgres-lab-cf154cd61d25`) | 0/5 full canonical; D01–D05 didukung local evidence | Definitions, scoped grade, safe import/release, dan historical reconciliation PASS lokal; academic approval, real inventory, shared/remote/QG pending. |
| [M10](modules/M10.md) | IN_PROGRESS (CP01–CP03 synthetic PASS) | PASS_SYNTHETIC_ONLY (`20260922T125746Z-postgres-lab-2e3def603dd8`) | 0/5 full canonical; D01–D04 didukung local evidence parsial | Rule/evidence/progress, transcript snapshot dan credential lifecycle teruji lokal; CP04, academic approval, shared/real/remote/QG gates pending. |
| [M11](modules/M11.md) | IN_PROGRESS (CP01 synthetic slice) | LOCAL_MEETING_GO_DB_PLAYWRIGHT_PASS | 0/5 full (0%) | Attendance/permit policy, CP02–04, shared runtime dan canonical gates pending. |
| [M12](modules/M12.md) | IN_PROGRESS (CP01–CP03 synthetic slices) | LOCAL_LOGBOOK_GO_DB_PLAYWRIGHT_PASS | 0/5 full (0%) | Period/deadline product policy, CP04 legacy mapping/closure, shared runtime/provider dan canonical gates pending. |
| [M13](modules/M13.md) | LOCAL_SYNTHETIC_CLOSURE (4/4 CP) | PASS_SYNTHETIC_ONLY (`20261031T100520Z-postgres-lab-246c640f9cf1`) | 0/5 full canonical; all D supported by bounded local evidence | Actual provider/secret manager, privacy/recipient policy, shared runtime and remote/QG pending. |
| [M14](modules/M14.md) | CP01–04 PASS local (4/4) | Full native journey reviewed; 21/21 direct cases, 1,080 requests, DB/pool/audit/dependencies PASS | 0/5 full canonical | Local lane closed; shared-runtime/key/identity, real-source parity, remote/QG and frontend/production evidence remain open. |
| [M15](modules/M15.md) | LOCAL_SYNTHETIC_SLICES_COMPLETE (CP01–05) | LOCAL_RECONCILIATION_GO_DB_PLAYWRIGHT_PASS | 0/6 full (0%) | Source nyata/CA/backup/academic approval, file bytes dan canonical mapping/backfill belum verified. |
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
| [B03](batches/B03.md) | IN_PROGRESS (M06 CP01–03; M07 CP01–03; M08 CP01–03 local slices, CP04 partial) | PARTIAL_API_DB | 0/15 | NOT_VERIFIED |
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

- Run: `20260910T081720Z-M03-d76b404a059e`; automated checks: **PASS**.
- Package: **BLOCKED**; evidence: `CURRENT`.
- Verified deliverables: **0/5 (0%)**.
- Bootstrap checks: 24/24 PASS; other outcomes: `{}`.
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
