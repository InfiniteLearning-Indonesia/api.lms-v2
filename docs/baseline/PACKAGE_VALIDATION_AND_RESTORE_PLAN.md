# Verifikasi paket baseline dan rencana restore offline

> **17 November — current recovery:** [Seven-case dependency accepted](../checkpoints/2026-11-17-protected-package-accepted.md); [new local recovery implementation](../checkpoints/2026-11-17-local-recovery.md): **81 Go/95 ops PASS**, current native **4/9 PASS + 5 fixture AccessDenied FAIL**, ten DB cases NOT_RUN. [Owner runner](LOCAL_RECOVERY_RUNBOOK.md); [why dotenv is not staging activation](STAGING_CONFIGURATION_READINESS.md). No native recovery/module acceptance, env/DB edits or publication. Earlier source-bound snapshots below are history.

> **17 November — current:** [Protected output/package implementation](../checkpoints/2026-11-17-protected-package.md), source `126c217ad43a732e67d5d3b32473233ed5bd94385596a78872f0ba9b7b63371b`: **79/79 Go + 93/93 ops + vet/Windows build PASS**, Linux cross-build/tagged compile PASS_BUILD_ONLY. Mandatory actual suite **4 PASS + 3 fixture AccessDenied FAIL**; restricted token confirmed. **IMPLEMENTED_PENDING_OWNER_NATIVE_QA**, no CP/module/restore closure. [Contract/runner](PROTECTED_PACKAGE_RECOVERY.md). Earlier source-bound results below remain history; no DB/delete/commit/push.

16 November 2026. Implementasi [baseline-package](../../cmd/baseline-package/main.go), [manifest/inventory](../../internal/migration/backup/package.go), [strict decoder](../../internal/migration/backup/package_decode.go), dan [planner](../../internal/migration/backup/restore_plan.go). Mendukung poin 3/4 dan DLV-04–06 preparation; bukan exporter, executor restore, atau izin production/staging.

## Dua mode yang tersedia

- `baseline-package --verify-only`: manifest JSON dari stdin → local file/header/size/hash dan inventory checks → report. Exit 0 berarti **OFFLINE_PACKAGE_INTEGRITY_VALID_NOT_AUTHORIZED**.
- `baseline-package --plan-restore`: JSON `RestorePlanRequest` dari stdin → package verification yang sama + restore declaration checks → static preparation steps dan pending gates. Exit 0 berarti **OFFLINE_RESTORE_PLAN_VALID_NOT_AUTHORIZED**.
- Mode lain, `--restore`, `--clean`, flags tambahan, input malformed/unknown/duplicate/null/invalid UTF-8/trailing/oversized ditolak dengan exit 2. Tidak ada DSN, raw SQL, command execution, `.env` loading, output-directory creation atau database connection.

Exit 0 **tidak mengotorisasi operasi**. Semua reports memiliki `authorizes_execution=false`, actual restore false dan external gates. Metadata/hash matching tidak menutup actual source identity, role/TLS, approval, key custody, archive authentication atau target isolation.

## Susunan input privat

```text
private-baseline/run-id/
  manifest.json                 <- stdin untuk --verify-only
  restore-plan-request.json     <- stdin untuk --plan-restore
  artifacts/                    <- intake.artifact_root menunjuk ke sini
    database.dump[.age]
    database.sql[.age]
    schema.json
    files.json
    objects.bundle[.age]        <- wajib bila inventory objects tidak kosong
  reports/                      <- simpan output di luar artifacts/
```

Folder `artifacts/` harus berisi **tepat file yang dideklarasikan**; subdirectory, file tambahan, key/secret, missing entry dan link/reparse ditolak. Simpan manifest, reports dan keys di lokasi terpisah. `>` output langsung ke artifacts/ akan menambah file di luar scope dan membuat pemeriksaan gagal. Tool tidak menghapus atau merapikan folder tersebut.

## Kontrak manifest

`PackageRequest` berisi `version=1`, `intake` dari [kontrak preflight](OFFLINE_PREFLIGHT_AND_STREAMING.md), `snapshot` dan `artifacts`. Input manifest/restore request maksimum **256 KiB**, kedalaman JSON dibatasi dan field casing persis. Preflight tetap melakukan actual local capacity/path checks; semua identity/review/custody references adalah declarations.

| Bagian | Syarat |
|---|---|
| Snapshot | Opaque ID, source identity/schema SHA-256 sesuai intake, UTC export timestamp ≤24 jam, consistency reference dan declared format `postgres-custom`. Server major 14–18; dump client major ≥server dan ≤18. Versi lain memerlukan review perubahan kontrak. |
| Artefak | Role dan basename unik, snapshot ID sama, expected size positif dan SHA-256. Role wajib: `db_custom`, `db_sql`, `schema_inventory`, `file_inventory`; `object_bundle` hanya untuk nonempty object scope. Unknown role ditolak. |
| Batas | Setiap artefak dan aggregate maksimal 1 TiB; metadata masing-masing maksimal 2 MiB; bounded manifest artifact list dan metadata entries. Batas bukan klaim bahwa throughput/kapasitas production telah diuji. |
| Kapasitas | Declared dump/SQL/object allocation harus cukup untuk actual artifact bytes; free space/intake reserve tetap dicek. Metadata/overhead harus tercakup sizing/reserve. |
| Representation | `none` atau `age-v1`. Dua artefak DB harus memakai deklarasi representation sama. Metadata harus readable JSON `none`; encrypted metadata belum didukung. Tidak ada decrypt/fallback plaintext otomatis. |
| Header | Plain custom dump harus diawali `PGDMP`; declared age diawali header age-v1. **Header bukan parser archive, authentication atau actual restore proof.** |

Setiap file diverifikasi secara streaming dengan anchored access, actual size/hash dan path/file identity checks yang sudah tersedia. Metadata diread ulang secara bounded dan dihash lagi sebelum parsing. Inputs tidak dikunci selamanya: downstream harus reverify/hold source immutable sebelum operasi berikutnya; report ini tidak menghilangkan race dengan writer lain setelah tool selesai.

## Inventory dan accounting

`SchemaInventory`: version, snapshot/source/schema hashes, exact schema set, tables `(schema,name,rows)`, extensions `(name,version)` dan opaque completeness reference. Negative/overflow row count, duplicate table/extension, foreign schema, inconsistent snapshot/source dan malformed fields ditolak. Discovered tables ikut accounting, tidak difilter ke baseline 20 tabel saja. Completeness reference tetap **bukan pembuktian semua source tables/jobs/constraints sudah tercakup**.

`FileInventory`: version, snapshot/source identity, objects `(opaque ref,bytes,sha256)`, bundle SHA-256 dan no-objects review reference. Nonempty inventory membutuhkan declared bundle dengan hash cocok; empty inventory membutuhkan explicit no-objects review reference dan tidak boleh membawa bundle tersembunyi. Duplicate object refs/invalid sizes/hashes ditolak.

Masing-masing inventory maksimum 4096 entries; data yang melebihi limit berhenti untuk desain batching/manifest scope yang direview, bukan dipotong. Report `declared_tables/rows/objects` menghitung metadata yang diberikan. Hash bundle valid **tidak membuktikan setiap byte object di dalamnya cocok inventory**; extraction/object reconciliation belum dijalankan.

## Planner restore

`RestorePlanRequest` memuat `version=1`, `package` di atas dan `restore` declarations:

- Target identity harus cocok intake target, kind `new-local-restore`, nama DB lowercase prefix `v3_restore_` (≤63 karakter).
- Target PostgreSQL major 18 untuk scope ini; extension name/version yang dibutuhkan inventory harus tersedia persis dalam declared availability, tanpa duplicates.
- Network/jobs disabled, executable dump reviewed dan reviewer reference wajib; `run_as_superuser=true` ditolak. Booleans ini **belum membuktikan** konfigurasi host/DB atau human review sebenarnya.
- Output berisi sembilan static step IDs: prove empty independent target, actual tools/extensions/privileges/space, isolation, executable review, fresh backup/key recovery, input reverify/authentication, restore to new target, reconcile dan retain receipts. **Tidak ada command shell/SQL/pg_restore yang dihasilkan atau dieksekusi.**

Actual extension provisioning, target identity/emptiness, outbound isolation, role/privilege checks, protected pre-state backup dan authenticated archive recovery masih external gates. Shared staging/live tidak dibuka oleh planner ini. Restore untrusted executable dump objects tidak boleh dilakukan dengan asumsi `--no-owner/--no-acl` merupakan sandbox.

## Yang masih belum selesai

Poin 2 tetap **ENCRYPTION_DEPENDENCY_BLOCKED**: retry download age v1.3.2 melalui normal proxy gagal. Tidak ada dependency/source/checksum baru, `go.mod/go.sum` tidak berubah, tidak ada crypto protocol buatan sendiri atau plaintext export fallback.

Poin 3 dan 4 adalah hasil offline yang dapat diterima secara terpisah. DLV-04 full backup/restore, DLV-06 real-baseline readiness, actual source/server/catalog completeness, archive authentication, object extraction, actual PostgreSQL restore, frontend/pilot dan cutover tetap belum verified. [Evidence pelaksanaan](../checkpoints/2026-11-16-package-validator-restore-plan.md).
