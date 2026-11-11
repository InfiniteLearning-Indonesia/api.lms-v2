# Implementation Plan — LMS v3

> **Status 29 Oktober 2026:** [M13-CP03](backend-v3/backend-go/docs/checkpoints/2026-10-29-m13-cp03.md) **IMPLEMENTED_UNVERIFIED**. Fresh synthetic QA terhenti sebelum backup/migrasi karena PostgreSQL 18 Windows restricted-token error 87 pada sandbox. Go unit/regression/vet lulus, tetapi CP03 belum memenuhi kontrak Playwright + Go/DB + restore/reconciliation; **M13 tetap 2/4 CP lokal teruji** dan canonical M00–M18 tetap **0/97 D fully verified**. Jangan mulai CP04 atau menggunakan data nyata; ulang CP03 pada fresh cluster yang dapat berjalan. Status bertanggal lebih lama di bawah adalah histori.

> **Status 30 September 2026:** [M07-CP04](backend-v3/backend-go/docs/checkpoints/2026-09-30-m07-cp04.md) lulus **7/7 frozen cases** pada fixture lokal dengan backup SQL/custom, isolated restore, reconciliation dan full dependency regression. M07 kini **4/4 CP slice lokal sintetis**, tetapi **0/5 canonical D**; total M00–M18 tetap **0/97 D fully verified**. Tidak ada koneksi staging/live, data copy, delete, commit, push atau deploy. Berikutnya M06-CP04 setelah scope/mapping freeze atau M13-CP03 setelah AI/policy freeze; M00/M01/M02 tetap gate sebelum data nyata. Semua status bertanggal lebih lama di bawah adalah histori, bukan pembatalan bukti.

> **Status 29 September 2026:** [M10-CP04](backend-v3/backend-go/docs/checkpoints/2026-09-29-m10-cp04.md) **4/4** dan [M05-CP05](backend-v3/backend-go/docs/checkpoints/2026-09-29-m05-cp05.md) **5/5 CP slice lokal sintetis**. Run final CP05 `20260929T133707Z-postgres-lab-46f4fba76430` PASS_SYNTHETIC_ONLY, 5/5 frozen cases, backup SQL/custom + isolated restore, reconciliation dan dependency regression. **Canonical M00–M18 tetap 0/97 D fully verified**; database staging/live belum disentuh. Urutan berikut: scope freeze M07-CP04 untuk lane sintetis; M00/M01/M02 serta academic/identity/source gates sebelum data nyata. Angka/status bertanggal lebih lama di bawah adalah histori.

> **Rekonsiliasi 24 September 2026:** [catatan koreksi](backend-v3/backend-go/docs/RECONCILIATION_2026-09-24.md), [peta M/CP](backend-v3/backend-go/docs/MODULE_CHECKPOINT_MAP.md), dan [baseline PostgreSQL lokal sintetis](backend-v3/backend-go/docs/LOCAL_SYNTHETIC_BASELINE.md) adalah pembacaan status terbaru. M09 4/4 CP lokal, M10 3/4, M13 2/4 berdasarkan review bukti retrospektif, M15 5/5 slice sintetis; M00–M18 tetap **0/97 D fully verified**. M10-CP04 adalah lanjutan dependency-linear lokal; M00/M01/M02 tetap prioritas sebelum data nyata. Semua pembaruan bertanggal lebih lama di bawah adalah histori.

> **Pembaruan M12-CP02/CP03, 15 September 2026:** [entry/revision dan scoped personal-mentor review](backend-v3/backend-go/docs/checkpoints/2026-09-15-m12-cp02-cp03.md) lulus 8/8 frozen Go/DB+Playwright cases dengan fresh SQL/custom backup + actual isolated restore sebelum masing-masing schema installment. M12 kini **3/4 slice lokal**, canonical **0/5 D**; full M00–M18 tetap 0/97 D. Run final `20260915T113408Z-postgres-lab-c710a6f77c5f`, source `a008d3be4df930256dbb273576fdac775fbdad199054adc40dada63885b9d4e0`, `PASS_SYNTHETIC_ONLY`. Tidak ada staging/production access, delete, commit atau push. M12-CP04 menunggu reviewed legacy mapping/authority; lane sintetis berikut dapat M13-CP01.

> **Pembaruan M12-CP01, 14 September 2026:** [period-set Class](backend-v3/backend-go/docs/checkpoints/2026-09-14-m12-cp01.md) lulus 4/4 frozen Go/DB+Playwright cases pada PostgreSQL sintetis baru setelah backup SQL/custom dan actual isolated restore. M12 kini **1/4 slice lokal**, canonical **0/5 D**; full M00–M18 tetap 0/97 D. Run `20260914T143931Z-postgres-lab-39c96c14ef9a` PASS_SYNTHETIC_ONLY, source `25174065464f9b8b1c4496c24fe8fe4268d33e7cdbe01f36bbfb050c0b1d7d84`. Tidak ada staging/production access, delete, commit atau push. Lanjutan dependensial M12-CP02; M11-CP02 menunggu policy attendance/permit. Status M12 `NOT_STARTED` pada tabel histori yang lebih lama tidak lagi berlaku.

> **Pembaruan 14 September 2026:** [M15-CP05 dan M11-CP01](backend-v3/backend-go/docs/checkpoints/2026-09-14-m15-cp05-m11-cp01.md) lulus pada fixture PostgreSQL lokal dengan fresh SQL/custom backup + actual isolated restore per schema/data installment, Go/DB dan Playwright (3/3 serta 4/4 frozen cases). M15 kini 5/5 **slice sintetis**, M11 1/4 **slice lokal**; canonical M15 tetap 0/6 dan M11 0/5. Run final `20260914T114807Z-postgres-lab-aba48d8f856a` PASS_SYNTHETIC_ONLY. Seluruh angka "status terkini" bertanggal 13 September di bagian bawah adalah histori. Tidak ada koneksi staging/production, data copy, delete, commit atau push; M16 real rehearsal tetap tertahan identitas/backup/approval. Lanjutan dependensial: M11-CP02 setelah policy attendance/permit disepakati, atau M12-CP01 sebagai lane independen.

**Versi dokumen:** 1.3 — pelacakan berbasis capaian M, CP sebagai bagian modul  
**Tanggal:** 5 September 2026; rekonsiliasi status 24 September 2026; evidence historis tetap bertanggal sesuai run  
**Backend target:** Go (Golang), modular monolith  
**Database target v3:** database staging yang sudah ada, dengan namespace dan akses terpisah  
**Database legacy:** database live tetap melayani LMS live sampai cutover terkontrol  
**Branch implementasi:** `be-v3`  
**Status terkini (29 September):** PARTIAL_IMPLEMENTATION pada PostgreSQL sintetis lokal. M03 6/6, M04 2/5, M05 5/5, M06 3/4, M07 3/4, M08 3/4 dengan CP04 parsial, M09 4/4, M10 4/4, M11 1/4, M12 3/4, M13 2/4, dan M15 5/5 CP slice lokal; ini bukan persentase kesiapan produksi. Semua D canonical M00–M18 tetap 0/97 fully verified. M02 containment legacy, backup production/staging, actual data copy, shared runtime, frontend integration, remote CI dan rollout belum ditutup. Perubahan M09/M10/M05 dan dokumen ini masih berada di worktree lokal, belum commit/push. Root service/Git: `backend-v3/backend-go`.

> Dokumen ini dibuat sebagai rencana pada 5 September. Tabel berlabel status awal/baseline dan contoh struktur tetap historis/target, bukan status terbaru. Mulai revisi 1.1, **M00–M19 adalah unit utama capaian; setiap M dipecah menjadi CP milik modul**. Baca [peta M → CP](backend-v3/backend-go/docs/MODULE_CHECKPOINT_MAP.md), [brief per M](backend-v3/backend-go/docs/README.md), dan [dashboard evidence](backend-v3/backend-go/docs/IMPLEMENTATION_STATUS.md). Restrukturisasi ini hanya mengubah dokumentasi, bukan menjalankan kode, backup, migrasi, frontend atau deployment baru.

### Navigasi dokumen

| Kebutuhan pembaca | Bagian |
|---|---|
| Keputusan produk dan perbedaan dari rancangan lama | 1–3 |
| Alur Admin, Teacher, Student dan fitur khas LMS | 4 |
| Backend Go, API, logical schema dan source of truth | 5–6 |
| Pengamanan web/backend terlebih dahulu | 7 |
| Paket developer M00–M19, dependency, complexity dan peta CP per M | 8, terutama 8.3 |
| Aturan no-delete, staging isolation, backup/restore | 9–11 |
| Copy production, delta/resume dan mapping seluruh tabel | 12–13 |
| Ledger status, test matrix, UX/performance readiness | 14–15 |
| Pilot, single writer, cutover dan fallback | 16 |
| Branch `be-v3`, push, roadmap, risiko dan checklist | 17–19 |

## 1. Keputusan produk dan batas pekerjaan

### 1.1 Keputusan yang sudah ditetapkan dari diskusi

1. Backend LMS v3 ditulis dengan **Go**, bukan melanjutkan NestJS/TypeScript sebagai runtime v3.
2. Sistem berpusat pada **Class**. Admin dapat membuat Class, menetapkan Teacher dan Student, lalu menjalankan pembelajaran.
3. Class tidak memerlukan Program atau Batch aktif. Banyak Class dapat berjalan bersamaan dengan jadwal berbeda.
4. Program/category dan cohort label adalah metadata opsional. Admin boleh mengisinya langsung dalam form Class; tidak ada prasyarat membuat hierarchy sebelumnya.
5. Teacher yang ditugaskan pada Class memiliki hak mengelola pembelajaran Class tersebut. Spesialisasi Web/Mobile/UIUX/Professional tidak memberi permission otomatis.
6. Seorang user dapat menjadi Teacher di Class A dan Student di Class B. Role diputuskan berdasarkan konteks resource yang sedang diakses.
7. Fondasi domain mengikuti benchmark Moodle: identity, enrollment, role assignment, activity, submission, gradebook, completion, file, dan audit dipisahkan.
8. Fitur khas LMS dipertahankan jika masih berguna: personal mentor, attendance, izin/sakit, SP, logbook, rubric/competency, AI suggestion, import nilai, transkrip, dan sertifikat.
9. Frontend v3 dikerjakan belakangan. Pengamanan frontend legacy yang diperlukan untuk menghentikan XSS/session leak tetap masuk tahap awal.
10. Database staging menjadi database v3. Database live tetap digunakan LMS live selama transisi.
11. Penyalinan production ke staging dilakukan bertahap, dengan provenance, validasi, dan pelacakan status.
12. **Tidak pernah menghapus data atau object database sebagai bagian dari rencana ini.** Backup wajib sebelum setiap migrasi.
13. Tidak menimpa staging dengan restore production. Data staging yang sudah ada harus tetap tersimpan.
14. Setiap paket mempunyai bukti test dan status migrasi. Commit/push terukur ke `be-v3` dilakukan saat diminta setelah diff, secret check, dan evidence direview; QA lokal tidak otomatis memublikasikan pekerjaan.

### 1.2 Urutan prioritas

| Urutan | Fokus | Hasil yang harus ada |
|---|---|---|
| 0 | Identitas environment, baseline, backup | Source/target benar, perlindungan data dapat dibuktikan |
| 1 | Pengamanan legacy web/backend | Jalur account takeover, perubahan nilai ilegal, file leak, dan penghapusan tertutup |
| 2 | Fondasi Go dan domain Class | Class dapat dibuat tanpa Batch/Program wajib; akses sesuai penugasan |
| 3 | Pembelajaran dan catatan akademik | Activity, submission history, gradebook, completion berjalan konsisten |
| 4 | Rehearsal migrasi production ke staging | Data sumber tersimpan, mapping dapat diulang, konflik dijelaskan |
| 5 | Fitur khusus dan kesiapan frontend | API stabil, extension berfungsi, backend siap diintegrasikan |
| 6 | Pilot dan perpindahan bertahap | Hanya unit yang lulus gate beralih ke v3 |

Pekerjaan dokumentasi, unit test Go, dan desain kontrak dapat berlangsung saat akses database production masih menunggu. Penyalinan production tidak dapat berlangsung sebelum identitas source/target dan backup terverifikasi.

Pada praktik sampai 24 September, jalur Go sintetis telah berjalan melewati beberapa modul saat M00/M01/M02 nyata tertahan. Ini tidak mengurangi prioritas pengamanan legacy web/backend atau memberi izin memakai data live. Sampai baseline live sah tersedia, gunakan [fixture PostgreSQL lokal](backend-v3/backend-go/docs/LOCAL_SYNTHETIC_BASELINE.md) untuk model dan kasus tepi; kemudian bandingkan dengan source live melalui inventory read-only, keputusan mapping dan rehearsal terpisah.

### 1.3 Hal yang tidak diwajibkan pada rilis pertama

- Rewrite frontend atau pergantian design system.
- Moodle sebagai dependency runtime, penyalinan schema Moodle, atau kompatibilitas plugin Moodle.
- Mandatory `CourseDefinition -> CourseRun -> Batch` sebelum Class dapat dibuat.
- Multi-tenant, microservices, SCORM, LTI, forum, wiki, payment, atau question engine lengkap.
- Satu Batch aktif global, satu Program aktif per Student, dan distribusi Mentor berdasarkan nama spesialisasi.
- Migrasi serentak seluruh user/Class dalam satu malam.

Quiz/question bank disediakan sebagai jalur pengembangan opsional; fitur tersebut belum menjadi prasyarat memindahkan fitur yang sudah digunakan.

## 2. Referensi, fakta awal, dan keputusan yang menggantikan rancangan lama

### 2.1 Referensi utama

- [Audit implementasi](LMS_CODEBASE_AUDIT_AND_REMEDIATION.md): temuan keamanan SEC-01 s.d. SEC-22, arsitektur AR-01 s.d. AR-18, data integrity, dan hasil verifikasi.
- [Benchmark Moodle/modern LMS](LMS_Modern_Moodle_Benchmark_Architecture_Business_Flow.md): invariant domain, enrollment, contextual capability, activity, gradebook, completion, file, dan histori.
- `backend/SourceOfTruth.MD`, `TechSpec.MD`, dan dokumentasi legacy: bukti intent lama, bukan aturan final v3.
- `backend-live`, `frontend-live`: checkout acuan perilaku live; SHA deployment server tetap perlu dikonfirmasi.
- `backend`, `frontend`: checkpoint staging dan perbaikan yang perlu diseleksi.

Audit menyarankan mempertahankan NestJS dan memetakan Program+Batch ke CourseRun. **Bagian rekomendasi tersebut digantikan keputusan produk terbaru:** Go, Class mandiri, cohort opsional, dan tanpa penghapusan schema/data. Temuan faktual audit tetap berlaku sampai diperbaiki dan diuji.

### 2.2 Baseline repository dari pemeriksaan sebelumnya

| Checkout | Branch | Commit | Hubungan |
|---|---|---|---|
| `backend-live` | `master` | `f73edf261d006686a3ae317aa93c411e3648b850` | Ancestor backend staging |
| `backend` | `staging` | `dd3413e771409bee9ccb38c59d9576b86545b0cf` | 8 commit di depan live; 19 file berubah |
| `frontend-live` | `master` | `77ba1a3f2333d56efefa4b0830c4743ca069d252` | Ancestor frontend staging |
| `frontend` | `staging` | `d1793b14e300af4b33ab7ddec7b29c0553d38038` | 14 commit di depan live; 49 file berubah |

Temuan ini bukan bukti bahwa server production sedang menjalankan SHA tersebut. Paket M00 harus mencatat deployed artifact, commit, environment override, dan perubahan manual server bila ada.

Backend live dan staging memiliki 110 route decorator dalam 7 controller. Staging tidak menambahkan backend endpoint untuk `/classes/unassigned-students` dan `/classes/claim-mentee`, meskipun frontend staging memanggil keduanya.

### 2.3 Kondisi database yang sudah diketahui dan batas buktinya

Pemeriksaan sebelumnya menemukan konfigurasi lokal `backend-live/.env` dan `backend/.env.staging` identik untuk host, port, database, user, dan password. Kesamaan itu **tidak membantah** informasi bahwa deployment memakai dua database; itu berarti dua file lokal belum dapat dipakai untuk mengidentifikasi keduanya.

Database yang berhasil diinventarisasi sebelumnya memiliki 20 tabel `public`, 50 constraint, 24 index, dan tanpa RLS policy pada tabel tersebut. Agregat saat pemeriksaan: 6 user, 4 Class, 2 enrollment, 2 assignment, 0 submission. Itu bukan inventory production yang telah dikonfirmasi dan bukan baseline kapasitas production.

Minimal satu schema declaration berbeda: `batches.logbookSchedule` ditambahkan pada staging. `importantLinks.scope` dan `importantLinks.programs` mengubah struktur payload JSON tanpa memerlukan kolom baru. Kesamaan jumlah tabel tidak membuktikan kesamaan schema atau makna data.

### 2.4 Status nyata pada awal rencana

| Item | Status | Keterangan |
|---|---|---|
| Audit source staging | COMPLETED_REFERENCE | Tersedia dalam dokumen audit; bukan audit deployment |
| Perbandingan commit live/staging | COMPLETED_REFERENCE | Sudah diperiksa; ulangi jika checkout berubah |
| Identitas database production | BLOCKED_INPUT | Konfigurasi production yang berbeda belum teridentifikasi |
| Identitas database staging untuk v3 | NEEDS_VERIFICATION | Label lokal belum cukup sebagai bukti project identity |
| Backup SQL yang lolos gate | NOT_AVAILABLE | Percobaan terdahulu gagal network sandbox, lalu validasi TLS; tidak dianggap backup |
| Restore rehearsal | NOT_STARTED | Archive-readable saja pun belum cukup |
| Penyalinan production -> staging | NOT_STARTED | Tidak ada import production yang sudah dinyatakan berhasil |
| Backend Go | NOT_STARTED | `backend-v3` disiapkan pengguna untuk implementasi mendatang |
| Security fix deployed | NOT_VERIFIED | Dokumen audit/rencana bukan bukti pengamanan sudah live |
| Branch `be-v3` | LOCAL_REFERENCE_EXISTS | Pernah dibuat lokal; isi/worktree terbaru perlu dicek kembali sebelum implementasi |
| Push paket rencana/implementasi | NOT_PERFORMED_BY_THIS_DOCUMENT | Dilakukan kemudian sesuai release ledger |

Artifact percobaan backup yang gagal, bila masih ada, tetap disimpan dengan status FAILED. Dilarang menggunakannya sebagai backup valid atau menganggapnya berisi copy production.

Tabel 2.4 adalah **snapshot awal 5 September**: baris `Backend Go NOT_STARTED` dan `Restore rehearsal NOT_STARTED` tidak berlaku untuk fixture lokal sekarang. Posisi terkini ada di [dashboard](backend-v3/backend-go/docs/IMPLEMENTATION_STATUS.md). Backup dan restore yang kemudian lulus hanya untuk cluster sintetis; backup production/target nyata tetap belum ada.

### 2.5 Koreksi penting terhadap diskusi sebelumnya

| Usulan lama | Keputusan v3 |
|---|---|
| Satu `ClassMembership` dengan kolom `role` sebagai seluruh sumber partisipasi dan permission | UI boleh menampilkan membership terpadu, tetapi enrollment dan role assignment disimpan terpisah |
| Program+Batch wajib menjadi CourseRun | Class adalah pelaksanaan pembelajaran mandiri; template/version opsional |
| Kelas lama otomatis digabung menjadi satu per Program+Batch | Default migrasi mempertahankan satu Class lama menjadi satu Class v3; penggabungan harus diputuskan eksplisit |
| Dual-write legacy dan v3 ke sumber akademik yang sama | Satu write owner per unit migrasi; projection/import satu arah dengan sumber jelas |
| Fase contract menghapus tabel/kolom legacy | Fase akhir hanya menghentikan pemakaian dan membatasi akses; object/data tetap ada |
| Status lulusan memblokir seluruh akun | Completion berada pada enrollment/Class; akses identitas global diputuskan terpisah |
| Semua staging fix aman dipromosikan | Setiap perubahan dipilah: bug fix, perubahan produk, risiko, dan ketergantungan schema |

## 3. Business model LMS v3

### 3.1 Class sebagai unit utama

Class mempunyai ID stabil, nama bebas yang dapat diedit, short code, deskripsi, status, konfigurasi pembelajaran, tanggal opsional, timezone, dan metadata opsional. Nama, program label, dan cohort label tidak pernah dipakai sebagai foreign key atau sumber authorization.

Create Class menerima nama terlebih dahulu. Short code dapat dibuat server; Teacher dan Student dapat ditambahkan dalam request yang sama atau setelah Class tersimpan sebagai DRAFT. Publish memvalidasi kesiapan Class, termasuk minimal satu Teacher aktif secara default.

Contoh Class yang semuanya valid:

- `Web Development — September 2026`, cohort label `September 2026`, dua Teacher.
- `UI/UX Fundamentals`, tanpa cohort dan tanpa tanggal akhir.
- `Professional Skills`, Student berasal dari beberapa Class lain.
- `Game Development Remedial`, berlangsung bersamaan dengan Class utama.

Mengubah label cohort tidak memindahkan enrollment, nilai, absensi, atau file. Mengarsipkan sebuah Class tidak mengarsipkan Class lain yang memiliki label sama.

### 3.2 Program, cohort, template, dan group

| Konsep | Fungsi | Pengaruh terhadap akses |
|---|---|---|
| Program/category label | Pengelompokan dan filter laporan | Tidak memberi akses otomatis |
| Cohort label pada Class | Keterangan angkatan/periode | Tidak memberi enrollment atau status otomatis |
| Cohort entity opsional | Daftar orang untuk bulk enrollment terkontrol | Perlu explicit sync/enroll command; membership cohort sendiri bukan akses Class |
| Class template | Menyalin struktur belajar untuk Class baru | Tidak menyalin Student, grade, submission, atau certificate |
| Learning group | Subkelompok peserta dalam Class | Filter kerja/bimbingan; Teacher default tetap memiliki hak di seluruh Class |
| Personal mentor assignment | Mentor pembimbing untuk peserta tertentu dalam Class | Tidak otomatis menjadi pemilik seluruh materi atau satu-satunya grader |

Metadata boleh menggunakan autocomplete dengan inline create, tetapi membuat registry category/cohort tidak menjadi langkah wajib bagi Admin.

### 3.3 Identity, enrollment, dan role assignment

`User` menjawab siapa orangnya. `Enrollment` menjawab keikutsertaan dan statusnya dalam Class. `RoleAssignment` menjawab capability yang diberikan pada konteks Class. API `add participant` mengoordinasikan record tersebut dalam satu transaksi.

Contoh: Admin memilih Budi sebagai Teacher. Backend membuat/mengaktifkan enrollment participant, menambahkan role Teacher pada Class yang dituju, menyimpan audit dan outbox. Admin tidak perlu memahami dua tabel itu.

Enrollment tidak memiliki kolom global `role` yang menentukan seluruh akses. Role Student juga tidak cukup untuk submit apabila enrollment sudah suspended/ended. Teacher biasa pada v3 membutuhkan enrollment staff aktif dan role Teacher aktif. Admin dapat memakai capability sistem untuk mengelola Class tanpa enrollment, dengan audit.

Satu enrollment stabil per `(class_id, user_id)` mempunyai enrollment episode/history. Keluar lalu bergabung kembali tidak menghapus episode lama. Jika peserta mengulang dengan penilaian baru, buat episode baru dan tentukan apakah histori sebelumnya hanya dilihat atau mendapat credit transfer; jangan otomatis mewariskan nilai.

### 3.4 Hak akses default

Default berikut adalah keputusan implementasi yang dapat direview sebelum paket M04 selesai. Semua Teacher di Class memiliki kemampuan akademik setara; tidak ada pembatasan berdasarkan nama spesialisasi.

| Aksi | Site Admin | Teacher pada Class | Facilitator pada Class | Student pada Class | User di luar Class |
|---|---|---|---|---|---|
| Create Class | Ya | Tidak, kecuali capability terpisah | Tidak | Tidak | Tidak |
| Edit metadata belajar | Ya | Ya | Tidak | Tidak | Tidak |
| Kelola section, materi, tugas, rubric | Ya | Ya | Tidak | Tidak | Tidak |
| Publish/revise aktivitas | Ya | Ya | Tidak | Tidak | Tidak |
| Lihat dan beri nilai submission | Ya, audit wajib | Ya | Tidak | Hanya hasil sendiri setelah release | Tidak |
| Tambah/suspend Student dalam Class | Ya | Ya melalui capability participant management | Tidak secara default | Tidak | Tidak |
| Assign/revoke Teacher | Ya | Tidak secara default | Tidak | Tidak | Tidak |
| Kelola attendance dan izin | Ya | Ya | Ya | Ajukan/lihat milik sendiri | Tidak |
| Review logbook | Ya | Ya | Tidak secara default | Isi/revisi milik sendiri | Tidak |
| Publish grade/transcript | Ya | Ya | Tidak | Lihat milik sendiri jika released | Tidak |
| Issue certificate | Ya | Ya jika capability dan eligibility terpenuhi | Tidak | Ambil milik sendiri | Verifikasi publik minimal jika diaktifkan |
| Archive Class | Ya | Tidak secara default | Tidak | Tidak | Tidak |
| Nonaktifkan akun global | Ya | Tidak | Tidak | Tidak | Tidak |
| Hard-delete data | Tidak tersedia | Tidak tersedia | Tidak tersedia | Tidak tersedia | Tidak tersedia |

Member Teacher+Student pada Class yang sama tidak boleh mengubah nilainya sendiri atau menyetujui certificate dirinya sendiri. Review independen diperlukan. Jika pola tersebut tidak dibutuhkan, API assignment dapat menolaknya dengan alasan yang jelas; keputusan dicatat dalam ADR.

Permission dari UI/view switcher tidak dipercaya. Backend memuat Class sebenarnya dari ancestry resource, memeriksa role aktif, enrollment, status Class, dan capability. Member di Class A tidak dapat mengubah submission Class B dengan mengganti route parameter.

### 3.5 Lifecycle yang eksplisit

| Domain | State utama | Aturan transisi |
|---|---|---|
| Akun | INVITED, ACTIVE, DISABLED | Admin/verified activation mengubah akses global; completion tidak mengubah akun |
| Class | DRAFT, PUBLISHED, CLOSED, ARCHIVED | Publish memvalidasi struktur; close menghentikan learner write; archive tetap menyimpan histori |
| Enrollment | INVITED, ACTIVE, SUSPENDED, ENDED | Status partisipasi independen dari kelulusan; seluruh transisi punya history |
| Enrollment episode | OPEN, ENDED | Rejoin/retake dapat membuat episode baru; episode lama tetap ada |
| Activity | DRAFT, PUBLISHED, WITHDRAWN | Withdraw menyembunyikan dari kegiatan baru; revision/data learner lama tetap ada |
| Submission | DRAFT, SUBMITTED, RETURNED, LOCKED | Revisi selalu append; status graded berada pada Grade, bukan pengganti submitted |
| Grade | UNGRADED, DRAFT, FINAL, VOIDED | Void menambah riwayat; tidak menghapus keputusan lama |
| Activity completion | NOT_STARTED, IN_PROGRESS, COMPLETED | Pass/fail dicatat sebagai outcome terpisah |
| Class completion | NOT_EVALUATED, IN_PROGRESS, COMPLETED, REOPENED | Evaluation menyimpan evidence dan policy revision |
| Izin/sakit | PENDING, APPROVED, REJECTED, CANCELLED | Effect pada attendance hanya saat approval eksplisit |
| Logbook review | PENDING, REVISION_REQUIRED, ACCEPTED | Revision baru dan feedback lama dipertahankan |
| Certificate | ISSUED, REVOKED, SUPERSEDED | Nomor, snapshot, dan artifact lama tidak dihapus |
| File | PENDING_UPLOAD, QUARANTINED, AVAILABLE, RESTRICTED, RETIRED | Retired tidak menghapus object |

Tanggal akhir Class tidak mengubah record secara diam-diam melalui GET. Scheduled command yang idempotent dapat menutup Class sesuai kebijakan. Correction Class tertutup harus lewat command tersendiri dengan alasan, actor, before/after, dan versi.

## 4. Alur bisnis end-to-end

### F01 — Provision user dan akses pertama

1. Admin mengundang user atau memilih user yang sudah ada. Pembuatan akun tidak memberikan akses semua Class.
2. Backend menormalkan email untuk lookup, memeriksa collision identitas, dan menyimpan identity/profil minimal.
3. Invitation/reset menggunakan token acak, hashed, purpose-specific, sekali pakai, dan memiliki expiry; tidak ada universal default password.
4. Email diproses outbox. Gagal email tidak membuat enrollment hilang atau dianggap terkirim.
5. Google login memvalidasi provider subject dan verified identity; akun yang sama tidak digabung hanya karena string nama/email mirip.
6. Session diterbitkan dengan cookie aman. Login tidak memperbaiki atau menambah Class/Program/mentor assignment.

### F02 — Create dan publish Class

1. Admin memasukkan nama, metadata opsional, Teacher, dan Student opsional.
2. Backend membuat Class DRAFT. Jika participant disertakan, Class+assignment+enrollment+audit dibuat atomik.
3. Teacher menambahkan section dan activity. Tidak perlu memilih Batch aktif.
4. Publish memvalidasi Teacher, title, ordering, reference, tanggal, file, grading policy, completion policy, dan prerequisite graph.
5. Class PUBLISHED terlihat oleh participant yang eligible; aktivitas DRAFT tetap tersembunyi.
6. Dua Teacher mengedit versi sama: edit pertama berhasil, edit kedua mendapat `409 VERSION_CONFLICT` beserta versi terbaru. Tidak ada silent overwrite.

### F03 — Assign Teacher/Student dan bulk enrollment

1. Admin atau Teacher dengan capability memilih Class dan user.
2. Backend memeriksa actor, target role yang boleh diberikan, dan status Class.
3. Simpan enrollment dan role assignment terpisah dalam transaksi; repeated request dengan idempotency key sama mengembalikan hasil sama.
4. Bulk import melalui preview: valid rows, duplicate candidates, invalid rows, dan proposed effects ditampilkan sebelum apply.
5. Chunk apply memiliki atomic boundary yang terdokumentasi. Status `PARTIAL` tidak pernah ditampilkan sebagai seluruh import berhasil.
6. Notification hanya dikirim setelah commit dan hanya di mode delivery yang diizinkan.
7. Mengakhiri Student di Class A menutup episode/aksesnya; tidak menghapus submission/grade dan tidak mengubah Class B.

### F04 — Akses materi dan aktivitas

1. Authenticate -> cek account access -> resolve ancestry activity -> cek Class permission/enrollment -> cek publish/availability.
2. Availability dapat berupa tanggal, dependency completion, group, dan nilai minimum yang dikonfigurasi.
3. Konten hanya dikirim sesuai akses, termasuk file/embed terkait. UUID yang sulit ditebak bukan kontrol authorization.
4. GET tidak menandai completion. Bila rule `view` dipilih, client mengirim explicit evidence command yang idempotent.
5. Backend menjelaskan alasan unavailable dalam kode terstruktur yang dapat diterjemahkan frontend nanti.

### F05 — Submit dan resubmit assignment

1. Backend memeriksa enrollment episode aktif, activity published, due/cutoff policy, dan file/link valid.
2. Submit membuat revision immutable dengan timestamp server, assignment revision, serta nomor urut.
3. Identitas Student diambil dari session, bukan body yang dapat diarahkan ke user lain.
4. Transaction menulis revision, status submission, audit, dan outbox bersama-sama.
5. Duplicate request tidak membuat revision kedua. Repeated intentional submit memakai idempotency key berbeda.
6. Resubmit menyimpan revision baru; grade sebelumnya tetap terkait revision yang dinilai. Kebijakan review ulang eksplisit.
7. Due date adalah batas terlambat; cutoff adalah batas penerimaan. Keduanya dapat berbeda. Waktu dihitung menggunakan timezone policy Class.

### F06 — Penilaian, rubric, import, dan AI

1. Teacher pada Class membaca submission dan revision yang dipilih.
2. Rubric version, kriterianya, bobot, dan maksimum nilai tervalidasi.
3. Manual grading menulis GradeRevision+Grade projection+Audit+Outbox dalam satu transaksi.
4. Gradebook adalah sumber nilai untuk laporan. Activity tidak menulis final Class score sendiri.
5. AI menghasilkan suggestion dengan source/model/prompt version, error state, dan evidence hash. Provider gagal tidak menghasilkan nilai lulus.
6. Teacher menyetujui atau mengganti suggestion. Approval, override reason, dan source revision disimpan.
7. Import external score memakai preview, actor capability, mapping item, score validation, serta duplicate policy. Import tidak bebas menciptakan kurikulum tanpa preview.
8. Grade draft dan release visibility terpisah. Student menerima hanya nilai sendiri yang dirilis.

### F07 — Progress, completion, transcript, dan certificate

1. Activity evidence memenuhi rule revision tertentu; completion bukan `score / 100`.
2. Progress menghitung eligible required activities menurut policy version, dengan exception untuk waived activity yang terdokumentasi.
3. Class completion mengevaluasi aktivitas wajib, passing grade bila dipilih, logbook/attendance bila dipilih.
4. Grade tinggi dengan logbook wajib belum accepted dapat menghasilkan `NOT_COMPLETED`; activity nongraded dapat tetap completed.
5. Transcript memakai Gradebook snapshot dan release policy. Nilai belum dinilai tampil sebagai belum dinilai, bukan 65.
6. Certificate diterbitkan per peserta/episode setelah eligibility terpenuhi; nomor unik, issuer, timestamp, completion evidence, dan document hash disimpan.
7. Perubahan policy atau grade setelah issuance tidak diam-diam mengganti certificate; gunakan superseding/correction workflow.
8. `Class CLOSED`, `Enrollment ENDED`, `Completion COMPLETED`, dan `Certificate ISSUED` adalah status berbeda.

### F08 — Attendance, izin/sakit, dan SP

1. Teacher/Facilitator menjadwalkan pertemuan dalam Class; attendance melekat pada meeting dan enrollment episode.
2. Ketidakhadiran tidak ditebak hanya dari record yang hilang. Meeting eligibility dan penutupan attendance menentukan `UNRECORDED` atau `ABSENT`.
3. Student mengajukan izin untuk dirinya dan meeting yang relevan, dengan evidence file private.
4. Reviewer menyetujui/menolak. Approval attendance dan audit bersifat atomik; approval ulang idempotent.
5. SP dihitung dari policy version Class. Buat disciplinary case/warning; jangan menyalin SP ke seluruh row attendance.
6. Sanksi Class memengaruhi enrollment Class tersebut. Menonaktifkan akun global tetap tindakan Admin terpisah.
7. Hari async hanya berlaku pada Class/group/meeting yang relevan. Aturan seorang Mentor tidak mengurangi attendance semua user di seluruh sistem.

### F09 — Logbook

1. Class dapat mengaktifkan logbook dengan periode dan jadwal sendiri; tidak harus empat bulan atau membutuhkan Batch.
2. Student menyimpan draft/submits dalam periode yang eligible.
3. Setiap resubmit membuat revision, reviewer melihat versi yang dinilai.
4. Teacher mengubah review ke revision-required atau accepted dengan feedback.
5. Accepted revision terkunci untuk edit; reopening eksplisit menyimpan alasannya.
6. Completion/nilai membaca acceptance evidence. Jadwal dan persyaratan minimal kata diperiksa backend, bukan UI saja.

### F10 — Personal mentor dan kolaborasi lintas Class

Admin dapat menugaskan Teacher UI/UX pada Class Web dan Mobile, serta Teacher Professional pada Class yang relevan. Tidak perlu hardcode hubungan tersebut. Teacher pada Class tetap boleh mengelola pembelajaran Class sesuai default yang disepakati.

Personal mentor opsional membantu pembagian student/logbook. Default satu personal mentor aktif per peserta per Class jika fitur diaktifkan, dijaga constraint dan temporal history. Pergantian mentor mengakhiri assignment lama dan membuat assignment baru; histori karya/nilai tidak berpindah ownership ke user yang salah.

### F11 — Duplikasi, arsip, dan perbaikan historis

- Duplicate Class menyalin struktur/content yang dipilih ke DRAFT baru dengan asal revision tercatat. Jangan menyalin enrollment, submission, grade, completion, certificate, izin, atau SP.
- Archive menyembunyikan Class dari aktivitas aktif dan membatasi write. Semua row dan object tetap tersimpan.
- Correction data historis memakai capability khusus, reason, immutable revision, audit, dan laporan affected outcomes.
- Renaming Class/category/cohort tidak mengubah meaning nilai atau relasi historis.

### F12 — Peralihan legacy ke v3

Class di bawah owner LEGACY tetap menulis ke live. Copy di staging berfungsi sebagai rehearsal/shadow. Setelah unit migrasi lulus gate, learner write difence, final delta disalin dan direkonsiliasi, ownership diganti ke V3, kemudian write dibuka pada v3. Detail copy dan backup ada pada Bagian 10–13; cutover dan fallback ada pada Bagian 16.

## 5. Arsitektur backend Go yang dituju

### 5.1 Bentuk aplikasi dan boundary

Gunakan **modular monolith Go**: satu backend dengan ownership modul yang tegas, satu PostgreSQL target staging, serta proses worker dari codebase Go yang sama. Belum perlu microservices, Kubernetes, Kafka, atau Redis agar Class dan grade dapat dibuat dengan benar.

Alur request: client → reverse proxy → HTTP middleware Go → authentication → resource-context policy → application command/query → domain/repository → PostgreSQL. Side effect keluar dari transactional outbox → worker → email, storage, atau provider yang diizinkan.

| Boundary | Tanggung jawab | Yang dilarang |
|---|---|---|
| HTTP handler | Decode, ukuran request, DTO, status/error, request ID | Menentukan role dari body; query langsung ke banyak modul |
| Application command | Policy, transaksi, idempotency, state transition | HTTP/provider call ketika transaksi DB masih terkunci |
| Domain | Invariant dan keputusan bisnis deterministik | Dependency ke framework HTTP, email, global batch |
| Repository | SQL terparameterisasi dan lock yang eksplisit | Menyembunyikan write dalam getter atau auto-migration |
| Policy | Resolve ancestry dan contextual capability | Memercayai `classId`, `role`, atau `studentId` dari frontend |
| Worker | Outbox, retry, timeout, quota, job state | Menulis nilai final AI; menjalankan production notification saat rehearsal |
| Migration tool | Backup gate, extract, map, validate, checkpoint | Menjadi fitur admin web atau berjalan saat API startup |

Go bukan obat otomatis untuk permission, SQL lambat, atau UX. Keuntungan yang ditargetkan: deployment binary sederhana, boundary eksplisit, concurrency yang dapat dibatasi, type checking, dan tooling test. Biayanya: tim harus membangun ulang session integration, validation, policy, jobs, dan parity fitur yang dahulu ditangani ekosistem NestJS.

### 5.2 Pilihan teknologi untuk implementation kickoff

| Area | Default rencana | Syarat keputusan |
|---|---|---|
| Bahasa | Go, versi supported yang dipin saat M03 | CI dan container memakai versi yang sama; jangan mengunci versi dari ingatan |
| HTTP | `net/http`; router ringan jika diperlukan | Library dipilih setelah spike, tanpa framework yang mengaburkan policy |
| PostgreSQL | `pgx/v5`, SQL eksplisit; `sqlc` dapat dipakai | Verifikasi kompatibilitas pooler/prepared statements; query tetap direview |
| Migration | File SQL forward-only, dijalankan tool/deployer terpisah | Tidak ada auto-run pada startup dan tidak menyediakan destructive down |
| Auth web | Opaque server-side session dalam HttpOnly cookie | Keputusan domain cookie/SSO dan revocation harus diuji bersama legacy |
| API | OpenAPI + DTO eksplisit `/api/v3` | Tidak menyalin response ORM atau menerima unknown authority fields |
| Async | PostgreSQL transactional outbox + worker Go | At-least-once, idempotent consumer, bounded concurrency |
| File | R2 yang sudah dipakai, private assets | IAM, private download, inventory/backup object terpisah dari SQL |
| Observability | Structured logs, metrics, traces, append-only domain audit | Redaction default, jangan jadikan log HTTP satu-satunya audit |
| Runtime | Container non-root, immutable image, health/readiness terpisah | Secrets dari deployment secret store, bukan baked ke image |

Pin dependency, jalankan `govulncheck`, dan review advisory pada saat implementasi. Tidak perlu memperkenalkan ORM hanya untuk menyamai TypeORM. Transaction handle harus diteruskan secara eksplisit ke repository yang terlibat dalam satu command. Dasar mekanisme transaksi, pool, dan cancellation: [dokumentasi database Go](https://go.dev/doc/database/).

### 5.3 Struktur usulan — belum membuat folder/file

```text
backend-v3/                     # hanya target struktur; tidak di-scaffold saat menulis plan
  cmd/api/                      # HTTP process
  cmd/worker/                   # jobs/outbox process
  cmd/migrate/                  # operator-only forward migrations/import/verification
  internal/platform/            # config, db, http, telemetry, transaction, clock
  internal/identity/             # account, verified identity, session, invite
  internal/access/               # context, role, capability, assignment, policy
  internal/classes/              # class lifecycle, metadata, groups
  internal/enrollment/           # participation, episodes, personal mentor
  internal/content/              # section, activity, version, availability
  internal/files/                # assets, references, upload, authorized download
  internal/submissions/          # attempts/revisions, hand-in rules
  internal/gradebook/            # grade items, policy, revisions, release
  internal/completion/           # evidence, rules, outcome, certificate
  internal/attendance/           # meetings, records, permits, disciplinary cases
  internal/logbook/              # periods, submissions, reviews
  internal/integrations/         # provider adapters, AI suggestions, import
  internal/notifications/        # templates, preferences, delivery state
  internal/audit/                # domain audit, sensitive-read evidence
  internal/reporting/            # scoped read models/export
  internal/migration/            # legacy readers, mapping, reconciliation, ledger
  api/openapi/                  # versioned contract
  db/migrations/                # additive SQL, pre/postconditions and checksum
  db/queries/                   # module-owned query definitions
  tests/contract/                # legacy/v3 behavior and capability matrix
  tests/integration/             # real PostgreSQL, concurrency, migration rehearsal
  docs/                         # ADR, runbooks, package/status evidence
```

Module tidak boleh mengubah table milik module lain tanpa application service/transaction contract yang disepakati. Query reporting boleh join lintas modul melalui read model; bukan jalur mutasi alternatif. Hindari menyalin `ClassesService` menjadi satu `classes_service.go` raksasa.

### 5.4 API dan UX yang harus didukung backend

- `POST /api/v3/classes`: nama wajib; metadata program/cohort opsional; optional initial participants dibuat secara atomik bila dalam satu request.
- `POST /api/v3/classes/{id}/participants`: UX Add Teacher/Add Student, tetapi backend menulis enrollment dan role assignment yang berbeda.
- `GET /api/v3/me/classes`: Class yang benar-benar dapat diakses, role/capability ringkas, status dan next actions. Tidak scan semua program lalu filter di browser.
- Activity/submission/grade endpoints selalu resolve resource→Class dari database. Nested path yang tidak cocok ditolak.
- `POST .../grade-imports/preview` lalu `POST .../grade-imports/{id}/commit`: preview terikat hash input, actor, Class, policy revision, dan expiry; perubahan setelah preview memerlukan preview ulang.
- Async command mengembalikan `202` + job ID + status URL. UI tidak harus menunggu AI/email/import besar dalam satu HTTP request.
- Gunakan pagination, allowlist sort/filter, field minimization, response error terstruktur, conflict `409`, validation error `422`, body too large `413`, rate limit `429`.
- Mutasi penting menerima idempotency key scoped actor+operation+Class. Key sama/body sama menghasilkan hasil lama; key sama/body berbeda ditolak.
- Include resource version/ETag untuk optimistic concurrency. Jangan silent last-write-wins pada grade, role assignment, dan publish rules.
- Waktu disimpan UTC `timestamptz`, display dan calendar memakai timezone Class; tanggal bisnis disimpan `date` bila memang bukan timestamp.
- Frontend boleh memakai capability response untuk menyembunyikan tombol, tetapi backend tetap memeriksa setiap request. Tidak ada Supabase service key dalam browser.
- Frontend v3 belum dibuat. OpenAPI, contoh payload, error catalog, dan mock fixtures adalah handoff agar nanti frontend tidak menebak flow.

## 6. Logical schema, ownership, dan invariant

Nama table berikut adalah target desain, **bukan SQL yang sudah dieksekusi**. Final DDL direview melalui M03/M04/M15 setelah snapshot aktual diverifikasi.

### 6.1 Registry entity

| Modul | Entity/tabel target | Source of truth dan relasi utama |
|---|---|---|
| Identity | `users`, `external_identities`, `account_state_events` | Identity global; bukan tempat Program/Batch/kelulusan menentukan akses |
| Authentication | `sessions`, `credential_versions`, `identity_tokens` | Session scoped issuer/environment; hashed token, expiry, consumed/revoked state |
| Access | `contexts`, `roles`, `capabilities`, `role_capabilities`, `role_assignments`, `role_assignment_events` | Site atau Class context; assignment provenance terpisah dari participation |
| Class | `classes`, `class_versions`, `class_categories` | Class merupakan delivery context; metadata optional, owner bukan satu mentor |
| Participation | `enrollments`, `enrollment_episodes`, `enrollment_events` | Satu participation aggregate per user+Class; episodes/history mempertahankan re-entry |
| Grouping | `class_groups`, `group_memberships`, `mentor_assignments` | Group/person-to-mentor assignment opsional, selalu dalam Class |
| Optional cohort | `cohorts`, `cohort_memberships`, `cohort_sync_runs` | Bulk grouping convenience; tidak wajib untuk membuat Class dan tidak memberi capability |
| Content | `class_sections`, `activities`, `activity_revisions`, `availability_rules` | Definisi dan revision; tidak berisi submission atau learner completion |
| Files | `file_assets`, `file_references`, `file_state_events` | Private object identity, checksum, ownership/context, lifecycle dan references |
| Assignment | `assignment_settings`, `submission_aggregates`, `submission_revisions` | Setting terikat activity revision; karya terikat enrollment episode dan immutable revision |
| Gradebook | `grade_items`, `grade_policy_versions`, `grade_revisions`, `grade_current`, `grade_release_events` | Revision adalah authoritative academic history; current projection bukan sumber independen |
| Rubric | `rubric_versions`, `rubric_criteria`, `competency_definitions`, `class_competencies`, `rubric_edges` | Definisi reusable boleh; penilaian terikat Class, grade item, dan learner episode |
| Completion | `completion_rule_versions`, `learning_evidence`, `activity_completions`, `class_completions`, `completion_events` | Evidence+rule menghasilkan outcome; grade hanya salah satu input opsional |
| Credentials | `transcript_snapshots`, `certificates`, `certificate_events` | Snapshot angka/policy/issuer saat diterbitkan, revocation bukan delete |
| Attendance | `class_meetings`, `attendance_records`, `attendance_revisions`, `calendar_exceptions` | Meeting Class + enrollment; exception tidak berlaku global ke semua student |
| Permits/SP | `permission_requests`, `permission_decisions`, `disciplinary_cases`, `disciplinary_events` | Request berbeda dari approval; SP scoped Class/participation |
| Logbook | `logbook_periods`, `logbook_entries`, `logbook_revisions`, `logbook_reviews` | Period fleksibel per Class, learner episode, reviewer berwenang |
| Integrations | `integration_accounts`, `integration_secret_versions`, `evaluation_suggestions`, `import_jobs`, `import_rows` | Secret reference/ciphertext; AI dan import mempunyai provenance/job state |
| Operations | `outbox_events`, `consumer_receipts`, `job_attempts`, `notification_deliveries`, `audit_events` | Append-only event/attempt history; retry bukan side effect ganda |
| Migration | `migration_ops.*`, `migration_landing.*` | Origin mapping, immutable source snapshots, evidence, ownership/cutover ledger; bukan API learner |

Tidak semua optional table harus dibuat di sprint pertama. Jangan menciptakan organization/tenant hierarchy tanpa kebutuhan yang jelas; deployment ini dapat tetap satu institusi. Jika multi-tenant nanti dibutuhkan, desain isolation dibuka sebagai ADR dan migration tersendiri, tidak ditiru setengah-setengah sekarang.

### 6.2 Constraint wajib pada table baru

1. UUID internal v3 terpisah dari legacy integer ID. `origin_environment + source_instance_id + source_table + source_pk` memetakan ke ID target. Email/nama bukan primary migration identity.
2. Unique enrollment `(class_id,user_id)`; rejoin memakai episode. Default maksimal satu episode aktif, dengan constraint/serialized command.
3. Role assignment unik untuk active `(context_id,user_id,role_id,assignment_source)`; revoke satu sumber tidak mencabut grant dari sumber lain tanpa kebijakan eksplisit.
4. Semua learner-state record terkait enrollment/episode, bukan hanya global `student_id`. Constraint parent `(id,class_id)` dan FK composite memastikan activity, submission, grade item, group, meeting, dan enrollment benar-benar satu Class. Evaluasi cost dan index dalam DDL review.
5. Submission aggregate unik sesuai `(assignment_id,enrollment_episode_id,attempt_number)`; immutable revision punya nomor unik. Default satu attempt dapat mempunyai banyak revision, bukan row “latest” yang bersaing.
6. Grade slot unik `(grade_item_id,enrollment_episode_id)`; revision ordinal unik dan current pointer memakai version compare-and-swap. Regrade membuat revision baru.
7. Attendance unik `(meeting_id,enrollment_id)`; logbook unik `(period_id,enrollment_episode_id)`; personal mentor active unique jika kebijakan satu mentor digunakan.
8. Grade menggunakan tipe decimal/numeric, bukan float untuk akumulasi authoritative. Bounds mengikuti grade item; tidak selalu 0–100 bila maksimum item berbeda. Weight finite, non-negative, sum sesuai versi policy. Missing tetap NULL/ungraded.
9. Publish memvalidasi rubric graph acyclic, child existence, bobot, due/cutoff, rule references. Version published tidak diubah tanpa revision dan impact review.
10. FK baru `RESTRICT`/`NO ACTION` untuk catatan akademik; tidak ada cascading DELETE. Archive tidak membuang relation.
11. Unique short code Class terpisah dari nama; nama dan cohort label boleh sama. Jangan dedupe Class berdasarkan nama.
12. Index pada FK dan query aktual: user→enrollment, Class→activity, grade item→current, meeting→attendance, status→pending job. Jangan menganggap FK otomatis membuat index di referencing column.
13. Audit/revisions mencatat actor, reason, request ID, source, effective time, recorded time. History yang tidak tersedia di legacy tidak boleh direka menjadi event seolah benar-benar pernah terjadi.
14. Tidak ada global unique “hanya satu Batch aktif”. Class A/B/C boleh published sekaligus.

### 6.3 Source-of-truth contracts

| Pertanyaan | Jawaban canonical | Bukan jawabannya |
|---|---|---|
| Siapa peserta Class? | Enrollment + episode state | `User.batchIds`, `selectedProgram`, global student role |
| Siapa boleh mengajar? | Active role/capability dalam Class context | Nama spesialisasi atau `Class.mentorId` tunggal |
| Apakah tugas sudah dikumpulkan? | Submitted revision dan server receipt | Nilai tidak NULL atau flag UI |
| Berapa nilainya? | Released grade revision sesuai policy | AI suggestion, fallback 65/75, completion percentage |
| Apakah selesai/lulus? | Completion outcome+rule snapshot | User status GRADUATED atau grade semata |
| Siapa boleh membuka bukti sakit? | File context + permit capability/owner | Siapa pun yang tahu public URL |
| Sistem mana boleh menulis? | Migration-unit ownership + enforced fence | UI sedang dibuka dari domain mana |

## 7. Pengamanan terlebih dahulu — legacy dan v3

### 7.1 Aturan pelaksanaan

Ini adalah **rencana perbaikan**, bukan pernyataan bahwa aplikasi live sudah aman. Temuan berasal dari audit source; SHA yang benar-benar deployed dan mitigasi infrastruktur harus diverifikasi. Emergency legacy patch merupakan containment selama Go dibangun, bukan perubahan keputusan bahasa v3.

Deploy security patch pada branch hotfix sesuai baseline live setelah review/test. Port patch relevan ke staging legacy secara selektif; jangan merge seluruh `be-v3` ke aplikasi NestJS live. Frontend legacy security patch harus coordinated dengan backend, terutama penghapusan token fallback agar login tidak terputus.

Perubahan data/DDL untuk hardening mengikuti backup gate yang sama dengan migrasi. Menonaktifkan route atau outbound integration melalui config dapat direncanakan tanpa migrasi, tetapi eksekusi deployment tetap memerlukan persetujuan berikutnya. Jangan menunggu seluruh backend Go selesai untuk menutup account takeover.

### 7.2 Security remediation registry

Path di kolom legacy relatif terhadap `backend/` atau `frontend/`; padanan `backend-live/` dan `frontend-live/` wajib dicek terhadap live SHA sebelum hotfix.

| Temuan audit / severity | Containment legacy dan target Go | Area sumber / acceptance test |
|---|---|---|
| SEC-01 Critical | Disable setup-password berbasis email; ganti verified single-use token, purpose, expiry, consumed state; account-wide session revocation | `backend/src/auth/auth.controller.ts`, `auth.service.ts`; tanpa token tidak dapat mengganti password, token replay/expired ditolak |
| SEC-02 Critical | Disable kedua grade import sampai context policy+preview+transaction tersedia | `backend/src/classes/classes.controller.ts`, `classes.service.ts` import/smart-import; student/unrelated teacher tidak dapat mengubah satu pun grade |
| SEC-03 High | Stop raw sessionID response/Bearer/query fallback; HttpOnly session, regenerate, server revocation; redact logs | `backend/src/auth/guards/session-auth.guard.ts`, `auth.controller.ts`, `common/middleware/http-logger.middleware.ts`; `frontend/lib/config.ts`, `app/dashboard/page.tsx`; leaked old ID tidak dapat replay |
| SEC-04 High | Sanitize rich content saat write/render; allowlist URL/embed; CSP bertahap; simpan original berbahaya hanya di restricted migration history | `frontend/components/rich-text-renderer.tsx`, material/assignment pages; XSS corpus tidak execute termasuk double encoding |
| SEC-05 High | CSRF token pada unsafe methods, exact trusted origin, JSON-only bila sesuai; cookie sesuai topologi | `backend/src/main.ts`, seluruh mutating routes; cross-origin form/request ditolak walau victim login |
| SEC-06 High | Resolve ancestry + capability policy sebelum read/write; disable destructive routes | `backend/src/classes/classes.controller.ts`, `classes.service.ts`; foreign Class/material/submission/grade IDs ditolak |
| SEC-07 High | Split self-profile dan admin assignment DTO; reject role/program/batch/specialization-as-authority pada self edit | `backend/src/users/users.controller.ts`, `users.service.ts`, `dto/create-user.dto.ts`; self PATCH tidak mengubah enrollment/scope |
| SEC-08 High | Actor-derived requester, Class-scoped review; izin PENDING tidak mengubah attendance | `backend/src/attendance/attendance.controller.ts`, `attendance.service.ts`; forge studentId/approve sendiri ditolak |
| SEC-09 High | Private proof files, authorized reads, MIME+size+scan; fail closed upload, tanpa base64 fallback | `backend/src/storage/storage.service.ts`, `attendance.service.ts`; public URL tak lagi memberi akses, file asing ditolak |
| SEC-10 High | Central egress policy, validate DNS/IP/redirect, deny metadata/private ranges, timeout/size; disable arbitrary runtime AI host | `backend/src/storage/storage.service.ts`, `classes/services/ai-evaluator.service.ts`, avatar flow `users.service.ts`; SSRF test di isolated environment |
| SEC-11 High | AI suggestion-only, explicit failed state, no fallback pass score, human approval | `ai-evaluator.service.ts`, bulk evaluation di `classes.service.ts`; provider failure/prompt injection tidak menulis grade final |
| SEC-12 High | Secret reference/encryption, write-only response, rotate existing credentials, least provider scopes | `backend/src/users/entities/user.entity.ts`, AI settings `classes.service.ts`, `frontend/app/dashboard/components/mentor/mentor-profile-settings.tsx`; response/log/dump operasional tidak expose token |
| SEC-13 High | Actor-scoped read models dan field minimization/pagination | `classes.service.ts` program/details/score reads, attendance reads; student tidak mendapat kontak/nilai/proof orang lain |
| SEC-14 High | Revoke/rotate embedded API key, hapus fallback pada patch kode nanti, secret scan | `backend/src/attendance/attendance.service.ts` holiday sync; tanpa configured key feature fail closed |
| SEC-15 High | Account ACTIVE wajib; disable→revoke. Kelulusan dipindah ke Class outcome, bukan blanket login deny | `backend/src/auth/auth.service.ts`, session guard, `users/entities/user.entity.ts`; graduated historical learner tetap boleh akses sesuai policy, disabled staff tidak |
| SEC-16 High | Validated config, no default secret/password/bootstrap admin; schema synchronize dan startup repair DDL off | `backend/src/main.ts`, `src/app.module.ts`, Google strategy; salah env gagal start tanpa mutation |
| SEC-17 Medium | TLS verify peer/hostname dengan provider CA; runtime role least privilege | DB pool di `app.module.ts` dan session pool di `main.ts`; invalid cert gagal, tidak fallback insecure |
| SEC-18 Medium | Request/import/file limits, queue, capped AI concurrency, pool/query deadlines | `main.ts`, storage, bulk endpoints; oversized request ditolak sebelum fan-out besar |
| SEC-19 Medium | Transactional domain audit; trusted proxy exact; durable restricted sink; hentikan audit-row deletion | `backend/src/audit/audit.service.ts`, `audit.interceptor.ts`, HTTP logger; committed grade selalu punya actor/history/audit |
| SEC-20 Low | Minimal public health; detail DB/memory/history internal-auth only | `backend/src/health/health.controller.ts`, `health-history.service.ts`; anonymous tidak melihat diagnostics |
| SEC-21 High | Tidak ada shared invitation password; invitation unique verified token; force secure activation | `backend/src/users/users.service.ts`, mail/activation flow; dua undangan tidak memiliki reusable shared credential |
| SEC-22 High dalam audit | Jalankan dependency advisory review baru pada locked production tree; patch+compatibility tests backend dan frontend | `backend/package.json`, `backend/package-lock.json`, `frontend/package.json`, `frontend/package-lock.json`; jangan menganggap advisory snapshot audit sudah menggambarkan versi deployed terbaru |

### 7.3 Data safety juga bagian P0

- Inventory dan tutup semua hard-delete route/service, CASCADE path, startup auto-DDL, read/login auto-repair, dan scheduled cleanup sebelum data rehearsal diakses legacy staging.
- Audit lama menyarankan “invalidate sessions”; implementasi harus menggunakan `revoked_at`, account auth epoch, atau validator yang menolak legacy session, **bukan DELETE table session**.
- Session-store pruning, audit archive-then-delete, expired token purge, outbox cleanup, file cleanup dan test teardown harus diaudit. Aturan no-delete berlaku juga pada job otomatis, dependency session store, dan operator tool.
- Rotasi secret membuat credential lama tidak berlaku. Restricted historical data tetap tersimpan sesuai aturan no-delete; jangan menghidupkan credential lama saat restore. Jangan menampilkan secret lama di report atau diff.
- Content berbahaya dipertahankan sebagai restricted raw snapshot; normal renderer hanya memakai revision yang sudah aman. Jangan merender “backup original” lewat UI biasa.
- Untuk public R2 proof, private ACL/policy dan penutupan public route harus diverifikasi. Perubahan row URL saja tidak mencabut akses ke object lama; cache/external copy mungkin sudah ada. Catat residual exposure dan lakukan respons insiden bila diperlukan, tanpa menghapus object.

### 7.4 Supabase/PostgreSQL access boundary

Staging yang menerima production data harus diperlakukan sebagai lingkungan sensitif: batasi developer/read access, audit ekspor, jangan share dumps lewat chat/Git, pisahkan secret runtime, migration, dan backup. Web Go menggunakan least-privilege DB role; browser tidak mendapat akses langsung ke academic tables.

Schema baru `v3`, `migration_landing`, dan `migration_ops` tidak diekspos ke Data API. Jika Data API tidak digunakan, rencanakan disable setelah dependency inventory; jika tetap diperlukan, grants dan RLS harus dikonfigurasi serta diuji untuk setiap exposed object. Tidak adanya RLS pada table private yang hanya diakses backend **bukan otomatis bukti public exploit**. Service/admin credentials tetap bukan mekanisme user authorization. Lihat [Supabase: securing your API](https://supabase.com/docs/guides/api/securing-your-api).

Config checklist: DB host/project/role/database/CA fingerprint; TLS `verify-full`; pool bounds; statement/lock/idle timeout; session issuer/cookie/secret; CSRF trusted origins; exact proxy trust; R2 endpoint/bucket/IAM; email sink; outbound allowlist; AI enabled=false di rehearsal; migration mode; backup root/encryption key reference; public/internal health endpoints. Nilai rahasia tidak ditulis dalam plan.

Prior backup attempt belum sukses karena trust certificate belum terselesaikan. Ambil CA yang sesuai dari provider/dashboard dan validasi hostname; jangan mengubah ke `rejectUnauthorized:false` atau `sslmode=disable` untuk melewati blocker. Pengaturan SSL enforcement provider dapat memengaruhi availability dan tidak boleh diubah otomatis saat diagnosis. Lihat [Supabase SSL enforcement](https://supabase.com/docs/guides/platform/ssl-enforcement).

## 8. Paket implementasi modular

### 8.1 Dependency dan baseline 5 September (historis)

Status pada tabel ini merekam **awal rencana 5 September**, bukan kondisi 9 September. Posisi aktual per M/CP ada di §8.3 dan brief modul; jangan menyalin NOT_STARTED historis menjadi status terbaru. `BLOCKED_INPUT` hanya memblokir aktivitas yang membutuhkan input tersebut; desain/unit test yang tidak memerlukan DB tetap dapat berjalan dalam otorisasi.

| Paket | Prioritas | Deliverable | Dependency | Kompleksitas | Status awal |
|---|---|---|---|---|---|
| M00 | P0 | Environment/repo/deployment inventory dan keputusan baseline | Persetujuan identitas deployment | Small | PARTIAL: source inventory ada, production identity belum |
| M01 | P0 | Backup+restore rehearsal, manifest dan gate | M00 untuk DB yang diakses, CA/access | Medium | BLOCKED_INPUT: backup verified belum ada |
| M02 | P0 | Emergency security legacy backend/web | M00; M01 sebelum setiap perubahan DB | Large | NOT_STARTED dalam plan ini |
| M03 | P1 | Go platform, schema foundation, CI/CD, outbox/audit substrate | M00; M01 untuk DDL | Medium | NOT_STARTED |
| M04 | P0/P1 | Identity, sessions, contextual policy | M03 | Large | NOT_STARTED |
| M05 | P1 | Class, enrollment, staff, grouping | M03, M04 | Large | NOT_STARTED |
| M06 | P0/P1 | Private FileAsset dan safe outbound fetch | M03, M04, M05 context contract | Medium | IN_PROGRESS — CP01–03 local |
| M07 | P1 | Content, activity revisions, assignment settings | M05, M06 | Medium | IN_PROGRESS — CP01–02 local |
| M08 | P1 | Submission aggregate/revisions | M07 | Medium | NOT_STARTED |
| M09 | P1 | Gradebook, rubric, import transaction, release | M05, M08, M03 audit/outbox | Large | NOT_STARTED |
| M10 | P1 | Completion, transcript, certificate | M07, M09 | Large | NOT_STARTED |
| M11 | P1 | Attendance, permit, calendar, Class-local SP | M05, M06 | Large | IN_PROGRESS: CP01 synthetic local PASS; canonical 0/5 |
| M12 | P1 | Logbook, periods, personal mentor review | M05, M06 | Medium | IN_PROGRESS: CP01 synthetic local PASS; canonical 0/5 |
| M13 | P1/P2 | Notifications, integration adapters, AI suggestions | M03, M04, M06; M08/M09 untuk AI | Medium | NOT_STARTED |
| M14 | P1/P2 | Scoped reports, API compatibility contract, performance | M05–M13 sesuai fitur | Medium | NOT_STARTED |
| M15 | P0/P1 | Mapping, import tool Go, reconciliation, migration ledger | M00, M01, M03 dan target schema per domain | Large | CP01–05 synthetic local PASS; canonical 0/6 dan real gates blocked |
| M16 | P0/P1 | Production snapshot→staging rehearsal lengkap | M02 containment staging, M15, modules untuk scope | Large | BLOCKED_INPUT: production/target identity + backup |
| M17 | P1 | Pilot readiness, frontend integration gate, write fence/cutover | M02, M14, M16, frontend readiness nanti | Large | NOT_STARTED |
| M18 | P1/P2 | Rollout bertahap, operational promotion, read-only archive | M17 pilot lulus | Large | NOT_STARTED |
| M19 | P3 | Optional templates/cohort sync/quiz/standards | Core stabil; kebutuhan produk disetujui | Large per fitur | DEFERRED |

Kompleksitas bukan estimasi hari. Large berarti lintas domain/data/security atau perlu rehearsal; ukuran tim, volume production, dan kebutuhan support menentukan durasi setelah M00. QA/security bukan pekerjaan yang ditunda ke M17: setiap paket punya test dan gate sendiri.

### M00 — Baseline dan identitas environment

- **What/why:** bekukan evidence repo dan deployed SHA, identifikasi source production dan target staging yang benar, hindari copy database ke dirinya sendiri atau memperbaiki checkout yang tidak deployed.
- **Tasks:** cek remote/branch/worktree read-only; route/schema/migration/job inventory; read-only fingerprint source/target; inventory table counts/size, extensions, constraints/indexes, object storage, scheduler, access roles, RPO/RTO. Catat status dirty tanpa menghapus perubahan user.
- **Affected:** `backend-live/`, `backend/`, `frontend-live/`, `frontend/`, existing deployment config, schema metadata; bukti sanitised nanti di `docs/baseline/`.
- **Migration:** tidak ada writes; raw PII/secret tidak masuk evidence versioned. Label DB tidak cukup, harus project identity dan konfigurasi deployment aktif.
- **Gate/tests:** source dan target benar-benar berbeda, deployed SHA diketahui, daftar blocking unknown mempunyai owner. Jika fingerprint sama, hentikan import.
- **Fallback:** tetap dokumentasi/design/local tests; tidak menebak credential production.
- **Commit rencana:** `docs(baseline): record verified environments and migration boundaries`.

### M01 — Backup, restore, dan perlindungan operator

- **What/why:** buat recovery evidence sebelum ada mutation. SQL file saja tanpa restore test bukan backup yang lulus gate.
- **Tasks:** gunakan tool PostgreSQL versi sesuai; source consistent dump dan target pre-migration dump; simpan local `.sql` serta custom archive bila diperlukan, checksum, encryption/ACL, manifest, separate storage file inventory; restore ke DB kosong disposable **yang baru**, validate schema/count/relation checks; tidak menjalankan `--clean`.
- **Affected:** target masa depan `cmd/migrate/`, `internal/migration/backup/`, `docs/runbooks/backup.md`, secret store/operator host; bukan script TS.
- **Migration:** first schema bootstrap pun perlu target backup terlebih dahulu. Source account read-only, target backup role berbeda dari target migrator. Detail Bagian 11.
- **Gate/tests:** invalid CA gagal; truncated dump gagal; wrong target gagal; restore berhasil; SQL plain file dapat diperiksa operator berwenang tanpa secrets terpampang di log.
- **Fallback:** stop sebelum write bila backup/restore gagal. Jangan mengklaim backup provider saja mencakup R2 objects.
- **Commit rencana:** `feat(migration): add verified backup gate and restore evidence`.

### M02 — Emergency containment legacy

- **What/why:** tutup SEC-01/02 lebih dulu lalu chain session/XSS/CSRF/BOLA, file/SSRF, insecure config dan unsafe deletion. Go rewrite tidak membenarkan membiarkan live vulnerable.
- **Tasks:** security registry Bagian 7 dijadikan subtask M02-A auth, M02-B contextual policy, M02-C browser/session, M02-D file/egress/secrets, M02-E schema/jobs/no-delete, M02-F dependencies/telemetry. Disable high-risk functionality jika scoped fix belum siap.
- **Affected:** file legacy pada Bagian 7; frontend security minimal, tanpa redesign. Patches live dibuat terhadap live baseline, ditautkan ke paket `be-v3`.
- **Migration:** revoke dengan history/epoch; jangan DELETE session atau rewrite grade historis. Additive DDL hanya setelah backup. Freeze schema synchronize dan archival deletion sebelum copy data.
- **Gate/tests:** reproducible negative tests semua Critical; safe authenticated regression login/submit/read; security reviewer sign-off; deployed SHA dan bukti mitigasi tercatat, bukan hanya commit ada.
- **Fallback:** feature flag off/read-only pada vulnerable flow; rollback tidak boleh membuka kembali known Critical route. Pertahankan evidence insiden.
- **Commit rencana:** `docs(security): track legacy containment and verified deployment` di `be-v3`; kode hotfix pada repo/branch live yang tepat.

### M03 — Fondasi Go dan CI/CD

- **What/why:** buat pondasi transaksi, audit/outbox, config, request budgets dan typed errors sebelum domain berkembang.
- **Tasks:** setelah izin implementasi baru, set struktur Go; pin toolchain; env validation; pool/CA; unit/integration harness; migration checksum lock/preconditions; secrets scanning, lint/test/race/vuln scan, container non-root dan immutable build. Outbox/audit substrate dibuat di sini agar domain tidak menunggu M13.
- **Affected:** calon `cmd/api`, `cmd/worker`, `internal/platform`, `internal/audit`, `db/migrations`, pipeline Go. Tidak ada NestJS runtime dalam service v3.
- **Migration:** create schema/role/table baru secara explicit forward-only. Runtime tidak memiliki DDL/DELETE rights; migration account terpisah. Build/test CI tidak mempunyai credential production.
- **Gate/tests:** missing secret/invalid CA fail start; API startup tidak DDL; graceful shutdown/cancellation; atomic audit+outbox rollback saat business transaction gagal; migration concurrent runner ditolak.
- **Fallback:** matikan API v3; legacy tetap melayani. Schema baru tetap tersimpan tanpa down migration.
- **Commit rencana:** `build(go): establish backend v3 platform and safety gates`.

### M04 — Identity dan contextual authorization

- **What/why:** identity bukan enrollment; login valid tidak berarti boleh mengubah semua Class.
- **Tasks:** users/external identities, secure invite/reset, login/session/revocation, site role dan Class role registry; deny-by-default policy; account status separation; safe last-admin invariant; issuer-specific session cookies; trusted identity bridge decision (Bagian 16).
- **Affected:** calon `internal/identity`, `internal/access`, OpenAPI auth/me; referensi legacy auth/user flow Bagian 7.
- **Migration:** identity mapping memakai origin PK; email collision review. Password hash hanya direuse bila kompatibilitas/verifikasinya diuji; kalau tidak, verified reset. Raw legacy session IDs/API tokens tidak menjadi session/secret v3.
- **Gate/tests:** Teacher A/Student B berbeda capability; stale/revoked assignment segera ditolak pada privileged write; self assignment/self grade denied; concurrent last-admin changes tidak menyisakan nol admin.
- **Fallback:** v3 auth off sampai compatible frontend flow ada; login legacy tetap owner sesuai phase. Jangan auto-login menggunakan raw copied session.
- **Commit rencana:** `feat(identity): implement secure sessions and contextual capabilities`.

### M05 — Class dan participation

- **What/why:** hilangkan dependency global active Batch tanpa kehilangan fitur personal mentor/grouping.
- **Tasks:** standalone Class CRUD non-destructive, metadata/manual cohort, publish/archive states; add Teacher/Student orchestration; enrollment episodes/events; staff assignments; optional group/person mentor. Business mutation tidak terjadi di GET/login.
- **Affected:** calon `internal/classes`, `internal/enrollment`, `internal/access`; pengganti legacy `Class`, `Enrollment`, `Batch` coupling dan user arrays.
- **Migration:** default 1 legacy Class→1 v3 Class; no automatic regroup. Existing duplicate enrollment disimpan di landing dan provenance, satu target aggregate dapat mewakili beberapa sources setelah review; bukan menghapus duplicates dari source.
- **Gate/tests:** nama saja cukup untuk draft; beberapa published Class independen; Teacher2 dapat mengedit Class yang sama; mentor specialization tidak memperluas hak; repeated/concurrent add tidak duplicate; revoke tidak menghapus karya.
- **Fallback:** Class hasil import belum published/learner-writable hingga gate; legacy Class source tetap utuh.
- **Commit rencana:** `feat(classes): add standalone classes and enrollment lifecycle`.

### M06 — File ownership dan outbound security

- **What/why:** proof, submission, avatar, material berbeda privacy; URL bukan permission.
- **Tasks:** FileAsset/private bucket references, MIME signatures/limits, quarantine/scan, authorized download, private signed URLs; centralized bounded fetcher dengan DNS/redirect/IP checks; IAM minimization; download access audit.
- **Affected:** calon `internal/files`, platform egress; legacy `src/storage/storage.service.ts` sebagai behavior yang perlu diganti.
- **Migration:** inventory URL/base64/object references, checksum, private copy bila perlu; retain old objects, restrict public serving; missing object menjadi explicit conflict. DB backup tidak menggantikan object backup.
- **Gate/tests:** student cannot open foreign proof; expired signed URL; blocked loopback/metadata/redirect; scan failure tetap quarantine; tidak ada upload fallback base64 ke normal row.
- **Fallback:** restrict downloads untuk file belum tervalidasi, tampilkan alasan belum tersedia; jangan publish file agar flow terlihat berhasil.
- **Commit rencana:** `feat(files): add private contextual assets and bounded egress`.

### M07 — Content dan activity

- **What/why:** definisi pembelajaran tetap stabil saat learner sudah mengerjakan.
- **Tasks:** sections, ordered activities, safe rich content, revisions, availability/due/cutoff, assignment settings, attachment references; clone structure-only; publish-time validation.
- **Affected:** calon `internal/content`; legacy material/assignment dan class important links menjadi mapping input.
- **Migration:** preserve original content restricted, imported sanitized revision terpisah; normalize shape `importantLinks` per source version, tidak menerka satu format. Snapshot assignment rules untuk future submission.
- **Gate/tests:** published update menghasilkan revision; old submission tetap menunjuk revision lama; content release lintas Class terlarang; clone tidak membawa grades/students/files yang tidak authorized.
- **Fallback:** imported activity DRAFT/RESTRICTED sampai mapping/scan selesai, source tetap available di legacy owner.
- **Commit rencana:** `feat(content): version class activities and availability rules`.

### M08 — Submission

- **What/why:** karya learner tidak hilang akibat resubmit atau dua request bersamaan.
- **Tasks:** aggregate+attempt+immutable revisions, safe draft edits via revisions, server receipt, optimistic lock, idempotency, cutoff/late policy dan file references.
- **Affected:** calon `internal/submissions`; legacy `Submission` dan submit paths.
- **Migration:** setiap legacy row disimpan; source duplicates dipetakan explicit ke revisions/attempts hanya jika chronology valid. Unknown chronology jangan diberi label “attempt 2” tanpa evidence.
- **Gate/tests:** concurrent hand-in konsisten; replay request sama tidak submission kedua; foreign assignment/enrollment denied; deadline timezone test; update karya setelah grading tidak diam-diam mengganti graded revision.
- **Fallback:** imported ambiguous submissions review-only; aktifkan write hanya setelah per-learner reconciliation.
- **Commit rencana:** `feat(submissions): preserve attempt and revision history`.

### M09 — Gradebook dan rubric

- **What/why:** single authoritative grade history, Class-scoped scores, tidak ada minimum fallback yang memalsukan hasil.
- **Tasks:** grade items/policy/rubric versions, cycle/bounds/weight validation, atomic grade revision/current/audit/outbox, explicit release, import preview+confirmed commit, regrade reason and impact report.
- **Affected:** calon `internal/gradebook`; legacy competency/rubrik score/transcript calculations dan import paths.
- **Migration:** simpan raw score, observed legacy display, dan v3 interpreted result terpisah; beda akibat fallback65 diberi reason/review, tidak diam-diam “dikoreksi”. Program-global score yang tidak dapat ditempatkan ke Class tetap legacy evidence.
- **Gate/tests:** two graders conflict bukan silent overwrite; all grade item/learner ancestry checked; self-grade blocked; NULL != 0 != 65; partial failure tidak meninggalkan grade tanpa history.
- **Fallback:** preserve original transcript snapshot, hold grade publication untuk ambiguity; tidak menyalin nilai satu learner ke semua Class yang namanya mirip.
- **Commit rencana:** `feat(gradebook): centralize scoped grades and immutable revisions`.

### M10 — Completion dan credential

- **What/why:** progress, complete, pass, transcript release dan certificate issue adalah keputusan berbeda.
- **Tasks:** evidence intake, rule versioning, explicit activity/class outcomes, override reason, reopen after correction, progress projection, transcript snapshot, certificate verification/revoke/supersede.
- **Affected:** calon `internal/completion`; legacy transcript/certificate code dan release flags.
- **Migration:** preserve legacy issued outputs bila tersedia; jika hanya flags yang tersedia catat `legacy_release_observed`, bukan mengaku punya certificate issuance event. Jangan auto-issue ribuan certificate akibat imported grade event.
- **Gate/tests:** activity complete tanpa nilai; nilai tinggi belum tentu complete; regrade tidak mengedit certificate lama; scoped teacher release/override; replay events tidak duplicate certificate.
- **Fallback:** read-only historical transcript plus clear provenance, no automated new issuance hingga policy disetujui.
- **Commit rencana:** `feat(completion): separate evidence outcomes and credentials`.

### M11 — Attendance, izin, calendar, SP

- **What/why:** pertahankan operasional LMS tetapi pindah dari student+Batch/global flags ke Class meeting dan participation.
- **Tasks:** meetings/schedule/exceptions, attendance revision, permission review lifecycle, private proof, class-scoped disciplinary case, recalculation with evidence. Holidays/provider disabled bila config tidak lengkap.
- **Affected:** calon `internal/attendance`; legacy attendance module, `MentorAsyncDay`, user suspended/SP projections.
- **Migration:** batch-scoped record bukan bukti kehadiran di setiap Class. Simpan legacy batch context dan mapping review; unknown outcome tidak diubah menjadi alpha/absent. Source global SUSPENDED perlu klasifikasi keamanan akun vs SP Class.
- **Gate/tests:** PENDING permit tidak auto-approve; unauthorized reviewer denied; correction tidak duplicate SP; asynchronous mentor date tidak memengaruhi Class lain; account Teacher di Class lain tidak diblok karena SP learner.
- **Fallback:** legacy-scoped archive/report untuk evidence belum terpetakan; module wajib ready sebelum migrate Class yang sedang memakai attendance/SP.
- **Commit rencana:** `feat(attendance): scope meetings permits and discipline to class`.

### M12 — Logbook dan personal mentoring

- **What/why:** logbook tetap ada tetapi bukan hardcoded program 4 bulan atau mentor specialization routing.
- **Tasks:** Class period config, deadline/timezone, entry revisions, submit/review/revise lifecycle, personal mentor assignment history, reviewer capability, atomic decision.
- **Affected:** calon `internal/logbook`, enrollment mentor assignment; legacy `src/classes/entities/logbook.entity.ts`, `batch.entity.ts` (`logbookSchedule`) dan `classes.service.ts`.
- **Migration:** staging `logbookSchedule` yang tidak ada di live tidak boleh diasumsikan tersedia; fallback adalah reviewed config atau preserved legacy rule, bukan ALTER live otomatis. Batch-based old logbook ambigu tetap archive.
- **Gate/tests:** Class berdurasi 2/6 bulan atau arbitrary period; reassignment mentor tidak kehilangan logbook; duplicate review conflict; foreign mentor denied; pending/revised/accepted terpisah.
- **Fallback:** historical logbook view read-only, outstanding reviews wajib diselesaikan/dipetakan sebelum unit cutover.
- **Commit rencana:** `feat(logbook): add configurable periods and scoped review`.

### M13 — Jobs, notifications, AI dan integrations

- **What/why:** external outage tidak merusak transaksi akademik atau memberi nilai lulus palsu.
- **Tasks:** durable delivery state, bounded retry/backoff/dead-letter status tanpa row deletion, notification templates/preferences, safe provider adapters, secret reference/encryption, AI EvaluationSuggestion dengan provenance dan human approval.
- **Affected:** calon `internal/integrations`, `internal/notifications`, `cmd/worker`; audit/outbox substrate dari M03.
- **Migration:** historical email/audit bukan pending outbox; migration event diberi `notification_policy=SUPPRESS`. Jangan copy API key menjadi active credential di staging; reauthorize/rotate ketika feature benar-benar dibutuhkan.
- **Gate/tests:** crash setelah provider success tidak menyebabkan grade/permit ganda; provider idempotency bila tersedia; jika provider email tidak mendukung, document possible duplicate delivery dan dedupe best effort. SMTP/AI/holiday/cron outbound benar-benar diblok di rehearsal, bukan cuma tombol disembunyikan.
- **Fallback:** integrations OFF, manual academic workflow tetap bisa berjalan; retry histories tetap tersimpan.
- **Commit rencana:** `feat(integrations): add durable jobs and non-authoritative AI suggestions`.

### M14 — Reporting, contract dan performance readiness

- **What/why:** frontend nanti mendapat API cepat dan konsisten tanpa mengembalikan legacy coupling.
- **Tasks:** actor-specific dashboard/Class overview, pagination, stable OpenAPI, typed fixtures, legacy route usage inventory, optional compatibility adapter yang read-only/explicitly routed; indexes/EXPLAIN/benchmarks, observability, export audit.
- **Affected:** calon `internal/reporting`, `api/openapi`, `tests/contract`, frontend integration specification; tidak membuat UI v3 sekarang.
- **Migration:** projections rebuildable dari canonical v3, mempunyai watermark/version; export legacy vs v3 normalization terdokumentasi. Tidak mem-persist fallback score sebagai authoritative grade.
- **Gate/tests:** privacy field tests, query count/performance budget, large Class, pagination stable, `/classes/unassigned-students` dan `/classes/claim-mentee` drift diputuskan replace/deprecate sebelum UI integration.
- **Fallback:** disable slow/unsupported report dengan error jelas; jangan fallback read ke live tanpa user/context check atau diam-diam menggabungkan dua canonical stores.
- **Commit rencana:** `feat(api): publish scoped reporting and frontend handoff contracts`.

### M15 — Migration mapping dan tool Go

- **What/why:** copy dapat diulang, data lama tetap utuh, hasil transform dapat dijelaskan per source record.
- **Tasks:** source adapters per schema fingerprint, encrypted dump/restore gate, immutable landing snapshots, ID mapping, reviewed conflict resolutions, dry-run, bounded batch imports, reconciliation, ledger, owner/fence state. Tools hanya operator, tidak terpapar HTTP public.
- **Affected:** calon `cmd/migrate`, `internal/migration`, `migration_ops`, `migration_landing`; mapping Bagian 13.
- **Migration:** source read-only; additive target writes; checkpoints/history; same-source/same-hash replay no-op; source update→new revision. Approve mapping per domain, bukan satu generic auto-map berdasarkan column name.
- **Gate/tests:** rerun/concurrent runner/crash-resume/wrong-target/schema-drift/partial batch fail; counts and totals reconciled; unresolved ambiguities terukur; no notification and no DELETE/TRUNCATE/DROP.
- **Fallback:** mark run failed/quarantined dan buat correction/new dataset; tidak menghapus partially imported rows. Reader hanya melihat build yang approved.
- **Commit rencana:** `feat(migration): add additive legacy import and reconciliation ledger`.

### M16 — Rehearsal production ke staging

- **What/why:** buktikan flow dan restore dengan data nyata yang dilindungi sebelum learner berpindah.
- **Tasks:** pilih connected dependency unit, backup source/target, consistent snapshot, raw landing ingest, mapping Class/identity, backfill, file verification, report/academic parity, sanitized shadow replay, restore drill dan estimated cutover duration.
- **Affected:** staging private namespaces, restricted operator artifacts, migration status reports. Production source tidak dimutasi oleh copy.
- **Migration:** preserve existing staging `public`; imported test writes dipisah dari future cutover candidate (Bagian 10). Semua ambiguity dibawa ke review queue, tidak dibuang agar count tampak bersih.
- **Gate/tests:** minimal dua rehearsal berhasil, salah satunya delta+resume; latest backup restore lulus; invariant/no-leak suite pass; hasil ditandatangani data owner, QA, security, ops.
- **Fallback:** repeat dengan new run/build; live tetap owner. Rehearsal success bukan izin otomatis cutover.
- **Commit rencana:** `docs(migration): record verified rehearsal and reconciliation evidence` — evidence sanitised, bukan dump.

### M17 — Pilot dan ownership switch

- **What/why:** satu unit mempunyai satu writer, tidak kehilangan aktivitas learner saat final copy.
- **Tasks:** frontend compatible readiness **di tahap berikutnya**, identity bridge, learner communication, read-only fence pada semua legacy write paths/jobs, drain, final snapshot/delta, reconcile, ownership epoch update, route switch, monitoring dan acceptance.
- **Affected:** legacy guards/jobs, routing deployment, Go owner check, frontend integration nanti, migration ledger/runbook.
- **Migration:** daftar dependencies menentukan apakah satu Class aman dipilotkan sendiri. Shared batch/global grade dependencies yang belum bisa dipisah dapat memerlukan cutover beberapa Class bersamaan.
- **Gate/tests:** direct legacy API call setelah switch ditolak; stale tabs retries tidak duplicate; live DB tidak mendapat domain write untuk unit owner V3; Go menolak unit owner LEGACY; rollback drill membuktikan penanganan new v3 writes.
- **Fallback:** sebelum first v3 write routing dapat dikembalikan setelah validasi. Setelah ada v3 writes gunakan hold/read-only+forward fix; jangan route kembali ke snapshot legacy stale.
- **Commit rencana:** `docs(cutover): record pilot gates and ownership switch evidence`.

### M18 — Rollout dan operational promotion

- **What/why:** perluas unit yang lulus tanpa membuat database target tetap diperlakukan sebagai playground staging.
- **Tasks:** rollout per dependency unit, support window, migration dashboard, incident response, capacity/backups/PITR/access review, archive reader, operational SLO; dokumentasi Source of Truth v3.
- **Affected:** target staging yang menjadi operational v3 DB, live read-only scope, Go deployment, docs/on-call.
- **Migration:** tiap wave mempunyai backup dan gate baru; jangan reuse bukti snapshot lama sebagai backup wave baru. Setelah final wave live DB tetap retained/read-only sesuai otorisasi, tidak di-drop.
- **Gate/tests:** semua active/historical data accounted, access/job routing sesuai ownership, restore readiness monitored, no dev reset/migration experiments terhadap operational target.
- **Fallback:** stop wave berikutnya, hold affected unit, keep data; tidak melakukan destructive contract phase.
- **Commit rencana:** `docs(rollout): update migration matrix and operational acceptance`.

### M19 — Fitur optional sesudah core stabil

- Templates/library reusable, explicit cohort bulk-sync, groups dengan lebih granular teacher restrictions, quizzes/attempts/question bank, calendar integrations, SCORM/LTI/xAPI hanya jika ada kebutuhan yang disetujui.
- Quiz belum menjadi parity feature wajib jika tidak ada implementasi authoritative di baseline. Bila dibangun, attempts, answer revisions, grading dan completion harus memakai prinsip yang sama; tidak sekadar JSON jawaban di `User`.
- Setiap fitur memiliki ADR, threat model, additive migration+backup, test, complexity/owner, dan paket commit terpisah. Jangan memblokir Class core demi mengimplementasikan seluruh Moodle.

### 8.2 Definition of Done setiap paket

- [ ] Scope dan API/domain invariant direview; dependency paket telah terpenuhi.
- [ ] Unit/integration/negative authorization tests serta relevant race tests lulus.
- [ ] Tidak ada production PII/secret/dump dalam commit, log, fixture, atau screenshot.
- [ ] Bila mengubah schema/data: backup+restore evidence, dry-run, additive SQL checksum, mapping/reconciliation, rollback/forward-fix procedure tersedia.
- [ ] No-delete review termasuk dependency job, session pruning dan test cleanup.
- [ ] OpenAPI, ADR, runbook dan status ledger diperbarui dengan bukti, bukan estimasi sebagai fakta.
- [ ] Review keamanan untuk trust boundary; review akademik untuk grading/completion/attendance policy.
- [ ] Commit scoped dan push ke `be-v3` baru dilakukan saat pekerjaan implementasi tersebut diminta; deployed status dicatat terpisah.

### 8.3 Pelacakan pekerjaan M → CP (rekonsiliasi 24 September)

M menjawab capaian domain/platform apa yang ingin selesai; CP menjawab potongan kerja berikutnya; canonical D membuktikan penerimaan modul. ID pekerjaan baru selalu `Mxx-CPnn` (contoh `M03-CP03`). Nomor CP tidak menjadi pengganti M atau urutan global wajib. CP01–CP12 historis tetap dipertahankan sebagai alias evidence; CP13–CP15 lama dipecah ulang tanpa mengubah makna atau hasil tes sebelumnya.

| M | Rencana CP | Bagian teruji lokal saat rekonsiliasi | Sisa utama |
|---|---:|---:|---|
| M00 | 4 | 1/4 | Production/deployed identity, full inventory/capacity dan owner/access handoff |
| M01 | 4 | 1/4 | Real host/key/object protection, backup source/target dan gate operasional |
| M02 | 4 | 0/4; disposition parsial | Hotfix/containment legacy dan actual release verification |
| M03 | 6 | 6/6 | Seluruh slice lokal teruji; canonical shared/provider/container/remote gates tetap 0/5 |
| M04 | 5 | 2/5 | Real owner/provisioning, refresh/browser/key integration dan full resource ancestry |
| M05 | 5 | 5/5 sintetis lokal | CP05 integration lulus; canonical/archive policy/real identity/shared gates tetap terbuka |
| M06 | 4 | 3/4 | CP04 preserved mapping/consumer closure; actual file/DNS providers/shared gates |
| M07 | 4 | 3/4 | CP01–03 producer teruji; CP04 menunggu M08 consumer/clone closure |
| M08 | 4 | 3/4 exit lokal; CP04 parsial | Receipt/deadline dan retained resubmit teruji; CP04 bounded raw landing/journey lulus, real chronology/closure menunggu M15 |
| M09 | 4 | 4/4 sintetis lokal | Shared runtime, real legacy inventory dan academic approval |
| M10 | 4 | 4/4 sintetis lokal | CP04 legacy output observation lulus; academic policy dan real/shared gates tetap terbuka |
| M11 | 4 | 1/4 sintetis lokal | CP02 attendance/permit policy dan private proof |
| M12 | 4 | 3/4 sintetis lokal | CP04 legacy schedule mapping/closure; source review |
| M13 | 4 | 2/4 sintetis lokal (review bukti) | CP03 AI suggestion, CP04 consumer closure, provider nyata |
| M14 | 4 | 0/4 | Reporting/API/performance contract dan shared runtime |
| M15 | 5 | 5/5 sintetis lokal; full D 0/6 | Preflight, provenance, mapping, private shadow import dan read-only reconciliation fixture lulus; real source/CA/backup/academic review masih gated |
| M16 | 4 | 0/4 | Protected actual rehearsal setup, dua rehearsal, parity/recovery approval |
| M17 | 4 | 0/4 | Pilot readiness, fence/drain, final delta/single writer, actual pilot |
| M18 | 3 | 0/3 | Operational rollout controls, recovery/archive, accepted waves |
| M19 | Belum frozen | N/A | Tiga template CP per approved feature, bukan denominator semua fitur optional |

Daftar tiap CP, dependency, target D, exit/QA dan bukti pendukung ada di [M00–M19 module briefs](backend-v3/backend-go/docs/MODULE_CHECKPOINT_MAP.md#posisi-seluruh-m). Jumlah CP adalah pemecahan awal, bukan estimasi hari, jumlah prompt, atau persentase effort; satu instruksi boleh mengerjakan beberapa CP dependency-ready. Jika perlu dipecah lagi, pertahankan scope union/acceptance serta catat alasannya. Setiap sesi implementasi mempunyai laporan bertanggal di `docs/checkpoints/YYYY-MM-DD-*.md`; status setiap CP tetap dicatat terpisah di brief `docs/modules/Mxx.md` dan peta CP, walaupun dua CP dikerjakan pada tanggal yang sama.

Urutan dependency-linear lokal berikut yang layak dibekukan: **M07-CP04 historical-owner content read/clone**; alternatif M13-CP03 setelah AI/policy scope freeze. M15-CP01–05 lulus hanya pada fixture, sedangkan real chronology M08-CP04 tetap tertahan source adapter, CA, inventory, backup dan academic approval nyata. CP02 `LOCKED` bukan hand-in resmi; receipt CP03 adalah bukti. M06-CP04/M07-CP04 menunggu preserved mapping/consumer closure. Jalur environment/backup adalah M00-CP02–04 + M01-CP02–04, sedangkan containment legacy adalah M02-CP01–04. [Peta dependency semua CP](backend-v3/backend-go/docs/MODULE_CHECKPOINT_MAP.md#peta-eksekusi-berdasarkan-dependency).

**Tidak meluluskan M lewat penomoran ulang:** 97 canonical D M00–M18 tetap sama; full verification masih 0/97 pada evidence terakhir. TERUJI_LOKAL hanya merekonsiliasi slice yang memang punya bukti. CP11 4/4 dan CP12 4/4 lama bukan M04/M05 100%. Penutupan tiap M tetap memerlukan seluruh D dan QG applicable, consumer/integration yang diperlukan dan external evidence. Closure dapat menunggu consumer sementara pekerjaan modul lain memakai kontrak core yang sudah diuji; tidak membuat dependency melingkar pada full-module completion.

## 9. Kebijakan tidak menghapus data — invariant operasional

### 9.1 Yang tidak boleh dijalankan

Dalam source production, staging existing, namespace v3, landing, ledger, atau database restore percobaan yang memuat data pengguna:

- Tidak ada `DELETE`, `TRUNCATE`, `DROP TABLE/SCHEMA/DATABASE/COLUMN`, reset database, destructive down migration, `pg_restore --clean`, atau restore overwrite.
- Tidak ada penghapusan lewat ORM `.remove()`/`.delete()`, cascade FK, maintenance cron, TTL cleaner, session-store pruning, import “replace all”, atau script test cleanup.
- Tidak ada deduplication dengan membuang row; tidak menghapus user/enrollment/assignment agar FK kembali valid.
- Tidak mengosongkan staging sebelum copy, tidak overwrite primary keys yang kebetulan sama, tidak memakai “hapus lalu insert ulang” untuk transfer peserta.
- Tidak menghapus object storage sebagai “orphan cleanup”; tidak mengganti isi object pada key lama. Private ACL/restriction diperbolehkan sebagai rencana pengamanan, history tetap dipertahankan.
- Tidak melakukan contract phase penghapusan legacy schema/columns. Decommission berarti tidak lagi dipakai untuk write dan aksesnya dibatasi.

Perubahan security grants/policy bukan penghapusan data, tetapi tetap diaudit. Perubahan state seperti revoke/archive/disable/cancel dilakukan dengan event/history. Jika nilai existing harus berubah, simpan revision/before-image dahulu dalam transaksi yang sama, lalu update projection; **no-delete tidak berarti membiarkan overwrite kehilangan nilai lama**.

### 9.2 Pengaman berlapis

1. Production extract role read-only tanpa DDL/DML. Hindari menggunakan admin/service role untuk tugas baca.
2. Target application role tanpa DELETE/TRUNCATE/DDL dan tanpa ownership table. Migrator bukan superuser; izin DDL hanya untuk scope yang disetujui. PostgreSQL tidak membatasi seluruh destructive DDL lewat satu grant sederhana, sehingga role ownership, review, allowlist tool, dan human approval tetap diperlukan.
3. Scan migration SQL dan codepath untuk destructive statement, ditambah review manual. String scan saja tidak membuktikan aman karena function/trigger dapat menghapus data.
4. Review triggers, FK actions, scheduled SQL, session dependencies, storage lifecycle rules, audit rotation, worker cleanup, dan test fixtures sebelum runtime dinyalakan.
5. Setiap run mempunyai allowlisted source/target fingerprint, namespace, operation list, backup evidence, dry-run result, approval record, lock, checksum, dan bounded timeout.
6. Quarantine conflict dan preserve original; target projections hanya diterbitkan setelah validation. Corrective migration forward-only, bukan rollback SQL yang menghapus target rows.
7. Sesudah run, bandingkan existing legacy table counts/checksums dan verify tidak ada unexpected mutation. Source dapat berubah karena live traffic; bedakan perubahan traffic sah dengan migration-origin writes melalui role/audit, bukan sekadar count source harus konstan.

Konsekuensi yang perlu disadari: retained session/audit/history, duplicate rehearsal namespaces, SQL dumps dan R2 copies terus menambah storage. Budget, monitoring dan encrypted archive diperlukan. Bila nanti muncul kewajiban penghapusan data/retention yang bertentangan, eskalasikan keputusan produk/keamanan secara eksplisit; plan ini tidak memberikan pengecualian otomatis.

## 10. Topologi database production → staging

### 10.1 Pembagian scope target

| Lokasi | Isi | Akses/write owner |
|---|---|---|
| Live DB, legacy tables | LMS yang masih dipakai learner | Aplikasi live untuk unit owner LEGACY; migration hanya baca |
| Staging DB, existing `public` | Data staging yang sekarang ada | Dipertahankan; legacy staging harus diamankan dan tidak auto-sync schema |
| Staging DB, `migration_landing` | Immutable snapshot row/version + provenance dari source terverifikasi | Restricted importer; tidak exposed ke browser/Data API |
| Staging DB, `migration_ops` | Runs, mappings, conflicts, approvals, backups, reconciliation, ownership | Restricted operator dan narrowly scoped runtime owner-reader |
| Staging DB, `v3_rehearsal_<id>` | Isolated hasil transform untuk latihan/test writes | Test-only Go deployment; side effects off |
| Staging DB, approved operational schema (default `v3`) | Clean candidate yang kelak melayani v3 | Go production-mode untuk unit yang owner V3; importer hanya unit belum cutover |
| Local restricted backup location | SQL/custom archives, encrypted object backup/manifest, checksums | Operator backup, bukan repo/CI artifacts public |

Nama namespace final dipin oleh deployment config yang divalidasi; bukan parameter dari request. Go repository/query tooling harus mendukung trusted schema configuration sejak M03. Jangan mengandalkan `search_path` yang dapat dimodifikasi caller atau schema yang dapat ditulis user; gunakan qualified identifier/strict allowlist dan safe identifier quoting.

**Pilihan baseline adalah isolasi rehearsal dengan schema terpisah**, bukan menambah `dataset_id` ke setiap domain table. Setiap target build mempunyai schema version, mapping version dan target namespace sendiri. Seluruh FK domain berada dalam namespace tersebut. Mapping ledger selalu memasukkan target namespace/build agar replay ke rehearsal tidak disalahartikan sudah masuk operational v3.

### 10.2 Preserve staging lama, pisahkan rehearsal dari calon operasional

- Backup existing staging sebelum membuat schema baru. Existing `public` tidak menjadi sasaran restore production.
- Copy production masuk landing dengan identity source yang jelas; data staging existing tetap sebagai sumber berbeda jika nanti memang ingin diimport.
- Teacher/student production yang PK-nya sama dengan data staging tidak berarti orang yang sama. Jangan upsert ke existing `public.users` berdasarkan ID/email.
- Rehearsal test yang membuat submission/nilai palsu hanya menulis namespace rehearsal. Tidak ada cleanup DELETE sesudah test.
- Candidate operasional dibangun dari snapshot/mapping approved, bukan mempromosikan dataset rehearsal yang sudah dicampur test writes. Run ulang pada namespace rehearsal baru bila perlu; retain yang lama dengan akses restricted.
- Existing staging test data **tidak otomatis menjadi data v3 nyata**. Preserve di public/backup. Import ke candidate memerlukan explicit source selection/review; default operational content berasal dari production source.
- Importer tidak boleh terus menyalin legacy snapshot ke unit owner V3 karena itu akan menimpa aktivitas baru. Status owner diperiksa ulang di dalam transaksi batch, bukan hanya saat job start.

### 10.3 Staging menjadi lingkungan operasional v3

User tetap mendapatkan target yang diminta: **v3 memakai database staging yang sekarang ada**. Namun begitu learner nyata mulai menulis ke sana, database tersebut bukan lagi playground: lock akses developer, pisahkan runtime/backup/migrator roles, production-grade monitoring/backup/restore, email/AI secrets yang benar, privacy review, kapasitas dan on-call.

Future development/test harus menggunakan local atau isolated non-operational environment; jangan menjalankan trial migrations/seed terhadap operational namespace. Jika tetap menjalankan rehearsal di project yang sama saat pilot, resource quota dan role isolation wajib; bila tidak bisa menjamin isolation/capacity, hentikan rehearsal di project itu dan gunakan local restore untuk eksperimen. Ini tidak mengubah keputusan database operasional v3 tetap target staging tersebut.

Schema separation tidak memisahkan CPU, storage, connection pool, superuser access, atau backup blast radius. M00/M16 wajib mengukur headroom untuk legacy staging + raw snapshot + beberapa builds + indexes + backups; hentikan copy sebelum storage limit.

## 11. Runbook backup wajib sebelum setiap migrasi

### 11.1 Definisi backup gate

Satu **migration run** adalah satu paket schema/data change dengan scope, checksum, snapshot, dan approval yang tetap. Boleh mempunyai chunk transaksi agar tidak mengunci DB lama. Backup wajib sebelum run pertama dan sebelum setiap run/batch baru yang dipromosikan sebagai operasi terpisah. Resume chunk hanya boleh memakai backup run yang sama bila input, target, schema, owner, dan checkpoint belum berubah; selain itu ambil backup baru.

Backup bukan per HTTP request learner. Aturan ini untuk schema migration, backfill, import, data correction, security data changes, dan tiap wave/final-delta cutover. Pekerjaan read-only inventory tidak membutuhkan migration backup.

### 11.2 Urutan operator

1. **Preflight:** pastikan source/target berbeda dan sudah verified; namespace allowlist; access read-only source; TLS peer+hostname verified; disk target backup cukup; source/target capacity sehat; tidak ada runner lain; artifact path absolute dan restricted.
2. **Protection:** secret tidak ada di command-line URL, console, commit, transcript atau shell history. Gunakan secure credential mechanism/secret store dan ACL operator-only. Catat reference/fingerprint, bukan password/token.
3. **Freeze scope:** tulis local preliminary manifest sebelum ledger DB ada. Definisikan run ID, tables/schemas in scope, schema fingerprint, mapping version, approval, expected row-count inventory, file buckets dan object inventory cutoff.
4. **Source snapshot:** gunakan consistent PostgreSQL dump untuk schema+data application scope yang lengkap. Satu exported snapshot dapat dipakai dump dan SQL inventory/hash validation di transaksi read-only yang tetap terbuka; jangan membuat export tiap table pada waktu berbeda. Monitor durasi snapshot agar tidak menahan vacuum terlalu lama.
5. **Target pre-state backup:** ambil backup existing target application schemas beserta landing/ops/operational candidate yang akan berubah. First bootstrap pun dibackup, walaupun schema target baru belum ada. Catat target concurrent writers; saat cutover candidate harus fenced/idle.
6. **Artifacts:** default buat custom-format archive sebagai canonical dump, lalu render plain SQL dari archive yang sama menggunakan `pg_restore` ke file lokal. Ini memberi `.sql` dan archive yang merepresentasikan snapshot sama, bukan dua dump independen yang mungkin berbeda.
7. **Completeness:** record scope dan exclusions eksplisit. `pg_dump` bukan backup cluster global roles/tablespaces dan bukan physical/PITR backup. Inventory grants/roles/extensions/provider-managed dependencies terpisah; jangan mengeksekusi source privileged grants/cron/foreign subscriptions pada staging tanpa review.
8. **File backup:** inventory dan secure copy R2 objects yang direferensikan, dengan object key/version bila ada, size, hash, availability, dan access policy. External GitHub/Figma URLs bukan object yang pasti dapat di-backup; catat sebagai external reference dan scope/izin archival. Jangan fetch massal tanpa egress policy/approval.
9. **Verify artifacts:** command exit success, expected sections/object count, nontruncated, SHA-256, readable archive. Backup SQL dan manifest bersifat sensitif; enkripsi at rest dan ACL, key recovery terpisah.
10. **Restore proof:** restore ke database local/isolated **baru dan kosong** dengan compatible PostgreSQL/extensions, outbound network denied, no active scheduler. Review dump SQL/objects sebelum execution; source dump dapat mengandung executable functions/triggers. Tidak restore di atas DB live/staging existing dan tidak `--clean`.
11. **Validate restore:** compare counts, key uniqueness, FK/invariant checks, normalized row hashes, numeric totals, raw source capture; verify file sample/full hash sesuai criticality. Restore readable tetapi data tidak cocok = FAIL.
12. **Approve:** manifest ditandatangani operator/reviewer, status `RESTORE_VERIFIED`, backup ID di migration run; baru buka target additive writes.
13. **After run:** reconciliation report dan post-state backup/checkpoint dibuat untuk evidence/recovery; tidak menggantikan pre-state backup. Catat durasi/size dan update capacity alerts.

PostgreSQL menyediakan consistent dump tanpa memerlukan penghentian seluruh live traffic, tetapi source DDL/perubahan schema selama export harus dicegah melalui release coordination. Detail: [PostgreSQL 17 pg_dump](https://www.postgresql.org/docs/17/app-pgdump.html), [pg_restore](https://www.postgresql.org/docs/17/app-pgrestore.html). Versi utilitas dan target restore tetap harus disesuaikan dengan server yang terverifikasi; jangan berasumsi dump dari utility major lebih baru pasti restore bersih ke server lebih lama.

Backup database Supabase tidak mencakup isi object Storage; demikian juga dump PostgreSQL LMS tidak mencakup R2 bytes. Rencana file backup harus terpisah. Referensi: [Supabase database backups](https://supabase.com/docs/guides/platform/backups).

### 11.3 Isi manifest minimum

| Kelompok | Field wajib |
|---|---|
| Identity | `backup_id`, `run_id`, `environment_label`, verified source/project fingerprint, DB/server version, tool version, timestamp UTC |
| Scope | schema/table list, exclusions + reason, schema checksum, snapshot reference/cutoff, selected migration units |
| Artifacts | private path/reference SQL+custom dump, byte size, SHA-256, encryption key reference, ACL review, object manifest reference |
| Validation | command result, schema/count/hash reconciliation, restore target fingerprint, restore start/end/result, FK/grade checks |
| Approval | operator, independent reviewer, approved operation checksum/scope, status, failure reason bila ada |

Local path rencana dapat memakai `.local-backups/<environment>/<run-id>/`, tetapi bukan janji bahwa folder/backup sukses sudah dibuat. Lokasi harus dikecualikan dari Git di repo mana pun yang menaunginya; root workspace saat ini bukan tempat aman otomatis hanya karena namanya lokal. Jangan commit file SQL, dump, CA private material, connection strings atau manifest mentah yang berisi PII.

Bila hanya plaintext `.sql` yang tersedia, izin file harus ketat dan host storage terenkripsi; jangan anggap ekstensi `.sql` aman dibagikan. Copy recovery kedua disimpan encrypted pada lokasi terpisah yang disetujui, bukan hanya disk laptop yang sama. Tidak menghapus artifacts gagal/backup lama otomatis; tandai FAILED/SUPERSEDED dan pantau storage.

### 11.4 Kondisi wajib STOP

- Source/target fingerprint sama/tidak jelas; prod connection belum dikonfirmasi.
- TLS chain/hostname gagal, credential tidak least privilege, atau secrets akan terekspos.
- Disk/DB capacity tidak cukup; dump/restore gagal; scope backup tidak mencakup table yang akan berubah.
- Schema source berubah dari adapter yang disetujui; active writer tidak terfence saat final cutover.
- File backup/external evidence belum tersedia untuk records yang memerlukan bukti utuh.
- Run checksum/mapping berubah saat resume; unresolved critical reconciliation; mode rehearsal dapat mengirim production notification.

Backup gate tidak boleh dilewati agar milestone terlihat selesai. Rollback restore destructive ke production bukan bagian runbook ini.

## 12. Algoritme copy, backfill, delta dan resume

### 12.1 Fase A — Extract tanpa mengubah production

1. Jalankan M00/M01; snapshot source yang terverifikasi menjadi input immutable dengan adapter schema fingerprint.
2. Untuk volume yang belum diketahui, baseline aman adalah full consistent snapshot + key/hash comparison; pilih subset **dependency-complete** untuk tahap awal. Jangan memakai `LIMIT 100` acak yang memutus learner/grade relations.
3. Restore snapshot ke isolated local DB untuk structured extraction jika diperlukan. Jangan mengganti nama schema melalui search/replace SQL dump; `pg_restore --schema` adalah selection, bukan general-purpose schema remap.
4. Masukkan original row representation dan required type metadata ke `migration_landing`, keyed source identity+table+PK+snapshot/hash. Preserve NULL, decimal precision, timestamp semantics, arrays, unknown enum values dan provenance.
5. Sensitive auth/API-secret fields tidak masuk normal searchable JSONB landing. Preserve original di encrypted restricted backup atau encrypted payload dengan granular access; identity importer hanya menerima minimum fields. “Copy isi database” tidak berarti membuat semua secrets source aktif dan dapat dibaca developer di staging.
6. Credential source, raw cookie/session, outbound pending jobs dan public secret settings tidak pernah diputar ulang ke runtime v3. Historical logs dapat disimpan restricted dengan provenance, bukan treated sebagai current audit of Go commands.

### 12.2 Fase B — Transform dan backfill additive

1. Buat import plan immutable: source snapshot, target namespace/build, mapping version, unit scope, dependency graph, conflict policy, expected counts; dry-run tidak menulis domain tables.
2. Resolve users/identities dahulu; classes dan metadata; enrollments/episodes dan roles; content/settings/files; submissions; gradebook; completion snapshots; attendance/permits/logbooks; reporting/archive. Actual order mengikuti FK graph, bukan urutan filename.
3. Dalam setiap bounded batch: verify backup gate dan ownership lagi; acquire runner/unit lock; read checkpoint; transform deterministik; insert source→target mapping, target revision/projection, migration audit, receipt/checkpoint dalam transaksi yang sama.
4. Uniqueness mapping minimal `(source_instance_id, source_table, source_pk, target_namespace, mapping_version)` dengan explicit source-version receipts. Batch replay dengan source hash sama tidak membuat event/enrollment/grade kedua.
5. Source berubah → landing revision baru → target revision baru **hanya jika unit masih owner LEGACY/shadow**. Jangan overwrite target changes yang dibuat tester; pisahkan namespace rehearsal/candidate sejak awal.
6. Conflict masuk review queue dengan category, source references, proposed options dan reviewer decision. Quarantine bukan silently skip; blocked domain tidak published.
7. Jangan mengirim notifications atas historical imports. Outbox jika diperlukan ditandai migration/suppress dan consumer tidak membuat external side effects.
8. Validate setiap chunk dan keseluruhan unit. Beri status BACKFILLED/VALIDATED hanya dengan counts/constraint/domain reconciliation lulus.

### 12.3 Fase C — Delta saat live masih berjalan

Legacy tidak menyediakan revision/change log yang andal untuk semua tabel. Jangan mengasumsikan `updatedAt` mencakup role arrays, grade overwrite, delete, startup repair, dan semua dependent rows. Timestamp watermark saja berisiko kehilangan perubahan.

Default tahap awal: consistent snapshot baru → compare all in-scope source keys+content hashes terhadap snapshot sebelumnya → ingest changed rows/absence observations → retransform/reconcile. Sebuah source row yang tidak lagi terlihat dicatat `SOURCE_ABSENT` beserta snapshot evidence, **bukan DELETE target** atau bukti pasti user menghapusnya. Bedakan filter scope berubah dari genuine absence.

Untuk volume besar, CDC/logical decoding dapat dipertimbangkan kemudian, tetapi memerlukan privilege, replication slots/publication/WAL retention dan mungkin source changes. Itu scope/approval terpisah, tidak dijalankan hanya karena copy data diizinkan. Logical replication langsung juga tidak otomatis memetakan schema Class v3. M15 harus mengukur apakah snapshot/delta cukup untuk maintenance window. Jika tidak cukup, tahan cutover sampai strategi delta yang teruji tersedia. Referensi mekanisme: [PostgreSQL logical replication](https://www.postgresql.org/docs/current/logical-replication.html).

### 12.4 Fase D — Final delta dan fence

Final source snapshot dilakukan **setelah** legacy writes untuk seluruh unit/dependencies dihentikan dan transaksi/jobs in-flight drained. Ambil backup fresh source+target sebelum apply final delta, import, reconcile, lock ownership, switch route, baru open v3 writes. LSN/timestamp snapshot dicatat sebagai evidence; jangan menganggap LSN yang dicatat tanpa consumer CDC menjamin semua event sudah diterapkan.

Bagian 16 menentukan global identity/shared dependency, maintenance UX, fencing, dan fallback setelah first v3 write.

### 12.5 Resume dan corrective runs

- Checkpoint hanya committed bersama target batch. Crash sebelum commit → retry; sesudah commit → verify receipt lalu lanjut.
- Two runners untuk scope sama ditolak/serialized. Idempotency menggunakan DB unique keys dan lock, bukan hanya in-memory mutex.
- Mapping logic berubah → mapping version/run baru, fresh backup, preview affected records, reviewer sign-off. Jangan memanipulasi ledger lama agar terlihat satu run sukses.
- Partial import yang tidak valid tetap disimpan restricted. Operational reader tidak melihat build/unit yang belum approved; mark INVALID/QUARANTINED, buat correction revision atau build baru.
- Correction terhadap data yang sudah operational memerlukan owner-aware Go command/revision; importer tidak diberi bypass untuk menimpa live v3 grades.

## 13. Pemetaan legacy → v3 dan reconciliation

### 13.1 Matriks seluruh tabel application yang terinventarisasi

Inventory awal mencakup 20 tabel `public` berikut; production inventory dapat berbeda dan M00 harus mendeteksi tambahan, bukan mengabaikannya. Entity source berada di `backend/src/{users,classes,attendance,audit}/entities/`; table `session` juga dibuat melalui `backend/src/main.ts`.

| Legacy source | Mapping target | Aturan preservation / ambiguity |
|---|---|---|
| `users` | User + identity/account history; profile terpisah dari assignment | Preserve source ID; role/status/program/batch hints tidak otomatis permission; secret/session material restricted |
| `programs` | Optional category/program metadata; legacy registry preserved | Same-name tidak auto-merge; program bukan prerequisite Class |
| `batches` | Legacy cohort context + optional Class cohort label/date defaults | Preserve `programId`, `includedProgramIds`, schedule/status; tidak memaksakan satu active batch; schedule config reviewed |
| `classes` | Standalone Class; legacy mentor→initial reviewed Teacher assignment | 1:1 default, names not keys; old program/batch links stored provenance; no automatic merge/split |
| `enrollments` | Enrollment aggregate + initial episode and origin edges | All raw duplicates retained; multiple source rows may map to one aggregate after explicit resolution |
| `program_competencies` | Versioned competency/rubric definition library | Program relation historical metadata; no academic score attached globally without Class context |
| `competencies` | Competency definition + Class binding where evidence exists | Mutable names/string references resolved by source IDs/evidence; ambiguity blocked |
| `materials` | Class section/activity+content revision+file references | Preserve raw HTML/URL restricted; safe operational rendition; infer section only with reviewed rule |
| `assignments` | Activity+assignment settings/rubric revision | Snapshot deadlines/weights/rubric JSON; unknown/cyclic rubric cannot publish |
| `submissions` | Submission aggregate/revisions; separately imported grade/suggestion evidence | Preserve link, status, actor/time, feedback; do not manufacture absent attempts/history or treat all AI values as human-approved |
| `competency_scores` | Class grade item/revision or legacy unallocated academic evidence | Source key lacks Class context; resolve one learner episode only with evidence, never broadcast to every Class |
| `rubrik_assessments` | Rubric version/criteria/validated graph edges | Preserve dangling/cyclic source graph in landing; target invalid structure not published |
| `rubrik_assessment_scores` | Grade criterion evidence/revision or legacy unallocated record | Shared/global score ambiguity reviewed; numeric transformations tracked |
| `attendances` | Class meeting attendance only if meeting/Class mapping proven; otherwise legacy batch-context evidence | No automatic duplication across all Classes; preserve conflicting statuses/SP indicators |
| `permission_requests` | Historical permission evidence + reviewed decision mapping + private files | Legacy auto-applied attendance is not proof of human approval; import `legacy_decision_unknown` evidence, no invented reviewer |
| `holidays` | Calendar dates/source versions + optional Class exception bindings | Global holiday date is not automatic absent/exempt status for every Class; timezone/region policy explicit |
| `mentor_async_days` | Class/group calendar exceptions when assignment proven | Existing mentor+date may lack Class; keep unallocated until mapping, never global denominator reduction |
| `logbooks` | Entry/revision/review tied to approved Class period/episode or legacy batch archive | Source student+batch+month may span Classes; preserve original statuses and missing chronology |
| `audit_logs` | Restricted legacy audit archive with source metadata | Not relabeled as transactional v3 domain audit; historic missing actor remains unknown |
| `session` | Restricted backup/archive only; fresh v3 sessions | Do not replay legacy SID, cookies, auth payload or pending expiry into v3; no DELETE cleanup |

Selain row database, inventory mencakup R2 objects, locally archived audit/health files, deployed cron config dan integrations. Table count lengkap bukan berarti seluruh data operasional telah ter-backup.

### 13.2 Keputusan mapping yang tidak boleh otomatis

- **Class merge:** source Class per mentor mungkin tampak seperti group dari Program+Batch sama, tetapi kesamaan tersebut bukan bukti mereka harus digabung. Default preserve identity. Bila produk memilih merge, buat new Class dan explicit transfer/equivalence mapping, preserve semua source Class/content/learner records, resolve duplicate assignments/grades sebelum publish.
- **Teacher scope:** legacy specialization Professional/UIUX yang memberi akses luas tidak otomatis diterjemahkan menjadi Teacher semua Class. Data owner mengonfirmasi actual staff assignments; existing learner mentor association preserved sebagai optional mentoring relation.
- **Identity collisions:** email sama lintas environment/provider bukan bukti aman untuk account merge. Verifikasi person/provider subject, keep separate origin map, record reviewed linking. Jangan overwrite password/status karena “email sudah ada”.
- **User status:** INACTIVE/SUSPENDED/GRADUATED tidak semuanya berarti DISABLED. Security disable dipisah dari completion/SP; ambiguity membutuhkan review agar tidak membuka akun berbahaya atau menutup alumni sah.
- **Program-global grades:** jika tidak dapat mengetahui Class/episode asal, tampilkan sebagai legacy historical academic evidence di scoped archive, bukan grade baru Class. Class terkait tidak dapat cutover dengan outstanding authoritative grade ambiguity tanpa explicit product-approved historical-only disposition.
- **Implicit defaults:** fallback score 65/AI75 bukan evidence nilai autentik. Preserve source raw and observed rendered outcome, flag calculation provenance, minta academic approval sebelum perubahan tampilan/outcome.
- **Dates:** legacy timestamp tanpa timezone perlu evidence deployment/database/business timezone. Jangan menganggap semua timestamp adalah Asia/Jakarta lalu menggesernya dua kali. Preserve raw string/type dan transformation rule.
- **Missing FK/files:** retain orphan record with source locator, classify and review; jangan membuat student/teacher fiktif berhak login agar FK “beres”. Placeholder actor historical boleh hanya non-login reference yang jelas unknown dan disetujui, bukan user dengan password default.

### 13.3 Reconciliation report wajib

| Lapisan | Validasi | Kriteria lulus |
|---|---|---|
| Raw capture | Source keys/counts/hash sesuai consistent snapshot | 100% source rows in scope accounted, termasuk duplicates/orphans |
| Mapping | Mapped + archived-only approved + quarantined = source records | Tidak ada silent skip; quarantine kritis = unit blocked |
| Identity/access | Person mapping, staff assignment, active episodes, account restrictions | Zero unauthorized grants; all selected learners accounted |
| Referential | Parent existence/same-Class checks, FK validation | Zero unexplained orphan/cross-Class learner-state |
| Content | Count/order/revision/attachment/link access | Source meaning preserved; unsafe content restricted with disposition |
| Submission | Row/revision receipt, learner/assignment/episode, file hashes | No lost submitted work; chronology unknown disclosed |
| Academic | Raw scores, grade items, scale/weights, observed legacy display, new computation | Every difference explained and reviewed; no manufactured passing result |
| Attendance/logbook | Dates/status/count/proof, Class mapping, old SP indicators | No invented presence/absence/approval or cross-Class duplication |
| Files | Object counts/size/checksum/access policy | Critical evidence available/private; missing critical proof blocks affected unit |
| Isolation | Existing staging pre-state unchanged, secrets/external actions blocked | No unrelated staging overwrite, no emails/AI calls to production recipients |
| Incremental | Snapshot delta receipts and owner checks | Rerun deterministic; all final changes accounted before ownership switch |

Acceptable differences harus mempunyai machine-readable reason code, source/target refs, reviewer, timestamp dan policy version. “Angkanya hampir sama” atau overall total yang cocok bukan bukti per-student grades benar.

## 14. Pelacakan status implementasi, migrasi dan ownership

### 14.1 Tiga status yang tidak boleh disatukan

| Dimensi | Menjawab | Contoh status |
|---|---|---|
| Implementation package | Apakah modul Go/hotfix/contract sudah dibuat dan diverifikasi? | NOT_STARTED, IN_PROGRESS, REVIEW, TESTED, MERGED, DEPLOYED |
| Migration run/unit | Apakah data sudah dicopy, dipetakan, dan direkonsiliasi? | DISCOVERED, BACKUP_PENDING, RESTORE_VERIFIED, EXTRACTED, MAPPED, BACKFILLED, VALIDATED, FAILED, QUARANTINED |
| Traffic/write ownership | Siapa saat ini melayani dan boleh menerima perubahan? | LEGACY, SHADOW_ONLY, FENCED, CUTOVER_READY, V3, HOLD_READ_ONLY |

`MERGED`/`DEPLOYED` tidak berarti migration complete. `VALIDATED` tidak berarti learner sudah memakai v3. `V3` tidak berarti seluruh Class lain sudah dipindahkan. UI migration dashboard nanti harus menampilkan ketiganya, plus timestamp dan evidence.

### 14.2 Ledger yang harus dibuat di tahap implementasi

| Registry/table usulan | Minimum data |
|---|---|
| `environment_registry` | Verified source/target fingerprint, role scope, schema fingerprint, verification evidence, approved use, no plaintext credentials |
| `backup_manifests` | Backup ID, immutable artifact/checksum references, scope, restore evidence, operator/reviewer, state |
| `migration_runs` | Run ID, source snapshot, target build/namespace, mapping/schema version, scope, migration file hashes, backups, started/finished/status/error, parent run |
| `migration_units` | Unit ID, included Class IDs, dependency reason, status, source/target counts, owner, owner epoch, fence state, approvals |
| `origin_mappings` | Source identity/table/PK, target namespace/entity/UUID, source hash/version, mapping version, resolution provenance |
| `row_receipts` / `batch_checkpoints` | Committed source versions, batch bounds, transformed counts, checksum, tx receipt, resume marker |
| `migration_conflicts` / `conflict_decisions` | Type/severity, affected entities, preserved evidence, options, append-only reviewer decision/history |
| `reconciliation_results` | Test/version, expected/actual, allowed difference reason, pass/fail, source/target refs, reviewer |
| `ownership_events` | Previous/new owner+epoch, fence/drain evidence, final snapshot/checkpoint, switch actor/reason/time, first v3 write receipt |

Ledger initial bootstrap memerlukan backup sebelum schema dibuat; local manifest menjadi evidence awal kemudian diregistrasikan ke DB. Ledger tidak boleh berisi password, sessionID, private signed URLs atau full medical content.

### 14.3 Template tracking paket di Git

Untuk setiap Mxx simpan ringkasan sanitised berikut di dokumen status nanti; tabel rencana Bagian 8 tetap referensi scope, bukan log raw migration:

```text
Module ID / module-owned CP ID (Mxx-CPnn) / historical CP alias:
Canonical deliverable IDs supported / remaining clause per D:
CP scope status / verified local parts count (not effort percentage):
Owner / reviewer:
Priority / complexity:
Dependencies / business decisions:
Implementation status:
Branch / commit SHA / deployed artifact SHA:
Tests / security review / contract version:
Schema migration IDs / checksum:
Migration run IDs / target namespace / scope:
Pre-migration backup IDs / restore evidence refs:
Reconciliation summary / unresolved conflicts:
Ownership before -> after / epoch:
Fallback readiness / known limitations:
Next action / blocker / verified-at:
```

Nilai awal selain baseline yang sudah dibuktikan adalah NOT_STARTED/UNKNOWN, bukan checkbox selesai. Tidak membuat sample Class/backup ID yang terlihat seperti real run.

### 14.4 Exit gate status data

- **DISCOVERED → RESTORE_VERIFIED:** source/target verified, scope selected, backups tersedia dan restore benar-benar lulus.
- **RESTORE_VERIFIED → EXTRACTED:** immutable snapshot/raw receipts lengkap; schema adapter sesuai.
- **EXTRACTED → MAPPED:** semua mapping termasuk ambiguous records mempunyai resolved atau explicit quarantined status.
- **MAPPED → BACKFILLED:** target transaction receipts dan FK/uniqueness checks pass untuk scope.
- **BACKFILLED → VALIDATED:** business reconciliation, negative access, files dan accepted differences disetujui. Unresolved critical data/security conflict menghalangi transisi.
- **VALIDATED → CUTOVER_READY:** final fence/drain/delta/reconcile, frontend/session readiness, ops/security/data-owner approval dan fallback rehearsal lulus.
- **CUTOVER_READY → V3:** owner epoch/routing activated dan first-write monitoring siap. Tidak boleh skip gate hanya karena code sudah selesai.
- **Apa pun → FAILED/QUARANTINED/HOLD_READ_ONLY:** simpan sebab/evidence, tidak menghapus rows atau mundur mengedit ledger sejarah.

### 14.5 Source of truth pelacakan M dan QA checkpoint

- Master plan ini mengatur scope bisnis, invariant dan dependency. `docs/modules/Mxx.md` memegang daftar CP/target D/exit dan next action setiap M. `docs/MODULE_CHECKPOINT_MAP.md` adalah ringkasan lintas M dan alias histori; dashboard menghubungkan status penuh serta evidence. Jangan membuat definisi CP berbeda di dua dokumen.
- Handoff wajib menyebut M, CP milik M, target D, hasil/run/hash, sisa untuk closure M dan next CP. Tampilkan jumlah bagian teruji, canonical D verified, code/test coverage, migration/ownership secara terpisah. Tidak memakai rata-rata CP atau merata-ratakan coverage package sebagai completion proyek.
- QA dijalankan sesudah setiap CP implementasi, lalu required regression/closure per M. Expected cases/gates dibekukan sebelum coding, failure history dipertahankan. Bukti existing dengan nama CP lama dapat direferensikan selama scope/hash cocok; tidak mengganti ID test/receipt sekadar menyesuaikan label baru.
- M03 kini memiliki registry fail-closed module→CP→case→D dan flag lab `--worker-journey`; case ID lama tetap dipakai. Package runner belum menerima semua M04–M19, sehingga suite modul lain tetap harus dibekukan/diadaptasi sebelum klaim completion; tulisan status tidak men-trigger QA/deploy.
- `ImplementationPlan.MD` ada di root workspace, di luar Git root backend Go. Peta/brief/status backend ada di repository `backend-v3/backend-go` supaya dapat direview/publish bersama source; jangan memindahkan root Git kembali atau menyalin master plan diam-diam menjadi authority kedua.

## 15. Verifikasi teknis, bisnis, keamanan dan UX readiness

### 15.1 Test matrix wajib

| Area | Skenario minimum | Expected |
|---|---|---|
| Invite/reset | Wrong/expired/purpose-mismatch/reused token; two simultaneous consume | Ditolak atau satu sukses saja; existing password tidak diganti via email-only |
| Session | Fixation, copied SID, query/Bearer fallback, privilege/account change | Session regenerate/revoke; old credential tidak diterima; no secret logs |
| CSRF/CORS | Cross-site form, unsafe method tanpa token, lookalike origin/subdomain | Mutasi ditolak; legitimate deployed frontend tetap login |
| Identity | Student self PATCH role/batch/status; two admins demote each other | Authority fields rejected; last-admin invariant survives concurrency |
| Context | Teacher A, Teacher B, Teacher+Student, facilitator, site admin | Full capability matrix server-side; self-grade disallowed |
| BOLA | Foreign Class/activity/submission/grade/file IDs dan mismatched nested URL | Tidak ada data leak/mutation; consistent denied/not-found policy |
| Class | Nama only draft, cohort kosong, many active Classes, shared label | Tidak ada global active-batch gating atau cross-Class status effect |
| Enrollment | Concurrent add/remove/rejoin, duplicate import, ended participant | History preserved, one active episode default, correct access change |
| Content | Double encoded HTML, unsafe SVG/embed/URL, publish revision | No script execution; old submission revision remains correct |
| Submission | Double click, timeout retry, concurrent revisions, cutoff timezone | Exactly one command effect; deterministic receipt/late status |
| Grades | Student import, foreign teacher, self grade, invalid weight/graph, concurrent graders | Deny/validation/conflict; no partial authoritative grade |
| Grade integrity | NULL vs 0, no work, legacy65, provider error75 | No fabricated grade; difference evidence explicit |
| Completion | Complete without grade, grade without completion, regrade/certificate | Rules separate; old credential immutable; supersession audited |
| Attendance | Forged requester, permit pending, conflicting review, missing record | No auto-approval or accidental absent; scoped atomic decision |
| SP/calendar | Mentor async date foreign Class, attendance correction | No global denominator/status side effect, no duplicate SP |
| Logbook | Configurable periods, reassign mentor, out-of-period edit | Correct context/deadline/history; no four-month assumption |
| File | MIME spoof, oversized/base64, malware/scan timeout, foreign proof | Quarantine/private/limit; no active-content/public fallback |
| Egress/AI | Redirect/DNS rebinding/private IP, provider outage, prompt injection | No internal network fetch; no AI final grade; bounded cost |
| Jobs | Crash before/after commit/provider, repeat delivery, dead-letter | Atomic outbox, dedupe, visible retry state, no cleanup loss |
| API | Unknown JSON fields, invalid enum, SQL injection payload, pagination boundary | Typed validation/parameterized SQL, bounded response, consistent errors |
| DB | Deadlock/serialization retry, lock timeout, pool exhaustion | Bounded retry with jitter, rollback complete, no external call within tx |
| Migration | Same source/target, wrong CA/schema, missing backup, failed restore | Stop before target write |
| Migration replay | Crash mid-chunk, rerun, changed hash/mapping, two runners | No duplicate canonical state; checkpoint and original evidence intact |
| Historical data | Duplicate/orphan/shared-score/missing-file, moved Class/batch | Preserved and classified; no silent guessed repair |
| Isolation | Production copy in staging, old staging rows, secret/jobs/proof access | No overwrite, no outbound production email/AI, no secret replay |
| Cutover | Direct legacy API, stale tabs, pending job, in-flight submission | Enforced fence/drain; one owner; retry safe and visible |
| Failure after cutover | First v3 write then outage | Hold/read-only+forward fix or proven projection, never stale-data route rollback |
| Recovery | Fresh restore from latest backup and object manifest | Agreed RPO/RTO achievable and actual measured result recorded |

Run Go unit tests, integration tests terhadap PostgreSQL compatible, `go test -race` untuk applicable codepaths, contract tests, migration fixtures dan deployment smoke checks. In-memory mocks tidak membuktikan FK/locking/transaction atau RLS/grants benar. Test security memakai isolated authorized environments; jangan melakukan destructive/exploit experiments terhadap murid live.

### 15.2 UX tertinggi: target backend, bukan klaim otomatis

1. **Sedikit langkah Admin:** create Class tanpa setup Program/Batch; Add Teacher/Student satu command dari perspektif UI; bulk import preview jelas dan dapat diulang.
2. **Jelas scope-nya:** context Class aktif dan capability response; tidak ada requirement mengenal istilah cohort untuk mengajar.
3. **Tidak kehilangan pekerjaan:** durable submission receipt, draft/revision recovery, conflict feedback, stable pagination, meaningful job status.
4. **Tidak ada hasil palsu:** ungraded/pending/failed ditampilkan eksplisit; tidak menyembunyikan error AI dengan nilai lulus.
5. **Respons cepat terukur:** query scoped/indexed, no N+1, read projections yang versioned, bounded background work. Go tidak menghapus bottleneck database/storage/provider.
6. **Migrasi transparan:** announce planned short read-only window, tampilkan status submission/retry, support route ketika Class sudah pindah; no login loops/broken old links.

Performance proposal awal untuk divalidasi M14/M16: p95 simple authenticated read ≤300 ms dan normal DB mutation ≤500 ms pada beban yang disepakati, diukur pada API tanpa network client/file transfer/provider wait. Ini **target diskusi, bukan hasil benchmark atau SLA yang dijanjikan**. Tetapkan user concurrency, data volume, warm/cold cache, error rate, p99, pool usage dan location pengukuran sebelum acceptance.

Untuk import/file/AI, ukur throughput, queue age dan completion/error rate terpisah; jangan memaksa semua masuk target 500 ms. Authorization dan account revocation tidak boleh stale karena caching tanpa invalidation/versioning. Grade/release response setelah mutation harus read-your-writes meski dashboard projection asynchronous.

### 15.3 Observability dan operational gates

- Metrics: request latency/error by endpoint, policy denials, login failure/lock, queue lag/retry, connection pool utilization, long queries/locks, missing file, grade conflict, migration throughput/conflict, owner fence violation.
- Logs: correlation ID, sanitized domain IDs/action/result, namespace/environment tag; no raw Cookie/Authorization/body secrets, medical contents atau SQL parameter dumps.
- Audit: privileged/academic transition atomik; sensitive file/grade export read events; immutable history dan centralized restricted copy, no archive-then-delete.
- Alerts: Critical auth exposure, impossible cross-Class FK/policy mismatch, unauthorized old-owner writes, backup/restore failure, storage pressure, prolonged migration hold, provider-cost spikes.
- Runbooks: secret revocation, compromised session containment, restore-to-isolated, file privacy incident, stuck import/retry, grade correction, fence/rollback, capacity incident.

## 16. Cutover bertahap tanpa kehilangan data

### 16.1 Tentukan unit migrasi berdasarkan dependency nyata

Satu Class adalah target produk, tetapi belum tentu unit teknis yang bisa dipindahkan sendirian. Legacy score dapat program-global, attendance/logbook batch-scoped, user profile global, class release flag dan mentor assignment bisa saling terkait. M15 harus membangun dependency graph berdasarkan records/queries aktual.

Default unit adalah satu Class **ditambah seluruh dependent records**. Jika beberapa Class berbagi authoritative state yang tidak dapat dipisah dengan mapping atau routing yang terbukti, gabungkan menjadi satu technical migration unit. Ini bukan mengembalikan Batch sebagai syarat bisnis v3; hanya strategi perpindahan sementara.

Dilarang membagi write traffic acak 10% ke Go dan 90% ke legacy untuk Class yang sama. Shadow read boleh dibandingkan dengan actor-aware filtering dan tanpa side effect; canonical write selalu satu owner. Pendekatan incremental ini sejalan dengan [strangler-fig migration pattern](https://docs.aws.amazon.com/prescriptive-guidance/latest/cloud-design-patterns/strangler-fig.html), tanpa mewajibkan microservices.

### 16.2 Identity dan shared entities selama dua runtime

Default transisi: **hardened legacy identity tetap menjadi authority untuk existing accounts hingga explicit global identity cutover**, sedangkan Go menjadi owner Class domains yang telah dipindah. Go identity row adalah mapped local representation, bukan password/status authority kedua yang bisa diedit sendiri.

Kontrak yang harus dibuat dan diuji pada M04/M17:

- Gunakan trusted server-to-server identity bridge atau standard OIDC broker dengan issuer/audience/signature/expiry/state/nonce checks. Untuk bridge custom, assertion short-lived dan single-use (`jti`), exchanged menjadi **session Go baru**, bukan raw legacy session ID.
- Bridge hanya membuktikan verified identity/account state; Class role/capability tetap berasal dari Go contextual assignments. Legacy global Mentor tidak otomatis Teacher di semua Class.
- Global profile/account disable/invite commands selama fase ini dirutekan ke satu hardened legacy identity owner. Go tidak mengubah password/email/account status secara independen; hasil sync ke Go punya version/audit.
- Account disabled/revoked di authority harus menutup Go access dalam batas yang eksplisit. Privileged operations memerlukan authoritative state/fresh revocation check; bridge unavailable → fail closed untuk operasi sensitif. Jangan menyatakan revocation immediate jika hanya mengandalkan cache TTL.
- Global identity sync merupakan unit terpisah yang owner-nya LEGACY, sehingga bukan alasan importer menimpa grade/submission Class owner V3.
- User yang mempelajari Class legacy dan v3 mendapat identity consistent tetapi dapat mempunyai session cookies issuer-specific. Jangan reuse cookie name/secret lintas environment.
- Pemindahan authority identity ke Go dilakukan sekali sebagai wave global tersendiri: fence account writes, verified snapshot/delta, issuer/session transition, legacy client integration, revoke/reissue aman, acceptance semua Class. Raw copied sessions tidak dipertahankan.

Jika bridge yang aman belum siap, pilihan sementara adalah explicit login ulang ke v3 dengan verified provisioning dan global account lifecycle yang tetap tersinkron secara teruji. Jangan membuka pilot multi-runtime jika disable/identity collision belum dapat ditangani. Detail broker vs custom bridge adalah ADR M04; tidak diam-diam menambahkan Supabase Auth rewrite sebagai requirement.

### 16.3 Runbook satu wave

| Tahap | Tindakan | Bukti untuk lanjut |
|---|---|---|
| 0. Select | Pilih unit, data owner/teacher reviewer, feature use, historical/pending work | Dependency list, no unsupported active feature, capacity forecast |
| 1. Ready | Minimal dua rehearsal, frontend/session/old-link readiness, support briefing | Security, QA, academic, ops sign-off; fallback drill |
| 2. Announce | Informasi jendela read-only dan cara konfirmasi submission | Learner/Teacher communication dan contact support |
| 3. Fence | Tutup seluruh legacy writes untuk unit, UI serta direct APIs/admin/jobs/imports | Persistent fence marker, unauthorized old route test, queued job hold |
| 4. Drain | Tunggu transaksi/fetch/grade/import in-flight selesai atau cancel safely | Zero in-flight unit writes, final command receipts, no unacknowledged hand-in |
| 5. Backup | Source+target fresh verified backup dengan scope final | Backup IDs/restore proof lulus; owner belum berpindah |
| 6. Final delta | Apply final changed source snapshot, files, identity dependencies | Run receipts, source cutoff, no source schema drift |
| 7. Reconcile | Per-learner work/grade/access checks, files, counts/hashes | Zero unexplained loss/leak; difference approvals |
| 8. Switch owner | Record epoch and owner→V3; change routing while writes still fenced | Consistent control-plane state; old writer remains denied |
| 9. Open | Read smoke then open Go write for approved unit | Go health/policy/owner check, submit+grade safe smoke, alert ready |
| 10. Observe | Track first Go write, errors, support cases, stale clients, audit | Acceptance window agreed, no old-owner writes, incident log |
| 11. Close wave | Record migration verified/deployed/owner status separately | Final report+post-state checkpoint/backup, approval wave berikutnya |

Fence tidak cukup berupa maintenance banner atau DNS change. Semua entry point legacy yang masih bisa dijangkau harus menolak unit tersebut, termasuk cron, direct API, admin scripts dan background imports. Untuk path global yang tidak dapat menentukan unit, fence seluruh dependent scope sementara; jika tidak bisa dijamin, jangan membuka v3 writer.

Tidak ada transaksi atomik otomatis antara live DB, staging DB dan reverse proxy. Karena itu switch menggunakan **periode keduanya fenced**: legacy gate dibuat persisten dan diverifikasi, drain, final copy, target epoch committed, routing switched, kemudian hanya Go dibuka. Jika salah satu tahap gagal, tetap HOLD_READ_ONLY; jangan mengandalkan best-effort dual updates sebagai atomic switch. Runtime menolak write saat ownership/control-plane tidak dapat diverifikasi.

Old tabs menerima structured `CLASS_MOVED`/maintenance response dan safe retry guidance, bukan silent drop. Idempotency receipt harus dapat dikenali melewati boundary bila retry command sebelum fence bisa sudah committed di legacy; bila legacy tidak memiliki key tersebut, reconcile source receipt sebelum mengizinkan retry, jangan mengasumsikan gagal hanya karena browser timeout.

### 16.4 Fallback/rollback yang aman

| Kondisi kegagalan | Tindakan yang diperbolehkan | Yang tidak boleh |
|---|---|---|
| Sebelum ada target learner writes | Hold; jika source masih current dan fence/routing konsisten, kembali LEGACY dengan event/epoch baru | Delete imported rows atau restore overwrite staging |
| Setelah target writes ada, bug terbatas | Fence affected commands, keep reads jika aman, forward fix/additive correction | Mengarahkan user ke legacy stale seolah tidak ada write baru |
| Setelah target writes ada, Go tidak available | HOLD_READ_ONLY/maintenance, recover Go from immutable artifact dan verified state | Reset target ke backup lama sehingga submission baru hilang |
| Perlu benar-benar kembali ke legacy | Hanya setelah semua v3 writes dapat direpresentasikan non-lossy, reverse projection teruji, backup, approval, reconciliation, epoch switch | Menganggap v3 feature baru pasti muat di schema lama |
| Model v3 tidak representable di legacy | Tetap hold, forward fix atau temporary safe read/export; pertahankan data | Menghapus Class/role/history baru agar “kompatibel” |

Backup adalah recovery substrate, bukan izin untuk menghilangkan transaksi setelah backup. RPO untuk approved cutover adalah **tidak kehilangan committed academic writes**; maintenance duration/RTO nyata harus diukur dan disetujui, tidak dijanjikan zero downtime tanpa evidence. Restore dilakukan ke isolated new target lalu rekonsiliasi, bukan destructive in-place restore atas database yang sedang dipakai.

### 16.5 Frontend gate tetap nyata walau dikerjakan belakangan

Backend-first berarti kontrak Go+domain+rehearsal dapat selesai tanpa UI baru. Itu **tidak berarti** frontend legacy otomatis compatible dengan Class-centric routes, sessions, role switching dan error semantics. M17 menunggu fase frontend berikutnya atau compatibility layer yang benar-benar lulus contract/end-to-end tests.

Minimal readiness nanti: login/cookie/CSRF, Class selector, add Teacher/Student, content/submit/grade/attendance/logbook sesuai use, owner-aware links, error/retry/job status, no token localStorage, correct private file flow. Tidak membuat komponen atau halaman tersebut dalam pekerjaan dokumen ini.

## 17. Strategi Git, branch `be-v3`, review dan push

### 17.1 Kondisi saat ini dan batas aman

- `ImplementationPlan.MD` dibuat di root workspace sesuai permintaan. Root workspace berisi beberapa checkout, bukan otomatis satu repository yang dapat dipush bersama.
- Reference `be-v3` pernah dibuat lokal. Pengguna telah mengosongkan implementasi lama dalam `backend-v3`; pemeriksaan akhir menemukan direktori `backend-go` dan `frontend-v3` di dalamnya. Direktori tersebut tidak dibuat/diubah oleh pekerjaan dokumen ini dan harus dipertahankan. Isi/index/worktree perlu diverifikasi ulang pada kickoff. Jangan restore file TypeScript yang dihapus pengguna atau staging seluruh deletion tanpa review.
- Tidak membuat branch tambahan, scaffold, commit atau push sebagai efek samping pembuatan dokumen ini.
- Sebelum implementation pertama: periksa `git status`, branch/worktree metadata, remote dan base SHA; jangan `git reset --hard`, `git checkout --`, atau `git add -A` untuk “merapikan” worktree user.

### 17.2 Target repository dan cara menjaga riwayat

Default pengerjaan backend Go berada pada branch **`be-v3` dalam repository backend**, dengan worktree `backend-v3` yang sudah disiapkan pengguna. Branch live/master dan staging legacy tetap dapat dibuild/deploy. Replacement file tree pada branch Go hanya dilakukan saat implementasi diotorisasi, dengan explicit file list dan review; TypeScript legacy tetap ada dalam riwayat Git/branch legacy, bukan dipakai sebagai runtime Go.

Struktur Bagian 5 adalah struktur relatif service Go. Bila direktori `backend-v3/backend-go` yang disiapkan pengguna dipakai sebagai service root, seluruh `cmd/`, `internal/`, `go.mod`, dan pipeline Go ditempatkan relatif terhadap direktori itu; bukan membuat dua backend Go sekaligus. `frontend-v3` tetap di luar scope implementasi backend. M03 mengonfirmasi layout sebelum membuat file apa pun.

Dokumen root menjadi sumber rencana pada tahap diskusi. Saat mulai commit ke backend repository, sepakati perpindahan canonical plan ke root repository branch `be-v3` dan dokumentasikan reference dari workspace; jangan memelihara dua file editable yang bisa berbeda tanpa aturan sync. Tidak memindahkan file sekarang.

Hotfix kode security untuk live berada pada branch hotfix yang berasal dari deployed baseline di repo backend/frontend masing-masing, lalu dirilis terpisah. Paket `be-v3` mencatat linked commits/deployment evidence dan menerapkan prinsip perbaikan di Go. Frontend merupakan repo lain; push backend tidak otomatis membawa frontend security patch.

### 17.3 Prosedur per paket

1. Pilih Mxx dan subtask scoped, dependency/gate; record owner, reviewer, contract dan migration impact.
2. Implementasi hanya scope itu; update package status dan ADR jika keputusan berubah.
3. Untuk schema/data changes, commit SQL/tool/tests dan **sanitised metadata**; jalankan DB changes nanti sebagai approved run, bukan otomatis saat `git push`.
4. Review explicit diff dan no-delete/security/secret checks; jangan staging unrelated user changes.
5. Commit dengan package ID; test evidence disertakan. Paket Large boleh beberapa commit yang coherent.
6. Push ke `be-v3` sesuai permintaan implementasi berikutnya. CI default build/test/scans; **tidak auto-deploy live, tidak auto-run migration, tidak auto-copy production**.
7. PR/review gate untuk protected branch atau integration policy yang disepakati. Catat commit SHA, review, actual deployment, migration run dan ownership state terpisah.
8. Security patch yang urgent dapat released lebih dulu di legacy tanpa menunggu seluruh package Go; tetap ada linked evidence agar tidak hilang dari roadmap.

Branch push success bukan bukti database compatible. Merge tidak boleh membuka production secret kepada CI atau memicu TypeORM synchronize dari legacy staging workflow.

## 18. Roadmap prioritas, milestone dan risiko

### 18.1 P0 — Critical: lindungi pengguna/data lebih dulu

- M00 verified environment/deployed baseline; source/target collision blocker diselesaikan.
- M01 backup/restore capability, encryption/ACL, no-delete gate, source read-only dan target access separation.
- M02 disable account takeover/unauthorized grade imports; session/XSS/CSRF/BOLA containment; private proof/SSRF controls; disable auto-DDL/repair/deletion/cleanup; credential rotation.
- M03/M04 baseline Go security dan role policy sebelum memberi akses data nyata.
- M15/M16 data preservation, reconciliation dan restore proof sebelum rehearsal/cutover dianggap lulus.

**Exit:** tidak ada unmitigated Critical path pada deployment yang in scope; backup terbaru dapat direstore; copy source/target valid; no-delete dan no-external-side-effect guard terbukti. Menulis checklist ini tidak menyatakan exit sudah tercapai.

### 18.2 P1 — High: core Class-centric dan perpindahan aman

- M03–M10: Go core, Class/enrollment/context, content/submission/grade/completion.
- M11/M12: attendance/SP/izin/logbook jika dipakai Class yang akan dipindah.
- M13 durable notification/integration safety, M14 API contract dan actor-specific reports.
- M15/M16 repeated production snapshot rehearsal+delta; M17 frontend-ready pilot; M18 rollout unit demi unit.

**Exit:** semua core workflow selected Class end-to-end, data reconciliation approved, teacher/student scope benar, no coupled active Batch, rollout/hold/forward-fix teruji.

### 18.3 P2 — Medium: operasional dan maintainability

- Perluas observability, dashboards migration, performance/load optimization berdasarkan query evidence.
- Harden dependency patch cadence, disaster recovery exercise, support tools untuk approved correction dan safe export.
- Dokumentasikan Source of Truth v3, schema registry, API lifecycle/deprecation, on-call dan access reviews.
- Improve report projections/cache invalidation/connection capacity bila hasil baseline menunjukkan perlu; jangan menambah cache tanpa correctness model.

### 18.4 P3 — Nice to have

- M19: reusable templates, optional cohort sync, advanced quiz/question bank, SCORM/LTI/xAPI, richer calendar, advanced analytics.
- Pemecahan service hanya bila ownership/team/load membuktikan manfaat; tidak menjadi kewajiban rewrite Go.
- Frontend redesign dan accessibility/interaction polish menjadi plan frontend tersendiri setelah contract/states matang; bukan berarti UX backend diabaikan sekarang.

### 18.5 Milestone dan exit evidence

| Milestone | Paket | Exit evidence utama |
|---|---|---|
| G0 Environment safe | M00/M01/M02 | Distinct DB proof, restored backups, deployed critical containment |
| G1 Go foundation | M03/M04 | Reproducible Go build, auth/policy/no-auto-DDL tests |
| G2 Academic vertical slice | M05–M10 | Create Class→assign→content→submit→grade→completion end-to-end |
| G3 LMS-specific parity | M11–M14 | Attendance/permit/SP/logbook/integrations/report contract sesuai use |
| G4 Migration proven | M15/M16 | Two verified rehearsals, delta+resume, zero unexplained data loss |
| G5 Pilot | M17 + frontend tahap berikutnya | One approved unit owner V3, observed learner writes, fallback ready |
| G6 Rollout accepted | M18 | All selected units reconciled, operational staging protected, legacy retained |

### 18.6 Risiko/tradeoff yang harus dibahas tim

| Risiko/tradeoff | Dampak | Mitigasi / keputusan |
|---|---|---|
| Rewrite bahasa + domain + schema sekaligus | Parity gap, scope membesar, reintroduce bug lama | Go vertical slice, modular gates, legacy security first, contract tests |
| Skill Go belum merata | Pool/transaction/context/concurrency mistakes | Spike M03, review patterns, standard library idioms, race/integration tests |
| Two runtime identity | Logout/disable/profile tidak konsisten | Single global identity owner, verified bridge, explicit global cutover |
| Shared legacy grades/attendance | Pilot Class tunggal ternyata tidak independen | Dependency-complete migration unit, reviewed archive mapping |
| Staging berisi production data | PII/secret leak, accidental dev reset | Private namespaces+roles, restricted backup, outbound off, operational promotion |
| No-delete selamanya | Storage growth, retention constraints, old secret artifacts | Encryption/access isolation, rotation, capacity budget; escalate policy conflict |
| Additive data history | More tables/SQL and storage | Typed repositories, projection rebuilds, index/query review, no unnecessary microservices |
| Source traffic selama copy | Snapshot stale/long vacuum pressure | Consistent snapshot, bounded runs, final fence+delta, monitor source load |
| Legacy write paths sulit difence | Split brain dan kehilangan akademik | Explicit guards+jobs inventory, broader temporary freeze if necessary |
| Frontend belakangan | Backend ready tetapi belum layak cutover | OpenAPI+fixtures now, dedicated integration gate later |
| Go diharapkan otomatis cepat | Disappointment karena DB/provider bottleneck | Measure p95/p99/query plan/cost, optimize based evidence |
| Backup tidak cover objects/global config | Restore DB berhasil tetapi proof/login gagal | File manifest+secure object copies, role/extension/deployment inventory |
| Old user status/grade provenance ambiguity | Akses salah, transkrip berubah diam-diam | Review queue, original snapshot, academic/security disposition |

### 18.7 Keputusan yang perlu dikonfirmasi sebelum eksekusi, bukan sebelum plan selesai

1. Identitas deployment production/staging dan source connection yang benar; CA/provider access; authorized operator/backup destination.
2. Approval data production berada di target staging dengan siapa saja yang boleh mengakses; storage capacity dan encrypted backup recovery owner.
3. Default Teacher boleh enroll Student tetapi tidak grant Teacher/SiteAdmin (Bagian 3); Facilitator default ops-only. Tim boleh mengubah capability melalui ADR, bukan specialization string.
4. Completion/kelulusan: mandatory evidence, pass thresholds, weight policy, attendance rule, SP escalation, transcript/certificate release dan override authority.
5. Kebijakan legacy Class merge **default tidak merge**, penempatan program-global scores dan batch-scoped attendance/logbook.
6. Identity bridge/broker, domain cookie topology, account lifecycle propagation dan timing global identity cutover.
7. Pilot unit, maintenance/read-only window yang diterima, success observation period, support owner, measured RTO.
8. Lokasi canonical plan dalam repo `be-v3` ketika implementation dimulai dan prosedur review/push/hotfix lintas repo.

Default desain di dokumen ini memungkinkan coding modul terisolasi setelah diotorisasi, tetapi tidak menggantikan input yang diperlukan untuk akses/copy production dan cutover.

## 19. Developer action checklist dan traceability

### 19.1 Checklist siap dibagikan

**Security / legacy owner**

- [ ] Konfirmasi deployed SHA; reproduce relevant findings safely pada environment terotorisasi.
- [ ] Tutup email-only setup-password dan unauthorized grade imports.
- [ ] Perbaiki session lifecycle/log redaction, coordinated cookie/CSRF/XSS frontend patch.
- [ ] Terapkan ancestry policy/self-profile allowlist, scope reads dan permits.
- [ ] Private proof assets, safe egress, secret rotation; AI suggestion-only.
- [ ] Stop startup synchronize/repair, hard deletes, session/audit/job/file purge; no-delete review.
- [ ] Catat tests, independent review, actual deployment dan residual risks.

**Backend Go owner**

- [ ] Mulai `be-v3` Go-only setelah izin implementasi; no inherited NestJS scaffold runtime.
- [ ] Buat platform config/DB/transaction/audit/outbox dan CI; pin Go/tooling.
- [ ] Implement identity/account/session dan contextual role/capability, resource ancestry helper.
- [ ] Implement standalone Class dengan optional program/cohort dan multiple Teachers.
- [ ] Pisahkan enrollment/episode/role assignment; group/personal mentor opsional.
- [ ] Implement files/content/assignment versioning dan immutable submissions.
- [ ] Implement atomic grade history, validated rubric, preview import, release.
- [ ] Implement evidence/completion/transcript/certificate terpisah.
- [ ] Implement Class attendance/permit/SP/calendar dan configurable logbooks.
- [ ] Implement bounded jobs/integrations, typed OpenAPI, scoped read models.

**Database / migration owner**

- [ ] Verifikasi DB source/target berbeda; audit exact schema/constraints/indexes/types/trigger/jobs.
- [ ] Peroleh verified CA dan least-privilege roles; jangan bypass TLS.
- [ ] Backup SQL/custom+objects, manifest/checksum, actual restore proof sebelum migration.
- [ ] Preserve staging public; siapkan private landing/ops/rehearsal/operational namespaces additive.
- [ ] Buat source adapters, origin maps, row receipts, conflict decisions dan same-Class FK constraints.
- [ ] Map semua 20 baseline tables plus discovered additions/external artifacts; no silent skip.
- [ ] Review collisions/duplicates/orphans/global grades/batch evidence tanpa menghapus original.
- [ ] Rehearse snapshot→backfill→delta→crash/resume; per-learner reconciliation.
- [ ] Enforce owner-aware importer, source read-only, suppress notifications, storage capacity alerts.

**QA / academic owner**

- [ ] Approve role matrix, states, grading/completion/attendance/logbook policies.
- [ ] Execute full test matrix, real PostgreSQL concurrency, authorization negatives.
- [ ] Compare original raw and observed legacy results dengan v3, approve every material difference.
- [ ] Verify file privacy+availability, no lost submissions, historical transcript/certificate provenance.
- [ ] Sign off rehearsal dan pilot unit, bukan hanya total row count atau UI demo.

**Operations / release owner**

- [ ] Tetapkan staged environment sebagai sensitive/operational target; roles/network/secrets/backups reviewed.
- [ ] CI push `be-v3` tidak auto-migrate/live-deploy; secrets/PII scanning aktif.
- [ ] Implement persistent write fence, job drain, ownership epochs, fail-closed routing.
- [ ] Rehearse safe hold/forward fix setelah first Go write; jangan rollback ke stale live DB.
- [ ] Frontend/session/old-links readiness pada tahap berikutnya sebelum user pilot.
- [ ] Update per-package/per-run/per-unit status+evidence; retain live/staging historical data dan backups.

### 19.2 Hubungan audit → paket

| Audit baseline | Penanganan dalam plan |
|---|---|
| SEC-01–22 | Registry Bagian 7 → M02 legacy, M03/M04/M06/M09/M13 Go, validation Bagian 15 |
| AR-01 giant ClassesService | Go module ownership Bagian 5 → M03–M14 |
| AR-02/04/15 role/enrollment/facilitator | Capability matrix dan participation Bagian 3/6 → M04/M05 |
| AR-03 Class coupling | Standalone Class; optional metadata/groups → M05; reviewed migration unit M15 |
| AR-05/06 grade-completion/history | Revision/evidence separation → M07–M10 |
| AR-07/08/09 read repairs/state/transactions | Explicit commands, states, idempotency, audit/outbox → M03–M13 |
| AR-10/11 JSON relations/files | FK+same-Class constraints, versioned graph, FileAsset → M05/M06/M09/M15 |
| AR-12 integrations | Durable jobs/suggestion-only/egress → M03/M06/M13 |
| AR-13/16 contract/test drift | OpenAPI contract + route inventory → M14/M17 |
| AR-14 timezone | UTC timestamp + Class timezone + raw migration type preservation → all domain modules/M15 |
| AR-17 deployment | Immutable Go build, explicit migrations, no deploy-on-push → M03/M18 |
| AR-18 performance | Scope/index/pagination/load evidence → M14/M16 |
| DI-01–04 duplicates | Preserve raw; canonical constraints/revisions/resolution, no deletion → M05/M08/M11/M12/M15 |
| DI-05 global active Batch | Requirement removed for v3; no single-active-batch constraint introduced |
| DI-06–10 transaction races | Locks/CAS/atomic command+audit/outbox → M04/M05/M09/M10 |
| DI-11–14 historical loss/state drift | No-delete, stable Class/episodes, one canonical grade/participation → M05/M09/M15 |
| DI-15–19 grading/SP/calendar | Graph validation, explicit missing grade, Class-local discipline/calendar → M09–M11 |
| DI-20–25 file/repair/schema/audit | Private asset history, explicit migration/constraints, durable retained audit → M02/M03/M06/M15/M18 |

Rekomendasi audit “dedupe/cleanup”, “single active batch”, “CourseRun per Program+Batch”, “keep NestJS”, dan “contract/drop legacy” tidak diadopsi secara literal. Target baru mempertahankan maksud integritas/keamanan tetapi mengikuti keputusan **Go, Class mandiri dan no-delete**.

### 19.3 Hubungan benchmark → fitur yang dipertahankan/diubah

| Prinsip benchmark/Moodle | Keputusan v3 | Fitur LMS yang tetap ada |
|---|---|---|
| Enrollment berbeda dari contextual role | Separate enrollment/role tables, satu UX Add Participant | Teacher/Student/personal mentor dalam Class |
| Teacher capability dalam course context | Teacher semua academic scope Class yang ditugaskan, bukan specialization | Multi-teacher, optional group mentor assignment |
| Content berbeda dari learner state | Activity revisions vs submissions/evidence | Materials, assignments, external links |
| Grade berbeda dari completion/progress | Gradebook+rule evidence+release terpisah | Rubric, competency, transkrip, sertifikat |
| Enrollment lifecycle tidak global user role | Episodes/suspend/end Class, identity disable terpisah | Retake, transfer, alumni access |
| Cohort/group mendukung enrollment | Optional metadata dan explicit bulk enrollment | Program/cohort labels untuk filter/laporan |
| File access contextual | Private FileAsset, owner/context authorization | Proof izin/sakit, assignment files, avatars |
| Durable audited state transitions | Transaction/outbox/revision/no-delete | Attendance, SP, logbook, notification |
| Integrations bukan canonical academic authority | AI suggestions + reviewed grade command | AI evaluation dan import nilai tetap tersedia secara aman |

Moodle sendiri memisahkan konteks role dan enrollment; seorang user dapat Teacher pada satu course dan Student pada yang lain. Desain v3 memakai prinsip itu, bukan menyalin seluruh platform. Rujukan behavior: [Moodle Assign roles](https://docs.moodle.org/502/en/Assign_roles) dan [Moodle enrolment API](https://moodledev.io/docs/5.0/apis/subsystems/enrol).

### 19.4 Hasil akhir yang dianggap berhasil

Admin dapat membuat Class dengan nama dan metadata opsional, menugaskan beberapa Teacher serta Student, lalu menjalankan pembelajaran tanpa ketergantungan Batch aktif. Teacher bekerja hanya di Class yang ditugaskan; identity, participation, capability, submission, grade, completion dan file mempunyai sumber yang jelas. Fitur khas LMS tetap tersedia sesuai kebutuhan Class.

Backend tersebut berjalan dengan **Go**, memakai database staging yang telah disiapkan dan diproteksi sebagai target v3. Perpindahan dari live terbukti melalui backup yang bisa direstore, mapping deterministik, reconciliation per learner, write fence dan satu owner. Data live lama, data staging lama, sejarah akademik, dan backup tidak dihapus. Frontend menyusul setelah contract stabil dan menjadi gate terpisah sebelum learner cutover.

**Status revisi 13 September: PARTIAL_IMPLEMENTATION dengan pelacakan M → CP.** Bagian core M00/M01/M03/M04/M05/M06/M07/M08/M15 mempunyai evidence lokal; M03 6/6, M05 4/5, M06 3/4, M07 3/4, M08 3/4 exit lokal dengan CP04 partial/BLOCKED_M15, dan M15 4/5 slice sintetis dengan canonical D 0/6 (modul lainnya yang disebut masih 0/5 canonical), sementara M02 disposition masih parsial. M15-CP03/CP04 final run `20260913T141341Z-postgres-lab-81b971a0ad85` PASS_SYNTHETIC_ONLY dengan dua backup+actual isolated restore baru, Go/DB 2/2 dan Playwright 1/1 per CP; [evidence](backend-v3/backend-go/docs/checkpoints/2026-09-13-m15-cp03-cp04.md). Pekerjaan berikut yang disarankan adalah M15-CP05 synthetic reconciliation; real source mapping/import tetap setelah source nyata/CA/backup/approval gate. Backend Go dan backup sintetis memang sudah diuji; production copy, actual file/DNS providers, full-module verification, security deployment dan rollout belum diklaim selesai.
