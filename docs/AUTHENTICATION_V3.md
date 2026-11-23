# Autentikasi v3 untuk akun undangan

Site Admin membuat akun. Setiap akun yang sudah diverifikasi bisa memakai email + password, Google Sign-In, atau keduanya. Satu ID akun dipakai untuk sesi, Class, dan histori.

Moodle memisahkan pembuatan akun dari enrollment. Akun manual dapat dibuat pengelola, registrasi sendiri adalah opsi terpisah, dan login provider dapat dikonfigurasi agar tidak menciptakan akun. V3 mengadopsi pemisahan ini dengan email sebagai pengenal login lokal. Dua credential pada akun LMS yang sama adalah kontrak v3. [Akun dan enrollment Moodle](https://docs.moodle.org/503/en/Add_users) · [Autentikasi OAuth Moodle](https://docs.moodle.org/503/en/OAuth_2_authentication) · [FAQ Moodle](https://docs.moodle.org/503/en/Authentication_FAQ)

| Alur | Kontrak v3 |
|---|---|
| Invite Mentor | Site Admin memilih Class dan email. Acceptance memberi Teacher di Class itu. Personal mentor tetap penugasan terpisah. |
| Invite Mentee | Site Admin memilih Class dan email. Acceptance memberi Student dan participation di Class itu. |
| Akun baru | Admin membuat akun invited. Penerima membuktikan akses ke undangan lalu menetapkan password, menghubungkan Google, atau memakai keduanya. |
| Akun existing | Pengguna menerima undangan sebagai akun target. Penugasan memakai ID akun yang sama. Hak di kelas lain mengikuti penugasannya masing-masing. |
| Google pertama kali | Login memerlukan link provider yang ada atau penautan melalui undangan terverifikasi yang mengikat akun target atau reautentikasi akun. Callback dan reset tidak membuat akun. Pencocokan email saja tidak memberi akses. |

**M04** memiliki akun, undangan akun, password credential opsional, external identity, reset dan sesi. Token undangan/reset bersifat acak, di-hash, sekali pakai, punya tujuan, target dan masa berlaku. Gunakan Argon2id dengan parameter yang diuji, rate limit, respons reset yang tidak membocorkan keberadaan akun, serta pencabutan sesi saat reset atau suspend. [OWASP password storage](https://cheatsheetseries.owasp.org/cheatsheets/Password_Storage_Cheat_Sheet.html) · [OWASP reset](https://cheatsheetseries.owasp.org/cheatsheets/Forgot_Password_Cheat_Sheet.html)

Implementasi lokal pertama memakai PBKDF2-HMAC-SHA256 600.000 iterasi dan menyimpan identifier skema pada credential. Alasan perubahan dan bukti QA tercatat pada checkpoint. [M04-CP03 password](checkpoints/2026-11-22-m04-cp03-password.md)

Google memakai alur authorization code dengan redirect URI tetap `<origin>/auth/google/callback`, state, nonce dan PKCE. Halaman callback mengirim code dan state lewat POST HTTPS ke API origin yang sama. Backend memvalidasi signature, issuer, audience, expiry dan nonce. Identitas yang ditautkan adalah pasangan issuer/subject unik. Kedua jalur menerbitkan sesi LMS yang memakai kebijakan account state, logout, CSRF dan revocation yang sama. [Google OpenID Connect](https://developers.google.com/identity/openid-connect/openid-connect) · [Kontrak API dan QA CP04](checkpoints/2026-11-23-m04-cp04-google.md)

**M05** memiliki acceptance undangan Class. Class, role dan akun target terikat pada undangan. Akun target yang sudah login menjalankan acceptance lewat sesi dan CSRF. Perubahan status undangan, participation, role, audit dan outbox terjadi dalam satu transaksi. [Endpoint dan QA M04-CP05](checkpoints/2026-11-23-m04-cp05-class-invitations.md)

**M15** membawa identitas historis dari ID legacy. Password hash, subject Google, sesi dan token provider dalam dump tetap di baseline. Default usulan aktivasi password lama adalah set/reset terverifikasi. Reuse hash membutuhkan evaluasi kompatibilitas terpisah. Site Admin legacy mengikuti keputusan provisioning tersendiri.

**M14** menyiapkan OpenAPI, contoh dan bukti tes kedua metode login, admin invite/accept, reset, logout, cookie dan CSRF. Proyek backend membuktikan kontrak lewat API/DB. Pemilik proyek meneruskan frontend produk dan menguji pengalaman pengguna saat integrasi dan pilot. [Peta integrasi](FRONTEND_INTEGRATION_ROADMAP.md)

Pembuktian inti data lokal M15 dan QA sintetis M04-CP03–05 sudah tersedia. Penerapan di lingkungan tujuan memerlukan satu authority, konfigurasi OAuth nyata dan penyimpanan secret. Bridge legacy tetap pilihan transisi jika dua runtime berbagi authority. [Impor inti](checkpoints/2026-11-22-go-core-import.md) · [Peta integrasi](FRONTEND_INTEGRATION_ROADMAP.md)
