# Checklist integrasi frontend produk LMS v3

Gunakan checklist ini saat menghubungkan build frontend produk yang sudah ada ke Go. Pemilik proyek memeriksa layar dan menerima hasil browser. Perubahan backend dicatat di modul pemilik API beserta test case yang sesuai. Mulai dari [kontrak API](../API-DOCS.MD) dan [urutan integrasi](FRONTEND_INTEGRATION_ROADMAP.md).

## 1. Petakan layar ke operasi Go

- [ ] Daftar layar aktif untuk Site Admin, Mentor dan Mentee. Untuk setiap layar catat path UI, aksi utama, operasi OpenAPI, aktor dan scope Class.
- [ ] Isi sumber setiap ID yang dipakai layar, termasuk setelah reload atau saat membuka deep link. Catat prioritas layar yang memerlukan daftar pengumpulan dan item nilai Mentor, akses lampiran atau fitur di luar kontrak inti.
- [ ] Tetapkan status per aksi `SIAP_DIUJI`, `GAP_BACKEND`, `GAP_FRONTEND`, atau `LULUS`. Isi gap dengan perilaku yang dibutuhkan dan kriteria uji, bukan endpoint dugaan.

| Layar atau aksi | Operasi API | Asal ID dan izin | Uji penerimaan | Status |
|---|---|---|---|---|
| Login dan resume sesi | `loginWithPassword`, `getCurrentAccount` | Cookie Go, CSRF per tab, user ID dari sesi | Login, reload, sesi tidak valid, logout | `SIAP_DIUJI` |
| Admin buat Class dan kirim undangan | `createClass`, `inviteAccountToClass`, `listMyClassInvitations`, `acceptClassInvitation` | ID Class dari create, ID undangan dari inbox akun | Mentor dan Mentee memperoleh role di Class yang tepat | `SIAP_DIUJI` |
| Tugas dan nilai | Operasi Content, Submissions dan Grades | ID aktivitas/revisi dari konten, ID revisi jawaban dari receipt | Nilai hanya muncul untuk Student setelah release | `SIAP_DIUJI` untuk alur dengan ID yang tersedia |
| Inbox grading Mentor | Discovery submission dan item nilai | Mentor Class yang sama, bekerja sesudah reload | Muat daftar tanpa ID manual, akses lintas Class ditolak | `GAP_BACKEND` jika layar memerlukan daftar tersebut |
| Akses lintas Class | `listMyClasses`, `getClassContent`, `createSection` | Role diambil per Class dari Go | Teacher di A dapat mengubah konten A, Student di B mendapat 403 saat mencoba mutasi B | `SIAP_DIUJI` |

Daftarkan semua layar produk yang akan diaktifkan. Baris di atas adalah sampel awal berdasarkan kontrak dan alur browser yang tersedia.

## 2. Sambungkan frontend produk lokal ke Go

- [ ] Jalankan frontend produk dan `cmd/api` yang menggunakan PostgreSQL preview. Pertahankan satu origin HTTPS untuk browser dan API. Verifikasi cookie, CSRF, path callback Google dan `GET /health/ready`.
- [ ] Pada layar yang diuji, baca dan tulis lewat API Go. Uji dengan akun dan data sintetis di database preview. Pastikan tidak ada jalur sukses yang hanya memperbarui state frontend.
- [ ] Jalankan tiga alur inti melalui build produk. Uji reload/deep link, error 401/403/404/409 yang relevan, dan hasil setelah mutasi. Catat commit/build frontend, commit/backend source, lingkungan, kasus dan hasil di [indeks QA](QA_EVIDENCE.md).

## 3. Terima staging-v2 dan data historis

- [ ] Buktikan runtime serta akses PostgreSQL `staging-v2` sebelum mengarahkan frontend produk. Uji Google dan email nyata, kesiapan Go, dan provider yang dipakai layar, termasuk berkas jika masuk cakupan.
- [ ] Ulangi browser acceptance dengan akun/data sintetis di `staging-v2`.
- [ ] Untuk akun/data historis, selesaikan pemetaan dan rehearsal lokal, backup terverifikasi sebelum tiap migrasi, lalu uji pembacaan dan izin berdasarkan hasil impor di lingkungan tujuan. Pilot dan rollout dipantau di M17–M18. [Roadmap](FRONTEND_INTEGRATION_ROADMAP.md)
