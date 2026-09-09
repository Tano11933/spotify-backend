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

## Mode 2 — Full-Stack Demo (showcase, satu command jalan semua)

Dipakai saat mau menunjukkan proyek ke recruiter/reviewer tanpa mereka perlu install Go/Node di laptop mereka.

### 1. `Dockerfile` backend — `spotify-backend/Dockerfile`

```dockerfile
# Stage 1: Build
FROM golang:1.23-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o api cmd/api/main.go

# Stage 2: Run (image kecil, tanpa toolchain Go)
FROM alpine:latest

WORKDIR /app
COPY --from=builder /app/api .

EXPOSE 9000
CMD ["./api"]
```

Multi-stage build: stage 1 pakai image `golang` (besar, ada compiler) untuk compile; stage 2 cuma copy hasil binary ke image `alpine` yang jauh lebih kecil (~15MB vs ~800MB) — cocok untuk image production.

### 2. `Dockerfile` frontend — `spotify-frontend/Dockerfile`

```dockerfile
# Stage 1: Build
FROM node:20-alpine AS builder

WORKDIR /app
COPY package*.json ./
RUN npm install

COPY . .
RUN npm run build

# Stage 2: Serve pakai Nginx
FROM nginx:alpine

COPY --from=builder /app/dist /usr/share/nginx/html
COPY nginx.conf /etc/nginx/conf.d/default.conf

EXPOSE 80
CMD ["nginx", "-g", "daemon off;"]
```

### 3. `nginx.conf` — `spotify-frontend/nginx.conf`

Wajib ada supaya React Router tidak 404 saat halaman di-refresh (SPA fallback routing):

```nginx
server {
    listen 80;
    root /usr/share/nginx/html;
    index index.html;

    location / {
        try_files $uri $uri/ /index.html;
    }
}
```

### 4. `docker-compose.prod.yml` — root `spotify-backend` (atau folder terpisah)

```yaml
services:
  postgres:
    image: postgres:16
    container_name: spotify-postgres
    environment:
      POSTGRES_USER: spotify
      POSTGRES_PASSWORD: spotify123
      POSTGRES_DB: spotify_db
    ports:
      - "5432:5432"
    volumes:
      - postgres_data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U spotify"]
      interval: 5s
      timeout: 5s
      retries: 5

  redis:
    image: redis:7-alpine
    container_name: spotify-redis
    ports:
      - "6379:6379"

  backend:
    build: ./spotify-backend      # sesuaikan path relatif ke lokasi file compose ini
    container_name: spotify-backend
    ports:
      - "9000:9000"
    environment:
      DB_HOST: postgres
      DB_PORT: 5432
      DB_USER: spotify
      DB_PASSWORD: spotify123
      DB_NAME: spotify_db
      REDIS_HOST: redis
      REDIS_PORT: 6379
      APP_PORT: 9000
      JWT_SECRET: ${JWT_SECRET}
      JWT_REFRESH_SECRET: ${JWT_REFRESH_SECRET}
      SMTP_HOST: ${SMTP_HOST}
      SMTP_PORT: ${SMTP_PORT}
      SMTP_USERNAME: ${SMTP_USERNAME}
      SMTP_PASSWORD: ${SMTP_PASSWORD}
      SMTP_FROM_EMAIL: ${SMTP_FROM_EMAIL}
      SMTP_FROM_NAME: ${SMTP_FROM_NAME}
    depends_on:
      postgres:
        condition: service_healthy
      redis:
        condition: service_started

  frontend:
    build: ./spotify-frontend
    container_name: spotify-frontend
    ports:
      - "3000:80"
    depends_on:
      - backend

volumes:
  postgres_data:
```

**Poin penting soal `DB_HOST`/`REDIS_HOST`:** nilainya `postgres`/`redis` (nama service di compose), **bukan** `localhost` — di dalam jaringan Docker Compose, antar container saling terhubung lewat nama service, bukan `localhost` (yang di dalam container merujuk ke container itu sendiri).

### 5. `.env` khusus untuk compose

Taruh di lokasi yang sama dengan `docker-compose.prod.yml`, isi variabel yang dipakai `${...}` di atas (nilai sama seperti `.env` backend biasa). **Wajib masuk `.gitignore`.**

### 6. Cara jalankan

```powershell
docker compose -f docker-compose.prod.yml up --build
```
Akses aplikasi via `http://localhost:3000` (frontend, yang otomatis komunikasi ke backend di `http://localhost:9000`).

---

## Kapan waktu yang tepat mengerjakan Mode 2?

Setelah frontend dasar (halaman Auth + Home + integrasi WebSocket) sudah berjalan normal secara manual (`npm run dev`) dan terhubung ke backend. Jangan buat `Dockerfile` frontend sebelum ada kode frontend untuk di-build — akan percuma.
