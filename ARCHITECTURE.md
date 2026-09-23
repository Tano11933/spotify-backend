# ARCHITECTURE.md — Spotify Clone

Dokumen ini menjelaskan struktur folder dan pola kode yang harus diikuti saat menambah fitur baru (Auth, WebSocket, SMTP, Caching) agar konsisten dengan kode yang sudah ada.

---

## 1. Backend — Struktur Folder Final (target)

```
spotify-backend/
├── cmd/
│   └── api/
│       └── main.go                  # entrypoint, wiring semua dependency
├── internal/
│   ├── handler/                     # terima HTTP request, panggil service
│   │   ├── artist_handler.go        # (sudah ada)
│   │   ├── song_handler.go          # (sudah ada)
│   │   ├── album_handler.go         # (sudah ada)
│   │   └── auth_handler.go          # register/login/refresh/logout/me + forgot/reset password
│   ├── service/                     # business logic
│   │   ├── artist_service.go
│   │   ├── song_service.go
│   │   ├── album_service.go
│   │   └── auth_service.go          # login/register/token pair/logout + reset password
│   │                                # (password logic digabung di sini karena berbagi
│   │                                #  sentinel error & normalisasi email dengan login/register)
│   ├── repository/                  # akses database (GORM)
│   │   ├── artist_repository.go
│   │   ├── song_repository.go
│   │   ├── album_repository.go
│   │   └── user_repository.go
│   ├── model/                       # struct = tabel database
│   │   ├── artist.go
│   │   ├── song.go
│   │   ├── album.go
│   │   └── user.go
│   ├── middleware/
│   │   └── auth.go                  # JWT middleware
│   ├── router/
│   │   └── router.go                # semua route didaftarkan di sini
│   └── websocket/
│       └── hub.go                   # hub + client + event dalam satu file
│                                    # (Client & Event masing-masing ~40 baris, memecahnya
│                                    #  belum berbayar — split nanti kalau tumbuh)
├── pkg/                              # reusable, tidak spesifik ke business logic
│   ├── database/
│   │   └── postgres.go
│   ├── cache/
│   │   └── redis.go                 # koneksi Redis + cache.Store (struct, di-inject via DI)
│   ├── jwt/
│   │   └── jwt.go
│   ├── mailer/
│   │   └── smtp.go                  # kirim email generik (dipakai auth_service utk reset password)
│   └── validator/
│       └── validator.go
├── .env
├── .env.example
├── .gitignore
├── go.mod
├── go.sum
└── docker-compose.yml
```

### Prinsip yang harus dipertahankan
- **Handler** tidak boleh berisi query database atau logic bisnis — hanya parsing request, validasi format, panggil service, format response
- **Service** tidak boleh tahu soal `fiber.Ctx` — service harus bisa dites tanpa HTTP sama sekali
- **Repository** hanya berisi query, tidak ada validasi atau business rule
- Semua dependency (db, redis, mailer) di-inject lewat constructor (`NewXxxService(...)`), bukan variabel global

---

## 2. Modul Baru — Detail Desain

### 2.1 `pkg/mailer/smtp.go`
Bertanggung jawab murni untuk mengirim email — tidak tahu soal "reset password", itu urusan `auth_service.go` (bagian reset password).

```go
package mailer

type Mailer struct {
	host      string
	port      string
	username  string
	password  string
	fromEmail string
	fromName  string
}

func NewMailer(host, port, username, password, fromEmail, fromName string) *Mailer

func (m *Mailer) Send(to, subject, body string) error
```

Menggunakan **go-mail** (mudah untuk HTML email dan header `From: Nama <alamat>`).

**Env yang dibutuhkan:**
```
SMTP_HOST=smtp.gmail.com
SMTP_PORT=587
SMTP_USERNAME=your_email@gmail.com
SMTP_PASSWORD=app_password_bukan_password_biasa
SMTP_FROM_EMAIL=noreply@spotifyclone.dev
SMTP_FROM_NAME=Spotify Clone
```
Catatan: `SMTP_FROM_EMAIL` dan `SMTP_FROM_NAME` dipisah karena go-mail membutuhkan nama & alamat terpisah untuk menyusun header `From: Nama <alamat>`. Kalau pakai Gmail, wajib generate **App Password** (bukan password akun biasa), karena Google memblokir SMTP auth dengan password biasa untuk akun dengan 2FA aktif.

### 2.2 `internal/websocket/hub.go`
Pola standar Go untuk WebSocket hub (broadcast ke banyak client). Hub, Client, dan definisi Event semuanya dalam satu file ini (masing-masing kecil, ~40 baris — belum perlu dipecah):

```go
package websocket

type Hub struct {
	clients    map[*Client]bool
	broadcast  chan []byte
	register   chan *Client
	unregister chan *Client
}

func NewHub() *Hub
func (h *Hub) Run()  // jalan sebagai goroutine, loop selamanya
```

`Hub.Run()` dipanggil sekali di `main.go` sebagai goroutine (`go hub.Run()`), lalu setiap koneksi WebSocket baru didaftarkan ke hub ini.

### 2.3 `internal/middleware/auth.go`
Sudah didesain di percakapan sebelumnya — pastikan middleware ini dipakai di:
- Semua route yang butuh login (logout, me, playlist)
- WebSocket upgrade handler (validasi token dari query param sebelum upgrade koneksi)

### 2.4 Redis Cache — `pkg/cache/redis.go` (`cache.Store` struct + DI)
Cache di-bungkus dalam struct `Store` yang di-inject ke service lewat constructor — konsisten dengan aturan "tidak ada variabel global, semua lewat constructor" di §1 (bukan package-level function):

```go
type Store struct {
	rdb *redis.Client
}

func NewStore(rdb *redis.Client) *Store
func (s *Store) Get(ctx context.Context, key string, dest interface{}) (bool, error)
func (s *Store) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error
func (s *Store) Invalidate(ctx context.Context, key string) error
```

Dipakai di `artist_service.go` / `album_service.go` untuk `GetAll`:
```go
func (s *ArtistService) GetAllArtists() ([]model.Artist, error) {
    var cached []model.Artist
    if found, _ := s.cache.Get(ctx, "artists:all", &cached); found {
        return cached, nil
    }
    artists, err := s.repo.FindAll()
    if err == nil {
        s.cache.Set(ctx, "artists:all", artists, 5*time.Minute)
    }
    return artists, err
}
```
Cache key: `artists:all`, `albums:all` (tanpa prefix `cache:` — redundan, karena Redis instance ini juga menyimpan token yang sudah punya prefix sendiri seperti `refresh_token:`, `reset_token:`). Invalidate cache ini di `CreateArtist`, `UpdateArtist`, `DeleteArtist` (dan analog untuk Album).

### 2.5 CORS (wajib ditambahkan sebelum frontend mulai memanggil backend)
Frontend (`localhost:5173` dev, `localhost:3000` Docker demo) beda origin dari backend (`localhost:9000`) — browser akan memblokir request tanpa konfigurasi CORS. Tambahkan middleware ini di `main.go` sebelum `router.SetupRoutes`:

```go
import "github.com/gofiber/fiber/v2/middleware/cors"

app.Use(cors.New(cors.Config{
	AllowOrigins:     "http://localhost:5173,http://localhost:3000",
	AllowHeaders:     "Origin, Content-Type, Accept, Authorization",
	AllowMethods:     "GET,POST,PUT,DELETE,OPTIONS",
	AllowCredentials: true,
}))
```

Install: `go get github.com/gofiber/fiber/v2/middleware/cors`

---

```
spotify-frontend/
├── src/
│   ├── main.tsx
│   ├── App.tsx
│   ├── pages/
│   │   ├── LoginPage.tsx
│   │   ├── RegisterPage.tsx
│   │   ├── ForgotPasswordPage.tsx
│   │   ├── ResetPasswordPage.tsx
│   │   ├── HomePage.tsx
│   │   ├── ArtistDetailPage.tsx
│   │   └── AlbumDetailPage.tsx
│   ├── components/
│   │   ├── ui/                      # button, input, card — styled Tailwind murni
│   │   ├── SongCard.tsx
│   │   ├── ArtistCard.tsx
│   │   └── Toast.tsx                # untuk notifikasi WebSocket
│   ├── schemas/                     # Zod schema, terpisah dari komponen
│   │   ├── auth.schema.ts           # loginSchema, registerSchema, resetPasswordSchema
│   │   └── ...
│   ├── lib/
│   │   ├── api.ts                   # wrapper axios, auto-attach JWT header + auto-refresh
│   │   └── websocket.ts             # koneksi & listener WebSocket
│   ├── store/                       # Zustand store
│   │   ├── authStore.ts             # user, access token, refresh logic
│   │   └── playerStore.ts           # (opsional) state lagu yang sedang diputar
│   ├── types/
│   │   └── index.ts                 # TypeScript types (Artist, Song, Album, User)
│   └── index.css                    # @import "tailwindcss" + @theme (token warna/font)
├── .env
├── vite.config.ts                   # termasuk plugin @tailwindcss/vite
├── tsconfig.json
├── .oxlintrc.json                   # config Oxlint (bawaan template)
└── package.json
```

### Tech stack frontend (keputusan final)
| Bagian | Pilihan | Catatan |
|---|---|---|
| Scaffold | `npm create vite@latest spotify-frontend -- --template react-compiler-ts` | React Compiler stable (v1.0, Okt 2025) — auto-memoization, kurangi `useMemo`/`useCallback` manual |
| Linter | **Oxlint** (bawaan template, bukan ESLint) | 50-100x lebih cepat, zero-config, cukup untuk skala project ini |
| Styling | Tailwind CSS **v4** via `@tailwindcss/vite` plugin | Config berbasis CSS (`@theme` di `index.css`), **bukan** `tailwind.config.js` seperti Tailwind v3 |
| Font | Inter (`@fontsource/inter`) | Pengganti "Circular" (proprietary Spotify) |
| Validasi form | Zod + `react-hook-form` + `@hookform/resolvers` | Schema di `src/schemas/`, terpisah dari komponen |
| State management | **Zustand** | Dipilih karena boilerplate minim, cocok skala proyek ini dibanding Redux Toolkit |
| HTTP client | Axios | Interceptor untuk auto-attach JWT & auto-refresh saat 401 |
| Routing | React Router | Halaman: Login, Register, Forgot/Reset Password, Home, Artist/Album Detail |

**Penting soal Tailwind v4:** token warna & font (lihat `DESIGN.md`) didefinisikan lewat `@theme { --color-spotify-black: #121212; ... }` langsung di `src/index.css`, bukan di `tailwind.config.js`. Jangan generate `tailwind.config.js` kecuali benar-benar perlu override lanjutan yang tidak bisa lewat `@theme`.

### Prinsip Frontend
- **Zod schema terpisah dari komponen** (`src/schemas/`) — reusable untuk validasi form maupun validasi response API bila perlu
- **`lib/api.ts`** sentralisasi semua HTTP call — termasuk logic auto-refresh token saat dapat 401 (interceptor pattern)
- **Auth state (Zustand `authStore`) terpisah dari UI state** — jangan campur dalam satu store besar
- **Tailwind only** — hindari inline style, hindari CSS module terpisah kecuali benar-benar tidak bisa lewat utility class
- **Ikuti `DESIGN.md`** untuk semua token warna, tipografi, dan pola komponen (card, button, layout 3-panel)

---

## 4. Alur (Sequence) Penting

### 4.1 Login → simpan token → auto-refresh
```
1. User submit form login → validasi Zod di client
2. POST /api/auth/login → dapat { access_token, refresh_token }
3. Simpan access_token di memory (store), refresh_token di httpOnly cookie
   idealnya (atau localStorage jika scope portofolio, dengan catatan risiko XSS)
4. Setiap request API, attach header Authorization: Bearer <access_token>
5. Jika response 401 (access token expired):
   a. Panggil POST /api/auth/refresh dengan refresh_token
   b. Dapat access_token baru, ulangi request yang gagal tadi
   c. Jika refresh juga gagal → redirect ke halaman login
```

### 4.2 Forgot Password → Reset Password
```
1. User submit email di ForgotPasswordPage
2. POST /api/auth/forgot-password { email }
3. Backend: generate random token, simpan di Redis (key: reset_token:{token}, value: user_id, TTL 15 menit)
4. Backend: kirim email via SMTP berisi link, misal:
   https://spotifyclone.dev/reset-password?token=abc123
5. User klik link → buka ResetPasswordPage, ambil token dari query param
6. User submit password baru → POST /api/auth/reset-password { token, new_password }
7. Backend: validasi token di Redis, ambil user_id, update password (hash baru), hapus token dari Redis
```

### 4.3 WebSocket connect + event
```
1. Frontend connect: new WebSocket(`ws://localhost:9000/ws?token=${accessToken}`)
2. Backend: validasi token sebelum upgrade koneksi HTTP → WebSocket
3. Jika valid: koneksi didaftarkan ke Hub, kirim event connection:ack
4. Saat ada Song baru dibuat (REST POST /api/songs berhasil):
   → SongService memanggil hub.Broadcast(event "song:created", data song)
   → Semua client terkoneksi menerima event ini secara real-time
5. Frontend: tampilkan toast notification saat menerima event song:created
```

---

## 5. Docker — Development vs Full-Stack Demo

Dua mode terpisah, jangan dicampur:

**Mode Development (harian):** `docker-compose.yml` di root `spotify-backend` hanya berisi Postgres + Redis. Backend jalan manual (`go run`), frontend jalan manual (`npm run dev`) — biar hot-reload cepat.

**Mode Full-Stack Demo (showcase ke recruiter):** `Dockerfile` multi-stage di `spotify-backend` dan `spotify-frontend`, plus `docker-compose.prod.yml` yang menjalankan seluruh stack (Postgres, Redis, Backend, Frontend via Nginx) dengan satu command:
```
docker compose -f docker-compose.prod.yml up --build
```
Detail lengkap `Dockerfile` dan `docker-compose.prod.yml` — lihat `DOCKER.md`. **Urutan pengerjaan: setup Docker mode ini paling akhir**, setelah backend dan frontend sudah berjalan normal secara manual.

---

## 6. Checklist Implementasi (urutan disarankan)

### Backend (SELESAI ✅ — di-commit per-fitur)
- [x] Tambah `internal/model/user.go`
- [x] Tambah `pkg/jwt/jwt.go`
- [x] Tambah `internal/repository/user_repository.go`
- [x] Tambah `internal/service/auth_service.go` (register, login, refresh, logout, reset password — password logic digabung di sini)
- [x] Tambah `internal/middleware/auth.go`
- [x] Tambah `internal/handler/auth_handler.go` (termasuk forgot/reset password)
- [x] Daftarkan route auth di `router.go`, update `main.go` (wiring + AutoMigrate User)
- [x] Tambah `pkg/mailer/smtp.go` (pakai go-mail, env `SMTP_FROM_EMAIL` + `SMTP_FROM_NAME`)
- [x] Reset password flow via SMTP (di dalam auth_service/auth_handler)
- [x] `pkg/cache/redis.go` dengan `cache.Store` struct (di-inject via DI)
- [x] Terapkan caching di `ArtistService.GetAllArtists` dan `AlbumService.GetAllAlbums` (key `artists:all` / `albums:all`)
- [x] Tambah `internal/websocket/hub.go` (hub + client + event dalam satu file)
- [x] Tambah endpoint `GET /ws` di router dengan middleware auth khusus WebSocket
- [x] Integrasikan broadcast `song:created` di `SongService.CreateSong`
- [x] Test ulang semua endpoint (Auth, forgot/reset password, cache, WebSocket) di Postman/Thunder Client sebelum commit
- [x] Commit per-fitur secara manual (mis. `feat(auth): ...`, `feat(smtp): ...`, `feat(cache): ...`, `feat(websocket): ...`)

### Frontend (SELESAI ✅ — repo terpisah: `spotify-frontend`)
- [x] Scaffold: `npm create vite@latest spotify-frontend -- --template react-compiler-ts`
- [x] Install Tailwind v4 (`@tailwindcss/vite`), konfigurasi `@theme` di `index.css` sesuai `DESIGN.md`
- [x] Install Inter font, Zod, React Router, Axios, Zustand, react-hook-form + resolver
- [x] Bikin struktur folder sesuai §3
- [x] `src/lib/api.ts` — axios instance + interceptor JWT + auto-refresh
- [x] Halaman Auth: Login, Register, Forgot/Reset Password (pakai schema Zod dari `src/schemas/auth.schema.ts`)
- [x] Layout utama: sidebar + top bar + now-playing bar (sesuai `DESIGN.md` §3)
- [x] Halaman Home: list Artist/Album/Song (card grid, sesuai `DESIGN.md` §4)
- [x] Integrasi WebSocket: `src/lib/websocket.ts` + toast notification saat event `song:created`

### Deployment (paling akhir)
- [ ] `Dockerfile` backend (multi-stage: golang builder → alpine runtime)
- [ ] `Dockerfile` frontend (multi-stage: node builder → nginx runtime) + `nginx.conf`
- [ ] `docker-compose.prod.yml` — full stack, env var lewat `.env` terpisah
- [x] Push ke GitHub (masing-masing repo, commit per-fitur)
- [ ] Update README dengan diagram arsitektur
