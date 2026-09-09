# CP11–CP15 — usulan kelanjutan backend Go

**Revisi pelacakan 9 September:** dokumen ini kini referensi alias historis, bukan antrean utama. Capaian diikuti lewat [M → CP](../MODULE_CHECKPOINT_MAP.md) dan module briefs. CP11/CP12 yang sudah diuji dipetakan ke M04-CP01–02 / M05-CP02. CP13 lama menjadi **M03-CP03–06**; CP14 menjadi M00-CP02–04 + M01-CP02–04 dan M02-CP01 handoff; CP15 menjadi M06-CP01–02 + M07-CP01–02. Checklist/scope rencana lama di bawah dipertahankan sebagai konteks, tidak otomatis dianggap selesai atau memicu implementasi.

Status terbaru: **CP11–CP12 VERIFIED_LOCAL_SLICES**, masing-masing 4/4 deliverable lokal; [hasil dan batas evidence](2026-09-08-cp11-cp12.md). CP13–CP15 belum dijalankan. Paket Mxx tetap acceptance canonical; checkpoint adalah potongan pekerjaan berdasarkan dependency, bukan penggantinya. Frontend tetap belakangan.

**Riwayat kickoff 8 September:** CP11 semula BLOCKED_DECISION pada pilihan transport; kelanjutan CP11–CP12 memakai bridge untuk slice lokal sesuai [acceptance](CP11-CP12_ACCEPTANCE.md). Global owner tetap hardened legacy; real adapter/provider belum verified. Status awal NOT_RUN di uraian rencana di bawah adalah baseline, bukan hasil terbaru. Tidak ada deployment/ekspor dari checkpoint ini.

Rekomendasi: **CP11 → CP12 → CP13** untuk kemajuan kode. CP14 dapat dipersiapkan lebih awal bersama operator tetapi tindakan terhadap provider/shared DB harus disetujui terpisah. CP15 dimulai setelah authority dan runtime matang; tidak membutuhkan data production.

## CP11 — Identity authority dan session lifecycle (M04)

- Capaian: backend tidak lagi bergantung pada session yang dibuat fixture. Satu pemilik identity dan kontrak penerbit session dinyatakan eksplisit.
- Coverage kerja: inventori mekanisme login legacy; ADR issuer/bridge yang diverifikasi, ID mapping stabil, expiry/replay/audience/purpose; provisioning/invite/reset sesuai authority yang disetujui; rotate/revoke/disable, last-admin invariant. Jangan menganggap email saja sebagai bukti identity atau menyalin session live.
- Checklist: [ ] bekukan ADR dan manifest; [ ] implement Go identity/session commands; [ ] pisahkan self-profile dari authority; [ ] atomic single-use/rotation; [ ] idempotency/audit dan safe logs; [ ] update OpenAPI serta M04 evidence.
- QA: Playwright Python actor contexts untuk invalid/expired/replayed token, disabled/revoked user dan authority injection; Go/DB concurrent token consumption, last-admin dan session rotation; dependency outage fail closed. Browser HttpOnly/SameSite/CSRF tetap external integration gate bila UI belum dikerjakan.
- Dependency/input: keputusan issuer dan siapa yang boleh provision akun. Jika belum disetujui, selesaikan ADR/negative contract terlebih dahulu, jangan buat login bypass. **Large**.
- Exit: slice lifecycle dengan evidence nyata, bukan full M04 jika browser/provider gates belum terpenuhi. Deliverable coverage awal **NOT_RUN / N/A** sampai manifest slice frozen.

## CP12 — Capability dan lifecycle Class lengkap (M04/M05)

- Capaian: hak Teacher/Student mengikuti Class dan capability, bukan percabangan role yang tersebar; perubahan akses aman ketika request bersamaan.
- Coverage kerja: capability matrix terpusat; keputusan Teacher boleh mengelola peserta sesuai master plan; enrollment/role/episode terpisah; suspend/reactivate/end/rejoin dan closed/archive; optional grouping/personal mentor hanya sesuai scope frozen. Content-readiness publish dicatat sebagai dependency M07, tidak dipalsukan.
- Checklist: [ ] policy ancestry resolver; [ ] izin setiap command; [ ] version/state transitions; [ ] fence perubahan account/session/role terhadap mutasi in-flight; [ ] history tetap utuh; [ ] migrasi additive ber-backup; [ ] error contract dan matrix tes.
- QA: Playwright cross-Class/nested ID/multi-role/ended/suspended access; Go/DB barrier-based concurrent revoke-versus-edit, last-teacher transitions, enrollment races dan retries; cek audit/outbox/history bukan hanya HTTP status.
- Dependency: CP11 authority contract + CP09 substrate. Perubahan policy tidak boleh diam-diam memperluas Teacher authority. **Large**.
- Exit: matrix approved dan state/denial tests lulus; broad M05 tetap partial jika groups/retake/content prerequisites belum lengkap. Coverage awal **NOT_RUN / N/A**.

## CP13 — Runtime non-lab, migration runner dan worker (M03)

- Capaian: business handler dapat dipasang pada entry point aplikasi melalui konfigurasi aman, tanpa seed atau DDL saat startup; operator migration terpisah; worker outbox benar-benar berjalan pada provider stub lokal.
- Coverage kerja: pindahkan fixed fixture migration ke versioned additive operator path, backup/restore/checksum/lock receipts dan schema compatibility; startup/readiness dependency checks; restricted role/TLS integration; bounded worker lease/retry/shutdown dan retained failure history; konsistensi API/OpenAPI.
- Checklist: [ ] operator/runtime roles terpisah; [ ] backup sebelum setiap migration run; [ ] replay aman/no destructive down; [ ] wiring business API gated; [ ] worker consume/ack fencing + consumer dedupe; [ ] CI PostgreSQL + Playwright slice; [ ] current hash/artifact evidence.
- QA: restart/crash/commit-uncertainty, concurrent runner, stale backup/checksum mismatch, worker redelivery/expired lease/provider outage, pool pressure, migration/runtime permission denial. Jalankan Linux race juga pada tagged integration ketika harness mendukung; existing race default bukan bukti DB race suite.
- Dependency: CP11–12 contract, verified local backup. Shared deployment **tidak termasuk**. **Large**.
- Exit: reusable local operator/API/worker lifecycle tested, no runtime DDL; container/remote CI evidence hanya PASS jika benar dijalankan. Coverage awal **NOT_RUN / N/A**.

## CP14 — Gate operasional production/staging dan backup nyata (M00/M01; M02 handoff)

- Capaian: identitas sumber/target dan keamanan backup sungguhan terverifikasi sebelum rencana copy. Kedua konfigurasi yang tersedia saat ini adalah staging; **production masih unknown**.
- Coverage kerja: owner/operator/reviewer, actual deployed SHA dan production reference, provider exposed schemas/RLS/grants/jobs/storage/capacity; scoped least-privilege provisioning; protected destination dan independent key custody; backup streaming/size budget, custom+SQL dari snapshot yang sama, object bytes dan restore reconciliation.
- Checklist: [ ] identity source/target berbeda terbukti; [ ] approved scope/account/CA; [ ] host ACL + disk + key recovery verified; [ ] target pre-state backup; [ ] source read-only snapshot; [ ] restore ke target isolasi baru, tidak menimpa staging; [ ] security disposition legacy dengan approval terpisah.
- QA: wrong-target/stale/missing-object/tamper/wrong-key/permission failures block; snapshot row/relationship reconciliation; tidak replay active sessions, emails/webhooks/jobs/provider credentials. Review recovery dan data minimization bersama pemilik data.
- Dependency: kredensial/referensi production yang benar, provider settings dan izin tertulis untuk setiap operasi. Keberhasilan synthetic CP08 bukan approval ekspor. **Large + external inputs**.
- Exit: operational evidence disetujui; data copy/transformation M15/M16 tetap langkah berikutnya, bukan otomatis setelah backup. Coverage awal **BLOCKED_INPUT / N/A**.

## CP15 — File/context dan content skeleton (M06/M07)

- Capaian: fondasi materi Class tanpa frontend, grade, completion atau learner state dicampur ke definisi content.
- Coverage kerja: section/activity/resource ownership, draft/publish/version; opaque file IDs dan server-resolved Class context; upload validation/size/type, quarantine/scan contract, access-gated download. Gunakan isolated object fixtures, bukan storage live.
- Checklist: [ ] freeze satu vertical slice content/file; [ ] additive schema + backup; [ ] contextual Teacher edit/Student published read; [ ] fail-closed file access; [ ] retained object/version history; [ ] API contract; [ ] M06/M07 partial status.
- QA: Playwright foreign Class/file ID, draft visibility, forged MIME/path/name, oversize and expired link; Go/DB/object consistency, interrupted upload/retry and publish races. Tidak menghapus orphan fixtures; reconciliation/report atau quarantine retained.
- Dependency: CP11–13 identity/capability/runtime; keputusan scanning/storage adapter. CP14 hanya diperlukan bila akan menyentuh shared environment. **Large**.
- Exit: satu content/file journey tested, bukan seluruh content/assignment/grading LMS. Coverage awal **NOT_RUN / N/A**.

## Kontrak penyelesaian setiap checkpoint

Freeze scope dan deliverable→test sebelum implementasi. Setelahnya: Go unit/vet/build + Playwright API + real isolated DB bila relevan + regression dependencies; simpan run ID/source hash, exact collected cases, negative assertions dan measured statement coverage. Bedakan **test pass rate**, **code coverage**, dan **fully verified deliverables**; jangan merata-ratakan persentase berbeda denominator. Shared gates yang belum tersedia tetap BLOCKED, bukan SKIP/PASS. Akhiri dengan capaian, sisa gate dan brief next step; commit/push `be-v3` hanya jika diminta.
