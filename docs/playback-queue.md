# Antrean playback dan reservasi

Ditambahkan pada 25 Oktober 2026. Semua endpoint berikut memerlukan access token dan selalu menggunakan identitas dari middleware autentikasi.

## Identitas entri

GET /api/me/player/queue/entries mengembalikan envelope pagination dengan item `{id, position, added_at, song}`. Dua entri dengan lagu yang sama tetap memiliki id berbeda.

DELETE /api/me/player/queue/entries/:entryId menghapus entri tersebut jika milik user. Penghapusan entri yang tidak ada atau milik user lain tetap idempoten, tanpa mengubah antrean user lain.

Endpoint GET /api/me/player/queue yang mengembalikan lagu, POST penambahan lagu, dan DELETE berdasarkan songId tetap tersedia untuk client lama. Frontend baru memakai identitas entri untuk memilih dan menghapus kemunculan yang tepat.

## Reservasi sebelum memutar

POST /api/me/player/queue/reservations menerima `{}` untuk entri tersedia berikutnya atau `{entry_id: 123}` untuk entri tertentu. Respons sukses berupa `{id, entry_id, expires_at, song}`. `id` adalah token reservasi UUID, berbeda dari ID entri.

- Reservasi tidak menghapus entri dan tidak mencatat history.
- Durasi lease 60 detik dihitung di server.
- Pemilihan menggunakan transaksi dan SELECT FOR UPDATE SKIP LOCKED, diurutkan position lalu id.
- Dua transaksi tidak memperoleh lease aktif untuk entri yang sama. Entri yang sedang dipegang perangkat lain dilewati pada pemilihan next.
- Jika tidak ada entri tersedia untuk permintaan next, respons 200 berisi null. Permintaan entry_id tertentu yang tidak tersedia, sudah dipegang, atau bukan milik user dibalas 409 dengan pesan yang sama.
- Penambahan antrean mengunci row pemilik selama penentuan posisi, agar append bersamaan tidak membuat posisi yang sama.

## Konfirmasi dan pelepasan

Setelah native audio mengirim event playing, client memanggil POST /api/me/player/queue/reservations/:reservationId/confirm. DELETE hanya berhasil jika token masih aktif, belum kedaluwarsa, dan milik user. Konfirmasi kedua ditolak 409; entri lain tidak ikut terhapus.

Jika audio gagal, pengguna berpindah lagu, atau pemutaran dibatalkan, client memanggil DELETE /api/me/player/queue/reservations/:reservationId. Pelepasan hanya mengosongkan lease dan bersifat idempoten. Token lama tidak dapat melepaskan lease baru pada entri yang sama.

Kolom reservation_id dan reserved_until ditambahkan lewat AutoMigrate yang sudah dijalankan aplikasi saat startup. Client yang hilang koneksi tidak membuat entri terkunci selamanya: reservasi kedaluwarsa bisa diambil lagi tanpa worker tambahan.

## Riwayat dan batasan

Client baru memanggil POST /api/me/player/play hanya setelah event playing. Untuk lagu antrean, konfirmasi konsumsi diselesaikan lebih dulu. Pause/resume tidak membuat sesi pemutaran baru; replay yang disengaja membuat versi pemutaran baru.

Konsumsi entri bersifat atomik, tetapi konfirmasi dan pencatatan history merupakan dua request terpisah. Jika konfirmasi berhasil tetapi request history gagal, entri sudah dikonsumsi sementara history dapat tertinggal. Frontend menampilkan kegagalan pencatatan dan tidak melakukan retry buta yang bisa menggandakan hitungan. Kontrak ini tidak menjanjikan exactly-once analytics pada gangguan jaringan.

Lease bukan jaminan audio hanya terdengar di satu perangkat selamanya. Batasnya adalah kepemilikan reservasi selama masa berlaku. Browser yang belum berhasil memulai audio sebelum lease habis dihentikan oleh frontend dan harus mengambil reservasi lagi.

Server tidak dapat membuktikan client benar-benar mendengarkan hanya dari request REST. Ambang durasi mendengarkan dan perlindungan manipulasi chart berada di backlog terpisah.

## Verifikasi

```powershell
go test ./...
go vet ./...
$env:DOCKER_HOST = 'npipe:////./pipe/dockerDesktopLinuxEngine'
go test -tags=integration -count=1 ./test/integration/...
```

Pada platform lain, gunakan Docker host sesuai instalasi; tidak perlu menetapkan endpoint Windows tersebut.

Integration suite memakai PostgreSQL/Redis sementara. Tes mencakup concurrent append dan reservation, konsumsi tepat satu entri dari lagu berulang, isolasi user, token kedaluwarsa, stale release, queue kosong, validasi ID, serta kompatibilitas endpoint lama.

Suite juga menemukan label metrics yang memakai string pinjaman buffer Fiber. Middleware sekarang menyalin method dan route sebelum menyimpannya ke Prometheus; tes regresi menjalankan campuran method berulang dan memastikan registry tetap dapat dikumpulkan.
