# Encrypted streaming — local acceptance and operational gates

> **17 November — current recovery:** [Seven-case dependency accepted](../checkpoints/2026-11-17-protected-package-accepted.md); [new local recovery implementation](../checkpoints/2026-11-17-local-recovery.md): **81 Go/95 ops PASS**, current native **4/9 PASS + 5 fixture AccessDenied FAIL**, ten DB cases NOT_RUN. [Owner runner](LOCAL_RECOVERY_RUNBOOK.md); [why dotenv is not staging activation](STAGING_CONFIGURATION_READINESS.md). No native recovery/module acceptance, env/DB edits or publication. Earlier source-bound snapshots below are history.

> **17 November — current:** [Protected output/package implementation](../checkpoints/2026-11-17-protected-package.md), source `126c217ad43a732e67d5d3b32473233ed5bd94385596a78872f0ba9b7b63371b`: **79/79 Go + 93/93 ops + vet/Windows build PASS**, Linux cross-build/tagged compile PASS_BUILD_ONLY. Mandatory actual suite **4 PASS + 3 fixture AccessDenied FAIL**; restricted token confirmed. **IMPLEMENTED_PENDING_OWNER_NATIVE_QA**, no CP/module/restore closure. [Contract/runner](PROTECTED_PACKAGE_RECOVERY.md). Earlier source-bound results below remain history; no DB/delete/commit/push.

> **Accepted 16 November:** [Reviewed actual-tool run](../checkpoints/2026-11-16-age-stream-accepted.md) `20261116T104948Z-age-stream-3bc681da9e6f`: **4/4 actual crypto + 75/75 Go + vet PASS**, source `469e7d3b57b55a418994173ef73c3a992863bf6f8f72b6b2d8c12176c58ee6ee`. Same-source 92/92 ops reused. Synthetic crypto slice accepted; full M01/DLV-04 and actual restore remain open. No need to repeat this run solely for documentation closure.

16 November 2026. Mendukung DLV-04 / M01-CP02; tidak membuat CP atau canonical deliverable baru. [Scope/evidence](../checkpoints/2026-11-16-age-stream-handoff.md). Scope hanya synthetic lokal, tanpa database/provider.

## Yang tersedia

[age_stream.go](../../internal/migration/backup/age_stream.go) menyediakan `SealAgeStream` dan `VerifyAgeStream`. Runtime crypto memakai native **age CLI v1.3.2**, bukan OpenSSL fallback atau cipher buatan proyek. Root `go.mod` / `go.sum` tidak ditambah dependency. Karena itu backend biasa tetap dapat dibangun offline, tetapi operasi ini memerlukan binary terverifikasi secara terpisah.

Tool path harus absolute, regular, bukan link/reparse; basename sesuai platform, SHA-256 cocok, embedded Go build info harus sesuai package/module/version/module checksum dan tanpa replace. Runner memperoleh checksum dari Go module download dengan sumdb aktif. Checksum yang dimasukkan sembarang oleh caller tetap bukan bukti approval atau custody. Direktori tool dan key harus berada di bawah kontrol operator yang dipercaya; ini bukan endpoint publik atau proteksi terhadap administrator lokal yang mengganti executable saat eksekusi.

Standard tar di dalam age berisi **dua entry**: `manifest.json` (version/run/target/scope/timestamp/size), lalu `payload`. Ini application framing `age-v1-tar-binding-v1`; algoritma/envelope age tetap standar. Raw `.age` dari alat lain belum tentu memiliki inner tar tersebut. Existing 32 MiB `Seal/Unseal` tetap terpisah; tidak ada silent conversion atau klaim format kompatibel.

Streaming menggunakan buffer terbatas, exact input length, max output budget dan deadline wajib. Short/trailing source, failed writer, tool failure, wrong binding atau deadline tidak menghasilkan success receipt. Partial ciphertext tetap milik caller dan tidak dihapus/promosikan. Caller wajib menyediakan writer/storage yang telah dilindungi; adapter ini belum menyediakan operational exporter dengan exclusive protected real-data destination.

Verify hanya mengonsumsi decrypted stream ke parser/hash. Tidak mengeluarkan plaintext ke file/caller. Sukses memerlukan process exit/authenticated EOF, exact encrypted size/hash, dua tar entry, binding dan payload hash. Receipt `PASS_AUTHENTICATED_STREAM_ROUNDTRIP_ONLY` tetap tidak memberi izin restore atau membuktikan SQL/archive/file recovery. Cancellation arbitrary blocking input tetap cooperative: reader wajib mendukung pembatalan I/O.

Identity saat ini dibatasi **satu unencrypted X25519 age secret key** di file lokal bounded; key comments diperbolehkan. Encrypted/password/plugin/multiple identities tidak didukung. Recipient harus berformat age; validitas kriptografis diperiksa age. Subprocess tidak mewarisi PATH/HOME/provider/DB/plugin environment dan stderr tidak direplay. Independent custody, ACL, key recovery orang kedua, PQ/plugin integration belum dibuktikan. Jangan meletakkan key di artifact package, repository atau public logs.

## Satu perintah dari terminal Windows biasa

```powershell
D:\Projects\lms-v2\backend-v3\backend-go\qa\RUN_AGE_STREAM_QA.cmd
```

Runner publik [age_stream_run.py](../../qa/age_stream_run.py), wrapper [CMD](../../qa/RUN_AGE_STREAM_QA.cmd), manifest [M01-AGE-STREAM.json](../../qa/manifests/M01-AGE-STREAM.json). Memerlukan Go project toolchain, Python `.venv` yang ada, izin workspace dan jaringan normal. Tidak menaikkan permission, mengubah proxy/TLS, atau memasang system-wide tool.

Urutannya: storage admission/lock → verified source-only backup → Go module checksum/install age + keygen v1.3.2 ke `.cache/qa-tools/age-v1.3.2` → empat actual tagged crypto cases → exact default Go registry → vet → small private receipt. Cache/module berada di workspace; file temporary mengikuti OS TEMP/TMP dari clean environment dan bukan context portabilitas permanen. Source harus tetap sama selama proses. Run time dibatasi 60 menit setelah backup, masing-masing subprocess memiliki timeout. Storage admission/backup dapat menambah durasi awal.

Runner **tidak memanggil retention cleanup**, tidak membuat PostgreSQL cluster, tidak memuat `.env`, tidak mengekspor source DB, tidak restore/migrate, tidak commit/push. Existing artifact tidak dihapus. Source backup/log kecil disimpan di `backend-v3/private-backup/<run>-age-stream-*`; cache tool/dependency adalah biaya disk baru yang tetap diukur admission. Ini bukan zero-storage claim atau pengganti kebijakan retensi terpisah.

Tools yang ada tanpa verified-install receipt atau hash berbeda dihentikan untuk review; tidak ditimpa otomatis. Kalau download/install terputus dan menyisakan binary tanpa proof, simpan evidence dan periksa sebelum retry; jangan memalsukan receipt. Owner boleh menjalankan probe tanpa download:

```powershell
Set-Location 'D:\Projects\lms-v2\backend-v3\backend-go'
.\.venv\Scripts\python.exe -m qa.age_stream_run --probe-only --no-download
```

Exit probe selalu 2: keberadaan tool belum membuktikan crypto QA. `PASS_LOCAL_CRYPTO_SLICE_ONLY` baru dapat muncul setelah empat actual cases dan default regression tepat PASS pada source yang sama. Failure/admission unknown tetap BLOCKED, bukan SKIP/PASS.

## Frozen actual crypto cases

| Go case dengan tag `qaage` | Assertion |
|---|---|
| `TestAgeActualLargeRoundTrip` | Real pinned age + keygen, plaintext virtual >33 MiB, encrypted temporary file, authenticated verify ke EOF; tanpa plaintext backup |
| `TestAgeActualTamperAndWrongKey` | Wrong key, truncation, trailing bytes, auth-tag corruption; outer digest diperbarui pada corrupt cases agar tidak hanya menguji checksum |
| `TestAgeActualBindingAndShortInput` | Binding salah dan short/trailing plaintext ditolak tanpa receipt sukses |
| `TestAgeActualCancellationAndWriterFailure` | Pre-cancel tidak menulis output, failed output writer tidak menghasilkan receipt |

Default empat Go safety tests dan dua Python ops tests menguji framing/refusal/evidence/admission/environment. Tes-tes itu **tidak menggantikan real crypto suite**. Tagged compile/list PASS hanya bukti build/discovery. Key sementara dibuat dalam Go test TempDir, tidak ditulis ke log; test framework membersihkan fixture miliknya sendiri, tidak menjalankan cleanup artifact/DB yang sudah ada.

## Remaining gates

1. Actual pinned-tool four-case suite **PASS accepted locally** pada run di atas. Source/tool changes require affected revalidation; operational custody/tool provenance tetap berlaku. Bukan general permission untuk real-data export.
2. Protected exclusive output/key custody/ACL + operational exporter yang menghubungkan snapshot konsisten ke streaming belum selesai.
3. Package validator saat ini memeriksa integritas/header/metadata; belum otomatis decrypt/consume format baru ini atau memberi authenticated-package status. Perlu wiring eksplisit setelah crypto gate PASS.
4. Actual SQL/custom archive readability, isolated DB restore, object recovery dan reconciliation belum dijalankan pada source ini.
5. Playwright/runtime, remote CI/race/PG dan production identity/scope gates tetap terpisah. DLV-06 / full M01 belum READY/closed.

Tidak perlu meminta data live untuk menutup actual synthetic crypto gate ini. Setelah tooling/recovery/control gates lulus barulah baseline nyata dibutuhkan untuk mapping dan actual source reconciliation.
