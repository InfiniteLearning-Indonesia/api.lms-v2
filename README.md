# LMS v3 backend Go

Repositori Go untuk LMS v3 class-sentris. Admin memprovisikan akun dan membuat Class. Keanggotaan serta izin Mentor dan Mentee berlaku pada Class masing-masing. Go memegang sesi dan otorisasi.

Worktree lokal berada di `backend-v3/backend-go`. Isi worktree ini tampil langsung di root branch `be-v3`. [Layout Git](docs/checkpoints/2026-09-08-git-root.md)

## Mulai dari sini

| Keperluan | Rujukan |
|---|---|
| Sambungkan frontend produk | [Panduan API](API-DOCS.MD) · [OpenAPI inti](api/openapi/lms-v3-core.json) · [Checklist per layar](docs/INTEGRATION_CHECKLIST.md) |
| Jalankan API Go lokal | [Preview HTTPS dan PostgreSQL](docs/LOCAL_PREVIEW.md) |
| Lihat urutan integrasi, staging-v2 dan histori | [Roadmap integrasi](docs/FRONTEND_INTEGRATION_ROADMAP.md) · [Persiapan data dan operasi](docs/PROJECT_STATE_BEFORE_LIVE_STAGING.md) |
| Lihat pekerjaan per modul | [Status implementasi](docs/IMPLEMENTATION_STATUS.md) · [Peta M00–M19](docs/MODULE_CHECKPOINT_MAP.md) · [Plan dan audit](docs/references/README.md) |
| Verifikasi hasil QA | [Indeks hasil](docs/QA_EVIDENCE.md) · [Rencana QA](docs/QA_AUTOMATION_PLAN.md) · [Retensi penyimpanan](docs/QA_STORAGE_POLICY.md) |

Tiga alur browser dan tujuh skenario API preview sudah lulus lokal. Validasi frontend produk, runtime bersama `staging-v2`, dan penerimaan data historis punya tahap tersendiri. Cek indeks untuk cakupan setiap run.

## Setup lokal di Windows

Gunakan Go sesuai `go.mod`, Python 3.12, serta PostgreSQL 18 untuk lab dan preview. Dari root repositori:

```powershell
py -3.12 -m venv .venv
.\.venv\Scripts\python.exe -m pip install -r qa/requirements.lock
go mod download
go test ./...
```

Jalankan perintah preview dari [runbook preview](docs/LOCAL_PREVIEW.md#menjalankan-preview-manual). Launcher memakai cluster sintetis baru di loopback, membuktikan backup dan restore target kosong, lalu menjalankan `cmd/api` dalam mode `v3-local-preview`. `/health/ready` memeriksa kesiapan target. Pemilik terminal menjalankan `stop` untuk menutup preview. Untuk pengujian browser, cek kebutuhan build `fe-test` dan browser pada runbook.

`cmd/api` juga memiliki mode health dan jalur sintetis lain. `cmd/migrate` menyediakan katalog migrasi maju dengan gate backup untuk lab. `qa/` menyimpan launcher, Go/DB dan Playwright, serta alat operator. Buka [rencana QA](docs/QA_AUTOMATION_PLAN.md) dan laporan checkpoint terkait untuk menjalankan suite yang tepat. Bootstrap `qa.run` mendukung M00 dan M03. Full journey PostgreSQL berjalan melalui `qa.postgres_lab`. [Workflow CI](.github/workflows/backend-go-qa.yml)

## Database dan artefak

- Staging lama merupakan sumber snapshot yang dibekukan. Bukti restore lokal dan impor inti Go tersedia. `staging-v2` merupakan target terpisah yang direncanakan. [Status snapshot](docs/baseline/STAGING_LOCAL_SNAPSHOT.md) · [Restore](docs/baseline/LOCAL_LEGACY_RESTORE.md)
- Backup terverifikasi mendahului setiap migrasi. Proses terhadap data nyata mengikuti runbook dan batas targetnya. [Konfigurasi staging](docs/baseline/STAGING_CONFIGURATION_READINESS.md) · [Intake data](docs/baseline/REAL_BASELINE_INTAKE.md)
- Git berisi source, kontrak, dan ringkasan bukti yang aman dibagikan. Simpan env, kunci, dump, backup dan artefak mentah secara privat. Pakai [contoh env publik](docs/examples/runtime.env.example) untuk melihat variabel runtime.
