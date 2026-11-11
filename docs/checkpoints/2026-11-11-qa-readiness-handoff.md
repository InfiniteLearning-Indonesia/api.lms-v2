# M14 readiness — inventory handoff, per-case preflight dan review HOLD

11 November 2026. **68/68 pemeriksaan readiness lokal PASS; QA penuh M14-CP04 belum dimulai.** Scope sesi ini tidak mencakup CP baru, perubahan aplikasi Go, database shared, disposal historis tambahan, ACL takeover, Git atau deployment.

## Scope, backup dan baseline

[Scope/prestate](../../../private-backup/qa-readiness-20261111T075308612661Z/scope-and-prestate.json) dibekukan sebelum perubahan. Baseline service `42af6eb44eced2691b3fd1a63338dad440c00a87ad20050ea436152aa2fe4944`; SHA256 archive source/context sebelumnya diverifikasi kembali terhadap receipt. [Backup sebelumnya](../../../private-backup/qa-retention-native-hooks-final-20261111T060852665277Z/manifest.json) tetap utuh.

Probe baca directory dan `icacls` tanpa mutasi kembali menghasilkan **Access denied / WinError 5** untuk dua scratch:

- `lms-v3-qa-artifacts/pytest-m14cp02-20261030`.
- `lms-v3-qa-artifacts/storage-tests-c3bcf5febae4438b81ee069b1120aef5`.

Tidak ada laporan owner-terminal baru saat awal sesi. Tidak dilakukan pengambilalihan ACL, perpindahan folder, penghapusan, atau pengecualian unknown bytes dari admission. Menjalankan command melalui sandbox tetap memakai token sandbox; launcher baru tidak menyamarkannya sebagai terminal owner.

## Perubahan konkret

| Bagian | Sebelum | Sekarang |
|---|---|---|
| Owner handoff | Operator harus memeriksa inventory lalu menjalankan QA terpisah. | `continue_m14_cp04.py`/`CONTINUE_M14_CP04_QA.cmd` melakukan inventory lebih dulu dan hanya memanggil runner lama bila admission lulus. `RUN_M14_CP04_QA.ps1` mengarah ke jalur yang sama. Tidak ada elevasi atau DSN/env input. |
| Preflight evidence | Runner launcher memeriksa exit code pytest; `QA_CASE_RESULTS` tidak diikat ke destination baru. Hasil per-case bisa tidak tersimpan atau diarahkan environment shell lama. | `qa/preparation.py` mengikat path evidence/artifact baru, memeriksa exit proses **dan** exact collected/executed IDs, duplikasi, status seluruh kasus, dan hash manifest/cases/log. Semua wajib PASS. |
| Frozen scope | Daftar file ops hardcoded dalam launcher, tanpa exact case-set contract khusus. | `qa/manifests/M14-CP04-preparation.json` membekukan daftar **68 kasus** dan file standalone. Tidak memanggil suite ops lain yang memerlukan harness/Git/backend binary. Test runner regression ditambahkan ke preflight yang sebelumnya tidak memasukkannya. |
| Public source/context filter | `.env` dan `.env.*` dikecualikan, namun nama `.env-staging` atau uppercase tidak tercakup helper lama. | `collect_important_context.config` mengklasifikasikan seluruh prefix `.env` secara case-insensitive dan key/certificate sebagai private. Tes archive nyata memakai data palsu memastikan nama tersebut tidak masuk archive source. Ini perbaikan filter, bukan bukti bahwa secret nyata telah bocor. |
| QA backup context | Launcher menyertakan source/portability/START_HERE saja. | Pin registry dan kebijakan aktivasi parent juga masuk source/context backup. Original secrets tetap dikecualikan. |

Source/context backup operator juga menyertakan launcher `.cmd` baru; filter private diterapkan pada file portability. Hash service tidak mencakup tools parent, sehingga hash tools dan archive akhir dicatat terpisah untuk handoff.

`ops-cases.json`, `ops-check.json`, `ops-targeted-regression.log`, dan manifest SHA256 menjadi evidence preflight per run. Receipt menyebut `LOCAL_FILESYSTEM_PREPARATION_ONLY`, bukan full-module acceptance. Exit 0 dengan skip atau expected ID yang hilang terbukti ditolak lewat subprocess pytest sungguhan.

Launcher tetap melakukan storage revalidation, source/context backup, Go unit/vet/tagged compilation, native PostgreSQL journey, SQL/custom backup + isolated restore, Playwright, Go/DB, performance dan final evidence review. Wrapper inventory tidak menggantikan pemeriksaan itu. `--inventory-only` tidak memulai QA; `--prepare-only` tidak menjalankan native DB journey.

## QA yang benar-benar dijalankan

- Source baru: `a6d12dcbf728e103667029c433b132243bcfbdc944e471c9c4701206cf1ace63`.
- [Summary](../../../private-backup/qa-readiness-checks-20261111T075656970098Z/summary.json), [exact ops cases](../../../private-backup/qa-readiness-checks-20261111T075656970098Z/ops-cases.json), [preflight receipt](../../../private-backup/qa-readiness-checks-20261111T075656970098Z/ops-check.json): **68/68 PASS**, source tidak berubah. 60 kasus lama + 8 readiness/handoff; bukan persentase coverage LMS atau penyelesaian CP04.
- Tiga subprocess pytest kecil membuktikan routing evidence terisolasi, penolakan skip walaupun exit 0, serta penolakan ID wajib hilang walaupun exit 0. Kasus lain membuktikan inventory unknown mencegah launch, mode inventory-only tidak launch, exit QA gagal tetap gagal, filter archive private dan frozen-manifest wiring.
- Validator manifest M13/M14 dijalankan read-only dan lulus: M13-CP04 **4** mapped cases; M14-CP01 **5**, CP02 **3**, CP03 **7**, CP04 **6**. Jumlah ini adalah mapping domain validator, bukan 68 kasus ops.
- ID ops baru diregistrasikan pada expected checks M00/M03; full-suite flags dan canonical deliverable coverage tidak dinaikkan. Tidak ada Go build/test, Playwright journey, migration atau native DB startup baru dari pekerjaan readiness ini.

## Review read-only 14 HOLD

[Laporan terperinci](../../../private-backup/qa-readiness-20261111T075308612661Z/hold-review.json) melakukan inspeksi sekarang terhadap 14 HOLD pada snapshot sebelumnya, dengan shared storage/runner locks. Bukan plan disposal baru dan tidak mengubah keputusan retensi.

| Alasan HOLD awal | Jumlah | Tindakan |
|---|---:|---|
| OPERATOR_REVIEW_REQUIRED | 7 | Pertahankan status FAIL_STOP historis. Clean control state sekarang tidak otomatis menyelesaikan stop failure atau memberi disposal authority. |
| MISSING_CASE_FILES | 2 | Backup yang ada tidak menggantikan case evidence yang hilang; jangan membuat kasus PASS retroaktif. |
| MISSING_CASE_EVIDENCE | 1 | Pertahankan raw/evidence dan review log secara terpisah; summary asli tidak ditulis ulang. |
| INSPECTION_FAILED / summary hilang | 2 | Identitas sintetis belum terbukti. Tidak mengevaluasi backup/native state seolah scope sudah aman. |
| UNKNOWN_OR_BLOCKED_RUN_STATUS | 2 | Kegagalan startup tidak membuktikan raw kosong atau aman dibuang. |

Dari 14: **12 identitas sintetis cocok dan native pg_controldata membuktikan shut down sekarang**; dua summary hilang tetap unknown. **4** memiliki backup/restore receipt historis yang hash-nya cocok; **8** tidak terverifikasi; **2** tidak dievaluasi karena identitas belum terbukti. Tidak ada SQL, koneksi DB, start/stop DB, perbaikan evidence lama, perubahan HOLD, atau disposal tambahan. File summary/cases/receipt yang diinspeksi tetap identik.

## Command operator dan exit berikutnya

Dari terminal Windows biasa, satu perintah:

```powershell
& 'D:\Projects\lms-v2\backend-v3\portability\CONTINUE_M14_CP04_QA.cmd'
```

Untuk pemeriksaan saja, tambahkan `--inventory-only`. Command tidak menaikkan hak akses; bila owner terminal juga mendapat Access denied, ia tetap berhenti. Pemeriksaan permission/content perlu dilakukan pemilik sebelum menentukan perubahan apa pun; tidak ada janji bahwa sekadar berganti terminal pasti memperbaiki ACL.

Gate berikutnya adalah inventory lengkap dan admission lulus. Sesudah itu full native M14-CP04 berjalan lewat command yang sama; review hasil/source/cases/performance sebelum mengubah CP status. M13 tetap **4/4 lokal historis**; M14 **3/4 lokal historis**, CP04 pending full QA. Retensi future-only dan cutoff yang telah diaktifkan tidak diubah; 51 kandidat historis tetap di luar auto-scope.

[Command baru benar-benar dijalankan dari sandbox](../../../private-backup/qa-owner-inventory-20261111T075930577649Z/inventory.json): exit 1, `BLOCKED_BEFORE_QA`, `qa_started: false`, alasan UNKNOWN_BYTES pada dua scratch yang sama. Subtotal terukur **22,566 GiB**, D free **50,412 GiB** saat snapshot. Tidak ada native journey yang dimulai. Ini gate permission yang tetap terbuka, bukan 68 tes yang gagal.

[Kebijakan](../QA_STORAGE_POLICY.md) · [Brief lanjutan](2026-11-11-qa-retention-next-steps.md) · [Current resume](../../../portability/RESUME_CONTEXT_2026-11-11.md).
