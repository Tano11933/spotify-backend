# DOCKER.md — Development vs Full-Stack Demo

Dua mode terpisah. Jangan campur keduanya dalam satu file compose.

---

## Mode 1 — Development (dipakai sehari-hari)

`docker-compose.yml` di root `spotify-backend` — **hanya** Postgres + Redis. Backend jalan manual (`go run cmd/api/main.go`), frontend jalan manual (`npm run dev`) — supaya hot-reload cepat dan tidak perlu rebuild image tiap ubah kode.

```powershell
docker compose up -d          # postgres + redis saja
go run cmd/api/main.go        # backend, di terminal terpisah
npm run dev                   # frontend, di terminal terpisah (folder spotify-frontend)
```

---

## Mode 2 — Full-Stack Demo (showcase)

Dipakai saat mau menunjukkan proyek ke recruiter/reviewer tanpa mereka perlu install Go/Node di laptop mereka.

### Yang dibangun

| Berkas | Isi |
|---|---|
| `Dockerfile` | Multi-stage: stage build menghasilkan 3 binary (`api`, `worker`, `seed`), stage runtime `alpine` non-root dengan healthcheck `/health`. |
| `docker-compose.prod.yml` | Project Compose `spotify-backend-prod`: Postgres + Redis + api + worker. Port DB/Redis tidak diekspos ke host. |
| `../spotify-frontend/Dockerfile` | Build SPA lalu disajikan nginx dengan fallback routing (lihat DOCKER.md repo frontend). |

### Menjalankan

```powershell
# dari root spotify-backend
if (-not (Test-Path .env)) { Copy-Item .env.example .env }
# Isi DB_USER, DB_PASSWORD, JWT_SECRET, JWT_REFRESH_SECRET di .env.
# Atur FRONTEND_URL dan CORS_ALLOWED_ORIGINS ke http://localhost:3000.
docker compose -f docker-compose.prod.yml config --quiet
docker compose -f docker-compose.prod.yml up --build -d --wait

# isi data demo: 13 genre, 14 artist, 233 lagu, 6 playlist, 4 akun demo
docker compose -f docker-compose.prod.yml exec api ./seed

# cek kesiapan dan metrik
curl.exe -fsS http://127.0.0.1:9000/ready
```

Lalu frontend dari repo `spotify-frontend`:

```powershell
docker compose -f docker-compose.prod.yml up --build -d --wait
# buka http://localhost:3000
```

### Audio yang bisa diputar tanpa URL eksternal

Seeder katalog memakai audio contoh dari luar stack. Untuk demo pemutaran
yang bisa diulang, buat WAV PCM lokal dengan PowerShell berikut.

```powershell
$wav = Join-Path ([IO.Path]::GetTempPath()) ('melodia-demo-' + [guid]::NewGuid().ToString('N') + '.wav')
$sampleRate = 8000
$samples = $sampleRate * 30
$writer = [IO.BinaryWriter]::new([IO.File]::Create($wav))
try {
  $writer.Write([Text.Encoding]::ASCII.GetBytes('RIFF'))
  $writer.Write([uint32](36 + $samples * 2))
  $writer.Write([Text.Encoding]::ASCII.GetBytes('WAVEfmt '))
  $writer.Write([uint32]16)
  $writer.Write([uint16]1)
  $writer.Write([uint16]1)
  $writer.Write([uint32]$sampleRate)
  $writer.Write([uint32]($sampleRate * 2))
  $writer.Write([uint16]2)
  $writer.Write([uint16]16)
  $writer.Write([Text.Encoding]::ASCII.GetBytes('data'))
  $writer.Write([uint32]($samples * 2))
  for ($i = 0; $i -lt $samples; $i++) {
    $writer.Write([int16](1200 * [Math]::Sin(2 * [Math]::PI * 220 * $i / $sampleRate)))
  }
} finally {
  $writer.Dispose()
}
$wav
```

1. Masuk ke frontend dengan akun admin demo yang dicetak seeder.
2. Buka **Admin**, pilih **Lagu**, lalu **Edit** pada lagu yang ingin dipakai.
3. Pilih WAV pada **File audio**, tekan **Unggah audio**, lalu **Putar hasil unggah**. Coba jeda dan geser posisi ke sekitar 10 detik.
4. Untuk menguji pemulihan posisi, jeda sebelum reload dan tunggu posisi tersimpan. Periksa bahwa audio kembali dalam keadaan jeda.
5. Untuk demo pustaka, sukai lagu dan tambahkan ke playlist. Antrekan lagu lain yang juga sudah diunggah audionya, lalu tekan **Lagu berikutnya** pada player.

Audio WAV ini adalah nada sintetis berdurasi 30 detik. Cover katalog contoh
masih bergantung pada URL gambar seeder. Untuk demo lokal, gunakan akun seed
di stack yang terisolasi dan hentikan kedua stack setelah selesai.

Alur ini diuji pada Docker Desktop dengan project Compose dan volume
sementara. Chrome berhasil membuka detail album, memutar WAV lokal, melakukan
seek, dan memulihkan posisi dalam keadaan jeda setelah reload. Saat Redis
dihentikan, `/ready` menjawab 503 dan `/health` tetap menjawab 200.

### Catatan penting

- **`name: spotify-backend-prod`** memberi project ini nama sendiri. Tanpa itu,
  service-nya (sama-sama `postgres`/`redis`) akan saling menimpa dengan stack
  development di folder yang sama.
- **JWT secret dibaca dari `.env`** dan wajib diisi; compose memakai sintaks
  `${VAR:?pesan}` sehingga gagal start dengan pesan jelas kalau kosong.
- **Worker memakai Dockerfile yang sama** dengan perintah `./worker`. Compose
  menunggu `/ready` API setelah migrasi. Worker tidak membuka port HTTP.
  Cek log worker untuk memastikan job `chart:refresh` selesai dan pastikan API
  serta worker memakai nilai `WORKER_QUEUE` yang sama.
- **Data tidak hilang saat `down` biasa**: Postgres dan unggahan audio disimpan
  di named volume (`postgres_data`, `uploads_data`). Pakai `down -v` kalau
  memang ingin mengosongkan.
- **Seeder bisa dijalankan ulang**: `docker compose -f docker-compose.prod.yml
  exec api ./seed -fresh` mengosongkan katalog lalu mengisinya kembali.
- **URL backend di frontend di-resolve saat BUILD**, bukan runtime: Vite
  menanamkan `import.meta.env` ke bundle. Karena itu nilainya
  `http://localhost:9000` (diakses browser di host), bukan nama service Docker.
- Log: `docker compose -f docker-compose.prod.yml logs -f api worker`.
- Gunakan `docker compose -f docker-compose.prod.yml down` di kedua repo untuk
  menghentikan demo. Prosedur snapshot dan pemulihan ada di [backup dan restore](docs/backup-restore.md).

### CI

Push ke `main` dan pull request menjalankan `.github/workflows/ci.yml`:
format (`gofmt`), `go vet`, build, unit test, dan integration test
(testcontainers). Detailnya ada di README bagian Testing.
