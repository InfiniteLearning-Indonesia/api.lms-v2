# Dry run data legacy untuk Class, User, Enrollment, dan Role

22 November 2026. **Profil baca-saja selesai.** Semua 7.843 record dari 21 tabel terhitung. Sebanyak 267 record masuk analisis inti. Dari jumlah itu, 266 memenuhi pemeriksaan awal sebagai kandidat dan satu enrollment ditahan untuk review. Sebanyak 7.576 record menunggu pemetaan domain berikutnya. Kandidat menunjukkan data sumber cukup untuk membangun mapping awal. Impor dan aktivasi masih tahap berikutnya.

Analisis memakai arsip SQL dari snapshot staging `20261118T131839Z-staging-baseline-3fe3bf70b4d4` yang sudah dibuktikan lewat restore lokal 11/11 kasus. Hash SQL dan ledger isi 21 tabel diverifikasi ulang sebelum analisis. Seluruh pemeriksaan berjalan dari arsip lokal. [Penerimaan restore](2026-11-19-local-legacy-restore-accepted.md)

## Cakupan record

| Tabel inti | Sumber | Kandidat | Ditahan | Arti kandidat |
|---|---:|---:|---:|---|
| `programs` | 4 | 4 | 0 | Label dan asal program. |
| `batches` | 3 | 3 | 0 | Label cohort dan metadata historis. |
| `classes` | 17 | 17 | 0 | Class berdasarkan ID legacy. Penamaan, publikasi, dan teacher dinilai terpisah. |
| `users` | 132 | 132 | 0 | Identitas historis. Aktivasi login dan wewenang global dinilai terpisah. |
| `enrollments` | 111 | 110 | 1 | Partisipasi yang struktur dan bukti sumbernya cocok. Lifecycle episode tetap perlu aturan tanggal. |
| **Total inti** | **267** | **266** | **1** | Jumlah ini menghitung record sumber. |

Penugasan Teacher dan Student bersumber dari Class dan enrollment sehingga tidak dihitung dua kali dalam total record sumber. Tabel lain tetap tercatat sebagai menunggu pemetaan.

| Domain berikutnya | Tabel dan jumlah | Total |
|---|---|---:|
| Materi dan pekerjaan | `materials` 84, `assignments` 29, `submissions` 778 | 891 |
| Kurikulum dan hasil akademik | `competencies` 65, `program_competencies` 28, `competency_scores` 640, `rubrik_assessments` 55, `rubrik_assessment_scores` 0 | 788 |
| Absensi, kalender, dan izin | `attendances` 4.298, `holidays` 17, `permission_requests` 66 | 4.381 |
| Logbook dan jadwal mentor | `logbooks` 149, `mentor_async_days` 35 | 184 |
| Administrasi, audit, dan session | `program_heads` 3, `audit_logs` 1.329, `session` 0 | 1.332 |
| **Total berikutnya** | **16 tabel** | **7.576** |

## Temuan yang menentukan aturan impor

| Bukti pada snapshot | Rekomendasi untuk pembuktian lokal pertama |
|---|---|
| **17 Class** punya program/batch dan label yang bisa dipakai. Schema lama tidak punya kolom nama Class. Rumus nama `Program — Batch` mengulang nama pada **10 Class di 2 kelompok**. | Gunakan ID legacy untuk identitas dan URL. Untuk nama awal, gunakan label program/cohort dan tambahkan pembeda stabil pada nama berulang. Semua Class dipertahankan sebagai entitas terpisah. Nama tampilan bisa direview tanpa mengubah mapping ID. |
| **8 Class** punya mentor valid dan aktif dengan role mentor lama. **9 Class** tidak punya mentor. Ada **1 relasi** dengan orang yang sama tercatat sebagai mentor sekaligus enrolled di Class tersebut. | Jadikan `classes.mentorId` bukti penugasan Teacher khusus Class. Biarkan Class tanpa mentor tetap tersedia sebagai draft untuk review. Catat konflik Teacher–Student untuk keputusan sebelum mengaktifkan kombinasi role pada kelas itu. |
| **127 akun aktif, 5 suspended**. Tidak ditemukan tabrakan email setelah trim/casefold atau tabrakan Google subject yang terisi. | Jaga mapping satu identitas per ID legacy. Akun suspended tetap tercatat dan aksesnya tetap tertahan saat pengujian kebijakan. Provisioning login dan Site Admin mengikuti aturan terpisah. |
| Role global lama memuat **3 admin, 10 facilitator, 9 mentor, 110 student**. **3 program head** memiliki referensi user dan program yang ditemukan. | Gunakan relasi eksplisit untuk role kontekstual. Tahan pemberian Site Admin dan akses facilitator/program head sampai cakupannya dipetakan. |
| **1 dari 111 enrollment** punya relasi Class dan user, tetapi akun tidak punya role Student legacy. Tidak ditemukan pasangan user–Class duplikat pada pemeriksaan ini. | Simpan enrollment tersebut dengan alasan hold. Putusan apakah relasi enrollment cukup sebagai bukti Student dibuat eksplisit sebelum pemberian akses. |
| **42 enrollment kandidat** punya `createdAt` sebelum waktu pembuatan Class yang tercatat sekarang. Backend lama bisa memindahkan `classId` pada enrollment yang sama. Bukti itu menunjukkan mekanismenya, sementara histori perpindahan setiap record belum diketahui. | Pertahankan waktu asli dalam provenance. Gunakan relasi Class saat snapshot sebagai keadaan yang teramati. Jika importer perlu tanggal episode untuk pembuktian, gunakan timestamp snapshot yang tetap sebagai waktu pengamatan dengan label asal, dan verifikasi kronologi sebelum migrasi histori akademik. |
| **5 enrollment kandidat** terkait akun nonaktif. | Pertahankan riwayat partisipasi dan uji penolakan akses akun suspended. Status akun terpisah dari lifecycle enrollment. |
| **8 Class** memiliki flag transcript release dan **9 Class** memiliki flag certificate release. Ketiga batch memiliki daftar program yang terisi. | Bawa flag dan relasi lama sebagai bukti asal. Penetapan publikasi Class, transcript, sertifikat dan relasi kurikulum masuk pemetaan domain terkait. |

Rujukan implementasi [relasi Class legacy](../../../../backend/src/classes/entities/class.entity.ts), [penulisan ulang classId pada enrollment](../../../../backend/src/classes/classes.service.ts#L1802), [skema Go untuk Class, episode, dan role](../../internal/labdb/schema.go#L26).

## Unit eksekusi berikutnya

Bangun importer Go dalam cakupan lokal baru. Gunakan snapshot yang sama, ID target stabil dan catatan mapping dari identitas sumber ke target. Uji Class dan identity secara terpisah dari provisioning login, lalu uji partisipasi dan role dalam transaksi. Gunakan backup dan bukti pemulihan target kosong sebelum penulisan migrasi. State Class pada pembuktian awal ditahan sebagai draft sampai kebijakan publish dan dependency akademik direview.

Rekonsiliasi 267 record inti berdasarkan keputusan sumber, lalu cek FK, keunikan pasangan user–Class, penolakan akses lintas Class, dan pelestarian akun suspended. Uji interupsi, resume, dan replay snapshot yang sama. Hitung penugasan akses serta episode tersendiri karena satu record sumber bisa membentuk beberapa entitas target. Kasus yang tertahan dipertahankan untuk diputuskan dan tidak menjadi pemberian role otomatis.

Jalur M15 kini punya temuan data nyata untuk merancang importer. Perubahan formal modul dan integrasi runtime Go/CI untuk `staging-v2` mengikuti urutan yang disepakati setelah pembuktian lokal. Handover backend nantinya memakai API dan kontrak yang teruji.

## Bukti pengujian

- `PASS_OFFLINE_MAPPING_PROFILE_ONLY` pada input aktual. Dua eksekusi menghasilkan JSON yang sama dan semua hash input asli cocok sebelum serta sesudahnya.
- Ledger hasil rekonstruksi cocok dengan bukti restore. Accounting menjumlah tepat ke 7.843 record.
- Output agregat disimpan di `.qa-artifacts/legacy-core-dry-run-20261122.json`. Pemeriksaan identitas dan relasi berlangsung dalam memori. Nama, email, ID akun, token, dan isi record tidak dicetak.
- Suite regresi operator yang sudah ada lulus **49/49**. Bukti impor Go, QA API setelah migrasi, dan rekonsiliasi target masih menunggu unit eksekusi berikutnya.

Jalankan ulang profil dari root backend Go dengan `.venv\Scripts\python.exe -B -m qa.operators.profile_legacy_core` untuk menghasilkan agregat yang sama. [Operator dry run](../../qa/operators/profile_legacy_core.py)
