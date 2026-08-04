# PRD — Spotify Clone (Portfolio Project)

## 1. Ringkasan Proyek

**Nama proyek:** Spotify Clone Backend & Frontend
**Tujuan:** Membangun aplikasi streaming musik (clone Spotify) sebagai portofolio, dengan fokus menunjukkan kemampuan backend Go yang production-grade (multi-layer architecture, caching, real-time, auth security) dan frontend React modern.
**Target audiens dokumen ini:** Developer (P) dan Claude Code sebagai asisten implementasi.

## 2. Latar Belakang & Motivasi

Proyek ini dibuat untuk memperkuat portofolio dengan menunjukkan variasi tech stack di luar PHP/Laravel (stack utama P), khususnya Go untuk backend performant. Proyek dipilih karena scope-nya jelas, kompleksitasnya bertahap (CRUD sederhana → relasi → auth → real-time), dan hasilnya mudah didemokan.

## 3. Tech Stack

### Backend
- **Bahasa:** Go
- **Router/Framework:** Fiber v2
- **ORM:** GORM
- **Database:** PostgreSQL
- **Cache & session store:** Redis
- **Auth:** JWT (access token + refresh token, refresh token disimpan di Redis agar revocable)
- **Real-time:** WebSocket (native `gofiber/websocket` atau `gorilla/websocket`)
- **Email:** SMTP (untuk reset password)
- **Arsitektur:** Multi-layer — Handler → Service → Repository → Model

### Frontend
- **Framework:** React + Vite + TypeScript (TSX)
- **Styling:** Tailwind CSS **only** (tidak menggunakan component library seperti shadcn/MUI, murni utility classes)
- **Validasi form:** Zod
- **State management:** Bebas (Zustand/Redux/Context API — direkomendasikan Zustand karena ringan dan cocok untuk scope proyek ini, tapi keputusan akhir fleksibel)
- **HTTP client:** Axios atau native fetch (disarankan Axios untuk interceptor refresh token)

### Infrastruktur Development
- Docker Compose untuk Postgres & Redis (sudah berjalan)
- Environment: Windows + Docker Desktop + WSL2

## 4. Status Saat Ini (Sudah Selesai)

- [x] Setup project Go + Fiber + GORM + Postgres + Redis
- [x] Struktur multi-layer (handler/service/repository/model)
- [x] CRUD **Artist**
- [x] CRUD **Song** (relasi `belongsTo` Artist, `belongsTo` Album opsional)
- [x] CRUD **Album** (relasi `belongsTo` Artist, `hasMany` Song)
- [x] Validasi input menggunakan `go-playground/validator`
- [x] Routing terpisah (`internal/router`)
- [x] Repo di-push ke GitHub

## 5. Scope Fitur Baru (To Be Built)

### 5.1 Authentication & Authorization (JWT)
- Register (email, password, name)
- Login → menghasilkan access token (short-lived, ~15 menit) + refresh token (long-lived, ~7 hari, disimpan di Redis)
- Refresh token endpoint
- Logout (revoke refresh token dari Redis)
- Middleware proteksi route (JWT Bearer token)
- **Reset password via email (SMTP)**:
  - User request reset → generate token reset (random, expire pendek ~15 menit) → simpan di Redis → kirim email berisi link/kode reset
  - User submit token + password baru → validasi token → update password

### 5.2 WebSocket (Real-time)
- Use case yang relevan untuk Spotify clone:
  - **"Now playing" broadcast** — ketika user memutar lagu, status bisa di-broadcast (misalnya untuk fitur "lihat teman sedang dengar apa", opsional/simulasi)
  - **Live notification** sederhana (misalnya notifikasi like/follow), atau
  - Minimal: endpoint WebSocket untuk **realtime playback sync** antara device (opsional, kompleksitas tinggi) — **untuk versi awal, cukup buat 1 use case sederhana**: broadcast "currently playing" ke channel per user/room, agar ada bukti nyata kemampuan WebSocket tanpa over-engineering.
- Implementasi: 1 endpoint WebSocket (`/ws`), dengan handler yang connect ke Fiber's websocket upgrade, terhubung ke sebuah in-memory hub/broker sederhana (bisa dikombinasikan dengan Redis Pub/Sub jika ingin multi-instance ready).

### 5.3 Redis Caching
- Cache untuk endpoint yang sering diakses dan jarang berubah, contoh:
  - `GET /api/artists` (list artist) — cache 5 menit
  - `GET /api/albums/:id` (detail album) — cache 5 menit
  - Invalidasi cache otomatis saat data terkait di-create/update/delete
- Refresh token storage (sudah dirancang di fase Auth)
- Reset password token storage

### 5.4 Playlist (opsional lanjutan, many-to-many dengan Song)
- User bisa membuat playlist, menambah/menghapus lagu
- Relasi many-to-many antara Playlist dan Song

## 6. Non-Functional Requirements

- **Keamanan:** Password di-hash dengan bcrypt, JWT secret disimpan di `.env` (tidak pernah di-commit), refresh token revocable, rate limiting pada endpoint auth (khususnya login & reset password) menggunakan Redis.
- **Konsistensi kode:** Semua fitur baru mengikuti pola multi-layer yang sudah ada (handler-service-repository-model), termasuk pola validasi (`go-playground/validator`) dan pola error response yang konsisten (`{"error": "..."}`).
- **Dokumentasi:** Setiap endpoint baru didokumentasikan (minimal di README atau file API reference terpisah).
- **Testing:** Ditambahkan bertahap setelah fitur-fitur inti (Auth, WebSocket) selesai — unit test untuk service layer minimal untuk business logic kritikal (validasi password, token generation/validation).

## 7. Out of Scope (untuk versi ini)

- Payment/subscription system
- Audio transcoding/streaming chunked (asumsikan file audio sudah tersedia via URL/storage eksternal)
- Multi-device sync playback yang kompleks
- Admin dashboard terpisah
- Mobile app (fokus web dulu)

## 8. Milestone / Urutan Pengerjaan yang Disarankan

1. **Auth lengkap** — Register, Login, Refresh, Logout, Middleware JWT
2. **Reset Password via SMTP**
3. **Redis Caching** pada endpoint read-heavy (Artist, Album)
4. **WebSocket** — 1 use case realtime sederhana
5. **Frontend setup** — React + Vite + TSX + Tailwind, integrasi Zod untuk semua form (register, login, reset password, create artist/song/album jika ada admin panel sederhana)
6. **Playlist** (jika waktu memungkinkan)
7. **Testing dasar** untuk service layer kritikal

## 9. Kriteria Sukses (Definition of Done)

- Semua endpoint di atas berjalan dan sudah ditest manual (Postman/Thunder Client)
- Tidak ada credential/secret yang ter-commit ke Git
- Frontend bisa register, login, browse artist/album/song, dan menampilkan koneksi WebSocket aktif (indikator sederhana)
- README project menjelaskan cara setup dari nol (docker compose, .env, migrasi)
- Kode konsisten mengikuti arsitektur multi-layer yang sudah ditetapkan
