# M04-CP05 — Undangan Class dan otorisasi lintas resource

## Scope dan bukti wajib

CP04 diterima pada run `20261122T183531Z-m04-google-92d85dc7d653`. CP05 menghubungkan akun undangan M04 ke Class M05 dan menggunakan policy resource yang sudah ada.

| Kasus | Bukti wajib |
|---|---|
| CLASS-01 | Site Admin mengundang akun baru dengan email dan peran Teacher/Student. Provision akun, undangan Class, audit dan outbox menggunakan satu transaksi. Class yang sudah ditutup dan aktor non-admin ditolak. |
| CLASS-02 | Penerima hanya melihat undangan miliknya. Login dan aktivasi akun tidak memberi akses sebelum acceptance. Acceptance terikat Class dan akun, dan penolakan/replay tidak membuat role atau episode tambahan. |
| CLASS-03 | Undangan kedua memakai ID akun yang sama, menjaga peran terpisah per Class. Gagal menyimpan efek audit/outbox membatalkan pembuatan akun dan undangan. |
| CLASS-04 | Role Teacher/Student yang diperoleh dari undangan diuji dengan handler nyata file, content, submission dan grade. Uji partner Class, scope Student, dan ID resource Class lain. |
| CLASS-05 | QA Go/PostgreSQL dan Playwright API pada cluster localhost baru, dengan backup dan restore target kosong, bukti source dan shutdown. |

## Kontrak integrasi

| Endpoint | Syarat dan respons |
|---|---|
| `POST /v3/classes/{class_id}/account-invitations` | Site Admin, cookie sesi, origin, `X-CSRF-Token`, dan `Idempotency-Key`. JSON hanya `email` dan `role` (`teacher` untuk Invite Mentor, `student` untuk Invite Mentee). Respons 200 memuat ID undangan, akun, Class, role, dan status `INVITED`. |
| `GET /v3/me/class-invitations` | Cookie sesi aktif. Mengembalikan `invitations` berisi paling banyak 100 undangan terbaru milik sesi. Isinya ID, Class, role, dan status. Undangan Class closed/archived serta yang sudah diterima tidak tampil. |
| `POST /v3/classes/{class_id}/invitations/{invitation_id}/accept` | Cookie sesi milik akun target, origin, CSRF, dan idempotency key. Body `{}`. Respons 200 memuat `invitation_id`, `class_id`, `enrollment_id`, `role`, `status` dan `episode_id`. `episode_id` dapat kosong untuk enrollment yang sudah aktif. |

Akun baru dibuat pending dan mendapat antrean aktivasi M04. Ia dapat mengaktifkan akun lewat password atau Google sesuai aturan CP03/CP04, lalu login dan melihat undangannya. Akun pending yang ditambahkan ke Class lain memakai ID dan token aktivasi yang masih berlaku. Akun aktif memakai ID yang sama dan mendapat undangan di aplikasi. Tautan Class diotorisasi oleh sesi target dan ID undangan yang terikat ke Class. Acceptance membuat atau memakai ulang enrollment, menerapkan role tersimpan, mencatat episode bila diperlukan, dan menulis bukti audit dalam transaksi yang sama. Replay dengan key dan payload yang sama menerima respons asli. Setelah undangan diterima, key baru mendapat konflik.

Endpoint lama `/v3/classes/{class_id}/invitations` tetap melayani undangan berdasarkan `user_id` aktif dengan capability Class. Site Admin yang mengundang lewat email dapat memakai kembali undangan pending pada tuple `(Class, akun, role)` tanpa membuat audit Class kedua. Pada endpoint lama, undangan pending duplikat tetap konflik. Peran Teacher pada suatu Class tidak memberi akses Teacher di Class lain. Personal mentor masih penugasan terpisah.

Handler CP05 memakai `classes.NewWithAccountInvitations` di belakang gateway identity. Fixture QA memakai policy asli untuk metadata file, revisi content, ownership submission dan definisi gradebook. Perilaku nilai yang sudah dirilis tetap dicakup regresi M09. Deliverability email, pemasangan runtime aplikasi, dan pencocokan akun historis M15 masih menunggu integrasi. Jalur email memakai direktori akun Go M04, sehingga email akun historis harus melalui mapping yang ditinjau sebelum mengundang pengguna hasil impor.

## Status dan cara verifikasi

**`PASS_M04_CP05_LOCAL_SYNTHETIC_ONLY`.** Run terminal `20261122T190140Z-m04-class-invitations-d579f0866558` lulus `TestM04ClassInvitationPostgresJourney` dan 5/5 probe API Playwright. Review mencocokkan log Go, hash source `08f64d626efaad45317728b64da886ec48d07d0361f6b8bcd43e0b8fcce16061`, hash kedua file backup, receipt pemulihan target kosong dan shutdown PostgreSQL. **M04 mencapai 5/5 checkpoint lokal sintetis.**

Run sebelumnya `20261122T185029Z-m04-class-invitations-96e688f39825` memverifikasi restore dan shutdown, lalu berhenti saat fixture revisi konten belum mengisi `content_format`. Perbaikan fixture menetapkan `SAFE_HTML` dan `UTC`, menyimpan aturan tugas saat revisi masih `DRAFT`, lalu memublikasikannya sesuai constraint M07. Run yang lulus menggantikan hasil gagal tersebut sebagai bukti CP05.

```powershell
D:\Projects\lms-v2\backend-v3\backend-go\qa\RUN_M04_CP05_CLASS_QA.cmd
```

Runner tersedia untuk regresi. [M14](../modules/M14.md#uji-manual-lokal-sebelum-handover) merujuk kontrak ini untuk persiapan `/fe-test`. Google dan email nyata, runtime bersama, dan pencocokan akun historis tetap memerlukan verifikasi integrasi.
