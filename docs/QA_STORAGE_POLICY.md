# Local QA storage policy — 11 November 2026

Latest accepted business QA: [M14 4/4 local synthetic closure](checkpoints/2026-11-11-m14-local-closure.md), run `20261111T093737Z-postgres-lab-6e4bc487c4d6`. Owner inventory, fresh full journey, normal stop/compression, post-stop retention and final evidence review passed. Historical sandbox UNKNOWN_BYTES observations below remain valid for that restricted context, not the owner-terminal inventory. Current owner retention snapshot: 1 DISPOSED, 53 historical candidates outside auto-cutoff, 3 KEEP, 14 HOLD; no disposal by this run. Native lifecycle hook execution is now evidenced; global byte/exception/pin/backup safeguards and wider canonical gates remain.

The development/build environment remains on this Windows laptop. This policy defines storage safeguards; CP acceptance is recorded separately in the source-bound closure linked above.

Latest readiness: [68-case preflight and owner handoff](checkpoints/2026-11-11-qa-readiness-handoff.md) are verified. `portability/CONTINUE_M14_CP04_QA.cmd` performs the read-only owner inventory and invokes existing QA only after admission passes. Running it in the sandbox still gives **BLOCKED_BEFORE_QA** for two inaccessible scratch directories; no gate bypass, ACL takeover or additional historical disposal. Preparation evidence is distinct from full native QA.

## Why clusters existed

The PostgreSQL lab creates a fresh synthetic cluster for each full journey. Before each migration it writes SQL/custom backup, restores into an isolated proof database and compares schema/data. Keeping those proof databases and every failed attempt expanded storage indefinitely. A report saying PASS is useful navigation, but cannot replace test-case evidence, source hashes or actual backup/restore proof.

Previously measured logical sizes: external QA 21.22 GiB, historical service-local QA 17.58 GiB, service cache 1.52 GiB and private backups 1.44 GiB; roughly 41.8 GiB total. These are baseline file lengths, not post-compression usage. One inaccessible historical pytest folder, plus a restricted-token pytest scratch directory from this maintenance attempt, remain explicitly unmeasured inside the sandbox. Do not report an incomplete inventory as a certified budget.

## Admission dan kompresi v2 — tetap berlaku bersama rollout v3

- One active result root: workspace `lms-v3-qa-artifacts`; bootstrap, release build and PostgreSQL runners use it. Historical `backend-go/.qa-artifacts` stays readable where permissions permit; evidence/backups remain intact. Only proven raw synthetic payloads can be removed under the separate v3 rollout below.
- One active Go build cache: `lms-v3-qa-artifacts/go-cache`. The module download cache remains `backend-go/.cache/go-mod` for offline reproducibility. Old build caches are retained; no dependency purge.
- PostgreSQL, bootstrap and artifact-build preflight account for both QA roots, service cache and private backups. Policy v2 admission ceiling **32 GiB**, including required **2 GiB reserve**; minimum disk floor **8 GiB**, so admission needs at least 10 GiB free. PostgreSQL checks its 2 GiB per-run budget/floor every 100 commands; build/bootstrap check after each command. These are refusal/stop gates, never deletion triggers. A shared `storage.lock` serializes these runners and maintenance. These are checkpoints rather than an OS-level write quota; a command/cache writes can grow storage between inventory checks.
- Unknown/inaccessible bytes prevent certified full-run admission; use the ordinary owner terminal to inspect those entries. No ACL takeover or hidden zero-size assumption.
- After normal PostgreSQL stop, Windows lab can losslessly NTFS-compress the finalized synthetic run: cluster files, SQL/logs, coverage and QA executables. No SQL DELETE/DROP/TRUNCATE, file unlink, filesystem move or archive replacement occurs. Unsupported platforms retain bytes and remain subject to admission limits; no unsafe fallback cleanup.
- Eligible clusters require exact known QA roots/run name, synthetic summary markers, no reparse paths, no `postmaster.pid`, PostgreSQL 18 control-data state `shut down`, and verified SQL/custom backup receipt hashes. Unknown, active, incompatible or unverified clusters are retained for review. Failed historical QA status remains FAIL even when its files can safely be compressed.
- Each compressed run gets before/after measurements and a separate receipt. `full` mode hashes every file before/after (used for the pilot and initial four bulk results). Default `backup-and-metadata` mode cryptographically checks logical backup archives/receipts, compares every file name/size plus `pg_control` and summary hashes; it explicitly does **not** claim to rehash every raw database byte. This avoids repeated expensive content reads across hundreds of thousands of tiny PostgreSQL files. Native NTFS compression retains file contents. Already space-efficient files are reported as unchanged, not falsely given a new integrity-verification claim. NTFS compression is reversible and does not encrypt private data. Compression starts only after shutdown; performance measurements continue using fresh uncompressed clusters.
- Historical `summary.json`, backups, case files, coverage and receipts are never rewritten. A separate ledger records reported status, source/summary hashes, per-case-group counts and independently checked backup-receipt counts. The ledger does not grant new PASS or production readiness.

## Retensi v3 — pilot native diterima, hook future-only aktif

Dicatat 11 November 2026 atas permintaan pengguna. **Satu pilot historis native PostgreSQL berhasil; hook serial sebelum admission dan sesudah lab sudah terpasang, diaktifkan hanya bagi run sintetis baru sejak cutoff 2026-11-11T06:02:58.248266+00:00.** Maksimum satu kandidat per evaluasi; 51 kandidat historis tersisa di luar scope otomatis awal ini. [Evidence native/hook](checkpoints/2026-11-11-qa-retention-native-and-hooks.md): 60/60 pemeriksaan fixture PASS, native prehook tidak menghapus tambahan, backup/evidence dipertahankan. V2 admission/kompresi tetap berlaku. Inventory masih tidak lengkap untuk dua scratch; full native lifecycle baru dan M14-CP04 belum diterima.

Pilot membuang raw payload sekitar **46,5 MiB**, dengan 322 file di luar cluster tetap identik dan KEEP tidak berubah pada indeks nama/ukuran/control/summary. Sisa saat review: 69 folder run berisi **1 DISPOSED, 51 CANDIDATE, 3 KEEP, 14 HOLD**, ditambah dua scratch. Estimasi 9,05 GiB sebelum pilot bukan ruang yang sudah dibebaskan. [Evidence apply awal](checkpoints/2026-11-11-qa-retention-apply.md) tetap histori tahap fixture, bukan status terbaru.

### Data yang dipertahankan

| Jenis | Kebijakan target |
|---|---|
| Evidence ringkas seluruh run | Pertahankan summary asli, ID/status kasus, coverage, source/manifest/schema hash yang tersedia, hasil rekonsiliasi, dan receipt backup/restore. Katalog hanya indeks; tidak mengganti bukti atau memberikan PASS baru. |
| Backup milestone | Pertahankan SQL/custom backup yang terverifikasi beserta receipt restore. Backup sebelum migrasi wajib tetap dijalankan. Pada fase ini tidak ada penghapusan backup otomatis, termasuk backup per-run yang menjadi bukti disposal. |
| Cluster PASS | Pertahankan **dua cluster PASS terbaru** yang memenuhi verifikasi retensi. |
| Cluster gagal | Pertahankan **satu cluster gagal terbaru** yang sudah berhenti dan memenuhi verifikasi retensi, untuk debugging. |
| Run aktif, tidak dikenal, tidak terverifikasi, atau sedang diinvestigasi | Lindungi sebagai pengecualian; jangan dibuang untuk memaksakan hitungan atau kuota. Catat alasan dan tindakan berikutnya. |

Hitungan **2 PASS + 1 gagal berlaku global untuk cluster PostgreSQL sintetis**, melintasi kedua root QA yang dikenal; bukan jatah baru per CP, modul, folder, atau tanggal. PASS build/bootstrap tidak dihitung sebagai cluster PASS database. Urutan menggunakan waktu run UTC yang tercatat, dengan timestamp run ID sebagai fallback; jangan memakai mtime/ctime yang dapat berubah akibat kompresi atau penyalinan.

**Pengecualian rollout:** seleksi global tidak otomatis memberi scope disposal historis. `qa-retention-automation.json` hanya mengizinkan run ID dan waktu run yang keduanya sesudah cutoff aktivasi. Historical backlog dan KEEP historis yang kelak keluar quota tetap di luar scope otomatis. Jumlah raw aktual belum tiga; perluasan backlog harus ditangani bertahap dalam scope yang jelas. Pin/HOLD/unknown tetap dilindungi. Tidak ada config berarti otomatisasi DISABLED; config dan proof aktivasi berada di parent package dan perlu dibackup saat restore.

Status FAIL_STOP_REQUIRES_OPERATOR, cluster yang masih berjalan, backup rusak/tidak ada, atau identitas ambigu tidak boleh dianggap aman hanya karena paling tua. Pengecualian/pin dapat membuat jumlah aktual melebihi tiga cluster. **Delapan cluster tanpa backup terverifikasi dan dua folder scratch yang belum terbaca tetap menjadi pengecualian**, sampai review menyelesaikannya. Menghemat ruang tidak boleh mengubah FAIL menjadi PASS.

### Waktu evaluasi dan syarat pembuangan

1. Evaluasi retensi **setelah setiap full QA selesai dan proses PostgreSQL berhenti**, bukan menunggu sejumlah run bertumpuk. Sebelum run berikutnya, preflight memeriksa ruang dan rencana retensi kembali.
2. Buat rencana dry-run yang mencantumkan path absolut terverifikasi, ukuran, sumber status, cluster yang dipertahankan, kandidat pembuangan, dan semua pengecualian.
3. Pembuangan hanya boleh menyasar subfolder fisik **`postgres/`** milik run sintetis yang terbukti berada di root QA yang dikenal. Jangan menghapus folder run keseluruhan, parent workspace, logs, source, evidence, backup, atau executable melalui kebijakan cluster ini.
4. Wajib ada identitas sintetis yang cocok, tidak ada symlink/junction/reparse traversal, tidak ada run aktif menurut OS lock, tidak ada `postmaster.pid`, dan control-data membuktikan clean shutdown. Ambil kunci penyimpanan yang sama dengan runner dan periksa ulang kandidat tepat sebelum tindakan.
5. Verifikasi summary/case evidence, hash source yang tercatat, SQL/custom backup dan receipt actual restore, serta pin/debugging hold. Catat cakupan backup dengan benar: backup sebelum migrasi tidak otomatis merupakan snapshot keadaan akhir run. Keterbatasan reproduksi/source reference harus terlihat dalam laporan.
6. Preview adalah default; apply eksplisit maksimum satu kandidat per pemanggilan. Plan schema v2 berusia maksimum 24 jam mengikat hash source/pin dan identitas fisik cluster; schema v1 ditolak. Journal di-fsync dan intent disimpan sebelum unlink; receipt/marker terpisah baru ditulis setelah payload hilang dan backup/evidence diverifikasi. Intent tanpa completion sah menjadi HOLD untuk review operator, termasuk bila payload sudah hilang; tidak ada blind resume atau rollback otomatis. Retry disposal selesai menjadi no-op terverifikasi. Summary QA asli tidak berubah. Implementasi dan 43 kasus ada pada [evidence apply](checkpoints/2026-11-11-qa-retention-apply.md).
7. Instruksi eksekusi harus jelas mencakup pengecualian pembuangan **cluster QA sintetis** terhadap larangan penghapusan database sebelumnya. Permintaan menyimpan dokumen ini tidak mengeksekusi pengecualian tersebut. Jika otorisasi apply sudah diberikan dalam sesi, tidak perlu meminta konfirmasi berulang untuk scope yang sama.

Database live/staging/production, data real/salinan baseline real, cluster di luar marker sintetis, dan direktori yang tidak bisa diidentifikasi **selalu di luar scope kebijakan ini**. Tidak ada SQL DELETE/DROP/TRUNCATE terhadap database aplikasi. Satu raw cluster historis sintetis sudah dibuang melalui pilot terverifikasi; tidak ada batch disposal historis. Lock dan revalidasi path mengoordinasikan runner kooperatif; Windows rmtree bukan boundary terhadap proses bermusuhan dengan hak user yang sama yang menukar path bersamaan.

### Hubungan dengan batas ruang dan QA

Plafon admission v2 **32 GiB**, reserve **2 GiB**, lantai disk **8 GiB**, dan pemeriksaan pertumbuhan tetap berlaku. Retensi berbasis jumlah bukan pengganti batas byte: tiga cluster besar, pengecualian, cache, atau backup tetap dapat memenuhi kuota. Bila ruang belum cukup, runner menolak run baru dan melaporkan alasannya; tidak membuang cluster yang dilindungi, mengecilkan suite, mengurangi pembuktian restore, atau melonggarkan ambang performance.

Bukti fixture dan pilot native sudah tersimpan. Hook berjalan di luar lock body runner; engine mengambil lock lagi dan memvalidasi ulang sesudah snapshot/backup. Kebijakan juga diperiksa ulang tepat sebelum intent, sehingga revoke/perubahan tidak memakai otorisasi lama. Kegagalan housekeeping dicatat terpisah tanpa mengubah exit/status bisnis QA atau melewati gate ruang. Body yang terinterupsi tidak memulai post-hook disposal baru. Tool kompresi mengenali marker disposal sah dan menahan intent belum lengkap.

Lifecycle native penuh pada source hook baru masih perlu dibuktikan bersama M14 setelah inventory lengkap; 60 tes maintenance bukan Go/DB/Playwright acceptance. Evaluasi berjalan per run, tetapi backup/evidence/cache tetap bisa tumbuh dan historical backlog belum dirampingkan massal. Jangan menjanjikan storage tidak pernah bertambah. [Brief berikutnya](checkpoints/2026-11-11-qa-retention-next-steps.md).

## Operator commands — v2 yang sudah aktif

From ordinary PowerShell, inventory and ledger only:

```powershell
& 'D:\Projects\lms-v2\backend-v3\backend-go\.venv\Scripts\python.exe' 'D:\Projects\lms-v2\backend-v3\portability\manage_qa_storage.py'
```

Compress eligible stopped synthetic clusters with all bytes retained:

```powershell
& 'D:\Projects\lms-v2\backend-v3\backend-go\.venv\Scripts\python.exe' 'D:\Projects\lms-v2\backend-v3\portability\manage_qa_storage.py' --compress-closed-clusters
```

Both use the existing QA OS locks and write new reports into `backend-v3/private-backup/qa-storage-<UTC>`. A locked QA run prevents compression. Compression never starts PostgreSQL or contacts staging/production. Each cluster must be individually eligible; skipped cases remain explicit.

Add `--verification full` for exhaustive raw byte rehashing. It is much slower and remains available; business QA/backup acceptance is not reduced by choosing a storage verification mode. The initial full bulk pass was interrupted after four verified runs because it was taking several minutes per run; its completed receipts remain in `qa-storage-20261110T204215138748Z`. The resumed bulk pass records its different verification mode per run.

## Longer term

The initial 20 GiB ceiling was an estimate, not a user-specified quota. Completed compression measured **22.58 GiB** of retained managed data before accounting for two inaccessible scratch entries. Therefore 20 GiB was not achieved. Operational admission was explicitly rebased to 32 GiB (policy version 2) to preserve historical data plus space for subsequent runs. This is not a relaxation of business/performance QA thresholds and no failed QA was relabeled PASS.

Completed maintenance `private-backup/qa-storage-20261110T205617340946Z`: 67 run dispositions, **55 compressed, 4 already space-efficient, 8 retained without verified logical backups**. Original summaries, case evidence and backups were preserved. The final measured managed footprint is about 22.6 GiB versus initial 41.8 GiB, and D drive free space rose from about 31.1 to 50.4 GiB. The inventory remains explicitly incomplete for the two sandbox-inaccessible scratch directories; owner-terminal inspection is required before certified full-QA admission. `actions.json` records all dispositions; its missing failure list in the original tool version was reconstructed from completed receipts plus read-only scope/backup checks, and future runs now persist it directly.

Keep compact evidence and reviewed milestone backups. V3 has one accepted native historical pilot and future-only automatic hooks; backlog expansion, complete inventory and full native lifecycle acceptance remain open. Lossless compression and bounded raw-cluster retention cannot make indefinitely growing evidence/backups consume zero storage. The current admission budget remains active. No reduction of mandatory backup/restore or acceptance cases is used to save space.
