package model

import "strings"

// File ini berisi DTO (Data Transfer Object) untuk alur autentikasi — bentuk
// data yang masuk dari request dan keluar sebagai response.
//
// Kenapa dipisah dari struct User, padahal Artist/Song/Album di-bind langsung
// dari JSON ke model-nya? Dua alasan:
//
//  1. Field User.PasswordHash bertanda `json:"-"`, jadi c.BodyParser() secara
//     harfiah TIDAK BISA mengisinya dari JSON request. Register butuh field
//     `password` yang tidak ada di entity — memang harus struct terpisah.
//
//  2. Keamanan: kalau register di-bind langsung ke model.User, penyerang cukup
//     mengirim {"role":"admin"} dan langsung jadi admin. Ini kelas bug bernama
//     mass assignment vulnerability (di Laravel dicegah pakai $fillable).
//     DTO menutupnya secara struktural: field Role tidak ada di RegisterRequest,
//     jadi tidak ada jalan untuk mengirimnya.
//
// Batas max=72 pada password bukan angka sembarangan: bcrypt hanya memproses
// 72 byte pertama dan implementasi Go mengembalikan error kalau input lebih
// panjang. Memvalidasinya di sini membuat user dapat 400 yang jelas, bukan 500.

// Normalize dipanggil handler SEBELUM validasi.
//
// Urutannya penting dan pernah salah: kalau validasi jalan lebih dulu, email
// " gabriel@example.com " (dengan spasi, seperti yang sering terjadi kalau user
// copy-paste) ditolak 400 oleh tag `email` padahal isinya sah. Membersihkan
// dulu, memvalidasi kemudian.
//
// Password TIDAK di-trim. Spasi adalah karakter yang sah di dalam password —
// memangkasnya diam-diam berarti password yang didaftarkan berbeda dari yang
// diketik user, dan login-nya akan gagal tanpa penjelasan.
func (r *RegisterRequest) Normalize() {
	r.Name = strings.TrimSpace(r.Name)
	r.Email = strings.ToLower(strings.TrimSpace(r.Email))
}

type RegisterRequest struct {
	Name     string `json:"name" validate:"required,min=2,max=100"`
	Email    string `json:"email" validate:"required,email,max=255"`
	Password string `json:"password" validate:"required,min=8,max=72"`
}

func (r *LoginRequest) Normalize() {
	r.Email = strings.ToLower(strings.TrimSpace(r.Email))
}

type LoginRequest struct {
	Email string `json:"email" validate:"required,email"`
	Password string `json:"password" validate:"required"`
}

type RefreshRequest struct {
	RefreshToken string `json:"refresh_token" validate:"required"`
}

func (r *ForgotPasswordRequest) Normalize() {
	r.Email = strings.ToLower(strings.TrimSpace(r.Email))
}

type ForgotPasswordRequest struct {
	Email string `json:"email" validate:"required,email"`
}

type ResetPasswordRequest struct {
	Token       string `json:"token" validate:"required"`
	NewPassword string `json:"new_password" validate:"required,min=8,max=72"`
}

type TokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
}

type AuthResponse struct {
	User   *User      `json:"user"`
	Tokens *TokenPair `json:"tokens"`
}

type MessageResponse struct {
	Message string `json:"message"`
}
