# LMS v3 backend — Go

Implementation checkpoint: M00 local source inventory + M03 local runtime/QA bootstrap + explicit Go staging-inspection tooling. **Belum backend LMS lengkap atau siap staging/production.** Product scope dan acceptance tetap di [docs](docs/README.md); posisi aktual di [implementation status](docs/IMPLEMENTATION_STATUS.md).

Latest scoped work: [CP06–CP07](docs/checkpoints/2026-09-07-cp06-cp07.md). CP06 verifies synthetic archive corruption/missing-object/data gates and database privilege denial; current Linux race/non-root and M00/M03 regression PASS. CP07 refreshes staging metadata and provides an [environment/security handoff](docs/baseline/CP07_ENVIRONMENT_SECURITY_HANDOFF.md), but remains blocked on production/provider/owner inputs. [CP08–CP10](docs/checkpoints/NEXT_FIVE_CHECKPOINTS.md) remain unstarted. [CP01–CP05 history](docs/checkpoints/2026-09-06-five-checkpoints.md) retained.

CP05 continuation **PASS_SYNTHETIC_ONLY** on 7 September: [actual PostgreSQL backup/restore + transaction integration evidence](docs/checkpoints/2026-09-07-cp05-closure.md). Local cluster stopped and retained; production/staging untouched.

## Yang sudah executable

- `cmd/api`: HTTP Go standard library, explicit loopback-only binding, timeouts, graceful shutdown, generated request ID, controlled error/log fields. `/health/live` 200; `/health/ready` **503** sampai DB/identity/ownership siap. Business/auth endpoints belum ada; 404 bukan bukti RBAC selesai.
- `cmd/baseline`: scan seluruh `src/**/*.ts` dua checkout backend, Git commit/dirty/source hash dan declaration locations. Config dibandingkan sebagai fingerprint, tanpa mencetak password/host/user/database. **Tidak membuka koneksi DB**, menjalankan NestJS, atau membuktikan deployment/schema aktual.
- `cmd/db-inspect`: operator-only pgx CLI dengan explicit staging acknowledgement, verified TLS, one connection, transaction read-only, static public-catalog queries dan timeouts. Tidak dipanggil API atau synthetic QA. [Inspeksi staging terbaru](docs/checkpoints/2026-09-06-staging-ca-verified.md) berhasil membaca metadata 20 tabel public dengan CA pengguna; credential teramati memiliki hak luas, bukan least-privilege runtime.
- `qa/run.py`: Go format/build/unit/vet/coverage → Pytest operator/self-tests → Playwright Python ke binary API baru khusus M03 → evidence/coverage/blockers. M00 menambahkan actual local source inventory. Full suite M00–M19 belum tersedia.
- `cmd/backup-plan --plan-only`: reads strict JSON from stdin, no env/DB/process execution; checks declarations and refuses unsafe/occupied destination. `PLAN_VALID_NOT_AUTHORIZED` is not real backup, verified disk/ACL/approval, or execution permission.
- `qa.release`, `qa.linux_qa`, `qa.postgres_lab`: separate static-artifact, Linux race/non-root, and explicitly authorized NEW synthetic PostgreSQL lab workflows. Lab never accepts existing/shared DSNs and retains cluster/backups; see checkpoint for actually executed vs pending evidence.
- CI bootstrap di [repository workflow](../.github/workflows/backend-go-qa.yml). File ini harus berada di root Git agar GitHub menemukannya; semua service/QA code tetap di `backend-go`. Belum push/CI remote/deploy.

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

Hanya `M00` dan `M03`, profile `local-synthetic`, yang didukung. M01 memiliki predicate regression saja, bukan backup executable. Paket lain/profile staging/live ditolak, tidak ditandai hijau. Satu runner aktif per environment melalui OS lock; crash melepas lock tanpa menghapus lock file. Tidak ada daemon/background agent yang menjanjikan pekerjaan berlanjut setelah sesi ditutup.

## Evidence dan status otomatis

Setiap run membuat `.qa-artifacts/<UTC-package-unique-id>/` baru: manifest summary, result per case, Go coverage, command logs, binary hashes, JUnit, report preview. Run lama tidak ditimpa. `--update-docs` memperbarui hanya blok `QA:Mxx` di module dan dashboard, mempertahankan scope/checklist manual, lalu mencatat `publication.json`. Update atomic **per file**, bukan transaksi dua file; bila publikasi terputus, run ID berbeda atau receipt tidak ada berarti publikasi belum lengkap. Rerun checkpoint untuk rekonsiliasi.

Hash mencakup source Go/Python, executable manifests/lock dan workflow. Documentation-only edits tidak mengubah executable hash. Source berubah setelah run → evidence lama bukan current confidence. Local artifacts belum signed/cryptographically attested dan belum memakai WORM; exclusive creation mencegah overwrite oleh runner, bukan modifikasi oleh operator filesystem. Tidak dapat digunakan sebagai izin migrasi/deploy.

`.gitignore` mengecualikan env, toolchain, venv, evidence, dump dan binary. **Gitignore bukan ACL atau enkripsi**: jangan simpan backup/PII nyata di folder evidence sebelum aturan akses, enkripsi dan backup M01 terverifikasi. Tidak ada dump production dibuat di checkpoint ini.

## Safety yang tidak boleh dilompati

API runtime tetap tanpa DB connection dan menolak mode staging/production. Driver PostgreSQL pgx v5.10.0 kini hanya dipakai operator CLI `cmd/db-inspect`; bukan implicit startup hook. Tidak ada SQL migration, seed, provider/email integration, file-delete endpoint atau worker DB. Predicate backup/approval masih synthetic evidence-shape checks, bukan dump/restore proof. TLS trust/hostname diuji dengan server lokal nyata; staging kemudian berhasil terverifikasi menggunakan CA pengguna, tanpa bypass. Hak role, production distinctness dan backup/restore tetap gate terpisah.

M00 harus menutup unknown deployed SHA dan identitas source/target/role/CA; M01 harus membuat **backup valid dan restore proof sebelum setiap migrasi**. Copy production → staging tidak boleh overwrite existing staging, menjalankan `--clean`, atau menghapus row/schema/object. Live tetap legacy owner sampai cutover disetujui. [Baseline dan blocker](docs/baseline/LOCAL_BASELINE.md).

CI memakai `pull_request` (bukan `pull_request_target`), token read-only, checkout tanpa persisted credential dan action commit pin. Dasar konfigurasi: [checkout](https://github.com/actions/checkout), [setup-go](https://github.com/actions/setup-go), [setup-python](https://github.com/actions/setup-python). Bootstrap job PASS bukan full module/release acceptance. Windows race attempt terhambat compiler C 32-bit; CP04 kini memiliki actual WSL Linux race PASS untuk 17 registered Go tests dan non-root process proof. Remote CI/container execution tetap belum dijalankan.

## Inspeksi staging eksplisit (bukan QA otomatis)

```powershell
.\.venv\Scripts\python.exe -m qa.inspect_staging --staging-readonly
```

Wrapper hanya memilih `.env.staging` di service root, mengompilasi Go CLI, dan menyimpan metadata sanitised dengan run ID baru. Tidak menerima raw SQL atau export/import/deploy options. Koneksi maksimum satu, connect timeout 5s, keseluruhan probe 20s, statement timeout 5s, lock timeout 1s. Read-only transaction tidak berarti credential telah terbukti least-privilege.

Jika `TLS_CERTIFICATE_VERIFICATION_FAILED`, periksa CA **dan hostname** dari provider. Unduh CA project dari Supabase Dashboard → Database Settings → SSL Configuration; simpan lokal lalu tambahkan `DB_SSL_ROOT_CERT=<path>` di `.env.staging` sendiri. Relative path dihitung dari lokasi env; system certificate store tidak dimutasi. Tidak menggunakan `InsecureSkipVerify`, `sslmode=require`, atau mengambil sertifikat dari endpoint belum trusted lalu otomatis mempercayainya. [Supabase SSL](https://supabase.com/docs/guides/platform/ssl-enforcement).

Lihat [template DB aman](docs/examples/db-inspect.env.example), [template runtime aman](docs/examples/runtime.env.example), dan [brief sesi staging](docs/checkpoints/2026-09-06-staging-inspection.md). `.env.example` milik pengguna tidak ditimpa; ada nilai `SESSION_SECRET` yang sama dengan staging. Kedua root env sekarang di-ignore dan QA menolak jika tracked; jangan force-add. Nilai secret tidak dicetak atau disalin ke template baru. `.gitignore` tidak menggantikan ACL/enkripsi atau rotasi jika secret pernah dipublikasikan.
