# 12 November — offline baseline preflight dan streaming backup

## Scope freeze sebelum implementasi

Instruksi: kerjakan urutan 1 (preflight baseline) dahulu, kemudian urutan 2 (backup besar) setelah QA lokal preflight terpenuhi. Mendukung DLV-03/04 dan sebagian M00/M01; bukan CP/module baru atau closure canonical. Backend/tool operator tetap Go; Python hanya orchestration QA.

- Tidak ada koneksi source/live/staging, dump/restore database, migration/DDL, perubahan ACL, penghapusan, commit atau push.
- Source awal `36dd36aa1f46b237c62b9216b1241c62895c921df0f30dbf01a3d63576da246a`. Existing worktree changes harus dipertahankan. Source backup diverifikasi sebelum edit.
- CLI preflight offline baru tidak mengubah kontrak planner legacy. Strict bounded/versioned JSON; duplicate/unknown/trailing input ditolak. Source/target canonical identity declarations harus berbeda; scope eksplisit, evidence references/freshness dan operator declarations diperiksa.
- Kapasitas: perhitungan peak dump + rendered SQL + objects + restore workspace + reserve dengan overflow protection; free bytes diobservasi dari filesystem, bukan dipercaya dari JSON. Target baru, no overwrite, link/reparse/path ambiguity ditolak. Tidak membuat directory candidate atau membuka database.
- Declaration, checksum atau receipt shape **tidak membuktikan** actual source identity, role/TLS, approval, ACL/key custody. Report selalu `authorizes_execution=false`, real readiness false, dengan residual gates eksplisit bahkan jika local preflight valid.
- Exit preflight: Go positive/negative tests + actual CLI subprocess tests dengan expected IDs; seluruh regression default Go dan vet/build terdampak. Playwright yang sebelumnya blocked tidak diberi PASS atau diganti client lain.
- Streaming: gunakan format/library enkripsi yang sudah dikenal, bounded memory + byte/time limits, exclusive destination, checksum, corruption/truncation/wrong-key failure dan authenticated read hingga EOF. Jangan merancang format kriptografi sendiri atau sekadar menaikkan batas Seal 32 MiB. Dependency/tool unavailable harus dilaporkan, bukan fallback plaintext.
- Uji payload sintetis >32 MiB secara streaming; artefak uji kecil jika memungkinkan, tanpa cluster database baru. Existing Seal/Unseal dan callers tetap kompatibel.
- Exit streaming: unit/CLI QA adversarial, existing backup regression; protected real-data file output tetap fail-closed bila OS protection belum dapat dibuktikan. Full operational exporter, actual restore dan baseline production tetap gate berikutnya.

Hasil dan evidence belum tersedia saat scope freeze; akan ditambahkan setelah pelaksanaan. Backup source bukan backup DB. Tidak memberi readiness dari keberadaan Markdown.

## Hasil pelaksanaan

| Scope | Status | Bukti / batas |
|---|---|---|
| DLV-03 — offline preflight slice | **PASS_OFFLINE_PREFLIGHT_SLICE** | CLI Go baru, strict/versioned JSON, fresh declarations, distinct identity declarations, scope/binding, actual disk observation dan no-overwrite/path gates. Positive/negative Go dan actual CLI tests lulus. Full DLV-03 tetap memerlukan actual identity/role/TLS/approval/custody evidence. |
| DLV-04 — streaming byte integrity slice | **PASS_INTEGRITY_ONLY** | Reader/file verifier bounded memory, expected size/hash, EOF, cancellation/error/path checks; >33 MiB virtual synthetic payload lulus. Ini bukan exporter atau enkripsi. |
| DLV-04 — protected encrypted large backup | **PARTIAL / ENCRYPTION_DEPENDENCY_BLOCKED** | Kandidat library age tidak dapat diunduh lewat konfigurasi proxy normal. Tidak ada checksum dependency yang diperoleh, tidak ada perubahan go.mod/go.sum, tidak ada custom crypto/plaintext fallback. Streaming encryption, authentication/wrong-key, recovery dan protected output belum dibuat/diuji. |
| Actual DB baseline / restore / staging migration | **NOT_RUN / NOT_AUTHORIZED** | Tidak ada DB connection, cluster baru, export, SQL migration, ACL mutation, deletion, deploy, commit atau push. |

[Kontrak penggunaan dan keterbatasan](../baseline/OFFLINE_PREFLIGHT_AND_STREAMING.md). Kode utama:

- [cmd/baseline-preflight](../../cmd/baseline-preflight/main.go): hanya `--plan-only`, stdin JSON bounded, exit 0 hanya local declaration/path/capacity checks. Report selalu `authorizes_execution=false`, `real_baseline_verified=false`, `database_access=NONE`; external gates tetap eksplisit.
- [backup/intake.go](../../internal/migration/backup/intake.go): duplicate/escaped duplicate/unknown/case-variant/null/trailing/oversized JSON ditolak, scope canonical, peak capacity overflow-safe, actual OS free bytes, candidate existing/unknown ditolak. Tidak memuat `.env`, membuka receipt sebagai credential, membuat target, atau menganggap operator booleans sebagai approval.
- [backup/stream_digest.go](../../internal/migration/backup/stream_digest.go): SHA-256 streaming buffer 64 KiB, size/budget/deadline wajib, ceiling 1 TiB, max deadline 30 menit, no-progress/cancel/short/trailing/error tidak menghasilkan digest sukses. Anchored os.Root dan file identity/size/mtime/hash checks, tanpa output-file write/decrypt/delete.
- [CLI QA](../../qa/tests/ops/test_baseline_preflight.py): actual Go process; env DB URL fixture tidak dipakai, ambiguous/secret-bearing input ditolak tanpa echo, no-op filesystem dan existing sentinel dipertahankan.
- `qa.run` kini build/hash binary baseline-preflight dan memasangnya dalam env fixture; diagnostic allowlist mengenali nama command tersebut. Registry Go dan M00/M03 menambahkan cases, tidak mengurangi existing cases/gates atau menambah canonical D.

## Urutan QA dan coverage

1. **Sebelum mulai bagian streaming:** source `29269028f28ec84204182905ee60beca4e8a642c35666361bbd11a462ff431c6`, targeted backup unit **9/9**, actual CLI **3/3**, kemudian default Go regression **60/60**, vet dan build PASS. Ini memenuhi urutan dependency preflight dahulu.
2. **Source final:** `85afcf9f20ce0efd36878746a99c4bea1db87dd21f355cc5fb1b787d1d2155ca`.
3. Targeted backup unit **12/12**, actual CLI **3/3**, targeted build/vet PASS pada source final. Delapan Go tests baru: lima preflight dan tiga integrity; empat test backup lama ikut regression.
4. Default Go registry **63/63 exact PASS**, `go vet ./...`, `go build ./...`, `gofmt -l` PASS.
5. Python ops **87/87 exact PASS**, termasuk ketiga CLI cases baru, diagnostic/registry/retention/helper regressions. Ketiga CLI cases adalah subset 87, tidak dijumlahkan ulang.
6. Coverage statement paket **internal/migration/backup = 92.0%**, dari profile unit targeted. Bukan whole-backend/CLI coverage, verified-D ratio, atau persentase migrasi selesai.
7. Linux amd64 cross-compile seluruh package PASS; Linux execution/race dan Linux-specific symlink fixture branch belum dijalankan. Windows fixture menggunakan attribute checks tanpa membuat junction atau mengubah ACL.

Current-source full bootstrap/Playwright/DB journey **tidak dijalankan** dalam slice ini. Tiga Playwright driver errors pada source sebelumnya tetap unresolved environment gate, bukan tiga ERROR baru yang direkayasa pada hash final dan bukan PASS yang diambil dari source lama. Tidak mengganti Playwright dengan client lain. Full M00/M01/M03, remote CI dan canonical M00–M18 **0/97 D** tetap unchanged.

### Assertion exit lokal

| Kelompok | Evidence | Status |
|---|---|---|
| Strict input + source/target/scope/freshness declarations | Go strict/declaration cases + actual CLI malformed input | PASS_LOCAL |
| Overflow/headroom + OS capacity / missing probe refusal | Go arithmetic/probe boundary + actual CLI sufficient/insufficient disk | PASS_LOCAL |
| No overwrite/path/link + no authority/no DB | Go filesystem/authority cases + CLI sentinel/no-op checks | PASS_LOCAL; actual operator protection tetap external |
| Bounded streaming >32 MiB / short/trailing/error/cancel | Three streaming Go tests | PASS_INTEGRITY_ONLY |
| Standard streaming encryption / wrong key / authenticated final EOF | Dependency unavailable, implementation tidak dibuat | BLOCKED, bukan PASS |
| Actual snapshot, protected key custody, file recovery dan DB restore | Tidak dieksekusi | NOT_RUN |

## Backup dan penyimpanan evidence

`backend-v3/private-backup/baseline-deliverables-20261112/`:

- `source-before.zip`: **1,101,222 bytes**, SHA-256 `defd559f639ecf15c1f97d54f52f34426b5ea8e20c9e5340beee25e5fc388345`; reopen verification PASS sebelum edit. Public source/context saja; env/key/Git metadata/raw DB/cache tidak ikut.
- `preflight-qa/summary.json`, `go-regression-preflight/summary.json`: bukti preflight lulus sebelum streaming dimulai.
- `final-targeted/summary.json`, `go-regression-final/summary.json`, `ops-regression/summary.json`, `build-coverage.json`: source-bound final evidence, kasus dan actual commands/logs.
- `dependency-inspect.json`: download age v1.3.2 gagal pada environment proxy sebelum module tersedia. Tidak mencoba bypass proxy, tidak menambahkan library tanpa integrity verification, dan tidak meminta perubahan permission.

Payload besar dibangkitkan saat dibaca, tidak disimpan sebagai dump/cluster. Test file corruption memakai fixture kecil; tidak ada cleanup backlog atau perubahan kebijakan retensi. Ini source backup dan synthetic evidence, bukan backup DB production.

## Next yang tetap terbuka

1. Dependency encryption tersedia melalui setup yang diizinkan → review/pin/checksum → implement encrypted streaming writer dan authenticated recovery/negative tests. Sampai itu terbukti, tidak ada export data nyata melalui jalur baru.
2. Validator paket baseline dapat memakai `VerifyStreamFile` untuk receipt integrity, dengan provenance/consistent-snapshot/object checks dan actual restore tetap terpisah. Jangan menyamakan hash match dengan restore proof.
3. DLV-06 belum READY. Input production/live belum dibutuhkan untuk melanjutkan tooling offline ini; actual metadata/snapshot/owner diperlukan pada gate baseline berikutnya.
4. Existing legacy `Seal/Unseal` (32 MiB), staging inspector, synthetic M15/migration fences, schema SQL dan runtime API tidak diubah menjadi menerima shared targets. Tidak ada CP/module closure atau commit/push dari pekerjaan ini.
