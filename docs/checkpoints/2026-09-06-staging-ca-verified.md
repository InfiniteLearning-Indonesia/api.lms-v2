# Checkpoint — CA staging terverifikasi, 6 September 2026 WIB

Kelanjutan setelah pengguna menyediakan `backend-v3/prod-ca-2021.crt`. **Inspeksi metadata staging berhasil; backup, copy dan migrasi belum dilakukan.** Hasil ini menggantikan blocker TLS pada [checkpoint sebelumnya](2026-09-06-staging-inspection.md), bukan meluluskan M00/M03 secara keseluruhan.

## Perubahan lokal dan batas operasi

- Ditambahkan `DB_SSL_ROOT_CERT=../prod-ca-2021.crt` pada private `backend-go/.env.staging`; credential dan konfigurasi lain tidak diubah. Path relatif terhadap file env.
- Sertifikat PEM berisi CA `Supabase Root 2021 CA`, tanpa private key; berlaku 28 April 2021–26 April 2031 UTC. SHA256 **file CA**: `700723581420dd1ac98fd7e9ac529f0ef210eadcaf87fc868a3ad7d114c2f3b7`.
- Trust hanya ditambahkan in-memory oleh Go inspector. Tidak mengubah trust store Windows, hostname, atau SSL enforcement server; tidak menggunakan certificate bypass.
- Probe memakai `cmd/db-inspect` melalui `qa/inspect_staging.py`: koneksi tunggal, TLS chain/hostname verification, transaksi `REPEATABLE READ READ ONLY`, static catalog SELECT, timeout, lalu `ROLLBACK` dan close. Tidak membaca baris murid/session atau nilai secret.
- Tidak ada perubahan source Go/Python/QA manifests, database grants/RLS, DML/DDL, storage, production, frontend, backup/export/import/restore, deployment, commit atau push.

## Evidence operasi nyata

| Field | Hasil |
|---|---|
| Successful run | `20260906T041532Z-staging-inspect-c6957a44c8b0` |
| Waktu observasi | `2026-09-06T04:15:34.4408166Z` / 11:15 WIB |
| Status / exit | `READ_ONLY_OBSERVATION_COMPLETE` / `0` |
| TLS / transaksi read-only | Keduanya `true` |
| Target | `.env.staging`, user-identified staging; bukan independently verified production/staging pair |
| PostgreSQL server version | `170006` (17.6) |
| Public tables | 20 regular tables |
| Catalog SHA256 | `4136f7d2b916f502902510e60bbe868b0460cf0f97db11ebade20aab6838791b` |
| Executable source SHA256 | `3babfd10b45359193dec90e0a551086fe1bb915b9d7d6e76c0489b03ff7d14ad` |
| CLI binary SHA256 | `376450505fc13857da56af827633a25a673edb412acaa7f72fc6c5cadff3dc02` |
| Leaf certificate SHA256 | `03709ca44d1b06e4504da0a83b6ca065761edc704cd5634bcc3894fd5c7b3248` |
| Database size observation | 11,611,283 bytes; bukan ukuran SQL backup, exact row count, atau bukti disk headroom |
| Backup / migration | `NOT_EXECUTED` / `NOT_EXECUTED` |

Evidence detail tetap lokal di `.qa-artifacts/20260906T041532Z-staging-inspect-c6957a44c8b0/summary.json`. Catalog hash mencakup public metadata dan effective current-role observations sesuai `internal/platform/dbinspect/catalog.go`; **bukan full database schema/data hash**. CA file hash dan leaf-certificate hash memang berbeda objek. Source hash diperiksa ulang dan sama dengan kedua QA checkpoint sebelumnya; private env/CA bukan bagian executable hash.

Percobaan sandbox sebelum izin network: `20260906T041122Z-staging-inspect-f7ef33aad8ba`, `CONNECTION_OR_QUERY_FAILED`, tanpa catalog atau transaksi read-only yang berhasil. Evidence gagal tetap disimpan. Percobaan berizin setelahnya berhasil tanpa perubahan kode atau pelonggaran TLS.

## Hasil catalog dan dampaknya

Tabel teramati: `assignments`, `attendances`, `audit_logs`, `batches`, `classes`, `competencies`, `competency_scores`, `enrollments`, `holidays`, `logbooks`, `materials`, `mentor_async_days`, `permission_requests`, `program_competencies`, `programs`, `rubrik_assessment_scores`, `rubrik_assessments`, `session`, `submissions`, `users`.

| Observasi terverifikasi | Arti untuk pekerjaan v3 |
|---|---|
| `public.batches.logbookSchedule` ada, nullable `jsonb` | Sesuai tambahan di staging `backend/src/classes/entities/batch.entity.ts`. Belum membuktikan live DB memiliki/tidak memiliki kolom ini; perbandingan kedua DB tetap pending. |
| 20 tabel public vs 19 deklarasi ORM di baseline source | Tabel `session` juga harus masuk penentuan scope backup/identity bridge; jangan mengandalkan daftar entity ORM saja. Nama/struktur session teramati, isi session tidak dibaca dan tidak disalin. |
| Akun koneksi memiliki `BYPASSRLS`, `CREATEDB`, `CREATEROLE`, serta CREATE pada public; bukan superuser | Credential ini bukan kandidat akun inspeksi/runtime least-privilege. Transaksi read-only membatasi operasi probe ini, tidak mencabut hak credential. |
| Akun menjadi owner dan memiliki SELECT/INSERT/UPDATE/DELETE/TRUNCATE pada semua 20 tabel | Pisahkan operator read-only, runtime dan migration owner; jangan memakai credential ini langsung untuk runtime Go. Perubahan grant harus direview sebagai pekerjaan operasional terpisah. |
| Seluruh 20 tabel public memiliki RLS=false, FORCE RLS=false dan tanpa policy | Belum ada proteksi row-level di tabel-tabel ini. **Bukan bukti anonymous/public API exposure**: grants `anon`/`authenticated`, API exposure configuration dan jalur akses aplikasi belum diperiksa oleh probe. Jangan enable RLS massal tanpa compatibility review. |
| 27 foreign keys teramati, seluruhnya validated: 12 CASCADE, 4 SET NULL, 11 NO ACTION pada DELETE | Delete induk berpotensi merambat. Aturan no-delete memerlukan kontrol privilege/API dan pengujian relasi; tidak menghapus atau mengubah FK existing dalam sesi ini. Validated FK bukan bukti semua business invariants terpenuhi. |
| Statistik planner pada sebagian besar tabel adalah `estimated_rows=-1` | Artinya estimasi belum tersedia, **bukan tabel kosong atau jumlah negatif aktual**. Tidak dilakukan COUNT/full-row scan. |

CASCADE teramati pada `competency_scores`→`competencies/users`, `enrollments`→`classes/users`, `logbooks`→`users/batches`, `mentor_async_days`→`users`, `rubrik_assessment_scores`→`users/rubrik_assessments`, `rubrik_assessments`→`program_competencies`, dan `submissions`→`users/assignments`. SET NULL teramati pada `classes`→`users`, `competencies`→`program_competencies/users`, dan `rubrik_assessments`→`users`. Ini metadata FK, bukan pengujian penghapusan.

## Capaian dan gate berikutnya

- S03 pada sesi sebelumnya sekarang selesai **sebatas public catalog/privilege observation**; query success path benar-benar dijalankan terhadap PostgreSQL staging. Inventory semua namespace/jobs/triggers/functions/storage dan sign-off belum selesai.
- M00-D02 masih partial: TLS staging terverifikasi, tetapi production distinctness dan least-privilege role belum. M00-D03 partial: public catalog ada, full dependency inventory belum.
- M03-D02 masih partial: keberhasilan operator single-connection tidak membuktikan runtime pool, namespace isolation, exhaustion handling atau migration safety.
- Canonical coverage tetap M00 **0/5**, M03 **0/5**, M00–M18 **0/97**. Tidak mengubah acceptance denominator atau menaikkan numerator dari satu probe.
- Synthetic QA tidak dijalankan ulang karena hanya env privat dan dokumentasi berubah; executable hash tetap cocok. Evidence terakhir: M00 **16/16** checks, M03 **19/19** checks, **13 Go cases**, instrumented Go statement coverage **67,4%**. Probe subprocess tidak menambah instrumented coverage dan bukan Playwright suite baru. Remote CI/race belum verified.

Urutan berikutnya:

1. Konfirmasi reference deployment/secret production yang benar dan identitas dua target berbeda; jangan menganggap `backend-live/.env` otomatis production karena fingerprint local configs masih sama.
2. Sepakati/provision akun operator khusus read-only dengan batas scope, tanpa owner/BYPASSRLS/DDL/write. Jangan mengubah credential atau privilege aplikasi legacy secara mendadak. Review grants/API exposure untuk menutup penilaian risiko public/RLS.
3. Lengkapi inventory jobs/triggers/functions/namespace/storage, kapasitas lokal/target dan owner/reviewer. Database size bukan bukti backup akan muat atau semua object bytes terlindungi.
4. Jalankan M01 setelah scope aman: backup source dan target ke lokasi lokal terproteksi/encrypted, checksum, serta restore proof pada target baru terisolasi. Tidak `--clean`, drop/truncate, overwrite staging, atau copy session/token/provider secret secara buta. Backup wajib sebelum setiap migrasi.
5. Pekerjaan Go lokal yang independen dapat dilanjutkan tanpa menyentuh DB; actual DB mutation/migration tetap menunggu gate. Live tetap berjalan dengan sistem/database live.

Skill Supabase/Postgres memandu penggunaan CA eksplisit dengan hostname verification dan pemisahan transaksi read-only dari privilege role. Verifikasi TLS mengikuti [panduan SSL Supabase](https://supabase.com/docs/guides/platform/ssl-enforcement); tidak diperlukan perubahan SSL enforcement server.
