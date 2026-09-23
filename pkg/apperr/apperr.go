// Package apperr menyatukan kode error machine-readable yang dikirim bersama
// pesan error ke client.
//
// Bentuk response tetap `{"error": "..."}` seperti sebelumnya — field `code`
// bersifat ADITIF, jadi frontend yang hanya membaca `error` tidak rusak.
// Dengan adanya kode, frontend bisa bereaksi berbeda per jenis kegagalan
// (mis. menampilkan halaman login saat UNAUTHORIZED, bukan toast generik)
// tanpa mencocokkan teks pesan yang bisa berubah kapan saja.
package apperr

type Code string

const (
	CodeValidation      Code = "VALIDATION_FAILED"
	CodeUnauthorized    Code = "UNAUTHORIZED"
	CodeForbidden       Code = "FORBIDDEN"
	CodeNotFound        Code = "NOT_FOUND"
	CodeConflict        Code = "CONFLICT"
	CodeRateLimited     Code = "RATE_LIMITED"
	CodeUpgradeRequired Code = "UPGRADE_REQUIRED"
	CodeInternal        Code = "INTERNAL"
)
