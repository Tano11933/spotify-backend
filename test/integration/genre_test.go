//go:build integration

package integration

import (
	"fmt"
	"testing"
)

// createGenre membuat genre lewat API sebagai admin dan mendaftarkan
// pembersihannya.
func createGenre(t *testing.T, admin, name string) uint {
	t.Helper()

	status, body := do(t, "POST", "/api/genres", admin, map[string]any{"name": name})
	if status != 201 {
		t.Fatalf("create genre %q = %d %v", name, status, body)
	}

	id := uint(body["id"].(float64))
	t.Cleanup(func() {
		do(t, "DELETE", fmt.Sprintf("/api/genres/%d", id), admin, nil)
	})
	return id
}

// createArtist membuat artist lewat API sebagai admin dan mendaftarkan
// pembersihannya. Artist baru tidak punya album/lagu, jadi aman dihapus.
func createArtist(t *testing.T, admin, name string) uint {
	t.Helper()

	status, body := do(t, "POST", "/api/artists", admin, map[string]any{"name": name})
	if status != 201 {
		t.Fatalf("create artist %q = %d %v", name, status, body)
	}

	id := uint(body["id"].(float64))
	t.Cleanup(func() {
		do(t, "DELETE", fmt.Sprintf("/api/artists/%d", id), admin, nil)
	})
	return id
}

// createSong membuat lagu lewat API sebagai admin dan mendaftarkan
// pembersihannya.
func createSong(t *testing.T, admin string, artistID uint, title string) uint {
	t.Helper()

	status, body := do(t, "POST", "/api/songs", admin, map[string]any{
		"title":     title,
		"duration":  180,
		"artist_id": artistID,
	})
	if status != 201 {
		t.Fatalf("create song %q = %d %v", title, status, body)
	}

	id := uint(body["id"].(float64))
	t.Cleanup(func() {
		do(t, "DELETE", fmt.Sprintf("/api/songs/%d", id), admin, nil)
	})
	return id
}

// genreNames mengambil nama-nama genre dari response detail artist.
func genreNames(artist map[string]any) []string {
	raw, ok := artist["genres"].([]any)
	if !ok {
		return nil
	}

	names := make([]string, 0, len(raw))
	for _, item := range raw {
		genre, ok := item.(map[string]any)
		if !ok {
			continue
		}
		names = append(names, genre["name"].(string))
	}
	return names
}

// hasArtist memeriksa apakah daftar artist (hasil items()) memuat id tertentu.
func hasArtist(list []map[string]any, id uint) bool {
	for _, item := range list {
		if uint(item["id"].(float64)) == id {
			return true
		}
	}
	return false
}

// TestGenreCatalogAndArtistAssignment mengunci alur genre end-to-end: kurasi
// admin-only, list/detail publik, penempelan ke artist yang bersifat
// mengganti (bukan menambah), dan pembersihan cache artist saat genre
// dihapus.
func TestGenreCatalogAndArtistAssignment(t *testing.T) {
	admin := login(t, "admin@test.local", "Password123!")
	user := login(t, "user@test.local", "Password123!")

	// User biasa tidak boleh mengubah katalog.
	status, body := do(t, "POST", "/api/genres", user, map[string]any{"name": "Nope Genre"})
	if status != 403 || body["code"] != "FORBIDDEN" {
		t.Fatalf("create genre oleh user = %d %v, mau 403 FORBIDDEN", status, body)
	}

	firstID := createGenre(t, admin, "Integration Pop")
	secondID := createGenre(t, admin, "Integration Rock")

	// Slug dihitung server dari nama.
	status, detail := do(t, "GET", fmt.Sprintf("/api/genres/%d", firstID), "", nil)
	if status != 200 || detail["name"] != "Integration Pop" || detail["slug"] != "integration-pop" {
		t.Fatalf("detail genre = %d %v", status, detail)
	}

	// Nama duplikat ditolak 409.
	status, body = do(t, "POST", "/api/genres", admin, map[string]any{"name": "Integration Pop"})
	if status != 409 {
		t.Fatalf("genre duplikat = %d %v, mau 409", status, body)
	}

	// List publik memuat genre baru.
	status, list := do(t, "GET", "/api/genres?limit=100", "", nil)
	if status != 200 {
		t.Fatalf("list genre = %d %v", status, list)
	}

	found := false
	for _, genre := range items(t, list) {
		if uint(genre["id"].(float64)) == firstID {
			found = true
		}
	}
	if !found {
		t.Fatalf("genre %d tidak muncul di list", firstID)
	}

	// Genre yang tidak ada → 404, termasuk daftar artist-nya.
	if status, _ = do(t, "GET", "/api/genres/999999", "", nil); status != 404 {
		t.Fatalf("detail genre hilang = %d, mau 404", status)
	}
	if status, _ = do(t, "GET", "/api/genres/999999/artists", "", nil); status != 404 {
		t.Fatalf("artists genre hilang = %d, mau 404", status)
	}

	// Tempelkan dua genre ke artist fixture.
	status, artist := do(t, "PUT", fmt.Sprintf("/api/artists/%d/genres", fixtureArtistID), admin, map[string]any{
		"genre_ids": []uint{firstID, secondID},
	})
	if status != 200 || len(genreNames(artist)) != 2 {
		t.Fatalf("set genres = %d %v, mau 2 genre", status, artist)
	}

	// Detail artist (yang di-cache) harus ikut berubah.
	status, artistDetail := do(t, "GET", fmt.Sprintf("/api/artists/%d", fixtureArtistID), "", nil)
	if status != 200 || len(genreNames(artistDetail)) != 2 {
		t.Fatalf("detail artist setelah set = %d %v", status, artistDetail)
	}

	// Daftar artist per genre.
	status, artists := do(t, "GET", fmt.Sprintf("/api/genres/%d/artists", firstID), "", nil)
	if status != 200 || !hasArtist(items(t, artists), fixtureArtistID) {
		t.Fatalf("artists genre = %d %v, fixture tidak ada", status, artists)
	}

	// Referensi genre yang tidak ada → 422; user biasa → 403.
	status, body = do(t, "PUT", fmt.Sprintf("/api/artists/%d/genres", fixtureArtistID), admin, map[string]any{
		"genre_ids": []uint{999999},
	})
	if status != 422 {
		t.Fatalf("set genre tak dikenal = %d %v, mau 422", status, body)
	}
	status, _ = do(t, "PUT", fmt.Sprintf("/api/artists/%d/genres", fixtureArtistID), user, map[string]any{
		"genre_ids": []uint{firstID},
	})
	if status != 403 {
		t.Fatalf("set genre oleh user = %d, mau 403", status)
	}

	// Sifatnya mengganti: kirim satu genre, yang lama lepas.
	status, _ = do(t, "PUT", fmt.Sprintf("/api/artists/%d/genres", fixtureArtistID), admin, map[string]any{
		"genre_ids": []uint{secondID},
	})
	if status != 200 {
		t.Fatalf("replace genres = %d, mau 200", status)
	}
	_, artistDetail = do(t, "GET", fmt.Sprintf("/api/artists/%d", fixtureArtistID), "", nil)
	names := genreNames(artistDetail)
	if len(names) != 1 || names[0] != "Integration Rock" {
		t.Fatalf("genres setelah replace = %v, mau [Integration Rock]", names)
	}

	// Daftar kosong melepas semua genre.
	status, _ = do(t, "PUT", fmt.Sprintf("/api/artists/%d/genres", fixtureArtistID), admin, map[string]any{
		"genre_ids": []uint{},
	})
	if status != 200 {
		t.Fatalf("lepas genres = %d, mau 200", status)
	}
	_, artistDetail = do(t, "GET", fmt.Sprintf("/api/artists/%d", fixtureArtistID), "", nil)
	if artistDetail["genres"] != nil {
		t.Fatalf("genres setelah dilepas = %v, mau kosong", artistDetail["genres"])
	}

	// Pasang lagi, lalu hapus genrenya: relasi cascade dan cache artist bersih.
	do(t, "PUT", fmt.Sprintf("/api/artists/%d/genres", fixtureArtistID), admin, map[string]any{
		"genre_ids": []uint{firstID},
	})
	if status, _ = do(t, "DELETE", fmt.Sprintf("/api/genres/%d", firstID), admin, nil); status != 200 {
		t.Fatalf("delete genre = %d, mau 200", status)
	}
	_, artistDetail = do(t, "GET", fmt.Sprintf("/api/artists/%d", fixtureArtistID), "", nil)
	if artistDetail["genres"] != nil {
		t.Fatalf("genres setelah genre dihapus = %v, mau kosong", artistDetail["genres"])
	}

	// Menghapus dua kali → 404.
	if status, _ = do(t, "DELETE", fmt.Sprintf("/api/genres/%d", firstID), admin, nil); status != 404 {
		t.Fatalf("delete genre dua kali = %d, mau 404", status)
	}
}
