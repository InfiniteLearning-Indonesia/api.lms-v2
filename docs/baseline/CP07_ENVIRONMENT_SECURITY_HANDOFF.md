# CP07 — Environment identity & security handoff

Tanggal: 7 September 2026 WIB. Status: **PARTIAL — BLOCKED_INPUT untuk penutupan operasional**. Scope M00 dan handoff M02; bukan izin mengubah legacy, grants, RLS, deployment, atau menyalin production. [Evidence sesi CP06–07](../checkpoints/2026-09-07-cp06-cp07.md).

## 1. Fakta terverifikasi dan batas klaim

**Koreksi pengguna setelah CP07:** `backend/.env` dan backend-go `.env.staging` keduanya mengarah ke staging. Pernyataan sementara bahwa `backend/.env` adalah production sudah dicabut pengguna. Production reference tetap UNKNOWN; tidak menginterpretasi nama file/folder sebagai bukti server. [CP08–CP10](../checkpoints/2026-09-07-cp08-cp10.md) hanya menjalankan synthetic local QA, tanpa probe provider/shared baru.

| Area | Evidence saat sesi ini | Yang belum terbukti |
|---|---|---|
| Source live/staging | M00 scan ulang kedua checkout; baseline [LOCAL_BASELINE](LOCAL_BASELINE.md): live `f73edf261d006686a3ae317aa93c411e3648b850`, staging `dd3413e771409bee9ccb38c59d9576b86545b0cf`; masing-masing 64 TS source | SHA/image yang benar-benar sedang melayani traffic |
| Config lokal | `.env.staging` backend-go dipilih eksplisit sesuai keterangan pengguna. Inventory empat config legacy/staging lokal belum membuktikan dua target berbeda | Secret reference production deployment yang benar; nama folder `backend-live` bukan identitas server |
| Staging DB | Probe `20260906T183039Z-staging-inspect-12716dcf0f85`; 6 Sep 18:30:40 UTC / 7 Sep 01:30:40 WIB; TLS certificate chain + hostname verified; READ ONLY; PostgreSQL 17.6 | Hak akses akun belum least privilege; observasi bukan approval |
| Catalog | 20 public tables; 8 non-system namespaces; 60 relations; 205 triggers termasuk internal; 97 routines; 40 pasangan role/table | Full semantic function/policy review, provider jobs dan storage object inventory |
| Ukuran | `pg_database_size`: 11.619.475 bytes pada waktu observasi | Bukan kebutuhan backup total; belum mencakup R2, log host, restore space, WAL, retention, encryption/ACL/key recovery |
| Production DB | Tidak dihubungi | Identity, CA, schema drift, role, capacity, Data API, backup scope |
| Operasi data | Tidak membaca rows murid/session, tidak export/import, tidak mengubah shared DB | Backup/restore riil dan migrasi tetap belum dijalankan |

Receipt terikat source hash `7238cf80fa4f07b2732beeeea5466aa9a84d460e92091e76fe40194c6511cf1b`. Catalog hash `4136f7d2b916f502902510e60bbe868b0460cf0f97db11ebade20aab6838791b`; dependency-metadata hash `f89fe67c0f98f623790f1e0ef7041c5bb820207c0f91776b58419442cdd8afe1`. Metadata berbeda dari full schema/data snapshot. Evidence lama tidak ditimpa; receipt baru diterima runner sebagai `OBSERVED_NOT_APPROVED`, tidak membuka mutation gate.

## 2. Disposisi keamanan yang perlu keputusan owner

### SEC-CP07-01 — Hak API-role luas pada tabel tanpa RLS

- **High: configuration risk teramati; exposure publik belum diverifikasi.** Probe mencatat `anon`/`authenticated` memiliki schema USAGE dan hak SELECT pada public tables; tabel sensitif `users` dan `session` juga memiliki INSERT/UPDATE/DELETE/TRUNCATE. Semua 20 public tables RLS=false. Bukti: `internal/platform/dbinspect/dependencies.go`, `catalog.go`, receipt staging di atas.
- Jika provider memang mengekspos schema/tabel ini melalui Data API kepada caller tersebut, data pribadi/sesi dan mutasi tidak sah berpotensi berdampak kritis. **Tidak menyatakan endpoint tersebut sudah diakses atau dieksploitasi.** SQL TRUNCATE privilege juga bukan klaim adanya HTTP endpoint TRUNCATE.
- Owner harus memberikan settings Data API enabled/disabled, exposed schemas, project identity, dan daftar consumer langsung REST/GraphQL/RPC. Jangan menyimpulkan setting dari grants, CORS aplikasi, hostname, atau default project baru.
- Opsi keputusan: backend-only dan tidak ada consumer langsung → rencanakan isolasi schema/exposure sesuai daftar consumer; jika Data API masih dibutuhkan → desain explicit grants + RLS/approved views/RPC sesuai kebijakan akses. `session` dan internal v3/landing/ops tidak boleh menjadi API publik.
- Review default privileges per pembuat objek, role membership/inheritance, function EXECUTE/SECURITY DEFINER serta search_path, views dan schema cache. Existing privilege dapat berbeda dari default untuk objek baru.
- **Belum mitigated.** Tidak menjalankan revoke, enable RLS, disable API, role switch pada shared DB, atau public endpoint test. Perubahan menunggu owner, reviewed change set, backup/restore, compatibility proof, jadwal dan izin terpisah.

Grants, exposed schemas, dan RLS adalah lapisan terpisah; panduan vendor: [Supabase securing your API](https://supabase.com/docs/guides/api/securing-your-api). [Changelog Supabase](https://supabase.com/changelog) ditinjau; perubahan default platform tidak menggantikan pembuktian konfigurasi target yang ada.

### SEC-CP07-02 — Credential observasi masih owner/write-capable

- Staging role memiliki BYPASSRLS, CREATEDB, CREATEROLE dan CREATE di public; owner semua 20 tabel, bukan superuser. Transaksi READ ONLY membatasi probe, **bukan** membatasi credential di luar probe.
- Metadata inspector produksi tidak boleh mewarisi hak runtime/migrator atau membaca rows sensitif. CA pengguna hanya memperbaiki autentikasi TLS probe Go, tidak memperbaiki konfigurasi legacy.
- Tetap `least_privilege_verified=false`. Lihat rencana provisioning di bawah; tidak ada akun/grants shared yang dibuat sesi ini.

### SEC-CP07-03 — Startup/job legacy bertentangan dengan no-delete/no-auto-migration

- `backend/src/audit/audit.service.ts::onModuleInit/archiveOldLogs` dan pasangan `backend-live`: memanggil archive saat startup + setiap 24 jam, append JSONL lokal lalu `auditRepository.delete` dalam chunk 500 untuk log >30 hari. File append bukan verified recoverable backup/restore receipt.
- `backend/src/main.ts::bootstrap`: membuat admin bila belum ada, ALTER TYPE/ALTER TABLE dan CREATE TABLE/INDEX sesi. `app.module.ts` mengaktifkan ORM synchronize jika NODE_ENV bukan production. **Menyalakan backend bukan pemeriksaan read-only**, termasuk ketika NODE_ENV=production karena DDL manual masih ada.
- `main.ts` memakai `connect-pg-simple` tanpa menonaktifkan pruning. Installed staging dependency `backend/node_modules/connect-pg-simple/index.js` v10.0.0: constructor mengaktifkan interval default; `#initPruneTimer` dipanggil oleh operasi session; `pruneSessions` menghapus expired rows; `destroy` menghapus sid. `backend/src/auth/auth.controller.ts` logout memanggil `req.session.destroy`. Dependency terpasang bukan bukti versi proses live; node_modules live tidak tersedia lokal.
- Rekomendasi M02 terpisah: hentikan hard-delete melalui retention/revocation state yang mempertahankan histori dan menolak penggunaan sesi revoked/expired; pindahkan DDL ke migration operator ber-backup; archive audit append-only dengan receipt. Jangan hanya mematikan logout atau membiarkan sesi lama tetap valid, dan jangan mencabut DELETE legacy secara mendadak sehingga logout rusak.
- **Tidak dipatch/deploy sesi ini.** Live dapat tetap melakukan deletion dari kode yang sedang deployed; pekerjaan CP06–07 ini tidak mengklaim menghentikannya.

## 3. Inventory dependency — apa yang harus ikut backup/rehearsal

| Komponen/trigger | Source yang ditelusuri | Data/efek dan boundary | Input yang diperlukan dari operator |
|---|---|---|---|
| Audit startup + daily | `audit/audit.service.ts` kedua checkout | DB → `logs/audit-YYYY-MM.jsonl`, lalu deletion legacy; job berjalan per instance, bukan distributed singleton | Apakah job benar aktif di deployment, replica count, lokasi persistent volume, retention/protected snapshot |
| Health 5 detik setelah init + 15 menit | `health/health-history.service.ts` kedua checkout | SELECT 1; menulis ulang `status-history.json`, membatasi history 700 entry; cutoff days bukan filter yang digunakan | Host/volume ownership, disk usage, apakah dibutuhkan untuk historical migration |
| Session library timer + logout | `main.ts`, `auth/auth.controller.ts`, installed dependency di atas | `session` memiliki payload sensitif dan lifecycle DELETE | Exact dependency deployment, revoke policy, larangan replay sesi aktif ke development |
| Startup bootstrap/synchronize | `main.ts`, `app.module.ts` kedua checkout | Admin insert dan DDL otomatis | NODE_ENV efektif dan secret-source precedence yang disanitasi; bukan dump seluruh environment |
| HTTP logs asynchronous | `common/middleware/http-logger.middleware.ts` | Append bulanan `logs/http-requests-YYYY-MM.log`, PM2 console; originalUrl/IP dapat sensitif | Log sink/retention, ACL, ukur kapasitas tanpa menyalin isi log atau query token ke report |
| Proof izin/sakit/chat | `attendance/attendance.service.ts::createPermissionRequest` → `storage/storage.service.ts::uploadMultipleBase64` | R2 prefixes `permissions/proofs`, `permissions/chat-proofs`; DB menyimpan URL atau fallback input base64; upload mendahului DB save | Bucket identity, access policy, object count/bytes/versioning/lifecycle; private proof file policy dan owner |
| Avatar | `users/users.service.ts` → `uploadBase64/uploadUrlToR2` | R2 `avatars`; URL eksternal dapat dipertahankan; tidak semua referensi berarti object R2 tersedia | Allowed external providers, provenance, bytes yang bisa/tidak bisa dicadangkan |
| Supabase scheduler/provider | Inspector `scheduler_relations` kosong pada namespace yang diperiksa | Tidak membuktikan tidak ada cron eksternal, CI scheduler, Edge Functions, queue atau webhook | Dashboard/host inventory metadata saja: nama job, enabled state, cadence, owner, target hash; jangan export command/token/body |
| Deployment | Kedua `deploy.sh` fetch/pull `staging`, build lalu PM2 restart dan Nginx reload | Nama folder/branch tidak menjamin identitas runtime; script bukan release receipt | Actual deployed SHA/image digest, host reference, timestamp, process working directory dan config reference yang disanitasi |

Storage source membentuk public URL dari R2_PUBLIC_DOMAIN; credentialless/error path mengembalikan input asli. Ini alasan melakukan inventory object bytes **dan** field referensi; bukan bukti semua file dapat diakses publik atau download berhasil. Tidak mengakses bucket/domain atau membaca proof milik murid sesi ini. Full namespace/routine semantic review dan provider scheduler inventory tetap pending; tidak menandai M00-D03 selesai dari regex scan saja.

## 4. Rencana provisioning least privilege (bukan SQL yang sudah dieksekusi)

Pelengkap [ACCESS_MODEL](ACCESS_MODEL.md). DBA/security reviewer memisahkan principal dan secret references per environment; tidak membagi password produksi ke runtime lokal.

| Principal | Izin yang direncanakan | Negative proof sebelum dipakai |
|---|---|---|
| Metadata inspector | LOGIN terbatas, CONNECT target, catalog metadata yang disetujui; default read-only + timeout + TLS CA/hostname | Tidak owner/BYPASSRLS/CREATEDB/CREATEROLE; tidak business SELECT, DML/DDL atau membership yang menaikkan hak |
| Backup operator | Snapshot-complete untuk namespace/rows yang disetujui; object access terpisah | Tidak menerima dump RLS-filtered sebagai lengkap; tidak runtime login/shared key, tidak restore ke source/existing staging |
| Go runtime | CONNECT, USAGE private v3, SELECT/INSERT/UPDATE pada allowlist, explicit sequence/routine access bila diperlukan | Dilarang legacy writes, DELETE/TRUNCATE, ownership, schema CREATE, role management, broad function EXECUTE; actor×Class policy tetap aplikasi |
| Migration operator | Reviewed additive DDL pada exact target, terpisah dari runtime | Guard identity/checksum/backup/restore/lock; tidak DROP/TRUNCATE/DELETE atau modify existing source; no automatic fallback to owner |

Urutan pelaksanaan nanti:

1. Owner menyetujui identity, data scope, role names, network scope, credential lifetime/rotation, reviewer dan jadwal. Inventori consumer legacy sebelum perubahan hak.
2. DBA menyiapkan reviewed additive provisioning change set dan recovery procedure; backup/restore prerequisite sebelum perubahan shared. Tidak mengeluarkan password via console/history/Markdown.
3. Uji exact privileges pada isolated equivalent fixture lebih dulu. CP06 `qa_reader` hanya membuktikan pola penolakan PostgreSQL; bukan provisioning role di Supabase atau bukti v3 RBAC.
4. Setelah izin spesifik, provision principal baru tanpa mengganti kredensial legacy; metadata-only credential verification, role flags, memberships, ownership, default ACL dan TLS. Destructive privileges diperiksa melalui catalog, **tidak** diuji dengan benar-benar mengirim DELETE/DROP/TRUNCATE.
5. Update probe support untuk approved production reference secara fail-closed sebelum memakai live target. Wrapper sekarang sengaja hanya `.env.staging`; jangan mengganti file staging dengan prod atau melabeli prod sebagai staging untuk melewati gate.
6. Jika role migration perlu owner privilege, scope dibatasi pada private target objects dan operator-run saja. Review role memberships dan routine execution agar akun read-only tidak mempunyai jalan eskalasi.
7. Catat receipts per target, role/hash, schema scope, waktu, reviewer; perubahan hash/target/role memerlukan validasi ulang. Tidak ada auto-start legacy atau auto-export sesudah review.

## 5. Form input minimal CP07 — tanpa secret values

Isi atau lampirkan bukti lokal yang dapat ditunjuk, bukan password/token:

| Input wajib | Production | Staging | Verifier/acceptance |
|---|---|---|---|
| Provider project/host reference dan deployment ID | PENDING | PENDING (hanya DB config user-identified) | Ops mencocokkan provider dengan deployment, bukan nama checkout |
| Deployed SHA atau immutable artifact digest | PENDING | PENDING | Runtime/release receipt, timestamp dan repo reference |
| Secret reference **lokasi/nama saja** | PENDING | `.env.staging` lokal tersedia; deployment reference PENDING | Selected target fingerprint + TLS + DB name/role hashes, tanpa nilai mentah |
| Approved metadata access window/scope | PENDING | Probe staging scope sudah dieksekusi; approval baru untuk scope tambahan | Tidak mengasumsikan izin row reads, export atau function-body dump |
| Restricted inspector role | PENDING | PENDING | Existing role tidak memenuhi least privilege |
| Data API enabled + exposed schemas/consumer list | PENDING | PENDING | Screenshot/provider metadata terikat project dan observation time |
| Jobs/host volumes/R2/lifecycle inventory | PENDING | PARTIAL source/catalog | Operator mengonfirmasi runtime; tidak ada job command/credential di report |
| Capacity + protection + restore destination | PENDING | PENDING | Free usable disk, source bytes + storage/log scope, retention/headroom policy, encryption/ACL/key recovery |
| Data owner / ops owner / security reviewer | PENDING | PENDING | Nama/identity, keputusan, scope, tanggal, evidence reference |

Capacity minimum harus dihitung untuk source snapshot + SQL/custom output + encrypted copies + restore footprint + retained failures + approved growth margin; jangan mengasumsikan dump compressed atau DB size saja cukup. SQL lokal kelak harus protected/encrypted untuk data nyata; file SQL sintetis CP06 tidak membuktikan kontrol tersebut.

## 6. Exit criteria CP07

- [x] Source/dependency handoff lokal dan refreshed staging metadata terikat hash; tidak ada secret values dalam Markdown ini.
- [x] Risiko API grants, no-delete legacy dan least-privilege gap dinyatakan tanpa mengklaim exploit/mitigation.
- [x] Rencana provisioning dan input owner disiapkan; keputusan belum direkayasa menjadi approval.
- [ ] **Kedua** actual deployed identities dan distinct DB targets terverifikasi dengan permitted per-target metadata tool.
- [ ] Dedicated read-only principal dan Data API exposure disposition direview/disetujui.
- [ ] Full jobs/storage/capacity inventory serta owner/reviewer sign-off tersedia.

Ini checklist handoff, bukan denominator deliverable M00. M00 tetap 0/5 full D terverifikasi; M02 containment belum implemented/deployed. CP08–CP10 tidak dimulai. Jika input eksternal belum tersedia, berhenti pada handoff ini; jangan memindahkan production/staging atau mengubah hak untuk memaksa gate hijau.
