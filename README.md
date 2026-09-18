# LMS v3 backend — Go

**Update 15 September:** [M12-CP02/CP03](docs/checkpoints/2026-09-15-m12-cp02-cp03.md) lulus 8/8 frozen Go/DB+Playwright cases dengan backup SQL/custom dan actual isolated restore terpisah. M12 kini 3/4 slice lokal; canonical 0/5. Entry/revision dan scoped personal-mentor review baru di harness sintetis, belum shared `cmd/api`. Tidak ada staging/production access, delete, commit atau push.

**Update 14 September:** [M12-CP01](docs/checkpoints/2026-09-14-m12-cp01.md) lulus 4/4 frozen Go/DB+Playwright cases pada cluster sintetis baru dengan backup SQL/custom dan actual isolated restore. M12 kini 1/4 slice lokal; canonical 0/5. Endpoint period-set masih hanya di harness QA, bukan `cmd/api` runtime bersama. Tidak ada staging/production access, delete, commit atau push.

**Update 14 September 2026:** [M15-CP05 + M11-CP01](docs/checkpoints/2026-09-14-m15-cp05-m11-cp01.md) lulus pada full synthetic PostgreSQL journey: masing-masing 3/3 dan 4/4 frozen QA cases, dengan fresh SQL/custom backups dan actual isolated restores. M15 sekarang 5/5 **slice sintetis**, M11 1/4 **slice lokal**; canonical tetap M15 0/6 dan M11 0/5. Status bertanggal 13 September di bawah adalah histori. Tidak ada DB nyata, commit, atau push.

**Pelacakan pekerjaan 13 September:** mulai dari [capaian M00–M19 dan CP di dalam modul](docs/MODULE_CHECKPOINT_MAP.md), bukan nomor CP global. Posisi lokal: M03 6/6 bagian, M04 2/5, M05 4/5, M06 3/4, M07 3/4, M08 3/4 exit lokal dengan CP04 parsial, M15 4/5 slice sintetis; full acceptance terpisah. Next recommended: **M15-CP05 synthetic reconciliation/closure**; source nyata tetap memerlukan CA/backup/approval gate. Peta tersebut menunjukkan urutan dependency seluruh CP lintas M.

Repository/worktree root: **`backend-v3/backend-go`** locally. On branch `be-v3`, GitHub now shows `go.mod`, `cmd/`, `internal/`, `qa/`, `docs/` and `.github/` directly at root, without legacy NestJS files or an extra `backend-go/` wrapper. See [layout correction](docs/checkpoints/2026-09-08-git-root.md). Links to master plan/audit outside this directory are workspace references; those external documents are not bundled in this backend-only repository.

Implementation checkpoint: Go platform, synthetic protected backup/migration foundation and first real Class API journey. **Belum backend LMS lengkap atau siap staging/production.** Product scope dan acceptance tetap di [docs](docs/README.md); posisi aktual di [implementation status](docs/IMPLEMENTATION_STATUS.md).

Latest scoped work: [M15-CP03/CP04 results](docs/checkpoints/2026-09-13-m15-cp03-cp04.md) **dua synthetic local slices PASS; canonical M15 0/6**. Deterministic quarantine mapping dan atomic private shadow backfill, dua backup+actual restore, Go/DB, Playwright dan seluruh backend regression lulus. [CP01/CP02 foundation](docs/checkpoints/2026-09-13-m15-cp01-cp02.md) tetap berlaku. Tidak ada source nyata/staging/live DB yang disentuh; M08-CP04 tetap partial/BLOCKED_M15 untuk chronology nyata.

Prior Class scope: [M05-CP03–CP04 results](docs/checkpoints/2026-09-10-m05-cp03-cp04.md) **VERIFIED_LOCAL_SLICES**. Class-owned grouping, personal mentor history, target-bound invitations, explicit retake/transfer/equivalence provenance, backup/restore dan seluruh backend regression lulus. Canonical M05 tetap 0/5 karena CP05/shared/QG gates.

Prior scoped work: [M03-CP03–CP04 results](docs/checkpoints/2026-09-09-m03-cp03-cp04.md) **VERIFIED_LOCAL_SLICES**. Forward migration runner + real `cmd/api` TLS process lulus pada fresh synthetic PostgreSQL; CP04 12/12 API cases, CP03/CP04 coverage 76.4%/69.4%, dan CP08–CP12 regressions PASS.

CP05 continuation **PASS_SYNTHETIC_ONLY** on 7 September: [actual PostgreSQL backup/restore + transaction integration evidence](docs/checkpoints/2026-09-07-cp05-closure.md). Local cluster stopped and retained; production/staging untouched.

## Yang sudah executable

- `cmd/api`: bootstrap health-only mode serta `v3-local-synthetic` real TLS business runtime. Mode business memasang cookie identity + contextual Class handler, dependency readiness dan restricted pools; startup tidak menjalankan DDL. Shared mode belum diaktifkan.
- `cmd/migrate`: forward-only operator runner dengan katalog terkompilasi, checksum/dependency/lock dan fresh restored-backup gate; checkpoint ini hanya mengizinkan cluster sintetis baru.
- `cmd/class-lab`: separate opt-in loopback fixture entry point for real Class handlers, hashed opaque bearer sessions and restricted PostgreSQL runtime. No dotenv/shared DSN, seed, migration or login bypass at HTTP startup. Class routes are not yet wired into `cmd/api`; no actual issuer/provider login.
- `cmd/identity-lab`: HTTPS cookie/CSRF fixture lokal yang juga memasang contextual Class/participation/private-file routes untuk QA. `cmd/bridge-fixture` hanya issuer sintetis offline. [OpenAPI identity lokal](docs/api/cp11-cp12.openapi.json); adapter legacy/real issuer, malware scanner dan private object provider belum tersedia.
- `cmd/baseline`: scan seluruh `src/**/*.ts` dua checkout backend, Git commit/dirty/source hash dan declaration locations. Config dibandingkan sebagai fingerprint, tanpa mencetak password/host/user/database. **Tidak membuka koneksi DB**, menjalankan NestJS, atau membuktikan deployment/schema aktual.
- `cmd/db-inspect`: operator-only pgx CLI dengan explicit staging acknowledgement, verified TLS, one connection, transaction read-only, static public-catalog queries dan timeouts. Tidak dipanggil API atau synthetic QA. [Inspeksi staging terbaru](docs/checkpoints/2026-09-06-staging-ca-verified.md) berhasil membaca metadata 20 tabel public dengan CA pengguna; credential teramati memiliki hak luas, bukan least-privilege runtime.
- `qa/run.py`: Go format/build/unit/vet/coverage → Pytest operator/self-tests → Playwright Python ke binary API baru khusus M03 → evidence/coverage/blockers. M00 menambahkan actual local source inventory. Full suite M00–M19 belum tersedia.
- `cmd/backup-plan --plan-only`: reads strict JSON from stdin, no env/DB/process execution; checks declarations and refuses unsafe/occupied destination. `PLAN_VALID_NOT_AUTHORIZED` is not real backup, verified disk/ACL/approval, or execution permission.
- `qa.release`, `qa.linux_qa`, `qa.postgres_lab`: separate static-artifact, Linux race/non-root, and explicitly authorized NEW synthetic PostgreSQL lab workflows. Lab never accepts existing/shared DSNs and retains cluster/backups; see checkpoint for actually executed vs pending evidence.
- CI bootstrap di [repository workflow](.github/workflows/backend-go-qa.yml), sekarang di root yang sama dengan `go.mod`. Historical checkpoint push status dicatat di masing-masing laporan; publikasi layout terbaru ada di [handoff Git](docs/checkpoints/2026-09-08-git-root.md). Remote CI/deploy tidak diasumsikan lulus dari push.

## Menjalankan checkpoint lokal (PowerShell)

Prasyarat: Git, Go **1.27.1**, Python 3.12, akses download dependency saat setup. Python lokal saat checkpoint ini 3.12.2; production Python bukan runtime aplikasi. Go memakai toolchain terpin; patch berikutnya wajib direview dan QA ulang. [Go releases](https://go.dev/doc/devel/release).

```powershell
cd D:\Projects\lms-v2\backend-v3\backend-go
python -m venv .venv
.\.venv\Scripts\python.exe -m pip install -r qa/requirements.lock
$env:GOTOOLCHAIN = 'go1.27.1'
$env:GOMODCACHE = "$PWD\.cache\go-mod"
$env:GOCACHE = "$PWD\.cache\go-build"
go version
go mod download
.\.venv\Scripts\python.exe -m qa.run --package M00 --workspace D:\Projects\lms-v2 --update-docs
.\.venv\Scripts\python.exe -m qa.run --package M03 --update-docs
```

Jalankan command M00 dan M03 secara terpisah; jangan memakai success chaining untuk menyembunyikan `BLOCKED`. M03 bootstrap lokal independen dapat diuji saat gate operasional M00 tertahan. Di shell Linux gunakan `.venv/bin/python` dan install Go 1.27.1 terlebih dahulu. Runner tidak mendownload Go/module/dependency, karena child `GOPROXY=off`; initial `go version` di atas menyiapkan toolchain jika Go terpasang berbeda.

Tidak perlu menginstall browser: QA menggunakan [Playwright APIRequestContext](https://playwright.dev/python/docs/api-testing), bukan browser/UI. Dependencies Python dikunci versi dalam `qa/requirements.lock`; artifact-hash lock/wheel provenance dan vulnerability scan penuh masih pekerjaan hardening M03, bukan dianggap selesai.

| Exit runner | Makna |
|---|---|
| `0` | Full module verified, **atau** hanya bootstrap checks dengan flag eksplisit `--checks-only`. Periksa `package_status`; flag tidak mengubah coverage/gates. |
| `1` | Build/test/collection/hash/runner error; baca evidence. |
| `2` | Checks lokal lulus, tetapi module evidence/dependencies/approvals belum lengkap. Kondisi normal pada bootstrap saat ini. |

Default package runner hanya mendukung `M00` dan `M03`, profile `local-synthetic`. CP08–CP10 DB/API integration memakai opt-in lab terpisah, bukan full M01/M04/M05 suite. Paket lain/profile staging/live ditolak, tidak ditandai hijau. Satu runner aktif per environment melalui OS lock; crash melepas lock tanpa menghapus lock file. Tidak ada daemon/background agent yang menjanjikan pekerjaan berlanjut setelah sesi ditutup.

Setelah izin membuat cluster lokal baru, jalankan CP06 regression + CP08–CP10 sebagai satu command (PostgreSQL tools diperlukan):

```powershell
.\.venv\Scripts\python.exe -m qa.postgres_lab --create-local-synthetic-cluster --class-journey --pg-bin 'C:\Program Files\PostgreSQL\18\bin'
```

Harness membackup/restore sebelum fixture/schema writes, menjalankan Go integration dan Playwright Python, lalu menghentikan server tanpa menghapus cluster/data/backup. Setiap rerun menggunakan direktori baru; tidak menyentuh staging/live. Default coverage 38.6% dan selected API coverage 81.4% punya denominator berbeda; [detail evidence](docs/checkpoints/2026-09-07-cp08-cp10.md).

Angka pada paragraf sebelumnya adalah historical CP10. Untuk CP11–CP12 beserta regresi CP08–CP10, gunakan flag `--identity-journey` menggantikan `--class-journey`. QA CP11 terpicu setelah installment CP11; CP12 hanya dimulai bila CP11 lulus dan backup berikutnya terbukti. `qa.checkpoint_report` menggabungkan evidence terbaru dengan M00/M03/Linux, menolak hasil stale/missing, serta menghitung deliverable dan statement coverage. [Command lengkap dan run final](docs/checkpoints/2026-09-08-cp11-cp12.md#reproduksi-dan-checkpoint-berikutnya).

Untuk M03-CP03–CP04 gunakan `--runtime-journey`; harness membuat cluster baru, backup+actual restore, menjalankan runner/CLI dan seluruh CP08–CP12 regression, lalu menguji proses `cmd/api` nyata dengan Playwright. [Hasil final](docs/checkpoints/2026-09-09-m03-cp03-cp04.md).

Untuk rantai terbaru sampai bounded M08-CP04 gunakan `--submission-journey`. Flag ini meng-include content/file/participation/worker journeys, membackup+restore sebelum tiap schema installment, lalu menjalankan Go/DB dan Playwright API. Gunakan hanya pada NEW synthetic cluster; [hasil dan batas receipt](docs/checkpoints/2026-09-13-m08-cp03-cp04.md).

Untuk M15-CP01/CP02 synthetic preflight/provenance beserta seluruh dependency, gunakan `--migration-foundation-journey` pada NEW loopback cluster saja. Harness membuat backup SQL/dump baru dan actual isolated restore sebelum tiap CP; [hasil dan batas source nyata](docs/checkpoints/2026-09-13-m15-cp01-cp02.md). Tidak menerima DSN staging/production.

Untuk menambah M15-CP03/CP04 mapping dan shadow backfill sintetis pada dependency journey yang sama, gunakan `--migration-backfill-journey`. Opsi ini mengambil backup SQL/dump baru dan actual isolated restore sebelum masing-masing CP; hasil QA dan batasnya dicatat terpisah dari canonical M15.

## Evidence dan status otomatis

Setiap run membuat `.qa-artifacts/<UTC-package-unique-id>/` baru: manifest summary, result per case, Go coverage, command logs, binary hashes, JUnit, report preview. Run lama tidak ditimpa. `--update-docs` memperbarui hanya blok `QA:Mxx` di module dan dashboard, mempertahankan scope/checklist manual, lalu mencatat `publication.json`. Update atomic **per file**, bukan transaksi dua file; bila publikasi terputus, run ID berbeda atau receipt tidak ada berarti publikasi belum lengkap. Rerun checkpoint untuk rekonsiliasi.

Hash mencakup source Go/Python, executable manifests/lock dan workflow. Documentation-only edits tidak mengubah executable hash. Source berubah setelah run → evidence lama bukan current confidence. Local artifacts belum signed/cryptographically attested dan belum memakai WORM; exclusive creation mencegah overwrite oleh runner, bukan modifikasi oleh operator filesystem. Tidak dapat digunakan sebagai izin migrasi/deploy.

`.gitignore` mengecualikan env, toolchain, venv, evidence, dump dan binary. **Gitignore bukan ACL atau enkripsi**: jangan simpan backup/PII nyata di folder evidence sebelum aturan akses, enkripsi dan backup M01 terverifikasi. Tidak ada dump production dibuat di checkpoint ini.

## Safety yang tidak boleh dilompati

API bootstrap tetap tanpa DB connection; mode business hanya mengizinkan marker-bound cluster sintetis lokal dan menolak staging/production. PostgreSQL dipakai restricted API/identity/worker pools serta operator runner terpisah; startup tidak menjalankan migration/seed. Worker substrate ada, tetapi belum ada provider/email nyata atau file-delete endpoint. Harness CP03–CP06 telah membuktikan dump+actual isolated restore lokal; ini bukan backup shared/production. Hak role nyata, production distinctness dan operational backup/restore tetap gate terpisah.

M00 harus menutup unknown deployed SHA dan identitas source/target/role/CA; M01 harus membuat **backup valid dan restore proof sebelum setiap migrasi**. Copy production → staging tidak boleh overwrite existing staging, menjalankan `--clean`, atau menghapus row/schema/object. Live tetap legacy owner sampai cutover disetujui. [Baseline dan blocker](docs/baseline/LOCAL_BASELINE.md).

CI memakai `pull_request` (bukan `pull_request_target`), token read-only, checkout tanpa persisted credential dan action commit pin. Dasar konfigurasi: [checkout](https://github.com/actions/checkout), [setup-go](https://github.com/actions/setup-go), [setup-python](https://github.com/actions/setup-python). Bootstrap job PASS bukan full module/release acceptance. Windows race attempt terhambat compiler C 32-bit; CP04 kini memiliki actual WSL Linux race PASS untuk 17 registered Go tests dan non-root process proof. Remote CI/container execution tetap belum dijalankan.

## Inspeksi staging eksplisit (bukan QA otomatis)

```powershell
.\.venv\Scripts\python.exe -m qa.inspect_staging --staging-readonly
```

Wrapper hanya memilih `.env.staging` di service root, mengompilasi Go CLI, dan menyimpan metadata sanitised dengan run ID baru. Tidak menerima raw SQL atau export/import/deploy options. Koneksi maksimum satu, connect timeout 5s, keseluruhan probe 20s, statement timeout 5s, lock timeout 1s. Read-only transaction tidak berarti credential telah terbukti least-privilege.

Jika `TLS_CERTIFICATE_VERIFICATION_FAILED`, periksa CA **dan hostname** dari provider. Unduh CA project dari Supabase Dashboard → Database Settings → SSL Configuration; simpan lokal lalu tambahkan `DB_SSL_ROOT_CERT=<path>` di `.env.staging` sendiri. Relative path dihitung dari lokasi env; system certificate store tidak dimutasi. Tidak menggunakan `InsecureSkipVerify`, `sslmode=require`, atau mengambil sertifikat dari endpoint belum trusted lalu otomatis mempercayainya. [Supabase SSL](https://supabase.com/docs/guides/platform/ssl-enforcement).

Lihat [template DB aman](docs/examples/db-inspect.env.example), [template runtime aman](docs/examples/runtime.env.example), dan [brief sesi staging](docs/checkpoints/2026-09-06-staging-inspection.md). `.env.example` milik pengguna tidak ditimpa; ada nilai `SESSION_SECRET` yang sama dengan staging. Kedua root env sekarang di-ignore dan QA menolak jika tracked; jangan force-add. Nilai secret tidak dicetak atau disalin ke template baru. `.gitignore` tidak menggantikan ACL/enkripsi atau rotasi jika secret pernah dipublikasikan.
