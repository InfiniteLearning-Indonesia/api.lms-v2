# API LMS v3 Go untuk frontend

Pakai [OpenAPI inti](api/openapi/lms-v3-core.json) untuk tipe dan referensi per operasi. Dokumen ini menjelaskan cara merangkai panggilan untuk tiga alur integrasi awal di `v3-local-preview`. Kolom izin di bawah merujuk ke tindakan dalam Class yang sedang dibuka. Untuk fitur di luar cakupan awal, mulai dari [peta modul](docs/MODULE_CHECKPOINT_MAP.md) dan [peta integrasi](docs/FRONTEND_INTEGRATION_ROADMAP.md). [Standar OpenAPI 3.1](https://spec.openapis.org/oas/v3.1.0.html)

## Koneksi dan aturan request

- Origin preview default `https://127.0.0.1:9443`. Jalankan preview dari terminal pemilik proyek dan gunakan HTTPS dengan sertifikat preview sesuai panduan. `GET /health/live` menunjukkan proses hidup. Saat siap, `GET /health/ready` mengembalikan HTTP 200 dengan `{"status":"ready"}`. Saat belum siap, responsnya HTTP 503. [Menjalankan preview lokal](docs/LOCAL_PREVIEW.md#menjalankan-preview-manual)
- Gunakan path relatif seperti `/v3/auth/me`, dengan `credentials: 'same-origin'`. Go menerima cookie `__Host-lms-v3` dengan Secure, HttpOnly, SameSite=Strict dan Path=/. JavaScript menerima `csrf_token` dari login, Google complete dan rotate. Kirim `X-CSRF-Token` pada mutasi bersesi. Simpan CSRF terpisah dari cookie, misalnya di `sessionStorage` per tab, lalu ganti setelah rotasi dan bersihkan setelah logout.
- Gateway meminta HTTPS, menolak header `Authorization` dan query string pada route `/v3`, serta memeriksa `Origin` yang persis sama pada mutasi, termasuk mutasi sebelum login. Browser mengirim `Origin` secara otomatis. Rute halaman `/auth/google/callback` menerima query Google dan meneruskan `code` dan `state` melalui body JSON. Jika memakai proxy untuk frontend produk, teruskan path API ke Go dan verifikasi TLS di sisi Go serta origin yang diterima gateway.
- Mutasi JSON memakai `Content-Type: application/json`. Untuk aksi tanpa field, kirim `{}`. Handler menolak field yang tidak dikenal. Untuk route domain bertanda **Key**, kirim `Idempotency-Key` dengan 8–80 karakter `A–Z a–z 0–9 _ -`. Buat key per aksi logis. Simpan key bersama payload selama retry. Key dan payload yang sudah berhasil dapat mengembalikan respons sebelumnya selama akses masih diizinkan. Gunakan key baru untuk aksi atau payload yang berubah.
- Perlakukan ID sebagai string opak. Pakai `version`, `attempt_version`, `expected_version` atau `expected_publication_revision` sesuai operasi dan respons terakhir. JSON mutation konten saat ini dibatasi **4 KiB termasuk seluruh body**. Jadwal opsional memakai timestamp RFC 3339 dan zona IANA seperti `Asia/Jakarta`.

Rujukan kode [gateway sesi](internal/identity/http.go) · [router preview](internal/localpreview/http.go) · [health handler](internal/platform/httpserver/server.go).

## 1. Login dan sesi

| Metode dan path | Body | Akses | Sukses |
|---|---|---|---|
| `POST /v3/auth/password/login` | `email`, `password` | Akun aktif punya password | 200 `{csrf_token}` dan cookie |
| `GET /v3/auth/me` | — | Sesi | 200 `{id,site_admin}` |
| `POST /v3/auth/rotate` | `{}` | Sesi + CSRF | 200 `{csrf_token}` dan cookie baru. Sesi sebelumnya dicabut. |
| `POST /v3/auth/logout` | `{}` | Sesi + CSRF | 200 `{"status":"revoked"}` dan cookie dihapus |
| `POST /v3/auth/revoke-all` | `{}` | Sesi + CSRF | 200 `{"status":"revoked"}`. Semua sesi akun dicabut. |
| `POST /v3/auth/accounts/invite` | `email` | Site Admin + CSRF | 202 `{user_id,status:"invited"}`. Jalur undangan akun tanpa Class. |
| `POST /v3/auth/password/invite/accept` | `token`, `password` | Token aktivasi akun | 200 `{"status":"password_set"}`, lanjutkan ke login |
| `POST /v3/auth/password/reset/request` | `email` | Origin aplikasi | 202 `{"status":"accepted"}`. Respons ini tidak menunjukkan apakah akun ada. |
| `POST /v3/auth/password/reset/confirm` | `token`, `password` | Token reset | 200 `{"status":"password_set"}`, lanjutkan ke login |

Password baru minimal 15 karakter Unicode dan maksimum 1.024 byte UTF-8. Setelah reload, cek `/me` dan baca CSRF yang masih tersimpan di tab. `/me` hanya mengembalikan ID dan flag Site Admin. Jika cookie valid tetapi tab kehilangan CSRF, minta login lagi sebelum mutasi. Rotasi juga meminta CSRF saat ini. Untuk role Class, baca `/v3/me/classes`.

### Google Sign-In

1. Panggil `POST /v3/auth/google/start`. Gunakan `{"mode":"login"}` untuk akun yang sudah ditautkan, `{"mode":"invite","invite_token":"…"}` untuk aktivasi akun, atau `{"mode":"link","email":"…","password":"…"}` dengan sesi dan CSRF untuk menautkan akun yang sudah aktif.
2. Buka `authorization_url` dari respons 200 melalui navigasi browser. Go menetapkan cookie binding `__Host-lms-v3-google`. Halaman callback pada origin yang sama membaca `code` dan `state`, lalu mengirim keduanya ke `POST /v3/auth/google/complete`. Pertahankan cookie sesi yang memulai alur link. Kirim kedua field lewat JSON dan hindari menyimpan URL callback, code atau token pada log.
3. Respons 200 memberi `csrf_token` dan sesi Go. Lanjutkan dengan `GET /v3/auth/me`. Jika ini aktivasi dari undangan Class, lanjutkan dengan acceptance Class di bagian berikutnya. Route Google mengembalikan 404 `OWNER_ROUTE_NOT_AVAILABLE` jika provider dimatikan. QA Google yang tercatat memakai provider sintetis. [Kontrak identitas dan provider](docs/AUTHENTICATION_V3.md)

### Contoh client TypeScript

```ts
type Write = {
  method: 'POST' | 'PATCH'
  body: object
  csrf?: string
  key?: string
}

async function api<T>(path: `/v3/${string}`, write?: Write): Promise<T> {
  const headers = new Headers()
  if (write) headers.set('Content-Type', 'application/json')
  if (write?.csrf) headers.set('X-CSRF-Token', write.csrf)
  if (write?.key) headers.set('Idempotency-Key', write.key)

  const response = await fetch(path, {
    method: write?.method ?? 'GET',
    credentials: 'same-origin',
    headers,
    body: write && JSON.stringify(write.body),
    cache: 'no-store',
  })
  const json = response.headers.get('Content-Type')?.includes('application/json')
  const result = json ? await response.json() : null
  if (!response.ok) {
    throw new Error(result?.error ?? `HTTP_${response.status}`)
  }
  if (!json) throw new Error('Respons API bukan JSON')
  return result as T
}

async function login(email: string, password: string) {
  const { csrf_token } = await api<{ csrf_token: string }>('/v3/auth/password/login', {
    method: 'POST',
    body: { email, password },
  })
  sessionStorage.setItem('lms-v3:csrf', csrf_token)
  return api<{ id: string; site_admin: boolean }>('/v3/auth/me')
}

// Buat sekali untuk satu aksi. Reuse objek ini jika mengulang request yang sama.
function classCreation(name: string): Write {
  const csrf = sessionStorage.getItem('lms-v3:csrf')
  if (!csrf) throw new Error('Masuk kembali sebelum melanjutkan')
  return { method: 'POST', body: { name }, csrf, key: crypto.randomUUID() }
}
```

Gunakan JSON client ini pada route inti. Laporan CSV dan unduhan file memiliki respons biner atau teks sesuai kontrak masing-masing. [Kontrak reporting dan CSV](api/openapi/reporting-v1.json)

## 2. Admin membuat Class dan mengundang Mentor atau Mentee

| Metode dan path | Akses | Body | Sukses |
|---|---|---|---|
| `POST /v3/classes` | Site Admin · Key | `name`, opsional `program_label`, `cohort_label` | 200 Class `DRAFT` |
| `GET /v3/me/classes` | Sesi | — | 200 `{classes:[...]}` dengan `roles` per Class |
| `GET /v3/classes/{class_id}` | Site Admin atau anggota yang diizinkan | — | 200 Class tanpa `roles` |
| `PATCH /v3/classes/{class_id}` | Site Admin atau Teacher di Class · Key | `name`, `version`, opsional label | 200 Class dengan versi baru |
| `POST /v3/classes/{class_id}/account-invitations` | Site Admin · Key | `email`, `role` | 200 `{id,class_id,user_id,role,status:"INVITED"}` |
| `GET /v3/me/class-invitations` | Sesi penerima | — | 200 `{invitations:[...]}`, khusus undangan tertunda akun itu |
| `POST /v3/classes/{class_id}/invitations/{invitation_id}/accept` | Sesi penerima · Key | `{}` | 200 `{invitation_id,class_id,enrollment_id,episode_id,role,status:"ACCEPTED"}` |
| `POST /v3/classes/{class_id}/publish` | Site Admin atau Teacher di Class · Key | `version` | 200 Class `PUBLISHED` bila syarat terpenuhi |

1. Admin membuat Class dengan nama yang diperlukan. Label program dan cohort adalah teks opsional. Simpan `class.id` dan `class.version` untuk aksi berikutnya.
2. Pilihan **Invite Mentor** mengirim `{"email":"mentor@example.invalid","role":"teacher"}`. Pilihan **Invite Mentee** mengirim `role:"student"` pada endpoint yang sama. Untuk akun yang baru diprovisikan, penerima mengaktifkan akun terlebih dahulu dengan password atau Google. Preview menyediakan token aktivasi melalui mailbox privat. ID undangan Class dipakai untuk acceptance setelah login.
3. Penerima login, membaca undangannya sendiri, lalu menerima undangan. Backend menambahkan participation dan role Class. Setelah minimal satu Teacher aktif, Site Admin atau Teacher di Class tersebut dapat memublikasikan Class. Student mulai melihat Class yang telah dipublikasikan.

Daftar Class diurutkan berdasarkan nama, lalu ID, dan dibatasi 250. Inbox Class berisi maksimal 100 undangan terbaru. Keduanya memakai route GET tanpa pagination saat ini. `roles: []` sah untuk Class yang bisa dilihat Site Admin tanpa role di Class tersebut. Flag `site_admin` tetap berasal dari `/me`. Pengaturan personal mentor adalah aksi terpisah dari undangan Teacher. [Handler Class](internal/classes/http.go) · [Akses Class](internal/access/capabilities.go) · [Undangan](internal/classes/invitations.go)

## 3. Teacher menerbitkan tugas, Student menyerahkan, Teacher merilis nilai

Pada tabel berikut, prefix semua path adalah `/v3/classes/{class_id}`. Semua POST dan PATCH di tabel menggunakan sesi, CSRF dan **Key**.

| Metode dan path relatif | Pemakai utama | Body atau versi yang dipakai | Sukses |
|---|---|---|---|
| `GET /content` | Anggota Class sesuai akses | — | 200 snapshot sections, activities, revisions dan assignment rules |
| `POST /sections` | Teacher | `title`, `position` | 201 section |
| `PATCH /sections/{section_id}` | Teacher | `title`, `position`, `version` | 200 section |
| `POST /activities` | Teacher | `section_id`, `activity_type:"ASSIGNMENT"`, `title`, `position` | 201 activity |
| `PATCH /activities/{activity_id}` | Teacher | `title`, `position`, `version` | 200 activity |
| `POST /activities/{activity_id}/revisions` | Teacher | `raw_body`, `timezone`, `assignment`, jadwal opsional | 201 revisi konten |
| `PATCH /activities/{activity_id}/revisions/{revision_id}` | Teacher | Data revisi draft dengan `version` | 200 revisi |
| `POST /activities/{activity_id}/revisions/{revision_id}/publish` | Teacher | `version` revisi konten | 200 revisi terbit |
| `GET /activities/{activity_id}/revisions/{revision_id}` | Anggota Class sesuai akses | — | 200 `{revision,rule}` untuk assignment |
| `POST /activities/{activity_id}/revisions/{revision_id}/submissions` | Student aktif | `{}` | 200 aggregate `Submission` dan attempt awal |
| `GET /submissions/{aggregate_id}` | Pemilik atau Teacher yang berwenang | — | 200 pengumpulan dengan attempt dan revisi |
| `POST /submissions/{aggregate_id}/revisions` | Student pemilik | `version` aggregate, `body`, `file_asset_ids` | 200 revisi jawaban |
| `POST /submissions/{aggregate_id}/handin` | Student pemilik | `version` aggregate, `attempt_version` | 200 receipt dengan `revision_id` yang diserahkan |
| `POST /grade-items` | Teacher | `activity_id`, `title`, `weight_bps`, `scale`, `criteria` | 201 definisi dengan `grade_item_id` dan `definition_version_id` |
| `GET /grade-items/{grade_item_id}` | Teacher | — | 200 array definisi yang diurutkan per versi |
| `POST /grade-items/{grade_item_id}/versions` | Teacher | `weight_bps`, `scale`, `criteria` | 201 versi definisi baru |
| `POST /grade-items/{grade_item_id}/versions/{definition_version_id}/publish` | Teacher | `expected_publication_revision`, kirim `0` untuk publikasi pertama | 200 publikasi dengan nomor `revision` |
| `POST /grade-items/{grade_item_id}/grades` | Teacher | `aggregate_id`, `submission_revision_id`, `expected_version`, `value_milli`. Pada koreksi sertakan `reason`. | 200 nilai tercatat |
| `POST /grade-items/{grade_item_id}/grades/{aggregate_id}/release` | Teacher | `expected_version` dari respons nilai | 200 release |
| `GET /grade-items/{grade_item_id}/grades/{aggregate_id}` | Teacher berwenang atau learner pemilik | — | 200 nilai sesuai izin. Untuk learner, nilai belum dirilis dapat menghasilkan 404. |
| `GET /reports/grades/{learner_user_id}` | Learner sendiri, Teacher dalam Class atau Site Admin | — | 200 `{contract_version,class_id,learner_user_id,watermark,grades}` |

**Payload tugas teks** untuk membuat revisi konten.

```json
{
  "raw_body": "Jelaskan perbedaan role Site Admin dan Teacher.",
  "timezone": "Asia/Jakarta",
  "assignment": {
    "submission_mode": "TEXT",
    "max_attempts": 2,
    "max_files": 0,
    "max_file_bytes": 0,
    "late_policy": "REJECT_AFTER_CUTOFF"
  }
}
```

- Untuk Student, tampilkan isi dari revisi terbit yang tersedia pada `/content`. Snapshot juga berisi metadata struktur. Susun UI dari revisi yang dikembalikan oleh Go. Published assignment dan submission menyimpan referensi revisi tertentu. Gunakan `canonical_body` dari response konten sesuai `content_format`; author mengirim isi awal sebagai `raw_body`.
- Saat menambah jawaban teks, pakai `file_asset_ids: []`. Setelah append, ambil versi aggregate dan attempt dari create atau GET terakhir; object revisi jawaban membawa `revision_no`, yang berbeda dari kedua versi itu. Handin menyerahkan revisi terakhir pada attempt dan responsnya memberi receipt. Untuk grading, ambil `submission_revision_id` dari `receipt.revision_id`. Bedakan ID revisi jawaban dari ID revisi konten.
- `value_milli: 85000` berarti 85 poin. `weight_bps: 10000` berarti 100%. Buat definisi nilai dengan skala dan kriteria, publish versi definisi, lalu catat nilai. `expected_version: 0` berarti nilai pertama. Jika memperbaiki nilai, kirim versi yang berlaku dan alasan. Release menjadikan versi nilai dapat dibaca learner. Daftar released grades yang kosong sah untuk learner tanpa nilai terbit. `watermark` bisa `null`.
- Jalur QA mendapat ID submission dan item nilai sebagai respons dari aksi create. **Gap integrasi produk:** kontrak ini belum menyediakan daftar pengumpulan atau daftar item nilai per Class yang bisa dipakai Mentor untuk membuka pekerjaan lama dari nol. GET yang tersedia di tabel membutuhkan ID. Sebelum mengaktifkan layar inbox grading atau navigasi berbasis item nilai, cocokkan kebutuhan layar dengan read model yang ada dan tambahkan daftar terlingkup bila diperlukan.

Rujukan kode [konten](internal/content/store.go) · [pengumpulan](internal/submissions/store.go) · [nilai](internal/gradebook/store.go) · [laporan](internal/reporting/report.go).

## 4. Izin per Class dan penanganan error

| Skenario | Hasil yang dipakai UI |
|---|---|
| Site Admin mengundang akun lewat email ke Class | Aksi tersedia untuk Site Admin. Teacher memakai jalur Class yang diizinkan oleh capability server. |
| Satu akun Teacher di A dan Student di B | Tampilkan kontrol author hanya di A. `POST /v3/classes/{id}/sections` dari akun tersebut memberi 201 di A dan 403 di B pada skenario browser yang diuji. |
| Penerima Class berbeda mencoba accept | Backend membatasi acceptance pada pemilik undangan. |
| Student mengakses nilai yang dicatat tetapi belum dirilis | Tangani 404 untuk detail dan daftar released grades yang masih kosong. UI memakai hasil server sebagai dasar visibility. |

Error normal berbentuk `{"error":"KODE"}` dengan `request_id` opsional pada handler domain. Simpan `X-Request-ID` jika ada untuk diagnostik, terutama saat menampilkan error generik. Tangani juga kegagalan jaringan atau respons proxy tanpa JSON.

| HTTP | Kode umum | Penanganan di UI |
|---|---|---|
| 400 | `INVALID_INPUT` | Periksa input dan bentuk JSON. |
| 401 | `UNAUTHENTICATED` | Periksa sesi atau kredensial. Dalam integrasi origin, periksa TLS, path tanpa query dan cookie browser. |
| 403 | `FORBIDDEN` | Periksa izin aktor, origin atau CSRF sesuai request. |
| 404 | `NOT_FOUND`, `OWNER_ROUTE_NOT_AVAILABLE` | Laporkan resource belum tersedia atau sembunyikan opsi provider yang dimatikan. |
| 409 | `CONFLICT`, `DEADLINE_PASSED` | Baca ulang state/versi jika konflik. Tampilkan deadline jika terlambat. |
| 413 | `REPORT_TOO_LARGE` | Gunakan endpoint halaman pada kontrak reporting jika laporan butuh paging. |
| 429 | `RATE_LIMITED` | Beri jeda sebelum mengulang aksi autentikasi. |
| 503 | `UNAVAILABLE` | Periksa readiness dan coba ulang sesuai jenis operasi. |

Create konten mengembalikan **201** pertama kali dan **200** pada replay. Create item atau versi nilai tetap **201**. Class dan submission pada alur ini memakai **200**. Submission dan gradebook dapat mengirim `Idempotency-Replayed: true` saat replay. Gunakan status dan bentuk response dari spesifikasi masing-masing.

## Bukti dan pekerjaan yang tersisa

Gunakan [checklist integrasi frontend produk](docs/INTEGRATION_CHECKLIST.md) untuk memetakan layar dan [indeks QA](docs/QA_EVIDENCE.md) untuk merekam hasil acceptance.

- Pemeriksaan statis mencakup 42 operasi, path parameter, referensi schema, contoh request/error dan status untuk 22 operasi HTTP yang dipakai alur browser. Contoh TypeScript lulus kompilasi strict. Tes Go lokal untuk health, identity, Class, konten, submission, gradebook dan reporting lulus. Bukti Playwright dan DB pada poin berikut berasal dari run preview yang sudah tercatat.
- Sudah ada bukti browser **3/3** untuk undangan Class, tugas hingga nilai dirilis, dan role kontekstual lintas Class pada `/fe-test`. QA API preview **7/7**, termasuk cookie/CSRF dan logout. QA identitas M04 punya bukti terpisah untuk password, Google sintetis dan undangan Class. [Bukti preview](docs/LOCAL_PREVIEW.md) · [Bukti M04](docs/modules/M04.md)
- Tiga pekerjaan dalam roadmap adalah **jalur kerja** yang mencakup beberapa modul. Integrasi lokal dimulai dari M14 bersama M04/M05/M07/M08/M09. Daftar yang diperlukan layar produk ditentukan saat pencocokan layar terhadap kontrak. [Peta integrasi](docs/FRONTEND_INTEGRATION_ROADMAP.md)
- Runtime bersama dan backup/restore melibatkan M00/M01/M03, serta penerimaan provider nyata M04. M15/M16 perlu menyelesaikan data historis dan rehearsal, termasuk M06-CP04 untuk R2, M08-CP04 untuk kronologi submission, dan M12-CP04 untuk logbook sesuai cakupan migrasi. M02 tetap relevan untuk keamanan legacy dan transisi identitas. M11-CP02–04 terbuka bila presensi/izin masuk gelombang awal. [Peta modul](docs/MODULE_CHECKPOINT_MAP.md)
- M17 dan M18 menangani pilot dan rollout. M19 masih opsional. Fitur lain seperti berkas, progress, transkrip, notifikasi, presensi, logbook, AI dan laporan lanjutan membutuhkan penyesuaian scope dan bukti per layar yang akan dipakai. [Kontrak reporting lengkap](api/openapi/reporting-v1.json)
