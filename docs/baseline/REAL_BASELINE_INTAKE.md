# Persiapan penerimaan baseline production secara offline

> **19 November — accepted local public baseline restore:** [Evidence](../checkpoints/2026-11-19-local-legacy-restore-accepted.md): **11/11 native cases + independent artifact/receipt/cold shutdown review PASS** for 21 tables/7,843 rows. Actual local restore verified and stopped; bounded acquisition/restore task complete. Next read-only real mapping, not another export. User-declared source identity/sync, privileged source credential, managed-schema/R2 exclusions, same-disk key custody, full DLV-06/runtime/migration/cutover gates remain explicit. Restore/review never reconnected source/R2; no delete/commit/push. Earlier NOT_RUN notes are history.

> **19 November — local startup diagnosis:** [Fix and owner handoff](../checkpoints/2026-11-19-local-restore-startup-fix.md): actual new local server was ready, but Windows pg_ctl PIPE capture hung before user DB creation/restore. File capture and prior-live-listener guard added, 38 offline tests + native synthetic process-I/O test PASS. Actual 11-case restore still NOT_VERIFIED; source snapshot/crypto acceptance unchanged. No source/staging connection, R2, deletion or DLV-06 closure.

> **18 November — local restore preparation:** [Operator/contract](LOCAL_LEGACY_RESTORE.md), [evidence](../checkpoints/2026-11-18-local-legacy-restore-prepared.md). Actual reviewed pinned public snapshot: 21 tables/7,843 rows, 4 enums, 27 FK, zero sequences, uuid-ossp dependency. 34 offline tests PASS. Actual new-local restore **NOT_RUN / 0 of 11 cases** due Codex private-output PermissionError; owner command prepared. Export acceptance remains valid; no staging/R2 reconnection, full migration or DLV-06 readiness implied.

> **18 November — accepted public staging snapshot:** [Receipt/scope](../checkpoints/2026-11-18-staging-snapshot-accepted.md): owner run `20261118T131839Z-staging-baseline-3fe3bf70b4d4` exported public application data. Independent local review verifies plaintext/encrypted hashes, both authenticated decryptions and 21 archive table-data entries. **Acquisition/export accepted only; actual restore NOT_RUN.** Source remains USER_IDENTIFIED_STAGING, role not least-privilege, independent key custody/full project/R2 coverage not established. Proceed to dump review and isolated restore preparation; keep DLV-06 and full migration gates open. Older no-input/no-export states are historical.

> **18 November — latest diagnostic correction:** [Client/pooler TLS fix](../checkpoints/2026-11-18-staging-client-tls-fix.md): owner runner incorrectly used backend `pg_stat_ssl` as client TLS proof. Same-connection client TLS1.3/verify-full and read-only now verified by corrected actual preflight; 19/19 offline tests PASS. Public snapshot export/restore still NOT_EXECUTED; normal-owner rerun remains necessary. No credential privilege certification, production distinction, complete project/R2 backup or DLV-06 acceptance is implied.

> **18 November — staging acquisition:** [Observed metadata/blocker](../checkpoints/2026-11-18-staging-local-snapshot.md), [bounded owner export](STAGING_LOCAL_SNAPSHOT.md). User authorized read-only access to existing staging credential; actual existing-CA TLS/read-only observation passed. Role has broad rights and is **not certified least-privilege**; source remains USER_IDENTIFIED_STAGING. Public-only acquisition tooling is added, 12 offline tests PASS; native export is blocked by Codex private-folder write denial. No dump/restore, full-project backup, R2 capture or DLV-06 acceptance. Do not interpret older no-connect/no-exporter statements as current facts, or remove synthetic fences for real data.

> **17 November — current recovery:** [Seven-case dependency accepted](../checkpoints/2026-11-17-protected-package-accepted.md); [new local recovery implementation](../checkpoints/2026-11-17-local-recovery.md): **81 Go/95 ops PASS**, current native **4/9 PASS + 5 fixture AccessDenied FAIL**, ten DB cases NOT_RUN. [Owner runner](LOCAL_RECOVERY_RUNBOOK.md); [why dotenv is not staging activation](STAGING_CONFIGURATION_READINESS.md). No native recovery/module acceptance, env/DB edits or publication. Earlier source-bound snapshots below are history.

> **17 November — current:** [Protected output/package implementation](../checkpoints/2026-11-17-protected-package.md), source `126c217ad43a732e67d5d3b32473233ed5bd94385596a78872f0ba9b7b63371b`: **79/79 Go + 93/93 ops + vet/Windows build PASS**, Linux cross-build/tagged compile PASS_BUILD_ONLY. Mandatory actual suite **4 PASS + 3 fixture AccessDenied FAIL**; restricted token confirmed. **IMPLEMENTED_PENDING_OWNER_NATIVE_QA**, no CP/module/restore closure. [Contract/runner](PROTECTED_PACKAGE_RECOVERY.md). Earlier source-bound results below remain history; no DB/delete/commit/push.

> **Accepted latest, 16 November:** [Actual synthetic age slice reviewed](../checkpoints/2026-11-16-age-stream-accepted.md): 4/4 crypto + 75/75 Go/vet PASS, same-source 92/92 ops reused. Tool/storage blockers below are historical. Protected output/custody, authenticated package-consumer wiring and actual restore remain open; DLV-06 NOT_READY. No need for live snapshot solely to finish those local tooling gates.

> **16 November — latest:** [Age stream adapter and exact crypto QA handoff](AGE_STREAM_RECOVERY.md), [evidence](../checkpoints/2026-11-16-age-stream-handoff.md). Implementation now exists without new root Go module dependency; actual pinned-tool crypto execution remains NOT_RUN. Protected exporter/key custody, package-decrypt integration, actual restore and DLV-06 readiness remain open. Earlier statements that no streaming code exists are historical; no real-baseline input/authority is fabricated.

> **Latest 16 November:** [Package validator dan restore planning](PACKAGE_VALIDATION_AND_RESTORE_PLAN.md), [evidence](../checkpoints/2026-11-16-package-validator-restore-plan.md): poin 3/4 PASS offline, Go 71/71 dan ops 90/90. Encrypted large backup masih dependency-blocked; actual source/catalog completeness, authentication/object reconciliation, role/target/backup/restore dan DLV-06 readiness tetap external. Tidak ada source/live/staging connection.

12 November 2026. Mendukung DLV-03–06 pada [roadmap](../DELIVERABLE_ROADMAP_TO_REAL_BASELINE.md). **Dokumen persiapan, bukan executable gate atau izin export/import.** Tidak ada identitas production, approval, dump atau reviewer yang difabrikasi dari contoh.

> **Update tooling offline:** [Preflight dan streaming integrity](OFFLINE_PREFLIGHT_AND_STREAMING.md), [evidence](../checkpoints/2026-11-12-baseline-preflight-streaming.md). CLI Go preflight + actual capacity/no-overwrite tests lulus; checksum streaming >33 MiB tersedia. Enkripsi streaming/protected export belum diimplementasikan karena dependency unavailable. Source/role/TLS/approval dan actual restore tetap external; tabel di bawah mencatat reusable tools dan gap, bukan full importer.

## Batas alat yang benar-benar ada

| Komponen | Kemampuan dalam source saat review | Gap sebelum data nyata |
|---|---|---|
| [backup-plan CLI](../../cmd/backup-plan/main.go), [backup.Build/VerifyArtifacts](../../internal/migration/backup/plan.go) | Planner stdin offline, distinct fingerprint/scope/path/capacity declarations, checksum/size/freshness dua artefak. Selalu `authorizes_execution=false`. | Declaration bukan server observation, actual role review atau persetujuan operator; hash cocok belum membuktikan snapshot lengkap/restore. Belum merupakan exporter production. |
| [backup.Seal/Unseal](../../internal/migration/backup/seal.go) | AES-GCM bound run/target/scope dan umur; plaintext dibatasi **32 MiB**. ProtectedDestination melakukan pemeriksaan host. | Jangan menaikkan batas tanpa desain atau membaca dump besar seluruhnya ke RAM. Jalur enkripsi streaming, ukuran nyata dan key recovery terpisah belum terbukti. |
| [dbinspect.Inspect](../../internal/platform/dbinspect/probe.go) | TLS peer/hostname, read-only repeatable-read transaction, bounded statement/lock timeouts, catalog/dependency observation. | Status environment saat ini `USER_IDENTIFIED_STAGING`, least privilege/distinct production dan full review tetap false/blocker. **Jangan mengganti label staging menjadi production dan menganggapnya tervalidasi.** Jangan menjalankan inspector ke source nyata dalam pekerjaan offline ini. |
| [foundation.Preflight/Capture](../../internal/migration/foundation/foundation.go) | Synthetic-only provenance, inventory baseline 20 tabel ditambah discovered objects; PK/version/hash, count accounting, lock/transaction/replay. | Fenced ke `qa_fixture`, `qa_cp05`, loopback dan synthetic marker/backup proof. Belum source adapter production. Jangan sekadar menghapus guard agar menerima live. |
| [foundation.classify/Decide](../../internal/migration/foundation/mapping.go) | `classes` satu versi → `MAP_CLASS`; ambiguous Class, score dan batch → quarantine; session → restricted archive; domain known lainnya → `DEFERRED_DOMAIN_SCOPE`; unknown table → quarantine. | **M15 5/5 local slices bukan seluruh domain sudah dapat diimport.** Aturan fixture tidak mengesahkan pemetaan pengguna, enrollment, submission, grade, attendance atau logbook nyata. |
| [foundation.Reconcile](../../internal/migration/foundation/reconcile.go) | Read-only snapshot, landing/decision/checkpoint/hash accounting, shadow Class/mapping receipts dan unpublished state. | FILE scope hanya manifest accounting, bukan verifikasi byte object; belum business parity seluruh domain, object recovery atau pilot. |
| [migrate CLI](../../cmd/migrate/main.go) | Forward-only catalogue pada target baru local-synthetic dengan gate/backup. | Bukan alat migrasi shared/staging/production. Jangan menunjuk CLI ini ke source nyata. |

## Kontrak input yang perlu disiapkan

Simpan isi data/receipt lengkap dalam lokasi privat di luar Git; dokumen publik hanya berisi opaque reference, status dan hash yang layak dipublikasikan. Nama file berikut adalah kategori kebutuhan, bukan file wajib dengan schema parser baru yang sudah diimplementasikan.

| Kelompok input | Isi minimum | Cara pemeriksaan dan kondisi stop |
|---|---|---|
| Source identity | Referensi production yang diverifikasi operator, DB/server identity, deployed SHA/artifact, schema fingerprint dan waktu observasi | Cocokkan dengan bukti independen; nama `.env`, nama host berisi `prod`, atau local checkout SHA saja tidak cukup. UNKNOWN menghentikan klaim verified production. |
| Access / trust | Dedicated source read-only role, permission scope, trusted CA/hostname, reviewer dan batas waktu operasi | Read-only transaction tidak membuktikan credential tidak bisa menulis. Belum ada role → permintaan provisioning terpisah; tidak membuat role/grant otomatis. Tidak merekam password/DSN/token di Git/log publik. |
| Inventory | Seluruh schema/table/column/PK/FK/unique/index/extension/sequence, jobs/integrations, file references dan object inventory | Tambahan di luar baseline 20 tabel harus di-account. Metadata lintas schema/jobs/storage tidak boleh dianggap selesai dari sampling public tables. Count estimasi statistik diberi label estimasi. |
| Snapshot DB | Run/snapshot ID, source identity, scope/exclusions beralasan, timestamp, tools/version, consistent custom archive, SQL dari snapshot yang sama, bytes dan SHA-256 | Dua export terpisah tanpa exported/shared snapshot dapat berbeda. Hindari pembandingan dengan live yang sudah bergerak. Checksum/readable archive saja belum restore proof. |
| Files | Bucket/scope/version/key reference yang diproteksi, expected bytes/hash, missing-object disposition | Dump PostgreSQL tidak membawa byte object storage. Jangan membuka ulang URL publik legacy atau menyalakan callback untuk mengisi file yang hilang. |
| Custody | Private destination/ACL/encryption, key reference terpisah, recovery procedure, operator/reviewer dan akses pihak frontend | Key tidak disimpan bersama dump dalam Git. Windows sandbox admission tidak dianggap bukti host owner aman. Tidak mengubah ACL untuk menghilangkan error tanpa review. |
| Capacity / timing | Ukuran DB/object, perkiraan dump+SQL+restore+indexes/WAL/temp, disk headroom dan source load window | Budget QA sintetis 32 GiB/per-run 2 GiB bukan otomatis budget baseline nyata. Stop bila ukuran/ruang/permission tidak diketahui; tidak membersihkan backup/data lama untuk memaksa masuk. |
| Restore destination | Identitas target lokal baru kosong, PostgreSQL/extension compatibility, outbound/network/jobs disabled, owner scope | Tidak memakai live atau staging existing, tidak `--clean`, overwrite, DROP/TRUNCATE. Review executable SQL/extension/function/trigger sebelum restore; `--no-owner/--no-acl` bukan sandbox untuk kode dump yang tidak dipercaya. |

## Urutan aman saat operator nanti menyediakan baseline

1. Terima receipt/metadata dahulu. Verifikasi scope/custody/kapasitas; catat missing input sebelum menerima file besar. Tidak perlu credential live dikirim ke Codex jika operator melakukan export sendiri.
2. Source export read-only hanya setelah window/role/TLS dan scope diotorisasi. Dump tetap membebani source dan dapat mengambil lock; timeout atau DDL bersamaan menghasilkan STOP dan receipt kegagalan, bukan retry tanpa batas.
3. Hash/reopen archive dan inventory file; kunci input snapshot agar mapping/testing tidak mengubah baseline asli. Fresh backup diperlukan sebelum setiap migration run yang memang diotorisasi.
4. Restore salinan ke **DB lokal baru** pada lingkungan yang diisolasi; jangan start aplikasi legacy, cron, notification/provider atau identity issuer. Verifikasi schema/row count/key/relation/hash/numeric/file checks terhadap snapshot yang sama. Privilege/extension provisioning yang diperlukan direview terpisah.
5. Definisikan source adapter untuk receipt/baseline itu, terpisah dari synthetic lab fences. Review mapping bersama owner dan dry-run terlebih dahulu; tidak ada apply shared dari dokumen ini.
6. Reconcile dan review; kalau input/mapping/source/target/owner epoch berubah, receipt lama tidak membuka run baru. Simpan before-image/provenance, jangan hard-delete data yang tidak cocok.

## Ledger keputusan mapping

Setiap source entity/version harus punya keputusan eksplisit. Minimal field: source instance/environment, snapshot/table/PK/version/raw hash, target namespace/entity/ID, mapping version, disposition/reason, reviewer reference, dependencies dan evidence reference. Source yang belum dipahami ditahan; tidak dipaksa masuk target dengan default.

| Area | Keputusan yang harus ditutup dari baseline nyata |
|---|---|
| Users / identity | Dedup identity, account disabled/history, authority owner dan session revocation/bridge. Jangan mengimport session legacy menjadi login aktif secara otomatis. |
| Class / program / batch | Definisikan Class delivery, label manual, mapping program/batch metadata dan historic ownership tanpa menjadikan cohort dependency universal. Nama sama bukan identity yang sama. |
| Enrollment / role / groups | Pisahkan participation episode, contextual role/capability, group/personal mentor, invitation dan retake. Tidak menyamakan enrollment dengan role. |
| Content / work | Stable content revision, assignment contract, submission author/attempt/receipt/deadline, attachment ownership dan chronology. Missing FK tidak diisi dengan tebakan. |
| Academic outcomes | Rubric/version, score provenance, release/regrade/history, completion policy/evidence/transcript. Nilai tidak otomatis berarti completion. AI bukan sumber grade final. |
| Attendance / mentoring | Kalender/timezone, permit/proof privacy, period/logbook revision/review dan authority. Policy M11/M12 yang belum dipilih tetap blocked untuk aktivasi fitur tersebut. |
| Files / jobs / audit | Byte integrity, private ownership, preserved audit, no automatic outbound jobs/callbacks, stale/unknown event handling. |

Accounting membedakan `MAPPED`, `QUARANTINED`, `ARCHIVE_ONLY` dan exclusion beralasan. **ARCHIVE_ONLY bukan berhasil memigrasikan fungsi bisnis**, dan quarantine bukan data boleh dihapus. Jangan mengubah enum/fungsi fixture sekarang hanya supaya menyerupai ledger rencana ini.

## Status persiapan dan readiness

- DLV-03: kontrak dan reusable predicate tersedia; operational acquisition/trust/role/key review belum lengkap. Dokumen ini sendiri tidak menutup DLV-03.
- DLV-04: synthetic dump/restore historis tersedia; exporter baseline operasional, streaming protection/capacity, executable-dump review dan actual source receipt masih gap. Tidak menjalankan cluster/dump baru dalam penulisan dokumen ini.
- DLV-05: actual scope dan deferred-domain gap diidentifikasi dari kode; production source adapter serta pemetaan lintas domain belum dibuat/direview. Tidak ada klaim real importer selesai.
- DLV-06: checklist handoff disiapkan; **`READY_FOR_REAL_BASELINE_INPUT` belum diberikan**. Butuh evidence DLV-01–05 dan gate operasional yang relevan, bukan keberadaan Markdown saja.

Jika pekerjaan independen sudah selesai tetapi masih kekurangan metadata/snapshot/owner, hentikan jalur migrasi pada `BLOCKED_REAL_BASELINE_INPUT` dan sebutkan input spesifik yang kurang. Tidak perlu memproduksi fixture tambahan tanpa pertanyaan baru yang dapat dibuktikan. Kebutuhan baseline read-only dan izin menulis/cutover production tetap dua gate berbeda.
