package pagination

import (
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestCursorRoundTrip(t *testing.T) {
	original := Cursor{
		Time: time.Date(2026, 10, 12, 10, 30, 0, 0, time.UTC),
		ID:   42,
	}

	parsed, err := ParseCursor(EncodeCursor(original))
	if err != nil {
		t.Fatalf("ParseCursor: %v", err)
	}
	if parsed == nil {
		t.Fatal("ParseCursor mengembalikan nil untuk cursor valid")
	}
	if !parsed.Time.Equal(original.Time) || parsed.ID != original.ID {
		t.Fatalf("round-trip = %+v, mau %+v", parsed, original)
	}
}

func TestParseCursorEmptyMeansStart(t *testing.T) {
	parsed, err := ParseCursor("")
	if err != nil {
		t.Fatalf("ParseCursor(\"\") error: %v", err)
	}
	if parsed != nil {
		t.Fatalf("ParseCursor(\"\") = %+v, mau nil", parsed)
	}
}

func TestParseCursorRejectsInvalidInput(t *testing.T) {
	cases := map[string]string{
		"bukan base64": "%%%",
		"json rusak":   "bm90LWpzb24",                                      // "not-json"
		"tanpa waktu":  "eyJpIjo0Mn0=",                                     // {"i":42}
		"tanpa id":     "eyJ0IjoiMjAyNi0xMC0xMlQxMDozMDowMFoifQ==",         // {"t":"..."}
		"waktu nol":    "eyJ0IjoiMDAwMS0wMS0wMVQwMDowMDowMFoiLCJpIjo0Mn0=", // {"t":"0001-01-01T00:00:00Z","i":42}
	}

	for name, raw := range cases {
		if _, err := ParseCursor(raw); !errors.Is(err, ErrInvalidCursor) {
			t.Errorf("%s: err = %v, mau ErrInvalidCursor", name, err)
		}
	}
}

func TestCursorPageOmitsNextCursorWhenEmpty(t *testing.T) {
	page := NewCursorPage([]string{"a"}, 1, Params{Limit: 20}, "")

	raw, err := json.Marshal(page)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, exists := payload["next_cursor"]; exists {
		t.Fatalf("next_cursor ikut terkirim meski kosong: %s", raw)
	}
	if payload["total"].(float64) != 1 {
		t.Fatalf("total = %v, mau 1", payload["total"])
	}
}
