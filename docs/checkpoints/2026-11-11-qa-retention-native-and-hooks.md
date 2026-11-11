# Retensi QA — pilot native dan hook serial

11 November 2026. Maintenance penyimpanan; tidak menambah CP, mengubah business code Go, menjalankan migrasi shared, atau menerima M14-CP04.

## Scope dan hasil

- User melanjutkan brief pilot satu cluster sintetis dan integrasi retensi. Pilot **satu** kandidat historis selesai; bukan izin eksekusi batch seluruh backlog dari file plan lama.
- Hook serial sudah dipasang dan **diaktifkan untuk run sintetis baru setelah cutoff aktivasi**, maksimum satu kandidat per evaluasi. Seluruh summary asli, SQL/custom backup dan evidence tetap dipertahankan. Ada pengecualian yang sengaja lebih sempit dari target global: **51 kandidat historis tersisa tidak masuk scope otomatis awal ini**.
- **60/60 pemeriksaan lokal PASS**: 43 regresi sebelumnya + 17 automation. Native pre-admission hook juga dijalankan terhadap data QA sebenarnya, tanpa memulai DB atau membuang kandidat tambahan.
- Dua scratch tetap tidak terbaca dari sandbox. Inventory/admission penuh serta lifecycle PostgreSQL/Playwright/Go-DB M14 belum diterima; jangan menganggap hook ini menyelesaikan full module.

## Pilot native yang benar-benar dijalankan

Run `20260907T113620Z-postgres-lab-d8b4e094f916`, root legacy `.qa-artifacts`, sebelumnya PASS sintetis dan di luar 2+1 KEEP. `pg_controldata` PostgreSQL **18.6** membuktikan `shut down`; tanpa PID aktif, reparse, pin atau scope shared. Hash SQL/custom backup cocok dengan receipt restore historis yang telah terverifikasi.

1. [Before state](../../../private-backup/qa-retention-native-pilot-20261111T054806650052Z/before.json) mencatat hash **322 file** di luar postgres, control/identitas cluster, backup/cases, dan indeks tiga KEEP.
2. CLI preview dijalankan, lalu source/context ZIP **448 file, 1.481.946 byte**, dibuka ulang dan setiap anggota diverifikasi, sebelum tindakan. [Backup receipt](../../../private-backup/qa-retention-apply-c95f26281225446b8ee53c609ba636c8/backup-receipt.json).
3. Apply satu target lewat engine yang sudah diuji pada source `9c6f75f68a722787aeb12d72bea62ab3b195edfdd939323180f5f81e1ed83a06`; [journal](../../../private-backup/qa-retention-apply-c95f26281225446b8ee53c609ba636c8/journal.jsonl), [receipt](../../../private-backup/qa-retention-apply-c95f26281225446b8ee53c609ba636c8/receipt.json), dan marker terpisah tersimpan. Hanya raw `postgres/` hilang, folder run tetap ada.
4. [Review sesudah pilot](../../../private-backup/qa-retention-native-pilot-20261111T054806650052Z/review.json): **322/322 file preserved tetap identik**; backup/cases/summary cocok. Nama/ukuran seluruh file KEEP, pg_control dan summary KEEP tidak berubah. Ini **bukan** rehash semua byte raw KEEP.

Payload raw sebelum tindakan: **134.395.555 byte logical**, **48.730.420 byte estimasi alokasi (~46,5 MiB)**. Delta free volume yang dicatat receipt: **49.377.280 byte (~47,1 MiB)**; pengukuran review berikutnya berbeda karena write lain dan overhead backup/journal. Jangan melaporkan delta volume sebagai penghematan eksklusif yang presisi atau mengklaim 9,05 GiB sudah dibebaskan.

Tidak ada PostgreSQL baru yang diinisialisasi, DB yang dinyalakan, SQL yang dieksekusi, atau restore baru pada sesi ini. Proof restore sebelum migrasi yang sudah tersimpan digunakan kembali; **bukan snapshot final-state**. Native pilot ini bukan full native QA terhadap source hook baru.

## Implementasi hook dan batas otomatisasi

| Bagian | Implementasi |
|---|---|
| `qa/retention_automation.py` | Kebijakan eksplisit, cutoff UTC berdasarkan timestamp run ID **dan** waktu summary yang tervalidasi, global 2 PASS + 1 FAIL selection, maksimum satu kandidat per evaluasi. Policy/hash harus berasal dari revisi sama. |
| Sebelum run | `qa.run.execute`, `qa.release.main`, lifecycle `qa.postgres_lab.run_lab`, dan launcher `portability/run_m14_cp04.py` mengevaluasi retensi **sebelum** admission. Storage preflight/growth tetap menolak budget/unknown bytes; cleanup tidak melewati gate. |
| Sesudah lab | Decorator lifecycle mempertahankan signature run_lab. Body berhenti dan melepas OS locks sebelum hook sesudahnya; run yang baru dibuat oleh pemanggilan itu dikecualikan. Exit code bisnis tetap dipertahankan. KeyboardInterrupt yang keluar dari body tidak memulai disposal baru. |
| Lock/plan | Snapshot plan mengunci kedua root, lalu melepas lock sebelum source backup. Engine apply mengambil lock lagi dan mengulang verifikasi global/path/native shutdown/backup. Tidak ada nested re-acquisition atau blind execution daftar path. Benturan lock menahan tindakan tanpa menunggu tak terbatas. |
| Backup | Source/context ZIP kecil hanya dibuat bila ada kandidat dalam scope, mengecualikan env, key/certificate, raw DB, cache dan Git. Hash setiap anggota diverifikasi lewat pembacaan ulang. Backup/evidence tetap dapat tumbuh; admission byte tetap diperlukan. |
| Pembatalan | Kebijakan diperiksa ulang setelah backup **dan tepat sebelum intent engine**. Disable/perubahan policy/source/pin/identity membatalkan tindakan. Intent/receipt/interupsi mengikuti kontrak apply sebelumnya. |
| Kompresi lama | `manage_qa_storage.py` mengenali marker disposal sah sebagai `ALREADY_DISPOSED_NOT_COMPRESSED`. Intent yang belum complete ditahan, tidak dikompresi seolah cluster utuh. |

Aktivasi di `backend-v3/qa-retention-automation.json`:

- `scope`: `FUTURE_LOCAL_SYNTHETIC_RUNS_ONLY`.
- `not_before`: **2026-11-11T06:02:58.248266+00:00**, atau 13:02:58 WIB.
- `max_candidates_per_cycle`: **1**, per evaluasi. Launcher persiapan dan lab masing-masing mempunyai pre-admission, kemudian lab mempunyai post-stop; ini bukan janji maksimum dua tindakan sepanjang seluruh launcher. Seluruh evaluasi tetap serial dan dalam cutoff yang sama.
- Registry aktivasi berada di parent package, bukan di root repo Go. Tidak ada config berarti **DISABLED**. Konfigurasi, pin dan native-pilot proof harus ikut private backup saat restore; jangan menganggap clone Go saja mengaktifkannya.
- [Activation receipt](../../../private-backup/qa-retention-activation-20261111T060258245762Z/activation.json), policy SHA256 `182dbe262ae6f817a0025f39c92a7acfa2f8c3321b9c9db60cfca90fa6e0fd67`.

**Batas rollout yang disengaja:** seleksi tetap global 2+1, tetapi apply otomatis hanya bagi run sejak cutoff. Raw historis, pin/HOLD dan scratch tidak dibuang untuk memaksakan jumlah tiga. KEEP historis yang kelak keluar quota pun tetap di luar cutoff. Jumlah aktual **belum tiga cluster**; tersisa 68 raw cluster historis saat review ini. Perluasan disposal backlog harus dikerjakan bertahap dalam scope yang jelas, setelah review; tidak otomatis tersirat dari plan JSON. Tidak perlu meminta ulang scope yang sudah diotorisasi.

## Evidence source hook baru

Source service: `42af6eb44eced2691b3fd1a63338dad440c00a87ad20050ea436152aa2fe4944`.

- [60 kasus](../../../private-backup/qa-retention-automation-checks-20261111T060053343444Z/cases.json), [summary](../../../private-backup/qa-retention-automation-checks-20261111T060053343444Z/summary.json), [log](../../../private-backup/qa-retention-automation-checks-20261111T060053343444Z/checks.log): PASS, source tetap sama selama tes.
- 17 kasus baru membuktikan scope cutoff/backlog/batas satu, native-proof prerequisite, pin/run baru, policy read race dan revoke sebelum intent, source berubah, lock nyata, source backup tanpa secrets, urutan hook di luar lock dengan exit gagal tetap gagal, interupsi, fixed root, wiring entrypoint, dan tool kompresi disposal-aware. Native-proof dalam tes ini adalah **stub fixture**, bukan PostgreSQL sungguhan; native pilot terpisah di atas.
- [Native prehook review](../../../private-backup/qa-retention-activation-20261111T060258245762Z/native-prehook-review.json): `NO_IN_SCOPE_CANDIDATE`; 69 folder run, **1 DISPOSED, 51 CANDIDATE, 3 KEEP, 14 HOLD**, ditambah 2 scratch. Tidak ada disposal tambahan. Plan SHA256 `2a460f3bcdc9c9128527f25613212d564b08178a83663d4c14b7cd6e8309b9b8`.
- Source hash service tidak mencakup parent portability/policy. Policy diikat hash terpisah; tools dan dokumen parent dicakup oleh archive backup akhir. Tidak ada klaim Go/DB/Playwright atau CI remote baru dari 60 tes ini. M00/M03 full-suite flags tetap false; M13/M14 coverage canonical tidak dinaikkan.

## Yang masih perlu dikerjakan

1. Jalankan [owner inventory review](../../../portability/review_qa_storage_owner.py) di Windows terminal biasa. Script hanya membaca/mencatat, tanpa cleanup/ACL/SQL. Selesaikan exception berdasarkan hasilnya; tidak ada takeover ACL, penghapusan scratch tak dikenal, atau asumsi byte nol.
2. Tinjau/perluas retensi backlog historis secara bertahap dengan plan baru, backup, revalidasi dan bukti preservasi; 51 kandidat tidak berubah menjadi otorisasi batch.
3. Saat inventory lengkap dan admission lulus, jalankan full workload M14-CP04 dari runner biasa. Ini diperlukan untuk membuktikan hook sesudah shutdown pada **native lifecycle penuh**, Playwright, Go/DB, restore/migrasi dan performance. Jangan mengubah threshold atau menganggap pilot disposal sebagai acceptance CP04.

M13 tetap **4/4 lokal historis**; M14 **3/4 lokal historis**, CP04 pending full QA. Tidak ada live/staging/frontend/deployment/commit/push.

[Inventory yang benar-benar dijalankan dari sandbox](../../../private-backup/qa-owner-inventory-20261111T060510910380Z/inventory.json) selesai dengan exit 1 / **BLOCKED UNKNOWN_BYTES**: dua scratch `pytest-m14cp02-20261030` dan `storage-tests-c3bcf5febae4438b81ee069b1120aef5` memberi PermissionError. Subtotal terukur sekitar **22,560 GiB**, D free **50,421 GiB** pada snapshot; byte yang tidak terbaca tidak dihitung nol atau disertifikasi. Bukan kegagalan pilot/60 tes; ini gate yang sengaja tetap menolak full admission sampai terminal owner memberikan inventory lengkap.

Perintah read-only dari PowerShell biasa:

```powershell
& 'D:\Projects\lms-v2\backend-v3\backend-go\.venv\Scripts\python.exe' 'D:\Projects\lms-v2\backend-v3\portability\review_qa_storage_owner.py'
```

Backup akhir source/context/config/evidence tercatat di [manifest](../../../private-backup/qa-retention-native-hooks-final-20261111T060852665277Z/manifest.json); bukan salinan seluruh mesin atau raw PostgreSQL.

[Kebijakan utama](../QA_STORAGE_POLICY.md) · [Brief berikutnya](2026-11-11-qa-retention-next-steps.md) · [Current resume](../../../portability/RESUME_CONTEXT_2026-11-11.md).
