# Sesi login per perangkat

Login membuat session ID UUID baru. Access token dan refresh token membawa
claim `sid` yang sama; masing-masing tetap memiliki `jti` acak sendiri.
Refresh mempertahankan sid dan merotasi token secara atomik. Login berikutnya
tidak menimpa login perangkat lain. Tab pada satu browser/profil biasanya
berbagi sesi melalui localStorage; satu baris bukan identitas perangkat keras.

## Kontrak HTTP

Semua endpoint pengelolaan sesi membutuhkan Bearer access token dengan sid
yang masih aktif. Kepemilikan selalu dari sub JWT, bukan parameter user.

| Method | Path | Hasil |
| --- | --- | --- |
| GET | `/api/auth/sessions` | 200, `{ "items": [LoginSession] }` |
| DELETE | `/api/auth/sessions/:sessionId` | 200, message; dapat mencabut sesi sendiri |
| DELETE | `/api/auth/sessions` | 200, message; mencabut semua termasuk sesi pemanggil |
| POST | `/api/auth/logout` | 200, message; hanya sesi sid pemanggil, idempoten |

LoginSession berisi id, user_agent, ip_address, created_at, last_used_at,
expires_at dan current. Waktu dikirim dalam RFC3339 UTC. Urutan daftar adalah
last_used_at terbaru, lalu ID untuk tie. Last used diperbarui saat login atau
refresh, bukan setiap lagu/request. Hash token dan versi kredensial tidak
pernah masuk response. Target sesi asing/tidak ditemukan sama-sama 404;
UUID salah/zero 400. Tanpa token, sid, atau sesi aktif menghasilkan 401.

User-Agent dibatasi 256 rune dan control character dibersihkan. IP berasal
dari alamat transport yang dilihat Fiber, bukan geolokasi; reverse proxy/NAT
dapat membuat beberapa perangkat memiliki alamat sama. User-Agent dapat
dipalsukan. Keduanya petunjuk, bukan bukti identitas perangkat.

## Penyimpanan dan atomisitas

`auth:{user_id}:sessions` adalah Redis hash dengan field session ID. Record
menyimpan SHA-256 refresh token, SHA-256 hash password saat login, metadata,
serta timestamp milidetik. Token mentah tidak disimpan. Lua memvalidasi digest
lama sebelum mengganti digest dan waktu expiry; satu token lama hanya menang
satu kali ketika request bersamaan.

Setiap sesi memiliki waktu expiry sendiri (JWT_REFRESH_TTL). Hash Redis diberi
TTL sampai expiry sesi terlama. Field yang lebih cepat expired ditolak saat
dibaca/dirotasi dan dipangkas pada operasi berikutnya. Expiry satu sesi tidak
menghapus sesi lain yang lebih lama berlaku. Tidak ada janji bahwa setiap field
expired langsung hilang dari memori Redis pada detik yang sama.

`auth:{user_id}:generation` menyimpan satu penanda pencabutan per akun.
Revoke-all merubah penanda dan menghapus hash sesi dalam satu script. Login
membaca penanda sebelum memeriksa password dan memeriksa ulang hash password
sebelum membuat sesi; create ditolak jika penanda sudah berubah. Ini mencegah
login lama yang sedang berjalan membuat sesi lagi setelah revoke-all/reset.
Penanda tidak diberi TTL agar nilainya tidak kembali ke keadaan awal. Jumlahnya
satu key per akun, tanpa riwayat pencabutan. Kedua key memakai hash tag user
yang sama untuk operasi Lua. Redis tetap dependency autentikasi, bukan cache
yang boleh diabaikan saat gagal.

Pencabutan satu sesi dan validasi sesi pemanggil berada pada script yang sama.
Pemanggil yang sudah dicabut tidak dapat mencabut sesi lain. Logout biasa hanya
menargetkan sid sendiri, termasuk jika request-nya selesai setelah login baru.

## Reset password dan batas revocation

Reset password memperbarui hash password lalu mencabut seluruh sesi dan
mengganti generation. Jika pencabutan gagal, endpoint mengembalikan 500,
bukan mengklaim semua sesi sudah dicabut. Password mungkin sudah berubah dan
reset token sudah terpakai; gunakan password baru atau minta link reset baru.
Refresh juga memeriksa credential digest terhadap password terbaru, sehingga
record lama tidak dapat terus dipakai setelah password berubah, meskipun
cleanup Redis gagal. SQL dan Redis bukan satu transaksi terdistribusi.

API biasa masih memvalidasi JWT secara stateless. Pencabutan mematikan refresh
dan pengelolaan sesi; access token yang sudah diterbitkan tetap dapat dipakai
sampai exp (default 15 menit, sesuai konfigurasi). Koneksi WebSocket yang sudah
terbuka tidak diputus server berdasarkan sid pada tahap ini. Klien sendiri
menutup koneksinya ketika sesi lokal dibersihkan. Jangan menjanjikan pemutusan
semua koneksi perangkat lain secara langsung.

## Transisi versi lama

Refresh token tanpa sid selalu ditolak 401, walaupun key lama
`refresh_token:user_id` masih ada dan signature JWT sah. Pengguna sebelum
perubahan ini perlu login ulang. Access token lama tetap bekerja pada API
biasa sampai expired, tetapi tidak dapat mengelola sesi; logout lama hanya
membersihkan klien dan tidak mencabut login baru yang memiliki sid.

Key refresh lama tidak dibaca lagi dan dibiarkan expired sesuai TTL aslinya.
Tidak ada migrasi SQL atau pencabutan seluruh pengguna lewat scan Redis.
Deploy revisi ini pada seluruh instance; jangan mencampurnya dengan server
lama yang masih membaca key legacy. Rollback harus memperhitungkan key legacy
yang belum expired. Key client `spotify-clone.auth-session` tetap kompatibel:
id-nya adalah identitas koordinasi tab, bukan sid backend.

## Verifikasi

```powershell
go test ./...
go vet ./...
go test -tags=integration -count=1 ./test/integration/...
```

Cakupan: JWT sid, login independen, rotasi bersamaan, hash token, metadata,
kepemilikan, revoke satu/semua, sid lama, expiry berbeda, generation login lama,
reset password, serta kegagalan pencabutan setelah password sudah berubah.
