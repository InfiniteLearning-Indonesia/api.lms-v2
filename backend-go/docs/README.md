# LMS v3 — Brief eksekusi backend Go

Service root: `D:/Projects/lms-v2/backend-v3/backend-go`. Branch rencana implementasi: `be-v3`. Versi brief: 1.0, 5 September 2026.

**Current continuation:** [CP06–CP07 implementation/evidence](checkpoints/2026-09-07-cp06-cp07.md), [CP07 security/environment handoff](baseline/CP07_ENVIRONMENT_SECURITY_HANDOFF.md), [selectable checkpoint status](checkpoints/NEXT_FIVE_CHECKPOINTS.md). CP06 synthetic QA PASS, CP07 PARTIAL/BLOCKED_INPUT, CP08–CP10 not started. M00/M03 bootstrap runner exists; M01 offline preflight and synthetic backup/restore tested, operational backup still pending. Later paragraphs describing an unavailable runner are historical planning context, not current runtime status.

**Status sekarang: implementasi bootstrap dimulai setelah otorisasi pengguna.** M00 source inventory dan M03 Go/QA lokal tersedia; full module masih pending. Panduan executable ada di [service README](../README.md), hasil aktual di dashboard. Tidak ada shared DB copy/mutation/backup, commit/push, frontend atau deployment. Backup dan fixture writes hanya pada cluster sintetis lokal baru yang dihentikan serta dipertahankan. Dokumentasi lengkap dan bootstrap checks lulus tidak berarti seluruh paket terverifikasi.

## Mulai dari sini

1. [ImplementationPlan.MD](../../../ImplementationPlan.MD): keputusan domain, keamanan dan migrasi utama.
2. [IMPLEMENTATION_STATUS.md](IMPLEMENTATION_STATUS.md): posisi seluruh paket, blocker dan checkpoint berikutnya.
3. [QA_AUTOMATION_PLAN.md](QA_AUTOMATION_PLAN.md): trigger runner, shared gates, evidence dan perhitungan coverage.
4. Pilih brief gelombang Bxx, lalu Mxx di dalamnya. Satu instruksi dapat mencakup beberapa paket dengan QA setiap checkpoint.

Referensi faktual: [audit codebase](../../../LMS_CODEBASE_AUDIT_AND_REMEDIATION.md), [benchmark Moodle](../../../LMS_Modern_Moodle_Benchmark_Architecture_Business_Flow.md). Brief ini menurunkan master plan menjadi acceptance contract; tidak menyalin schema Moodle atau mengubah target ke TypeScript.

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

Saat meminta implementasi, sebut scope Bxx atau rentang M beserta batas environment/operasi. Pelaksana membaca brief, memeriksa dependency, mengimplementasikan paket, menjalankan QA+regression, mencatat evidence lalu melanjutkan paket dependency-ready tanpa perlu prompt per M. Berhenti untuk keputusan/otorisasi baru, failed safety gate, limit atau external blocker; checkpoint tetap tersimpan.

QA runner lokal M00/M03 tersedia dan menjalankan Go/Python/Playwright serta report gates. M01 preflight diuji melalui regression Go; full M01 suite dan operational backup belum selesai. Explicit operator probes/lab berbeda dari synthetic QA otomatis. Markdown berubah tidak men-trigger proses sendiri. Detail trigger dan reporting ada pada QA plan.

Kriteria sukses suatu M: semua required D dan case/evidence terpetakan, latest required regression/shared gates lulus, manual/external requirements terpenuhi untuk scope claim. `BACKEND_VERIFIED` tidak identik `DEPLOYED`, `MIGRATED`, atau full release `VERIFIED`.

## Membaca angka capaian

Setiap brief berisi planned mandatory D dan suite IDs T. Saat ini M00–M18 memiliki **97** acceptance units planned; semuanya belum diverifikasi oleh QA suite ini. M19 belum memiliki frozen feature scope. Angka 0/N mencerminkan belum adanya evidence, **bukan hasil test atau penilaian bahwa source legacy nol persen bekerja**.

Coverage D, pass rate tests, Go code coverage, external approval dan migration/deployment status dipisahkan. Missing/skipped/flaky/blocked required cases tidak menjadi PASS. Lihat formula dan falsification guards di QA plan.

## Melanjutkan setelah sesi terputus

Baca dashboard, relevant B/M, latest evidence refs dan actual worktree. Cocokkan tested commit/tree/schema/test manifest hash; stale evidence diuji ulang. Jangan reset user changes atau mulai migration ulang tanpa membaca committed receipts. Simpan last safe action/next action serta outstanding approval agar instruksi selanjutnya cukup menyebut checkpoint.

Pekerjaan berikut yang aman adalah review brief lalu meminta implementasi scope pertama yang dipilih. Perintah `[LAKSANAKAN]` yang menghasilkan folder ini hanya menyetujui **brief dan rencana QA**, tidak otomatis seluruh M00–M19 implementation.
