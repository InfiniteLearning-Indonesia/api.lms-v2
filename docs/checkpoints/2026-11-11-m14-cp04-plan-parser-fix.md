# M14-CP04 native run gagal — kompatibilitas parser EXPLAIN PostgreSQL 18

11 November 2026. **Perbaikan test-only dan verifikasi targeted selesai; fresh full native run masih wajib. M14 tetap 3/4 lokal historis.** Tidak ada perubahan query aplikasi, schema, budget performance, data shared, Git atau deployment.

## Evidence run yang dilaporkan operator

Run `20261111T084515Z-postgres-lab-dc2f748e9472`, source `a6d12dcbf728e103667029c433b132243bcfbdc944e471c9c4701206cf1ace63`, berakhir FAIL. [Outcome launcher](../../../private-backup/m14-cp04-20261111T084225250541Z/outcome.json). Summary/log/backup asli dipertahankan; kompresi setelah stop dan retensi bukan penyebab error tes ini.

- [Owner inventory](../../../private-backup/qa-owner-inventory-20261111T083807191738Z/inventory.json) **PASS_STORAGE_ADMISSION_ONLY**, complete=true. Dua scratch dapat dibaca dari terminal pemilik; kegagalan baca sandbox sebelumnya tidak berarti host owner masih blocked.
- [Preparation](../../../private-backup/m14-cp04-20261111T084225250541Z/preparation.json) PASS: validator manifest, **68/68 ops**, Go unit, go vet, tagged DB compile dan load build.
- Native journey menjalankan lebih dari 5.300 command. Dependency groups yang tercatat sampai M14-CP01/02/03 dan rekonsiliasinya PASS; worker/consumer regression lanjutan yang dijadwalkan setelah CP04 tetap belum dibuktikan pada run ini.
- Native load dan kedua API performance cases PASS: **1.080 measured requests**, sembilan phase kecil/boundary, tanpa acceptance yang dilonggarkan. Bukan klaim SLA produksi.
- Command **5389** gagal pada `TestSyntheticReportPerformancePlanAndPool`, `performance_integration_test.go:66`, pesan `actual reporting plan exceeded frozen row/time bound`. Command 5390 kemudian melakukan normal stop (`server stopped`); cluster retained dan dikompresi.
- Pool/audit dan final reconciliation setelah titik gagal belum selesai. Status keseluruhan tetap FAIL; partial benchmark PASS tidak menerima CP04.

## Bug kompatibilitas yang ditemukan

Probe lama menggabungkan kegagalan decode JSON, jumlah plan/baris, dan waktu eksekusi menjadi satu pesan. `Actual Rows` didekode menjadi **int**. PostgreSQL 18 mengubah EXPLAIN agar menghasilkan fractional row counts: [release notes resmi](https://www.postgresql.org/docs/18/release-18.html), bagian EXPLAIN.

Reproduksi Go terhadap parser integer yang sama benar-benar gagal untuk `2.00` dan `200.00` dengan `cannot unmarshal number ... into Go struct field ... of type int`; juga menunjukkan missing/null Execution Time diterima sebagai nol oleh model lama. [Before-fix evidence](../../../private-backup/m14-plan-parser-fix-20261111T091136748475Z/before-fix.log).

**Batas diagnosis:** raw EXPLAIN tidak disimpan sebelum fatal pada run lama. Maka nilai row/time native persis pada titik gagal tidak tersedia untuk membedakan setiap cabang pesan generik. Bug parser PG18 terbukti secara terpisah dan konsisten dengan lokasi error; kelulusan probe native setelah fix tetap membutuhkan run baru. Jangan menyimpulkan query lambat hanya dari pesan lama atau mengubah threshold untuk melewatinya.

## Perubahan

- `internal/reporting/performance_plan_test.go`: helper shared test-only membaca row/time numerik float dengan presence check, membandingkan jumlah baris **persis** terhadap 2/200 tanpa truncation, dan mempertahankan waktu **0 sampai 500 ms**.
- Regression mencakup integer lama/desimal PG18, fractional/wrong rows, 500 ms tepat, di atas batas, nilai negatif, missing/null, plan kosong/ganda, tipe salah dan JSON rusak. Semua 14 sample berada dalam satu unit test; bukan 14 acceptance deliverable baru.
- `performance_integration_test.go`: memakai helper yang sama, menulis raw plan `m14cp04-plan-small.json`/`m14cp04-plan-boundary.json` secara exclusive **sebelum** validasi, dan memisahkan error decode/rows/time dengan nama workload serta path evidence.
- Tidak ada perubahan `releasedGradesSQL`, runtime permissions, migration, budget JSON, jumlah workload atau pool limits. Tidak menyentuh/memulai ulang cluster gagal.

## Verifikasi dan langkah berikutnya

Source setelah fix: `cb0a9af604011f90e6bc14844f637e5e0bb094104b509ff03905407feae0a7bc`.

[After-fix receipt](../../../private-backup/m14-plan-parser-fix-20261111T091136748475Z/after-fix.json): reporting unit PASS, tagged integration compilation PASS, source tetap sama. Native DB tidak dimulai pada perbaikan ini. Tes Python 68 dari run operator tetap evidence source sebelumnya; tidak diubah menjadi acceptance full source baru.

Jalankan kembali dari PowerShell Windows biasa:

```powershell
& 'D:\Projects\lms-v2\backend-v3\portability\CONTINUE_M14_CP04_QA.cmd'
```

Runner akan melakukan inventory/preparation/backup baru dan fresh cluster. Jangan restart/reuse cluster gagal atau mengubah summary lama menjadi PASS. Review raw plans, final DB/pool/audit, seluruh dependency regressions dan final evidence sebelum menerima M14-CP04.

Snapshot retensi owner setelah run gagal: **1 DISPOSED, 52 CANDIDATE, 3 KEEP, 14 HOLD**. Kandidat lama yang keluar failed KEEP tetap di luar auto-cutoff; tidak ada disposal tambahan pada diagnosis/fix ini. Run gagal terbaru dilindungi sebagai failed KEEP. Kebijakan aktivasi future-only tidak berubah.
