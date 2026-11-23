# LMS v3 — rangkuman sebelum baseline live dan staging

> **Rujukan terkini.** Gunakan [roadmap](FRONTEND_INTEGRATION_ROADMAP.md) untuk urutan integrasi frontend dan staging-v2 serta [indeks QA](QA_EVIDENCE.md) untuk bukti yang sudah diterima. Catatan lama di bawah merekam posisi pada tanggal masing-masing.

> **23 November — migrasi inti Go lokal dan 5/5 checkpoint sintetis M04 lulus.** Run `20261122T144153Z-go-core-rehearsal-ffcec7233b7e` lulus 4/4 tahap. Target `legacy_core_v3` berisi 17 Class draft, 132 identitas, dan 118 participation. CP05 undangan Class lulus QA PostgreSQL dan Playwright lokal. Runtime bersama, provisioning akun historis dan mapping data domain masih terbuka. [Bukti impor](checkpoints/2026-11-22-go-core-import.md) · [Bukti CP05](checkpoints/2026-11-23-m04-cp05-class-invitations.md)

> **22 November — dry run data nyata.** Profil Class, User, Enrollment, dan Role mencakup 21 tabel dan 7.843 record. Dari 267 record inti, 266 kandidat dan satu enrollment ditahan. Berikutnya adalah importer Go dan pembuktian migrasi ke target v3 lokal terpisah. [Hasil dan usulan mapping](checkpoints/2026-11-22-legacy-core-dry-run.md)

> **19 November — current accepted local legacy baseline:** [Evidence](checkpoints/2026-11-19-local-legacy-restore-accepted.md), run `20261119T093357Z-legacy-local-restore-debaab74a513`: **11/11 native cases PASS; actual restore verified**, 21 tables/7,843 rows. Independent receipt/hash/cold shutdown review PASS; private cluster retained stopped (~77.3 MiB). Do not repeat export/restore. Next read-only real mapping and a separate local v3 target; M15 synthetic mapping is not a full importer. Full modules/DLV-06/runtime/R2/cutover and independent key custody remain open. No source/R2 connection, delete, Go runtime change, commit or push. Earlier failure/NOT_RUN notes below are history.

> **19 November — latest local restore startup fix:** [Evidence/handoff](checkpoints/2026-11-19-local-restore-startup-fix.md). Latest local server was ready but the old Windows pg_ctl PIPE wrapper stalled before creating user DBs/owner role. Fixed private regular-file capture and duplicate-live-cluster guard: **38/38 offline tests + native synthetic process-I/O reproduction PASS**. Actual 11-case DB restore NOT_VERIFIED; old owner process must be interrupted/cleaned up before the same updated launcher. No source DB/R2 access, deletes, shared service changes, full-module closure, commit or push.

> **18 November — latest local restore preparation:** [Evidence](checkpoints/2026-11-18-local-legacy-restore-prepared.md), [owner command](baseline/LOCAL_LEGACY_RESTORE.md). Complete pinned snapshot schema/COPY review PASS: 21 tables, 7,843 rows, 4 enums, 27 FK, no sequences; uuid-ossp dependency identified. New isolated restore operator + 34/34 offline tests PASS. **Actual restore NOT_RUN, 0/11 native cases**: Codex private output write is denied; normal-owner command required. No staging/production connection, R2, deletion, Go runtime/migration changes, module closure, commit or push. Accepted snapshot remains intact.

> **18 November — latest accepted baseline:** [Public snapshot evidence](checkpoints/2026-11-18-staging-snapshot-accepted.md), run `20261118T131839Z-staging-baseline-3fe3bf70b4d4`: **PASS_REVIEWED_PUBLIC_SNAPSHOT_EXPORT_ONLY**. One archive, SQL derived from it, two encrypted artifacts; independent local hashes/decryption/TOC checks PASS for 21 public table-data entries. ~5.1 MiB, no cluster created. Next: review dump dependencies and a fresh isolated local restore, then data reconciliation/mapping. Restore NOT_RUN; key custody remains same-disk, non-public Supabase/R2 excluded; no full-module/DLV-06 readiness, source writes, delete, commit or push. Earlier no-snapshot notes below are historical.

> **18 November — latest client TLS correction:** [Diagnosis/evidence](checkpoints/2026-11-18-staging-client-tls-fix.md). Owner export stopped before dump due to a runner bug confusing backend-hop TLS with client TLS at a pooler. Fixed: same-connection client proof, verify-full retained, TLS1.2 minimum. **19/19 offline tests + actual corrected metadata preflight PASS** (client TLS1.3/read-only true; backend flag false). Snapshot/restore still NOT_EXECUTED; rerun the [same owner command](baseline/STAGING_LOCAL_SNAPSHOT.md). No source mutations/R2/delete/commit/push or CP/module closure. Prior 12-test and blocked-preflight notes describe earlier source states.

> **18 November — latest:** [Staging snapshot attempt](checkpoints/2026-11-18-staging-local-snapshot.md), [owner command/scope](baseline/STAGING_LOCAL_SNAPSHOT.md): actual metadata connection with existing CA **PASS verified TLS/read-only**; PG17.6, 21 public tables, ~14.5 MiB. Credential privileged, not dedicated read-only; source distinction/sync not independently verified. Snapshot **NOT TAKEN**: private directory write denied to Codex. New bounded public-schema exporter: 12/12 offline tests PASS, actual export/restore NOT_RUN. Native owner command required; no remote mutations/R2/delete/commit/push. Earlier “no connection” statements below are historical. Publication `a322d14` was subsequently confirmed by user push; this new work remains unpublished.

> **Publication requested 17 November:** [Scope/validation boundaries](checkpoints/2026-11-17-publication.md). Latest source/docs are being packaged for `be-v3` before DB access; final publication state belongs to Git history/remote receipt. User now identifies `.env.staging` as real synchronized staging; this is not independent identity/TLS/permission proof and no connection was performed. Earlier no-push/local-only statements below describe prior work, not a new prohibition on this authorized publication.

> **17 November — current recovery:** [Seven-case dependency accepted](checkpoints/2026-11-17-protected-package-accepted.md); [new local recovery implementation](checkpoints/2026-11-17-local-recovery.md): **81 Go/95 ops PASS**, current native **4/9 PASS + 5 fixture AccessDenied FAIL**, ten DB cases NOT_RUN. [Owner runner](baseline/LOCAL_RECOVERY_RUNBOOK.md); [why dotenv is not staging activation](baseline/STAGING_CONFIGURATION_READINESS.md). No native recovery/module acceptance, env/DB edits or publication. Earlier source-bound snapshots below are history.

> **17 November — current:** [Protected output/package implementation](checkpoints/2026-11-17-protected-package.md), source `126c217ad43a732e67d5d3b32473233ed5bd94385596a78872f0ba9b7b63371b`: **79/79 Go + 93/93 ops + vet/Windows build PASS**, Linux cross-build/tagged compile PASS_BUILD_ONLY. Mandatory actual suite **4 PASS + 3 fixture AccessDenied FAIL**; restricted token confirmed. **IMPLEMENTED_PENDING_OWNER_NATIVE_QA**, no CP/module/restore closure. [Contract/runner](baseline/PROTECTED_PACKAGE_RECOVERY.md). Earlier source-bound results below remain history; no DB/delete/commit/push.

Tanggal rekonsiliasi: **17 November 2026**. Scope: backend Go, hasil implementasi/QA lokal, kesiapan data nyata dan handoff developer. Dokumen ini merangkum keputusan dan evidence yang sudah ada; tidak mengotorisasi koneksi/export/migrasi database, deployment, penghapusan, commit atau push. Tidak ada suite/cluster baru dijalankan untuk membuat rangkuman ini.

## 1. Kesimpulan saat ini

**Backend v3 sudah memiliki banyak alur domain yang lulus pada database sintetis lokal. Belum siap dipakai sebagai pengganti live atau langsung diarahkan ke staging existing.** Hambatan bukan hanya mendapatkan credential/database: runtime bersama, identity authority, exporter/recovery operasional, pemetaan seluruh data legacy dan beberapa policy belum selesai.

User dapat mulai menyiapkan **metadata dan scope baseline sekarang**, sambil tooling lokal dirampungkan. Memperoleh snapshot untuk dipelajari secara lokal, mengisi staging v3, dan mengalihkan pengguna live adalah tiga tahap berbeda. Tujuan tahap terdekat adalah **menerima snapshot terverifikasi lalu memulihkannya ke database lokal baru**, bukan cutover.

Posisi sumber:

- Worktree/repository root Go: `backend-v3/backend-go`; branch `be-v3`.
- Baseline sebelum publikasi: HEAD lokal dan remote `be-v3` diverifikasi `7e7e86b2fa6408d8945f9d2e983cdcb76a27a2d2`; commit publikasi berikutnya dilihat dari Git history/branch, bukan menebak dari source fingerprint.
- Current executable source fingerprint: `454187dda3c48e32681a73c9810d47d92fee196e17f00958a2898df608bdfa33`.
- Paket publikasi mencakup perubahan source/docs setelah baseline HEAD; hash source bukan commit. Secret/env/private evidence/backup/policy tetap hanya lokal dan tidak ikut GitHub; clone public source saja tidak memulihkan seluruh state privat.
- Git common directory masih di `backend/.git`; `.git` dalam Go project adalah worktree pointer. Folder Go saja berisi source tetapi bukan repository Git mandiri ketika dipindahkan. Jangan membuang parent Git metadata dalam backup lokal; clone baru yang lengkap adalah opsi terpisah setelah publikasi yang diminta.

## 2. Keputusan produk yang tetap berlaku

- Backend aplikasi **Golang**. Python/Pytest/Playwright adalah QA; frontend dikerjakan tim lain dan penyelarasan menyusul.
- **Class menjadi pusat**: admin membuat Class mandiri dengan nama/label program/cohort; relasi program/batch tidak menjadi prasyarat universal.
- Teacher/Student/capability berlaku kontekstual per Class/resource. Enrollment/participation episode berbeda dari role; invitation/retake/provenance dipertahankan.
- Definisi content/activity dan revision terpisah dari learner submission/attempt. Grade/release/history terpisah dari progress/completion/transcript.
- Fitur lama yang masih dibutuhkan dipertahankan melalui mapping/adapter yang direview. Mengikuti domain Moodle bukan menyalin seluruh schema/fitur Moodle.
- Live tetap melayani pengguna lama. Target operasional v3 nantinya **database staging**, setelah disiapkan dengan benar; staging existing juga tidak boleh ditimpa sembarangan.
- Tidak hard-delete data/database/object untuk mempermudah migrasi. Data ambigu di-account, ditahan/quarantine atau diarsipkan dengan alasan. Backup terverifikasi wajib sebelum perubahan/migrasi, termasuk pre-state target; recovery evidence bukan sekadar file SQL ada.

Rujukan keputusan: [ImplementationPlan](references/ImplementationPlan.MD), [audit legacy](references/LMS_CODEBASE_AUDIT_AND_REMEDIATION.md), [benchmark](references/LMS_Modern_Moodle_Benchmark_Architecture_Business_Flow.md). Audit adalah baseline temuan, bukan bukti bahwa semua finding sudah diperbaiki/deployed.

## 3. Status seluruh modul

Angka berikut adalah **CP slice lokal yang pernah diterima**, sesuai [peta M/CP](MODULE_CHECKPOINT_MAP.md), bukan persentase proyek atau jaminan full regression terbaru. Evidence lama tetap valid untuk source/scope lamanya; acceptance real environment dan canonical D terpisah.

| M | Area | CP lokal / rencana | Sisa penting |
|---|---|---:|---|
| [M00](modules/M00.md) | Environment/baseline | 1/4 | Identitas production/deployed artifact, inventory penuh, authority dan target berbeda belum terverifikasi |
| [M01](modules/M01.md) | Backup/restore | 1/4; CP02 parsial | Actual age streaming slice PASS; protected exporter/custody/ACL, package wiring dan actual real-baseline restore belum |
| [M02](modules/M02.md) | Containment legacy | 0/4; CP01 parsial | Perbaikan/containment dan bukti deployment legacy belum lengkap; Go v3 tidak otomatis memperbaiki aplikasi live lama |
| [M03](modules/M03.md) | Fondasi Go/audit/outbox/worker | 6/6 | Full current bootstrap/remote CI/race/shared operational gates belum closed |
| [M04](modules/M04.md) | Identity/session/capability | 5/5 lokal sintetis | Integrasi runtime, provider nyata dan provisioning akun historis |
| [M05](modules/M05.md) | Class/participation | 5/5 sintetis | Real identity/mapping, archive policy, shared runtime/acceptance |
| [M06](modules/M06.md) | Private files/egress | 3/4 | CP04 preserved file mapping, byte/object recovery dan consumer/provider closure |
| [M07](modules/M07.md) | Content/activity/revisions | 4/4 sintetis | Real content/attachment import, shared/frontend gates |
| [M08](modules/M08.md) | Submission | 3/4; CP04 parsial | Actual chronology/source mapping dan reconciliation; fixture landing bukan full importer |
| [M09](modules/M09.md) | Gradebook/rubric | 4/4 sintetis | Academic approval, actual grade provenance/mapping dan runtime/operational gates |
| [M10](modules/M10.md) | Completion/transcript/credential | 4/4 sintetis | Academic policy, actual output parity dan identity/shared integration |
| [M11](modules/M11.md) | Attendance/permit/SP | 1/4 | CP02–04: attendance/permit/SP/correction policy dan historical mapping |
| [M12](modules/M12.md) | Logbook/mentoring | 3/4 | CP04 legacy schedule/entry mapping dan policy/authority |
| [M13](modules/M13.md) | Jobs/notifications/AI | 4/4 sintetis | Provider/secret/privacy dan shared runtime; AI suggestion tidak authoritative untuk grade |
| [M14](modules/M14.md) | Reporting/API/performance | 4/4 sintetis | Frontend/shared runtime, actual-data parity dan production capacity/ops gates |
| [M15](modules/M15.md) | Mapping/migration ledger | 5/5 sintetis | Real source adapter dan full-domain mapping belum selesai |
| [M16](modules/M16.md) | Rehearsal | 0/4 | Verified actual source/target, backup/restore, mapping; dua snapshot/delta/resume/parity |
| [M17](modules/M17.md) | Pilot/single writer | 0/4 | M16 accepted, frontend/session readiness, owner/write fence, final delta/recovery |
| [M18](modules/M18.md) | Rollout/operations | 0/3 | Pilot accepted, per-wave controls/backup, observability/recovery dan acceptance |
| [M19](modules/M19.md) | Optional extensions | Scope belum frozen | Bukan prioritas jalur baseline; tidak masuk denominator |

Canonical M00–M18 tetap **0/97 D fully verified**. Artinya belum ada row acceptance penuh yang memenuhi seluruh shared/external/operational requirements; **bukan 0% kode atau 0 tes lulus**. Tidak menghitung rata-rata CP sebagai progress effort. Rangkuman ini tidak menambah CP/D atau menutup M.

## 4. QA yang selesai, gagal dan belum dijalankan

### Evidence current executable source — 17 November, recovery continuation

[Local recovery checkpoint](checkpoints/2026-11-17-local-recovery.md), `local-recovery-20261117/qa-G` / review-G.json:

- **81/81 default Go + 95/95 ops + vet/Windows build PASS**. Linux cross-build/tagged compile PASS_BUILD_ONLY.
- Current mandatory nine-case native suite: **4 PASS, 5 private-fixture AccessDenied FAIL** in this restricted-token environment. Full suite NOT_ACCEPTED. **Ten new DB recovery cases NOT_RUN**; no cluster created. Positive recovery/restore cannot be inferred from implementation/compile.
- Backup default statement coverage **64.7% (712/1100)**. Full current API/Playwright/remote CI remains NOT_RUN.
- [Previous seven-case owner run](checkpoints/2026-11-17-protected-package-accepted.md) is now accepted on source `126c217…`, not still waiting: 7/7 actual + 79/79 Go/vet, same-source 93 ops. It does not accept the changed nine-case recovery tree.
- Local seal/recover APIs and the small isolated custom/SQL/object recovery runner are implemented. [One command](baseline/LOCAL_RECOVERY_RUNBOOK.md) executes native gate then DB profile; [staging config assessment](baseline/STAGING_CONFIGURATION_READINESS.md) explains why changing dotenv is insufficient.

### Evidence bisnis historis yang tetap dipertahankan

- [M13 closure](checkpoints/2026-10-31-m13-local-closure.md): 4/4 local CP, 19/19 direct cases pada source/run tercatat.
- [M14 closure](checkpoints/2026-11-11-m14-local-closure.md): 4/4 local CP, 21/21 direct cases; 1,080 measured requests, query/pool/audit/dependency checks dan 40 logical backup/restore receipt sets. Source `cb0a9af6…`; **bukan current-source performance rerun atau production SLA**.
- Modul lain punya checkpoint synthetic Go/Playwright/DB evidence sebagaimana brief masing-masing; tidak dianggap deploy/shared readiness.

### Kegagalan yang relevan

| Masalah | Status sekarang |
|---|---|
| PostgreSQL Windows restricted-token error 87 | Historical full journeys kemudian berhasil di terminal owner; bukan bukti PostgreSQL saat ini rusak. Source baru tetap memerlukan affected QA yang sesuai |
| Tiga Playwright setup errors, WinError 5 named pipe | Belum ada current-source evidence yang menutup residual DLV-01 gate. Jangan mengubah ERROR/SKIP menjadi PASS atau mengganti Playwright diam-diam |
| Dependency download / storage UNKNOWN_BYTES di sandbox | Owner-terminal crypto run sudah melewati admission/download dan PASS. Tidak perlu mengulang crypto hanya karena sandbox sebelumnya blocked; akses/inventory sandbox tetap terbatas |
| Run A age adapter regex panic | Sudah diperbaiki dan diikuti source-bound Go/ops/actual crypto PASS; failure history retained |
| Reporting/runner/parser failures sebelumnya | Accepted later full runs menutup slice yang relevan; ringkasan FAIL lama tidak dihapus/ditulis ulang |
| Workflow remote sebelumnya gagal | Local registry/helper/diagnostic repairs ada; actual remote result untuk perubahan terbaru belum dibuktikan. Cross-build tidak menggantikannya |

Rujukan: [QA repair/gates](checkpoints/2026-11-12-qa-deliverables.md), [QA contract](QA_AUTOMATION_PLAN.md).

## 5. Kesenjangan teknis yang mudah terlewat

1. **Lab wiring belum product runtime lengkap.** [cmd/api](../cmd/api/main.go) masih memakai synthetic identity/lab DB guards dan Class handler; banyak domain disusun secara opt-in di [identity-lab](../cmd/identity-lab/main.go). Jangan deploy lab, mengganti env ke staging atau menghapus guard untuk menjadikannya production. Perlu runtime composition/config/trust/provider yang eksplisit dan QA integrasi.
2. **M15 bukan full-domain importer.** [classify/Decide](../internal/migration/foundation/mapping.go) memetakan satu-versi Class ke shadow Class; scores/batches ambigu di-quarantine, session diarsipkan, banyak domain known lainnya `DEFERRED_DOMAIN_SCOPE`. Archive/quarantine berarti bytes/accounting dipertahankan, bukan fungsi Users/Enrollment/Submission/Grade sudah termigrasi.
3. **Planner belum restore executor.** [PlanRestore](../internal/migration/backup/restore_plan.go) menghasilkan static steps dan false authority/restore flags. Hash/header/metadata validation tidak membuktikan authenticated plaintext, validitas seluruh SQL atau actual restored rows/files.
4. **Age stream belum operational exporter.** Caller memiliki output writer; protected exclusive file creation/key custody/ACL belum selesai. Format baru adalah tar berisi binding+payload di dalam age, terpisah dari legacy Seal/Unseal 32 MiB. [Batas format/tool](baseline/AGE_STREAM_RECOVERY.md); tidak ada fallback plaintext atau conversion otomatis.
5. **Real identity/security legacy belum closed.** Jangan mengimport session lama menjadi login aktif atau mempercayai issuer live hanya karena bridge fixture lulus. M02/M04 dan resource ownership tetap perlu bukti actual target sebelum real traffic.
6. **Frontend produk menunggu verifikasi.** Pemilik proyek meneruskan frontend produk. `/fe-test` sudah membuktikan tiga alur browser lokal. Cakupan semua layar dan acceptance frontend produk diuji dengan [checklist integrasi](INTEGRATION_CHECKLIST.md).

## 6. Yang dapat disiapkan user sekarang

Siapkan informasi dahulu; **jangan kirim password/DSN/private key dalam Markdown atau Git**. [Intake contract](baseline/REAL_BASELINE_INTAKE.md) memuat minimum evidence. Label environment tidak ditebak dari nama `.env`; konfigurasi sebelumnya yang diperiksa telah diidentifikasi user sebagai staging.

| Paket persiapan | Isi yang diperlukan | Mengapa |
|---|---|---|
| Identitas live dan staging terpisah | Bukti server/project/database identity, deployed artifact/SHA, waktu observasi dan owner; opaque private refs untuk secrets | Membuktikan source dan target berbeda serta source sesuai aplikasi yang benar-benar melayani murid |
| Metadata database | Engine/version, extensions, schema/tables/keys/FKs/uniques/indexes/sequences, ukuran dan count scope; jobs/integrations | Memeriksa schema drift, compatibility, capacity dan side effects. Jangan hanya mengasumsikan schema staging sama dengan live |
| Scope snapshot | Tabel/schema/files yang masuk, exclusions beralasan, snapshot/run ID, timestamp, consistent custom dump + SQL dari snapshot sama, checksum/bytes/tool versions | Row count/hash harus dibandingkan terhadap snapshot yang sama; live tetap bergerak |
| File/object inventory | Bucket/key/version/ref privat, expected size/hash dan missing-file disposition; backup byte file sesuai scope | Dump PostgreSQL tidak mengandung attachment/object storage |
| Source access/window | Dedicated read-only role, TLS CA/hostname verification, timeout/load window, owner/reviewer | Export read-only tetap memberi beban/lock. Tidak perlu memberikan credential live ke agent bila operator mengekspor sendiri |
| Private storage/key recovery | Lokasi protected, disk capacity, siapa memiliki key, tempat key terpisah dan prosedur pemulihan | Enkripsi PASS tidak membuktikan ACL/custody/independent recovery; key hilang berarti backup tidak dapat dipulihkan |
| Target dan staging pre-state | Target restore lokal baru; inventori/backup existing staging sebelum perubahan masa depan; namespace/write owner baru | Baseline local, staging v3 dan live berbeda. Jangan restore dump ke staging existing atau overwrite data staging |
| Keputusan akademik minimum | Arti status/enrollment/retake, legacy grade/release/completion, attendance/permit dan logbook schedule/mentor | Menutup ambiguitas mapping tanpa mengarang histori/nilai/kelulusan |

Snapshot/metadata dapat diperoleh operator dengan prosedur organisasi yang sudah aman. Tool v3 saat ini belum menjadi autonomous live exporter. Penerimaan/pemrosesan dump nyata oleh tool baru menunggu storage/custody/recovery gates; menyiapkan metadata sekarang tidak menunggu seluruh fitur/frontend selesai.

## 7. Urutan pengerjaan sesudah rangkuman ini

Urutan ini memecah **deliverable yang sudah ada**, tidak menambah nomor CP baru.

| Urutan | Hasil kerja | Dependency / batas keluar |
|---|---|---|
| 1 — kontrol backup lokal | Previous seven-case owner gate accepted; current recovery tree adds two cases, so execute/review all nine. Sandbox has five fixture AccessDenied failures | Mendukung M01-CP02 / DLV-03/04. Crypto suite yang diterima direuse selama source/tool/input terkait tetap sama |
| 2 — pemulihan operasional sintetis | Small runner implemented: same command continues after native nine-case PASS into ten actual DB stages (backups, custom/SQL/object recovery, two target restores, parity/FK/normal stop). These DB stages remain NOT_RUN | Memerlukan 1 dan backup-before-mutation. Bukan sekadar static restore plan; tanpa live/staging dan tanpa destructive overwrite |
| 3 — QA runtime/publikasi | Tutup current Playwright bootstrap; review pending changes/secret/artifact boundaries; publikasi hanya ketika diminta, lalu actual remote CI/race/PG | Bisa paralel dengan persiapan metadata. Remote run butuh published commit; dokumen ini tidak meminta/menjalankan push |
| 4 — handoff baseline | Reconcile DLV-01–05 tooling/control evidence + receipt identity/scope/capacity/custody; nyatakan missing input spesifik | DLV-06 tetap NOT_READY sampai exit benar-benar terbukti; tidak menutup full M00/M01 dari fixture |
| 5 — baseline nyata lokal | Review metadata/SQL/extensions, verify/decrypt snapshot, restore **salinan** ke DB lokal baru terisolasi, reconciliation dan source adapter reviewed | Baru perlu actual snapshot untuk schema/semantic drift yang tidak dapat dijawab mock. No legacy startup, cron, provider, email, webhook atau login replay |
| 6 — mapping dan runtime | Full-domain mapping/dedup/ownership/chronology/provenance, M06/M08/M12 consumer closure; real identity/security dan shared runtime composition sesuai scope | Kebijakan M11 yang belum dipilih tetap diblokir untuk aktivasi; bukan semua M11 harus selesai sebelum menerima snapshot pertama |
| 7 — staging rehearsal | M16 snapshot pertama + snapshot/delta kedua, idempotency/conflict/resume/recovery/business parity pada target terisolasi; staging pre-state tetap dibackup | Tidak menjadikan live source sebagai write target, tidak overwrite staging existing, tidak menghapus data lama |
| 8 — pilot dan rollout | M17 frontend/session/authority readiness, drained jobs/write fence, final delta/single writer/recovery; kemudian M18 per-wave acceptance | Izin traffic/write/cutover adalah gate terpisah jauh setelah menerima snapshot. Belum siap sekarang |

**Titik kebutuhan data live:** mock tidak dapat memastikan schema/deployed SHA/populasi/chronology/orphan/file byte/arti field actual. Setelah tooling dan penerimaan input siap, hentikan memperpanjang fixture tanpa pertanyaan baru; minta baseline spesifik lalu kerjakan di restore lokal. Tidak harus memberi backend v3 koneksi langsung ke DB live.

## 8. Kesiapan: jangan satukan tiga gerbang

| Gerbang | Status 17 November | Yang boleh disimpulkan |
|---|---|---|
| Menyiapkan metadata/scope/owner/capacity | Dapat dimulai user | Persiapan read-only sesuai prosedur operator; bukan izin agent mengakses DB |
| Tooling menerima/memproses real snapshot | **Belum READY** | Crypto slice PASS; protected output/custody, package recovery wiring, actual restore dan upstream gates masih terbuka |
| Menjalankan Go terhadap staging/production atau cutover | **Belum READY** | Runtime/identity/security/mapping/rehearsal/frontend/pilot belum lengkap; env switch bukan solusi |

Sebelum cutover, recovery juga harus memperhitungkan **writes yang terjadi setelah switch**. Mengarahkan traffic kembali ke DB lama tanpa write fence/delta reconciliation dapat menghilangkan perubahan baru. Rencana recovery harus menjaga semua data/receipts dan satu writer berwenang; jangan mengartikan rollback sebagai restore-overwrite atau menghapus data.

## 9. Storage, backup dan publikasi yang perlu dijaga

- [QA storage policy](QA_STORAGE_POLICY.md): admission global 32 GiB dengan 2 GiB run reserve dan 8 GiB minimum disk floor. UNKNOWN/inaccessible tidak dianggap nol; angka penggunaan lama bukan hasil inventory hari ini.
- Retensi otomatis future-only memiliki scope/cutoff dan proof sendiri; target global 2 PASS + 1 failed synthetic cluster yang eligible, dengan HOLD/pins/exceptions. Backlog historis dan private backups tidak otomatis hilang. **Tidak ada jaminan pertumbuhan disk nol**: logical backups/cache/private evidence terus bertambah dan perlu review terukur.
- Crypto runner yang baru tidak membuat cluster atau memanggil deletion hook. Hasil ringkas membantu navigasi tetapi tidak menggantikan backup/restore proof. Budget baseline nyata harus dihitung terpisah; jangan memaksa dump real masuk budget QA sintetis.
- Backup developer: source/uncommitted changes, docs/QA manifests, `backend-v3/portability`, private evidence/SQL-custom receipts, policy/pins dan credentials/CA/key references yang diperlukan. **Secrets/keys/dumps tetap privat dan terpisah dari public source/Git.** Go caches boleh diregenerasi dengan network/toolchain yang benar; jangan menyamakan cache dengan data penting.
- Perhatikan worktree pointer ke `backend/.git` dan root QA/cache di luar `backend-v3`; [restore map](../../portability/RESTORE_MAP.md) dan [root context index](../../portability/ROOT_CONTEXT_INDEX.md) menjelaskan packaging lama. Snapshot packaging bertanggal lama bukan jaminan semua perubahan terbaru sudah terarsip.
- Sebelum publikasi nanti: review diff, tidak ada env/key/dump/raw logs/cache, pertahankan history/evidence, lalu commit/push `be-v3` hanya ketika diminta. Rangkuman ini tidak mengubah Git refs/index/remote.

## 10. Cara melanjutkan oleh developer/agent berikut

1. Baca dokumen ini, [roadmap DLV](DELIVERABLE_ROADMAP_TO_REAL_BASELINE.md), [peta M/CP](MODULE_CHECKPOINT_MAP.md) dan [intake](baseline/REAL_BASELINE_INTAKE.md).
2. Cocokkan actual executable fingerprint dengan accepted evidence; baca `review.json` crypto dan source-bound business closure. Jangan menjadikan paragraf "latest" bertanggal lebih lama sebagai current jika bertentangan.
3. Run/review the bundled owner command for native nine-case gate and ten-case local recovery. Do not rewrite the accepted seven-case dependency or lower ACLs because sandbox fixture access fails. No stage/live activation from these local results. Jangan memperluasnya diam-diam ke restore live, production adapter penuh atau deployment.
4. Untuk tiap deliverable: catat M/CP/D yang didukung, source/test/hash, scope, exact case results, backup/recovery evidence, remaining blockers dan next safe action. Markdown bukan otomatisasi trigger.
5. Hindari dua penyimpangan yang sudah terlihat dalam dokumen lama: stale next-action pada entrypoint/frontend handoff, serta penyamaan synthetic CP completion dengan real importer/runtime completeness. Rekonsiliasi entrypoint dilakukan bersama rangkuman ini; historical evidence tidak dihapus atau diberi PASS baru.

**Rekomendasi langsung:** user siapkan metadata/identity/scope dan perkiraan ukuran live/staging; developer review owner nine-case/ten-stage local recovery results; then complete actual metadata/control and shared-runtime gates. Keduanya dapat berjalan bersamaan tanpa mengubah live/staging atau menunggu frontend final.
