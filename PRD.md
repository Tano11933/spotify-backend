# PRD — Spotify Clone (Portfolio Project)

**Pemilik proyek:** Gabriel Gaetano Onen Baskara
**Tujuan:** Portofolio backend developer — menunjukkan kemampuan Go, arsitektur multi-layer, relational DB design, caching, real-time protocol, dan integrasi frontend modern.
**Status dokumen:** Living document — update seiring progress.

---

## 1. Overview

Backend + frontend clone Spotify sederhana, fokus pada kualitas arsitektur dan best practice (bukan replikasi fitur Spotify secara lengkap). Prioritas: kerapian kode, skalabilitas pola (bisa ditambah fitur tanpa refactor besar), dan variasi teknis yang menarik buat dibahas saat interview/portofolio.

## 2. Tujuan Proyek

- Menunjukkan kemampuan backend dengan bahasa selain PHP (Go)
- Menunjukkan pemahaman arsitektur multi-layer (handler → service → repository → model)
- Menunjukkan penggunaan Redis untuk hal nyata (bukan sekadar dipasang): caching, refresh token store, rate limiting
- Menunjukkan kemampuan real-time (WebSocket)
- Menunjukkan end-to-end auth flow yang aman (JWT + refresh token + reset password via email)
- Menunjukkan integrasi frontend modern dengan validasi terstruktur (Zod)

## 3. Tech Stack

### Backend
| Layer | Teknologi |
|---|---|
| Bahasa | Go |
| Router/Framework | Fiber |
| ORM | GORM |
| Database | PostgreSQL |
| Cache/Store | Redis |
| Auth | JWT (access + refresh token), bcrypt |
| Real-time | WebSocket (native `gorilla/websocket` atau `fasthttp/websocket` — kompatibel Fiber) |
| Email | SMTP (untuk reset password) |
| Validasi | `go-playground/validator` |

### Frontend
| Layer | Teknologi |
|---|---|
| Framework | React (Vite template `react-compiler-ts`) + TypeScript |
| React Compiler | Aktif (stable v1.0) — auto-memoization, kurangi `useMemo`/`useCallback` manual |
| Linter | Oxlint (bawaan template, bukan ESLint — jauh lebih cepat) |
| Styling | Tailwind CSS v4 (via `@tailwindcss/vite`, config berbasis `@theme` di CSS) |
| Validasi form | Zod + `react-hook-form` + `@hookform/resolvers` |
| State management | Zustand |
| HTTP client | Axios (dengan interceptor auto-refresh token) |
| WebSocket client | Native `WebSocket` API browser |
| Desain visual | Lihat `DESIGN.md` — terinspirasi Spotify asli (dark theme, hijau `#1DB954`) |

### Infrastruktur Dev
- Docker Compose (Postgres + Redis)
- Git + GitHub

---

## 4. Status Saat Ini (Progress)

✅ **Backend — SELESAI (di-commit per-fitur):**
- Setup project Go + Fiber + GORM + Postgres + Redis (Docker Compose)
- CRUD Artist, Song (relasi ke Artist, opsional ke Album), Album (relasi ke Artist, hasMany Song)
- Struktur multi-layer: `handler → service → repository → model`, validasi via struct tag
- User Auth lengkap: register, login, refresh token, logout, middleware JWT
- Forgot/Reset password via SMTP
- Redis caching untuk endpoint read-heavy (Artist, Album list) + invalidasi saat write
- WebSocket hub + event `song:playing`, `song:created`
- Playlist user (many-to-many User ↔ Song) dengan playlist publik
- Rate limiting per endpoint (Redis, Lua script)
- Seeder data demo idempoten (`go run ./cmd/seed`)
- Unit test: `pkg/jwt`, `internal/service`, `internal/websocket` (termasuk `-race`)
- Pagination envelope seragam + kode error machine-readable (`code`) di seluruh API
- Pencarian katalog (tsvector + trigram, tahan typo) dengan migrasi SQL ringan
- Integration test lewat testcontainers (Postgres + Redis sungguhan)

✅ **Frontend — SELESAI (repo terpisah):**
- React + Vite + TypeScript + Tailwind v4 + Zod + Zustand
- Halaman auth, katalog, detail artist/album, playlist, dashboard admin, player, toast real-time
- https://github.com/Tano11933/spotify-frontend

🔲 **Belum dikerjakan:**
- Docker: `Dockerfile` backend & frontend + `docker-compose.prod.yml` untuk mode full-stack demo (lihat `DOCKER.md`)
- Handler layer unit test (dengan service tiruan)
- Cursor pagination untuk feed/history

---

## 5. Fitur & Requirement Detail

### 5.1 Auth
- **Register**: nama, email (unique), password (hash bcrypt)
- **Login**: return access token (JWT, 15 menit) + refresh token (JWT, 7 hari, disimpan di Redis agar bisa di-revoke)
- **Refresh**: tukar refresh token valid → access token baru
- **Logout**: hapus refresh token dari Redis (revoke)
- **Forgot Password**: user submit email → sistem generate token reset (random string, disimpan di Redis dengan TTL 15 menit) → kirim email via SMTP berisi link reset
- **Reset Password**: user submit token + password baru → validasi token di Redis → update password → hapus token dari Redis

### 5.2 WebSocket (real-time)
Skenario yang relevan untuk Spotify clone:
- **"Now Playing" broadcast**: saat user memutar lagu, event dikirim ke WebSocket (berguna untuk fitur "lihat aktivitas teman" di masa depan)
- **Live notification**: contoh sederhana, notifikasi saat lagu/album baru ditambahkan oleh artist yang di-follow (fitur follow tidak wajib diimplementasi penuh, tapi struktur event-nya disiapkan)
- Minimal viable scope untuk portofolio: **1 WebSocket hub** yang broadcast event `song:playing` dan `song:created` ke semua client yang terkoneksi (broadcast sederhana dulu, bukan targeted per-user — itu bisa jadi enhancement lanjutan)

### 5.3 Caching (Redis)
- Cache **GET /api/artists** dan **GET /api/albums** (list) selama beberapa menit (TTL 5 menit) — data ini jarang berubah dibanding dibaca
- Cache di-invalidate (delete key) setiap kali ada Create/Update/Delete pada entity terkait
- Refresh token store (sudah ada di scope auth)
- Reset password token store

### 5.4 Frontend
- Halaman: Login, Register, Forgot Password, Reset Password, Home (list artist/album/song), Detail Artist, Detail Album, Player sederhana (mock, tidak perlu streaming audio sungguhan untuk MVP)
- Validasi semua form pakai **Zod schema**, terintegrasi dengan komponen form (misal `react-hook-form` + `@hookform/resolvers/zod`, atau validasi manual on-submit — bebas dipilih)
- Tailwind CSS murni — tidak pakai UI kit, styling manual untuk menunjukkan kemampuan CSS/utility-first design
- Koneksi WebSocket untuk menampilkan event real-time (misal toast notification saat ada lagu baru)

---

## 6. Data Model (ERD ringkas)

```
User
 - id, name, email (unique), password (hashed), created_at, updated_at

Artist
 - id, name, bio, image_url, created_at, updated_at

Album
 - id, title, cover_url, release_date, artist_id (FK), created_at, updated_at

Song
 - id, title, duration, file_url, artist_id (FK), album_id (FK, nullable), created_at, updated_at

Redis Keys
 - refresh_token:{user_id} -> refresh token string, TTL 7 hari
 - reset_token:{token} -> user_id, TTL 15 menit
 - artists:all -> JSON list artist, TTL 5 menit
 - albums:all -> JSON list album, TTL 5 menit
```

Relasi:
- Artist 1—N Album
- Artist 1—N Song
- Album 1—N Song (opsional, song bisa tanpa album/"single")
- User tidak berelasi langsung ke entity musik pada scope ini (Playlist ditunda ke fase berikutnya)

---

## 7. API Endpoints

### Auth
| Method | Endpoint | Auth | Deskripsi |
|---|---|---|---|
| POST | `/api/auth/register` | - | Daftar user baru |
| POST | `/api/auth/login` | - | Login, return access+refresh token |
| POST | `/api/auth/refresh` | - | Tukar refresh token → access token baru |
| POST | `/api/auth/logout` | JWT | Revoke refresh token |
| GET | `/api/auth/me` | JWT | Info user dari token |
| POST | `/api/auth/forgot-password` | - | Kirim email reset password |
| POST | `/api/auth/reset-password` | - | Set password baru dengan token |

### Artist / Album / Song
*(sudah ada, lihat kode existing — pertahankan pola yang sama)*

### WebSocket
| Endpoint | Deskripsi |
|---|---|
| `GET /ws` (upgrade) | Koneksi WebSocket, autentikasi via query param `?token=<access_token>` atau header saat handshake |

### Search
| Method | Endpoint | Auth | Deskripsi |
|---|---|---|---|
| GET | `/api/search?q=&type=&limit=&offset=` | - | Cari lagu/artist/album/playlist; hasil dikelompokkan per tipe; tahan typo |

---

## 8. WebSocket Protocol

**Format pesan (JSON), konsisten di semua event:**
```json
{
  "event": "song:playing",
  "data": { "song_id": 1, "title": "Monokrom", "user_id": 3 },
  "timestamp": "2026-08-04T10:00:00Z"
}
```

**Event yang didukung (MVP):**
| Event | Arah | Deskripsi |
|---|---|---|
| `song:playing` | Client → Server → Broadcast | User memutar lagu, di-broadcast ke semua client terkoneksi |
| `song:created` | Server → Broadcast | Otomatis dikirim saat ada Song baru dibuat via REST API |
| `connection:ack` | Server → Client | Konfirmasi koneksi berhasil setelah autentikasi |

**Autentikasi WebSocket:** validasi JWT access token saat handshake (sebelum upgrade koneksi). Jika token invalid/expired, tolak upgrade dengan HTTP 401 sebelum masuk ke protokol WebSocket.

---

## 9. Non-Functional Requirements

- Semua password di-hash (bcrypt), tidak pernah return password di response (`json:"-"` pada struct)
- Refresh token & reset token disimpan di Redis dengan TTL yang jelas, bukan di database permanen
- Endpoint yang butuh login wajib pakai middleware JWT, bukan pengecekan manual per-handler
- Response error konsisten: `{"error": "pesan"}`
- Environment variable (`.env`) tidak pernah di-commit ke Git

---

## 10. Roadmap / Fase Pengerjaan

1. **Fase 1 (selesai):** Setup infra + CRUD Artist/Song/Album
2. **Fase 2 (selesai):** Auth lengkap (register, login, refresh, logout, middleware JWT)
3. **Fase 3 (selesai):** Forgot/Reset password via SMTP
4. **Fase 4 (selesai):** Redis caching di endpoint read-heavy
5. **Fase 5 (selesai):** WebSocket hub + integrasi event `song:playing`, `song:created`
6. **Fase 6 (selesai):** Frontend React + Vite + Tailwind v4 + Zod + Zustand — repo terpisah: https://github.com/Tano11933/spotify-frontend
7. **Fase 7 (sekarang):** Docker full-stack demo (`Dockerfile` backend & frontend + `docker-compose.prod.yml`) — lihat `DOCKER.md`
8. **Fase 8 (selesai):** Playlist (many-to-many User–Song) + playlist publik
9. **Fase 9 (selesai sebagian):** Unit testing `pkg/jwt`, `internal/service`, `internal/websocket` (termasuk `-race`); handler & integration test menyusul

---

## 11. Catatan Desain (untuk dijelaskan saat interview)

- Kenapa access token pendek + refresh token panjang: keamanan (kompromi token akses tidak bertahan lama) tanpa mengorbankan UX (user tidak perlu login ulang tiap 15 menit)
- Kenapa refresh token disimpan di Redis, bukan hanya divalidasi via signature JWT: agar bisa **di-revoke** (logout paksa, misal saat user ganti password atau report device hilang) — JWT stateless murni tidak bisa di-revoke sebelum expired
- Kenapa cache di-invalidate manual saat Create/Update/Delete, bukan TTL saja: mencegah client melihat data basi terlalu lama setelah perubahan
- Kenapa WebSocket broadcast sederhana dulu (bukan targeted per-user/room): scope portofolio, cukup untuk menunjukkan pemahaman konsep; desain event JSON sudah dibuat agar mudah dikembangkan ke targeted messaging nanti
