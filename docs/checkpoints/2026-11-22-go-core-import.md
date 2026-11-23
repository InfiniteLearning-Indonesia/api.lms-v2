# Go importer untuk pembuktian inti M15

**Pembuktian PostgreSQL lokal diterima.** Run `20261122T144153Z-go-core-rehearsal-ffcec7233b7e` selesai dengan `PASS_GO_CORE_LOCAL_REHEARSAL_ONLY`. Dry run, rollback, impor, dan replay lulus pada snapshot aktual. Review dari file mencocokkan sembilan pemeriksaan, termasuk receipt, plan, hash binary, hash snapshot dan backup, identitas target, serta state shutdown langsung dari `pg_controldata`.

Target `legacy_core_v3` tersimpan dalam keadaan berhenti. Ukuran logical run sekitar 86,7 MiB. Satu enrollment tetap ditahan dan 7.576 record domain lanjutan masih menunggu mapping. Bukti ini mencakup database dan CLI untuk domain inti. Pengujian autentikasi dan akses melalui runtime API serta rehearsal staging-v2 masih menjadi tahap tersendiri. Receipt review ada di `.qa-artifacts/go-core-import-review-20261122.json`.

M15 CP01–05 tetap memiliki bukti sintetis. Jalur baru membaca snapshot aktual yang sudah diterima dan memetakan Class, User, Enrollment dan Role ke skema inti Go. Ini menambah penerapan untuk data nyata dengan tetap memakai skema inti yang sama. Jalur lab lama tetap membatasi targetnya pada fixture sintetis. [Dry run awal](2026-11-22-legacy-core-dry-run.md) · [Brief M15](../modules/M15.md)

| Hasil dry run Go | Jumlah |
|---|---:|
| Record sumber terhitung dari 21 tabel | 7.843 |
| Kandidat inti termasuk metadata program/batch | 266 |
| Enrollment ditahan | 1 |
| Menunggu mapping domain lain | 7.576 |
| User historis, termasuk 5 suspended | 132 |
| Class draft, termasuk 9 tanpa mentor | 17 |
| Participation dan episode awal | 118 |
| Role Teacher dan Student | 8 dan 110 |

Pembaca memverifikasi ukuran dan SHA-256 snapshot. SQL dibaca sebagai data COPY. ID target deterministik dari namespace dan ID sumber. Class dengan nama awal yang berulang mendapat akhiran stabil. Importer menyimpan keputusan untuk seluruh 267 record inti. Tanggal enrollment lama dicatat sebagai provenance. Semua episode tahap rehearsal memakai waktu pengambilan snapshot sebagai waktu observasi. Tanggal tersebut tidak membuktikan riwayat mulai Class. Status suspended dipertahankan dan akun hasil impor belum mempunyai password, tautan provider, sesi atau hak Site Admin. Ini menyisakan aktivasi akun, email terverifikasi serta penautan credential untuk [M04](../AUTHENTICATION_V3.md).

## Bukti yang tersedia

- Dry run Go aktual lulus dua kali dengan output identik. Jumlah per tabel, kandidat, hold, deferred dan Teacher cocok dengan profil sebelumnya. Hash input asli cocok.
- `go test ./...`, dua tes Go bertag `core_rehearsal`, `go vet` untuk importer, dan **49/49** tes regresi operator Python lulus.
- Run awal dari Codex berhenti pada `BLOCKED_BEFORE_IMPORT / PermissionError` saat menulis direktori privat. Run terminal pemilik kemudian menyelesaikan keempat tahap PostgreSQL.
- Bukti agregat di `.qa-artifacts/go-core-import-dry-run-20261122.json`. Go hanya mengeluarkan jumlah, status, hash plan dan kode error. Log dan snapshot privat tetap di luar Git.

## Prosedur pembuktian PostgreSQL lokal

Run yang diterima di atas sudah memakai prosedur ini, sehingga tidak perlu menjalankannya lagi untuk snapshot dan plan yang sama. Runner dijalankan dari PowerShell pengguna pemilik folder privat dengan PostgreSQL 18 dan Go.

```powershell
D:\Projects\lms-v2\backend-v3\backend-go\qa\RUN_GO_CORE_IMPORT_QA.cmd
```

Runner membuat PostgreSQL loopback baru dengan akun khusus. Target tetap `legacy_core_v3`, disimpan privat dalam `backend-v3/private-backup/<run>-go-core-rehearsal-*/`. Target kosong dicadangkan dalam format custom dan SQL, lalu custom dipulihkan ke DB pembanding kosong sebelum penulisan migrasi. Go memverifikasi marker, role, direktori cluster, port, versi dan hash backup sebelum menerima target.

Urutan bukti wajib adalah dry run, transaksi yang di-rollback dan kembali kosong, impor atomik dan rekonsiliasi seluruh field yang dipetakan, lalu replay dengan plan identik tanpa menambah data. Bukti akhir `PASS_GO_CORE_LOCAL_REHEARSAL_ONLY` memerlukan semua tahap tersebut dan penghentian cluster terverifikasi. Runner menyimpan cluster, backup dan receipt privat untuk review. Snapshot `legacy_frozen` tetap menjadi baseline pembanding.

Rehearsal ini menguji PostgreSQL dan CLI. Pengujian API untuk akun nyata, lifecycle OAuth/password dan wewenang lintas Class di runtime bersama mengikuti M04/M05. Rehearsal M16 di target Supabase dicatat tersendiri setelah cakupan dan lingkungan siap.
