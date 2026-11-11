# Catatan kontrak backend untuk penyelarasan frontend — 24 September 2026

**Backend contract update, 31 October:** M14 CP01/02's released-grade report was accepted locally on run `20261031T111156Z-postgres-lab-d5ed91220fae`. CP03 adds a v1.1 POST page/export contract, currently implemented/unverified: exact Class/learner authority, signed session-bound cursor, 409 restart after changed release snapshot, explicit row/byte limits and audited private CSV. See [CP03 evidence](checkpoints/2026-10-31-m14-cp03.md) and `api/openapi/reporting-v1.json`. This remains opt-in synthetic lab wiring; shared runtime/frontend integration is not accepted by these backend results. Earlier September notes below preserve their original scope/date.

Tim frontend dapat memakai [ImplementationPlan](references/ImplementationPlan.MD) sebagai keputusan produk dan [peta M/CP](MODULE_CHECKPOINT_MAP.md) untuk melihat scope backend. Catatan ini memisahkan kontrak domain yang telah diuji secara lokal dari API/browser yang belum siap diintegrasikan. Brief frontend milik tim lain tidak diubah oleh dokumen ini.

**Batas review:** brief/file yang dibuat di komputer tim frontend belum tersedia di workspace ini, sehingga rekonsiliasi ini belum membandingkan isi brief frontend baris demi baris. Penyelarasan dua arah perlu dilakukan setelah artefak itu dibagikan.

## Domain yang dapat dijadikan dasar UX

- **Class adalah pusat pengalaman.** Admin membuat Class tanpa harus membuat Program/Batch; label program/cohort adalah metadata. Teacher dan Student ditetapkan per Class. Enrollment/episode partisipasi dan role/capability berbeda; user boleh punya konteks berlainan antar Class.
- **Materi/assignment punya revision; submission mengikat revision dan episode.** Resubmit mempertahankan histori. Nilai adalah grade revision dengan release eksplisit; nilai kosong, nol, nilai tampilan legacy dan AI suggestion tidak boleh ditafsirkan sama.
- **Completion adalah outcome dari evidence/rule, terpisah dari grade.** Progress bisa di-override/reopen dengan alasan dan history. Transcript adalah snapshot yang dirilis eksplisit; credential punya issue, verify, revoke dan supersede. Import historis belum boleh dianggap penerbitan credential.
- **File privat dan izin personal** memerlukan akses kontekstual; jangan memakai URL publik atau field UI sebagai sumber izin. Logbook/personal mentor dan attendance masih punya policy/closure yang terbuka.

## Yang belum boleh dianggap kontrak frontend final

M09/M10 dan beberapa domain lain baru berjalan pada `cmd/identity-lab` opt-in untuk QA; shared `cmd/api` belum me-wiring seluruhnya. Path/DTO lab, rate limit per proses, cookie dan CSRF browser, renderer HTML, OpenAPI gabungan, error/empty/loading semantics, dan real provider/object delivery belum menjadi kontrak produksi. Synthetic Playwright saat ini menguji HTTP API, bukan browser UX atau mobile. M14 harus memublikasikan dan menguji kontrak API; M17 memerlukan integrasi frontend/browser sebelum pilot.

Untuk sinkronisasi kedua tim, bekukan matriks per layar: aktor dan Class context, sumber data/command, state kosong/pending/error, authorization denial, idempotency/retry, histori, file privacy, dan perubahan setelah release/withdrawal. Cocokkan dengan source/manifest/run terbaru; simpan perbedaan kontrak sebagai isu yang diputuskan bersama, bukan mengasumsikan bahwa perilaku lab sudah final.

**Status data:** seluruh journey akademik terbaru hanya diuji terhadap PostgreSQL sintetis lokal. Database staging v3 dan data live belum direkonsiliasi atau di-cutover. [Aturan baseline lokal dan live nanti](LOCAL_SYNTHETIC_BASELINE.md).
