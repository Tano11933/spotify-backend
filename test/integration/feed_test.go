//go:build integration

package integration

import (
	"fmt"
	"testing"
)

// TestFeedShowsFollowedUsersActivity mengunci feed: aktivitas user yang
// diikuti muncul (lagu diputar & playlist publik dibuat), playlist privat
// tidak bocor, data actor aman (tanpa email), dan endpointnya butuh login.
func TestFeedShowsFollowedUsersActivity(t *testing.T) {
	user := login(t, "user@test.local", "Password123!")
	admin := login(t, "admin@test.local", "Password123!")

	// Feed admin kosong: ia tidak mengikuti siapa pun, jadi aktivitas user
	// tidak boleh muncul di sana.
	_, adminFeed := do(t, "GET", "/api/me/feed?limit=50", admin, nil)
	if adminFeed["total"].(float64) != 0 {
		t.Fatalf("feed user tanpa following = %v, mau 0", adminFeed["total"])
	}

	// Bersihkan lalu buat user mengikuti admin.
	followPath := fmt.Sprintf("/api/users/%s/follow", fixtureAdminID)
	do(t, "DELETE", followPath, user, nil)
	t.Cleanup(func() {
		do(t, "DELETE", followPath, user, nil)
	})

	if status, _ := do(t, "PUT", followPath, user, nil); status != 200 {
		t.Fatalf("follow = %d, mau 200", status)
	}

	// Admin memutar lagu fixture.
	if status, body := do(t, "POST", "/api/me/player/play", admin, map[string]any{"song_id": fixtureSongID}); status != 200 {
		t.Fatalf("play = %d %v", status, body)
	}

	// Admin membuat playlist publik dan privat.
	_, public := do(t, "POST", "/api/playlists", admin, map[string]any{
		"name": "Feed Public", "description": "", "is_public": true,
	})
	publicID := uint(public["id"].(float64))

	_, private := do(t, "POST", "/api/playlists", admin, map[string]any{
		"name": "Feed Private", "description": "", "is_public": false,
	})
	privateID := uint(private["id"].(float64))

	t.Cleanup(func() {
		do(t, "DELETE", fmt.Sprintf("/api/playlists/%d", publicID), admin, nil)
		do(t, "DELETE", fmt.Sprintf("/api/playlists/%d", privateID), admin, nil)
	})

	status, feed := do(t, "GET", "/api/me/feed?limit=50", user, nil)
	if status != 200 {
		t.Fatalf("feed = %d %v", status, feed)
	}

	entries := items(t, feed)
	foundSong := false
	foundPlaylist := false
	foundPrivate := false

	for _, entry := range entries {
		actor, _ := entry["user"].(map[string]any)
		if actor == nil {
			t.Fatalf("entri feed tanpa actor: %v", entry)
		}
		if actor["name"] != "Admin Test" {
			t.Fatalf("aktor feed salah: %v", actor)
		}
		if _, leaked := actor["email"]; leaked {
			t.Fatal("aktor feed membocorkan email")
		}

		switch entry["type"] {
		case "song_played":
			song, _ := entry["song"].(map[string]any)
			if song != nil && uint(song["id"].(float64)) == fixtureSongID {
				foundSong = true
			}

		case "playlist_created":
			playlist, _ := entry["playlist"].(map[string]any)
			if playlist == nil {
				continue
			}
			if uint(playlist["id"].(float64)) == publicID {
				foundPlaylist = true
			}
			if uint(playlist["id"].(float64)) == privateID {
				foundPrivate = true
			}
		}
	}

	if !foundSong {
		t.Fatalf("aktivitas memutar lagu tidak muncul di feed: %v", entries)
	}
	if !foundPlaylist {
		t.Fatalf("playlist publik baru tidak muncul di feed: %v", entries)
	}
	if foundPrivate {
		t.Fatal("playlist privat bocor ke feed")
	}

	// Tanpa login → 401.
	status, body := do(t, "GET", "/api/me/feed", "", nil)
	if status != 401 || body["code"] != "UNAUTHORIZED" {
		t.Fatalf("feed tanpa token = %d %v, mau 401", status, body)
	}
}
