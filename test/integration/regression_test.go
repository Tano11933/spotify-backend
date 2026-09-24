//go:build integration

package integration

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// TestAuthFlow mengunci perilaku inti auth: register, login, me, rotasi
// refresh token, dan pencabutan saat logout.
func TestAuthFlow(t *testing.T) {
	email := fmt.Sprintf("flow-%d@test.local", time.Now().UnixNano())

	status, _ := do(t, "POST", "/api/auth/register", "", map[string]string{
		"name": "Flow User", "email": email, "password": "Password123!",
	})
	if status != 201 {
		t.Fatalf("register = %d, mau 201", status)
	}

	status, payload := do(t, "POST", "/api/auth/login", "", map[string]string{
		"email": email, "password": "Password123!",
	})
	if status != 200 {
		t.Fatalf("login = %d, mau 200", status)
	}

	tokens, _ := payload["tokens"].(map[string]any)
	access, _ := tokens["access_token"].(string)
	refresh, _ := tokens["refresh_token"].(string)
	if access == "" || refresh == "" {
		t.Fatalf("token kosong: %v", tokens)
	}

	status, me := do(t, "GET", "/api/auth/me", access, nil)
	if status != 200 || me["email"] != email {
		t.Fatalf("me = %d %v", status, me)
	}

	// Rotasi: refresh token lama harus mati setelah dipakai.
	status, rotated := do(t, "POST", "/api/auth/refresh", "", map[string]string{"refresh_token": refresh})
	if status != 200 {
		t.Fatalf("refresh = %d, mau 200", status)
	}
	newAccess, _ := rotated["access_token"].(string)
	newRefresh, _ := rotated["refresh_token"].(string)

	status, _ = do(t, "POST", "/api/auth/refresh", "", map[string]string{"refresh_token": refresh})
	if status != 401 {
		t.Fatalf("refresh token lama masih berlaku: %d", status)
	}

	status, _ = do(t, "POST", "/api/auth/logout", newAccess, nil)
	if status != 200 {
		t.Fatalf("logout = %d, mau 200", status)
	}

	status, _ = do(t, "POST", "/api/auth/refresh", "", map[string]string{"refresh_token": newRefresh})
	if status != 401 {
		t.Fatalf("refresh token setelah logout masih berlaku: %d", status)
	}
}

// TestCacheInvalidationArtistToAlbum mengunci perbaikan bug: mengubah artist
// harus ikut menyegarkan cache album yang meng-embed data artist.
func TestCacheInvalidationArtistToAlbum(t *testing.T) {
	admin := login(t, "admin@test.local", "Password123!")

	t.Cleanup(func() {
		do(t, "PUT", fmt.Sprintf("/api/artists/%d", fixtureArtistID), admin,
			map[string]string{"name": "Alpha"})
	})

	// Prime cache list & detail.
	_, list := do(t, "GET", "/api/albums?limit=100", "", nil)
	if got := albumArtistName(t, list, fixtureAlbumID); got != "Alpha" {
		t.Fatalf("nama artist awal = %q, mau Alpha", got)
	}

	status, _ := do(t, "PUT", fmt.Sprintf("/api/artists/%d", fixtureArtistID), admin,
		map[string]string{"name": "Alpha Renamed"})
	if status != 200 {
		t.Fatalf("rename artist = %d, mau 200", status)
	}

	_, list = do(t, "GET", "/api/albums?limit=100", "", nil)
	if got := albumArtistName(t, list, fixtureAlbumID); got != "Alpha Renamed" {
		t.Fatalf("cache list album basi: %q, mau Alpha Renamed", got)
	}

	_, detail := do(t, "GET", fmt.Sprintf("/api/albums/%d", fixtureAlbumID), "", nil)
	artist, _ := detail["artist"].(map[string]any)
	if artist["name"] != "Alpha Renamed" {
		t.Fatalf("cache detail album basi: %v", artist["name"])
	}
}

// TestDeleteGuards mengunci perbaikan bug: delete entitas yang masih
// direferensikan harus 409 dengan pesan jelas, bukan 500.
func TestDeleteGuards(t *testing.T) {
	admin := login(t, "admin@test.local", "Password123!")

	// Artist tanpa relasi tetap bisa dihapus.
	_, created := do(t, "POST", "/api/artists", admin, map[string]string{"name": "Temp Solo"})
	soloID := uint(created["id"].(float64))
	if status, _ := do(t, "DELETE", fmt.Sprintf("/api/artists/%d", soloID), admin, nil); status != 200 {
		t.Fatalf("delete artist tanpa relasi = %d, mau 200", status)
	}

	// Artist yang punya album -> 409.
	_, created = do(t, "POST", "/api/artists", admin, map[string]string{"name": "Temp Parent"})
	parentID := uint(created["id"].(float64))

	_, album := do(t, "POST", "/api/albums", admin, map[string]any{
		"title": "Temp Album", "artist_id": parentID, "release_date": "2024-01-01T00:00:00Z",
	})
	albumID := uint(album["id"].(float64))

	status, payload := do(t, "DELETE", fmt.Sprintf("/api/artists/%d", parentID), admin, nil)
	if status != 409 || payload["code"] != "CONFLICT" {
		t.Fatalf("delete artist berelasi = %d %v, mau 409 CONFLICT", status, payload)
	}

	// Album yang punya lagu -> 409.
	_, album2 := do(t, "POST", "/api/albums", admin, map[string]any{
		"title": "Temp Album 2", "artist_id": fixtureArtistID, "release_date": "2024-01-01T00:00:00Z",
	})
	album2ID := uint(album2["id"].(float64))

	_, song := do(t, "POST", "/api/songs", admin, map[string]any{
		"title": "Temp Song", "duration": 100, "artist_id": fixtureArtistID, "album_id": album2ID,
	})
	songID := uint(song["id"].(float64))

	status, payload = do(t, "DELETE", fmt.Sprintf("/api/albums/%d", album2ID), admin, nil)
	if status != 409 || payload["code"] != "CONFLICT" {
		t.Fatalf("delete album berisi lagu = %d %v, mau 409 CONFLICT", status, payload)
	}

	// Cleanup: setelah lagu & album dihapus, artist bisa dihapus.
	do(t, "DELETE", fmt.Sprintf("/api/songs/%d", songID), admin, nil)
	if status, _ := do(t, "DELETE", fmt.Sprintf("/api/albums/%d", album2ID), admin, nil); status != 200 {
		t.Fatalf("delete album kosong = %d, mau 200", status)
	}
	do(t, "DELETE", fmt.Sprintf("/api/albums/%d", albumID), admin, nil)
	if status, _ := do(t, "DELETE", fmt.Sprintf("/api/artists/%d", parentID), admin, nil); status != 200 {
		t.Fatalf("delete artist setelah relasi dibersihkan = %d, mau 200", status)
	}
}

// TestPayloadShapes mengunci perbaikan preload: artist di lagu album, album di
// lagu, dan user di playlist publik.
func TestPayloadShapes(t *testing.T) {
	_, detail := do(t, "GET", fmt.Sprintf("/api/albums/%d", fixtureAlbumID), "", nil)
	songs, _ := detail["songs"].([]any)
	if len(songs) == 0 {
		t.Fatal("album fixture tidak punya lagu")
	}
	first, _ := songs[0].(map[string]any)
	artist, _ := first["artist"].(map[string]any)
	if artist == nil || artist["name"] != "Alpha" {
		t.Fatalf("artist tidak ter-preload di lagu album: %v", first["artist"])
	}

	_, songsPage := do(t, "GET", "/api/songs?limit=100", "", nil)
	foundAlbum := false
	for _, song := range items(t, songsPage) {
		if song["title"] != "Alpha Song" {
			continue
		}
		album, _ := song["album"].(map[string]any)
		if album == nil || album["title"] != "First Album" {
			t.Fatalf("album tidak ter-preload di lagu: %v", song["album"])
		}
		foundAlbum = true
	}
	if !foundAlbum {
		t.Fatal("lagu fixture tidak ditemukan di /api/songs")
	}

	userToken := login(t, "user@test.local", "Password123!")
	_, public := do(t, "GET", "/api/playlists/public?limit=100", userToken, nil)
	foundUser := false
	for _, playlist := range items(t, public) {
		if playlist["name"] != "Public Mix" {
			continue
		}
		owner, _ := playlist["user"].(map[string]any)
		if owner == nil || owner["name"] != "User Test" {
			t.Fatalf("user tidak ter-preload di playlist publik: %v", playlist["user"])
		}
		foundUser = true
	}
	if !foundUser {
		t.Fatal("playlist fixture tidak ditemukan di /api/playlists/public")
	}
}

// TestMassAssignmentProtection mengunci perbaikan: client tidak boleh
// menentukan id maupun created_at pada endpoint katalog.
func TestMassAssignmentProtection(t *testing.T) {
	admin := login(t, "admin@test.local", "Password123!")

	_, created := do(t, "POST", "/api/artists", admin, map[string]any{
		"id": 99999, "name": "Mass Test", "created_at": "2001-01-01T00:00:00Z",
	})

	id := uint(created["id"].(float64))
	if id == 99999 {
		t.Fatal("client berhasil menentukan id")
	}
	if createdAt, _ := created["created_at"].(string); strings.HasPrefix(createdAt, "2001") {
		t.Fatalf("client berhasil menentukan created_at: %s", createdAt)
	}

	t.Cleanup(func() {
		do(t, "DELETE", fmt.Sprintf("/api/artists/%d", id), admin, nil)
	})

	_, updated := do(t, "PUT", fmt.Sprintf("/api/artists/%d", id), admin, map[string]any{
		"name": "Mass Test 2", "created_at": "2001-01-01T00:00:00Z",
	})
	if createdAt, _ := updated["created_at"].(string); strings.HasPrefix(createdAt, "2001") {
		t.Fatalf("created_at tertimpa lewat PUT: %s", createdAt)
	}
	if uint(updated["id"].(float64)) != id {
		t.Fatalf("id berubah setelah PUT: %v", updated["id"])
	}
}

// TestPaginationAndErrorCodes mengunci envelope pagination dan kode error.
func TestPaginationAndErrorCodes(t *testing.T) {
	_, page := do(t, "GET", "/api/artists?limit=1", "", nil)
	if got := len(items(t, page)); got != 1 {
		t.Fatalf("limit=1 mengembalikan %d item", got)
	}
	if page["limit"].(float64) != 1 {
		t.Fatalf("limit di envelope = %v", page["limit"])
	}
	if page["total"].(float64) < 2 {
		t.Fatalf("total = %v, mau >= 2", page["total"])
	}

	status, payload := do(t, "GET", "/api/artists?limit=abc", "", nil)
	if status != 400 || payload["code"] != "VALIDATION_FAILED" {
		t.Fatalf("limit non-numerik = %d %v", status, payload)
	}

	status, payload = do(t, "GET", "/api/artists/999999", "", nil)
	if status != 404 || payload["code"] != "NOT_FOUND" {
		t.Fatalf("artist tidak ada = %d %v", status, payload)
	}

	status, payload = do(t, "GET", "/api/playlists", "", nil)
	if status != 401 || payload["code"] != "UNAUTHORIZED" {
		t.Fatalf("tanpa token = %d %v", status, payload)
	}
}

// TestSearch mengunci endpoint pencarian: exact, typo, filter tipe, dan
// validasi query pendek.
func TestSearch(t *testing.T) {
	status, payload := do(t, "GET", "/api/search?q=alpha", "", nil)
	if status != 200 {
		t.Fatalf("search = %d, mau 200", status)
	}

	artists, _ := payload["artists"].(map[string]any)
	if artists == nil || len(items(t, artists)) == 0 {
		t.Fatalf("search 'alpha' tidak menemukan artist: %v", payload)
	}
	if got := items(t, artists)[0]["name"]; got != "Alpha" {
		t.Fatalf("hasil pertama = %v, mau Alpha", got)
	}

	// Typo: "alpa" harus tetap menemukan "Alpha" lewat word_similarity.
	_, typo := do(t, "GET", "/api/search?q=alpa", "", nil)
	typoArtists, _ := typo["artists"].(map[string]any)
	found := false
	if typoArtists != nil {
		for _, artist := range items(t, typoArtists) {
			if artist["name"] == "Alpha" {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("search typo 'alpa' tidak menemukan Alpha: %v", typo)
	}

	// Filter tipe: hanya grup artists yang dikembalikan.
	_, filtered := do(t, "GET", "/api/search?q=alpha&type=artist", "", nil)
	if filtered["tracks"] != nil {
		t.Fatalf("type=artist masih mengembalikan tracks: %v", filtered["tracks"])
	}
	if filtered["artists"] == nil {
		t.Fatal("type=artist tidak mengembalikan grup artists")
	}

	status, payload = do(t, "GET", "/api/search?q=a", "", nil)
	if status != 400 || payload["code"] != "VALIDATION_FAILED" {
		t.Fatalf("query pendek = %d %v", status, payload)
	}
}

// TestMigrationsApplied memastikan runner migrasi benar-benar dijalankan dan
// extension pg_trgm terpasang.
func TestMigrationsApplied(t *testing.T) {
	var count int64
	if err := db.Table("schema_migrations").Count(&count).Error; err != nil {
		t.Fatalf("baca schema_migrations: %v", err)
	}
	if count == 0 {
		t.Fatal("tidak ada migrasi yang tercatat")
	}

	var extensions int64
	if err := db.Raw("SELECT count(*) FROM pg_extension WHERE extname = 'pg_trgm'").Scan(&extensions).Error; err != nil {
		t.Fatalf("cek pg_trgm: %v", err)
	}
	if extensions != 1 {
		t.Fatal("extension pg_trgm tidak terpasang")
	}
}

// albumArtistName mencari nama artist untuk album tertentu di response list.
func albumArtistName(t *testing.T, payload map[string]any, albumID uint) string {
	t.Helper()

	for _, album := range items(t, payload) {
		if uint(album["id"].(float64)) != albumID {
			continue
		}
		artist, _ := album["artist"].(map[string]any)
		if artist == nil {
			t.Fatalf("album %d tidak menyertakan artist", albumID)
		}
		return artist["name"].(string)
	}

	t.Fatalf("album %d tidak ditemukan di list", albumID)
	return ""
}
