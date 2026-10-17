//go:build integration

package integration

import (
	"fmt"
	"testing"
	"time"
)

// createAlbum membuat album lewat API admin dan mendaftarkan pembersihannya.
func createAlbum(t *testing.T, adminToken, title, releaseDate string) uint {
	t.Helper()

	status, body := do(t, "POST", "/api/albums", adminToken, map[string]any{
		"title":        title,
		"artist_id":    fixtureArtistID,
		"release_date": releaseDate,
	})
	if status != 201 {
		t.Fatalf("create album %q = %d %v", title, status, body)
	}

	id := uint(body["id"].(float64))
	t.Cleanup(func() {
		do(t, "DELETE", fmt.Sprintf("/api/albums/%d", id), adminToken, nil)
	})
	return id
}

// TestNewReleasesOrderingAndPagination mengunci endpoint rilisan terbaru:
// urutannya menurun berdasarkan release_date, halaman pertama berisi yang
// paling baru, envelope pagination standar, artist ikut ter-preload, dan route
// /new-releases tidak tertangkap /:id (kalau tertangkap, statusnya 400).
func TestNewReleasesOrderingAndPagination(t *testing.T) {
	admin := login(t, "admin@test.local", "Password123!")

	// Dua album di ujung skala: 2099 memastikan berada di puncak daftar,
	// 1990 memastikan tetap ikut terdaftar di halaman penuh.
	newestID := createAlbum(t, admin, "Rilisan 2099", "2099-12-31T00:00:00Z")
	oldestID := createAlbum(t, admin, "Rilisan 1990", "1990-01-01T00:00:00Z")

	status, page := do(t, "GET", "/api/albums/new-releases?limit=100", "", nil)
	if status != 200 {
		t.Fatalf("new releases = %d %v", status, page)
	}

	list := items(t, page)
	if len(list) == 0 {
		t.Fatal("rilisan terbaru kosong")
	}

	// Urutan menurun berdasarkan release_date, sekaligus catat posisi id.
	var previous time.Time
	index := make(map[uint]int, len(list))

	for i, album := range list {
		released, err := time.Parse(time.RFC3339, album["release_date"].(string))
		if err != nil {
			t.Fatalf("release_date tidak valid di index %d: %v", i, album["release_date"])
		}
		if !previous.IsZero() && released.After(previous) {
			t.Fatalf("urutan tidak menurun di index %d: %s setelah %s", i, released, previous)
		}
		previous = released
		index[uint(album["id"].(float64))] = i
	}

	if _, ok := index[newestID]; !ok {
		t.Fatal("album 2099 tidak muncul di rilisan terbaru")
	}
	if _, ok := index[oldestID]; !ok {
		t.Fatal("album 1990 tidak muncul di rilisan terbaru")
	}
	if index[newestID] > index[oldestID] {
		t.Fatalf(
			"album 2099 (index %d) harus sebelum album 1990 (index %d)",
			index[newestID], index[oldestID],
		)
	}

	// Pagination offset: halaman pertama limit 1 harus album terbaru.
	status, first := do(t, "GET", "/api/albums/new-releases?limit=1&offset=0", "", nil)
	if status != 200 {
		t.Fatalf("halaman pertama = %d %v", status, first)
	}

	firstItems := items(t, first)
	if len(firstItems) != 1 || uint(firstItems[0]["id"].(float64)) != newestID {
		t.Fatalf("halaman pertama = %v, mau album %d", firstItems, newestID)
	}
	if first["total"].(float64) < 2 {
		t.Fatalf("total = %v, mau minimal 2", first["total"])
	}
	if _, ok := firstItems[0]["artist"].(map[string]any); !ok {
		t.Fatalf("rilisan terbaru tidak menyertakan artist: %v", firstItems[0])
	}
}
