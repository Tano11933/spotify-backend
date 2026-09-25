//go:build integration

package integration

import (
	"fmt"
	"testing"
	"time"
)

// TestLibraryTracks mengunci alur liked songs: simpan idempoten, urutan
// terbaru-dulu, relasi ter-preload, contains, 404 untuk lagu tak dikenal, dan
// hapus yang juga idempoten.
func TestLibraryTracks(t *testing.T) {
	user := login(t, "user@test.local", "Password123!")
	admin := login(t, "admin@test.local", "Password123!")

	// Lagu sementara supaya test tidak mengganggu fixture.
	_, created := do(t, "POST", "/api/songs", admin, map[string]any{
		"title": "Library Temp Song", "duration": 120, "artist_id": fixtureArtistID,
	})
	tempSongID := uint(created["id"].(float64))
	t.Cleanup(func() {
		do(t, "DELETE", fmt.Sprintf("/api/songs/%d", tempSongID), admin, nil)
	})

	savePath := func(id uint) string { return fmt.Sprintf("/api/me/tracks/%d", id) }

	if status, body := do(t, "PUT", savePath(fixtureSongID), user, nil); status != 200 {
		t.Fatalf("save track = %d %v", status, body)
	}

	// Jeda kecil supaya saved_at kedua lagu pasti berbeda (urutan diuji di bawah).
	time.Sleep(15 * time.Millisecond)

	if status, body := do(t, "PUT", savePath(tempSongID), user, nil); status != 200 {
		t.Fatalf("save track kedua = %d %v", status, body)
	}

	// Idempoten: menyimpan ulang tetap 200, bukan 409.
	if status, body := do(t, "PUT", savePath(fixtureSongID), user, nil); status != 200 {
		t.Fatalf("save ulang = %d %v, mau 200", status, body)
	}

	_, page := do(t, "GET", "/api/me/tracks?limit=50", user, nil)
	list := items(t, page)
	if len(list) != 2 {
		t.Fatalf("daftar tersimpan = %d item, mau 2", len(list))
	}

	// Terbaru disimpan tampil lebih dulu.
	if got := uint(list[0]["id"].(float64)); got != tempSongID {
		t.Fatalf("item pertama = %d, mau %d (terbaru dulu)", got, tempSongID)
	}

	// Lagu fixture punya album & artist; keduanya harus ter-preload.
	for _, item := range list {
		if uint(item["id"].(float64)) != fixtureSongID {
			continue
		}
		if _, ok := item["artist"].(map[string]any); !ok {
			t.Fatalf("artist tidak ter-preload: %v", item["artist"])
		}
		if album, ok := item["album"].(map[string]any); !ok || album["title"] != "First Album" {
			t.Fatalf("album tidak ter-preload: %v", item["album"])
		}
	}

	// contains: id tersimpan true, id asing tetap muncul false.
	status, contains := do(t, "GET",
		fmt.Sprintf("/api/me/tracks/contains?ids=%d,999999", fixtureSongID), user, nil)
	if status != 200 {
		t.Fatalf("contains = %d, mau 200", status)
	}
	if contains[fmt.Sprintf("%d", fixtureSongID)] != true {
		t.Fatalf("id tersimpan tidak true: %v", contains)
	}
	if contains["999999"] != false {
		t.Fatalf("id tak tersimpan tidak false: %v", contains)
	}

	// Lagu tak dikenal -> 404.
	status, body := do(t, "PUT", "/api/me/tracks/999999", user, nil)
	if status != 404 || body["code"] != "NOT_FOUND" {
		t.Fatalf("save lagu tak dikenal = %d %v, mau 404 NOT_FOUND", status, body)
	}

	// Hapus idempoten.
	if status, body := do(t, "DELETE", savePath(fixtureSongID), user, nil); status != 200 {
		t.Fatalf("hapus = %d %v", status, body)
	}
	if status, body := do(t, "DELETE", savePath(fixtureSongID), user, nil); status != 200 {
		t.Fatalf("hapus ulang = %d %v, mau 200", status, body)
	}

	do(t, "DELETE", savePath(tempSongID), user, nil)
}

// TestLibraryAlbumsAndFollowing menguji album tersimpan dan artist yang
// diikuti, termasuk 404 untuk id yang tidak ada.
func TestLibraryAlbumsAndFollowing(t *testing.T) {
	user := login(t, "user@test.local", "Password123!")

	albumPath := fmt.Sprintf("/api/me/albums/%d", fixtureAlbumID)
	if status, body := do(t, "PUT", albumPath, user, nil); status != 200 {
		t.Fatalf("save album = %d %v", status, body)
	}

	_, page := do(t, "GET", "/api/me/albums?limit=50", user, nil)
	albums := items(t, page)
	if len(albums) != 1 || uint(albums[0]["id"].(float64)) != fixtureAlbumID {
		t.Fatalf("daftar album tersimpan tidak sesuai: %v", albums)
	}
	if _, ok := albums[0]["artist"].(map[string]any); !ok {
		t.Fatalf("artist tidak ter-preload di album tersimpan: %v", albums[0]["artist"])
	}

	status, contains := do(t, "GET",
		fmt.Sprintf("/api/me/albums/contains?ids=%d,999999", fixtureAlbumID), user, nil)
	if status != 200 ||
		contains[fmt.Sprintf("%d", fixtureAlbumID)] != true ||
		contains["999999"] != false {
		t.Fatalf("contains album tidak sesuai: %d %v", status, contains)
	}

	if status, body := do(t, "PUT", "/api/me/albums/999999", user, nil); status != 404 || body["code"] != "NOT_FOUND" {
		t.Fatalf("save album tak dikenal = %d %v", status, body)
	}

	followPath := fmt.Sprintf("/api/me/following/%d", fixtureArtistID)
	if status, body := do(t, "PUT", followPath, user, nil); status != 200 {
		t.Fatalf("follow artist = %d %v", status, body)
	}

	_, following := do(t, "GET", "/api/me/following?limit=50", user, nil)
	artists := items(t, following)
	if len(artists) != 1 || uint(artists[0]["id"].(float64)) != fixtureArtistID {
		t.Fatalf("daftar following tidak sesuai: %v", artists)
	}

	status, containsFollowing := do(t, "GET",
		fmt.Sprintf("/api/me/following/contains?ids=%d,999999", fixtureArtistID), user, nil)
	if status != 200 ||
		containsFollowing[fmt.Sprintf("%d", fixtureArtistID)] != true ||
		containsFollowing["999999"] != false {
		t.Fatalf("contains following tidak sesuai: %d %v", status, containsFollowing)
	}

	if status, body := do(t, "PUT", "/api/me/following/999999", user, nil); status != 404 || body["code"] != "NOT_FOUND" {
		t.Fatalf("follow artist tak dikenal = %d %v", status, body)
	}

	// Cleanup.
	do(t, "DELETE", albumPath, user, nil)
	do(t, "DELETE", followPath, user, nil)
}

// TestLibraryCascade mengunci perilaku ON DELETE CASCADE: lagu yang dihapus
// admin otomatis hilang dari library semua user.
func TestLibraryCascade(t *testing.T) {
	user := login(t, "user@test.local", "Password123!")
	admin := login(t, "admin@test.local", "Password123!")

	_, created := do(t, "POST", "/api/songs", admin, map[string]any{
		"title": "Cascade Temp Song", "duration": 90, "artist_id": fixtureArtistID,
	})
	tempSongID := uint(created["id"].(float64))
	path := fmt.Sprintf("/api/me/tracks/%d", tempSongID)

	if status, body := do(t, "PUT", path, user, nil); status != 200 {
		t.Fatalf("save track = %d %v", status, body)
	}

	// Admin menghapus lagunya -> FK cascade membersihkan library.
	if status, body := do(t, "DELETE", fmt.Sprintf("/api/songs/%d", tempSongID), admin, nil); status != 200 {
		t.Fatalf("delete song = %d %v", status, body)
	}

	status, contains := do(t, "GET", fmt.Sprintf("/api/me/tracks/contains?ids=%d", tempSongID), user, nil)
	if status != 200 {
		t.Fatalf("contains = %d, mau 200", status)
	}
	if contains[fmt.Sprintf("%d", tempSongID)] != false {
		t.Fatalf("entri library tidak ikut terhapus: %v", contains)
	}
}

// TestLibraryAuthAndValidation mengunci guard: wajib login, dan ids pada
// endpoint contains divalidasi.
func TestLibraryAuthAndValidation(t *testing.T) {
	status, body := do(t, "GET", "/api/me/tracks", "", nil)
	if status != 401 || body["code"] != "UNAUTHORIZED" {
		t.Fatalf("tanpa token = %d %v, mau 401 UNAUTHORIZED", status, body)
	}

	user := login(t, "user@test.local", "Password123!")

	status, body = do(t, "GET", "/api/me/tracks/contains", user, nil)
	if status != 400 || body["code"] != "VALIDATION_FAILED" {
		t.Fatalf("contains tanpa ids = %d %v, mau 400", status, body)
	}

	status, body = do(t, "GET", "/api/me/tracks/contains?ids=abc", user, nil)
	if status != 400 || body["code"] != "VALIDATION_FAILED" {
		t.Fatalf("contains ids bukan angka = %d %v, mau 400", status, body)
	}
}
