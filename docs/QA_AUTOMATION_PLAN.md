# Backend Go — Automation QA Plan

**Update M12-CP02/CP03, 15 September:** `qa.postgres_lab --logbook-lifecycle-journey` menjalankan dependency chain penuh, membuat backup SQL/custom + actual isolated restore terpisah sebelum schema entry dan review, lalu Go/DB, Playwright API, reconciliation dan privilege checks. [Evidence final](checkpoints/2026-09-15-m12-cp02-cp03.md) PASS 8/8 frozen cases pada source `a008d3be…`. Playwright binary kini menginstrumentasi `internal/logbook` dan menolak profil hilang; remote CI/shared runtime tetap belum verified. Manifest frozen [M12-CP02/CP03](../qa/manifests/M12-CP02-CP03.json).

**Update M12-CP01, 14 September:** `qa.postgres_lab --logbook-period-journey` membuat cluster sintetis baru, menjalankan seluruh dependency M03–M08/M15/M11, mengambil fresh SQL/custom backup dan actual isolated restore sebelum schema M12, lalu 2 Go/DB + 2 Playwright case, FK/history reconciliation dan regresi akhir. [Evidence final](checkpoints/2026-09-14-m12-cp01.md) PASS pada source `25174065…`; remote CI dan runtime bersama belum verified. Manifest frozen [M12-CP01](../qa/manifests/M12-CP01.json). Deskripsi runner bertanggal lebih lama di bawah adalah histori.

**Update 14 September:** `qa.postgres_lab --migration-completion-journey` sekarang menjalankan dependency M03–M08, M15-CP01–05 dan M11-CP01 pada cluster PostgreSQL sintetis baru; CP05 dan M11 masing-masing memerlukan fresh SQL/custom dump + actual isolated restore sebelum installment. Manifest frozen [M15-CP05/M11-CP01](../qa/manifests/M15-CP05-M11-CP01.json) mengikat 3 dan 4 case. [Run final](checkpoints/2026-09-14-m15-cp05-m11-cp01.md) lulus; remote CI belum terbukti. Daftar runner bertanggal 13 September di bawah adalah histori.

## Pelacakan M → CP — revisi 13 September 2026

[Peta modul](MODULE_CHECKPOINT_MAP.md) dan bagian CP pada setiap `modules/Mxx.md` menjadi tampilan utama pekerjaan. **Mxx-Dnn tetap acceptance canonical; Mxx-CPnn adalah potongan scope**, bukan test-case ID atau numerator D baru. CP01–CP12/test IDs/run lama dipertahankan; scope hasil lama dapat mendukung CP baru melalui alias yang eksplisit. Tidak ada manifest executable atau hasil QA diubah oleh revisi Markdown ini.

Sesudah satu CP diimplementasikan: freeze scope/case/gates → preflight → suite yang relevan + regression dependencies → evidence/hash → update CP milik M → evaluasi klausa D yang benar-benar tertutup → laporkan sisa M. Sesudah seluruh CP coding, masih wajib full module/integration/QG closure; external/browser/provider gate tidak berubah menjadi PASS dari local suite.

Handoff minimal: `M / CP / supported D / scope environment / tests dan source hash / CP status / D residual / migration+owner status / next CP`. Jumlah bagian teruji boleh ditampilkan sebagai `2/6`; bukan 33% effort atau 33% fitur. Jangan double-count shared evidence menjadi overall project completion. M19 denominator tetap unfrozen.

**Batas automation aktual:** runner menerima M00/M03. Lab menerima CP08–CP12 via `--identity-journey`, M03-CP03–CP04 via `--runtime-journey`, M03-CP05–CP06 + seluruh regresi melalui `--worker-journey`, M05-CP03–CP04 melalui `--participation-journey`, M06-CP01–CP02 melalui `--file-journey`, M06-CP03 + M07-CP01 melalui `--content-journey`, M07-CP02 melalui `--content-safety-journey`, M07-CP03 melalui `--content-contract-journey`, bounded M08-CP01–CP04 melalui `--submission-journey`, M15-CP01/CP02 sintetis melalui `--migration-foundation-journey`, dan M15-CP01–04 sintetis beserta semua dependency melalui `--migration-backfill-journey`. Registry fail-closed module→CP→case→D tersedia dan menolak evidence duplicate, missing, unexpected, stale, non-PASS, atau unknown; `qa.checkpoint_report` lama tetap dipertahankan untuk kompatibilitas. Belum ada command generik `--checkpoint Mxx-CPnn`, package runner M04–M19, atau bukti workflow remote. Pelaksana tetap menjalankan suite eksplisit untuk scope yang belum didaftarkan; perubahan tulisan DONE tidak men-trigger proses.

[Indeks](README.md) · [Dashboard](IMPLEMENTATION_STATUS.md) · [Master plan](../../../ImplementationPlan.MD)

Status: PARTIAL_IMPLEMENTATION, checkpoint terbaru M15-CP03/CP04 synthetic slice pada 13 September 2026 WIB. Runner M00/M03, Go tests, Pytest/Playwright API tests, outbox/participation/file/content/submission/migration journeys, registry dan workflow definition tersedia. M03-CP03–CP06, M05-CP03–CP04, M06-CP01–CP03, M07-CP01–CP03, M08-CP01–CP04 bounded cases dan M15-CP01–04 synthetic cases lulus pada cluster sintetis baru. Native Linux race/non-root lokal historis lulus; ini bukan bukti remote CI atau tagged-DB race di CI. Real legacy chronology/source mapping, container, shared operator/provider/identity mode, frontend renderer dan full QG acceptance belum verified. Kontrak berikut tidak memberikan izin DB mutation shared, deployment, frontend work atau production copy.

CP01–CP05 additions: optional validated operational-receipt projection, canonical cross-OS source-path ordering, offline backup/receipt unit tests, static artifact validator, separate Linux race/non-root runner and explicit opt-in synthetic PostgreSQL lab. See [scoped implementation evidence](checkpoints/2026-09-06-five-checkpoints.md). Default QA never auto-connects to staging or creates a DB; tagged integration cases must be invoked through the approved lab and cannot silently skip. Observation digest detects accidental changes, not malicious operator forgery; it never certifies approval or clears broad module gates.

## 1. Tujuan dan boundary

Latest [M06-CP01–CP02 final evidence](checkpoints/2026-09-11-m06-cp01-cp02.md): `--file-journey` membuat fresh PostgreSQL cluster, menjalankan backup+actual isolated restore sebelum setiap migrasi, dua focused Go/DB cases dan tujuh Playwright API cases, lalu memverifikasi persistence. Focused statements CP01/CP02 35.7%/53.0%; instrumented API 43.8%. Locator object privat wajib berbeda dari checksum. Actual scanner/object provider, outbound-fetch CP03, mapping CP04, shared DB dan remote/container gates tetap pending.

Latest [M06-CP03 + M07-CP01 final evidence](checkpoints/2026-09-12-m06-cp03-m07-cp01.md): `--content-journey` membuat cluster baru, memverifikasi dump+isolated restore sebelum migration M07, menjalankan focused Go/DB 1+1 dan Playwright API 3+4 cases, lalu seluruh dependency/persistence regression. Focused statements 34.9%/48.9%; API instrumentation 29.1%/30.0%. Actual provider DNS/object/scanner, shared DB, remote/container dan content consumer gates tetap pending.

Latest [M07-CP02 final evidence](checkpoints/2026-09-12-m07-cp02.md): `--content-safety-journey` membuat cluster baru, memverifikasi dump+isolated restore tepat sebelum `m07-cp02-1`, menjalankan Go/DB 1/1 dan Playwright 4/4, serta seluruh dependency/persistence regression. Focused statements 45.6%; API instrumentation 30.7%. Raw authoring source tidak dapat di-SELECT runtime role; frontend renderer, M08 consumer, shared/remote/container gates tetap pending.

Latest [M07-CP03 final producer evidence](checkpoints/2026-09-12-m07-cp03.md): `--content-contract-journey` membuat cluster baru, memverifikasi dump+isolated restore tepat sebelum `m07-cp03-1`, menjalankan Go/DB 1/1 dan Playwright 4/4. Focused statements 40.7%; API instrumentation 30.9%. Consumer exact revision ID berikutnya ada di M08; historical-owner read tetap pending.

Prior [M08-CP01/CP02 evidence](checkpoints/2026-09-13-m08-cp01-cp02.md): `--submission-journey` membackup+actual isolated restore sebelum masing-masing `m08-cp01-1` dan `m08-cp02-1`, menjalankan Go/DB 2/2, Playwright 5/5 dan seluruh dependency/persistence regression. Focused statements 20.3%/51.2%; API instrumentation 30.2%/41.1% (denominator terpisah). CP03 receipt/deadline dan CP04 bounded chronology kini mempunyai evidence terpisah di bawah.

Latest [M08-CP03/CP04 bounded evidence](checkpoints/2026-09-13-m08-cp03-cp04.md): `--submission-journey` melanjutkan backup+actual isolated restore sebelum `m08-cp03-1` dan `m08-cp04-1`, Go/DB 1/1 per CP dan Playwright API 3/3 + 2/2, termasuk replay same-key, cutoff rollback, return hanya setelah receipt, two-attempt history dan raw chronology restriction. Focused Go statements 42.0%/1.3%; API instrumentation 34.7%/36.3% (denominator terpisah). CP04 importer/real-source mapping, exact clock-boundary/UTC transition, M09 consumer, shared/remote/container gates tetap pending.

Latest [M15-CP01/CP02 synthetic evidence](checkpoints/2026-09-13-m15-cp01-cp02.md): `--migration-foundation-journey` mengambil dua fresh dump+actual isolated restore, menguji Go/DB 1/1 dan Playwright 1/1 per CP pada source `407ec8d5…`, seluruh dependency/health/worker regression PASS. Focused Go statements 51.8%/78.1%; ini bukan full M15 coverage. CI workflow sekarang memanggil journey ini tetapi remote execution belum dibuktikan; real source identity/CA/adapter/backup tetap gate terpisah.

Latest [M15-CP03/CP04 synthetic evidence](checkpoints/2026-09-13-m15-cp03-cp04.md): `--migration-backfill-journey` mengambil dua tambahan fresh dump+actual isolated restore sebelum tiap CP, menguji Go/DB 2/2 dan Playwright 1/1 per CP pada source `3252177f…`, seluruh dependency/health/worker regression PASS. Focused package statements 54.4%/67.5%; case coverage 3/3 per CP, bukan full M15. CI workflow sekarang memanggil journey ini, tetapi remote execution belum dibuktikan; actual mapping/import/academic approval tetap gate terpisah.

Latest [CP08–CP10 final evidence](checkpoints/2026-09-07-cp08-cp10.md): default registry now **21 Go cases**, M00 19/M03 22 checks; opt-in `qa.postgres_lab --class-journey` adds exact CP08/CP09 Go integration and 6 Playwright API cases from `qa/manifests/CP08-CP10.json`, with real persistence reconciliation. Default QA does not launch this lab or connect shared DB. API coverage collection now requires nonempty executed counters and all expected packages; missing/zero coverage fails the lab. Default whole-Go statements 38.6%, separate four-package CP10 instrumentation 81.4%; do not average or treat as full-module acceptance. Linux race default PASS is not tagged DB race coverage. Historical 17-case counts below refer to CP06–CP07 only.

Checkpoint terbaru [CP06–CP07](checkpoints/2026-09-07-cp06-cp07.md): synthetic PostgreSQL lab menambah seeded row/definition reconciliation, 5 negative archive assertions, dan SQLSTATE-specific permission denial. Separate integration registry sekarang 4 Go top-level cases; default bootstrap tetap 17 Go cases, M00 19 checks/M03 22 checks. Current Linux race/non-root dan static build proof tersedia; container NOT_RUN. CP07 operator observation terpisah `OBSERVED_NOT_APPROVED`; tidak auto-connect dari Playwright/CI atau meluluskan full module dari receipt metadata.

Setiap paket M00–M19 mempunyai acceptance criteria ber-ID, test/evidence yang eksplisit, regression dependency, dan gate sebelum dianggap terverifikasi. Backend aplikasi tetap **Go**, sedangkan **Python + Pytest + Playwright** dipakai untuk QA. Frontend v3 tidak dibangun atau dianggap tested oleh plan ini.

Playwright Python dapat menguji REST API melalui `APIRequestContext` tanpa halaman/browser. Gunakan request context terpisah per actor agar cookie/credential tidak tertukar. Response status saja tidak cukup: periksa schema, state persisten, history/audit, access denial dan idempotency. Dasar API testing: [dokumentasi Playwright](https://playwright.dev/python/docs/api-testing).

Browser tests kelak digunakan untuk integration/CSRF/cookie/rendering yang memang memerlukan browser. API header checks tidak membuktikan browser SameSite behavior atau bahwa renderer frontend bebas XSS. Gate tersebut tetap external pending, bukan diberi PASS dari mock API.

## 2. Lapisan pengujian

| Lapisan | Tooling rencana | Membuktikan | Tidak membuktikan |
|---|---|---|---|
| Unit/domain Go | `go test`, table-driven cases, race detector bila applicable | State machine, policy predicates, calculations, concurrency code | SQL/FK/real deployment correctness |
| Integration PostgreSQL | Go integration + Pytest DB assertions dengan driver terpin | Transactions, locks, constraints, indexes, persisted state | Production parity bila hanya synthetic fixture |
| API functional/security | Pytest + Playwright request contexts | Real Go HTTP→policy→DB behavior dan negative access | Frontend usability/XSS renderer atau high-volume load |
| CLI/operator | Pytest invokes reviewed tools via safe argument arrays | Preflight, manifests/checksums, restore and migration receipts | Human approval jika bukti approval tidak ada |
| Browser external | Playwright Python, nanti sesuai scope terpisah | Cookie/CSRF/browser interactions dan UI end-to-end | Seluruh backend coverage hanya dari beberapa clicks |
| Performance | Dedicated approved load harness + DB/Go metrics | p95/p99/throughput/error/pool pada workload terdefinisi | Kecepatan production dari sequential API smoke tests |
| Review/operational evidence | Human sign-off + automated evidence validation | Policy approval, actual restore/pilot/deployment disposition | Boleh diganti fake signature/empty JSON |

M02 memakai tooling native legacy untuk unit/build; tidak menjalankan `go test` terhadap TypeScript. Python bukan pengganti backend maupun pembenaran menambahkan TypeScript runtime v3.

## 3. Bootstrap QA sebelum M00 dan fondasi M03

Minimum QA harness lintas paket kini mulai diterapkan setelah otorisasi implementasi; daftar berikut menjelaskan target lengkap, **bukan semuanya sudah selesai**:

1. Registry requirement/test IDs, environment allowlist, read-only inventory checks dan structured run result.
2. Pytest operator tests untuk M00/M01 dan negative preflight fixtures; tidak memerlukan API Go.
3. M01 dapat memakai PostgreSQL CLI operator yang sudah diverifikasi untuk backup/restore sebelum full platform Go tersedia. Go migration CLI kemudian mengintegrasikan prosedur ini; jangan menunggu M15 untuk backup pertama.
4. M03 memperluas harness menjadi runner package/regression, Go build/race/coverage, real API service bootstrap dan CI. Self-test runner wajib sebelum result dipakai sebagai gate.
5. Minimal gate yang belum implemented → BLOCKED/NOT_RUN; tidak mengklaim automation aktif hanya karena punya rencana command.

Struktur target (yang sudah tersedia: `qa/run.py`, `qa/manifests/`, `qa/tests/ops`, `qa/tests/baseline`, `qa/tests/api`, unit Go dan docs):

```text
backend-go/
  qa/
    run.py                 # entry point orkestrasi QA, bukan application backend
    manifests/             # manifest D/T IDs, cases, gates, dependencies, hashes
    tests/api/             # Playwright API tests per paket
    tests/db/              # DB/reconciliation assertions
    tests/ops/             # manifest/preflight/restore/runner checks
    fixtures/              # synthetic, no secrets/production PII
  internal/...             # Go domain/repository tests berdampingan kode
  tests/integration/       # real DB/concurrency Go suites
  docs/                    # brief dan sanitised summary
```

Raw run artifacts lokal tersimpan di ignored `.qa-artifacts/`; `.gitignore` tersedia, tetapi bukan ACL/enkripsi. Hanya evidence source/synthetic pada bootstrap ini; storage policy untuk data nyata masih gate M01. Workflow bootstrap tersedia di root Git; tidak mengunggah raw artifacts atau memakai production secrets.

## 4. Trigger dan execution loop

### Kontrak trigger

Tidak ada watcher yang memercayai perubahan tulisan `DONE` di Markdown. Authoritative event adalah **permintaan runner untuk memverifikasi paket pada working-tree/commit tertentu**. Command tersedia sekarang untuk M00/M03: `python -m qa.run --package M03 --profile local-synthetic --update-docs`. M00 juga memerlukan `--workspace <absolute-local-root>`. M05 dan paket lain belum executable; pilihan unsupported ditolak. Push/PR `be-v3` memiliki workflow bootstrap M03 + predicate regression, belum full per-module CI.

Urutan wajib per M:

1. Paket memiliki scope, dependency dan acceptance manifest yang sudah direview. Record `IMPLEMENTED_PENDING_QA` saat code siap diuji.
2. Runner membuat unique run ID, lock environment/namespace, membekukan source tree hash/commit, test manifest hash, schema/fixture/policy version dan runtime artifact identity.
3. Preflight identity/allowlist/permission/backup applicability/no-delete/outbound gate. Gagal preflight tidak boleh memulai seed atau endpoint mutation.
4. Collect expected test cases; zero collected, duplicate IDs atau required case missing = gate failure, bukan sukses kosong.
5. Jalankan shared smoke, package suite dan regression dependency yang dipilih; fixtures langsung ke server nyata untuk integration layer, bukan semua responses dimock.
6. Simpan immutable run result lengkap, per-case evidence, checksums, environment tag dan safe diagnostics. Schema version hasil apply juga dicatat.
7. Compute deliverable verification deterministik, bukan penilaian bahasa model. Renderer memperbarui hanya bagian hasil pada `Mxx.md` dan baris dashboard; scope/checklist manual tidak ditimpa.
8. Jika gagal, perbaiki dalam scope yang diizinkan, jalankan ulang dengan run ID baru; failure history tetap ada. Jangan mengurangi assertion/mark skip agar lulus.
9. Jika semua required D, regression dan shared gates lulus, set `BACKEND_VERIFIED`. Full package `VERIFIED` memerlukan external/manual gates bila ada.
10. Lanjut ke dependency-ready paket berikut dalam gelombang yang sudah diotorisasi. QA PASS tidak memicu commit/push/deploy/migration wave otomatis.

Setelah setiap M dilakukan QA; di akhir Bxx lakukan cumulative regression lagi. Jika later code mematahkan paket yang pernah lulus, dashboard latest confidence menjadi `STALE`/`REGRESSION_FAILED`, bukan tetap hijau berdasarkan run lama.

### CI dan failure semantics

- Push/PR pada `be-v3`: build/lint/unit/security scans dan applicable synthetic integration suites; manual gated workflow untuk authorized rehearsal environment. Tidak menyediakan production credential di CI default, terutama untrusted PRs.
- Local runner adalah trigger setiap M, CI verifikasi ulang commit. Jika push menggabungkan beberapa M, semua paket terdampak tetap diuji; tidak menunggu push untuk QA per paket.
- Registry dependency memuat module schema/policy/consumer relations, bukan hanya file-path matching. Perubahan identity/policy/platform/DDL memicu full implemented-core regression.
- Policy/DB contract changed → invalidate dependent previous evidence. Unknown impact → full implemented suite.
- Test timeout, collection error, missing fixture/credential, interrupted process dan absent result bukan PASS; exit nonzero, status ERROR/BLOCKED/INTERRUPTED dengan sebab.
- Shared runner lock mencegah dua QA jobs menulis namespace sama; workers punya namespace/IDs/port unik bila execution parallel kelak diotorisasi. Tidak memerlukan multi-agent untuk mengimplementasikan plan ini.
- Retries boleh untuk diagnosis/known infra transience dengan batas; fail→pass otomatis ditandai FLAKY. Required flaky case tetap tidak meluluskan gate sampai diperbaiki dan clean rerun lulus tanpa hidden retry.
- Baseline redacted failure tetap retained. Build/coverage artifact yang hilang tidak dianggap 0-error success.

Pytest/Playwright menyediakan test selection dan dukungan CI; orkestrasi per-paket, score renderer dan gate di atas harus dibangun oleh proyek. Referensi: [Pytest plugin](https://playwright.dev/python/docs/test-runners), [Playwright CI](https://playwright.dev/python/docs/ci-intro).

## 5. Shared gates QG01–QG08

| Gate | Bukti wajib | Fail condition |
|---|---|---|
| QG01 Environment & authority | Verified DB/server identity, allowlisted namespace, purpose, actor/role, approval applicable | Source=target, unknown live endpoint, real operation tanpa izin |
| QG02 Data preservation | Runtime/migrator permissions, reviewed SQL/trigger/job/session dependencies, existing-row invariants | DELETE/TRUNCATE/DROP/reset/--clean, cascade/purge, overwrite history |
| QG03 Build & test integrity | Reproducible Go/native tooling, full test collection, hashes, no missing evidence | Failed build, zero/missing required cases, fabricated/mock-only integration success |
| QG04 Security boundaries | Context/field authorization, validation, secret/log redaction, TLS, CSRF/egress per scope | Critical auth leak/tamper/fail-open remains |
| QG05 DB correctness & regression | Real constraints/transactions/history/idempotency, required dependency suites | Partial writes, orphan/cross-Class state, broken earlier critical flow |
| QG06 Backup & restore | Fresh scope-bound backup, actual restored validation for migration/rehearsal | Missing/stale/corrupt/unrestored artifacts or invalid resume scope |
| QG07 Side-effect & artifact isolation | Provider sinks, private file fixtures, no production recipients/secrets in report | Email/AI/cron real side effect or PII/signed URL/credential artifact leak |
| QG08 Evidence & release boundary | Source/test/schema/contract hashes, reviewer/manual gates, separate ownership/deploy state | Stale evidence reused, external/frontend pending hidden as PASS, auto-cutover from tests |

Shared gates tidak menambah poin ke numerator D. Mereka adalah veto: coverage 100% tetapi gate mandatory gagal tetap **NOT_VERIFIED**. Applicability dibekukan sebelum run; read-only M00 tidak memerlukan QG06 migration backup, tetapi alasan N/A tercatat dan harus berubah ketika scope menambah writes. N/A bukan PASS dan tidak boleh dipakai menghilangkan mandatory domain criteria.

## 6. Acceptance manifest dan metrik capaian

### Traceability

Setiap row `Mxx-Dnn` di brief adalah satu acceptance unit dengan semua klausa wajib. Mapping `Mxx-Tnn` adalah suite/case-family, **bukan bukti sebuah executable test sudah ada**. Pada implementation kickoff, pecah menjadi case IDs stabil seperti `M05-T03-atomic-rollback`, `M05-T03-foreign-role` dan sertakan layer, input fixture, expected status/state, evidence type serta `mandatory=true`.

Mapping manifest mencatat scope version, D→all required case IDs, manual evidence IDs, gate applicability dan regression dependencies. Tidak boleh menambah test bernama baru tanpa memetakannya atau mengklaim suite lulus dari sebagian cases. Satu case boleh mendukung beberapa D, tetapi denominator berdasarkan D yang unik, bukan test count.

### Perhitungan

- **Deliverable verification coverage** = jumlah mandatory D yang semua required tests dan evidence-nya valid/lulus ÷ seluruh mandatory D dalam scope frozen × 100%.
- **Test pass rate** = PASS ÷ seluruh required expected cases × 100%; FAIL/ERROR/BLOCKED/NOT_RUN/SKIPPED/FLAKY tetap di denominator. Sebelum manifest granular dibekukan atau belum ada run, tampilkan N/A dan expected/collected counts, bukan fabricated pass percentage.
- **Go code coverage** = hasil native Go coverage untuk measured packages dan mode yang dicatat; terpisah dari dua metrik di atas. Tidak menjumlahkan persentase tiap modul secara rata-rata untuk mengklaim overall code coverage.
- **Release readiness** = gate boolean + unresolved external/manual requirements; tidak disimpulkan dari rata-rata coverage.

Contoh hipotetis: 5 D, 4 fully verified dan satu belum ada DB evidence → 80% deliverable coverage. Seluruh 12 tests yang dijalankan bisa PASS, tetapi jika manifest mengharuskan 15 cases, pass rate adalah 12/15 = 80%, bukan 100%. Ini contoh aritmetika, bukan hasil QA LMS.

Awal dokumen: M00–M18 memiliki total **97 D** planned, 0 verified tercatat; QA seluruh paket NOT_RUN. Nilai 0% berarti belum ada verification evidence dalam sistem ini, bukan pengukuran kualitas code. M19 denominator UNFROZEN: lima gerbang awal tidak mencakup seluruh fitur optional, sehingga coverage N/A sampai satu feature dipilih dan cases granular disetujui.

Scope approved berubah → version baru, baseline denominator baru, retained old run history dan reviewer/reason. Jangan menurunkan denominator saat test gagal. Manual sign-off tidak boleh dibuat oleh test; runner hanya memverifikasi evidence identity/scope/hash dan keberadaannya dari authorized reviewer.

### Status model

| Dimensi | States |
|---|---|
| Implementation | NOT_STARTED, PARTIAL_REFERENCE, IN_PROGRESS, IMPLEMENTED_PENDING_QA, BACKEND_VERIFIED |
| QA run | NOT_RUN, RUNNING, PASS, FAIL, ERROR, BLOCKED, INTERRUPTED, FLAKY |
| Evidence currency | CURRENT, STALE, REGRESSION_FAILED |
| Full package | NOT_VERIFIED, VERIFIED, BLOCKED_EXTERNAL, DEFERRED |
| Migration | NOT_EXECUTED, BACKFILLED, VALIDATED, FAILED, QUARANTINED, operator-reviewed states master plan |
| Deployment/owner | NOT_DEPLOYED_BY_THIS_WORK, DEPLOYED_VERIFIED; LEGACY/FENCED/V3/HOLD separately |

M03-CP05–06 menambahkan `qa.postgres_lab --worker-journey`; M05-CP03–04 melalui `--participation-journey`; M06-CP01–02 melalui `--file-journey`; M06-CP03 + M07-CP01 melalui `--content-journey`; M07-CP02 melalui `--content-safety-journey`; M07-CP03 melalui `--content-contract-journey`; bounded M08-CP01–04 melalui `--submission-journey`; M15-CP01/02 fixture preflight/provenance melalui `--migration-foundation-journey`, dan CP03/04 mapping/shadow backfill melalui `--migration-backfill-journey`. Setiap schema installment memiliki backup/restore proof sendiri. Native tagged DB race dijadwalkan di job Linux fresh-cluster; definisi workflow bukan bukti eksekusi remote. Historical PARTIAL_REFERENCE M00 bukan fresh QA pass. M02 browser/security companion, M14 frontend integration, M16 operator approvals, M17/M18 frontend/live rollout memerlukan evidence terpisah. Backend-first bukan alasan menandai full release VERIFIED.

## 7. Environment, fixtures dan no-delete

### Profiles yang direncanakan

| Profile | Data/akses | Batas |
|---|---|---|
| `local-synthetic` | Synthetic users/Class, isolated local PostgreSQL dan fake object/provider services | Default safe automation; tanpa production connection/credential |
| `staging-synthetic` | Explicit approved isolated rehearsal namespace di staging | Tidak existing public/operational candidate; backup sebelum schema/data migration |
| `staging-rehearsal` | Restricted authorized source snapshot/imported data | Manual scoped approval, verified backups, private artifacts, outbound disabled |
| `live-readonly` | Explicit approved inventory/release observation | Tidak default, tidak seed/mutate/exploit/load test; operator checks only |
| `pilot-ops` | Scope-limited real transition checks | Approval/fence/backup gates; runner observasi/evidence, bukan auto-flip |

Environment config tidak dipercaya hanya karena `ENV=staging`. Cocokkan allowlisted project/database/server fingerprint dan target namespace di runner serta Go API; metadata pembuktian environment tidak diekspos sebagai diagnostic public. Unknown fingerprint abort sebelum mutation.

Fixtures minimum: site admin, Teacher A/Teacher B, Student A/Student B, user Teacher A+Student B, facilitator, disabled user, invited account, ended/suspended enrollment; multiple Classes dengan labels sama, multiple teachers, foreign activity/grade/file IDs, legacy duplicate/orphan/shared-score cases, timezone/deadline boundaries dan provider failures. Semua orang/email/file sintetis; email sink wajib.

No-delete berlaku pada tests juga. Tiap run menggunakan namespace/schema atau immutable fixture namespace baru dengan ID unik; retain rows/artifacts dan mark retired/restricted setelah selesai. Jangan `flush`, truncate, test database reset, DROP schema atau session/outbox purge sebagai teardown. Transaction rollback untuk uncommitted test writes boleh menjadi isolasi, tetapi tidak menggantikan real committed/retry behavior tests. Snapshot source/history tidak boleh hilang lewat UPDATE tanpa before-image/revision.

Backup scope minimum mengikuti master plan; setiap schema/data migration termasuk fixture schema/bootstrap run dibackup sebelum dijalankan, dapat memakai manifest empty pre-state untuk target baru yang benar-benar kosong. Resume yang input/schema/owner/checksum-nya berubah memerlukan fresh run/backup. No in-place restore: restore ke DB baru kosong, network/cron off, review executable dump objects.

Pertumbuhan retained test schemas/objects/backups dimonitor; hentikan tests bila kapasitas tidak aman, bukan menghapus data untuk membebaskan ruang. Private clone/synthetic test dataset tidak pernah dipromosikan menjadi operational candidate setelah dicampur test writes.

Pada Supabase, grants/exposed schemas dan RLS harus diperiksa terpisah. Private Go/landing/ops schemas tidak diekspos ke Data API; browser/service keys bukan contextual policy. Default platform bisa berubah sehingga M00 merekam actual configuration, bukan asumsi dari versi library. Referensi: [securing API](https://supabase.com/docs/guides/api/securing-your-api), [perubahan exposure default](https://supabase.com/changelog/45329-breaking-change-tables-not-exposed-to-data-and-graphql-api-automatically).

Restore checks mencatat actual PostgreSQL/extension versions; jangan berasumsi clause pin extension berlaku di managed environment. [Catatan Supabase extension pinning](https://supabase.com/changelog/extension-version-pinning-ignored). Tidak ada provider config yang diubah saat membuat dokumen ini.

Database dump tidak mencakup object bytes R2/Storage. Checksums/schema/count validation perlu object evidence terpisah, dan restore baru membuktikan backup usable. Referensi: [Supabase backups](https://supabase.com/docs/guides/platform/backups), [PostgreSQL pg_dump](https://www.postgresql.org/docs/17/app-pgdump.html).

## 8. Artifact dan pembaruan Markdown

Minimum immutable run record nantinya:

```text
run_id, package_ids, batch_id, scope_version, started_at, ended_at
source_commit OR dirty_tree_hash, tested_artifact_hash, test_manifest_hash
schema_version, migration_checksum, fixture_version, environment_fingerprint, namespace
case_results: id, layer, status, duration, evidence_refs, retry_history, sanitized_failure
deliverables: id, required_case_ids, manual_evidence_refs, verified, blocking_reasons
gates: id, applicable, disposition, reason, approval_ref
coverage: verified_D, total_D, expected_tests, collected_tests, passed_tests, go_coverage_ref
backup_refs, migration_run_refs, owner_before_after_if_applicable
dependency_regression_results, unresolved_external_gates, reviewer_refs, next_checkpoint
```

Tidak ada sample result yang berpura-pura run nyata. Raw JUnit/JSON/log/trace tersimpan restricted; Markdown versioned hanya summary sanitised dan opaque evidence refs. Run ID plus tested hashes mencegah report dari source/environment lain dipakai sebagai bukti.

Playwright traces/screenshots dapat berisi credentials dan data sensitif; jangan otomatis meng-upload traces rehearsal ke public CI artifacts. API-only tests tidak membutuhkan screenshot. Browser traces kelak default synthetic-only dan private access/redaction. Lihat [peringatan artifact pada Playwright CI](https://playwright.dev/python/docs/ci-intro).

Renderer harus atomic dan idempotent, mempertahankan human decisions serta all prior run records. Stale/partial/forged report ditolak. Update `Mxx.md` dan dashboard memakai source run yang sama; jika crash di tengah render, status dianggap stale sampai consistency check/rerender selesai. Schema perubahan report versioned agar old evidence masih dapat dibaca.

## 9. Acceptance QA harness sendiri

- [ ] Semua M/D/T IDs unik, links dan batch membership valid; dependency graph tidak melompati required integration closure.
- [ ] Test fixture results PASS/FAIL/ERROR/SKIP/BLOCKED/INTERRUPTED/FLAKY/zero-collected dihitung benar.
- [ ] Satu D dengan dua required cases tidak PASS jika hanya satu case executed; missing manual approval tetap blocked.
- [ ] Wrong hash/namespace/commit dan stale dependency evidence invalidate green report.
- [ ] Wrong environment/role/backup gate berhenti sebelum fixture writes.
- [ ] No-delete termasuk dependency session destroy/pruning, cron, ORM cascade, report cleanup dan storage lifecycle.
- [ ] Negative permissions diuji dengan real runtime DB role, bukan superuser yang menutupi konfigurasi salah.
- [ ] FK/same-Class constraints, unique keys dan atomic transactions diuji pada actual PostgreSQL; SQL creation idempotence diuji, bukan asumsi sintaks.
- [ ] External calls di luar sink/allowlist dihentikan; request/trace/log secrets tidak bocor.
- [ ] Runner rerun/crash/parallel-environment locks, deterministic renderer dan checkpoint continuation teruji.
- [ ] CI/local command contract tersedia dan documented sebelum automation disebut aktif.
- [ ] Tidak ada test yang memanggil deployment/cutover atau copy production tanpa separately approved operational run.

## 10. Checkpoint ketika limit/sesi terputus

Simpan package/subtask, last edited paths, source hash, latest completed QA run, failing cases, migration receipts, next safe action dan approvals outstanding. Tidak memerlukan prompt per M: satu scope gelombang dapat dilanjutkan sesuai dependencies dan gates. Tetapi tool/usage/access limits bisa menghentikan sesi; tidak ada janji proses terus hidup setelah session berakhir.

Saat resume: baca README/status/brief, periksa worktree user tanpa reset, cocokkan run hashes, ulangi relevant preflight dan lanjut checkpoint. Jangan mulai ulang migration dari nol atau menganggap command yang timeout pasti belum commit. Wajib cek receipt sebelum retry.
