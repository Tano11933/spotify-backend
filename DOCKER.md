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
docker compose -f docker-compose.prod.yml up --build -d

# isi data demo: 13 genre, 14 artist, 233 lagu, 6 playlist, 4 akun demo
docker compose -f docker-compose.prod.yml exec api ./seed

# cek kesiapan dan metrik
curl http://127.0.0.1:9000/ready
curl http://127.0.0.1:9000/metrics
```

Lalu frontend dari repo `spotify-frontend`:

```powershell
docker compose -f docker-compose.prod.yml up --build -d
# buka http://localhost:3000
```

### Catatan penting

- **`name: spotify-backend-prod`** memberi project ini nama sendiri. Tanpa itu,
  service-nya (sama-sama `postgres`/`redis`) akan saling menimpa dengan stack
  development di folder yang sama.
- **JWT secret dibaca dari `.env`** dan wajib diisi; compose memakai sintaks
  `${VAR:?pesan}` sehingga gagal start dengan pesan jelas kalau kosong.
- **Worker memakai image yang sama**, hanya `command: ["./worker"]`.
  Healthcheck bawaannya dimatikan (worker tidak membuka port HTTP), dan ia
  menunggu `api` healthy supaya migrasi selesai lebih dulu.
- **Data tidak hilang saat `down` biasa**: Postgres dan unggahan audio disimpan
  di named volume (`postgres_data`, `uploads_data`). Pakai `down -v` kalau
  memang ingin mengosongkan.
- **Seeder bisa dijalankan ulang**: `docker compose -f docker-compose.prod.yml
  exec api ./seed -fresh` mengosongkan katalog lalu mengisinya kembali.
- **URL backend di frontend di-resolve saat BUILD**, bukan runtime: Vite
  menanamkan `import.meta.env` ke bundle. Karena itu nilainya
  `http://localhost:9000` (diakses browser di host), bukan nama service Docker.
- Log: `docker compose -f docker-compose.prod.yml logs -f api worker`.

### CI

Push ke `main` dan pull request menjalankan `.github/workflows/ci.yml`:
format (`gofmt`), `go vet`, build, unit test, dan integration test
(testcontainers). Detailnya ada di README bagian Testing.
