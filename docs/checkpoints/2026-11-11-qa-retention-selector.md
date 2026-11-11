# Retensi QA v3 — langkah pertama selesai

11 November 2026. **Selector dan dry-run implemented and locally verified. Apply/pembuangan belum diimplementasikan atau dijalankan.** Tidak ada kompresi baru, startup/koneksi database, migrasi shared, commit/push, atau CP bisnis pada tahap ini.

## Evidence final

- Source planner/test: `821bcb62248c2907a92af5b86f91f45d28b0f7d27a18d9d49fc9a7878ce15902`.
- [Rencana yang dapat dibaca](../../../private-backup/qa-retention-plan-20261110T222051692152Z/PLAN.md) dan [path/keputusan/hash lengkap](../../../private-backup/qa-retention-plan-20261110T222051692152Z/plan.json).
- [Summary dry-run](../../../private-backup/qa-retention-plan-20261110T222051692152Z/summary.json): `DRY_RUN_ONLY / PLAN_WITH_PROTECTED_EXCEPTIONS`; SHA-256 plan `26e611e99f552d25b1c80bda998ef500b07eca3e4e21c1dddb20d52148828036`.
- [Tes lokal](../../../private-backup/qa-retention-selector-checks-20261110T222220499966Z/summary.json): **18/18 PASS** pada source yang sama, 6 kasus selector/registry dan 12 regresi storage/reporting/worker/runner. Ini bukan QA Go/API/DB penuh M14 atau CI remote.
- Rencana pertama `qa-retention-plan-20261110T221546578872Z` tetap menjadi histori pada source sebelumnya. Setelah registry diselaraskan, dry-run diulang untuk source final. Recovery daemon tidak menyebabkan operasi penghapusan/kompresi.

## Hasil histori saat ini

| Keputusan | Jumlah | Arti |
|---|---:|---|
| KEEP | 3 | Dua PASS terbaru dan satu gagal terbaru yang eligible, secara global |
| CANDIDATE | 52 | Di luar jatah; memenuhi gate rencana, belum diotorisasi untuk dibuang |
| HOLD | 14 | Dilindungi karena bukti/status/operator perlu review |
| Scratch di luar run | 2 | Tidak terbaca dari sandbox; ukuran tidak diketahui, bukan nol |

Estimasi alokasi **subfolder `postgres/` kandidat saja: 9.717.733.404 byte (sekitar 9,05 GiB)**. Backup, laporan, source dan executable tidak dihitung. Ini bukan ruang yang sudah dibebaskan; shared extents/metadata filesystem dapat memengaruhi hasil apply aktual.

Tiga KEEP:

- PASS `20261031T115506Z-postgres-lab-53f7b900538f`.
- PASS `20261031T111156Z-postgres-lab-d5ed91220fae`.
- FAIL `20261031T090236Z-postgres-lab-a4653b8fe7a1`.

HOLD: 7 `OPERATOR_REVIEW_REQUIRED`, 2 `MISSING_CASE_FILES`, 2 `INSPECTION_FAILED` (summary tidak tersedia), 1 `MISSING_CASE_EVIDENCE`, 2 `UNKNOWN_OR_BLOCKED_RUN_STATUS`. Gate ini lebih ketat daripada gate kompresi, tanpa mengubah status QA lama. Registry pin awal kosong dan tidak ada pin yang belum terselesaikan.

## Implementasi dan batas

- `qa/retention.py`: selector 2+1 lintas root `workspace`/`legacy`; memakai waktu UTC run/ID, bukan mtime. Source historis tidak dipaksa cocok dengan source saat ini.
- Gate: identitas sintetis, root/ancestor reparse, clean shutdown/PG18/no PID, status yang dikenal, hash source, kasus, SQL/custom backup dan hash/receipt actual restore. Operator failure, pin, ID duplikat, bukti invalid dan ukuran yang tak terbaca tidak menjadi kandidat.
- Global pin: `backend-v3/qa-retention-pins.json`, format `{"version":1,"pins":[{"root":"workspace","run_id":"<ID valid>","reason":"Investigasi ..."}]}`; label lain `legacy`. Local pin opsional `<run>/retention-pin.json`: `{"version":1,"reason":"Investigasi ..."}`. Pin di luar jatah normal, sehingga total cluster bisa melebihi tiga. Pin invalid atau target hilang tetap terlihat, tidak diam-diam diabaikan.
- `portability/plan_qa_retention.py`: root/pin registry tetap dari project, shared locks, pengecekan source/pin hash stabil, laporan baru eksklusif. **Tidak menyediakan `--apply`.** Tes no-mutation membandingkan hash fixture sebelum/sesudah.
- M00/M03 `expected_checks` diselaraskan dengan 16 ID ops yang terkumpul tetapi belum terdaftar (termasuk selector baru). Deliverables, pending gates dan `full_suite_implemented=false` tidak diubah. Selector ikut preflight `run_m14_cp04.py`. Ini registrasi QA, bukan kelulusan modul.
- Penambahan nyata hanya kode QA, registry, konfigurasi pin kosong, laporan/fixture lokal dan dokumentasi. Tidak ada ruang yang diklaim sudah dibebaskan oleh dry-run.

## Jalankan ulang

```powershell
& 'D:\Projects\lms-v2\backend-v3\backend-go\.venv\Scripts\python.exe' 'D:\Projects\lms-v2\backend-v3\portability\plan_qa_retention.py'
```

Rencana adalah snapshot review, bukan daftar perintah penghapusan. Apply mendatang wajib memverifikasi ulang kandidat. Jangan menyalin path kandidat ke perintah penghapusan manual.

## Brief keberlanjutan

1. **Langkah kedua:** implementasikan apply terpisah pada fixture disposable setelah scope disposal sintetis diotorisasi. Gunakan backup sumber/konteks, shared lock, pemeriksaan ulang path/pin/identity/shutdown/backup, journal sebelum tindakan dan receipt sesudahnya. Uji interupsi, stale-plan, idempotensi, path escape, dan bahwa evidence/backup/non-sintetis utuh. Tinjau HOLD/scratch dari terminal pemilik, tanpa menurunkan gate.
2. **Integrasi otomatis:** setelah apply aman dan scope diotorisasi, evaluasi setelah full QA berhenti dan sebelum run berikutnya. Batas admission 32 GiB/reserve 2 GiB/lantai 8 GiB tetap berlaku; pengecualian dapat menghentikan admission. Suite dan pembuktian restore tetap wajib.
3. **M14-CP04:** jalankan full performance journey pada source final, review kasus/hash/DB/backup/coverage/shutdown, lalu tutup M14 4/4 lokal jika exit criteria terpenuhi. Saat ini M14 tetap 3/4 historis dan CP04 pending full QA.
