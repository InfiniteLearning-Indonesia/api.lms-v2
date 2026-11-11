# Brief berikutnya — retensi QA, lalu penyelesaian lokal M14

**Closure accepted 11 November:** [M14 CP04 and combined local closure](2026-11-11-m14-local-closure.md) passed on fresh run `20261111T093737Z-postgres-lab-6e4bc487c4d6`: **M14 4/4 local CPs**, 21/21 direct cases, measured load/DB/pool/dependencies and backups reviewed. The inventory → full QA → evidence → local-doc closure sequence below is complete for this source. Next is review of policy/real-data/shared/frontend inputs before selecting another lane; no M16/pilot/cleanup/Git action starts automatically. Older pending entries below describe completed or failed historical stages.

**Latest owner result:** inventory passed and full native run reached M14-CP04. [EXPLAIN parser compatibility fix](2026-11-11-m14-cp04-plan-parser-fix.md) follows a FAIL at the DB-plan probe after benchmark/API PASS. Targeted fix verification passed; **next is a fresh full run through `CONTINUE_M14_CP04_QA.cmd`**, then read final evidence before accepting CP04. No threshold relaxation, original-run relabeling or cluster reuse. Historical sandbox gate notes below do not override the complete owner inventory.

**Latest continuation:** [Readiness/handoff](2026-11-11-qa-readiness-handoff.md): 68/68 local preparation cases PASS, 14 HOLD inspected without changing decisions. Owner handoff is now one command, `portability/CONTINUE_M14_CP04_QA.cmd`; it checks inventory then invokes QA only after admission. Sandbox execution still blocked on the two scratch permissions. No extra historical deletion or M14 acceptance. The native-pilot/hook notes below describe the previous completed slice.

11 November 2026. **Selector/apply selesai; satu pilot native historis diterima dan hook serial future-only sudah aktif. 60/60 pemeriksaan lokal PASS.** [Evidence native dan hook](2026-11-11-qa-retention-native-and-hooks.md). Satu raw payload sekitar 46,5 MiB dibuang; backup/evidence utuh. Sekarang 1 DISPOSED, 51 kandidat historis di luar auto-scope, 3 KEEP, 14 HOLD dan dua scratch belum terbaca. Inventory/admission penuh dan native lifecycle source baru masih terbuka; [kebijakan utama](../QA_STORAGE_POLICY.md) tetap mempertahankan byte gates v2.

## 1. Implementasikan selector dan dry-run retensi v3

**Status: selesai untuk selector dan laporan; definisi scope/exit di bawah dipertahankan sebagai acuan.** Audit owner-terminal terhadap scratch dan penyelesaian HOLD tetap pengecualian terbuka; tidak ada pembuangan yang diotorisasi oleh laporan.

- Audit dua folder scratch dari terminal pemilik; klasifikasikan delapan cluster yang belum memiliki backup terverifikasi tanpa menganggapnya aman dibuang.
- Tambahkan selector 2 PASS + 1 gagal berdasarkan timestamp run, lintas kedua root QA. Pin/unknown/aktif/unverified tetap terlindungi dan dilaporkan terpisah.
- Buat rencana path/ukuran/keep/dispose/exceptions yang bisa ditinjau; simpan seluruh evidence dan backup. Tahap ini tidak menghapus cluster.
- QA: urutan deterministik, status campuran, source historis, pin, backup tidak valid, path keluar root/reparse, dan cluster aktif harus ditangani dengan benar.
- Exit: dry-run teruji dan daftar kandidat konkret; tidak ada klaim ruang sudah dibebaskan dari sekadar rencana.

## 2. Implementasikan dan validasi apply yang dibatasi

**Status tahap fixture: selesai untuk engine, CLI preview/apply eksplisit satu kandidat, dan filesystem-fixture QA.** Source/hash/pin/path/backup/shutdown/global quota diperiksa ulang; plan schema v2 maksimum 24 jam. Intent/journal sebelum unlink, receipt/marker terpisah sesudahnya. Interupsi menahan retry untuk review; completion sah menjadi no-op. Native pilot dan integrasi kemudian dilaksanakan pada 2b; baca status terbaru di sana, bukan menganggap batas tahap fixture ini kondisi terkini.

- Hanya setelah scope pembuangan cluster sintetis diotorisasi, tambahkan apply eksplisit dengan lock bersama, pemeriksaan ulang path/identitas/shutdown/backup, journal sebelum tindakan, dan receipt hasil.
- Backup/source konteks sebelum perubahan. Uji pada fixture disposable terisolasi; verifikasi bahwa laporan, source dan backup tetap utuh serta data non-sintetis tidak dapat dipilih.
- Tangani interupsi dan eksekusi ulang dengan aman. Ukur ruang yang benar-benar dibebaskan.
- Integrasikan evaluasi setelah setiap full QA selesai serta pemeriksaan sebelum run baru. Di bawah kebijakan yang sudah diotorisasi, tidak perlu instruksi manual untuk tiap run; pengecualian tetap menghentikan tindakan yang tidak aman.
- Exit: receipt apply + QA perilaku retensi, bukan hanya angka run yang berhasil dipangkas. Jangan mengeksekusi apply historis hanya karena dokumen rencana sudah tersimpan.

### 2b. Pilot native dan integrasi operasional — implementasi selesai, residual terbuka

Pilot satu historis sudah selesai; hook pre-admission di semua entrypoint/launcher dan post-lab di luar lock telah dipasang. Aktivasi hanya untuk run sejak 11 November 2026 13:02:58 WIB, maksimum satu kandidat per evaluasi. Native prehook diverifikasi tanpa disposal tambahan; post-hook full native lifecycle belum dijalankan karena inventory sandbox tidak lengkap. Daftar scope awal di bawah dipertahankan; residual yang perlu dikerjakan berikutnya dirangkum pada 2c.

- Audit scratch/HOLD dari terminal owner tanpa menganggap ukuran yang tak terbaca nol. Scope pilot harus jelas mengizinkan disposal sintetis historis; tidak perlu meminta ulang bila sudah diberikan.
- Buat plan baru source/pin yang sama, review satu kandidat, backup source/context, verifikasi backup SQL/custom serta shutdown native, lalu pilot satu cluster dalam scope tersebut.
- Review journal, receipt, identitas KEEP, hash seluruh evidence/backup, byte payload hilang dan delta disk aktual. Interupsi tetap HOLD; tidak ada auto-resume.
- Setelah pilot diterima, integrasikan evaluasi sesudah stop dan sebelum admission secara serial. Runner tidak boleh mengambil ulang lock yang masih dipegang; kegagalan cleanup tidak mengubah status bisnis QA menjadi PASS atau melewati gate ruang.
- Maksimum dua PASS dan satu FAIL berlaku global hanya untuk yang eligible; pin/unknown tetap pengecualian. Jangan memulai cleanup paralel atau menghapus backup demi memaksa kuota.
- Exit operasional: fresh native evidence + hook teruji + inventory/admission lengkap. Sampai itu selesai, tidak ada klaim pertumbuhan storage sudah otomatis terkendali.

### 2c. Langkah terdekat — owner inventory dan review backlog

- Jalankan `portability/CONTINUE_M14_CP04_QA.cmd` di Windows terminal biasa. Inventory read-only dulu, lalu QA hanya bila admission PASS. Tambah `--inventory-only` untuk hanya mengukur; `review_qa_storage_owner.py` tetap tersedia. Tidak ada perubahan ACL atau penghapusan scratch; inventory sandbox tetap BLOCKED selama dua scratch tidak terbaca.
- Review 14 HOLD dan 51 kandidat backlog dalam scope disposal yang jelas. Auto-scope baru tidak mencakup historical backlog atau KEEP historis yang kelak keluar quota; jangan mengklaim jumlah raw sudah tiga.
- Setelah inventory/admission lengkap, lanjut langkah 3. Full run itu juga membuktikan native lifecycle hook pada source baru, termasuk post-stop; jangan menerimanya dari unit test atau pilot source sebelumnya.

## 3. QA penuh M14-CP04 — accepted local pada 11 November

**Status: selesai untuk source `cb0a9af6...`, M14 4/4 lokal.** Scope/exit di bawah dipertahankan sebagai kontrak historis; bukan instruksi mengulang run yang sudah diterima. Canonical M14 0/5 dan project 0/97 tidak dinaikkan.

- Pastikan inventory lengkap, ruang/reserve memenuhi v2, dan backup sebelum migrasi tersedia.
- Bekukan source; jalankan workload asli melalui runner Windows biasa: Playwright seed/auth, 1.080 measured requests native Go, Go/DB EXPLAIN/pool, backup/restore, dan seluruh regresi dependensi.
- Review manifest kasus, hash source, hasil performance, rekonsiliasi, coverage, serta normal shutdown sebelum menerima CP04.
- Jika lulus, catat M14 **4/4 lokal sintetis** dan perbarui status/map/handoff. Jika gagal, simpan diagnosis dan lanjutkan perbaikan pada sumber gagal; jangan mengubah threshold agar lulus.

Posisi saat brief dibuat: M13 **4/4 lokal historis**, M14 **3/4 lokal historis**, CP04 **pending full QA**. Shared runtime, provider, data nyata, frontend, keamanan legacy dan gerbang produksi tetap pekerjaan terpisah. Tidak ada commit/push atau akses database shared dari pembuatan brief ini.
