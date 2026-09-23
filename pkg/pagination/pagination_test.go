package pagination

import (
	"errors"
	"testing"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name      string
		rawLimit  string
		rawOffset string
		want      Params
		wantErr   error
	}{
		{name: "kosong memakai default", rawLimit: "", rawOffset: "", want: Params{Limit: 20, Offset: 0}},
		{name: "nilai valid", rawLimit: "5", rawOffset: "10", want: Params{Limit: 5, Offset: 10}},
		{name: "limit 0 di-clamp ke 1", rawLimit: "0", want: Params{Limit: 1, Offset: 0}},
		{name: "limit negatif di-clamp ke 1", rawLimit: "-5", want: Params{Limit: 1, Offset: 0}},
		{name: "limit di atas maksimum di-clamp", rawLimit: "5000", want: Params{Limit: MaxLimit, Offset: 0}},
		{name: "offset negatif di-clamp ke 0", rawOffset: "-3", want: Params{Limit: 20, Offset: 0}},
		{name: "limit bukan angka", rawLimit: "abc", wantErr: ErrInvalidLimit},
		{name: "offset bukan angka", rawOffset: "x", wantErr: ErrInvalidOffset},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse(tt.rawLimit, tt.rawOffset)

			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("Parse() error = %v, mau %v", err, tt.wantErr)
				}
				return
			}

			if err != nil {
				t.Fatalf("Parse() error tak terduga: %v", err)
			}
			if got != tt.want {
				t.Fatalf("Parse() = %+v, mau %+v", got, tt.want)
			}
		})
	}
}

func TestNewPageNilItems(t *testing.T) {
	page := NewPage[[]int](nil, 0, Params{Limit: 20})

	if page.Items == nil {
		t.Fatal("Items nil; harus slice kosong supaya JSON-nya [] bukan null")
	}
}

func TestSlice(t *testing.T) {
	items := []int{1, 2, 3, 4, 5}

	tests := []struct {
		name   string
		params Params
		want   []int
	}{
		{name: "halaman pertama", params: Params{Limit: 2, Offset: 0}, want: []int{1, 2}},
		{name: "halaman kedua", params: Params{Limit: 2, Offset: 2}, want: []int{3, 4}},
		{name: "halaman terakhir tidak penuh", params: Params{Limit: 2, Offset: 4}, want: []int{5}},
		{name: "offset melewati panjang", params: Params{Limit: 2, Offset: 99}, want: []int{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Slice(items, tt.params)
			if len(got) != len(tt.want) {
				t.Fatalf("Slice() = %v, mau %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("Slice() = %v, mau %v", got, tt.want)
				}
			}
		})
	}
}
