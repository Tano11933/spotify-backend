//go:build integration

package integration

import (
	"fmt"
	"testing"
)

// TestRelatedArtistsBySharedFollowers mengunci "Fans also like" jalur utama:
// artist yang pengikutnya beririsan muncul, dan artist asalnya sendiri tidak.
func TestRelatedArtistsBySharedFollowers(t *testing.T) {
	admin := login(t, "admin@test.local", "Password123!")
	user := login(t, "user@test.local", "Password123!")

	originID := createArtist(t, admin, "Related Origin")
	peerID := createArtist(t, admin, "Related Peer")

	// Dua user mengikuti kedua artist, jadi irisan pengikutnya jelas.
	for _, token := range []string{admin, user} {
		for _, id := range []uint{originID, peerID} {
			status, body := do(t, "PUT", fmt.Sprintf("/api/me/following/%d", id), token, nil)
			if status != 200 {
				t.Fatalf("follow artist %d = %d %v", id, status, body)
			}
		}
	}
	t.Cleanup(func() {
		for _, token := range []string{admin, user} {
			for _, id := range []uint{originID, peerID} {
				do(t, "DELETE", fmt.Sprintf("/api/me/following/%d", id), token, nil)
			}
		}
	})

	status, related := do(t, "GET", fmt.Sprintf("/api/artists/%d/related?limit=20", originID), "", nil)
	if status != 200 {
		t.Fatalf("related = %d %v", status, related)
	}

	list := items(t, related)
	if !hasArtist(list, peerID) {
		t.Fatalf("artist dengan pengikut beririsan tidak muncul: %v", list)
	}
	if hasArtist(list, originID) {
		t.Fatal("artist asal ikut muncul di daftar related")
	}
}

// TestRelatedArtistsGenreFallback mengunci jalur cadangan: saat belum ada
// irisan pengikut sama sekali, related diisi artist yang berbagi genre.
func TestRelatedArtistsGenreFallback(t *testing.T) {
	admin := login(t, "admin@test.local", "Password123!")

	genreID := createGenre(t, admin, "Fallback Genre")
	originID := createArtist(t, admin, "Fallback Origin")
	peerID := createArtist(t, admin, "Fallback Peer")

	for _, id := range []uint{originID, peerID} {
		status, body := do(t, "PUT", fmt.Sprintf("/api/artists/%d/genres", id), admin, map[string]any{
			"genre_ids": []uint{genreID},
		})
		if status != 200 {
			t.Fatalf("set genre artist %d = %d %v", id, status, body)
		}
	}

	// Artist baru belum diikuti siapa pun, jadi hasilnya murni fallback genre.
	status, related := do(t, "GET", fmt.Sprintf("/api/artists/%d/related?limit=20", originID), "", nil)
	if status != 200 {
		t.Fatalf("related fallback = %d %v", status, related)
	}

	list := items(t, related)
	if !hasArtist(list, peerID) {
		t.Fatalf("artist segenre tidak muncul di fallback: %v", list)
	}
	if hasArtist(list, originID) {
		t.Fatal("artist asal ikut muncul di fallback")
	}

	// Artist tidak ada → 404.
	if status, _ := do(t, "GET", "/api/artists/999999/related", "", nil); status != 404 {
		t.Fatalf("related artist hilang = %d, mau 404", status)
	}
}

// TestRecommendationsMadeForYou mengunci rekomendasi personal: lagu dari genre
// yang baru diputar user ikut direkomendasikan, dan lagu yang belum pernah
// diputar didahulukan. Endpointnya wajib login.
func TestRecommendationsMadeForYou(t *testing.T) {
	admin := login(t, "admin@test.local", "Password123!")
	user := login(t, "user@test.local", "Password123!")

	genreID := createGenre(t, admin, "Recovery Pop")
	artistID := createArtist(t, admin, "Recovery Artist")

	status, body := do(t, "PUT", fmt.Sprintf("/api/artists/%d/genres", artistID), admin, map[string]any{
		"genre_ids": []uint{genreID},
	})
	if status != 200 {
		t.Fatalf("set genre = %d %v", status, body)
	}

	anchorID := createSong(t, admin, artistID, "Played Anchor")
	freshID := createSong(t, admin, artistID, "Fresh Pick")

	// User memutar satu lagu dari genre ini; lagu satunya belum pernah.
	if status, body = do(t, "POST", "/api/me/player/play", user, map[string]any{"song_id": anchorID}); status != 200 {
		t.Fatalf("play anchor = %d %v", status, body)
	}

	status, recs := do(t, "GET", "/api/me/recommendations?limit=100", user, nil)
	if status != 200 {
		t.Fatalf("recommendations = %d %v", status, recs)
	}

	indexOf := func(id uint) int {
		for i, item := range items(t, recs) {
			if uint(item["id"].(float64)) == id {
				return i
			}
		}
		return -1
	}

	freshIdx := indexOf(freshID)
	anchorIdx := indexOf(anchorID)
	if freshIdx == -1 {
		t.Fatalf("lagu dari genre favorit tidak direkomendasikan: %v", recs)
	}
	if anchorIdx != -1 && anchorIdx < freshIdx {
		t.Fatalf("lagu yang sudah diputar muncul lebih dulu dari yang belum diputar")
	}

	// Tanpa login → 401.
	if status, body = do(t, "GET", "/api/me/recommendations", "", nil); status != 401 || body["code"] != "UNAUTHORIZED" {
		t.Fatalf("recommendations tanpa token = %d %v, mau 401 UNAUTHORIZED", status, body)
	}
}

// TestChartsTopTracksAndArtists mengunci chart: lagu yang baru diputar muncul
// di chart lagu (dengan jumlah putar) dan artist-nya muncul di chart artist.
// Keduanya publik.
func TestChartsTopTracksAndArtists(t *testing.T) {
	admin := login(t, "admin@test.local", "Password123!")

	if status, body := do(t, "POST", "/api/me/player/play", admin, map[string]any{"song_id": fixtureSongID}); status != 200 {
		t.Fatalf("play = %d %v", status, body)
	}

	status, tracks := do(t, "GET", "/api/charts/tracks?limit=100", "", nil)
	if status != 200 {
		t.Fatalf("chart tracks = %d %v", status, tracks)
	}

	foundSong := false
	for _, track := range items(t, tracks) {
		if uint(track["id"].(float64)) == fixtureSongID {
			foundSong = true
			if track["plays"].(float64) < 1 {
				t.Fatalf("plays lagu = %v, mau minimal 1", track["plays"])
			}
		}
	}
	if !foundSong {
		t.Fatalf("lagu yang diputar tidak muncul di chart: %v", tracks)
	}

	status, artists := do(t, "GET", "/api/charts/artists?limit=100", "", nil)
	if status != 200 {
		t.Fatalf("chart artists = %d %v", status, artists)
	}

	foundArtist := false
	for _, artist := range items(t, artists) {
		if uint(artist["id"].(float64)) == fixtureArtistID {
			foundArtist = true
			if artist["plays"].(float64) < 1 {
				t.Fatalf("plays artist = %v, mau minimal 1", artist["plays"])
			}
		}
	}
	if !foundArtist {
		t.Fatalf("artist dari lagu yang diputar tidak muncul di chart: %v", artists)
	}
}
