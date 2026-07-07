# Infinite Learning LMS v2 Domain Model & Source of Truth

> Version 2.0

> Status: Final Domain Architecture

---

# Bab 1: Pendahuluan dan Tujuan

Dokumen ini merupakan **Source of Truth** utama, mutlak, dan final mengenai seluruh arsitektur domain, spesifikasi entitas, aturan alokasi, logika kepemilikan (ownership), matriks hubungan, serta hierarki perizinan akses di dalam Learning Management System (LMS). Seluruh ketentuan yang tercantum dalam dokumen ini bersifat mengikat untuk seluruh fase siklus pengembangan sistem teknis berikutnya.

Implementasi pada tingkat Backend (arsitektur database, skema tabel, validasi API, permission system, alokasi otomatis, dan role management) serta tingkat Frontend (state management, alur validasi UI, dan perizinan akses komponen) **wajib tunduk dan mematuhi aturan bisnis yang tertera di dalam dokumen ini tanpa pengecualian.** Dokumen ini dirancang dengan tingkat kejelasan dan kompleksitas tinggi untuk mengeliminasi ambiguitas sebelum melangkah ke tahap desain teknis.

---

# Bab 2: Daftar Entitas Domain (Domain Entities)

Sistem LMS ini tersusun atas entitas sistem dasar (Administrator) dan entitas akademik utama (Mentor, Student, Program). Masing-masing entitas didefinisikan secara formal sebagai berikut:

## 1. Entitas Administrator (Admin)

Entitas pengelola sistem tertinggi yang bertanggung jawab atas seluruh operasional manajerial LMS, termasuk eksekusi alokasi murid, pengaturan penugasan mentor, pembuatan program, dan manajemen hak akses pengguna.

## 2. Entitas Mentor

Entitas Mentor diklasifikasikan berdasarkan spesialisasi keahlian teknis dan fungsionalitasnya. Setiap jenis peran mentor di bawah ini dapat diisi oleh lebih dari 1 (satu) orang personel yang bergerak bersama sebagai sebuah tim:

* **Mentor AI**: Mentor dengan spesialisasi penuh pada bidang Artificial Intelligence Development. Bisa berbentuk tim (lebih dari 1 orang).
* **Mentor Web**: Mentor dengan spesialisasi penuh pada bidang Web Development. Bisa berbentuk tim (lebih dari 1 orang).
* **Mentor Mobile**: Mentor dengan spesialisasi penuh pada bidang Mobile Development. Bisa berbentuk tim (lebih dari 1 orang).
* **Mentor Game**: Mentor dengan spesialisasi penuh pada bidang Game Development. Bisa berbentuk tim (lebih dari 1 orang).
* **Mentor UI/UX**: Mentor lintas program dengan spesialisasi pada User Interface dan User Experience Design. Bisa berbentuk tim (lebih dari 1 orang).
* **Mentor Professional**: Mentor lintas program dengan spesialisasi pada pengembangan kemampuan profesional dan Soft Skills. Bisa berbentuk tim (lebih dari 1 orang).

## 3. Entitas Student (Murid)

Entitas Student diklasifikasikan berdasarkan fokus bidang studi yang mereka ambil:

* **Murid AI**: Student yang terdaftar secara aktif pada jalur pembelajaran AI.
* **Murid Web**: Student yang terdaftar secara aktif pada jalur pembelajaran Web.
* **Murid Mobile**: Student yang terdaftar secara aktif pada jalur pembelajaran Mobile.
* **Murid Game**: Student yang terdaftar secara aktif pada jalur pembelajaran Game.

## 4. Entitas Program

Entitas Program merepresentasikan kurikulum terpadu yang disediakan oleh LMS:

* **Program AI Development**
* **Program Game Development**
* **Program Web Development and UI/UX Design**
* **Program Mobile Development and UI/UX Design**

---

# Bab 3: Diagram Hubungan Domain (High Level Domain Diagrams)

Bagian ini mendefinisikan visualisasi hubungan struktural antar entitas akademik dalam bentuk diagram Mermaid.

## 1. High Level Domain Relationship

```mermaid
graph LR

ProgramAI["Program AI Development"]
ProgramGame["Program Game Development"]
ProgramWeb["Program Web Development and UI/UX Design"]
ProgramMobile["Program Mobile Development and UI/UX Design"]

MentorAI["Mentor AI"]
MentorGame["Mentor Game"]
MentorWeb["Mentor Web"]
MentorMobile["Mentor Mobile"]
MentorUX["Mentor UI/UX"]
MentorProfessional["Mentor Professional"]

ProgramAI --> MentorAI

ProgramGame --> MentorGame

ProgramWeb --> MentorWeb
ProgramWeb --> MentorUX
ProgramWeb --> MentorProfessional

ProgramMobile --> MentorMobile
ProgramMobile --> MentorUX
ProgramMobile --> MentorProfessional

```

## 2. Mentor Ownership Diagram

```mermaid
graph TD

MentorAI --> StudentAI

MentorGame --> StudentGame

MentorWeb --> StudentWeb

MentorMobile --> StudentMobile

MentorUX --> StudentWeb
MentorUX --> StudentMobile

MentorProfessional --> StudentWeb
MentorProfessional --> StudentMobile

```

## 3. Program Mentor Assignment

```mermaid
graph TD

ProgramAI -->|Primary Mentor| MentorAI

ProgramGame -->|Primary Mentor| MentorGame

ProgramWeb -->|Primary Mentor| MentorWeb
ProgramWeb -->|Secondary Mentor| MentorUX
ProgramWeb -->|Supporting Mentor| MentorProfessional

ProgramMobile -->|Primary Mentor| MentorMobile
ProgramMobile -->|Secondary Mentor| MentorUX
ProgramMobile -->|Supporting Mentor| MentorProfessional

```

## 4. Competency Ownership

```mermaid
graph TD

CompetencyAI --> MentorAI

CompetencyGame --> MentorGame

CompetencyWeb --> MentorWeb
CompetencyWeb --> MentorUX

CompetencyMobile --> MentorMobile
CompetencyMobile --> MentorUX

```

## 5. Mentor Collaboration Diagram

```mermaid
graph LR

MentorAI --- MentorProfessional
MentorAI --- MentorUX

MentorGame --- MentorProfessional
MentorGame --- MentorUX

MentorWeb --- MentorProfessional
MentorWeb --- MentorUX

MentorMobile --- MentorProfessional
MentorMobile --- MentorUX

```

## 6. Conceptual ERD

```mermaid
erDiagram

PROGRAM ||--o{ COMPETENCY : contains

PROGRAM ||--o{ PROGRAM_MENTOR : has

MENTOR ||--o{ PROGRAM_MENTOR : assigned_to

MENTOR ||--o{ COMPETENCY : creates

PROGRAM ||--o{ STUDENT : contains

MENTOR ||--o{ STUDENT_MENTOR : mentors

STUDENT ||--o{ STUDENT_MENTOR : guided_by

PROGRAM {
    UUID program_id
    string program_name
}

MENTOR {
    UUID mentor_id
    string specialization
}

STUDENT {
    UUID student_id
    UUID program_id
}

COMPETENCY {
    UUID competency_id
    UUID program_id
    UUID creator_mentor_id
}

```

---

# Bab 4: Spesifikasi Rinci Program & Aturan Hubungan

Bagian ini menjelaskan secara mendalam aturan operasional, logika pembagian murid, batas hak akses, bentuk kolaborasi, dan kepemilikan personal untuk setiap program di LMS.

## 1. Program AI Development

### Mentor Utama (Primary Mentor)

* **Mentor AI** bertindak sebagai mentor utama program AI Development. Entitas ini dapat diisi oleh lebih dari 1 orang personel yang bergerak bersama sebagai satu tim.

### Logika Distribusi Murid (Student Distribution Logic)

* Seluruh **Murid AI** yang terdaftar wajib dibagi rata secara sistematis berdasarkan jumlah personel **Mentor AI** yang tersedia dalam tim saat itu.

### Penulis Kompetensi (Competency Author)

* Seluruh Kompetensi pada Program AI Development wajib dan hanya boleh dibuat oleh **Mentor AI**.

### Kepemilikan Student (Student Ownership)

* **Mentor AI** memiliki murid AI dan hanya murid AI. **Mentor AI** tidak memiliki hak kepemilikan atas murid dari rumpun program lain.

### Mentor Personal (Personal Mentor)

* **Mentor AI** bertindak sebagai mentor personal untuk murid AI saja (*only*).
* **Murid AI** hanya bisa memiliki mentor AI sebagai personal mentor mereka.
* **Murid AI** secara mutlak hanya boleh memiliki tepat 1 (satu) orang personel **Mentor AI** sebagai personal mentor aktif mereka.

### Kerja Sama dan Kolaborasi (Collaboration)

* **Mentor AI** wajib bekerjasama dengan **Mentor Professional** khusus untuk penyediaan materi *Soft Skills (CCA)* sebagai salah satu Kompetensi.
* **Mentor AI** wajib bekerjasama dengan **Mentor UI/UX** khusus untuk penyediaan materi *Capstone Project* sebagai salah satu kompetensi.

---

## 2. Program Game Development

### Mentor Utama (Primary Mentor)

* **Mentor Game** bertindak sebagai mentor utama program Game Development. Entitas ini dapat diisi oleh lebih dari 1 orang personel yang bergerak bersama sebagai satu tim.

### Logika Distribusi Murid (Student Distribution Logic)

* Seluruh **Murid Game** yang terdaftar wajib dibagi rata secara sistematis berdasarkan jumlah personel **Mentor Game** yang tersedia dalam tim saat itu.

### Penulis Kompetensi (Competency Author)

* Seluruh Kompetensi pada Program Game Development wajib dan hanya boleh dibuat oleh **Mentor Game**.

### Kepemilikan Student (Student Ownership)

* **Mentor Game** memiliki murid Game dan hanya murid Game. **Mentor Game** tidak memiliki hak kepemilikan atas murid dari rumpun program lain.

### Mentor Personal (Personal Mentor)

* **Mentor Game** bertindak sebagai mentor personal untuk murid Game saja (*only*).
* **Murid Game** hanya bisa memiliki mentor Game sebagai personal mentor mereka.
* **Murid Game** secara mutlak hanya boleh memiliki tepat 1 (satu) orang personel **Mentor Game** sebagai personal mentor aktif mereka.

### Kerja Sama dan Kolaborasi (Collaboration)

* **Mentor Game** wajib bekerjasama dengan **Mentor Professional** khusus untuk penyediaan materi *Soft Skills (CCA)* sebagai salah satu Kompetensi.
* **Mentor Game** wajib bekerjasama dengan **Mentor UI/UX** khusus untuk penyediaan materi *Capstone Project* sebagai salah satu kompetensi.

---

## 3. Program Web Development and UI/UX Design

### Struktur Hierarki Mentor Program

* **Primary Mentor**: Mentor Web (Mentor Utama)
* **Secondary Mentor**: Mentor UI/UX (Mentor Kedua)
* **Supporting Mentor**: Mentor Professional (Mentor Ketiga)

### Spesifikasi Tim Mentor Web

* Personel yang menyandang peran sebagai **Mentor Web** dapat berjumlah lebih dari 1 orang sebagai sebuah tim akademik, dan diwajibkan melakukan kolaborasi intensif dengan mentor kedua (UI/UX) dan mentor ketiga (Professional).

### Logika Distribusi Murid (Student Distribution Logic)

* Seluruh volume **Murid Web** yang terdaftar pertama-tama harus dibagi rata berdasarkan jumlah personel **Mentor Web** yang tersedia dalam tim utama.
* Apabila terdapat sisa dari hasil pembagian rata tersebut (pembagian tidak habis), maka sisa jumlah murid tersebut wajib dialokasikan dan dibagikan secara merata kepada **Mentor Professional** dan **Mentor UI/UX**.

### Penulis Kompetensi (Competency Author)

* Seluruh Kompetensi pada Program Web Development and UI/UX Design wajib dibuat secara kolaboratif oleh **Mentor Web** dan **Mentor UI/UX**.

### Kepemilikan Student (Student Ownership)

* **Mentor Web** memiliki murid Web dan hanya murid Web.
* **Mentor UI/UX** memiliki fleksibilitas hak kepemilikan untuk dapat memiliki murid Web sekaligus murid Mobile.
* **Mentor Professional** memiliki fleksibilitas hak kepemilikan untuk dapat memiliki murid Web sekaligus murid Mobile.

### Mentor Personal (Personal Mentor)

* Secara fungsional sistem, **Mentor Web**, **Mentor UI/UX**, dan **Mentor Professional** semuanya memiliki kapasitas untuk menjadi mentor personal dari murid Web.
* Aturan pembatasan mutlak (*constraint*): **Murid Web** hanya bisa memiliki salah satu di antara Mentor Web, Mentor Professional, atau Mentor UI/UX sebagai personal mentor mereka.
* Jumlah personal mentor untuk **Murid Web** dibatasi secara ketat, di mana mereka hanya bisa memiliki tepat 1 (satu) personal mentor aktif dari pilihan kategori tersebut.

### Kerja Sama dan Kolaborasi (Collaboration)

* **Mentor Web** wajib bekerjasama dengan **Mentor Professional** untuk penyediaan materi *Soft Skills (CCA)* sebagai salah satu Kompetensi.
* **Mentor Web** wajib bekerjasama dengan **Mentor UI/UX** untuk penyediaan materi *UI/UX Design* serta materi *Capstone Project* sebagai bagian dari Kompetensi program.

---

## 4. Program Mobile Development and UI/UX Design

### Struktur Hierarki Mentor Program

* **Primary Mentor**: Mentor Mobile (Mentor Utama)
* **Secondary Mentor**: Mentor UI/UX (Mentor Kedua)
* **Supporting Mentor**: Mentor Professional (Mentor Ketiga)

### Spesifikasi Tim Mentor Mobile

* Personel yang menyandang peran sebagai **Mentor Mobile** dapat berjumlah lebih dari 1 orang sebagai sebuah tim akademik, dan diwajibkan melakukan kolaborasi intensif dengan mentor kedua (UI/UX) dan mentor ketiga (Professional).

### Logika Distribusi Murid (Student Distribution Logic)

* Seluruh volume **Murid Mobile** yang terdaftar pertama-tama harus dibagi rata berdasarkan jumlah personel **Mentor Mobile** yang tersedia dalam tim utama.
* Apabila terdapat sisa dari hasil pembagian rata tersebut (pembagian tidak habis), maka sisa jumlah murid tersebut wajib dialokasikan dan dibagikan secara merata kepada **Mentor Professional** dan **Mentor UI/UX**.

### Penulis Kompetensi (Competency Author)

* Seluruh Kompetensi pada Program Mobile Development and UI/UX Design wajib dibuat secara kolaboratif oleh **Mentor Mobile** dan **Mentor UI/UX**.

### Kepemilikan Student (Student Ownership)

* **Mentor Mobile** memiliki murid Mobile dan hanya murid Mobile.
* **Mentor UI/UX** memiliki fleksibilitas hak kepemilikan untuk dapat memiliki murid Web sekaligus murid Mobile.
* **Mentor Professional** memiliki fleksibilitas hak kepemilikan untuk dapat memiliki murid Web sekaligus murid Mobile.

### Mentor Personal (Personal Mentor)

* Secara fungsional sistem, **Mentor Mobile**, **Mentor UI/UX**, dan **Mentor Professional** semuanya memiliki kapasitas untuk menjadi mentor personal dari murid Mobile.
* Aturan pembatasan mutlak (*constraint*): **Murid Mobile** hanya bisa memiliki salah satu di antara Mentor Mobile, Mentor Professional, atau Mentor UI/UX sebagai personal mentor mereka.
* Jumlah personal mentor untuk **Murid Mobile** dibatasi secara ketat, di mana mereka hanya bisa memiliki tepat 1 (satu) personal mentor aktif dari pilihan kategori tersebut.

### Kerja Sama dan Kolaborasi (Collaboration)

* **Mentor Mobile** wajib bekerjasama dengan **Mentor Professional** untuk penyediaan materi *Soft Skills (CCA)* sebagai salah satu Kompetensi.
* **Mentor Mobile** wajib bekerjasama dengan **Mentor UI/UX** untuk penyediaan materi *UI/UX Design* serta materi *Capstone Project* sebagai bagian dari Kompetensi program.

---

# Bab 5: Matriks Kepemilikan (Ownership Matrix)

| Nama Program | Primary Mentor (Team) | Secondary Mentor | Supporting Mentor | Aturan Alokasi Volume Murid | Pembuat Kompetensi (Author) | Aturan Personal Mentor |
| --- | --- | --- | --- | --- | --- | --- |
| **AI Development** | Mentor AI (Bisa > 1 orang) | Mentor UI/UX (Batas: Capstone) | Mentor Professional (Batas: CCA) | Dibagi rata per jumlah Mentor AI yang tersedia | Mentor AI | Tepat 1 orang Mentor AI |
| **Game Development** | Mentor Game (Bisa > 1 orang) | Mentor UI/UX (Batas: Capstone) | Mentor Professional (Batas: CCA) | Dibagi rata per jumlah Mentor Game yang tersedia | Mentor Game | Tepat 1 orang Mentor Game |
| **Web Development and UI/UX Design** | Mentor Web (Bisa > 1 orang) | Mentor UI/UX | Mentor Professional | Dibagi rata ke Mentor Web, sisa pembagian diberikan ke Professional dan UI/UX | Mentor Web + Mentor UI/UX | Tepat 1 pilihan antara Web / Professional / UI/UX |
| **Mobile Development and UI/UX Design** | Mentor Mobile (Bisa > 1 orang) | Mentor UI/UX | Mentor Professional | Dibagi rata ke Mentor Mobile, sisa pembagian diberikan ke Professional dan UI/UX | Mentor Mobile + Mentor UI/UX | Tepat 1 pilihan antara Mobile / Professional / UI/UX |

---

# Bab 6: Aturan Bisnis Mutlak (Business Rules Constraints)

Setiap poin di bawah ini merupakan aturan validasi logika (*hard constraints*) yang wajib ditanamkan pada sistem tanpa toleransi pengecualian:

* **Rule 1**: Murid AI secara eksklusif hanya diperbolehkan terhubung dengan personel dari entitas Mentor AI dalam skema bimbingan personal mentor.
* **Rule 2**: Murid Game secara eksklusif hanya diperbolehkan terhubung dengan personel dari entitas Mentor Game dalam skema bimbingan personal mentor.
* **Rule 3**: Setiap satu individu Murid Web dikunci oleh aturan sistem untuk hanya bisa memilih dan memiliki tepat 1 (satu) orang personal mentor aktif, yang dipilih dari salah satu kategori: Mentor Web, Mentor Professional, atau Mentor UI/UX. Sistem wajib menolak apabila murid memiliki lebih dari satu mentor personal atau tidak memiliki sama sekali.
* **Rule 4**: Setiap satu individu Murid Mobile dikunci oleh aturan sistem untuk hanya bisa memilih dan memiliki tepat 1 (satu) orang personal mentor aktif, yang dipilih dari salah satu kategori: Mentor Mobile, Mentor Professional, atau Mentor UI/UX. Sistem wajib menolak apabila murid memiliki lebih dari satu mentor personal atau tidak memiliki sama sekali.
* **Rule 5**: Otoritas pembuatan dan modifikasi Kompetensi pada Program AI Development secara mutlak berada di bawah kendali eksklusif Mentor AI.
* **Rule 6**: Otoritas pembuatan dan modifikasi Kompetensi pada Program Game Development secara mutlak berada di bawah kendali eksklusif Mentor Game.
* **Rule 7**: Otoritas pembuatan dan modifikasi Kompetensi pada Program Web Development and UI/UX Design dipegang bersama dan hanya diperbolehkan bagi entitas Mentor Web dan Mentor UI/UX.
* **Rule 8**: Otoritas pembuatan dan modifikasi Kompetensi pada Program Mobile Development and UI/UX Design dipegang bersama dan hanya diperbolehkan bagi entitas Mentor Mobile dan Mentor UI/UX.
* **Rule 9**: Hak kepemilikan dan pengelolaan modul instruksional Soft Skills (CCA) sebagai bagian kompetensi program secara mutlak hanya boleh dimiliki dan dikelola oleh Mentor Professional.
* **Rule 10**: Hak kepemilikan dan pengelolaan modul instruksional Capstone Project sebagai bagian kompetensi program secara mutlak hanya boleh dimiliki dan dikelola oleh Mentor UI/UX.
* **Rule 11**: Hak kepemilikan dan pengelolaan materi kurikulum UI/UX Design secara mutlak berada di bawah otoritas penuh Mentor UI/UX (berlaku khusus pada rumpun Program Web dan Program Mobile).
* **Rule 12**: Setiap individu Student hanya boleh terdaftar pada 1 (satu) Program aktif dalam satu satuan waktu tunggal.
* **Rule 13**: Fungsi Primary Mentor memegang peranan tertinggi atas koordinasi struktur akademik makro dari masing-masing program yang dipimpinnya.
* **Rule 14**: Fungsi Secondary Mentor dibatasi secara ketat hanya memiliki hak akses operasional dan tanggung jawab penuh pada pemenuhan aspek kompetensi spesialisasi desain antarmuka dan pengalaman pengguna (UI/UX).
* **Rule 15**: Fungsi Supporting Mentor bertindak sebagai penyedia intervensi kompetensi pendukung (Soft Skills/CCA) lintas program tanpa memiliki hak kepemilikan struktural atas program tersebut.
* **Rule 16**: Logika otomatisasi alokasi pada Backend wajib menerapkan fungsi pembagian rata (pembagian integer tanpa sisa) untuk distribusi murid ke tim utama, dan mengalihkan sisa nilai pembagian (*remainder*) ke tabel penugasan mentor pendukung (UI/UX dan Professional) khusus untuk rumpun Program Web dan Mobile.
* **Rule 17 (Administrator Authority & Delegated Operations)**: Konfigurasi peran otoritas (*System Roles*) dibatasi pada tiga hierarki utama: **Student**, **Mentor**, dan **Admin**. Seluruh eksekusi, inisiasi, dan konfigurasi tingkat sistem adalah wewenang mutlak seorang **Admin**. Namun, pada level operasional program studi yang diampunya, Administrator secara sistemik **mendelegasikan wewenang operasional** kepada **Primary Mentor** untuk melakukan pendaftaran murid (*Student Enrollment*), pengeditan data akademik murid, penonaktifan akun murid, serta distribusi alokasi tim mentor di dalam rumpun program tersebut. Admin tetap memegang hak kendali tertinggi untuk memvalidasi, mengedit, atau membatalkan tindakan operasional tersebut kapan saja.
* **Rule 18 (Role Overlap & Escalation)**: Entitas pengguna dengan jabatan **Admin** diperbolehkan oleh sistem untuk merangkap posisi fungsional sebagai **Mentor** (bertindak ganda sebagai pengelola sistem dan pembimbing akademik). Sebaliknya, seorang **Mentor** tidak memiliki akses otorisasi sebagai **Admin** secara bawaan (*default*). Seorang Mentor hanya dapat diangkat untuk merangkap jabatan sebagai Admin apabila pengajuan tersebut telah disetujui (*approved*) secara eksplisit oleh entitas yang sudah menjabat sebagai Admin sebelumnya.
* **Rule 19 (Safe Deletion & Active Batch Protection)**: Sistem dilarang keras melakukan penghapusan fisik (*Hard Delete*) terhadap akun Mentor maupun Student yang masih terikat secara aktif pada **Batch yang sedang berjalan (`status: 'active'`)**. Penghapusan langsung pada kondisi aktif akan merusak integritas relasi personal mentor dan meninggalkan kelas dalam kondisi tanpa pengajar.
* **Rule 20 (Mandatory Reassignment / Handover Workflow)**: Apabila seorang Mentor pada Batch Aktif harus diganti (karena resign, mutasi, atau hal berhalangan tetap), Administrator wajib melalui alur **Handover / Reassignment** terlebih dahulu. Melalui alur ini, seluruh kelas ajar, antrean penilaian tugas, dan siswa binaan personal yang dimiliki oleh mentor lama dipindahkan secara atomik ke **Mentor Pengganti** sebelum akun mentor lama dapat dinonaktifkan.
* **Rule 21 (Historical Read-Only Preservation & Soft Delete)**: Untuk Batch yang telah berakhir (*Completed Batch*), seluruh data pembelajaran (termasuk kelas, tugas, nilai, dan penugasan mentor) berubah status menjadi **Read-Only (Arsip Historis)**. Terhadap pengguna pada batch yang telah selesai, sistem menerapkan prinsip **Soft Delete / Deactivation (`status: 'inactive'`)**. Jejak nama mentor dan jejak evaluasi di batch lampau tetap dipertahankan secara utuh demi validitas audit dan akreditasi akademik, tanpa mengizinkan akun tersebut untuk login kembali ke dalam aplikasi.
* **Rule 22 (Flexible Program Expansion & Built-in Defaults)**: Pembuatan dan pengelolaan Program Akademik oleh Admin diawali dengan 4 (empat) program bawaan (*default seed*): AI Development, Game Development, Web Development and UI/UX Design, dan Mobile Development and UI/UX Design. Namun, Admin diberikan fleksibilitas penuh untuk menambahkan Program Akademik baru secara dinamis saat membuat atau mengonfigurasi angkatan/batch baru guna mendukung ekspansi kurikulum perusahaan.
* **Rule 23 (Program Batch Read-Only Mode)**: Setiap program terikat pada status siklus hidup Batch (`Persiapan / Draft`, `Berjalan / Active` vs `Selesai / Completed`). Ketika Batch bermarkah `Selesai`, seluruh data program, kelas, penilaian, dan enrollment di dalamnya beralih ke mode **Read-Only** bagi entitas Mentor dan Student. Hak modifikasi pada batch selesai secara mutlak hanya dimiliki oleh Administrator.
* **Rule 24 (Mentor Student CRUD & Deactivation Guardrail)**: Dalam melakukan manajemen murid pada programnya (Case 1 & Case 2), Mentor diperbolehkan melakukan pendaftaran (*enrollment*), pengeditan data akademik, dan penonaktifan akun murid (`status: 'suspended' | 'inactive'`), tetapi **dilarang keras melakukan penghapusan fisik (*Hard Delete*)** dari database.
* **Rule 25 (Cross-Program Clean Transfer & Progress Reset)**: Apabila seorang siswa dipindahkan dari Program A ke Program B (*Case 3 Transfer*), sistem wajib menerapkan **Clean Transfer**. Seluruh riwayat pendaftaran kelas (*enrollments*), nilai, absensi, tugas, dan progres di Program A akan dihapus secara menyeluruh. Siswa yang dipindahkan wajib memulai pembelajaran dari awal di Program B dan mengejar ketertinggalan materi. Admin wajib memberikan konfirmasi peringatan eksplisit mengenai penghapusan progres ini sebelum mengeksekusi pemindahan.
* **Rule 26 (Global Cohort Batch Management & Opt-In Programs)**: Pembuatan dan pengelolaan siklus angkatan (*Batch / Cohort*) dilakukan secara terpusat di tingkat institusi melalui menu khusus **Manajemen Angkatan / Batch**. Saat membuat atau mengonfigurasi sebuah batch, Admin menentukan program studi resmi mana saja yang diikutsertakan (*opt-in checked programs*), termasuk kemampuan menambahkan program baru secara langsung. Seluruh program yang dicentang akan berjalan berbarengan dalam satu rentang periode batch tersebut. Program studi yang tidak diikutsertakan tidak akan terbuka pada siklus batch tersebut.
* **Rule 27 (Single Active Batch Constraint & Lifecycle Flexibility)**: Mutlak hanya boleh ada **1 (satu) Batch yang berstatus `ACTIVE`** dalam satu waktu untuk seluruh program yang berjalan. Sistem menyediakan status **`DRAFT / UPCOMING`** untuk persiapan pendaftaran murid dan mentor pada angkatan masa depan tanpa mengganggu atau mematikan batch yang sedang aktif. Apabila sebuah batch masih dalam kondisi kosong (*Empty Batch* — belum ada materi, tugas, atau progres belajar), Admin diberikan fleksibilitas untuk mengedit, mereset, atau menghapusnya tanpa langsung terbuang ke dalam tabel arsip Read-Only.
* **Rule 28 (Re-Enrollment & Progress Isolation)**: Setiap relasi akademik diikat oleh kombinasi unik `(User, Program, Batch)`. Apabila seorang murid mendaftar ulang (*re-enroll*) pada program studi yang sama di batch/angkatan baru, seluruh progres pembelajaran (materi, tugas, absensi, dan nilai) pada batch baru tersebut wajib dimulai dari 0 (*fresh start*). Riwayat nilai dan progres dari batch lamanya tetap dipertahankan secara utuh di dalam arsip Read-Only batch sebelumnya demi validitas historis.

---

# Bab 7: Catatan Implementasi Sistem Teknis

Dokumen Source of Truth ini menjadi rujukan legal tunggal yang melandasi pengerjaan teknis:

1. **Perancangan Skema Database**: Implementasi *Logical and Physical Database Schema*, pengaturan *Table Constraints*, *Foreign Key*, dan *Integrity Check* untuk memastikan tidak ada relasi silang di luar aturan yang diizinkan.
2. **Spesifikasi Kontrak API**: Pembangunan endpoint yang aman dan terstruktur (seperti *Payload Request* alokasi murid dan validasi pembatasan 1 *Personal Mentor* per murid).
3. **Desain Role-Based Access Control (RBAC) 3 Lapis**: Sistem wajib mendukung arsitektur multi-role yang fleksibel, memetakan tiga entitas akses utama: `ROLE_STUDENT`, `ROLE_MENTOR`, dan `ROLE_ADMIN`. Sistem pengguna (User Table) harus memungkinkan penyimpanan list relasi *many-to-many* untuk otorisasi akses (misal: satu *user_id* memiliki array role `['ADMIN', 'MENTOR']`).
4. **Implementasi Workflow Approval**: Pembangunan antarmuka (*Interface*) dan *Endpoint* khusus bagi Admin aktif untuk melakukan penyetujuan (*Approval Workflow*) terhadap permohonan eskalasi hak akses seorang Mentor yang ingin merangkap menjadi Admin.
5. **Keamanan Data dan Proteksi Lintas Modul**: Desain sistem perizinan yang memblokir keras upaya *Mentor* mengakses modul administratif (*Admin Dashboard*) jika tidak merangkap peran Admin, serta membatasi Mentor dari melihat data program yang tidak menjadi wilayah kekuasaannya.
6. **Implementasi Validasi Aturan Bisnis Otomatis**: Logika backend terpusat yang menjalankan fungsi distribusi *round-robin* atau pembagian rata (modulo) untuk murid, mendistribusikan sisa (*remainder*) dengan akurat, dan menolak form jika menyalahi kepemilikan personal.

---

# Bab 8: Aturan Keselamatan & Siklus Hidup Batch (Safety Rules & Batch Lifecycle)

Demi menjaga stabilitas operasional ratusan siswa dan kesinambungan kurikulum antar batch, sistem menerapkan protokol keselamatan berikut:

1. **Proteksi Batch Aktif (*Active Batch Guardrail*)**:
   * Selama sebuah Batch bermarkah `status = 'active'`, entitas yang berelasi di dalamnya (Kelas, Enrollment, Tugas, dan Mentor Pengajar) berada dalam perlindungan transaksional aktif.
   * Upaya penghapusan paksa (*Hard Delete*) terhadap Mentor aktif akan ditolak oleh sistem dan dialihkan ke fitur **Reassignment (Pemindahan Mentor)**.
2. **Alur Pemindahan Tugas (*Handover Workflow*)**:
   * Admin memilih Mentor Lama -> Pilih Mentor Pengganti dari spesialisasi program yang sama -> Sistem memindahkan `mentorId` pada kelas dan senarai siswa binaan secara instan -> Mentor Lama dapat dinonaktifkan dengan aman.
3. **Kekebalan Arsip Historis (*Read-Only Historical Batch*)**:
   * Saat Batch berakhir dan diubah menjadi `status = 'completed'`, seluruh entitas akademik di dalamnya dikunci (*Read-Only*).
   * Perubahan status akun di masa depan (baik mentor maupun admin yang nonaktif/resign) **tidak akan mempengaruhi atau menghapus jejak sejarah** pada batch lampau. Sistem menjamin kekebalan arsip (*historical immutability*).
4. **Siklus Hidup & Riwayat Batch per Program (*Per-Program Batch Lifecycle*)**:
   * Setiap program studi memiliki riwayat batch independen.
   * Pembuatan batch baru oleh Admin secara otomatis mengarsipkan batch aktif sebelumnya menjadi `status = 'completed'` (Read-Only Mode) dan memulai periode baru yang bersih (*fresh start*).

---

# Bab 9: Manajemen Program, Enrollment, & Delegasi Otoritas (Program & Mentee Management)

Bab ini mengatur alur pendaftaran siswa binaan dan pembagian tanggung jawab operasional antara Admin dan Mentor:

1. **Alur Pendaftaran Murid Binaan (*Enrollment Cases*)**:
   * **Case 1 (No-Program Mentor & No-Program Student)**: Pendaftaran siswa yang belum memilih program ke mentor yang belum memiliki alokasi kelas. Sistem otomatis mengikat program studi pada kedua akun tersebut dan membuat *enrollment* kelas.
   * **Case 2 (Active Program Mentor & No-Program Student)**: Pendaftaran siswa tanpa program ke mentor yang sudah mengampu program aktif. Siswa otomatis terdaftar ke program yang sama dengan mentor tersebut.
   * **Case 3 (Cross-Program Student Transfer)**: Pemindahan siswa yang sudah ber-program ke program baru. Sesuai **Rule 25**, sistem menjalankan **Clean Transfer** yang menghapus seluruh progres, absensi, dan nilai di program lama agar siswa memulai lembaran baru di program tujuan.
2. **Delegasi Wewenang Operasional (*Delegated Authority*)**:
   * Sesuai **Rule 17**, Admin mendelegasikan wewenang operasional kepada *Primary Mentor* untuk melakukan pendaftaran siswa (Case 1 & Case 2), pengeditan data akademik, serta penonaktifan siswa di dalam programnya sendiri.
   * Mentor dilarang melakukan *Hard Delete* (Rule 24). Hak penghapusan dan override lintas program secara mutlak tetap dipegang oleh Administrator.
3. **Distribusi Otomatis Modulo & Round-Robin (*Web & Mobile Tracks*)**:
   * Khusus pada program kolaboratif (Web & Mobile Development), sistem menyediakan eksekusi otomatis yang membagi siswa secara rata (*Round-Robin*) kepada tim Mentor Utama, dan mengalihkan sisa pembagian (*remainder*) kepada Mentor UI/UX dan Mentor Professional sesuai **Rule 16**.