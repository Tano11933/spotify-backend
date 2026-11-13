//go:build integration

package integration

import (
	"fmt"
	"net/http"
	"testing"

	"spotify-backend/internal/model"
)

func TestPlaylistSongPages(t *testing.T) {
	owner := login(t, "user@test.local", "Password123!")
	viewer := login(t, "admin@test.local", "Password123!")

	// Roll back fixture data after the test using explicit cleanup so the
	// requests can see committed rows through the application's connection.
	playlist := model.Playlist{Name: "Paged songs fixture", UserID: fixtureUserID}
	if err := db.Create(&playlist).Error; err != nil {
		t.Fatal(err)
	}
	songs := make([]model.Song, 103)
	for i := range songs {
		songs[i] = model.Song{
			Title:    fmt.Sprintf("Playlist page fixture %03d", i),
			ArtistID: fixtureArtistID, AlbumID: &fixtureAlbumID, Duration: 30,
		}
	}
	t.Cleanup(func() {
		if err := db.Model(&playlist).Association("Songs").Clear(); err != nil {
			t.Error(err)
		}
		if err := db.Delete(&playlist).Error; err != nil {
			t.Error(err)
		}
		if songs[0].ID != 0 {
			if err := db.Delete(&songs).Error; err != nil {
				t.Error(err)
			}
		}
	})
	if err := db.Create(&songs).Error; err != nil {
		t.Fatal(err)
	}
	for i, j := 0, len(songs)-1; i < j; i, j = i+1, j-1 {
		songs[i], songs[j] = songs[j], songs[i]
	}
	if err := db.Model(&playlist).Association("Songs").Append(&songs); err != nil {
		t.Fatal(err)
	}

	path := fmt.Sprintf("/api/playlists/%d/songs", playlist.ID)
	for _, tc := range []struct {
		name, path, token string
		want              int
	}{
		{"guest", path, "", http.StatusUnauthorized},
		{"other user", path, viewer, http.StatusNotFound},
		{"unknown", "/api/playlists/99999999/songs", owner, http.StatusNotFound},
		{"invalid limit", path + "?limit=bad", owner, http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			status, body := do(t, "GET", tc.path, tc.token, nil)
			if status != tc.want {
				t.Fatalf("status = %d, want %d: %v", status, tc.want, body)
			}
		})
	}
	for _, tc := range []struct {
		offset, length int
	}{{0, 100}, {100, 3}, {103, 0}} {
		status, body := do(t, "GET", fmt.Sprintf("%s?limit=999&offset=%d", path, tc.offset), owner, nil)
		if status != http.StatusOK || body["total"] != float64(103) ||
			body["limit"] != float64(100) || body["offset"] != float64(tc.offset) {
			t.Fatalf("unexpected page at offset %d: status %d, %v", tc.offset, status, body)
		}
		rows := items(t, body)
		if len(rows) != tc.length {
			t.Fatalf("offset %d: got %d rows, want %d", tc.offset, len(rows), tc.length)
		}
		for i, row := range rows {
			wantID := songs[len(songs)-1-(tc.offset+i)].ID
			if row["id"] != float64(wantID) || row["artist"] == nil || row["album"] == nil {
				t.Fatalf("row %d: want hydrated song %d, got %v", tc.offset+i, wantID, row)
			}
		}
	}

	if err := db.Model(&playlist).Update("is_public", true).Error; err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/api/playlists", "/api/playlists/public"} {
		status, body := do(t, "GET", path+"?limit=100&include_songs=false", owner, nil)
		if status != http.StatusOK {
			t.Fatalf("metadata list: status %d, %v", status, body)
		}
		found := false
		for _, row := range items(t, body) {
			if row["id"] == float64(playlist.ID) {
				found = true
				if row["songs"] != nil {
					t.Fatalf("metadata included songs: %v", row)
				}
			}
		}
		if !found {
			t.Fatalf("metadata list did not include playlist %d", playlist.ID)
		}
	}
	membership := fmt.Sprintf("%s/contains?ids=%d,99999999", path, songs[0].ID)
	for _, tc := range []struct {
		token string
		want  int
	}{{owner, http.StatusOK}, {viewer, http.StatusNotFound}, {"", http.StatusUnauthorized}} {
		status, body := do(t, http.MethodGet, membership, tc.token, nil)
		if status != tc.want {
			t.Fatalf("membership: status %d, want %d", status, tc.want)
		}
		if status == http.StatusOK {
			ids, ok := body["song_ids"].([]any)
			if !ok || len(ids) != 1 || ids[0] != float64(songs[0].ID) {
				t.Fatalf("unexpected membership: %v", body)
			}
		}
	}
	status, body := do(t, "GET", path+"?limit=1", viewer, nil)
	if status != http.StatusOK || len(items(t, body)) != 1 {
		t.Fatalf("public page: status %d, %v", status, body)
	}
	status, body = do(t, "GET", fmt.Sprintf("/api/playlists/%d", playlist.ID), viewer, nil)
	legacySongs, ok := body["songs"].([]any)
	if status != http.StatusOK || !ok || len(legacySongs) != len(songs) {
		t.Fatalf("legacy detail: status %d, %d songs", status, len(legacySongs))
	}
}
