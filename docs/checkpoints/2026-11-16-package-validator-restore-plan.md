# 16 November — lanjutan backup, validator paket dan persiapan restore

## Scope freeze sebelum perubahan

Instruksi: lanjut poin 2 dan selanjutnya dari urutan offline. Backend/tool operator tetap Go; Python hanya QA. Source awal `85afcf9f20ce0efd36878746a99c4bea1db87dd21f355cc5fb1b787d1d2155ca`; existing worktree dipertahankan. Tidak ada live/staging connection, DB dump/restore/migration, ACL mutation, delete, commit/push.

1. **Poin 2:** periksa kembali ketersediaan dependency enkripsi melalui konfigurasi jaringan normal. Jika tersedia terverifikasi, implementasikan encrypted streaming + authentication/corruption/wrong-key/EOF QA. Jika masih unavailable, catat blocker dan jangan mengubah proxy, membuat format crypto sendiri atau fallback plaintext. Validator byte-integrity tetap boleh berjalan sebagai scope independen, bukan membuka gate protected export.
2. **Poin 3:** CLI Go offline untuk verifikasi paket baseline: strict/versioned/bounded manifest, role artefak wajib/unik, snapshot/source/scope binding, streamed size/hash dan anchored file access. Inventory schema/files harus sesuai manifest, duplicates/unknown scope/missing object bundle ditolak; metadata lengkap yang dideklarasikan bukan bukti production completeness. Tidak menjalankan SQL atau mempercayai hash sebagai approval/authentication/restore proof.
3. **Poin 4:** planner restore offline, bukan executor. Gunakan paket yang byte-integrity-nya valid; periksa declared target/versi/extension compatibility, reviewer reference, isolasi/outbound/jobs declarations dan no-superuser policy. Output hanya langkah/pending gates, tanpa executable restore command, DB connection, directory creation atau authority.

Exit lokal per scope: Go positive/adversarial cases, actual CLI subprocess QA, exact registry dan regression Go/ops terkait, vet/build/format dan Linux cross-compile. Cases existing tidak dikurangi; current-source Playwright/full DB/remote CI tidak boleh diberi PASS dari evidence lama. Backup source harus reopened/checksum verified sebelum edit; fixtures kecil, tanpa retained cluster atau payload besar.

Paket/restore output selalu `authorizes_execution=false`, actual baseline/restore verification false; DLV-06 readiness dan canonical M00–M18 counts tidak berubah. Hasil belum ada saat scope freeze; tambahkan evidence setelah QA.

## Hasil per poin

| Poin | Capaian | Status dan residual gate |
|---|---|---|
| 2 — encrypted large backup | Retry normal dependency acquisition dilakukan; age v1.3.2 belum tersedia/checksum belum diperoleh. `go.mod/go.sum`, legacy Seal 32 MiB dan protection gate tidak diubah. | **DEPENDENCY_BLOCKED**. Tidak ada encrypted streaming writer/recovery/authentication/wrong-key proof; tanpa custom crypto, bypass proxy atau plaintext fallback. |
| 3 — validator paket | CLI Go memeriksa exact folder/manifest, snapshot/source/scope/size/hash, format header dan metadata/inventory accounting. Semua supplied/discovered inventory entries diperiksa; unlisted files ditolak. | **PASS_OFFLINE_PACKAGE_INTEGRITY_SLICE**. Bukan actual source completeness, archive authentication/readability, object extraction atau restore proof. |
| 4 — persiapan restore | Planner offline memakai hasil validasi paket, declaration checks target baru/PG18/extensions/isolation/review/no-superuser, semantic binding dan sembilan static steps. | **PASS_OFFLINE_RESTORE_PLAN_SLICE**. Tidak ada DB connection/command, target creation atau actual restore; actual operational gates belum terbukti. |

[Kontrak input dan penggunaan](../baseline/PACKAGE_VALIDATION_AND_RESTORE_PLAN.md). Mendukung M01-D01/D03/D05 dan persiapan D04; tidak menutup full M01/operational backup gate atau CP canonical baru.

## Implementasi yang nyata

- [cmd/baseline-package](../../cmd/baseline-package/main.go): stdin JSON, hanya `--verify-only`/`--plan-restore`, error redacted, deadline 5 menit. Tidak menerima `--restore`, `--clean`, raw SQL/DSN/credential atau flags tambahan. Env DB URL tidak dimuat.
- [package_decode.go](../../internal/migration/backup/package_decode.go): 256 KiB manifest/request dan 2 MiB metadata bound; duplicate keys (termasuk escaped), exact field casing di nested array/object, null, invalid UTF-8 dan trailing input ditolak. Tidak mengulang raw decoder errors.
- [package.go](../../internal/migration/backup/package.go): empat required roles, optional object bundle; basename/role unik, snapshot/source/schema binding, fresh declarations, compatible declared dump versions, budget size, anchored streaming checksum, metadata reread checksum. Folder harus tepat berisi listed artifacts; tidak membaca isi private unlisted files.
- Catalog: exact schema set, duplicate/foreign table/extension checks, nonnegative overflow-safe declared row accounting, discovered tables ikut dihitung. File inventory: duplicates/sizes/hashes, explicit no-object disposition atau matching bundle required. Counts tetap **declared**, bukan reconciled actual DB counts.
- Header `PGDMP`/age-v1 diperiksa, tetapi header synthetically valid tidak dipromosikan menjadi authenticated/restoreable archive. Bundle hash tidak dipromosikan menjadi verifikasi setiap object di dalamnya.
- [restore_plan.go](../../internal/migration/backup/restore_plan.go): plan hanya untuk declared `new-local-restore`, `v3_restore_` DB name, target identity cocok, PG18, declared extension version tepat, no superuser dan isolation/review declarations. Booleans/references tidak dijadikan proof aktual. Output tidak berisi executable command atau private DB name/path.
- [Actual CLI QA](../../qa/tests/ops/test_baseline_package.py): valid integrity boundary, tampered/malformed/unlisted/flags refusal, restore-plan isolation/extension checks dan byte preservation. `qa.run` build/hash/env wiring dan diagnostic allowlist diperluas; old expected cases/canonical gates tetap utuh.

## QA dan evidence yang diterima

Final source fingerprint **`078543fa1709650101e98c495a0c10deca1fd5a90db2f75767b043e8a89e9840`**.

| Pemeriksaan | Hasil | Scope |
|---|---|---|
| Backup unit | **20/20 exact PASS** | 12 existing + 8 package/planner Go cases baru; termasuk corruption, mixed/missing snapshot/artifact, invalid inventory, deadline, traversal, no-authority dan UTF-8 cases. |
| Default Go regression | **71/71 exact PASS** | Semua registry cases, tidak ada missing/extra/skip. |
| Python operator regression | **90/90 exact PASS** | Existing 87 + 3 actual package CLI cases. CLI cases adalah subset 90, bukan denominator tambahan. |
| Vet / format / Windows build | **PASS** | Seluruh Go packages, source unchanged selama run. |
| Linux amd64 cross-build | **PASS** | Build-only; Linux runtime/race tidak dijalankan. |
| Statement coverage backup unit package | **89.0%** | Dari targeted profile, bukan whole backend, CLI coverage, D verification atau readiness. Sebelumnya 92.0% pada source lebih kecil; angka itu histori, tidak dipakai pada source final. |
| Playwright/full DB journey/remote CI | **NOT_RUN_CURRENT_SOURCE** | Prior Windows named-pipe blocker tidak dibypass atau dianggap PASS. |
| Encryption/authenticated recovery/actual restore | **BLOCKED / NOT_RUN** | Tidak ada dependency/module baru dan tidak ada DB operation. |

Pertama scope A sudah lulus pada source `57939b20151f34c027ecab7da18054940e65c8817251f1f1561158ee9f93ab22`. Review kemudian menambahkan explicit invalid-UTF-8 refusal dan test pada case yang sama; scope B dan regression-B diulang pada source final di atas. Tidak mengubah hasil lama menjadi current; seluruh bukti tetap source-bound.

## Backup dan penggunaan storage

Private root: `backend-v3/private-backup/package-deliverables-20261116/`.

- Source backup `source-before.zip`: **1,123,926 bytes**, SHA-256 `0636ac8698b37b8baaa22f6cc3281eb4e97061f06a6bc3ae29307c5e70fd3a1a`; reopen verification PASS sebelum edit, env/keys/cache/Git metadata/raw DB tidak ikut.
- Accepted targeted/ops evidence `qa-B/summary.json`, `qa-B/cases.json`, `qa-B/backup.coverage` dan command logs.
- Accepted full Go/build evidence `regression-B/summary.json`; older `qa-A/` dan `regression/` tetap history pada hash sebelumnya.
- `dependency-inspect.json`: normal download gagal pada proxy `127.0.0.1:9`. Tidak mencoba jalur tanpa proxy, install executable, atau perubahan permission untuk melewatinya.

Semua fixtures berupa file kecil/generated bytes; tidak ada cluster baru, copied production dataset, retained large payload atau cleanup backlog. Tidak mengubah kebijakan retensi. Canonical M00–M18 tetap **0/97 D fully verified**; existing local CP history tidak dihapus atau dipromosikan menjadi real acceptance. Tidak ada commit/push/deploy.

## Langkah selanjutnya

1. Lengkapi poin 2 saat dependency/codec terverifikasi dapat tersedia melalui setup yang diizinkan: encrypted writer, protected exclusive destination, key recovery, authentication hingga EOF dan wrong-key/corruption/interruption tests. Tanpa ini DLV-04 full masih terbuka.
2. Actual target/role/extension/isolation/backup proof dan restore executor masih pekerjaan lain; jangan menjalankan langkah planner ke live/staging atau menghapus synthetic migration fences agar menerima target shared.
3. Sebelum actual baseline rehearsal, lengkapi M00/M01/operator/mapping inputs: independent source/deployed identity, metadata/capacity/object completeness, protected consistent snapshot dan review receipts. Tidak perlu credential live dibagikan bila operator menyerahkan snapshot yang dipulihkan lokal.
4. DLV-06 belum READY dan belum saatnya mengalihkan backend Go ke DB live. Poin 3/4 selesai lokal tidak membuat M15 menjadi real importer seluruh domain.
