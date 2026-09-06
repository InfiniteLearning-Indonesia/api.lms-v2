# Brief hasil — staging inspection, 6 September 2026 WIB

> **Riwayat sebelum CA diberikan.** Blocker TLS dan S03 di bawah telah diperbarui oleh [checkpoint CA terverifikasi](2026-09-06-staging-ca-verified.md): public catalog berhasil dibaca dengan verified TLS/read-only. Pernyataan kegagalan/no SQL pada dokumen ini tetap disimpan sebagai hasil sesi terdahulu, bukan status terkini.

Sesuai permintaan sesi terbatas, pekerjaan dibatasi pada **empat sub-deliverable M00/M03**. Ini bukan empat paket M selesai, bukan backend Class lengkap, dan bukan izin migrasi. Semua implementasi tetap Go di `backend-v3/backend-go`; orchestration QA memakai Python.

## 1. Hasil per item

| Item sesi | Capaian dan bukti | Status / batas kelulusan |
|---|---|---|
| S01 — Pembaca konfigurasi dan perlindungan env | `internal/platform/dbinspect/config.go`: explicit file, selected DB keys, literal credentials tanpa expansion, alias/port/TLS validation, redacted formatting/errors; env asli tetap utuh. `.env.staging` dan root `.env.example` sekarang ignored, QA memeriksa keduanya tidak tracked; safe templates terpisah. | DONE untuk scope config lokal, diuji Go/Python. Tidak membuktikan validitas credential/provider. |
| S02 — Operator CLI dengan batas aman | `cmd/db-inspect`, `qa/inspect_staging.py`: explicit acknowledgement, single bounded connection, verified TLS/no plaintext fallback, tidak terhubung API/automatic CI. CLI tanpa ack/config valid ditolak; handshake staging yang gagal tidak diteruskan menjadi SQL. | DONE untuk CLI/config/TLS refusal boundary. Jalur sukses SQL di PostgreSQL nyata masih membutuhkan S03. |
| S03 — Inventory schema/privilege staging | `catalog.go`/`probe.go` telah diimplementasikan untuk public tables, columns, constraints, indexes, policy metadata, effective privilege observations, extensions, size/row estimates dan catalog hash; readonly transaction dan timeouts terpasang di kode. | BLOCKED: actual provider TLS certificate verification gagal sebelum SQL. Queries dan transaction success path belum verified pada PostgreSQL nyata; tidak mengklaim catalog sudah tersedia. |
| S04 — Regression QA dan checkpoint | Empat Go test functions + satu Python operator case ditambahkan; registry expected cases, build CLI, evidence renderer dan CI dependency bootstrap diperbarui. M00/M03 regression lulus; `go mod verify` lulus; handoff ini tersedia. | DONE untuk scope local regression/reporting. Remote CI, race detector, full QG dan release tetap pending. |

Dengan definisi scope di atas: **3 item lokal selesai, 1 item operasional terhambat**. Tidak menaikkan coverage canonical D: M00 **0/5**, M03 **0/5** fully verified; masing-masing broad deliverable masih memiliki acceptance lain. Total roadmap M00–M18 tetap 0/97. Jangan menyamakan scope-session progress dengan completion LMS v3.

## 2. QA yang benar-benar dijalankan

| Paket | Latest published run | Bootstrap checks | Python cases | Go cases |
|---|---|---:|---:|---:|
| M00 | `20260905T185317Z-M00-c8f47997ceae` | 16/16 PASS | 12 PASS | 13 PASS |
| M03 | `20260905T185200Z-M03-fccdc08ed49b` | 19/19 PASS | 15 PASS | 13 PASS |

Empat bootstrap checks adalah aggregate Go format/build/unit/vet; Python cases menambah sisanya. Go/Python shared regression dipakai ulang antarrun, jadi angka tidak boleh dijumlahkan sebagai jumlah unique tests. Full-package pass rate N/A karena acceptance suite belum lengkap. `package_status=BLOCKED` dan default exit 2 disengaja meski local checks PASS.

Go statement coverage terkini **67,4%**, turun dari bootstrap lama 76,6% karena penambahan jalur DB/operator yang belum seluruhnya terinstrumentasi/teruji. Coverage rendah tidak disembunyikan dengan mengecualikan package. API/CLI subprocess tests dan percobaan TLS staging tidak menambah instrumented coverage `go test`; actual SQL success path tetap belum diuji.

Tambahan negative/positive checks meliputi:

- Literal password dengan karakter khusus dipertahankan tanpa dicetak; formatted settings/JSON tidak mengandung host/password.
- Duplicate/conflicting DB keys, port salah, SSL disabled, missing/nonregular config dan CA invalid ditolak.
- TLS lokal nyata: untrusted certificate dan wrong hostname ditolak; explicit trusted synthetic certificate dengan hostname benar berhasil. Tidak memakai bypass certificate verification.
- Ambient PG options/password tidak mengambil alih selected configuration/runtime parameters.
- Static catalog-only query scope dan sanitized driver errors diperiksa; ini **bukan** PostgreSQL syntax/integration validation.
- CLI tanpa acknowledgement, relative config path, missing file, atau private env yang tracked ditolak oleh regression gates.
- Existing Playwright Python API checks, Go shutdown/config/source inventory tests dan QA evidence/lock/self-tests tetap lulus.

`go mod verify`: **all modules verified**. pgx dipin `v5.10.0` dengan `go.sum`; ini integrity check dependency cache, bukan vulnerability scan. Workflow YAML berhasil diparse dan token tetap `contents: read`; workflow remote belum dijalankan. Race detector Windows tetap belum verified dari blocker compiler 64-bit sesi sebelumnya.

## 3. Evidence dan source identity

Current tested executable source hash kedua published QA run:

`3babfd10b45359193dec90e0a551086fe1bb915b9d7d6e76c0489b03ff7d14ad`

Manifest SHA256:

- M00: `98526a5767b40bbc63b3df8541628bc360d60a9440b15fc033150518b0fe6736`.
- M03: `e97c4d56e9a6119322229d4bef31d69e2e177e5ffe5874e5ac78a411ea627e69`.

Kedua run memiliki `publication.json`; module/dashboard generated sections sudah cocok dengan run ID masing-masing. Raw evidence dipertahankan lokal di `.qa-artifacts/<run_id>/`. Source hash kini **tidak** memasukkan root `.env.example` milik pengguna karena diperlakukan sebagai file privat; safe templates di `docs/examples` ikut hashed. Tidak ada isi secret pada Markdown/command arguments.

Riwayat koneksi staging yang dipertahankan:

| Probe run | Hasil |
|---|---|
| `20260905T184103Z-staging-inspect-f555a081393b` | Percobaan dalam sandbox: connection failure; tidak ada catalog. |
| `20260905T184252Z-staging-inspect-c212917373fd` | Setelah izin network: `TLS_CERTIFICATE_VERIFICATION_FAILED`, `read_only_transaction=false`; berhenti sebelum query. |

Probe di atas memakai source hash historis `b8c5a692d34cb7b8b67029c7dce6c2f2cd4ddaabccac47dba554c71ff3f3ee1d`, sebelum refinement reporting/QA/env-ignore terakhir. Bukti itu adalah diagnosis historis, **bukan current successful operational attestation**. Tidak mengulang koneksi gagal hanya untuk menghasilkan run ID baru; rerun setelah CA/hostname diperbaiki. Raw `public_tables` tidak tersedia; tampilan awal count 0 berarti tidak terukur, **bukan staging mempunyai nol tabel**. Wrapper terbaru menampilkan null bila catalog belum tersedia.

## 4. Staging configuration dan risiko yang ditemukan

Pengguna telah menunjuk `backend-go/.env.staging` sebagai staging dan memberikan izin membacanya. File dibaca secara lokal tanpa menampilkan nilai credential. DB_SSL meminta TLS dan alias username cocok. Non-DB integration keys tidak diteruskan ke driver atau API runtime.

Fingerprint host/port/user/database dari file baru masih sama dengan local `backend-live/.env`, `backend/.env.staging`, dan `backend/.env`. Ini **tidak membuktikan deployment live dan staging memakai database sama**; file lokal legacy mungkin bukan secret deployment aktual. Gunakan penandaan pengguna untuk scope inspeksi staging, tetapi jangan menggunakan local `backend-live/.env` sebagai sumber production copy sebelum identitasnya dikonfirmasi.

Root `.env.example` mengandung nilai `SESSION_SECRET` yang sama dengan `.env.staging`. Belum dibuktikan bahwa secret itu pernah dipublikasikan atau aktif di deployment. Isi kedua file **tidak diubah/dihapus**; keduanya memang belum tracked lalu dilindungi ignore rule dan QA guard. Contoh tanpa nilai tersebut ada di [runtime template](../examples/runtime.env.example) dan [DB inspection template](../examples/db-inspect.env.example). Jangan force-add file privat. Jika nilai tersebut nyata dan pernah tersebar, koordinasikan rotasi/session invalidation sebagai perubahan operasional terpisah—tidak dilakukan otomatis di sesi ini.

## 5. Langkah berikutnya

1. **TLS prerequisite:** unduh CA database staging dari Supabase Dashboard → Database Settings → SSL Configuration. Periksa juga hostname connection string dari provider; diagnosis saat ini membuktikan certificate verification failure, belum membedakan CA/hostname/expiry secara pasti. Simpan CA lokal, misalnya `backend-go/certs/staging-provider-ca.crt`, lalu tambahkan `DB_SSL_ROOT_CERT=certs/staging-provider-ca.crt` ke `.env.staging`. Tidak perlu mengirim password/private key atau mengganti trust store global. [Panduan Supabase SSL](https://supabase.com/docs/guides/platform/ssl-enforcement).
2. Jalankan explicit operator command: `.\.venv\Scripts\python.exe -m qa.inspect_staging --staging-readonly`. System trust digunakan bila sudah valid; custom CA ditambahkan in-memory khusus koneksi ini. Tidak menerima `sslmode=require`/`InsecureSkipVerify` sebagai pengganti verifikasi.
3. Setelah TLS valid, verifikasi actual readonly transaction dan review catalog/role flags. Write-capable role dapat diamati dalam readonly transaction, tetapi hasil itu **tidak mensertifikasi least privilege**. Operator schema scope saat ini public-only; jobs/triggers/function privilege closure, R2 files, semua namespace, exact row reconciliation dan headroom masih pekerjaan berikutnya.
4. Konfirmasikan production secret/deployment reference yang benar dan berbeda; lengkapi owner/reviewer/capacity. Baru lanjut M01 backup source/target yang terverifikasi dan isolated restore proof. Jangan menimpa existing staging atau menjalankan restore `--clean`.
5. Setelah gates selesai: lanjut M03 actual runtime pool/namespace/transaction/audit/outbox, kemudian M04/M05 identity/Class. Independent local work seperti non-root artifact dan Linux CI/race bisa dikerjakan terpisah; jangan menandai dependency selesai dari synthetic checks.

Sesuai skill Supabase/Postgres, pekerjaan koneksi nyata **berhenti pada TLS failure**, bukan mengendurkan trust. Read-only transaction juga bukan sandbox untuk arbitrary SQL; tool hanya mengirim static catalog queries. Desain transaction mengikuti [PostgreSQL read-only transactions](https://www.postgresql.org/docs/current/sql-set-transaction.html), driver mengikuti [pgx documentation](https://pkg.go.dev/github.com/jackc/pgx/v5).

## 6. Yang sengaja tidak dilakukan

Tidak ada query SQL staging yang berhasil dijalankan pada sesi ini, pembacaan baris murid, production DB connection, DB insert/update/delete/truncate/drop, backup SQL baru, copy/export/import/restore, migration, perubahan grant/RLS, frontend, legacy source mutation, deployment, commit atau push. API Go tetap loopback bootstrap dengan readiness 503. Existing user's worktree deletions tetap dipertahankan.

Status backup/migration: **NOT_EXECUTED**. Karena tidak ada migrasi, tidak ada backup yang diklaim berhasil. Sebelum migrasi apa pun, backup+restore evidence tetap wajib. Kelanjutan cukup memakai checkpoint ini dan latest M00/M03 reports; tidak perlu mengulang pembuatan fondasi dari nol.
