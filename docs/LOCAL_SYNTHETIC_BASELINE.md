# Baseline database lokal sintetis sebelum data live tersedia

**Keputusan kerja 24 September 2026:** lanjutkan backend Go dan QA dengan PostgreSQL lokal sintetis. Ini adalah baseline **perilaku dan struktur v3 sementara**, bukan snapshot live, hasil migrasi nyata, atau bukti kesetaraan data production. Target operasional v3 tetap database staging yang sudah ada setelah gate M00/M01/M02/M15/M16 terpenuhi; LMS live tetap memakai database live.

## Yang sudah tersedia

`qa/postgres_lab.py` membuat cluster PostgreSQL baru pada loopback `127.0.0.1` dengan marker fixture, namespace v3 terpisah, role runtime terbatas, dan database `qa_cp05`. Runner tidak menerima DSN, env file, atau cluster existing. Pada journey yang sudah dijalankan, ia membuat SQL backup dan custom dump, membuktikan restore ke database terisolasi sebelum tiap schema installment, menjalankan Go/DB dan Playwright API, lalu memeriksa persistence dan privilege. Cluster, rows, dump, dan riwayat run dipertahankan; proses server dihentikan. Contoh bukti terbaru: [M10-CP03](checkpoints/2026-09-22-m10-cp03.md).

Fixture saat ini membuktikan irisan Class/participation, file, content, submission, grade, completion, attendance meeting, logbook, notification, dan migration shadow. **Belum ada bukti bahwa seluruh variasi, cardinality, nilai NULL, object bytes, kalender, atau kombinasi relasi data live telah dimodelkan.** Angka 6/6 kasus CP bukan cakupan seluruh tabel atau seluruh pengguna.

| Kelompok data | Slice fixture yang ada | Paritas live yang belum dibuktikan |
|---|---|---|
| Identity, Class, participation, mentor/group | M04–M05 lokal | Akun/role deployed, episode lama, duplikasi dan ownership nyata |
| File, content, submission | M06–M08 lokal | Bytes objek, reference lama, chronology dan orphan nyata |
| Grade, completion, transcript/credential | M09–M10 lokal | Rule akademik, nilai raw vs observed, legacy release/issue history |
| Attendance, permit, logbook | M11 meeting saja; M12 CP01–03 | Permit/SP, kalender historis, schedule Batch→Class |
| Jobs/notification dan migration | M13 CP01–02, M15 CP01–05 lokal | Provider nyata, outbound policy, source adapter, mapping dan counts live |
| Reporting/API/browser | M14 belum mulai; frontend di tim terpisah | Kontrak shared `cmd/api`, browser UX dan workload nyata |

Fixture sekarang dibuat per journey QA; **belum ada satu database mock lengkap yang siap dipakai bersama tim frontend**. Jika dibutuhkan untuk integrasi, buat target lokal terpisah yang di-seed secara deterministik dari fixture yang direview, tanpa menghubungkan ke staging/live dan tanpa mengubah denominator CP/D hanya karena seed tersedia.

## Cara memperluas mock dengan aman

1. Definisikan skenario berdasarkan invariant domain: multi-Class dan multi-role; episode retake/transfer; enrollment berakhir; revision content/submission; nilai missing/NULL/0/65/75; correction dan release; completion berbeda dari grade; credential private/revoked; file hilang/quarantine; notifikasi suppressed; waktu batas/deadline; konflik dan replay.
2. Tambahkan fixture deterministik, anonim, kecil dan dapat diulang pada harness lokal. Catat pemilik skenario, versi fixture, tabel/relasi yang disentuh, expected state, serta case Go/DB dan Playwright yang mengujinya. Jangan menyebut fixture lengkap hanya karena schema terpasang.
3. Untuk setiap schema/data installment, ambil backup SQL+custom dan uji actual isolated restore terlebih dahulu. Pakai role aplikasi minimum tanpa DELETE/DDL; pisahkan role migrasi. Simpan hash, source fingerprint, run ID dan reconciliation. Pertahankan raw run gagal.
4. Default seluruh integrasi outbound adalah rehearsal/disabled; tidak ada alamat penerima atau secret nyata di fixture, log, Git, atau screenshot. Simpan artefak lokal privat di luar Git.

## Ketika baseline live tersedia nanti

Sebelum membaca atau mengimpor data: M00 harus membuktikan source production, target staging, deployed SHA dan perbedaan identitas keduanya; M01 harus membuktikan backup source/target dan restore, termasuk object/file scope; M02 harus menilai pengamanan legacy/staging; owner data menyetujui tujuan, akses, retensi dan redaksi. Akses source pertama bersifat read-only. Jangan menjadikan `.env` staging sebagai production hanya berdasarkan nama file.

Mulailah dari metadata schema dan agregat non-PII. Untuk sampel baris, utamakan ekspor representatif yang dianonimkan dengan mapping stabil agar FK dan chronology tetap dapat dibandingkan. Jika dump mentah lengkap benar-benar diperlukan, tempat penyimpanan, enkripsi, ACL, masa retensi, dan orang yang boleh mengaksesnya harus diputuskan sebelum download; laptop developer atau Git bukan tempat default untuk data murid mentah. Simpan source backup, target backup dan artefak percobaan secara terpisah, semuanya dengan manifest/hash dan uji restore.

Lalu bandingkan inventaris schema, constraints, FK/index, cardinality, null/zero/default, state dan chronology, zona waktu, file references/bytes, grade raw-vs-observed, session/identity, serta orphan/ambiguity dengan fixture lokal. Catat setiap mismatch sebagai mapping decision atau quarantine dengan provenance; jangan mengubah data agar tampak cocok. Snapshot live masuk private landing yang terpisah, bukan restore menimpa `public` staging atau fixture lokal. Import rehearsal M16 baru boleh dilakukan setelah gate dan otorisasi tersendiri; historis tidak mengirim notifikasi atau menerbitkan credential.

**Status sekarang:** baseline lokal sintetis tersedia; completeness matrix terhadap live **NOT_VERIFIED**; download/copy live **NOT_EXECUTED**; backup production **NOT_EXECUTED**.
