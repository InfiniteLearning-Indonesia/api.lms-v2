# Offline baseline preflight dan integritas arsip besar

> **17 November — current recovery:** [Seven-case dependency accepted](../checkpoints/2026-11-17-protected-package-accepted.md); [new local recovery implementation](../checkpoints/2026-11-17-local-recovery.md): **81 Go/95 ops PASS**, current native **4/9 PASS + 5 fixture AccessDenied FAIL**, ten DB cases NOT_RUN. [Owner runner](LOCAL_RECOVERY_RUNBOOK.md); [why dotenv is not staging activation](STAGING_CONFIGURATION_READINESS.md). No native recovery/module acceptance, env/DB edits or publication. Earlier source-bound snapshots below are history.

> **17 November — current:** [Protected output/package implementation](../checkpoints/2026-11-17-protected-package.md), source `126c217ad43a732e67d5d3b32473233ed5bd94385596a78872f0ba9b7b63371b`: **79/79 Go + 93/93 ops + vet/Windows build PASS**, Linux cross-build/tagged compile PASS_BUILD_ONLY. Mandatory actual suite **4 PASS + 3 fixture AccessDenied FAIL**; restricted token confirmed. **IMPLEMENTED_PENDING_OWNER_NATIVE_QA**, no CP/module/restore closure. [Contract/runner](PROTECTED_PACKAGE_RECOVERY.md). Earlier source-bound results below remain history; no DB/delete/commit/push.

> **Accepted latest, 16 November:** [Actual age streaming QA reviewed](../checkpoints/2026-11-16-age-stream-accepted.md): 4/4 real-tool crypto cases + 75/75 Go/vet PASS, same-source 92/92 ops reused. Earlier crypto NOT_RUN/dependency blocker below is historical. No protected real-data exporter, actual DB restore or full M01 closure inferred.

> **16 November — latest:** [Age streaming adapter/runner](AGE_STREAM_RECOVERY.md) now implemented using pinned native age CLI v1.3.2; actual crypto execution still NOT_RUN without verified tools. [Scope/evidence](../checkpoints/2026-11-16-age-stream-handoff.md). No protected real-data exporter, DB restore or full M01 closure. The dependency/implementation status below records prior source history.

> **16 November:** [Validator paket + restore planner](PACKAGE_VALIDATION_AND_RESTORE_PLAN.md) sudah tersedia dan [lulus scope offline](../checkpoints/2026-11-16-package-validator-restore-plan.md). Encrypted streaming tetap dependency-blocked; tidak ada actual restore atau source/live/staging access. Hasil 12 November di dokumen ini tetap source-bound history.

Implementasi Go: [baseline-preflight](../../cmd/baseline-preflight/main.go), [intake](../../internal/migration/backup/intake.go), [stream digest](../../internal/migration/backup/stream_digest.go). Scope DLV-03/04; tidak memberikan izin koneksi, export, restore atau migrasi. [Scope dan evidence](../checkpoints/2026-11-12-baseline-preflight-streaming.md).

## Kontrak CLI

`baseline-preflight --plan-only` membaca satu JSON UTF-8 dari stdin. Tidak memuat `.env`, tidak menerima DSN/credential, tidak menjalankan command eksternal dan tidak membuat directory target. Exit 0 berarti **local preflight valid saja**; operational authority dan real-baseline verification tetap false. Exit 2 berarti input/scope/path/kapasitas ditolak. Jangan menggerakkan migrator berdasarkan exit 0 tanpa memeriksa gate operasional yang terpisah.

Input dibatasi 64 KiB, versi 1; duplicate keys (termasuk escaped keys), unknown/case-variant fields, null dan trailing JSON ditolak. Report error tidak mengulang nilai input atau error OS. Format identitas adalah declaration hash yang harus terikat receipt independen pada fase berikutnya; hash berbeda bukan otomatis bukti dua DB benar-benar berbeda.

Contoh berikut sengaja **tidak siap dijalankan**: timestamp stale, path placeholder dan references adalah fixture. Jangan mengubah booleans agar memperoleh PASS tanpa bukti nyata; output tetap tidak mengotorisasi operasi.

```json
{
  "version": 1,
  "run_id": "replace-with-reviewed-run-id",
  "source": {
    "identity_sha256": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
    "schema_sha256": "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
    "deployed_commit": "cccccccccccccccccccccccccccccccccccccccc",
    "receipt_ref": "fixture-not-verified",
    "observed_at": "2000-01-01T00:00:00Z"
  },
  "target": {
    "identity_sha256": "dddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddddd",
    "kind": "new-local-restore",
    "namespace": "fixture-namespace"
  },
  "schemas": ["public"],
  "artifact_root": "REPLACE_WITH_EXISTING_ABSOLUTE_PRIVATE_ROOT",
  "declarations": {
    "tls_mode": "verify-full",
    "source_read_only": false,
    "access_reviewed": false,
    "owner_ref": "fixture-owner",
    "reviewer_ref": "fixture-reviewer",
    "approval_ref": "fixture-not-approval",
    "key_custody_ref": "fixture-not-key",
    "artifact_scope_ref": "fixture-object-inventory-ref"
  },
  "capacity": {
    "dump_bytes": 1024,
    "sql_bytes": 1024,
    "object_bytes": 0,
    "restore_bytes": 1024,
    "reserve_bytes": 8589934592
  }
}
```

Schema scope hanya identifier lowercase yang eksplisit dan unik, maksimum 128; system schemas ditolak. Input produksi dengan quoted/nonstandard schema names memerlukan review/adapter terpisah, bukan silent normalization. Source/deployed SHA/receipt reference, declaration UTC freshness ≤24 jam, target baru, namespace dan owner/reviewer references diperlukan. References ini belum diverifikasi ke server atau approval service.

## Kapasitas dan filesystem

- Required peak = dump + SQL + objects + restore workspace + reserve. Penjumlahan uint64 menolak overflow; dump/SQL/restore wajib positif, reserve minimal 8 GiB. Scope tanpa objects boleh menyatakan nol, tetapi actual object inventory/disposition tetap external gate.
- Restore workspace declaration harus sudah mencakup indexes/WAL/temp dan overhead yang relevan. Angka perkiraan ini belum membuktikan ukuran data production; tooling receipt/snapshot nyata perlu melengkapinya.
- Free bytes diobservasi lewat API filesystem OS, tidak ada field `available_bytes` yang dapat dipercaya dari JSON. Kapasitas tepat batas diterima; kurang satu byte atau probe tidak tersedia ditolak.
- Root harus absolut, bersih, existing directory, bukan volume root/UNC atau link/reparse pada komponen path. Candidate `<root>/<run_id>` yang sudah ada atau tidak dapat diobservasi ditolak. Preflight tidak membuat/mengunci candidate; fase penulisan kelak tetap perlu exclusive creation dan pemeriksaan ulang karena hasil preflight dapat menjadi stale.
- Preflight ini **bukan** pemeriksaan lengkap ACL/encryption/key custody. `ProtectedDestination` legacy tetap fail-closed untuk Windows yang belum punya protection review. Tidak mengubah ACL atau mengambil ownership.

## Streaming integrity yang sudah tersedia

`DigestStream(ctx, reader, expectedBytes, maximumBytes)` menghitung SHA-256 dengan buffer 64 KiB, tanpa membaca seluruh arsip ke RAM. Expected length harus positif dan ≤ maximum, hard ceiling 1 TiB. Deadline wajib, maksimum 30 menit. Truncation, trailing bytes, reader error, repeated no-progress dan cancellation tidak menghasilkan receipt sukses/partial hash.

Context dicek sebelum/sesudah tiap read. **Ini cooperative cancellation**, bukan jaminan dapat memutus arbitrary blocking reader/syscall. Sumber blocking memerlukan adapter yang mendukung cancellation sebelum dipakai untuk exporter operasional.

`VerifyStreamFile` membaca satu basename melalui anchored `os.Root`, memeriksa link/reparse, file regular, inode/size/mtime dan expected SHA-256. Tidak membuat, mengganti, mengekstrak, mendekripsi atau menghapus file. Root/path replacement atau file corruption ditolak sejauh observasi/anchoring tersebut; expected digest tetap harus datang dari receipt yang terverifikasi secara terpisah.

**Checksum cocok hanya membuktikan integritas byte terhadap expected digest.** Tidak membuktikan encrypted archive authentication, provenance server, validitas SQL/extension/trigger, snapshot consistency, object recovery atau actual restore. Tidak ada format backup terenkripsi baru atau plaintext fallback.

## Riwayat blocker enkripsi sebelum adapter CLI 16 November

Enkripsi streaming direncanakan memakai library/format yang sudah ada, kandidat [Go age v1.3.2](https://pkg.go.dev/filippo.io/age@v1.3.2), bukan chunk protocol kriptografi buatan proyek ini. Download modul melalui konfigurasi jaringan normal gagal pada proxy lingkungan; source dependency, checksum, `go.mod` dan `go.sum` **tidak diubah**. Tidak ada percobaan melewati proxy atau memasang executable pihak ketiga.

Karena itu DLV-04 masih **PARTIAL / ENCRYPTION_DEPENDENCY_BLOCKED**. Belum ada new streaming encrypted writer, wrong-key/truncated-authentication test, key recovery atau protected large-file export. Existing `Seal/Unseal` tetap berformat dan berbatas 32 MiB; receipt lama tidak dianggap kompatibel dengan format baru yang belum ada.

Setelah dependency dapat tersedia secara terverifikasi: pin/checksum/review dependency; implementasikan encrypted stream + exclusive protected output dan recovery/read-to-EOF checks; uji corruption/truncation/wrong-key/interruption. Baru lanjutkan integration snapshot/restore pada executor yang sesuai. Tidak membutuhkan DB live untuk pekerjaan tersebut.
