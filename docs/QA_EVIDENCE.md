# Indeks hasil QA LMS v3

Pembaruan 23 November 2026. Gunakan tabel ini untuk mencari run, cakupan, dan dokumen pemeriksaan. Detail per kasus, receipt backup dan bukti sumber berada di laporan checkpoint. Penyimpanan file QA mengikuti [kebijakan retensi](QA_STORAGE_POLICY.md).

| Bukti | Run | Hasil dan cakupan |
|---|---|---|
| Browser `/fe-test` | `20261122T211331Z-local-preview-ead81cc41a30`, `journeys-192cb3ba2b7a` | 3/3 alur, 10 langkah Chrome. Admin/Class/undangan, tugas sampai nilai dirilis, dan izin per Class. [Bukti preview](LOCAL_PREVIEW.md#qa-browser-dari-prompt-preview) |
| Go preview dengan PostgreSQL lokal | `20261122T193033Z-local-preview-18d5120012dc` | 7/7 skenario API Playwright, HTTPS, backup target kosong dan restore, shutdown Go/PostgreSQL. [Bukti runtime](LOCAL_PREVIEW.md#verifikasi-sebelum-integrasi-fe-test) |
| M04 password | `20261122T151935Z-m04-password-bc74f4758b23` | QA sintetis Go/PostgreSQL dan 5 skenario API Playwright. [CP03](checkpoints/2026-11-22-m04-cp03-password.md) |
| M04 Google dan sesi | `20261122T183531Z-m04-google-92d85dc7d653` | Verifikasi provider sintetis, Go/PostgreSQL dan 5 skenario API Playwright. Google nyata menunggu target integrasi. [CP04](checkpoints/2026-11-23-m04-cp04-google.md) |
| M04 undangan Class | `20261122T190140Z-m04-class-invitations-d579f0866558` | Lulus Go/PostgreSQL dan 5 skenario API Playwright. Percobaan sebelumnya `20261122T185029Z-m04-class-invitations-96e688f39825` gagal pada integrasi fixture. [CP05](checkpoints/2026-11-23-m04-cp05-class-invitations.md) |
| M15 pembuktian importer Go | `20261122T144153Z-go-core-rehearsal-ffcec7233b7e` | Dry-run, rollback, import dan replay lulus. 17 Class draft, 132 akun historis, 118 participation. Satu enrollment ditahan dan 7.576 record sumber menunggu mapping domain. [Impor inti](checkpoints/2026-11-22-go-core-import.md) |
| Restore snapshot staging lama | `20261119T093357Z-legacy-local-restore-debaab74a513` | Restore terisolasi lulus 11/11, rekonsiliasi 21 tabel dan 7.843 record. [Restore aktual](checkpoints/2026-11-19-local-legacy-restore-accepted.md) |
| Snapshot staging lama | `20261118T131839Z-staging-baseline-3fe3bf70b4d4` | Export read-only cakupan skema aplikasi public. SQL dan custom dump berasal dari snapshot yang sama. [Snapshot](checkpoints/2026-11-18-staging-snapshot-accepted.md) |
| M01 protected package | `20261116T210959Z-age-stream-09579d7d1bf9` | 7/7 kasus native pada versi sumber yang dicatat. QA recovery lanjutan punya hasil tersendiri. [Protected package](checkpoints/2026-11-17-protected-package-accepted.md) · [Recovery lanjutan](checkpoints/2026-11-17-local-recovery.md) |
| M14 closure sintetis | `20261111T093737Z-postgres-lab-6e4bc487c4d6` | 4/4 CP lokal, 21/21 kasus langsung termasuk CP04 6/6. Run sebelumnya `20261111T084515Z-postgres-lab-dc2f748e9472` gagal. [Review M14](checkpoints/2026-11-11-m14-local-closure.md) |
| Kontrak frontend inti | OpenAPI 3.1, 23 November | Pemeriksaan statis 42 operasi, cek cakupan 22 operasi browser, contoh TypeScript strict dan tes 7 paket Go lulus. [Kontrak](../API-DOCS.MD#bukti-dan-pekerjaan-yang-tersisa) |
| Konsolidasi sebelum publikasi | 23 November | `go test ./...`, `go vet ./...`, tes Go bertag `core_rehearsal`, 49 tes operator baseline Python, dan tautan dokumen integrasi lulus. Bukti Playwright dan PostgreSQL memakai run sebelumnya pada baris di atas. |

## Membaca bukti

- Hasil PASS berlaku untuk run, versi sumber, lingkungan dan skenario yang disebutkan. Status pekerjaan per modul ada di [peta M/CP](MODULE_CHECKPOINT_MAP.md) dan [status implementasi](IMPLEMENTATION_STATUS.md).
- Lingkungan yang masih menunggu penerimaan adalah frontend produk, runtime bersama `staging-v2`, data historis di lingkungan tujuan, serta pilot/rollout. [Peta integrasi](FRONTEND_INTEGRATION_ROADMAP.md)
- Katalog lama 11 November menyimpan **166 ringkasan** dalam rekaman privat. Tabel ini menambah navigasi untuk run milestone setelah katalog dibuat. Pakai laporan tiap checkpoint untuk hash, case breakdown, dan receipt.
- Simpan evidence mentah dan backup/materi pemulihan pada lokasi privat. Dokumen ini menyimpan ringkasan sanitised untuk GitHub. [Retensi QA](QA_STORAGE_POLICY.md)
