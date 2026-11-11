# LMS v3 — Brief eksekusi backend Go

**Latest accepted 31 October:** [M14 CP03 verified](checkpoints/2026-10-31-m14-cp03-accepted.md), M14 now 3/4 local slices. All seven CP03 cases, original reporting flow and dependency/backup evidence pass on source dc4618a4. Next proposed checkpoint is CP04 measured performance/closure; no new implementation has started. Earlier pending instructions below are historical.

**Current M14 step, 31 October:** [CP01/02 accepted](checkpoints/2026-10-31-m14-cp01-cp02-accepted.md); [CP03 bounded pagination/private CSV implemented](checkpoints/2026-10-31-m14-cp03.md), full QA pending. M14 is 2/4 accepted local slices. Next trigger is `--reporting-pagination-journey`; CP04 not started. Older instructions below are historical where superseded.

**Accepted 31 October 2026:** [M13 4/4 local synthetic closure](checkpoints/2026-10-31-m13-local-closure.md), run `20261031T100520Z-postgres-lab-246c640f9cf1`, exact current source and manifest evidence accepted. M13 canonical gates remain open; M14 CP01/02 remain implemented/unverified. Read active status/module map and this closure before older historical instructions below.

**Current, 31 October 2026:** use [active status](IMPLEMENTATION_STATUS.md) and [M13-CP04 repair evidence](checkpoints/2026-10-31-m13-cp04-runner-repair.md). M13 CP01–03 have prior local evidence; CP04 runner ordering is repaired and direct local checks pass, but full repaired-source PostgreSQL/Playwright QA is pending. M14 CP01–02 are implemented/unverified. Later paragraphs with earlier dates preserve historical state and must not determine today's next action.

**Status 29 Oktober 2026:** [M13-CP03](checkpoints/2026-10-29-m13-cp03.md) **implemented/unverified**, bukan PASS. Fresh-cluster QA terhenti sebelum backup/migrasi oleh `initdb`/`pg_ctl` restricted-token error 87. M13 tetap **2/4 CP lokal teruji**, M07 tetap 4/4 sintetis, canonical M00–M18 tetap **0/97 D fully verified**. Berikutnya ulang QA CP03 pada host yang mendukung fresh PostgreSQL; jangan lanjut CP04. Status lama di bawah adalah histori.

**Status 30 September 2026:** [M07-CP04](checkpoints/2026-09-30-m07-cp04.md) lulus **7/7 frozen cases** pada fixture lokal, sehingga M07 **4/4 CP slice sintetis** namun **0/5 canonical D**. M00–M18 tetap **0/97 D fully verified**. Run `20260930T104933Z-postgres-lab-a4ed4b030dc1` disertai backup/restore dan rekonsiliasi. Kandidat berikut M06-CP04 setelah scope/mapping freeze atau M13-CP03 setelah AI/policy freeze; M00/M01/M02 sebelum data nyata. Pernyataan bertanggal lebih lama adalah histori.

**Status 29 September 2026:** [M10-CP04](checkpoints/2026-09-29-m10-cp04.md) **4/4** dan [M05-CP05](checkpoints/2026-09-29-m05-cp05.md) **5/5 CP slice lokal sintetis**. Run final CP05 `20260929T133707Z-postgres-lab-46f4fba76430` PASS_SYNTHETIC_ONLY; canonical M00–M18 tetap **0/97 D**. Berikutnya lane sintetis M07-CP04 memerlukan scope freeze; M00/M01/M02 tetap gate sebelum data nyata. Pembaruan lama di bawah adalah histori.

**Rekonsiliasi 24 September:** mulai dari [status terkini dan koreksi](RECONCILIATION_2026-09-24.md), [peta M/CP](MODULE_CHECKPOINT_MAP.md), [baseline database lokal](LOCAL_SYNTHETIC_BASELINE.md), dan [handoff frontend](FRONTEND_HANDOFF_STATUS.md). M09 4/4, M10 3/4, M13 2/4 CP lokal sintetis; M15 5/5 slice sintetis. Canonical M00–M18 tetap 0/97 D fully verified. M10-CP04 adalah lanjutan dependency-linear; pembaruan lama di bawah adalah histori.

**Update 15 September:** [M12-CP02/CP03](checkpoints/2026-09-15-m12-cp02-cp03.md) lulus pada fixture PostgreSQL lokal (4 Go/DB + 4 Playwright, backup+restore per installment). M12 3/4 slice lokal, canonical 0/5. M12-CP04 tertahan legacy mapping/authority; M13-CP01 menjadi opsi lane sintetis berikut. Tidak ada operasi DB nyata.

**Update 14 September:** [M12-CP01](checkpoints/2026-09-14-m12-cp01.md) lulus pada fixture PostgreSQL lokal (2 Go/DB + 2 Playwright, backup+restore). M12 1/4 slice lokal, canonical 0/5; M15 5/5 slice sintetis dan M11 1/4 slice lokal dari checkpoint sebelumnya. Urutan berikutnya M12-CP02; M11-CP02 menunggu policy. Tidak ada operasi DB nyata.

**Pembaruan 14 September:** [M15-CP05/M11-CP01](checkpoints/2026-09-14-m15-cp05-m11-cp01.md) lulus pada fixture lokal dengan backup+restore per migrasi dan QA Playwright/Go/DB. M15 5/5 slice sintetis, M11 1/4 slice lokal; canonical tetap 0/6 dan 0/5. Angka/status 13 September di bawah adalah histori.

Service root: `D:/Projects/lms-v2/backend-v3/backend-go`. Branch: `be-v3`. Versi brief: 1.8, status direkonsiliasi 24 September 2026.

**Repository boundary corrected 8 September:** this service directory is the source worktree root; `.github/` lives inside it. [Layout/publication handoff](checkpoints/2026-09-08-git-root.md) preserves the historical publication record. Primary plan/audit/benchmark copies now live in [project-local references](references/README.md); the old workspace-root originals remain historical copies. This documentation update performs no Git operation or publication.

**Mulai dari capaian M:** [MODULE_CHECKPOINT_MAP.md](MODULE_CHECKPOINT_MAP.md) menunjukkan jumlah CP per M, bagian teruji, dependency DAG dan next action. Setiap [Mxx.md](modules/M03.md) mempunyai CP miliknya sendiri. Posisi lokal: M00 1/4, M01 1/4, M03 6/6, M04 2/5, M05 4/5 bagian teruji. Full canonical D/gates belum tertutup; angka bagian bukan effort/completion percentage.

**Status sekarang: partial backend implementation.** M06 3/4, M07 3/4, M08 3/4 dengan CP04 parsial, M09 4/4, M10 3/4, M13 2/4, dan M15 5/5 hanya teruji lokal sintetis. Canonical seluruh modul tetap terbuka karena real source/CA/backup, academic mapping/policy, provider, consumer/shared/QG dan deployment belum lengkap. Run terbaru [M10-CP03](checkpoints/2026-09-22-m10-cp03.md) memakai cluster loopback baru; referensi production tetap belum teridentifikasi dari kedua konfigurasi staging.

## Mulai dari sini

1. [ImplementationPlan.MD](references/ImplementationPlan.MD): keputusan domain, keamanan dan migrasi utama.
2. [MODULE_CHECKPOINT_MAP.md](MODULE_CHECKPOINT_MAP.md): peta semua M beserta CP, progress bagian, alias historis dan urutan dependency.
3. [IMPLEMENTATION_STATUS.md](IMPLEMENTATION_STATUS.md): status full module, current evidence dan blockers; tidak menggantikan progress bagian.
4. [QA_AUTOMATION_PLAN.md](QA_AUTOMATION_PLAN.md): trigger runner, shared gates, evidence dan perhitungan coverage.
5. Pilih M dan CP-nya, atau bundle beberapa M/CP dalam gelombang Bxx. Satu instruksi dapat mencakup beberapa bagian dengan QA setelah setiap CP.

Referensi faktual: [audit codebase](references/LMS_CODEBASE_AUDIT_AND_REMEDIATION.md), [benchmark Moodle](references/LMS_Modern_Moodle_Benchmark_Architecture_Business_Flow.md). Brief ini menurunkan master plan menjadi acceptance contract; tidak menyalin schema Moodle atau mengubah target ke TypeScript.

## Batas keputusan

- Backend aplikasi **Golang**, QA automation **Python/Pytest/Playwright**; backend API-first, frontend belakangan.
- Class mandiri; Program/cohort metadata opsional; banyak Teachers dan Students per Class. Enrollment berbeda dari role/capability; grade berbeda dari completion; content berbeda dari learner state.
- V3 memakai staging DB sebagai target operasional nanti; live tetap live selama transisi. Existing staging data tidak ditimpa.
- **Never delete data**: termasuk fixtures, session-store cleanup, outbox/audit purge, ORM cascade dan object storage. Backup terverifikasi sebelum tiap migration run; raw artifacts tidak masuk Git.
- Scope implementasi backend di service root ini, plus satu workflow integrasi di root Git. Tidak mengubah frontend atau legacy. M02 tetap mencatat pekerjaan backend legacy terpisah yang harus diotorisasi pada repository tepat; menulis Go tidak memperbaiki legacy secara otomatis.
- Kebutuhan frontend security/integration tetap external gates. Tidak membuat brief implementasi frontend sekarang dan tidak menyatakan full release aman dari API test saja.
- Latest user keputusan service root ini berlaku pada seluruh path Go di master plan. Master plan tetap canonical keputusan produk; brief canonical granular D/T scope setelah direview. Jika konflik substantive ditemukan, catat ADR/approval dan sinkronkan secara eksplisit, bukan diam-diam mengganti aturan.

## Gelombang pengerjaan

| Brief | Paket | Hasil |
|---|---|---|
| [B01](batches/B01.md) | M00, M01, M02 | Baseline, backup dan containment |
| [B02](batches/B02.md) | M03, M04, M05 | Fondasi Go, identity dan Class |
| [B03](batches/B03.md) | M06, M07, M08 | Files, content dan submissions |
| [B04](batches/B04.md) | M09, M10 | Gradebook dan completion |
| [B05](batches/B05.md) | M11, M12, M13, M14 | Fitur LMS dan API readiness |
| [B06](batches/B06.md) | M15, M16 | Go migration tooling dan rehearsal |
| [B07](batches/B07.md) | M17, M18 | Pilot readiness dan rollout |
| [B08](batches/B08.md) | M19 | Ekstensi backend terpilih |

Gelombang adalah scope kerja, bukan ukuran jaminan satu sesi. B01/B06 bisa tertahan oleh access/backup; B07 menunggu frontend dan izin operasional; B08 optional. Approved independent local Go work dapat dilanjutkan tanpa memalsukan status external blocker. Tidak ada automatic source export, deploy, push atau cutover hanya karena QA lulus.

## Brief paket

| Paket | Capaian | Gelombang | Prioritas | Kompleksitas |
|---|---|---|---|---|
| [M00](modules/M00.md) | Baseline dan identitas environment | [B01](batches/B01.md) | P0 | Small |
| [M01](modules/M01.md) | Backup dan restore terverifikasi | [B01](batches/B01.md) | P0 | Medium |
| [M02](modules/M02.md) | Containment keamanan backend legacy | [B01](batches/B01.md) | P0 | Large |
| [M03](modules/M03.md) | Fondasi Go, audit/outbox dan CI | [B02](batches/B02.md) | P1 | Medium |
| [M04](modules/M04.md) | Identity, session dan contextual capability | [B02](batches/B02.md) | P0/P1 | Large |
| [M05](modules/M05.md) | Class mandiri dan enrollment lifecycle | [B02](batches/B02.md) | P1 | Large |
| [M06](modules/M06.md) | Private FileAsset dan safe outbound fetch | [B03](batches/B03.md) | P0/P1 | Medium |
| [M07](modules/M07.md) | Content dan activity revisions | [B03](batches/B03.md) | P1 | Medium |
| [M08](modules/M08.md) | Submission aggregate dan immutable revisions | [B03](batches/B03.md) | P1 | Medium |
| [M09](modules/M09.md) | Gradebook, rubric dan safe import | [B04](batches/B04.md) | P1 | Large |
| [M10](modules/M10.md) | Completion, transcript dan certificate | [B04](batches/B04.md) | P1 | Large |
| [M11](modules/M11.md) | Class attendance, permits dan disciplinary cases | [B05](batches/B05.md) | P1 | Large |
| [M12](modules/M12.md) | Configurable logbooks dan mentoring | [B05](batches/B05.md) | P1 | Medium |
| [M13](modules/M13.md) | Durable jobs, notifications dan AI suggestions | [B05](batches/B05.md) | P1/P2 | Medium |
| [M14](modules/M14.md) | Scoped reporting, API contract dan performance | [B05](batches/B05.md) | P1/P2 | Medium |
| [M15](modules/M15.md) | Go migration tool, mapping dan ledger | [B06](batches/B06.md) | P0/P1 | Large |
| [M16](modules/M16.md) | Production→staging rehearsal terverifikasi | [B06](batches/B06.md) | P0/P1 | Large |
| [M17](modules/M17.md) | Pilot readiness dan single-writer cutover | [B07](batches/B07.md) | P1 | Large |
| [M18](modules/M18.md) | Rollout, operational staging dan retained archive | [B07](batches/B07.md) | P1/P2 | Large |
| [M19](modules/M19.md) | Optional backend extensions | [B08](batches/B08.md) | P3 | Large per approved feature |

## Cara menjalankan pekerjaan nanti

Saat meminta implementasi, sebut M dan CP atau bundle Bxx, misalnya “kerjakan M03-CP03 sampai M03-CP05, QA tiap CP, laporkan sisa M03”. Pelaksana membaca brief, memeriksa dependency, mengimplementasikan bagian, menjalankan QA+regression dan memperbarui capaian M. Tidak perlu prompt per CP. Berhenti pada dependency/otorisasi baru atau failed safety gate; pekerjaan independen yang diotorisasi tetap dapat maju.

QA runner lokal M00/M03 tersedia dan menjalankan Go/Python/Playwright serta report gates. M01 preflight diuji melalui regression Go; full M01 suite dan operational backup belum selesai. Explicit operator probes/lab berbeda dari synthetic QA otomatis. Markdown berubah tidak men-trigger proses sendiri. Detail trigger dan reporting ada pada QA plan.

Kriteria sukses suatu M: semua required D dan case/evidence terpetakan, latest required regression/shared gates lulus, manual/external requirements terpenuhi untuk scope claim. `BACKEND_VERIFIED` tidak identik `DEPLOYED`, `MIGRATED`, atau full release `VERIFIED`.

## Membaca angka capaian

Setiap brief berisi planned mandatory D dan suite IDs T. M00–M18 mempunyai **97** acceptance units planned; belum ada yang memenuhi **seluruh** klausa canonical, meskipun banyak D didukung bukti slice lokal. M19 belum memiliki frozen feature scope. Angka 0/N mencerminkan gate penuh yang belum lengkap, bukan nol tes atau penilaian source legacy nol persen bekerja.

Coverage D, pass rate tests, Go code coverage, external approval dan migration/deployment status dipisahkan. Missing/skipped/flaky/blocked required cases tidak menjadi PASS. Lihat formula dan falsification guards di QA plan.

## Melanjutkan setelah sesi terputus

Baca dashboard, relevant B/M, latest evidence refs dan actual worktree. Cocokkan tested commit/tree/schema/test manifest hash; stale evidence diuji ulang. Jangan reset user changes atau mulai migration ulang tanpa membaca committed receipts. Simpan last safe action/next action serta outstanding approval agar instruksi selanjutnya cukup menyebut checkpoint.

Rekomendasi berikut: **M10-CP04** untuk legacy outputs/closure pada lane sintetis, dengan academic policy dan real mapping tetap gate tersendiri. M13-CP03 adalah lane lain setelah AI/policy scope freeze. M15-CP01–05 sudah fixture PASS; M08-CP04, M06-CP04, M07-CP04 dan M05-CP05 masih perlu mapping/consumer closure. M00/M01/M02 tetap prioritas sebelum source live atau staging target dipakai. Package runner generik M04–M19 dan gate remote/shared belum lengkap.
