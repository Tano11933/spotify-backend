package validator

import (
	"errors"

	"github.com/go-playground/validator/v10"
)

var validate = validator.New()

func ValidateStruct(s interface{}) error {
	return validate.Struct(s)
}

// FieldError adalah bentuk ringkas error validasi per field. Dipakai handler
// untuk mengisi `details` pada response 400 — frontend bisa menyorot field
// yang salah tanpa mengurai pesan error.
type FieldError struct {
	Field string `json:"field"`
	Rule  string `json:"rule"`
}

// Details menerjemahkan error dari ValidateStruct menjadi daftar FieldError.
// Mengembalikan nil kalau error bukan berasal dari validator.
func Details(err error) []FieldError {
	var validationErrs validator.ValidationErrors
	if !errors.As(err, &validationErrs) {
		return nil
	}

	details := make([]FieldError, 0, len(validationErrs))
	for _, fieldErr := range validationErrs {
		details = append(details, FieldError{
			Field: fieldErr.Field(),
			Rule:  fieldErr.Tag(),
		})
	}
	return details
}
