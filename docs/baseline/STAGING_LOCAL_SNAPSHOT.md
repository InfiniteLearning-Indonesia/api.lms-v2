# Snapshot staging legacy ke lokal

> **18 November — next restore step prepared:** [New local restore runbook](LOCAL_LEGACY_RESTORE.md), [evidence](../checkpoints/2026-11-18-local-legacy-restore-prepared.md). Snapshot/COPY review identifies 7,843 rows and UUID dependency. 34 offline tests PASS; native restore still awaits Windows owner terminal, not a new source export. Use `RESTORE_STAGING_BASELINE_LOCAL.cmd`; retain the accepted snapshot.

> **18 November — accepted export:** [Owner run + independent review](../checkpoints/2026-11-18-staging-snapshot-accepted.md), run `20261118T131839Z-staging-baseline-3fe3bf70b4d4`: **PASS_REVIEWED_PUBLIC_SNAPSHOT_EXPORT_ONLY**. Custom/SQL and both encrypted copies have matching hashes; both decrypt/authenticate against originals; actual archive TOC covers 21 tables. Artifact directory ~5.1 MiB. **Actual restore NOT_RUN**, independent key custody not established, non-public/R2 excluded. Preserve this baseline; no need to repeat the export now. Earlier NOT_TAKEN/NOT_EXECUTED notes describe preceding attempts.

> **18 November — client TLS fix:** Owner run `20261118T131356Z-staging-baseline-9dcc44508b30`
> stopped before export because the original probe incorrectly required backend
> `pg_stat_ssl=true`. The configured endpoint is a Supabase pooler: actual same-
> connection client evidence shows **TLSv1.3 + verify-full + read-only PASS**,
> while backend TLS observation is false. The runner now checks libpq transport
> through PostgreSQL 18 `\conninfo` and preserves backend TLS as separate metadata.
> **19/19 offline regressions PASS; corrected actual metadata preflight PASS.**
> Dump/restore remain NOT_EXECUTED. [Evidence and limits](../checkpoints/2026-11-18-staging-client-tls-fix.md).

18 November 2026. Scope: pengambilan baseline aplikasi untuk analisis/migrasi lokal;
bukan aktivasi backend Go ke staging, import v3, atau cutover. User mengotorisasi
pemeriksaan baca-saja dan pengambilan snapshot memakai `.env.staging` yang ada.
Tidak ada izin membuat role/grant, mengubah schema/data sumber, mengubah R2,
menghapus data, commit, atau push dalam pekerjaan ini.

## Hasil pemeriksaan aktual

- Inspector Go yang ada berhasil tersambung ke sumber pilihan user dengan CA
  `backend-v3/prod-ca-2021.crt`: TLS chain/hostname terverifikasi, transaksi
  repeatable-read/read-only terverifikasi, ditutup dengan rollback.
- PostgreSQL sumber `170006` (17.6); ukuran observasi `15,232,147` bytes,
  sekitar 14.5 MiB; 21 tabel aplikasi `public`, seluruhnya dapat dibaca role ini.
  Ini observasi metadata, bukan dump atau row-count/hash parity.
- Credential **bukan least-privilege**: bukan superuser tetapi dapat bypass RLS,
  membuat database/role/schema. Credential tidak diubah; tidak ada role/grant
  yang dibuat. Permintaan user memakai credential existing dibatasi menjadi
  one-off read-only acquisition. Jangan menyebut role ini dedicated read-only,
  meneruskannya ke runtime v3, atau menganggap gate privilege sudah lulus.
- Identitas **staging** berasal dari deklarasi user; keterpisahan production,
  waktu sinkronisasi, dan deployed legacy SHA belum diverifikasi independen.
- Percobaan pertama memakai system trust saja gagal verifikasi sertifikat;
  percobaan memakai CA existing lulus. Tidak ada `sslmode=disable`,
  `rejectUnauthorized=false`, perubahan `.env`, atau pelonggaran TLS/ACL.
- File privat baru tidak dapat ditulis dari token proses Codex: `PermissionError`.
  Runner berhenti sebelum membaca credential/membuka koneksi/mengambil dump.
  Folder kosong/parsial dari percobaan tetap dipertahankan.

**Status: READ_ONLY_METADATA_OBSERVED; SNAPSHOT_NOT_TAKEN.** Metadata inspection
tidak membuktikan file dump dapat direstore. Tidak ada CP/module/DLV-06 yang ditutup.

## Satu perintah di terminal Windows biasa

Jalankan sebagai pengguna Windows pemilik folder, di luar proses Codex yang
dibatasi; tidak perlu Run as Administrator atau mengubah izin folder existing:

```powershell
D:\Projects\lms-v2\backend-v3\backend-go\qa\FREEZE_STAGING_BASELINE.cmd
```

Launcher memanggil [operator Python](../../qa/operators/freeze_staging_baseline.py).
Default client `C:\Program Files\PostgreSQL\18\bin`; jika berbeda, jalankan
modul dari root Go dengan `--pg-bin` eksplisit. Tidak ada parameter DSN, env-file,
schema bebas, restore target, import, atau mode write. Tidak dijalankan oleh CI.

Urutan:

1. Buat dua folder **baru** dengan ACL pemilik/SYSTEM dan uji tulis, sebelum
   credential dibaca. Artifact dan key dipisahkan ke folder bersaudara.
   Izin existing tidak diperbaiki otomatis; kegagalan berarti STOP.
2. Verifikasi hash binary age v1.3.2 terhadap cache install terverifikasi,
   client PostgreSQL 18, CA existing, dan config DB literal. Tidak mengunduh tool,
   mengevaluasi shell/interpolasi dotenv, atau memuat secret R2/SMTP.
3. Session libpq `verify-full`, TLS minimal 1.2, GSS encryption alternative
   disabled, default read-only, connect/statement/lock timeouts. Probe transaksi
   repeatable-read/read-only, versi, bytes, akses tabel, dan FK lintas schema;
   `\conninfo` pada koneksi client yang sama membuktikan TLS laptop ke endpoint.
   `pg_stat_ssl` dicatat sebagai hop sisi backend, bukan pengganti bukti client.
   Override libpq inherited diganti; proxy tetap dipertahankan. Metadata privat
   disimpan sebelum validasi lanjutan agar kegagalan dapat didiagnosis.
4. Ambil **satu** `pg_dump --format=custom --schema=public`, tanpa owner/ACL/
   tablespace restore options, ke file baru privat. Perintah tidak memiliki
   `--clean`, tidak membuat/mengubah database sumber, dan tidak menyalakan aplikasi.
5. Periksa TOC seluruh table-data entries terhadap inventory preflight.
   Turunkan SQL dari archive yang sama lewat `pg_restore --file=-`;
   **SQL tidak dieksekusi**. Inventory preflight bukan exported/shared snapshot;
   record waktu awal/akhir export, jangan bandingkan dengan live yang bergerak.
6. Enkripsi custom+SQL memakai age, autentikasi/dekripsi di memori, dan cocokkan
   bytes/SHA-256 dengan payload asli. Stdout/log tidak memuat payload, key,
   password, DSN, atau diagnostic koneksi mentah.
7. Simpan manifest privat dengan hash/bytes/scope/status. Tidak ada restore,
   cluster baru, migrasi, cleanup, upload R2, atau Git operation.

## Scope dan keterbatasan yang disengaja

- **Public-schema application baseline**, bukan seluruh project Supabase.
  `auth`, `storage`, `realtime`, `vault`, schema non-public lain, global roles,
  tablespaces, konfigurasi service, dan byte R2 **tidak disalin**. Presence metadata
  schema tersebut sudah tercatat inspector. Dependency/usage perlu ditinjau
  sebelum menyatakan backup LMS lengkap; FK lintas schema menghentikan runner.
- Seluruh `public` termasuk tabel `session` ikut dipertahankan privat. Session
  legacy tidak boleh diaktifkan sebagai sesi login v3. Tidak ada filtering data
  diam-diam, tetapi tidak ada klaim migrasi seluruh domain selesai.
- Custom+SQL adalah satu baseline aplikasi. Sequences bukan state MVCC setara
  row snapshot; pemetaan dan restore perlu memeriksa sequence secara eksplisit.
- Extensions sumber meliputi `pgcrypto`, `uuid-ossp`, `supabase_vault`,
  `pg_stat_statements`, `plpgsql`; schema-filtered dump bukan jaminan dependency
  extension/function/provider lengkap. Review archive/SQL sebelum restore ke
  cluster lokal **baru** yang terisolasi. Jangan restore dump langsung ke DB lama.
- Key baru di folder `<run>-keys`; artifact di `<run>`. **Satu disk bukan custody
  independen.** Owner perlu membuat salinan key terpisah yang aman sebelum
  mengandalkan archive sebagai backup pemulihan. Jangan kirim key/dump ke chat/Git.
- Plaintext custom+SQL tetap disimpan di folder ACL privat; terenkripsi juga
  tersedia. Tidak ada penghapusan otomatis, termasuk saat gagal. Bila proses
  terputus, partial artifacts mungkin berisi data: perlakukan semuanya privat.
- Ruang kosong pada pemeriksaan awal sekitar 14 GiB. Admission: ukuran DB
  maksimum 256 MiB, free floor 4 GiB ditambah reserve sedikitnya 1 GiB atau
  16x ukuran DB; file maksimum 256 MiB, artifact run maksimum 512 MiB. Monitoring
  bukan kuota OS: proses cepat dapat menulis sebelum pemeriksaan berikutnya.
  Tidak ada full QA journey atau clone cluster berulang untuk mengambil snapshot.

## Membaca hasil

`PASS_PUBLIC_SNAPSHOT_EXPORT_ONLY` berarti dump aplikasi + SQL dari archive sama
tersedia, TOC cocok, enkripsi terautentikasi dan roundtrip hash cocok. **Bukan**
restore proof, full project backup, snapshot R2, kesiapan backend v3, atau cutover.

`BLOCKED_BEFORE_EXPORT`: preflight/private directory/tool/input gagal.
`FAILED_OR_INCOMPLETE_EXPORT`: dump sudah dicoba; mungkin ada partial data.
Jika `private_summary_written=false`, path report yang ditampilkan belum berisi
summary; gunakan console status, jangan mengasumsikan file exists atau PASS.
Jangan mengulang run sukses tanpa kebutuhan baseline baru; kembalikan status dan
run ID agar scope/manifest diperiksa sebelum restore lokal.

Setelah export lulus: review archive/dependencies dan isolasi restore, pulihkan
ke database legacy lokal baru, verifikasi data/relations/sequences, baru buat
target v3 lokal terpisah. Staging-v2 Supabase menyusul setelah mapping/reconciliation.

Referensi: [pg_dump](https://www.postgresql.org/docs/18/app-pgdump.html),
[pg_restore](https://www.postgresql.org/docs/18/app-pgrestore.html),
[migrasi Supabase](https://supabase.com/docs/guides/platform/migrating-within-supabase/backup-restore),
[kontrak intake](REAL_BASELINE_INTAKE.md), [runtime staging](STAGING_CONFIGURATION_READINESS.md).
