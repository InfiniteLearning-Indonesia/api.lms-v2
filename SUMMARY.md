# Rangkuman Update & Pembaruan Sistem (LMS v2)

Dokumen ini berisi rangkuman komprehensif mengenai seluruh fitur baru, peningkatan keamanan (*hardening*), dan perbaikan UX yang telah diimplementasikan pada repositori **Frontend** dan **Backend** LMS v2.

---

## 🛡️ 1. Keamanan & Hardening Backend (*Security & Infrastructure*)

* **Sesi Login Persisten di PostgreSQL (`connect-pg-simple`)**:
  * Mengganti `MemoryStore` bawaan `express-session` dengan **PostgreSQL Session Store** (`connect-pg-simple`).
  * **Manfaat**: Data sesi login user kini tersimpan aman di database Supabase. Saat server backend di-restart atau di-deploy ulang, **seluruh user tidak akan ter-logout secara mendadak** dan RAM VPS aman dari kebocoran memori (*memory leak*).
  * **Skema Otomatis**: Tabel `"session"` dan indeksnya akan otomatis dibuat saat server *bootstrap*.

* **Pencegahan Mass Assignment**:
  * Menerapkan `ValidationPipe` secara global di NestJS (`main.ts`) dengan opsi `whitelist: true`.
  * **Manfaat**: Secara otomatis membuang (*strip*) field liar yang dikirim oleh penyerang melalui payload JSON.

* **Penguncian IDOR & Escalation Guard**:
  * Memperketat pengubahan status user dan role menggunakan guard `@Roles('admin')`.
  * Menutup celah pengubahan role/status secara mandiri (*self-escalation*) dan *Insecure Direct Object Reference* (IDOR).

* **Perlindungan DDoS & Brute-force (`Rate Limiting & Helmet`)**:
  * Mengaktifkan `@nestjs/throttler` secara global (30 request/menit per IP) serta memproteksi header HTTP menggunakan `helmet`.
  * Menghilangkan header fingerprint `x-powered-by: Express`.

* **Pengaturan CORS Ketat**:
  * Mengunci *origin* yang diizinkan hanya untuk domain terverifikasi (`infinitelearningstudent.id`, `localhost`) dengan dukungan `credentials: true`.

* **Sistem Audit Log Opsional (`AuditModule`)**:
  * Membuat `AuditInterceptor` dan `AuditLog` entity untuk mencatat aktivitas penting (perubahan role, kelulusan, manipulasi data).

* **HTTP Access Logger Middleware**:
  * Menambahkan `HttpLoggerMiddleware` yang mencatat setiap request HTTP ke file log bulanan di `logs/http-requests-YYYY-MM.log` secara *asynchronous* (zero-blocking).

---

## 📊 2. Monitoring Layanan & System Status Page (`/status`)

* **Backend Health Check & History API (`/health` & `/health/daily`)**:
  * Menambahkan endpoint kesehatan sistem yang memeriksa status koneksi PostgreSQL/Supabase dan latensinya secara *real-time*.
  * Menambahkan `HealthHistoryService` dengan cron job (setiap 15 menit) untuk mencatat status uptime dan menghitung persentase SLA 90 hari terakhir.

* **Halaman Publik Status Layanan (`/status`)**:
  * Membangun halaman status modern berdesain *glassmorphism* untuk menampilkan:
    * Status Sistem (*Operational*, *Degraded*, *Outage*).
    * Latensi Database Supabase (ms).
    * Penggunaan Memori Server (RSS MB).
    * *Uptime* server sejak restart terakhir.
    * Grafik Uptime Bar 90 hari terakhir dengan tooltip tanggal & rasio ketersediaan.

* **Integrasi Link Navigasi Status**:
  * Menambahkan link **Status Layanan** yang elegan dengan indikator status di footer **Landing Page (`/`)** dan **Halaman Login (`/login`)**.

---

## 👥 3. Pemisahan Peran & Perbaikan Manajemen User

* **Isolasi Akun Facilitator**:
  * Memisahkan akun berstatus `facilitator` dari daftar siswa utama ke tab khusus **Facilitator**.
  * Akun Facilitator tidak lagi tercampur di Manajemen Siswa Binaan, Logbook, maupun Absensi Mentor.

* **Pembersihan Fitur Mentor**:
  * Menghapus filter Program Studi pada view Mentor sesuai dengan alur operasional terbaru.
  * Membatasi fungsi `Suspend` / `Unsuspend` hanya untuk role Admin. Tab Facilitator pada view Mentor murni bersifat informatif (*read-only*).

---

## 🛠️ 4. Pengujian & Verifikasi

* **Verifikasi Build Complete**:
  * Frontend (`Next.js`) dan Backend (`NestJS`) telah di-build ulang (`npm run build`) dan lulus 100% tanpa error kompilasi TypeScript maupun linting.
* **Update Graphify**:
  * Knowledge graph repositori telah diperbarui menggunakan `graphify update .` untuk memetakan hubungan antar komponen terbaru.

---

*Disusun pada: 2 Agustus 2026*  
*Status: Ready to deploy to Staging*
