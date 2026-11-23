# Runtime Go lokal untuk uji LMS v3

Untuk memasang frontend produk ke API, gunakan [kontrak inti](../API-DOCS.MD) dan [checklist integrasi](INTEGRATION_CHECKLIST.md). Panduan ini menjelaskan cara menjalankan preview dan membaca bukti lokal.

`cmd/api` menjalankan mode `v3-local-preview`. Launcher menyiapkan cluster PostgreSQL 18 baru di loopback, membackup target kosong dalam custom dan SQL, lalu membuktikan restore sebelum memasang skema. API berjalan dengan role terbatas dan memeriksa marker serta identitas cluster. Status `/health/ready` baru berhasil setelah pemeriksaan database lulus. Konfigurasi preview berasal dari launcher. Data dan koneksi staging/live berada di luar cakupan.

## Verifikasi sebelum integrasi `/fe-test`

```powershell
D:\Projects\lms-v2\backend-v3\backend-go\qa\RUN_LOCAL_PREVIEW_QA.cmd
```

Target `PASS_LOCAL_PREVIEW_SYNTHETIC_ONLY` mencakup backup/restore, HTTPS, tujuh skenario Playwright API, dan shutdown Go serta PostgreSQL. Uji memakai biner `cmd/api`. Skenario meliputi halaman dan API satu origin, login admin, undangan dua role, mailbox lokal, aktivasi, acceptance, pembatasan akses lintas Class, content, dan logout. Bukti tersimpan di `backend-v3/private-backup/<run>`.

**QA runtime lulus lokal sintetis.** Run `20261122T193033Z-local-preview-18d5120012dc` menghasilkan `PASS_LOCAL_PREVIEW_SYNTHETIC_ONLY`, 7/7 skenario Playwright API, dan `/health/ready` HTTP 200 pada biner `cmd/api`. Hash source `14508c4d31fe421aca9d47ba2c1b844ce747a7fb44ee355f406aa61d246e6d33`, hash backup custom dan SQL, receipt restore, dan shutdown Go serta PostgreSQL sudah dicocokkan. Run ini menggantikan percobaan yang tertahan izin folder di Codex.

QA memakai halaman fixture untuk membuktikan penyajian frontend dan API pada origin yang sama. Build `/fe-test` dan pengujian manual browser adalah tahap integrasi M14. Suite Go, `go vet`, registry Go dan 49 tes Python operator juga lulus.

## Menjalankan preview manual

```powershell
D:\Projects\lms-v2\backend-v3\backend-go\qa\START_LOCAL_PREVIEW.cmd
```

- Buka `https://127.0.0.1:9443`. Launcher membuat sertifikat RSA HTTPS loopback yang berlaku sekitar satu jam. Browser dapat meminta persetujuan lokal karena sertifikat tersebut self-signed. File `runtime-tls.crt` ada di direktori run untuk verifikasi.
- Launcher melayani build yang tersedia di `lms-v2/fe-test/dist` pada origin yang sama. Backend tidak menjalankan build frontend. Gunakan URL relatif `/v3/...` dari halaman ini agar cookie dan pemeriksaan origin bekerja.
- Launcher mencetak path `preview-admin.private.json`. Login memakai akun sintetis di file itu. Ketik `qa` untuk menjalankan tiga alur browser lewat `/fe-test`, atau `mail` untuk mengekspor undangan dan reset yang masih aktif. Ketik `stop` untuk mematikan Go dan PostgreSQL. Setiap start membuat fixture baru dan run lama dipertahankan.
- File memakai storage privat di direktori run dan scanner fixture. AI memakai provider rehearsal. Pengiriman email, worker background dan service eksternal punya verifikasi tersendiri.

Jika sesi preview lama mencatat `tls: peer doesn't support any of the certificate's signature algorithms`, ketik `stop` lalu jalankan launcher manual kembali. Sesi baru menggunakan sertifikat RSA. Perubahan juga menyajikan `/favicon.svg` dan `/icons.svg` dari build `/fe-test`.

### QA browser dari prompt preview

`qa` menjalankan Chrome yang terlihat dan memperlambat aksi agar bisa diikuti. `qa headless` menjalankan tugas yang sama di latar belakang. Sesi Site Admin, Mentor, dan Mentee terpisah. Browser hanya mengakses origin loopback yang sertifikatnya diperiksa launcher. Runner membuat nama Class dan email sintetis unik, mengambil token aktivasi lewat mailbox privat, lalu menguji undangan, tugas sampai nilai dirilis, serta batas hak Teacher dan Student antar Class. Hasil dan screenshot panel saat gagal tersimpan dalam direktori `journeys-*` di run preview. Targetnya `PASS_FE_TEST_JOURNEYS` dengan 3/3 alur lulus. Backend tetap berjalan setelah browser selesai.

Run `journeys-76cb56753d83` berhasil melewati login lalu timeout di form Buat Class. Label lama ikut memuat nilai awal textarea saat dicari secara persis. Editor sekarang memiliki label terpisah dan ID tetap. Laporan baru mencatat posisi timeout melalui `operation` dan `ui_stage`.

**Bukti browser 23 November 2026.** Run `20261122T211331Z-local-preview-ead81cc41a30`, laporan `journeys-192cb3ba2b7a`, lulus **3/3 alur dan 10 langkah** di Chrome terlihat. Hash build `/fe-test` cocok dengan `feae258656be579e1030bf8e74b38414a42242c5287d7f602aba8d2d1264e5af`. Bukti mencakup akun undangan, tugas hingga nilai dirilis di tampilan mini, serta peran berbeda untuk akun yang sama antar-Class. Bukti ini berlaku pada runtime sintetis lokal.

## Kontrak awal untuk frontend

| Alur | API |
|---|---|
| Login, sesi, logout | `POST /v3/auth/password/login`, `GET /v3/auth/me`, `POST /v3/auth/logout`. Simpan nilai `csrf_token` dari respons autentikasi untuk sesi tab. Kirim `X-CSRF-Token` pada aksi yang membutuhkan sesi. |
| Class yang tersedia | `GET /v3/me/classes`. Menerima cookie sesi dan mengembalikan `classes` berisi maksimal 250 Class terurut menurut nama dan ID. Setiap item memuat `id`, `name`, `program_label`, `cohort_label`, `state`, `version`, dan `roles` pengguna di Class itu. Site Admin melihat seluruh Class dalam batas respons. |
| Buat Class dan undang | `POST /v3/classes` dan `POST /v3/classes/{id}/account-invitations`. Role input `teacher` (Mentor) atau `student` (Mentee). JSON, CSRF dan `Idempotency-Key` digunakan untuk aksi Class. |
| Terima undangan | Aktifkan akun dari token mailbox, login, lalu gunakan `GET /v3/me/class-invitations` dan `POST /v3/classes/{id}/invitations/{invitation_id}/accept`. Class muncul sesuai state dan role. |
| Content dan fitur lain | Router menyambungkan domain file, content, submission, gradebook, completion, attendance, logbook, notification, AI, dan reporting. Perlu walkthrough per fitur untuk acceptance M14. |

Gunakan [kontrak autentikasi](AUTHENTICATION_V3.md), [payload undangan CP05](checkpoints/2026-11-23-m04-cp05-class-invitations.md) dan [checklist manual M14](modules/M14.md#uji-manual-lokal-sebelum-handover) saat menghubungkan UI. API bisnis memeriksa hak akses di server. Gunakan response `401`, `403`, `409` dan `429` untuk pesan UI sesuai hasil aksi.

Google nyata bersifat opsional di preview manual. Setelah project Google memiliki redirect URI tepat `https://127.0.0.1:<port>/auth/google/callback`, jalankan launcher dengan `--google --port <port>` dan variabel proses `LMS_PREVIEW_GOOGLE_CLIENT_ID` serta `LMS_PREVIEW_GOOGLE_CLIENT_SECRET`. Build frontend perlu mengimplementasikan halaman callback sesuai kontrak CP04. QA otomatis preview memakai jalur password, dan bukti Google sintetis berasal dari CP04.
