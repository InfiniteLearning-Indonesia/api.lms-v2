# Retensi QA — apply satu kandidat dan fixture disposable

11 November 2026 (Asia/Jakarta). **Langkah kedua selesai untuk engine/operator CLI dan verifikasi filesystem fixture. Bukan penerimaan PostgreSQL, M14-CP04, atau cleanup historis.**

## Scope yang dibekukan

- Implementasikan preview default dan apply eksplisit untuk **maksimum satu** subfolder `postgres/` per pemanggilan, di dua root QA tetap milik project.
- Uji unlink nyata hanya pada fixture disposable buatan tes. Tidak menyalakan PostgreSQL, mengakses live/staging, menjalankan SQL, menyentuh Git, atau mengubah business code Go.
- Jangan mengeksekusi 52 kandidat historis. Jangan memasang trigger otomatis sebelum pilot/review dan scope disposal historis jelas.
- Backup sebelum perubahan: `private-backup/qa-retention-selector-final-20261110T222601305268Z/source-context-and-dry-run.zip`; SHA256 `46427d5048825d390450d6200568dca6865807faa1bfc0bf80b4075ec8372822`. Archive dibuka ulang dan semua hash anggota diverifikasi pada langkah sebelumnya. Backup akhir pekerjaan dirujuk melalui [resume context](../../../portability/RESUME_CONTEXT_2026-11-11.md).

## Implementasi

| Komponen | Perilaku |
|---|---|
| `qa/retention.py` | Plan schema v2 menambahkan identitas direktori fisik dan hash control/version. Selector tetap read-only; mengenali DISPOSED hanya melalui marker/receipt terpisah yang cocok. |
| `qa/retention_apply.py` | Mengambil shared storage/runner locks di kedua root; memuat ulang pin dan global 2 PASS + 1 gagal; mengulang pemeriksaan shutdown, path, summary, cases dan backup. Plan harus cocok dengan hash, source, registry pin, policy, roots, dan berusia maksimum 24 jam. |
| `portability/apply_qa_retention.py` | Preview default. Apply memerlukan `--apply` dan acknowledgement persis `delete-verified-synthetic-qa-cluster-only`. Target berdasarkan run ID unik, bukan input path penghapusan bebas. Membuat source/context backup ZIP terverifikasi sebelum apply. Belum dipanggil runner secara otomatis. |
| Journal/intent | Journal append + fsync sebelum tindakan dan `retention-intent.json` exclusive sebelum raw payload dihapus. Shared lock mencegah benturan dengan runner yang mengikuti kontrak. |
| Receipt/marker | Setelah raw directory hilang dan hash evidence/backup tetap cocok, tulis receipt baru dan `retention-disposal.json`. Summary/status QA asli tidak ditulis ulang. Simpan SQL/custom backup dan restore receipts; backup sebelum migrasi **bukan** snapshot keadaan akhir. |
| Interupsi | Intent tanpa completion sah menjadi HOLD `INCOMPLETE_DISPOSAL_REQUIRES_REVIEW`, termasuk bila raw folder sudah hilang. Tidak ada blind resume atau janji rollback setelah sebagian file dihapus. Retry sukses menjadi no-op terverifikasi; cluster yang muncul lagi dilindungi. |

Plan schema v1 lama dipertahankan sebagai histori dan **ditolak engine apply**. Perubahan source/pin atau plan lewat 24 jam memerlukan plan baru; laporan lama tidak menjadi izin disposal.

## Evidence

- Source executable yang diuji: `9c6f75f68a722787aeb12d72bea62ab3b195edfdd939323180f5f81e1ed83a06`.
- [Summary final](../../../private-backup/qa-retention-apply-checks-20261110T223952395469Z/summary.json), [43 kasus](../../../private-backup/qa-retention-apply-checks-20261110T223952395469Z/cases.json), [log](../../../private-backup/qa-retention-apply-checks-20261110T223952395469Z/checks.log): **43/43 PASS**, source tidak berubah selama tes. Ini 25 kasus apply + 6 selector + 12 regresi storage/reporting/worker/runner, bukan persentase coverage seluruh LMS.
- Pengujian memakai filesystem lokal Windows dan OS locks nyata; hanya output `pg_controldata` dimock. Fixture berisi control/page sintetis kecil, **bukan PostgreSQL cluster sungguhan**. Penghapusan benar-benar dijalankan hanya pada fixture ini.
- `Q00-retention-apply-preservation` mengukur alokasi payload sebelum/sesudah, memastikan postgres hilang, backup/evidence semua run tetap identik, KEEP utuh, selector mengenali DISPOSED, dan retry no-op. Receipt menyimpan alokasi payload yang terhapus serta delta free-space volume; delta volume dapat dipengaruhi write lain/journal dan bukan janji ruang netto.
- Kasus negatif: acknowledgement hilang, source/pin/hash/contract/path/time berubah, control berubah, cases/backup rusak, pin baru/terlambat, PID aktif, scope non-sintetis, quota berubah, lock dipegang, reparse, job keluar root, direktori diganti, marker/receipt berubah.
- Tiga titik interupsi: sebelum unlink, sesudah unlink sebagian, dan sesudah raw payload hilang sebelum receipt. Semua menahan retry untuk review; tidak mengubah PASS/FAIL QA historis.
- [Receipt/journal/backup fixture yang disalin](../../../private-backup/qa-retention-apply-checks-20261110T223952395469Z/fixture-proof-and-history-review.json): payload terukur **14.019 byte** menjadi nol; delta free volume teramati **12.288 byte**, dengan batas pengukuran di atas. Salinan ini tidak mengubah path dalam receipt asli dan bukan receipt baru yang menggantikan run asal. Perbandingan dry-run memeriksa keputusan dan hash inspeksi yang tersedia; untuk HOLD tanpa evidence lengkap, tidak mengklaim verifikasi hash yang tidak tersedia.
- ID kasus terdaftar dalam expected checks M00/M03; full-suite flags tetap false. Preflight M14-CP04 memasukkan suite ini. CLI `--help` juga dijalankan; **tidak ada pemanggilan CLI dengan `--apply` pada data historis**.

## Batas dan kelanjutan

1. Windows Python `shutil.rmtree.avoids_symlink_attacks` bernilai false. Revalidasi path/reparse/identity dan OS locks melindungi alur QA yang kooperatif; **bukan** jaminan terhadap proses bermusuhan dengan hak user yang sama yang sengaja menukar filesystem di sela operasi. Jangan menjalankan disposal saat ada tooling lain yang mengedit raw cluster.
2. Belum ada pilot disposal pada cluster PostgreSQL sintetis sungguhan. Belum ada integrasi before-run/after-stop otomatis. V2 kompresi/admission tetap operasional; riwayat masih dapat bertambah sampai integrasi selesai.
3. [Dry-run schema v2 baru](../../../private-backup/qa-retention-plan-20261110T223952470466Z/summary.json), SHA256 plan `93925a0ff37285b4a618442780a29c4531660a9dc6e8fc93e0e1494a20e21090`: source sama dengan 43 kasus; 3 KEEP, 52 CANDIDATE, 14 HOLD, 2 scratch, nol pin unresolved. Seluruh 69 raw cluster historis tetap ada. Sekitar 9,05 GiB adalah estimasi, **bukan** penghematan yang telah dilakukan. HOLD/scratch dan 2+1 KEEP dilindungi; inventory/admission penuh tetap memerlukan penyelesaian pengecualian.
4. Berikutnya: audit pengecualian/inventory lewat terminal owner, otorisasi scope pilot sintetis historis, plan baru, satu pilot dengan bukti native shutdown/backup/receipt/preservasi. Sesudah review, integrasikan retensi serial sesudah stop/sebelum admission; jangan mengambil ulang lock yang masih dipegang runner. Baru kemudian lanjut full QA M14-CP04.

[Kebijakan utama](../QA_STORAGE_POLICY.md) · [Brief lanjutan](2026-11-11-qa-retention-next-steps.md) · [Evidence selector sebelumnya](2026-11-11-qa-retention-selector.md).
