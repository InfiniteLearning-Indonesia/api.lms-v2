# Restore baseline aplikasi nyata ke lokal baru

> **19 November — accepted native restore:** [Evidence/review](../checkpoints/2026-11-19-local-legacy-restore-accepted.md), run `20261119T093357Z-legacy-local-restore-debaab74a513`: **11/11 native cases PASS; actual_restore_verified=true**, 21 tables/7,843 rows. Independent artifact/receipt/control-state review PASS; cluster retained **stopped**, ~77.3 MiB measured. **Do not rerun the successful restore.** Next read-only real mapping, then a separate local v3 target. Full modules/DLV-06/runtime/R2/cutover remain open; older NOT_RUN/failure notes below are history.

> **19 November — latest identity format fix:** [Evidence](../checkpoints/2026-11-19-local-identity-format-fix.md). Startup/stop now work; owner run `20261119T080846Z-legacy-local-restore-bf1ed15f0f91` rejected identity before data restore and stopped cleanly. Fixed an inet::text `/32` versus bare-host comparison bug; exact IPv4 host/mask, path, port and PG18 guards retained, with per-call private identity evidence. **49/49 offline tests PASS; post-fix native identity/restore NOT_RUN.** Same updated owner command; no marker deletion or interrupt required for the stopped run.

> **19 November — latest prior-cluster guard:** [Evidence](../checkpoints/2026-11-19-prior-cluster-guard-fix.md). An old unclean startup marker timed out under the socket-only check. Read-only Windows process/listener tables now independently prove no PID/listener while preserving the directory and marking clean shutdown unverified. **43/43 offline tests + actual negative and synthetic positive kernel checks PASS.** Current prior guard passes; use the same updated owner launcher. Actual 11-case DB restore still NOT_VERIFIED.

> **19 November — latest startup supervision fix:** [Incident/evidence](../checkpoints/2026-11-19-local-restore-startup-fix.md). A local server became ready but old pipe capture blocked the wrapper before provisioning/data restore. Fixed pg_ctl regular-file capture and prior-live-cluster guard: **38 offline tests + actual synthetic process-I/O reproduction PASS**. Actual DB restore NOT_VERIFIED. If the old terminal is still stuck, Ctrl+C once, wait for its prompt/cleanup, then use the same updated command. Do not start duplicate runs, delete PID markers or treat an old ready log as restore completion.

18 November 2026. User mengotorisasi review snapshot, restore lokal terisolasi,
dan verifikasi data. Ini tooling operator Python; backend aplikasi tetap Go.
Tidak ada koneksi staging/production, pembacaan dotenv, R2, deployment, cutover,
penghapusan data/file, commit atau push. Inputnya hanya snapshot aplikasi yang
sudah [diterima](../checkpoints/2026-11-18-staging-snapshot-accepted.md).

## Status sebelum eksekusi owner

- Review **seluruh schema hasil archive** dan seluruh blok COPY lokal lulus.
  Scope: **21 tabel, 7.843 record, 4 enum, 52 constraint termasuk 27 FK,
  satu index tambahan session, nol sequence**. Isi record tidak dicetak.
- Tidak ada function/procedure/trigger/view/foreign table/subscription/job yang
  direstore dari archive ini. Statement schema di luar daftar yang direview,
  default function lain, atau input berbeda dari hash snapshot ditolak.
- Dependency aktual: `extensions.uuid_generate_v4()` memerlukan `uuid-ossp`
  di schema `extensions`. Control file tersedia pada PostgreSQL18 lokal;
  pembuktian provisioning native masih menunggu owner run.
- Dua entri TOC `SCHEMA public` dan komentarnya dilewati saat restore karena
  target baru sudah memiliki public schema. **Tidak ada table/data entry yang
  dilewati, schema lama yang dihapus, atau file backup yang diubah.** Perubahan
  hanya pada selection list baru untuk target lokal.
- Offline regressions: **34/34 PASS** (19 exporter + 15 restore).
  This is the initial preparation result; latest startup fix adds four cases,
  making **38/38 offline PASS** (19 exporter + 19 restore).
- Actual operator attempt dari Codex: `BLOCKED_BEFORE_RESTORE / PermissionError`,
  sebelum dekripsi input/cluster/initdb. Private folder Windows tidak writable
  oleh token proses ini. **Actual restore NOT_RUN; 0/11 native cases.**

## Perintah satu kali

Jalankan di terminal Windows biasa sebagai pemilik folder. Tidak perlu
Administrator, perbaikan ACL existing, install dependency, atau ekspor ulang:

```powershell
D:\Projects\lms-v2\backend-v3\backend-go\qa\RESTORE_STAGING_BASELINE_LOCAL.cmd
```

Kode: [operator](../../qa/operators/restore_legacy_baseline.py).
Scope/cases: [manifest](../../qa/manifests/REAL-LEGACY-LOCAL-RESTORE.json).
Manifest scope dan exact cases divalidasi saat startup sebelum input data dibaca.
Script hanya menerima acknowledgement new-local dan lokasi binary PG18;
tidak menerima host/DSN/database existing, env file, target shared, atau apply v3.

## Tahap dan bukti yang wajib

1. Buat **folder baru** owner/SYSTEM-only dan uji tulis sebelum membaca data/key.
   Kegagalan berarti STOP; permission tidak dilonggarkan. Root:
   `backend-v3/private-backup/<UTC>-legacy-local-restore-<id>/`.
2. Verifikasi empat ukuran/hash input yang dipin di kode terhadap snapshot
   `20261118T131839Z-staging-baseline-3fe3bf70b4d4`. Cached age binary diverifikasi;
   tidak ada unduhan. Dekripsi/authenticate custom+SQL dari encrypted backups,
   cocokkan plaintext hash, simpan ke subfolder recovered privat.
3. Review schema dengan lexer terbatas dan allowlist; pin DDL hash yang sudah
   direview. Cocokkan seluruh tabel/urutan kolom COPY, 7.843 record, dependency,
   dan metadata TOC. COPY reference menyimpan jumlah dan hash **multiset baris**
   tiap tabel, sehingga urutan fisik boleh berbeda tetapi duplikat tetap dihitung.
4. Buat PostgreSQL18 cluster **baru** dengan SCRAM/random credentials, bind hanya
   `127.0.0.1` di port baru, tanpa preload provider/job/application. Verifikasi
   actual data_directory, address/port dan versi sebelum provisioning. Service
   PostgreSQL yang sudah terpasang tidak dihentikan/diubah. TLS off berlaku hanya
   untuk loopback cluster baru ini, tidak mengubah TLS staging atau aplikasi.
5. Buat role owner/reader terbatas dan dua database baru `legacy_frozen` dan
   `empty_proof`. Sebelum dependency/schema/data restore: backup target kosong
   ke custom+SQL, restore custom ke proof DB, cocokkan empty public catalog
   (relations/types/constraints/comment). Ini bukan backup global roles/cluster.
6. Provision hanya `uuid-ossp` lokal di schema `extensions`; grant schema usage
   pada owner/reader dan default SELECT tables kepada reader, tanpa public grants.
   Tidak meniru seluruh platform Supabase, Vault/Auth/Storage/cron.
7. Restore **custom archive** secara single transaction, exit-on-error,
   no-owner/no-privileges/no-tablespaces, memakai role owner non-superuser,
   fixed local `--dbname=legacy_frozen`. Tidak ada `--clean`, `--create`, DROP,
   DELETE, TRUNCATE, overwrite atau source mutation. SQL backup dipakai sebagai
   pembanding COPY; full SQL restore kedua belum termasuk scope.
8. Dump schema hasil restore secara read-only dan cocokkan signature semua DDL,
   enum, kolom/default, constraints/FK/index terhadap sumber yang direview.
9. Baca seluruh tabel menggunakan reader, render COPY dengan UTF8/UTC/ISO dan
   extra_float_digits=3, cocokkan jumlah serta hash multiset terhadap snapshot.
   Periksa semua FK validated, nol sequence, reader tanpa hak DML/admin/bypass;
   set default_transaction_read_only reader dan buktikan pada koneksi baru.
10. Verifikasi ulang hash semua input asli; simpan koneksi reader **privat**,
    receipts/hashes/counts tanpa plaintext record/password di console/log publik.
11. Normal-stop cluster ini saja. Bukti `pg_controldata: shut down` dan tidak
    ada postmaster.pid diperlukan. Cluster, input/recovered files, prestate
    backups dan evidence dipertahankan. Tidak ada cleanup/retention disposal.

`pg_ctl` memakai stdout/stderr file privat, bukan PIPE, agar background server
Windows tidak menahan EOF supervisor. Startup/stop memiliki progress eksplisit.
Sebelum clone baru, recorded PID dan listener IPv4 milik run sebelumnya diperiksa
langsung via API Windows read-only: proses **atau** listener aktif menghentikan
runner. Hanya bila keduanya tidak ada, marker stale dipertahankan/ditandai dengan
clean shutdown tetap unverified. Enumeration failure tetap memerlukan review;
socket timeout bukan bukti tidak aktif. Tidak ada auto-kill atau marker removal.

Full PASS memerlukan exact **11/11** cases dan manifest privat yang dapat
ditulis. Partial/duplicate/missing/failed stop bukan PASS. `normal-stop` saat
kegagalan sebelumnya tidak mengubah run gagal menjadi lulus.

## Resource dan custody

- Admission free >=6 GiB, floor saat berjalan 4 GiB, logical run limit 1 GiB,
  command budget 15 menit dan stop terpisah. Budget diperiksa, **bukan kuota OS**;
  pertumbuhan bisa terjadi sebelum pemeriksaan berikutnya. Ini satu cluster
  kecil untuk baseline, bukan full synthetic QA journey berulang.
- Lock mencakup restore, stop, dan acceptance receipt. Receipt dengan source hash
  mencegah restore ulang baseline yang sama setelah sukses; jangan membuat clone
  identik tanpa kebutuhan. Jika gagal, private partial files/cluster tetap disimpan.
- Data ini nyata, termasuk password hashes/session history: simpan seluruh root
  privat dan di luar Git. Jangan jalankan legacy/Go API di atas clone ini.
  Tidak ada login session yang diaktifkan atau data yang dikirim ke provider.
- Source age identity tetap di folder `-keys` snapshot; credential lokal baru
  disimpan di root restore `/keys`. Key custody tetap satu disk dan belum
  independen. Tidak ada identitas Supabase/DB source yang disalin ke target.
- Target memakai engine PG18 dengan locale C dari fresh initdb; locale/collation
  production belum dicapture/diverifikasi. Text COPY parity bukan bukti seluruh
  perbedaan semantik engine/collation/performance sudah tertutup.
- Public schema saja; non-public platform schemas, byte R2, global config, actual
  provider access, baseline ke-v3 mapping, delta/cutover tetap di luar scope.
  Nol sequence adalah hasil snapshot ini, bukan proof recovery sequence nonzero.

## Membaca hasil

`PASS_REVIEWED_LOCAL_BASELINE_RESTORE_ONLY` + `actual_restore_verified=true`
berarti restore dan pemeriksaan snapshot ini lulus, cluster ditinggal berhenti.
Tidak berarti Go runtime cocok, seluruh LMS termigrasi, M01/M16 selesai,
staging-v2 siap, full project backup, R2 parity, atau production cutover.

`BLOCKED_BEFORE_RESTORE`: private/input/tool/review gate belum lolos.
`FAILED_OR_INCOMPLETE_LOCAL_RESTORE`: cluster sudah dicoba; lihat receipt.
Jika `stop_failure` terisi, jangan melanjutkan sampai status cluster itu diperiksa.
Jika `private_summary_written=false`, path report belum tentu berisi file.
`BASELINE_ALREADY_RESTORED_REVIEW_ACCEPTED_RECEIPT`: gunakan receipt yang ada;
jangan bypass marker untuk membuat cluster duplikat.

Setelah owner run sukses, kembalikan status/run ID untuk pemeriksaan evidence.
Langkah berikutnya adalah mapping nyata dan database **v3 lokal terpisah**, dengan
backup/provenance sebelum setiap migrasi. Jangan mengubah clone legacy pembanding.

Referensi: [pg_restore](https://www.postgresql.org/docs/18/app-pgrestore.html),
[initdb](https://www.postgresql.org/docs/18/app-initdb.html),
[uuid-ossp](https://www.postgresql.org/docs/18/uuid-ossp.html).
