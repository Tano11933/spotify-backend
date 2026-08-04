# Arsitektur — Spotify Clone

## 1. Arsitektur Backend (Go)

### 1.1 Prinsip

Multi-layer architecture, tiap layer punya tanggung jawab tunggal:

```
Request masuk
     ↓
[Middleware]   → auth check, rate limit, logging
     ↓
[Handler]      → parse request, validasi format (struct tag), format response
     ↓
[Service]      → business logic, orchestrasi antar repository, caching decision
     ↓
[Repository]   → query database (via GORM) / cache (via Redis)
     ↓
[Model]        → representasi struct tabel database
```

Aturan: Handler tidak boleh langsung memanggil Repository. Service tidak boleh tahu detail HTTP (Fiber context). Repository tidak boleh berisi business logic.

### 1.2 Struktur Folder Final

```
spotify-backend/
├── cmd/
│   └── api/
│       └── main.go                  # entrypoint, wiring semua dependency
├── internal/
│   ├── handler/
│   │   ├── artist_handler.go
│   │   ├── song_handler.go
│   │   ├── album_handler.go
│   │   ├── auth_handler.go
│   │   ├── playlist_handler.go      # (opsional)
│   │   └── ws_handler.go            # WebSocket handler
│   ├── service/
│   │   ├── artist_service.go
│   │   ├── song_service.go
│   │   ├── album_service.go
│   │   ├── auth_service.go
│   │   ├── playlist_service.go      # (opsional)
│   │   └── mail_service.go          # SMTP sender
│   ├── repository/
│   │   ├── artist_repository.go
│   │   ├── song_repository.go
│   │   ├── album_repository.go
│   │   ├── user_repository.go
│   │   └── playlist_repository.go   # (opsional)
│   ├── model/
│   │   ├── artist.go
│   │   ├── song.go
│   │   ├── album.go
│   │   ├── user.go
│   │   └── playlist.go              # (opsional)
│   ├── middleware/
│   │   ├── auth.go                  # JWT protected middleware
│   │   └── rate_limit.go            # rate limiter berbasis Redis
│   ├── router/
│   │   └── router.go
│   └── websocket/
│       └── hub.go                   # connection manager / broadcaster
├── pkg/
│   ├── database/
│   │   └── postgres.go
│   ├── cache/
│   │   └── redis.go
│   ├── jwt/
│   │   └── jwt.go
│   ├── mailer/
│   │   └── smtp.go                  # low-level SMTP client wrapper
│   └── validator/
│       └── validator.go
├── .env
├── .env.example
├── .gitignore
├── docker-compose.yml
├── go.mod
└── go.sum
```

### 1.3 Auth Flow (JWT + Redis)

```
Register:
  POST /api/auth/register → hash password (bcrypt) → simpan ke Postgres

Login:
  POST /api/auth/login → cek email+password → generate access token (JWT, 15m)
                        → generate refresh token (JWT, 7d) → simpan refresh token di Redis
                          key: refresh_token:{user_id}, value: token, TTL: 7 hari
                        → return {access_token, refresh_token}

Akses endpoint terproteksi:
  Request header: Authorization: Bearer {access_token}
  → Middleware validasi signature + expiry JWT
  → set c.Locals("userID", ...) untuk dipakai handler

Refresh:
  POST /api/auth/refresh {refresh_token}
  → validasi signature refresh token
  → cek refresh token ini masih ada di Redis (belum di-revoke)
  → generate access + refresh token baru (rotate refresh token)

Logout:
  POST /api/auth/logout (butuh access token valid)
  → hapus refresh_token:{user_id} dari Redis
```

### 1.4 Reset Password Flow (SMTP)

```
Step 1 — Request reset:
  POST /api/auth/forgot-password {email}
  → cek email exist
  → generate random token (crypto/rand, bukan JWT — cukup string acak)
  → simpan di Redis: key reset_token:{token}, value: user_id, TTL 15 menit
  → kirim email via SMTP berisi link: https://frontend-url/reset-password?token={token}
  → response sukses selalu sama meskipun email tidak ditemukan
    (mencegah email enumeration attack)

Step 2 — Submit reset:
  POST /api/auth/reset-password {token, new_password}
  → cek token exist di Redis → ambil user_id
  → hash password baru → update ke Postgres
  → hapus token dari Redis (single-use)
  → (opsional) revoke semua refresh token user ini juga, agar sesi lama logout otomatis
```

**Rate limiting penting** untuk endpoint `forgot-password` — gunakan Redis untuk membatasi maksimal beberapa request per email/IP per jam, mencegah spam email.

### 1.5 Redis Caching Strategy

Pola **cache-aside** (paling umum dan mirip `Cache::remember()` di Laravel):

```
GET /api/artists:
  1. Cek Redis key "artists:all"
  2. Jika ada → return langsung dari cache
  3. Jika tidak ada → query Postgres → simpan ke Redis (TTL 5 menit) → return

POST/PUT/DELETE /api/artists:
  → setelah operasi berhasil, hapus (invalidate) key "artists:all"
    dan key spesifik seperti "artist:{id}" jika ada
```

Terapkan pola yang sama untuk `albums`. Untuk `songs`, caching opsional karena datanya lebih sering berubah (tergantung use case), fokuskan caching pada data yang read-heavy dan write-light.

### 1.6 WebSocket Design (Minimal Viable)

Use case: **broadcast status "now playing"** ke semua client yang connect (simulasi sederhana, tidak perlu room per user dulu di versi awal).

```
internal/websocket/hub.go:
  - Hub menyimpan daftar koneksi aktif (map[*websocket.Conn]bool)
  - method Register(conn), Unregister(conn), Broadcast(message)

internal/handler/ws_handler.go:
  - GET /ws (upgrade ke websocket)
  - Saat client kirim pesan {"song_id": 5, "action": "play"}
    → hub broadcast ke semua client lain: {"user": ..., "song_id": 5, "action": "play"}

Opsional pengembangan lanjutan:
  - Gunakan Redis Pub/Sub sebagai broker jika nanti backend di-scale ke multiple instance
    (karena in-memory hub tidak akan sinkron antar instance)
```

Untuk versi awal, **in-memory hub cukup** — cukup untuk demo dan portofolio, dan bisa disebutkan di README sebagai "next improvement: Redis Pub/Sub untuk horizontal scaling".

### 1.7 Environment Variables (`.env`)

```
# Database
DB_HOST=localhost
DB_PORT=5432
DB_USER=spotify
DB_PASSWORD=spotify123
DB_NAME=spotify_db

# Redis
REDIS_HOST=localhost
REDIS_PORT=6379

# App
APP_PORT=9000

# JWT
JWT_SECRET=
JWT_REFRESH_SECRET=

# SMTP (contoh pakai Mailtrap/Gmail App Password untuk development)
SMTP_HOST=
SMTP_PORT=
SMTP_USERNAME=
SMTP_PASSWORD=
SMTP_FROM_EMAIL=noreply@spotifyclone.dev
SMTP_FROM_NAME=Spotify Clone

# Frontend URL (untuk link reset password di email)
FRONTEND_URL=http://localhost:5173
```

## 2. Arsitektur Frontend (React + Vite + TSX)

### 2.1 Struktur Folder

```
spotify-frontend/
├── src/
│   ├── api/
│   │   ├── axiosInstance.ts         # base axios + interceptor refresh token
│   │   ├── auth.ts                  # fungsi call API auth
│   │   ├── artist.ts
│   │   ├── song.ts
│   │   ├── album.ts
│   │   └── websocket.ts             # koneksi WS
│   ├── components/
│   │   ├── ui/                      # button, input, card dasar (Tailwind only)
│   │   └── layout/                  # navbar, sidebar, dll
│   ├── features/
│   │   ├── auth/
│   │   │   ├── LoginForm.tsx
│   │   │   ├── RegisterForm.tsx
│   │   │   ├── ForgotPasswordForm.tsx
│   │   │   └── schema.ts            # Zod schema untuk validasi form auth
│   │   ├── artist/
│   │   ├── album/
│   │   └── song/
│   ├── store/                       # state management (Zustand disarankan)
│   │   ├── authStore.ts
│   │   └── playerStore.ts           # state "now playing" untuk WebSocket
│   ├── pages/
│   ├── routes/
│   │   └── ProtectedRoute.tsx       # wrapper cek auth sebelum render halaman
│   ├── types/
│   │   └── index.ts
│   ├── App.tsx
│   └── main.tsx
├── .env
├── tailwind.config.js
├── vite.config.ts
└── package.json
```

### 2.2 Validasi Form dengan Zod

Contoh pola yang konsisten dipakai di semua form:

```ts
// features/auth/schema.ts
import { z } from "zod";

export const loginSchema = z.object({
  email: z.string().email("Email tidak valid"),
  password: z.string().min(6, "Password minimal 6 karakter"),
});

export type LoginInput = z.infer<typeof loginSchema>;
```

Digunakan bersama `react-hook-form` (disarankan, meskipun state management bebas — untuk form spesifik, `react-hook-form` + `@hookform/resolvers/zod` adalah kombinasi paling umum dan ringan) untuk validasi real-time di sisi client, selaras dengan validasi `go-playground/validator` di backend (validasi dobel: client untuk UX, server untuk keamanan).

### 2.3 State Management

Karena dibebaskan, rekomendasi: **Zustand** — alasan:
- Setup minimal (tidak perlu provider wrapping berlapis seperti Redux)
- Cocok untuk scope menengah (auth state, player state, playlist state)
- Sintaks sederhana, mudah dipelajari untuk yang baru pertama kali pakai state management library

Alternatif: Context API bawaan React (jika ingin zero-dependency), atau Redux Toolkit (jika ingin exposure ke pola yang lebih sering dipakai di perusahaan besar).

### 2.4 Styling — Tailwind Only

- Tidak menggunakan component library (shadcn/ui, MUI, Chakra, dll)
- Semua komponen UI dasar (Button, Input, Card, Modal) dibuat manual di `components/ui/` menggunakan utility classes Tailwind
- Gunakan `tailwind.config.js` untuk mendefinisikan design tokens (warna, spacing) agar konsisten — rujuk ke skill `frontend-design` untuk arahan estetika yang tidak generik

### 2.5 Koneksi WebSocket dari Frontend

```ts
// api/websocket.ts
const ws = new WebSocket(`ws://localhost:9000/ws?token=${accessToken}`);

ws.onmessage = (event) => {
  const data = JSON.parse(event.data);
  // update playerStore (Zustand) berdasarkan broadcast "now playing"
};
```

### 2.6 Axios Interceptor — Auto Refresh Token

Pola standar: jika request gagal karena 401 (access token expired), otomatis panggil `/api/auth/refresh` menggunakan refresh token tersimpan, lalu retry request asli. Ini penting untuk UX yang mulus tanpa user harus login ulang setiap 15 menit.

## 3. Diagram Relasi Database (ERD Ringkas)

```
Artist (1) ──< (N) Album
Artist (1) ──< (N) Song
Album  (1) ──< (N) Song  [Song.AlbumID nullable]
User   (1) ──< (N) Playlist        [opsional lanjutan]
Playlist (N) ──< >── (N) Song      [many-to-many, opsional lanjutan]
```

## 4. Urutan Implementasi Teknis (untuk Claude Code)

1. Lengkapi Auth Service (Register, Login, Refresh, Logout) — jika belum selesai dari sesi sebelumnya
2. Tambah Reset Password (SMTP + Redis token)
3. Tambah Redis caching di Artist & Album (read + invalidation)
4. Tambah WebSocket hub + endpoint `/ws`
5. Setup project frontend (Vite + React + TS + Tailwind, tanpa component library)
6. Implementasi halaman Auth (Register/Login/Forgot Password) dengan Zod + react-hook-form
7. Implementasi halaman list Artist/Album/Song (consume API yang sudah ada)
8. Integrasi WebSocket sederhana di frontend (indikator "now playing")
