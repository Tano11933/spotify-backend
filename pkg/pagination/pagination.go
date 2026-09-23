// Package pagination menyatukan cara membaca limit/offset dan bentuk envelope
// response untuk semua endpoint list.
//
// Semua list endpoint memakai bentuk yang sama:
//
//	{ "items": [...], "total": 233, "limit": 20, "offset": 0 }
//
// sehingga frontend cukup menulis satu tipe generik dan satu komponen daftar.
package pagination

import (
	"errors"
	"strconv"
)

const (
	DefaultLimit = 20
	MaxLimit     = 100
)

var (
	ErrInvalidLimit  = errors.New("limit must be a number")
	ErrInvalidOffset = errors.New("offset must be a number")
)

type Params struct {
	Limit  int
	Offset int
}

// Parse membaca limit/offset dari query string.
//
// Aturan yang disepakati:
//   - kosong        → default (limit 20, offset 0)
//   - bukan angka   → error; handler membalas 400 VALIDATION_FAILED
//   - di luar batas → di-clamp (limit 1..100, offset >= 0), bukan error
//
// Clamp dipilih untuk nilai yang "masuk akal tapi berlebihan" supaya client
// tidak pernah menerima error hanya karena minta limit 500.
func Parse(rawLimit, rawOffset string) (Params, error) {
	params := Params{Limit: DefaultLimit, Offset: 0}

	if rawLimit != "" {
		limit, err := strconv.Atoi(rawLimit)
		if err != nil {
			return Params{}, ErrInvalidLimit
		}
		params.Limit = limit
	}

	if rawOffset != "" {
		offset, err := strconv.Atoi(rawOffset)
		if err != nil {
			return Params{}, ErrInvalidOffset
		}
		params.Offset = offset
	}

	if params.Limit < 1 {
		params.Limit = 1
	}
	if params.Limit > MaxLimit {
		params.Limit = MaxLimit
	}
	if params.Offset < 0 {
		params.Offset = 0
	}

	return params, nil
}

// Page adalah envelope response list.
type Page[T any] struct {
	Items  []T   `json:"items"`
	Total  int64 `json:"total"`
	Limit  int   `json:"limit"`
	Offset int   `json:"offset"`
}

// NewPage membangun envelope. Items dijamin slice kosong (bukan null) supaya
// frontend tidak perlu menangani dua bentuk untuk "hasil kosong".
func NewPage[T any](items []T, total int64, params Params) Page[T] {
	if items == nil {
		items = []T{}
	}

	return Page[T]{
		Items:  items,
		Total:  total,
		Limit:  params.Limit,
		Offset: params.Offset,
	}
}

// Slice memotong list yang sudah dimuat penuh — dipakai service yang mengambil
// data dari cache (artist & album), supaya cache tetap menyimpan list utuh dan
// tidak perlu key per halaman.
func Slice[T any](items []T, params Params) []T {
	start := params.Offset
	if start > len(items) {
		start = len(items)
	}

	end := start + params.Limit
	if end > len(items) {
		end = len(items)
	}

	return items[start:end]
}
