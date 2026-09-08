# M00 — Local source baseline dan operational blockers

Observed 5 September 2026 UTC / 6 September 2026 WIB. Source inventory dijalankan read-only oleh `cmd/baseline`; latest immutable run ID ada pada [M00 checkpoint](../modules/M00.md). Tidak membuka koneksi database, menyalakan aplikasi legacy, membaca object storage atau mengeksekusi lifecycle hooks.

## Checkout yang benar-benar dibaca

| Checkout | Branch | Checkout commit | Dirty source checkout | Deployed artifact |
|---|---|---|---|---|
| `backend-live` | `master` | `f73edf261d006686a3ae317aa93c411e3648b850` | false saat scan | UNKNOWN |
| `backend` | `staging` | `dd3413e771409bee9ccb38c59d9576b86545b0cf` | false saat scan | UNKNOWN |

Source hashes: live `3b4fbda707b76e3fc02ab802ebc25c6e5d359d7043a8ba6d45bbefd2625f8d5d`; staging `a6faa566b4803f62d870ac655023486b210348b3d89e362c9eed2864c51ee3d0`. Hash membekukan file TS yang dibaca, bukan isi database atau image deployment.

Selisih Git exact commit pair: 0 commit hanya-live, 8 commit hanya-staging; 19 changed files (18 source, satu `.gitignore`). Seluruh 64 file TS `src/` pada **masing-masing** checkout diperiksa scanner, bukan sample controller.

| Declaration inventory per checkout | Live | Staging |
|---|---:|---:|
| Controller declarations | 7 | 7 |
| HTTP method decorators | 110 | 110 |
| Named `@Entity` declarations | 19 | 19 |
| Key/index declarations | 21 | 21 |
| Relation declarations | 58 | 58 |
| Timer/scheduler call locations | 3 | 3 |
| FK CASCADE/SET NULL declaration locations | 16 | 16 |
| DDL-in-source locations | 4 | 4 |
| Schema synchronize config locations | 1 | 1 |

**Batas interpretasi:** regex scanner menghasilkan declaration locations, bukan parser semantik, effective route authorization, runtime scheduler inventory atau PostgreSQL catalog. Jumlah tabel DB bisa berbeda; session table/manual DDL tidak direpresentasikan seluruhnya oleh `@Entity`. Jumlah sama tidak berarti definition/business behavior sama.

## Schema tidak boleh diasumsikan identik

Diff konkret di `backend/src/classes/entities/batch.entity.ts`: staging menambahkan nullable JSONB `logbookSchedule` yang tidak ada dalam live checkout pada commit di atas. `class.entity.ts` menambahkan `scope` dan `programs` pada bentuk JSON quick links; ini perubahan JSON contract, tidak otomatis kolom SQL baru. Diff `audit-log.entity.ts` pada pair ini hanya formatting import.

`backend/src/app.module.ts` mengubah pemilihan/fallback env; konfigurasi `synchronize` dan DDL startup tetap harus diperiksa terhadap environment runtime. Checkout diff membuktikan ada **perbedaan schema yang dideklarasikan kode**, bukan membuktikan live DB belum/sudah memiliki kolom tersebut. Actual schema drift masih BLOCKED sampai catalog read-only kedua DB tersedia.

## Identitas koneksi lokal — jangan dipakai untuk copy

**Update sesi staging:** pengguna kini menyediakan `backend-v3/backend-go/.env.staging`; latest scanner mencatat **empat** config observations, sedangkan tiga file di paragraf historis berikut adalah baseline sebelumnya. File baru ber-fingerprint sama dan user-identified sebagai staging. Setelah CA diberikan, explicit Go probe berhasil verified TLS/read-only dan membaca 20 tabel public, termasuk nullable JSONB `batches.logbookSchedule`. Ini bukan perbandingan actual dua DB. Lihat [checkpoint CA terverifikasi](../checkpoints/2026-09-06-staging-ca-verified.md).

`backend-live/.env`, `backend/.env.staging`, dan `backend/.env` semuanya present dan menghasilkan fingerprint host/port/user/database **identik**. Password/token tidak dicetak atau dimasukkan ke report; parser tidak mengeksekusi dotenv/interpolation. Fingerprint berisi identity fields, bukan validasi bahwa credential bisa dipakai.

Ini **tidak membantah** keterangan pengguna bahwa deployment live dan staging memakai dua DB. Ini membuktikan tiga file lokal tersebut **belum menyediakan bukti dua identitas DB berbeda**. Deployment bisa memakai secret/env lain. Tidak mencoba salah satu credential sebagai production hanya karena nama foldernya `backend-live`.

## Gate yang belum tertutup

| Gate | Bukti yang dibutuhkan | Aman dikerjakan sekarang |
|---|---|---|
| Deployed version | Deployment manifest/image digest dan Git SHA live/staging | Source diff dan unknown-state reporting |
| Source/target identity | Secret **reference/lokasi file** yang benar; provider project/database IDs diverifikasi read-only | Pemeriksaan metadata lokal tanpa koneksi |
| Least privilege dan TLS | Source role read-only, target role scope dan production TLS masih pending | Staging TLS/hostname verified; role teramati owner/BYPASSRLS/write-capable, bukan least privilege |
| Schema/jobs/files/capacity | Semua namespace/triggers/functions, scheduled tasks, R2 references, headroom dan owner | Public catalog/constraints/indexes/policies/extensions/size tersedia; row counts hanya estimasi planner |
| Backup dan import | Approved scope, backup terverifikasi + isolated restore proof, enkripsi/ACL, source→target distinctness | Tidak ada export/import/DDL dijalankan |

Sesuai skill Supabase, identitas provider, certificate verification dan backup/restore evidence harus diperiksa sebelum operasi nyata. Tidak mengganti `verify-full` dengan `require`/certificate bypass saat troubleshooting. [SSL enforcement](https://supabase.com/docs/guides/platform/ssl-enforcement). Grant/RLS dan object-byte backup tetap gate terpisah, bukan dianggap selesai oleh connection predicate.

## Worktree v3 dan rollback lokal

`backend-v3` sudah merupakan worktree branch `be-v3`. Tracked deletions legacy/TypeScript yang sudah dibuat pengguna tetap dipertahankan; tidak di-restore, di-stage massal, atau di-commit oleh checkpoint ini. Go/QA/docs baru ada di `backend-go`; satu CI integration file ada di `.github/workflows/` root worktree. Tidak ada perubahan source pada checkout `backend` atau `backend-live`.

Belum ada branch push/commit dari implementasi ini. Jangan `git add -A` sebelum meninjau deletions pengguna; stage hanya paths hasil implementasi yang telah direview. Tidak ada runtime live yang perlu di-rollback karena tidak ada deployment atau data mutation.
