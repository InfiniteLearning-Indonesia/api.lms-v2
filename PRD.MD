## Product Requirements Document (PRD): Infinite Learning LMS

### 1. Informasi Umum

Fase pertama dari pengembangan Learning Management System (LMS) Infinite Learning berfokus secara eksklusif pada pembuatan **Landing Page** dan **Login Page**. Pengembangan fitur inti LMS ditangguhkan ke fase berikutnya, memastikan fondasi antarmuka, tata letak, dan sistem desain terbangun dengan sempurna terlebih dahulu.

### 2. Spesifikasi Teknis & Sistem Desain

Bagian ini wajib dibaca oleh agen pengembang (*agent antigravity*) saat menginisiasi proyek dan menggunakan *MCP tools open-design*.

* **Framework Utama:** NextJS
* **UI Library:** ShadcnUI
* **Animasi & Transisi:** Framer Motion
* **Ikonografi:** Lucide React
* **Paradigma Tata Letak (Instruksi Agen):** Agen harus menggabungkan struktur komponen Google Classroom (seperti *card layout*, ruang putih yang lega, dan navigasi modular) dengan gaya visual dari sistem desain Anthropic Claude (elemen antarmuka minimalis, tipografi modern, dan kontras yang tajam).

### 3. Panduan Gaya (Style Guide)

| Elemen | Spesifikasi / Nilai Hex | Penggunaan Utama |
| --- | --- | --- |
| **Font Header** | Lexend Deca | Judul utama, nama halaman, *hero text*. |
| **Font Body** | Inclusive Sans | Teks paragraf, tombol, deskripsi, form input. |
| **Warna Putih** | `#FFFFFF` | Latar belakang utama, teks pada tombol warna gelap. |
| **Warna Hitam** | `#000000` | Teks utama, teks penekanan. |
| **Warna Ungu** | `#8A3DFF` | Warna merek utama, *Call to Action* (CTA), elemen aktif. |
| **Warna Kuning** | `#FFCD29` | Aksen, sorotan peringatan, elemen gamifikasi/notifikasi. |
| **Warna Abu-abu** | `#8E8E93` | Teks sekunder, *border*, teks *placeholder*. |
| **Ungu Gradasi** | `#8A3DFF` ke `#667EFF` | Latar belakang *hero section*, tombol premium, spanduk. |

### 4. Manajemen Peran (User Roles)

Meskipun fitur khusus peran belum dikembangkan pada fase ini, sistem autentikasi dasar harus dipersiapkan untuk mengakomodasi jenis entitas pengguna berikut di masa depan:

* **Admin:** Mengelola keseluruhan sistem dan pengguna.
* **Mentor:** Mengelola kelas, materi, dan evaluasi siswa.
* **Mentor-Admin:** Peran ganda dengan akses manajerial tingkat tinggi dan manajemen kelas.
* **Student:** Pengguna akhir yang mengonsumsi materi pembelajaran.

### 5. Kebutuhan Halaman: Landing Page

Halaman ini adalah titik kontak pertama bagi pengunjung dan calon pengguna Infinite Learning. Fokus pada performa rendering yang cepat dan animasi yang mulus menggunakan Framer Motion.

* **Navbar:** Minimalis, memuat logo Infinite Learning di sebelah kiri dan tombol "Login" (Warna Ungu `#8A3DFF`) di sebelah kanan.
* **Hero Section:** Menggunakan latar belakang Ungu Gradasi atau putih bersih dengan teks judul (*Lexend Deca*) yang mencolok. Menawarkan pesan penyambutan yang kuat.
* **Value Proposition / Features Placeholder:** Menggunakan gaya *grid/card* khas Google Classroom untuk menampilkan secara singkat apa itu Infinite Learning.
* **Footer:** Sederhana, berisi tautan statis dan hak cipta. Latar belakang putih atau abu-abu terang.

### 6. Kebutuhan Halaman: Login Page

Halaman autentikasi sentral bagi seluruh pengguna (Admin, Mentor, Student) sebelum masuk ke *dashboard* masing-masing (yang akan dibuat di fase selanjutnya).

* **Layout:** Bersih, terpusat di tengah layar (*center-aligned modal style*) mengadopsi estetika minimalis antarmuka Claude.
* **Form Input:** Bidang *Email/Username* dan *Password* menggunakan komponen form dari ShadcnUI dengan gaya *border* warna Abu-abu (`#8E8E93`) dan fokus *border* warna Ungu (`#8A3DFF`).
* **Tipografi Form:** Label menggunakan *Inclusive Sans*.
* **Tombol Login:** Lebar penuh (*full width*), warna latar belakang Ungu (`#8A3DFF`), teks warna Putih (`#FFFFFF`), dengan animasi *hover* sederhana dari Framer Motion.
* **Tautan Bantuan:** Teks "Lupa Kata Sandi?" berwarna Abu-abu yang berubah menjadi Hitam saat *hover*.