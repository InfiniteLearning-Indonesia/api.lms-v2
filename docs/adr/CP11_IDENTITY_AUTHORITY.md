# CP11 — Identity authority dan session lifecycle decision

Tanggal: 8 September 2026 WIB. Status **PROPOSED / NEEDS_OWNER_DECISION**, bukan approved ADR atau completed checkpoint. Scope hanya CP11; CP12 tidak dimulai. [Brief](../checkpoints/CP11-CP15_BRIEF.md), [M04](../modules/M04.md), [master plan §16.2](../../../../ImplementationPlan.MD).

## Keputusan yang sudah ada vs yang masih terbuka

Master plan menetapkan hardened legacy identity sebagai satu authority existing accounts selama transisi. Go menyimpan mapped identity dan menerbitkan session Go sendiri; tidak menjadi password/email/status authority kedua. Admin di identity owner melakukan provisioning; Class roles bukan efek login. Global identity cutover ke Go merupakan wave tersendiri, bukan CP11.

Belum ada approved choice antara trusted server-to-server bridge dan standard OIDC broker dalam dokumen/source yang diperiksa. Mengimplementasikan salah satunya sebagai keputusan final tanpa review akan melewati dependency/input CP11. Instruksi menjalankan CP11 tidak menghapus syarat pada brief: bila authority belum disetujui, siapkan ADR/negative contract terlebih dahulu dan jangan membuat login bypass.

CP11 tidak membutuhkan CP12 untuk session lifecycle. CP12 menggunakan hasil CP11 untuk capability dan Class-state commands serta fencing revoke-versus-Class-write. Memindahkan seluruh pencabutan session ke CP12 juga tidak benar: revoke/rotation/account-state propagation milik CP11, sedangkan serialisasi dengan command domain Class termasuk CP12.

## Inventori source, bukan klaim deployment

Pemeriksaan read-only pada checkout `backend` dan `backend-live`, tanpa membaca secrets atau menghubungi DB/provider. Hash file `auth.service.ts` dan `google.strategy.ts` sama di kedua checkout; controller/guard/users/main berbeda byte-level, sehingga tidak disimpulkan seluruh auth deployment identik. Path berikut relatif terhadap workspace root.

| Area | Evidence implementasi | Implikasi bagi CP11 |
|---|---|---|
| Local password | `backend/src/auth/auth.service.ts::validateLocalUser`, sama dengan checkout live: password melalui bcrypt, INVITED dapat menjadi ACTIVE | Bukan trusted assertion protocol; Go tidak mengimpor raw SID/password secara otomatis |
| Setup password | `auth.service.ts::setupInitialPassword` menerima email/new password, lookup lalu menulis hash; controller `POST /auth/setup-password` tidak memakai verified reset token | Legacy belum dapat dinyatakan hardened identity owner hanya berdasarkan plan; mitigation M02 wajib sebelum real bridge dipercaya |
| Google | `auth/strategies/google.strategy.ts::validate` mengambil profile email/ID; `AuthService.validateUser` lookup email, bind pada first login; jalur subsequent tidak membandingkan subject payload dengan stored googleId | Provider-subject/verified-identity contract harus eksplisit, bukan auto-link berdasarkan email |
| Exchange | Kedua `auth.controller.ts::exchangeCode` memakai in-memory short-lived code lalu mengembalikan `{token: req.sessionID}` | Ini exchange session legacy, bukan signed audience-bound assertion untuk Go; tidak boleh dipakai ulang sebagai bridge |
| Session validation | Kedua `auth/guards/session-auth.guard.ts::canActivate` menerima cookie/query/Bearer SID fallback dan mencetak diagnostik cookie/SID; lookup user dan menolak SUSPENDED | Jangan menyalin transport/logging legacy; v3 harus menerbitkan credential baru dan meredaksi secrets |
| Logout | `backend/src/auth/auth.controller.ts::logout` memanggil `req.session.destroy` | Tidak mengikuti retention-only lifecycle v3; CP11 harus revoke retained row, bukan destroy/prune |
| Provisioning | `backend/src/users/users.controller.ts`: invite/bulk diberi Admin guard; `users.service.ts::invite` mencampur pembuatan akun dengan program/batch/auto-enrollment dan default password untuk sebagian email | Go tidak meniru default password atau coupling akademik; authority/provisioning adapter harus membatasi field/actor dan tidak memutasi Class |
| Disable / last admin | `backend/src/users/users.service.ts::ensureNotLastAdmin`, `suspend`, `update`: count active admins lalu write terpisah, tanpa transaksi/lock bersama pada jalur yang diperiksa | Tidak merupakan bukti invariant concurrent. Global last-admin harus dijaga di owner; tes Go simulated authority tidak memperbaiki legacy |
| Go CP10 | `backend-v3/backend-go/internal/identity/session.go::Resolve`: hash opaque bearer token, join active user dan valid session; session dibuat operator fixture | Resolver tersedia, tetapi belum issuer, exchange, rotate/revoke commands, verified identity mapping atau authority adapter |

Tidak menjalankan exploit pada endpoint legacy dan tidak mengubah source legacy. Temuan ini adalah blocker trust integration, bukan klaim bahwa deployment saat ini sudah diperbaiki atau dites.

## Usulan untuk disetujui

**Pilihan A — bridge server-to-server dari hardened legacy (rekomendasi transisi untuk review):**

- Legacy tetap owner login/provisioning/reset/profile/account state untuk existing accounts; Admin di sana tetap pihak yang berwenang provision. Go bertindak sebagai relying service dengan mapping issuer + immutable subject/source identity, bukan email.
- Implementasi CP11 di Go mencakup verifier/exchange dan Go session lifecycle, serta adapter account state yang fail closed. Private signing key tetap milik authority; Go hanya menerima pinned/rotatable public verification keys. Tidak memakai session secret atau raw SID legacy sebagai signing key/credential v3.
- Contract-level synthetic issuer dapat dipakai pada lab untuk menghasilkan assertion, **bukan langsung INSERT session Go**. Session harus benar-benar diterbitkan service yang diuji. Synthetic signer tidak pernah menjadi HTTP bypass atau mode shared; ini tetap bukan certification issuer legacy.
- Legacy issuer/provisioning hardening, provider integration dan deployment perlu pekerjaan/approval terpisah. CP11 boleh melaporkan local slice verified dengan external gate jelas; tidak boleh menyatakan full M04 atau login operasional selesai.
- Tradeoff: menjaga default authority master plan dan menghindari immediate global account migration, tetapi tim harus mengimplementasikan serta merawat replay protection, signing-key rotation, logout/disable propagation dan bridge availability.

**Pilihan B — standard OIDC broker:** mengurangi custom authentication protocol dengan issuer/client flow standar, tetapi membutuhkan pilihan broker, client registration, subject mapping, operasional broker, dan keputusan hubungan broker dengan password/status/provisioning legacy. Login Google yang ada belum menjadikan NestJS sebuah OIDC issuer. Ini bukan perubahan dependency/library kecil. OIDC tetap memerlukan validasi issuer, audience, signature, expiry dan nonce sesuai [OIDC Core](https://openid.net/specs/openid-connect-core-1_0.html#IDTokenValidation).

**Tidak dipilih:** langsung memindahkan seluruh identity authority ke Go, Supabase Auth rewrite wajib, email-only activation, shared password/default secret, atau copied legacy SID. Itu bertentangan dengan phase ownership saat ini atau membutuhkan keputusan migrasi baru.

Persetujuan yang diminta: memilih A (atau B dengan broker yang ditentukan) dan mengonfirmasi batas implementasi Go + isolated issuer contract tests, sementara global account writes tetap dirutekan ke hardened legacy owner. Persetujuan ini bukan izin mengubah legacy/shared DB, mengirim email, deploy, commit atau push.

## Draft contract sebelum implementation freeze

Detail di bawah **usulan, belum executable atau terverifikasi**. Setelah owner memilih authority path, finalisasi manifest granular, OpenAPI serta numeric limits sebelum coding.

1. Assertion bukan API session. Allowlist exact issuer/audience/purpose, algorithm/key, short expiry, issued-at/not-before, immutable subject, unique jti dan login challenge binding. Tolak unsigned/wrong algorithm/unknown key; jangan fetch key URL dari input. Signature bukan bukti state masih aktif selamanya. JWT implementation mengikuti [RFC 8725](https://www.rfc-editor.org/rfc/rfc8725.html), bukan crypto/protocol buatan tanpa library review.
2. Browser-bound login challenge memakai random state/nonce; exchange single-use di DB, terikat client/session context dan environment. Atomic consume + identity resolution + fresh Go session + audit/outbox. Replay lintas proses dan concurrent consume harus ditolak, bukan Map lokal.
3. Session opaque baru disimpan sebagai hash, issuer/environment scoped, absolute/idle expiry terdefinisi. Secure/HttpOnly host-only cookie dan CSRF/origin checks; exact cookie topology disiapkan lewat config tervalidasi, tidak meniru domain wildcard legacy. Rotation menginvalidasi credential lama secara atomik; logout/account revoke mempertahankan history. Session management baseline: [OWASP](https://cheatsheetseries.owasp.org/cheatsheets/Session_Management_Cheat_Sheet.html).
4. Response credential tidak boleh masuk persistent command response/audit/outbox/log/URL. Idempotency menghindari duplicate durable mutation; lost credential response memerlukan fresh verified exchange bila tidak dapat replay aman tanpa raw credential storage. Semantics concurrent rotate dan retry harus frozen, tidak blind retry commit-uncertain.
5. Account/password/profile/invite/reset ditangani identity owner; Go tidak mengubah salinan account menjadi authority sendiri. Verified origin mapping/version dan retained state events menolak stale/out-of-order updates. Authority outage menolak operasi sensitif; definisikan freshness deadline eksplisit, bukan mengklaim immediate revoke dari cache TTL.
6. Pisahkan global identity Admin dari site authority Go dan Class Teacher. Jangan otomatis memetakan `roles` assertion menjadi site_admin atau Class grants. Reviewed authority provisioning/last-admin contract harus menyebut pemilik tiap invariant; last-admin global diuji di owner, bukan diklaim beres dari simulasi.
7. Runtime identity writer bila diperlukan memakai explicit least-privilege role/command boundaries; jangan melonggarkan role Class CP09 agar semua handler dapat mengubah users/sessions. No runtime DDL, no SECURITY DEFINER shortcut, no shared/public API exposure otomatis.
8. Schema/data migration synthetic memakai backup baru + restore proof sebelum setiap installment, checksum/ledger/lock dan retained history. Tidak mengedit checksum migration lama untuk cluster yang sudah ada; tidak DELETE/TRUNCATE/DROP. Tidak membutuhkan production data untuk local lifecycle QA.

## Draft deliverable → negative/positive evidence

IDs berikut draft traceability, **belum test registry frozen atau hasil QA**. Semua scenario harus dipecah ke exact executable case IDs saat approval, termasuk expected counts dan manual gates; jangan meluluskan aggregate row dari satu status HTTP.

| Slice D | Evidence wajib | Draft case families |
|---|---|---|
| CP11-D01 Authority contract | Approved ADR, inventory, stable mapping/owner boundary; deny unknown authority and email-only linking | `CP11-authority-contract`, `CP11-mapping`, `CP11-authority-injection` |
| CP11-D02 Verified exchange | Real issuer verification + challenge + DB single-use consume/session issue; forged/expired/future/purpose/audience/issuer/replay/cross-client rejected | `CP11-exchange-positive`, `CP11-exchange-negative`, `CP11-consume-concurrent` |
| CP11-D03 Session lifecycle | Cookie/session issue/rotate/revoke/logout, retained rows, old credential denied, conflict/lost response behavior, redacted output | `CP11-session-lifecycle`, `CP11-rotation-concurrent`, `CP11-cookie-csrf`, `CP11-secret-redaction` |
| CP11-D04 Account/authority lifecycle | Verified owner-directed provisioning/reset/disable/profile contracts, stale version and outage rejection, authority field denial, last-admin concurrency at correct owner | `CP11-account-projection`, `CP11-owner-routing`, `CP11-outage`, `CP11-last-admin` |
| CP11-D05 Persistence and regression | Rollback at exchange/audit/outbox boundaries, restricted-role denial, backup/restore/source hash, no-delete history; existing Class journey regression | `CP11-atomicity`, `CP11-db-privileges`, `CP11-history`, CP08–CP10 regression |

Run after implementation: Go format/build/vet/unit + Linux race, exact Playwright Python API cases against fresh Go binary, real isolated PostgreSQL integration, actual synthetic issuer protocol service, CP08–CP10 and M00/M03 dependency regression. Capture run/source/manifest/binary/schema hashes, collected/required cases, backup refs, coverage by package, and pass/fail/blockers. Browser UI integration, real issuer and provider approvals remain explicit external gates if not executed; local actor API tests cannot certify browser UX.

Full M04 acceptance and its 0/5 denominator remain unchanged. No test pass rate or code coverage is assigned to this draft. Session tables seeded by existing CP10 fixtures are historical CP10 evidence, not CP11 verified exchange.

## Current handoff

- Completed now: dependency analysis, source inventory, proposed authority choice, draft negative/positive acceptance contract.
- **CP11 lifecycle implementation NOT_STARTED / BLOCKED_DECISION**; exit criteria not reached. No CP11 QA executed, no DB/backup needed because no code/schema/data change was made.
- CP12–CP15 NOT_STARTED. No changes to production/staging, no legacy patch, no commit/push.
- Supabase/session-security guidance informed revocation freshness and private least-privilege boundaries. No Supabase Auth/provider/schema feature was implemented; skill review is not approval for shared operations.
- Next action: obtain authority-path approval, then freeze executable scope/manifest and implement CP11 only. If global owner implementations remain unavailable, report their gate honestly instead of writing an independent Go password/status owner to make tests green.
