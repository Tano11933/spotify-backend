# Spotify Clone — Backend API

REST + WebSocket API untuk aplikasi streaming musik, dibangun dengan Go. Proyek
portofolio dengan fokus pada arsitektur multi-layer, autentikasi JWT yang bisa
dicabut, caching Redis, dan real-time WebSocket.

> **Frontend:** https://github.com/Tano11933/spotify-frontend — aplikasi React
> yang mengonsumsi API ini (katalog, playlist, player, event real-time).

## Tech Stack

| Komponen | Pilihan |
|---|---|
| Bahasa | Go 1.26 |
| HTTP framework | Fiber v2 |
| ORM | GORM |
| Database | PostgreSQL 16 |
| Cache & token store | Redis 7 |
| Auth | JWT (HS256), access + refresh token, bcrypt cost 12 |
| Real-time | WebSocket (`gofiber/contrib/websocket`) |
| Email | SMTP (`wneessen/go-mail`) |
| Validasi | `go-playground/validator` |

## Arsitektur

```
Request
   ↓
[Middleware]   auth JWT, rate limit (Redis)
   ↓
[Handler]      parse body, validasi, format response, map error → status HTTP
   ↓
[Service]      business logic, keputusan caching, orkestrasi repository
   ↓
[Repository]   query Postgres (GORM) / Redis
   ↓
[Model]        struct tabel + DTO request/response
```

Aturan yang dipegang konsisten:

- Handler **tidak boleh** berisi query database atau business logic
- Service **tidak boleh** menerima atau mengetahui `fiber.Ctx` — bisa dites tanpa HTTP
- Repository **tidak boleh** berisi business logic
- Semua dependency di-inject lewat constructor `NewXxx(...)` — **tidak ada variabel global**
- Field sensitif selalu `json:"-"` di struct model

```
cmd/api/main.go                    entrypoint, wiring semua dependency
internal/
├── handler/                       artist, song, album, auth, playlist, ws, errors
├── service/                       + mail_service, cache (key builders)
├── repository/                    + user, token (Redis), playlist, errors (sentinel)
├── model/                         artist, song, album, user, playlist, auth_dto
├── middleware/                    auth.go (JWT + admin guard), rate_limit.go
├── router/router.go               semua definisi route
└── websocket/hub.go               connection manager berbasis channel
pkg/
├── database/postgres.go           koneksi GORM
├── cache/                         redis.go (koneksi), store.go (helper JSON)
├── jwt/jwt.go                     generate & verify token
├── mailer/smtp.go                 SMTPMailer + LogMailer
└── validator/validator.go         wrapper go-playground/validator
```

---

## Setup dari Nol

### Prasyarat

- Go 1.26+
- Docker Desktop (untuk Postgres & Redis)

### 1. Jalankan database

```bash
docker compose up -d
docker ps      # pastikan spotify-postgres dan spotify-redis berstatus Up
```

### 2. Siapkan `.env`

```bash
cp .env.example .env
```

**`JWT_SECRET` dan `JWT_REFRESH_SECRET` wajib diisi** — aplikasi menolak start
kalau salah satu kosong. Ini disengaja: secret kosong berarti siapa pun bisa
menerbitkan token palsu yang valid. Generate dua nilai **berbeda**:

```bash
# Git Bash / WSL
openssl rand -base64 32

# PowerShell
$b=[byte[]]::new(32);[System.Security.Cryptography.RandomNumberGenerator]::Create().GetBytes($b);[Convert]::ToBase64String($b)
```

`SMTP_*` boleh dibiarkan kosong. Kalau `SMTP_HOST` kosong, aplikasi memakai
**LogMailer**: email reset password dicetak ke terminal alih-alih dikirim, jadi
seluruh alur reset bisa diuji tanpa kredensial SMTP. Untuk kirim email sungguhan,
isi dengan kredensial [Mailtrap](https://mailtrap.io) (gratis untuk dev).

### 3. Jalankan server

```bash
go mod download
go run ./cmd/api
```

Migrasi tabel berjalan otomatis via GORM AutoMigrate saat startup. Tidak ada
perintah migrasi terpisah.

```bash
curl http://127.0.0.1:9000/health
# {"status":"ok","message":"Spotify backend is running"}
```

> **Windows: pakai `127.0.0.1`, jangan `localhost`.** `localhost` di Windows
> resolve ke `::1` (IPv6) lebih dulu sementara server bind ke IPv4 — menambah
> ~200ms per request, dan koneksi WebSocket dari Node gagal total.

### 4. Buat akun admin

Endpoint tulis katalog (artist/album/song) hanya bisa diakses role `admin`, dan
**tidak ada endpoint untuk promote diri sendiri jadi admin** — itu disengaja.
Register dulu lewat API, lalu naikkan rolenya lewat SQL:

```bash
docker exec spotify-postgres psql -U spotify -d spotify_db \
  -c "UPDATE users SET role='admin' WHERE email='emailmu@example.com';"
```

Login ulang setelahnya — role dibaca dari database saat login.

### 5. Jalankan test

```bash
go test ./...

# Race detector (butuh gcc; di Windows biasanya gagal, jalankan di Docker):
docker run --rm -v "${PWD}:/app" -w /app golang:1.26 go test -race ./...
```

---

## Environment Variables

| Variable | Wajib | Default | Keterangan |
|---|---|---|---|
| `DB_HOST` `DB_PORT` `DB_USER` `DB_PASSWORD` `DB_NAME` | ✅ | — | Koneksi Postgres |
| `REDIS_HOST` `REDIS_PORT` | ✅ | — | Koneksi Redis |
| `APP_PORT` | — | `9000` | Port HTTP server |
| `CACHE_TTL` | — | `5m` | TTL cache artist & album |
| `JWT_SECRET` | ✅ | — | Secret access token — **app tidak start kalau kosong** |
| `JWT_REFRESH_SECRET` | ✅ | — | Secret refresh token, harus **berbeda** dari di atas |
| `JWT_ACCESS_TTL` | — | `15m` | Masa berlaku access token |
| `JWT_REFRESH_TTL` | — | `168h` | Masa berlaku refresh token (7 hari) |
| `RESET_TOKEN_TTL` | — | `15m` | Masa berlaku token reset password |
| `SMTP_HOST` | — | *(kosong)* | Kosong → pakai LogMailer (cetak ke terminal) |
| `SMTP_PORT` `SMTP_USERNAME` `SMTP_PASSWORD` | — | — | Kredensial SMTP |
| `SMTP_ENCRYPTION` | — | `starttls` | `starttls` \| `ssltls` \| `none` |
| `SMTP_FROM_EMAIL` `SMTP_FROM_NAME` | — | — | Pengirim email |
| `FRONTEND_URL` | — | — | Basis link reset password di email |
| `CORS_ALLOWED_ORIGINS` | — | `http://localhost:5173,http://localhost:3000` | Origin yang boleh memanggil API dari browser, dipisah koma. **Tidak boleh `*`** — app menolak start |

Format durasi mengikuti Go: `30s`, `15m`, `168h`, `5m30s`.

### CORS

Frontend (`localhost:5173` dev / `localhost:3000` Docker demo) beda origin dari
backend (`localhost:9000`) — port berbeda saja sudah cukup membuat browser
menganggapnya origin lain, dan tanpa header CORS seluruh response akan diblokir.

Middleware CORS dipasang di `main.go` **sebelum** route didaftarkan. Urutan itu
wajib: Fiber menjalankan middleware sesuai urutan pendaftaran, jadi kalau
dipasang belakangan, request preflight `OPTIONS` akan lebih dulu tertangkap route
matcher dan dibalas `405`.

Header `Authorization` ada di `AllowHeaders`. Tanpa itu, preflight untuk request
ber-token gagal dan semua endpoint terproteksi tidak bisa dipanggil dari browser
— gejalanya membingungkan karena request yang sama tetap sukses lewat curl.

> **CORS bukan mekanisme keamanan server.** Ia hanya instruksi ke browser tentang
> siapa yang boleh *membaca* response. curl, Postman, dan skrip apa pun
> mengabaikannya. Yang mengamankan endpoint tetap middleware auth dan rate
> limiter.

**WebSocket tidak ikut kebijakan CORS ini** — browser tidak menerapkan CORS pada
handshake WebSocket. `gofiber/contrib/websocket` punya pengecekan `Origin`
terpisah, dan di proyek ini sengaja dibiarkan permisif: autentikasi WS memakai
token di query string, bukan cookie, jadi halaman penyerang tidak punya
kredensial ambient untuk disalahgunakan (Cross-Site WebSocket Hijacking baru
relevan kalau auth-nya berbasis cookie). Membatasi origin justru akan memutus
client non-browser seperti Postman dan skrip test, yang memang tidak mengirim
header `Origin`.

---

## API Reference

Base URL: `http://127.0.0.1:9000`

**Format error selalu sama:** `{"error": "pesan singkat"}`

**Autentikasi:** kirim header `Authorization: Bearer <access_token>`.

Kolom akses:
- 🌐 publik
- 🔒 butuh login
- 👑 butuh role `admin`

### Health

| Method | Endpoint | Akses |
|---|---|---|
| GET | `/health` | 🌐 |

### Auth

| Method | Endpoint | Akses | Rate limit |
|---|---|---|---|
| POST | `/api/auth/register` | 🌐 | 5 / jam per IP |
| POST | `/api/auth/login` | 🌐 | 10 / 5 menit per IP |
| POST | `/api/auth/refresh` | 🌐 | 30 / jam per IP |
| POST | `/api/auth/logout` | 🔒 | — |
| GET | `/api/auth/me` | 🔒 | — |
| POST | `/api/auth/forgot-password` | 🌐 | 3 / jam per **email** + 10 / jam per IP |
| POST | `/api/auth/reset-password` | 🌐 | 10 / jam per IP |

Respons `429` menyertakan header `Retry-After` dan `X-RateLimit-Remaining`.

<details>
<summary><b>POST /api/auth/register</b></summary>

Email dinormalisasi (trim + lowercase) sebelum divalidasi. Role selalu dipaksa
`user` — mengirim `"role":"admin"` di body tidak berpengaruh.

```json
{ "name": "Gabriel", "email": "gabriel@example.com", "password": "rahasia123" }
```
`201`
```json
{
  "id": "e444d641-d93f-4269-b623-4fb898648006",
  "name": "Gabriel", "email": "gabriel@example.com", "role": "user",
  "created_at": "2026-08-05T00:55:28Z", "updated_at": "2026-08-05T00:55:28Z"
}
```
| Kode | Sebab |
|---|---|
| `400` | Validasi gagal (nama < 2 char, email invalid, password < 8 atau > 72 byte) |
| `409` | Email sudah terdaftar |
</details>

<details>
<summary><b>POST /api/auth/login</b></summary>

```json
{ "email": "gabriel@example.com", "password": "rahasia123" }
```
`200`
```json
{
  "user": { "id": "e444d641-...", "name": "Gabriel", "email": "gabriel@example.com", "role": "user" },
  "tokens": {
    "access_token": "eyJhbGciOiJIUzI1NiIs...",
    "refresh_token": "eyJhbGciOiJIUzI1NiIs...",
    "token_type": "Bearer",
    "expires_in": 900
  }
}
```
`expires_in` dalam detik, merujuk **access token** (konvensi OAuth 2.0).

`401` untuk password salah **maupun** email tidak terdaftar — pesannya identik
(`invalid email or password`) supaya endpoint ini tidak bisa dipakai mengecek
email siapa yang punya akun.
</details>

<details>
<summary><b>POST /api/auth/refresh</b></summary>

```json
{ "refresh_token": "eyJhbGciOiJIUzI1NiIs..." }
```
`200` → `TokenPair` baru (bentuk sama seperti `tokens` di login).

**Refresh token dirotasi:** token lama langsung tidak berlaku setelah dipakai.
Memakainya lagi → `401`. Ini membatasi kerusakan kalau refresh token tercuri —
hanya satu pihak yang bisa memakainya, dan pihak kedua yang tertolak jadi sinyal
bahwa token sudah bocor.

`401` juga kalau token sudah dicabut lewat logout (dicek ke Redis, bukan cuma
signature).
</details>

<details>
<summary><b>POST /api/auth/logout</b> · 🔒</summary>

Tanpa body. Menghapus refresh token user dari Redis.

`200` → `{"message":"logged out successfully"}`

⚠️ **Access token yang sudah beredar tidak bisa dicabut** — ia tetap valid sampai
`exp`-nya lewat (maks 15 menit). Itu konsekuensi bawaan JWT stateless, dan
justru alasan access token dibuat berumur pendek.
</details>

<details>
<summary><b>GET /api/auth/me</b> · 🔒</summary>

`200` → objek user. `password_hash` tidak pernah muncul di response manapun
(`json:"-"` di model).
</details>

<details>
<summary><b>POST /api/auth/forgot-password</b></summary>

```json
{ "email": "gabriel@example.com" }
```
`200` — **selalu**, bahkan kalau email tidak terdaftar:
```json
{ "message": "if the email is registered, a password reset link has been sent" }
```

Email dikirim di goroutine terpisah, bukan sinkron. Selain demi UX, ini juga
menutup timing oracle: kalau pengiriman ditunggu, email terdaftar akan merespons
~3 detik sementara yang tidak terdaftar instan — selisih itu membocorkan
informasi yang justru sedang disembunyikan.

Token reset: 32 byte dari `crypto/rand`, base64url, TTL 15 menit, **sekali pakai**.
</details>

<details>
<summary><b>POST /api/auth/reset-password</b></summary>

```json
{ "token": "SXU80NrcuXDiI4-BlIikN9qfPx2HvvxXAOKRECweGCQ", "new_password": "passwordbaru123" }
```
`200` → `{"message":"password has been reset, please log in again"}`

Setelah reset, semua sesi berjalan dicabut — kalau alasan reset adalah akun
dibajak, membiarkan sesi lama hidup berarti pembajaknya tetap masuk.

| Kode | Sebab |
|---|---|
| `400` | Password baru < 8 karakter atau > 72 byte |
| `401` | Token tidak valid, kedaluwarsa, atau sudah terpakai |

Validasi password dijalankan **sebelum** token dihabiskan, jadi salah ketik
password tidak membuang token.
</details>

### Artist

| Method | Endpoint | Akses |
|---|---|---|
| GET | `/api/artists` | 🌐 *(cache 5 menit)* |
| GET | `/api/artists/:id` | 🌐 *(cache 5 menit)* |
| GET | `/api/artists/:artistId/songs` | 🌐 |
| POST | `/api/artists` | 👑 |
| PUT | `/api/artists/:id` | 👑 |
| DELETE | `/api/artists/:id` | 👑 |

```json
// POST/PUT body
{ "name": "Hindia", "bio": "Solo project", "image_url": "https://example.com/h.jpg" }
```
`name` wajib (2–100 char), `bio` maks 1000 char, `image_url` harus URL valid.

### Album

| Method | Endpoint | Akses |
|---|---|---|
| GET | `/api/albums` | 🌐 *(cache 5 menit)* |
| GET | `/api/albums/:id` | 🌐 *(cache 5 menit, menyertakan `songs`)* |
| POST | `/api/albums` | 👑 |
| PUT | `/api/albums/:id` | 👑 |
| DELETE | `/api/albums/:id` | 👑 |

```json
{ "title": "Menari Dengan Bayangan", "artist_id": 3,
  "release_date": "2019-04-05T00:00:00Z", "cover_url": "https://example.com/c.jpg" }
```
`422` kalau `artist_id` menunjuk artist yang tidak ada.

### Song

| Method | Endpoint | Akses |
|---|---|---|
| GET | `/api/songs` | 🌐 |
| GET | `/api/songs/:id` | 🌐 |
| POST | `/api/songs` | 👑 → broadcast `song:created` |
| PUT | `/api/songs/:id` | 👑 |
| DELETE | `/api/songs/:id` | 👑 |

```json
{ "title": "Evaluasi", "duration": 240, "artist_id": 3, "album_id": 2,
  "file_url": "https://example.com/evaluasi.mp3" }
```
`album_id` opsional (nullable — lagu bisa berupa single). `duration` dalam detik.

Lagu sendiri tidak di-cache (write-light tidak berlaku di sini), tapi setiap
perubahan lagu **menginvalidasi cache detail album** terkait — termasuk album
lama **dan** album baru saat lagu dipindahkan antar album.

### Playlist

Semua endpoint butuh login. Playlist adalah data milik user, bukan katalog
publik — tidak ada endpoint admin di sini, tapi tidak ada yang bisa diakses
anonim juga.

| Method | Endpoint | Akses |
|---|---|---|
| GET | `/api/playlists` | 🔒 milik sendiri |
| GET | `/api/playlists/public` | 🔒 semua playlist `is_public: true` |
| GET | `/api/playlists/:id` | 🔒 pemilik, atau kalau publik |
| POST | `/api/playlists` | 🔒 |
| PUT | `/api/playlists/:id` | 🔒 pemilik |
| DELETE | `/api/playlists/:id` | 🔒 pemilik |
| POST | `/api/playlists/:id/songs` | 🔒 pemilik |
| DELETE | `/api/playlists/:id/songs/:songId` | 🔒 pemilik |

```json
// POST — user_id diambil dari token, TIDAK dari body
{ "name": "Lagu Sedih", "description": "Buat hujan", "is_public": false }

// PUT — semua field opsional, yang tidak dikirim tidak berubah
{ "is_public": true }

// POST /:id/songs
{ "song_id": 4 }
```

| Kode | Sebab |
|---|---|
| `403` | Mengubah playlist milik orang lain |
| `404` | Playlist tidak ada, **atau** playlist privat milik orang lain |
| `404` | Lagu tidak ada / lagu tidak ada di playlist ini |
| `409` | Lagu sudah ada di playlist ini |

> **Kenapa `GET` playlist privat orang lain balas `404` bukan `403`?** Karena
> `403` memberi tahu bahwa playlist dengan ID itu **ada** — itu sudah kebocoran
> informasi. Untuk operasi tulis, `403` memang dipakai: pemanggil sudah
> menunjukkan niat mengubah sesuatu yang spesifik, dan pesan jelas lebih berguna.

### WebSocket

```
GET /ws?token=<access_token>
```

Token lewat **query string**, bukan header — browser tidak menyediakan cara
mengirim header `Authorization` pada `new WebSocket(url)`. Trade-off-nya: URL
bisa tercatat di access log, karena itu varian middleware ini hanya dipasang di
route `/ws`, tidak di endpoint REST.

Handshake tanpa token valid → `401`, tidak pernah naik ke `101`.

**Format pesan (dua arah):**
```json
{ "type": "...", "payload": { }, "user_id": "...", "at": "2026-08-04T18:03:17Z" }
```

**Client → server**

| `type` | Payload | Efek |
|---|---|---|
| `ping` | — | Server balas `{"type":"pong"}` |
| `song:playing` | `{"song_id": 4}` | Verifikasi lagu ada di DB, lalu broadcast ke **semua client lain** |

**Server → client**

| `type` | Kapan | Payload |
|---|---|---|
| `connection:ack` | Segera setelah token tervalidasi | — (`user_id` di level atas) |
| `pong` | Balasan `ping` | — |
| `song:playing` | User lain memutar lagu | `{"song": {...}}` + `user_id` di level atas |
| `song:created` | Lagu baru dibuat via REST | Objek song |
| `error` | Pesan tidak valid | `{"message": "..."}` |

`connection:ack` adalah sinyal yang harus ditunggu frontend sebelum mulai
mengirim event. Event `open` milik WebSocket hanya berarti socket-nya terbuka —
bukan berarti token diterima; tanpa ack, satu-satunya cara mengetahui token
ditolak adalah menunggu koneksi tertutup begitu saja.

`user_id` **selalu ditentukan server** dari token, tidak pernah dari isi pesan
client — kalau tidak, siapa pun bisa menyiarkan event atas nama orang lain.

Contoh:
```js
const ws = new WebSocket(`ws://127.0.0.1:9000/ws?token=${accessToken}`);

ws.onmessage = (e) => {
  const event = JSON.parse(e.data);

  // Tunggu ack sebelum mengirim apa pun — `open` saja tidak menjamin
  // token diterima.
  if (event.type === "connection:ack") {
    ws.send(JSON.stringify({ type: "song:playing", payload: { song_id: 4 } }));
  }
};
```

Hub-nya in-memory: satu goroutine memiliki daftar koneksi dan yang lain
berkomunikasi lewat channel, jadi tidak ada mutex sama sekali. Client yang
terlalu lambat mengonsumsi pesan otomatis diputus supaya tidak membekukan
broadcast untuk semua orang.

---

## Catatan Desain

**Read publik, write admin-only.** `GET` katalog terbuka supaya halaman depan
frontend bisa menampilkan konten tanpa login — dan endpoint itulah yang di-cache.
Menambah artist/album/lagu adalah pekerjaan kurasi, bukan sesuatu yang boleh
dilakukan user yang baru mendaftar.

**Satu sesi aktif per user.** Refresh token disimpan dengan key
`refresh_token:{user_id}` — satu key per user. Login di device kedua menimpa key
itu dan mematikan sesi di device pertama. Untuk multi-device, formatnya perlu
jadi `refresh_token:{user_id}:{jti}`.

**Cache fail-open.** Redis mati membuat aplikasi **lambat**, bukan **rusak**:
error cache di-log lalu diabaikan, request tetap dilayani dari Postgres. Rate
limiter juga fail-open — Redis mati tidak boleh mematikan endpoint login total.

**User ID pakai UUID, katalog pakai `uint`.** Campur tipe ini keputusan sadar:
ID artist/album/song tidak sensitif kalau sekuensial, tapi ID user muncul di JWT
claim dan berpotensi di URL — UUID menutup enumerasi (`/api/users/1`, `/2`, ...)
dan menyembunyikan jumlah user.

**Tidak ada soft delete.** Konsisten di semua model. Untuk User, soft delete juga
bentrok dengan unique index email — baris terhapus tetap menahan email itu, dan
memperbaikinya butuh partial unique index.

## Testing

```bash
go test ./...
```

| Package | Cakupan |
|---|---|
| `pkg/jwt` | Round-trip token, tolak: secret salah, kedaluwarsa, payload diubah, `alg: none`, tipe token tertukar; keunikan `jti` |
| `internal/service` (auth) | Register (hash bcrypt, normalisasi email, role dipaksa, duplikat, batas panjang password rune vs byte), login (error identik untuk password salah & email tak dikenal), refresh (rotasi + pencabutan), forgot/reset (sekali pakai, urutan validasi) |
| `internal/service` (mail) | Pembentukan URL reset, escaping XSS di nama user, encoding token |
| `internal/websocket` | Broadcast, exclude pengirim, buang client lambat, **akses konkuren (`-race`)**, shutdown |

Test auth service dan mail service jalan **tanpa Postgres dan tanpa Redis** —
dependency-nya interface (`UserStore`, `TokenStore`, `Mailer`) dengan implementasi
tiruan in-memory di file test.

## Belum Dikerjakan

- Redis Pub/Sub sebagai broker WebSocket — hub in-memory tidak sinkron kalau
  backend di-scale ke beberapa instance
- Integration test yang menyentuh Postgres/Redis sungguhan
- Handler layer belum punya test
- Upload file audio (asumsi file sudah tersedia via URL eksternal)
- Pagination & pencarian katalog — list endpoint masih mengembalikan seluruh isi tabel
- Docker full-stack demo (`Dockerfile` backend & frontend +
  `docker-compose.prod.yml`) — lihat `DOCKER.md`
