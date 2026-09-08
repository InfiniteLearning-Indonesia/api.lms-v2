# Checkpoint implementasi — 6 September 2026 WIB

Scope selesai pada checkpoint ini: **M00 local source portion + M03 local Go/QA bootstrap**, bukan seluruh M00/M03 dan bukan M00–M19. Root backend `backend-v3/backend-go`, existing branch `be-v3`; belum commit/push. Frontend dan legacy runtime tidak diubah.

## Hasil eksekusi terakhir

| Paket | Run ID (UTC) | Bootstrap checks | Python cases | Go cases | Full D verification |
|---|---|---:|---:|---:|---:|
| M00 | `20260905T180907Z-M00-e6000fc420cc` | 15/15 PASS | 11 PASS | 9 PASS | 0/5; BLOCKED |
| M03 | `20260905T180800Z-M03-014489b8a6f6` | 18/18 PASS | 14 PASS | 9 PASS | 0/5; BLOCKED |

Bootstrap checks terdiri dari empat aggregate Go checks (format/build/unit/vet) dan Python cases; **jangan menjumlahkan Go cases lagi sebagai checks tambahan**. Sepuluh Python cases regression serta sembilan Go cases dipakai ulang kedua run; dua angka run bukan jumlah unique tests. Go statement coverage **76,6%** pada source saat ini, bukan persentase kemampuan LMS v3. API/CLI child-process tests tidak terhitung sebagai instrumented statement coverage `go test`.

Full-package suite belum implemented sehingga full-package test pass rate N/A. Tidak ada mandatory deliverable diubah/dikecilkan untuk membuat score terlihat hijau. Total seluruh roadmap tetap 0/97 fully verified D; status implementation M00/M03 IN_PROGRESS menunjukkan pekerjaan nyata, bukan NOT_STARTED.

Executable source hash kedua run:

`84588109f2b4384da3a611f9788f7241a1789faacdbd95c35a30fbc81ee93e9a`

Manifest hashes:

- M00: `53c42637ca0ee76213c83964d6f0cc0b75bb7aea40abdf778cb216b455db66fa`.
- M03: `09234ec7451795f367e8fb9cfa1f9fa5bee84edc4bc62048e57592dc00b42516`.

Kedua run mempunyai `publication.json`; generated module/dashboard sections memuat run ID yang sama untuk masing-masing paket. Raw evidence di ignored `.qa-artifacts/<run_id>/`: `summary.json`, Go/pytest case results, JUnit, command logs, binaries dan hashes, coverage, serta preview report. Raw artifact tidak dipush/upload; local file permissions belum merupakan approved backup/PII storage policy. Hash dipakai untuk current-source matching, bukan tanda tangan operator/CI attestation.

## Deliverable parsial nyata

- Go standard-library HTTP bootstrap terpin 1.27.1; explicit local-synthetic numeric loopback binding, timeouts, generated request IDs, controlled logs/errors, graceful shutdown unit test. Readiness sengaja 503; tidak ada business/auth API maupun database connection.
- Full local source declaration inventory dua backend checkout (64 TS files masing-masing), checkout SHA/dirty/hash, dan sanitized dotenv identity fingerprint. Source parser aid bukan schema/authorization auditor otomatis.
- Pytest/Playwright memanggil binary API baru di dynamic loopback port; no existing live/staging service digunakan. Tests unknown/admin mutation routes mengharapkan 404 karena **fitur belum ada**, bukan bukti seluruh authorization LMS selesai.
- QA runner dengan explicit module registry, frozen expected Go/Python case IDs, missing/duplicate/zero/skip/stale/XPASS failures, local OS lock, stripped child environment, unique evidence paths dan automatic generated Markdown sections. Predicate fixture PASS tidak membuka real import gate.
- Workflow definition untuk `be-v3` push/PR, pinned action commits, read-only token, synthetic-only checks serta Linux race step. **Remote execution belum diuji**; green bootstrap job kelak bukan full-module approval.

## Failure history dan keterbatasan yang dipertahankan

Run awal `20260905T164157Z-M03-69462cf1ded1` FAIL: isolated QA environment tidak memperoleh VCS status saat build. Diperbaiki dengan invocation-scoped `safe.directory` hanya untuk checkout yang dipilih, tanpa wildcard/global Git config changes. Raw failed run tetap ada; bukan dihapus atau dimark PASS.

`go test -race -count=1 ./...` lokal dicoba terpisah dan gagal build `runtime/cgo`: `cc1.exe: sorry, unimplemented: 64-bit mode not compiled in`. Installed C compiler bukan toolchain x64 yang kompatibel. Normal Go tests/build lulus, tetapi **race safety belum verified**. Workflow Linux menyiapkan percobaan terpisah; tidak menganggap definisi workflow sebagai hasil run.

Static Markdown link/fence/encoding validation dan YAML parse dilakukan lokal. Ini bukan actual GitHub workflow validation/run, vulnerability scan, DB integration test, non-root release certification, performance benchmark, auth/security audit ulang, atau end-to-end LMS acceptance.

## Gate berikutnya — jangan menebak source database

[Local baseline](../baseline/LOCAL_BASELINE.md) merekam tiga env lokal ber-fingerprint identik dan staging code menambah nullable JSONB `Batch.logbookSchedule`. Nama checkout/file tidak cukup untuk menentukan database production. Deployment mungkin menggunakan secret berbeda dari file lokal ini.

Input operator yang diperlukan (berikan **lokasi file atau secret reference**, bukan password di chat):

1. Deployed backend live/staging SHA atau image/manifest reference.
2. Source production dan target staging yang benar, provider project/database reference, serta koneksi read-only untuk inventory.
3. CA provider untuk TLS peer/hostname verification; hak role sumber/target.
4. Owner/reviewer dan allowed read/export/restore/import scopes; backup destination ACL/encryption dan capacity budget.

Urutan lanjutan: verify dua DB identity secara read-only → catalog/jobs/files/capacity dan schema drift → M01 approved backup + isolated restore proof → additive isolated PostgreSQL foundation M03 → M04/M05 identity/Class sesuai dependency. M02 security legacy tetap separate checkout/change authorization; bootstrap Go tidak mengamankan live legacy dengan sendirinya.

M00/M01 operational blockers tidak harus menghentikan setiap independent local engineering task, tetapi tidak boleh dilewati untuk shared DB work. Linux toolchain/CI proof, non-root local artifact dan isolated synthetic PostgreSQL dapat disiapkan sebagai task terpisah dengan scope yang sesuai; status dependency tetap jujur.

**Operasi data checkpoint ini: NONE.** Tidak ada DB access, backup SQL baru, export/copy/restore/migration, delete/truncate/drop, schema reset, object deletion atau deployment. Tidak ada backup diklaim berhasil. Test fixture source files/Git repositories lokal bukan database dan tidak menyentuh checkout/data pengguna.

## Resume tanpa mengulang dari nol

1. Baca [status](../IMPLEMENTATION_STATUS.md), [M00](../modules/M00.md), [M03](../modules/M03.md) dan README; periksa dirty worktree tanpa restore/reset. User's tracked legacy deletions bukan perubahan agent.
2. Cocokkan current source hash terhadap run di atas. Source/dependency/workflow change → rerun applicable checkpoint, bukan memakai old PASS.
3. Gunakan `python -m qa.run --package M00 --workspace <absolute-root> --update-docs` dan `python -m qa.run --package M03 --update-docs`; default exit 2 berarti remaining package gates, bukan test success palsu.
4. Jika publikasi terputus/dua run IDs tidak cocok, evidence belum current; ulang checkpoint dan biarkan renderer memperbarui owned sections. Jangan menghapus raw run lama.
5. Baru buka tindakan operasional ketika identitas/backup/approval gates untuk tindakan tersebut terpenuhi. QA tidak otomatis push, migrate atau deploy.
