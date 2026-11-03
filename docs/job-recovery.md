# Job reservation dan recovery

Antrean ini untuk pekerjaan latar `email:send` dan `chart:refresh`, terpisah dari antrean lagu akun. API dan worker harus memakai Redis dan `WORKER_QUEUE` yang sama (default `jobs`).

## Siklus delivery

1. Enqueue menyimpan ID job UUID dan payload JSON asli di list `queue:jobs`.
2. Reserve menjalankan Lua untuk memindahkan satu job ke hash `queue:jobs:processing` dan mencatat batas waktu di sorted set `queue:jobs:leases`. Field hash/member sorted set adalah UUID reservation baru, bukan ID job. `Attempt` menghitung delivery mulai dari 1.
3. Lease berlaku 1 menit menurut jam Redis. Worker memperpanjangnya setiap 20 detik selama handler berjalan. Permintaan perpanjangan dibatasi 20 detik; kegagalan membatalkan context handler. Handler yang mengabaikan context masih bisa menghasilkan efek samping.
4. Handler berhasil: ACK menghapus reservation dan lease dalam satu script. Handler gagal: reservation dipindah atomik ke delayed atau dead-letter. Tipe job tanpa handler langsung masuk dead-letter.
5. Setiap Reserve memulihkan hingga 100 lease kedaluwarsa dan mempromosikan hingga 100 retry yang sudah waktunya. Antrean kosong dipoll setiap 50 ms dengan context yang dapat dibatalkan; tidak ada BLPOP yang menghapus job tanpa reservation.

Recovery, retry, dan promosi tidak memiliki celah antara penghapusan status lama dan penambahan status baru. ACK, retry, dead-letter, dan renew mensyaratkan reservation masih ada dan lease belum kedaluwarsa. Worker lama tidak dapat menyelesaikan reservation baru. Redis Cluster tidak didukung oleh susunan key multi-key ini; aplikasi memakai satu instance Redis.

Setelah kegagalan/crash delivery ke-1 sampai ke-4, retry menunggu 2, 4, 8, dan 16 detik. Delivery ke-5 yang gagal atau ditinggalkan masuk dead-letter. Berhenti karena shutdown juga meninggalkan reservation yang dapat dipulihkan. Recovery berjalan ketika worker aktif kembali, bukan oleh proses Redis terpisah.

## Duplikasi dan ketahanan data

Ini at-least-once dengan batas 5 delivery, bukan jaminan seluruh job berhasil. Jika worker menyelesaikan efek samping lalu crash sebelum ACK, job dapat diproses lagi. Lease/ACK mencegah penghapusan oleh pemilik lama, tetapi tidak memagari efek samping di SMTP atau database eksternal.

- Chart memakai `REFRESH MATERIALIZED VIEW CONCURRENTLY`: menghitung ulang data sumber, bukan menambah counter. Pengulangan dengan data sumber yang sama tidak menggandakan play. Refresh bersamaan diserialisasi PostgreSQL dan masih dapat gagal/di-retry.
- SMTP tidak menyediakan transaksi bersama Redis. Email bisa terkirim dua kali, termasuk ketika koneksi putus sesudah server SMTP menerima email. Tidak ada klaim deduplikasi/exactly-once; hindari replay email reset yang sudah kedaluwarsa atau berisi token lama. Penyelesaian lease bukan bukti penerima menerima email.
- Redis harus mempertahankan data agar job dapat pulih. Restart tanpa persistence atau kehilangan volume dapat menghilangkan ready, processing, delayed, dan dead-letter. Compose development saat ini tidak mengonfigurasi volume/persistence Redis; lease tidak mengubah batasan tersebut.

Payload disimpan sebagai string `raw_job` dalam envelope delivery (`attempt`, `last_error`, `raw_job`). Lua tidak mengurai dan menulis ulang isi payload, agar integer besar, array kosong, dan objek tidak berubah. Record baru di ready masih menggunakan bentuk Job lama; setelah reservation, delayed/processing/dead memakai envelope. JSON rusak yang terdeteksi saat Reserve dipindah ke dead-letter untuk inspeksi.

## Transisi worker lama

Hentikan seluruh worker versi lama sebelum menjalankan versi ini; worker BLPOP lama tidak memahami envelope delivery dan tidak membuat reservation. API lama masih dapat enqueue bentuk Job lama, dan ready/delayed/dead-letter lama tetap dapat dibaca/replay. `attempt` lama menghitung retry (0 sebelum percobaan pertama), sedangkan metadata baru menghitung delivery; satu delivery berikutnya menaikkan nilainya.

Job yang sudah diambil worker lama sebelum upgrade tidak bisa dipulihkan karena tidak pernah tercatat sebagai processing. Jangan melakukan rollback worker ketika masih ada envelope baru di Redis; drain/migrasikan antrean terlebih dahulu. Jangan flush Redis untuk upgrade karena sesi login dan job akan ikut hilang.

## Pemeriksaan

Contoh berikut memakai Redis compose development (`spotify-redis`) dan antrean default. Sesuaikan nama untuk lingkungan lain. Payload email bisa mengandung data pribadi dan token reset: batasi akses terminal/log ketika memeriksa isi record.

```powershell
docker exec spotify-redis redis-cli LLEN queue:jobs
docker exec spotify-redis redis-cli HLEN queue:jobs:processing
docker exec spotify-redis redis-cli ZRANGE queue:jobs:leases 0 9 WITHSCORES
docker exec spotify-redis redis-cli ZCARD queue:jobs:delayed
docker exec spotify-redis redis-cli LLEN queue:jobs:dead
docker exec spotify-redis redis-cli LINDEX queue:jobs:dead 0
```

Score lease adalah Unix milliseconds menurut Redis. Jika processing tidak berkurang setelah expiry, pastikan ada worker aktif, Redis dapat dijangkau, nama antrean sama, dan periksa log `worker: job loop gagal`. Hentikan worker yang masih melakukan efek samping sebelum replay manual; jangan menghapus hash/lease secara terpisah.

## Replay dead-letter

Perbaiki penyebabnya dahulu (SMTP/database/handler), periksa record tertua, dan pastikan efek samping aman diulang. Script berikut memindahkan **satu** record tertua dari dead-letter ke ready secara atomik, mempertahankan ID/payload, dan mereset hitungan delivery ke 0. Reserve berikutnya menjadi delivery 1 dengan reservation baru. Record JSON rusak tetap ditahan untuk diperbaiki manual; antrean kosong tidak mengubah data.

Dari root backend, untuk Redis compose development:

```powershell
docker cp scripts/replay-dead-job.lua spotify-redis:/tmp/melodia-replay-dead-job.lua
docker exec spotify-redis redis-cli --eval /tmp/melodia-replay-dead-job.lua queue:jobs:dead queue:jobs
docker exec spotify-redis rm /tmp/melodia-replay-dead-job.lua
```

Script mengembalikan ID job yang dipindah, bukan payload email. Pastikan dua key merujuk antrean yang sama. Jangan menjalankan perintah berulang tanpa memeriksa ID dan efek samping: setiap eksekusi memindahkan record berikutnya. Setelah worker memprosesnya, periksa ready/processing/delayed/dead dan log penyelesaian. Kedua key harus list Redis dan Redis harus punya kapasitas memori untuk menambahkan job.

## Pengujian

```powershell
go test ./...
go vet ./...
go test -tags=integration -timeout 10m ./test/integration/...
```

Integration suite memakai container PostgreSQL/Redis sementara. Test mencakup reserve bersamaan, expiry/crash berulang, penolakan operasi worker lama, renew, payload utuh, retry/backoff, dead-letter, recovery oleh worker, refresh chart berulang, dan script replay. Deadline khusus test di Redis dapat dipercepat tanpa tidur selama backoff produksi. Unit test worker mencakup heartbeat, kegagalan renew/settlement, dan shutdown.
