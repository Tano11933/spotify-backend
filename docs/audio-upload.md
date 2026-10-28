# Upload dan penggantian audio

`POST /api/admin/songs/:id/audio` menerima multipart dengan satu field `file`,
Bearer token admin, dan mengembalikan objek Song (200). Frontend selalu memutar
melalui `GET /api/stream/songs/:id`; `file_url` tidak berubah setelah upload.

Unggahan baru menerima:

- `.mp3`, MPEG Layer III versi 1/2/2.5, bitrate terindeks, sedikitnya dua frame
  utuh. ID3v2.2/2.3/2.4 di awal dan ID3v1 di akhir didukung. Free-format dan
  metadata tambahan di luar bentuk tersebut tidak didukung.
- `.wav`, RIFF/WAVE PCM integer, 8/16-bit, mono/stereo, sample rate 8–192 kHz.
  Ukuran RIFF, batas chunk, block alignment, byte rate dan data sampel diperiksa.
  Float, extensible WAV, RF64, serta WAV terkompresi ditolak.

Ekstensi diperiksa tanpa membedakan huruf besar/kecil. MIME dari browser tidak
dipercaya. Validasi memeriksa struktur, bukan melakukan decoding penuh MP3;
payload frame yang rusak masih dapat gagal pada decoder browser. Tidak ada
transcoding atau ekstraksi durasi otomatis. Durasi katalog tetap diisi admin;
player memakai durasi native saat metadata audio tersedia.

Default ukuran satu file adalah 20 MiB (20 × 1024 × 1024 byte); BodyLimit HTTP
21 MiB memberi ruang untuk boundary multipart. Batas sebenarnya diperiksa saat
membaca stream, termasuk bila ukuran yang dinyatakan terlalu kecil. Validasi
membaca paling banyak batas + 1 byte ke memori. Konfigurasi MaxUploadBytes yang
lebih kecil tetap berlaku di server, meskipun UI memakai batas default 20 MiB.

| Kondisi | Status |
| --- | --- |
| Field hilang, kosong, format/isi tidak valid | 400 |
| Tanpa token / bukan admin | 401 / 403 |
| Lagu tidak ditemukan | 404 |
| Ukuran melewati batas | 413 |
| Kegagalan storage/database | 500 |

Error API memakai `{ "error": "pesan", "code": "kode" }`. Body yang ditolak
pada lapisan transport sebelum handler dapat mengikuti error transport Fiber.

Isi divalidasi sebelum Save. Server menyimpan file dengan key baru, mengganti
referensi database memakai row lock, lalu menghapus key lama. Kegagalan Save
atau update database mencoba menghapus key baru. Dua upload bersamaan diurutkan
pada update referensi; update terakhir yang menang. Penghapusan storage bersifat
best effort: kegagalan Delete atau proses mati bisa meninggalkan file yatim,
tetapi validasi gagal tidak mengganti referensi lagu. Tidak ada transaksi atomik
antara filesystem dan database.

Streaming file unggahan memakai `Cache-Control: no-store`, `nosniff`, serta
HTTP Range (206/416), sehingga pemutaran ulang setelah penggantian meminta
file terbaru. Stream lama yang sudah terbuka dapat tetap memainkan file lama.
Permintaan Range tidak valid juga menutup file descriptor.

File `.ogg`, `.m4a`, dan `.aac` yang tersimpan sebelumnya tetap mempunyai MIME
stream yang sesuai; unggahan baru dalam format tersebut ditolak. URL eksternal
pada lagu tanpa unggahan tetap dialihkan dengan 302.

Pembatalan di browser menghentikan permintaan klien, bukan rollback server.
Jika file sudah terkirim, penyimpanan mungkin sudah selesai. Periksa playback
sebelum mencoba ulang. Berganti lagu, tab admin, halaman, atau akun membatalkan
request lokal dan mencegah response lama memperbarui editor yang berbeda.

## Verifikasi

```powershell
go test ./...
go vet ./...
go test -tags=integration -count=1 ./test/integration/...
```

Unit test memeriksa framing MP3, tag, WAV PCM, truncation dan format palsu.
Integration test memeriksa HTTP upload/otorisasi, ukuran deklarasi dan aktual,
preservasi file lama saat gagal, penggantian bersamaan, cleanup file serta Range.

TestMain mengembalikan hasil test melalui fungsi terpisah sebelum os.Exit agar
defer benar-benar menutup hub, koneksi database/Redis, menghapus file sementara,
dan menghentikan serta menghapus container dependency. Docker Desktop yang
dinyalakan secara manual masih perlu dihentikan setelah rangkaian pengujian.
