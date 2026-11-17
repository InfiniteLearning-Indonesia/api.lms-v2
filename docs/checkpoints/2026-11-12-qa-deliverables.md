# 12 November 2026 — QA deliverables dan baseline preparation

Scope yang diotorisasi: pekerjaan lokal aman tanpa intervensi manusia, berdasarkan [roadmap deliverable](../DELIVERABLE_ROADMAP_TO_REAL_BASELINE.md). Backend Go; tidak ada koneksi database live/staging, migration shared, deploy, commit atau push.

## Exit yang dibekukan sebelum perubahan

1. **DLV-01 / M03-D01,D05 partial evidence:** registry exact Go sesuai seluruh tes default; helper publik operator berada dalam repo dan parent launcher kompatibel; fresh isolated checkout menjalankan bootstrap regression tanpa parent portability/private configuration. Missing, extra, duplicate, skip dan fail tetap ditolak.
2. **DLV-02 / M03-D01,D02,D05 partial evidence:** runner mengeluarkan diagnostik terstruktur yang disanitasi, workflow menyimpan hanya diagnostik kecil dengan retensi terbatas, dependency/toolchain PostgreSQL eksplisit. Local unit/ops/API/build/vet dan fresh checkout diperiksa. Remote execution tetap PENDING_REMOTE sampai publikasi yang diminta kemudian; Linux race/DB proof tidak boleh diklaim dari Windows.
3. **DLV-03–06 preparation:** review gap dari tooling yang sudah tersedia dan siapkan kontrak input/handoff offline yang dapat dikerjakan tanpa input nyata. Jangan membuat adapter production berdasarkan tebakan atau menandai canonical gates lulus. Pemenuhan masing-masing paket dilaporkan terpisah.

## Safety dan evidence

- Backup source/context yang dipilih serta parent helper sebelum edit, reopen byte verification dan checksum; secrets, database artifacts, caches dan Git metadata tidak ikut.
- Targeted QA dan fresh source snapshot saja; hindari cluster DB baru dan artefak masif bila tidak dibutuhkan. Retensi/otorisasi lokal existing tidak diaktifkan atau diubah.
- Source fingerprint sebelum edit `cb0a9af604011f90e6bc14844f637e5e0bb094104b509ff03905407feae0a7bc`; accepted M14 sebelumnya tetap berlaku hanya pada fingerprint lamanya.
- Hasil belum ada saat scope freeze. Diagnosis sebelumnya bukan PASS implementasi; laporan akhir akan ditambahkan setelah pemeriksaan.

## Hasil implementasi dan review

Source yang diuji: **`36dd36aa1f46b237c62b9216b1241c62895c921df0f30dbf01a3d63576da246a`**. Tidak ada perubahan `.go`, schema SQL atau runtime bisnis. Kode yang berubah adalah QA Python, registry dan workflow; backend tetap Go.

| Paket | Hasil dan batas completion |
|---|---|
| DLV-01 | Registry Go 41 → 55; enam helper publik dipindah ke `qa/operators/`; enam parent entry point tetap kompatibel. Fresh isolated source + independent local Git metadata berhasil tanpa parent portability, env atau private policy. Go dan ops regression PASS, tetapi **full exit belum ditutup** karena tiga tes API Playwright gagal setup pada sandbox Windows. |
| DLV-02 | Diagnostic allowlist ≤64 KiB, dua negative/exact-evidence tests PASS; workflow upload hanya diagnostic JSON dengan retention 3 hari, action SHA pinned. PostgreSQL 18.6 eksplisit; job kedua kini mengunduh Go dependencies sendiri sebelum memakai GOPROXY=off. Static Linux repeat-build PASS. **Remote CI, Linux race, container runtime dan fresh PG job belum dijalankan pada perubahan ini.** |
| DLV-03–06 | [Kontrak intake dan gap ledger](../baseline/REAL_BASELINE_INTAKE.md) disusun dari implementasi. Reusable controls, synthetic-only guards dan deferred-domain mapping dicatat. Ini persiapan parsial; belum operational exporter/source adapter lengkap atau `READY_FOR_REAL_BASELINE_INPUT`. |

Perubahan konkret:

- [qa/operators](../../qa/operators/) menjadi rumah implementasi `continue_m14_cp04`, `review_qa_storage_owner`, `manage_qa_storage`, `run_m14_cp04`, `review_m14_contract_run`, serta utility `context_archive`. Parent `collect_important_context.py` tetap memiliki entry point backup luas historis; utility yang dipakai QA berasal dari repo. Tidak ada parent main operasional dipanggil saat uji kompatibilitas.
- Parent `portability/*.py` menjadi compatibility launcher; file-file itu tetap berada di luar repo Go. Policies, native proof, pins, keys dan raw backups tidak ikut dipindahkan ke public package. Policy tidak ada tetap berarti automation disabled, bukan membuat policy default aktif.
- Public M14 launcher/reviewer memanggil module versioned, tidak script parent yang hilang. Source backup menerima checkout tanpa optional parent files; reviewer menulis receipt privat di luar source, bukan JSON baru di folder kode.
- [qa/ci_diagnostics.py](../../qa/ci_diagnostics.py) hanya memproyeksikan known case IDs, hash, status dan fixed command names/exit codes. Raw output, args, environment, credential, unknown IDs dan DB artifacts tidak ikut. Missing/extra/duplicate/skip/stale tetap gagal; diagnostic PASS pun hanya bootstrap, tidak memberi release/migration authority.
- Empat consumer fixture menggunakan `qa_scratch` dari [ops conftest](../../qa/tests/ops/conftest.py): direktori unik retained di artifact root, inherited permissions, tanpa ACL rewrite/cleanup. Ini hanya synthetic scratch, **bukan** protected destination untuk data nyata. Assertion test tetap; environment gate tidak disembunyikan.

## QA yang benar-benar dijalankan

Fresh-source run **`20261112T030752Z-M03-2784ab085b27`**:

- Go default unit **55/55 PASS**, exact registry cocok; format, lima binary builds, vet dan coverage command exit 0.
- Python **85/88 PASS**, **3 ERROR**, 0 skipped. Rinciannya 84 ops + 1 API process/config assertion PASS; tiga Playwright API cases gagal saat memulai driver.
- Aggregate bootstrap **89/92 PASS, 3 ERROR → FAIL/BLOCKED**, bukan full M03 PASS. Missing/unexpected/duplicate check IDs semuanya nol.
- Error: Python asyncio Windows named pipe `CreateFile` → **`PermissionError: [WinError 5] Access is denied`** sebelum driver Playwright berjalan. Go loopback API berhasil start. Tidak ada bypass, substitusi HTTP client, perubahan expected cases atau relabel ERROR menjadi PASS.
- Dua diagnostic tests termasuk redaction marker, extra Go/test IDs, missing/duplicate/skip/stale evidence dan exclusive report creation PASS.
- Enam parent compatibility imports mengarah ke function canonical dan tidak menjalankan `main()`.

Independent static artifact run **`20261112T031146Z-release-594719a2da8c`**, source sama: **PASS_BUILD_ONLY**, static Linux amd64 API/worker, dua build identik. Binary hashes:

```text
api     3b4b58c3750346d1c015be0d5516f7b7530f3be6590d17e37852f207e689594b
worker  7c6206955c35daf6fd53484345400d7caf7235f3bfe3829227174cfb76e5e3fb
```

Ini cross-build dari Windows; tidak membuktikan Linux execution/race/non-root container. Tidak ada DB cluster dibuat dan tidak ada database diakses.

## Kegagalan sebelumnya tetap dipertahankan

- `fresh-A`: snapshot tanpa Git metadata membuat bootstrap Git/VCS checks gagal; beberapa default pytest private temp fixtures juga gagal izin. Ini bukan bukti defect live atau PostgreSQL. Tidak dipromosikan menjadi PASS.
- `fresh-B/C`: local Git clone child shell gagal membuat signal pipe pada sandbox. Tidak ada perubahan permission atau remote access. Untuk pemeriksaan lokal berikutnya digunakan independent metadata snapshot dari worktree/common Git yang dibaca saja, tanpa config remote/hooks; source repository index/refs/config tidak diubah.
- `fresh-D`: guard private verification script menolak asumsi `.git` adalah direktori. Service asli merupakan worktree dengan `.git` pointer ke `backend/.git`. Script kemudian menangani worktree/common metadata secara eksplisit. Kegagalan tersebut bukan perubahan Git root.
- `fresh-E`: metadata/fixture issues teratasi, sisa blocker tiga Playwright named-pipe errors di atas. Ini hasil terbaru, bukan seluruh rangkaian green.

## Backup, ukuran dan evidence

Private evidence: `backend-v3/private-backup/qa-deliverables-20261112/`.

- `source-before.zip`: 1,095,259 bytes; SHA-256 `0f87e03ba13ac38b0c6deea46f608449de049ce0bf450304342cc0e132f60036`; reopened byte verification PASS. Ini source backup, bukan DB backup.
- `fresh-E/verification.json`, bootstrap `summary.json`, `go-cases.json`, `pytest-cases.json`, `ci-diagnostic.json`; `local-results.json` dan `linux-artifact.log` memuat bukti local checks.
- Snapshot/cache/build files masih merupakan artefak privat investigasi. Retry E memakai hardlinks untuk public dependency cache dan immutable Git objects agar tidak menyalin ratusan MiB lagi; source/index/config tidak di-hardlink. Tidak ada policy pembersihan baru atau penghapusan backlog.
- Raw test logs/metadata dan history tidak dipublish otomatis. Diagnostic CI baru dibatasi ukuran dan retensi; tidak menjanjikan seluruh artifact lama otomatis hilang.

## Residual gates dan langkah aman berikut

1. Tutup tiga Playwright cases pada executor yang mengizinkan named pipes/driver. Sandbox saat ini tidak mengizinkannya; tidak meminta elevasi, mengubah ACL atau mematikan gate. Sampai ada bukti tersebut DLV-01 tetap belum closed.
2. Review perubahan/publikasi jika diminta kemudian; baru verifikasi actual Linux CI/race/PG18.6. Definisi workflow dan cross-build tidak menggantikan remote result.
3. Lanjut implementasi offline operational backup/restore tooling DLV-03/04 dengan scope/budget/streaming protection dan negative tests yang dibekukan terlebih dahulu. M01 predicates/Seal sekarang belum exporter production lengkap; jangan hanya menaikkan batas 32 MiB.
4. Gunakan ledger DLV-05 untuk menyiapkan adapter tanpa menghapus synthetic fences. Baseline metadata/owner diperlukan untuk keputusan nyata; jangan mengklaim seluruh domain berhasil dimigrasikan dari MAP_CLASS/shadow-only fixture.
5. DLV-06 belum memberikan readiness. Tidak perlu mengakses live untuk memperbaiki QA, dan tidak ada izin source copy/staging overwrite/pilot/cutover dari pekerjaan ini.

Canonical M00–M18 tetap **0/97** fully verified D; M14 accepted **4/4 local CP** pada source 11 November tetap histori yang sah. Tidak ada CP baru atau full-module completion ditambahkan. Source repo tidak di-commit/push; live/staging tidak disentuh.
