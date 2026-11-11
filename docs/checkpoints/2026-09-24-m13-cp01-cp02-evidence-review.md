# Review bukti M13-CP01/CP02 — 24 September 2026

Ini **review retrospektif bukti yang sudah ada**, bukan run QA atau perubahan kode baru. Scope dibekukan pada [16 September](2026-09-16-m13-cp01-cp02-scope.md) dengan [manifest](../../qa/manifests/M13-CP01-CP02.json). Artefak paling baru yang memuat kedua slice ialah run `20260922T125746Z-postgres-lab-2e3def603dd8` pada source SHA-256 `25fb237a09bc2cf81e9e988dfd74646624f6a7485a6ee1da0600aa121db1833c`. Profil `new-local-synthetic`, hasil keseluruhan `PASS_SYNTHETIC_ONLY`.

Fingerprint executable source dihitung ulang pada review 24 September dan masih sama dengan source hash run. Perubahan Markdown rekonsiliasi tidak masuk fingerprint executable; perubahan kode, manifest, SQL atau workflow berikutnya akan membuat bukti ini stale sampai QA diulang.

| CP | Frozen Go/DB | Frozen Playwright API | Backup+restore sebelum installment | Rekonsiliasi | Keputusan |
|---|---:|---:|---|---|---|
| M13-CP01 | 2/2 PASS | 2/2 PASS | SQL/custom, `pre_m13_cp01_proof`, `VERIFIED_FIXTURE_SCOPE` | `m13_cp01_reconciliation=PASS` | PASS lokal sintetis |
| M13-CP02 | 2/2 PASS | 2/2 PASS | SQL/custom, `pre_m13_cp02_proof`, `VERIFIED_FIXTURE_SCOPE` | `m13_cp02_reconciliation=PASS` | PASS lokal sintetis |

Kode berada di `internal/labdb/m13.go`, `internal/notifications/`, worker M03, opt-in `cmd/identity-lab`, dan `qa/tests/notification_api/`. CP01 membuktikan receipt/attempt, lease dan visibility pada rehearsal. CP02 membuktikan preference, secret reference, suppression untuk historical import, dan sink QA tanpa pengiriman jaringan. Regresi M13 diulang dalam full dependency journey M09/M10; raw result dan dump disimpan lokal pada `D:\Projects\lms-v2\lms-v3-qa-artifacts\20260922T125746Z-postgres-lab-2e3def603dd8`.

Digest backup CP01: custom `eb7bd74e2603a2eb1375eaf83838dbcaaf1a671415143166421e1e8f996e5653`, SQL `7c6a6f388d8ff8207bc1f30992f0a463c0472e2cb5a2dadfce38106c19a5bf5f`. CP02: custom `af930b74a656a8e4a2781bab4d1e128e4cd0dc0369b60aa0e4f4a21b892396ed`, SQL `138b6df0a9227bd17a475767ac7cfbb41dc4958acd3b8bb95fabb21529ef73c6`. Semua digest berasal dari `summary.json` run yang sama.

**Batas:** Ini 2/4 CP lokal, bukan penerimaan M13-D01–D05 penuh: canonical tetap 0/5. Provider/SMTP dan secret manager nyata, recipient/template policy, shared `cmd/api`, remote CI, AI suggestion CP03 dan consumer closure CP04 belum diverifikasi. Jangan menyimpulkan exactly-once dari at-least-once/rehearsal sink. Run mencatat `shared_database_access=NONE`, `production_backup=NOT_EXECUTED`, `database_deletion=NONE`, dan `cleanup=STOP_PROCESS_RETAIN_CLUSTER`; tidak ada bukti copy atau backup data live.

Status `NOT_STARTED` untuk M13-CP01/02 pada brief/dashboard lama tidak cocok dengan source dan bukti ini. Koreksi hanya mengubah pembacaan status; tidak mengubah manifest, raw run, atau tanggal scope freeze.
