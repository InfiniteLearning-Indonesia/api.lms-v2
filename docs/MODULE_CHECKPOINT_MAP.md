# Peta capaian M00–M19 dan checkpoint di dalam modul

Revisi **9 September 2026**. Ini restrukturisasi pelacakan pekerjaan, bukan perubahan business scope atau hasil QA baru. Backend tetap Go, frontend belakangan, target v3 tetap staging yang diproteksi, live tetap melayani sampai cutover yang disetujui. Tidak ada eksekusi kode, DB copy/migration/deploy atau commit/push yang dipicu dokumen ini.

## Cara membaca pekerjaan mulai sekarang

**M = capaian produk/platform yang ingin selesai. CP = bagian kerja untuk mencapai M. D = acceptance requirement yang membuktikan M memenuhi kontrak.** Gelombang B hanya pengelompokan beberapa M untuk satu instruksi; bukan denominator progress baru.

Contoh posisi saat ini: **M03 — 2 dari 6 bagian teruji lokal; full D 0/5; berikutnya M03-CP03 migration runner**. Ini jauh lebih jelas daripada hanya “CP12 sudah selesai”. Full D yang masih nol tidak menghapus bukti dua bagian yang sudah diuji; syarat runtime/CI/operasional lengkap memang belum seluruhnya tersedia.

- ID baru selalu `Mxx-CPnn`, misalnya `M03-CP03`; nomor CP diulang dalam setiap M dan tidak dipakai tanpa nama M.
- CP01–CP12 lama tidak dihapus/diubah nomor, dan case IDs/receipts/hash lama tetap utuh. Pemetaan di bawah menjadi alias dokumentasi.
- CP13–CP15 lama masih rencana, sekarang dibagi ke CP per M. **CP13 lama bukan M13**; sebagian besarnya pekerjaan M03. Demikian pula CP11 lama adalah identity, sedangkan M11 adalah attendance.
- Satu CP boleh membutuhkan evidence modul lain, tetapi mempunyai satu pemilik pekerjaan. Shared evidence direferensikan; kode/worker/migration tidak dibuat dua kali.
- CP berikutnya boleh memakai kontrak/slice dependency yang sudah diuji sambil full module dependency menunggu integration/ops gate. Jangan membuat siklus “semua M04 harus selesai sebelum mulai M05” padahal closure M04 memerlukan real Class M05.

## Posisi seluruh M

| Modul utama | Jumlah CP rencana | Bagian teruji lokal | Sisa bagian | Fokus berikutnya |
|---|---:|---:|---|---|
| [M00 — Environment & baseline](modules/M00.md) | 4 | 1/4 | 3 tertahan_input | M00-CP02 setelah referensi production/deployed SHA tersedia; M03-CP03 dapat berjalan lokal sambil menunggu. |
| [M01 — Backup & restore](modules/M01.md) | 4 | 1/4 | 1 parsial; 1 tertahan_input; 1 rencana | M01-CP02 dapat menyiapkan prosedur; pelaksanaan backup nyata menunggu M00. |
| [M02 — Containment legacy](modules/M02.md) | 4 | 0/4 | 1 parsial; 3 tertahan_input | Tuntaskan M02-CP01 lalu pilih hotfix legacy dengan otorisasi repo/deployment yang tepat; ini gate real identity M04. |
| [M03 — Platform Go & runtime](modules/M03.md) | 6 | 2/6 | 4 rencana | M03-CP03 → M03-CP04 → M03-CP05 → M03-CP06; ini pemecahan CP13 lama. |
| [M04 — Identity & capability](modules/M04.md) | 5 | 2/5 | 1 tertahan_input; 2 rencana | M04-CP03 memerlukan owner legacy yang hardened; jangan mengganti issuer sintetis dengan trust ke legacy yang belum aman. |
| [M05 — Class & participation](modules/M05.md) | 5 | 2/5 | 3 rencana | M05-CP03 untuk grouping/mentor, lalu CP04; CP05 menunggu content/learner-state yang relevan. |
| [M06 — Private files & egress](modules/M06.md) | 4 | 0/4 | 4 rencana | M06-CP01 setelah runtime/context siap; penggalan file dari CP15 lama. |
| [M07 — Content & activity](modules/M07.md) | 4 | 0/4 | 4 rencana | M07-CP01 sesudah M06 asset/context minimum; penggalan content dari CP15 lama. |
| [M08 — Submission](modules/M08.md) | 4 | 0/4 | 4 rencana | M08-CP01 setelah content/file/enrollment contracts siap. |
| [M09 — Gradebook & rubric](modules/M09.md) | 4 | 0/4 | 4 rencana | M09-CP01 setelah academic policy disepakati dan submission contract tersedia. |
| [M10 — Completion & credentials](modules/M10.md) | 4 | 0/4 | 4 rencana | M10-CP01 setelah evidence dan grade release contracts stabil. |
| [M11 — Attendance & permits](modules/M11.md) | 4 | 0/4 | 4 rencana | M11-CP01 dapat dikerjakan setelah Class/file stabil tanpa menunggu seluruh gradebook. |
| [M12 — Logbook & mentoring](modules/M12.md) | 4 | 0/4 | 4 rencana | M12-CP01 setelah grouping/mentor dan file contracts siap. |
| [M13 — Jobs, notifications & AI](modules/M13.md) | 4 | 0/4 | 4 rencana | M13-CP01 memakai worker dasar M03-CP05, bukan mengimplementasikan worker dua kali. |
| [M14 — Reporting & API/performance](modules/M14.md) | 4 | 0/4 | 4 rencana | M14-CP01 setelah read-model domain yang hendak dipublikasikan stabil. |
| [M15 — Migration tool & mapping](modules/M15.md) | 5 | 0/5 | 5 rencana | M15-CP01 boleh disiapkan lokal setelah reusable operator foundation; actual copy tetap gated. |
| [M16 — Production→staging rehearsal](modules/M16.md) | 4 | 0/4 | 1 tertahan_input; 3 rencana | M16-CP01 menunggu production identity, M01 dan M15 siap; tidak menganggap env staging sebagai source production. |
| [M17 — Pilot & ownership switch](modules/M17.md) | 4 | 0/4 | 1 tertahan_input; 3 rencana | M17-CP01 setelah rehearsal, owner bridge dan frontend integration readiness tersedia. |
| [M18 — Rollout & operations](modules/M18.md) | 3 | 0/3 | 3 rencana | M18-CP01 sesudah actual pilot accepted; setiap wave tetap punya izin dan backup sendiri. |
| [M19 — Optional extensions](modules/M19.md) | N/A (3 template) | N/A | Scope feature belum dipilih | Pilih satu fitur optional dahulu; denominator tidak dibekukan oleh restrukturisasi ini. |

Total rencana M00–M18 saat restrukturisasi: **80 potongan CP** dengan ukuran berbeda; **bukan jumlah sesi atau prompt yang diwajibkan**. M19 tidak masuk denominator. Satu instruksi dapat mencakup beberapa CP dependency-ready atau satu rangkaian M. Tidak menghitung persentase proyek dari rata-rata/jumlah CP karena ukuran berbeda dan evidence lintas M dapat dipakai bersama.

Angka bagian teruji adalah **rekonsiliasi scope bukti existing**, bukan hasil menjalankan suite baru bernama Mxx-CPnn. Status lengkap semua M00–M18 tetap belum verified; 97 canonical D dipertahankan (M02/M15 masing-masing 6, lainnya 5). M19 memiliki 5 gerbang template tetapi feature scope/denominator belum frozen.

## Pemilik source of truth

| Dokumen/evidence | Tanggung jawab |
|---|---|
| [ImplementationPlan.MD](../../../ImplementationPlan.MD) | Business model, invariant, prioritas, batas data, dependency dan DoD utama; §8.3/§14.5 mengadopsi pelacakan M → CP |
| [modules/Mxx.md](modules/M03.md) | Daftar CP milik M, target D, dependency, exit/QA, progress bagian dan next action; tabel canonical D tetap acceptance |
| Dokumen peta ini | Ringkasan per M, alias CP lama dan urutan lintas M; projection dari module briefs, bukan authority test baru |
| [IMPLEMENTATION_STATUS.md](IMPLEMENTATION_STATUS.md) | Dashboard status module penuh + penunjuk progress bagian + latest run/gates |
| [QA_AUTOMATION_PLAN.md](QA_AUTOMATION_PLAN.md) dan executable manifests | Cara pengumpulan/validasi evidence, exact-case checks dan evaluasi D; ID lama tetap didukung |
| Raw local `.qa-artifacts/` + sanitised checkpoint reports | Bukti apa yang benar-benar diuji, pada hash/environment mana; tidak diganti oleh checkbox dokumentasi |

Scope CP masa depan adalah decomposition awal. Sebelum coding, freeze case/contract/gate yang relevan; jika CP terlalu besar, pecah menjadi child `M03-CP03a/b` dengan alasan dan scope union tetap sama. Jangan memecah/mengecilkan acceptance untuk meluluskan test gagal. Owner/reviewer orang belum ditetapkan; pemilik M bukan nama assignee.

## Pemetaan CP lama → pekerjaan modul

| CP lama / baseline | Pemilik M dan alias baru | Batas bukti |
|---|---|---|
| Bootstrap sebelum penomoran CP | M00-CP01; M03-CP01 | Inventory/source/QA/API bootstrap lokal, bukan environment atau runtime production lengkap |
| CP01 evidence reconciliation | M00-CP01; mendukung QA M03 | Local receipt/checksum/fail-closed evidence; bukan operator approval |
| CP02 metadata/access inventory | M00-CP02–03 (parsial), mendukung M01-CP02 | Observasi staging historis; source production dan inventory seluruh dependensi masih belum lengkap |
| CP03 offline backup preflight | M01-CP01 | Predicate planner bukan backup production |
| CP04 artifact/Linux QA | M03-CP01; sisa gate M03-CP06 | Default Linux/non-root proof tersedia; container dan full remote CI terpisah |
| CP05 transaction/isolated harness | M03-CP02; mendukung M01-CP01 | Local synthetic backup/rollback; bukan worker/runtime deployed |
| CP06 backup/restore/permissions | M01-CP01; mendukung M03-CP02 | Synthetic fixture, bukan source/target live |
| CP07 security/environment handoff | M00-CP02–04 dan M02-CP01 parsial | Handoff/disposition, bukan mitigation deployed atau full audit provider |
| CP08 protected backup recovery | M01-CP01; input parsial M01-CP02 | Encryption/restore lokal; real key custody/objects/host gate belum lengkap |
| CP09 runtime persistence substrate | M03-CP02 | Restricted pool, atomic Class/audit/outbox, fixed local migration; reusable operator/API/worker belum ada |
| CP10 Class API | M05-CP01; mendukung context evidence M04 | Multi-Teacher/Student/Class/enrollment/role/episode lokal |
| CP11 identity/session | M04-CP01 | Verified synthetic issuer, session/cookie/CSRF/projection/local site-admin; real owner belum diuji |
| CP12 capability/lifecycle | M05-CP02, evidence bersama M04-CP02 | Class policy/history dan actual DB fence; bukan semua resource ancestry/content/group/retake |
| CP13 (rencana lama) | **M03-CP03–06** | Migration runner → API runtime → worker → robustness/closure; M13 nanti memakai worker ini |
| CP14 (rencana lama) | **M00-CP02–04 + M01-CP02–04**, M02-CP01 handoff | Environment/owner/protected actual backup gate, bukan otomatis full security fix/copy |
| CP15 (rencana lama) | **M06-CP01–02 + M07-CP01–02** | File/content slice, bukan seluruh file/content module |

Evidence lokal terbaru yang direkonsiliasi: [CP11–CP12 + regresi](checkpoints/2026-09-08-cp11-cp12.md), commit implementasi published **a0d296e**, tested source hash `7614e19efe1480f86deb0900bc078ae78ff4d7710efbb4603e00fc108056d280`. CP11 4/4 dan CP12 4/4 **slice lama** tidak diubah menjadi M04/M05 100%. Case test lama dan dokumen historis tetap dipertahankan. Restrukturisasi Markdown tidak menambah/mengurangi verified D atau menjalankan QA aplikasi ulang.

## Urutan kerja yang disarankan

1. **Tuntaskan inti runtime M03:** M03-CP03 → CP04 → CP05; lanjut CP06 untuk evidence robustness/CI dan closure. Bagian operator nyata tetap mengikuti gate M00/M01. Local testing dapat memakai synthetic resources yang sudah diotorisasi.
2. **Jalur keamanan/authority:** M02-CP01 lalu hotfix yang diotorisasi; M00 input/operator dan M01 proteksi/backup dipersiapkan tanpa menebak production. M04-CP03–04 baru dapat dibuktikan terhadap owner yang aman. Ini gate wajib sebelum real identity integration/rollout, bukan pekerjaan ops yang boleh dilupakan karena coding lokal maju.
3. **Lengkapi domain Class:** M05-CP03–04 untuk grouping/mentor/invited/retake, memakai M04/M05 core yang sudah tersedia.
4. **File → content → karya → nilai:** M06-CP01–03 → M07-CP01–03 → M08-CP01–03 → M09-CP01–03; tutup CP integration/closure masing-masing ketika consumer dan mapping contracts tersedia. M07 closure boleh menunggu M08 tanpa menghalangi pembangunan M08 dari M07-CP03.
5. **Fitur lanjutan:** M10 setelah grading/evidence; M11/M12 dapat dibangun dari Class/file contracts tanpa harus menunggu seluruh M09. M13 memakai worker M03; AI menunggu M08/M09. M14 mengikuti endpoint/domain yang benar-benar akan dipublikasikan.
6. **Migrasi dan rilis:** M15 local tooling sesuai schema domain → M16 actual rehearsal hanya setelah M00/M01/M02 gates → M17 pilot dengan frontend readiness nanti → M18 wave rollout. M19 tetap optional scope baru.

Ini urutan dependency, bukan kewajiban menuntaskan semua M00 sebelum satu baris Go ditulis. Sebaliknya, lulus kode lokal tidak membuka operasi production/staging yang gated. Domain yang belum dibangun tidak boleh diberi fake PASS untuk menutup integration module.

## Kontrak progress dan QA

| Status bagian | Makna |
|---|---|
| TERUJI_LOKAL | Bounded local scope sudah punya source-bound evidence; belum claim real provider/deploy/full M |
| PARSIAL | Sebagian implementasi/review/evidence tersedia; exit CP belum lengkap |
| RENCANA | Scope/dependency/exit ditulis; pekerjaan belum dilakukan. Dependency yang belum selesai tetap harus dipenuhi |
| TERTAHAN_INPUT | Ada input/authority/environment nyata yang belum tersedia; pekerjaan independen boleh berlanjut |
| BELUM_DIBEKUKAN | Template optional; tidak masuk completion denominator |
| VERIFIED (future, scope explicit) | Seluruh exit CP dan applicable gates terbukti pada scope/environment yang dinyatakan |

Setiap handoff dimulai dengan **M / CP yang dikerjakan / D yang didukung / bukti / sisa untuk menutup M / next CP**, bukan nomor CP global saja. Tampilkan empat dimensi terpisah: jumlah bagian teruji, D canonical verified, measured code coverage/test pass rate, dan migration/traffic ownership.

Closure suatu M: seluruh mandatory D dan QG applicable terbukti, required consumers/integration ada, evidence current, external/reviewer requirement terpenuhi. Tidak cukup semua CP coding selesai atau semua tests yang kebetulan dijalankan lulus. Sebaliknya, blocker eksternal harus tampak sebagai blocker, tidak membuat implementasi yang sudah ada terlihat NOT_STARTED.

Automation yang **sudah ada**: `qa.run` M00/M03, `qa.postgres_lab --identity-journey` CP08–CP12, `qa.linux_qa`, dan `qa.checkpoint_report`. **ID Mxx-CPnn baru belum menjadi parameter runner.** Jangan menjalankan `qa.run --package M04` atau mengira tulisan DONE memicu tes. M03-CP06 mencakup registry/dependency/reporting module→CP→case→D yang kompatibel dengan case IDs lama; sebelum adapter itu ada, pelaksana menjalankan suite existing yang relevan dan mencatat pemetaan secara eksplisit. Tidak ada source/manifests/test code diubah oleh restrukturisasi ini.

## Contoh satu instruksi yang cukup

- “Kerjakan **M03-CP03 sampai M03-CP05**. Jalankan QA setelah setiap CP, lalu laporkan sisa M03. Lokal sintetis, tanpa deploy/commit/push.”
- “Lanjutkan **M05-CP03** saja. Jelaskan dampaknya ke M04/M12 dan update capaian M05.”
- “Laporkan **M04**: bagian mana yang teruji, D mana yang belum tutup, dan input yang diperlukan.”

Tidak perlu puluhan prompt sesuai jumlah CP. Satu instruksi boleh membundel CP selama dependency/otorisasi cukup; kegagalan checkpoint wajib diperbaiki/diungkap sebelum melanjutkan pekerjaan yang bergantung padanya. Dokumen ini sendiri tidak mengotorisasi coding CP berikutnya, frontend, actual copy, commit/push atau deployment.

## Validasi restrukturisasi

Pemeriksaan lokal 9 September: 20 module briefs mempunyai CP unik dengan pemilik M yang benar; seluruh 97 canonical D M00–M18 dan 5 template D M19 identik dengan tabel acceptance di HEAD `a0d296e`, serta semuanya terpetakan ke CP. Ada 80 CP rencana M00–M18 dan 3 template M19 yang tidak masuk denominator. Pemeriksaan 51 dokumen (termasuk master plan workspace) tidak menemukan broken local file link, code fence tidak berpasangan atau encoding replacement. Executable source hash tetap sama dengan evidence 8 September; tidak menjalankan ulang QA aplikasi karena perubahan hanya Markdown.
