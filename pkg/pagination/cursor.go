package pagination

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// ErrInvalidCursor dikembalikan ParseCursor kalau cursor tidak bisa diurai.
// Handler memetakannya ke 400 VALIDATION_FAILED.
var ErrInvalidCursor = errors.New("cursor tidak valid")

// Cursor adalah posisi terakhir yang sudah dibaca, dipakai keyset pagination:
// halaman berikutnya mengambil baris yang lebih tua dari pasangan
// (waktu, id). Tie-breaker id diperlukan karena waktu saja bisa kembar, dan
// tanpa itu baris di batas halaman bisa terlewat atau terkirim dua kali.
//
// Bentuknya OPAQUE bagi client (base64 dari JSON): isinya detail kolom
// pengurutan, dan client tidak boleh menyusunnya sendiri. Ia juga TIDAK
// ditandatangani, jadi bukan alat otorisasi; mengubah isinya hanya
// menghasilkan halaman yang salah, bukan akses ke data milik user lain
// (query tetap difilter per user).
type Cursor struct {
	Time time.Time `json:"t"`
	ID   uint64    `json:"i"`
}

// EncodeCursor mengubah posisi menjadi string yang aman dikirim lewat query.
func EncodeCursor(cursor Cursor) string {
	raw, err := json.Marshal(cursor)
	if err != nil {
		// Struct ini tidak mungkin gagal di-marshal; kalau terjadi, lebih baik
		// ketahuan saat development daripada mengirim cursor kosong diam-diam.
		panic(fmt.Sprintf("pagination: marshal cursor: %v", err))
	}
	return base64.RawURLEncoding.EncodeToString(raw)
}

// ParseCursor mengurai cursor dari query. String kosong berarti "mulai dari
// awal" dan mengembalikan nil tanpa error.
func ParseCursor(raw string) (*Cursor, error) {
	if raw == "" {
		return nil, nil
	}

	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		return nil, ErrInvalidCursor
	}

	var cursor Cursor
	if err := json.Unmarshal(decoded, &cursor); err != nil {
		return nil, ErrInvalidCursor
	}
	if cursor.Time.IsZero() || cursor.ID == 0 {
		return nil, ErrInvalidCursor
	}
	return &cursor, nil
}

// CursorPage adalah envelope untuk daftar yang mendukung DUA mode sekaligus:
// offset (perilaku lama, tetap jalan) dan cursor (baru). `next_cursor` hanya
// muncul kalau masih ada halaman berikutnya, jadi client lama tidak
// terpengaruh oleh field tambahan ini.
type CursorPage[T any] struct {
	Page[T]
	NextCursor string `json:"next_cursor,omitempty"`
}

// NewCursorPage membangun envelope halaman + cursor halaman berikutnya.
func NewCursorPage[T any](items []T, total int64, params Params, nextCursor string) CursorPage[T] {
	return CursorPage[T]{
		Page:       NewPage(items, total, params),
		NextCursor: nextCursor,
	}
}
