# Rekonsiliasi status backend v3 — 24 September 2026

Dokumen ini mencatat koreksi status lintas brief. Ia tidak mengubah kode, schema, data, atau hasil QA. Bukti eksekusi tetap berada pada manifest, laporan checkpoint, dan artefak run yang dirujuk di bawah.

## Aturan membaca status

- `CP lokal PASS` berarti scope dan kasus yang dibekukan lulus pada PostgreSQL sintetis loopback. Itu bukan persentase effort atau penerimaan seluruh modul.
- `D canonical verified` mensyaratkan seluruh klausa acceptance, integrasi, policy/owner, lingkungan bersama, dan quality gates yang berlaku. Posisi M00–M18 tetap **0/97 D fully verified**.
- Paragraf bertanggal lama, tabel `Status awal`, dan contoh rencana adalah histori. Posisi terbaru mengikuti dokumen ini, [peta CP](MODULE_CHECKPOINT_MAP.md), [dashboard](IMPLEMENTATION_STATUS.md), serta laporan checkpoint terbaru.
- Hash source dan backup mengikat bukti pada run tertentu. Perubahan kode berikutnya membutuhkan QA ulang sebelum status PASS untuk source baru diklaim.

## Koreksi utama

| Area | Bukti saat ini | Koreksi |
|---|---|---|
| M09 | [CP04](checkpoints/2026-09-21-m09-cp04.md) | 4/4 CP lokal sintetis; canonical 0/5 D. |
| M10 | [CP03](checkpoints/2026-09-22-m10-cp03.md) | 3/4 CP lokal sintetis; CP04 belum; canonical 0/5 D. |
| M13 | [review CP01–02](checkpoints/2026-09-24-m13-cp01-cp02-evidence-review.md) | 2/4 CP memiliki bukti lokal sintetis; provider nyata, AI, closure dan canonical 0/5 D masih terbuka. Status `NOT_STARTED` sebelumnya salah untuk dua slice ini. |
| M15 | [CP05](checkpoints/2026-09-14-m15-cp05-m11-cp01.md) | 5/5 slice sintetis lokal; source nyata dan canonical 0/6 D belum diuji. |
| Database | [baseline lokal](LOCAL_SYNTHETIC_BASELINE.md) | Fixture PostgreSQL lokal menjadi baseline pengembangan sementara. Backup/restore lokal tidak menggantikan backup live/staging atau memberi izin copy production. |

## Batas penting bagi tim frontend

Backend v3 saat ini backend-first: beberapa domain Go yang diuji di `cmd/identity-lab` belum dipasang pada `cmd/api` bersama. Kontrak domain Class, enrollment, role/capability, grade, completion, file dan histori dapat dipakai untuk menyusun brief frontend; URL/DTO lab, cookie browser, OpenAPI gabungan, serta perilaku UI produksi masih memerlukan kontrak integrasi dan uji lintas tim. [Catatan handoff frontend](FRONTEND_HANDOFF_STATUS.md) merinci batasnya.

Brief yang dihasilkan tim frontend pada komputer lain belum tersedia di workspace ini; karena itu belum ada klaim bahwa dokumen frontend tersebut sudah direkonsiliasi baris demi baris.

## Gate yang tetap terbuka

Identitas source production/deployed SHA (M00), backup+restore nyata (M01), containment keamanan legacy web/backend (M02), academic policy, shared runtime, provider dan object store nyata, remote CI, real legacy mapping, rehearsal M16, pilot M17, dan rollout M18 belum diterima. Kedua konfigurasi `.env` yang pernah diperiksa tidak boleh diasumsikan sebagai production. Jangan memakai data live untuk QA sampai identitas, akses, perlindungan dan otorisasi terbukti.

## Status publikasi

Saat review ini branch lokal `be-v3` berada pada commit `2cbbcbf` dengan perubahan dan file baru M09/M10 yang belum commit/push. Dokumen ini dan artefak QA lokal juga belum dipublikasikan oleh review ini. Artefak terbaru berada di `D:\Projects\lms-v2\lms-v3-qa-artifacts\20260922T125746Z-postgres-lab-2e3def603dd8`; dump dan fixture mentah tidak boleh masuk Git.
