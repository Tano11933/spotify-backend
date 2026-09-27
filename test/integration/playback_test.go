//go:build integration

package integration

import (
	"fmt"
	"testing"
)

// TestPlayerStateAndPlay mengunci alur playback: state kosong di awal,
// sinkronisasi posisi TIDAK mencatat riwayat, dan play mencatat riwayat +
// menaikkan play_count.
func TestPlayerStateAndPlay(t *testing.T) {
	user := login(t, "user@test.local", "Password123!")

	// Awal: belum ada state — 200 dengan song null, bukan 404.
	status, state := do(t, "GET", "/api/me/player", user, nil)
	if status != 200 {
		t.Fatalf("GET state = %d, mau 200", status)
	}
	if state["song"] != nil {
		t.Fatalf("state awal harus kosong: %v", state["song"])
	}

	// Sinkronisasi posisi (resume lintas device).
	status, state = do(t, "PUT", "/api/me/player", user, map[string]any{
		"song_id": fixtureSongID, "position_seconds": 42,
	})
	if status != 200 {
		t.Fatalf("PUT state = %d %v", status, state)
	}
	song, _ := state["song"].(map[string]any)
	if song == nil || uint(song["id"].(float64)) != fixtureSongID {
		t.Fatalf("state tidak menunjuk lagu yang benar: %v", state["song"])
	}
	if state["position_seconds"].(float64) != 42 {
		t.Fatalf("position_seconds = %v, mau 42", state["position_seconds"])
	}

	// UpdateState bukan "play" — riwayat harus tetap kosong.
	_, history := do(t, "GET", "/api/me/history", user, nil)
	if history["total"].(float64) != 0 {
		t.Fatalf("update state tidak boleh mencatat riwayat, total = %v", history["total"])
	}

	// Play: mencatat riwayat + menaikkan play_count.
	_, before := do(t, "GET", fmt.Sprintf("/api/songs/%d", fixtureSongID), "", nil)
	beforeCount := before["play_count"].(float64)

	status, state = do(t, "POST", "/api/me/player/play", user, map[string]any{"song_id": fixtureSongID})
	if status != 200 {
		t.Fatalf("play = %d %v", status, state)
	}
	if state["position_seconds"].(float64) != 0 {
		t.Fatalf("play harus mereset posisi ke 0: %v", state["position_seconds"])
	}

	_, after := do(t, "GET", fmt.Sprintf("/api/songs/%d", fixtureSongID), "", nil)
	afterCount := after["play_count"].(float64)
	if afterCount != beforeCount+1 {
		t.Fatalf("play_count tidak bertambah: %v → %v", beforeCount, afterCount)
	}

	_, history = do(t, "GET", "/api/me/history?limit=10", user, nil)
	entries := items(t, history)
	if len(entries) == 0 {
		t.Fatal("riwayat kosong setelah play")
	}
	if _, ok := entries[0]["song"].(map[string]any); !ok {
		t.Fatalf("entri riwayat tidak menyertakan lagu: %v", entries[0])
	}

	// Lagu tak dikenal → 404.
	status, _ = do(t, "POST", "/api/me/player/play", user, map[string]any{"song_id": 999999})
	if status != 404 {
		t.Fatalf("play lagu tak dikenal = %d, mau 404", status)
	}
}

// TestPlayerQueue mengunci antrean: tambah, daftar, hapus idempoten.
func TestPlayerQueue(t *testing.T) {
	user := login(t, "user@test.local", "Password123!")

	_, queue := do(t, "GET", "/api/me/player/queue", user, nil)
	initial := queue["total"].(float64)

	status, _ := do(t, "POST", "/api/me/player/queue", user, map[string]any{"song_id": fixtureSongID})
	if status != 200 {
		t.Fatalf("tambah antrean = %d, mau 200", status)
	}

	_, queue = do(t, "GET", "/api/me/player/queue", user, nil)
	if queue["total"].(float64) != initial+1 {
		t.Fatalf("total antrean = %v, mau %v", queue["total"], initial+1)
	}
	queueItems := items(t, queue)
	if uint(queueItems[0]["id"].(float64)) != fixtureSongID {
		t.Fatalf("antrean tidak memuat lagu yang ditambahkan: %v", queueItems)
	}

	status, _ = do(t, "POST", "/api/me/player/queue", user, map[string]any{"song_id": 999999})
	if status != 404 {
		t.Fatalf("tambah lagu tak dikenal = %d, mau 404", status)
	}

	removePath := fmt.Sprintf("/api/me/player/queue/%d", fixtureSongID)
	if status, _ := do(t, "DELETE", removePath, user, nil); status != 200 {
		t.Fatalf("hapus dari antrean = %d, mau 200", status)
	}
	// Idempoten: menghapus yang tidak ada tetap 200.
	if status, _ := do(t, "DELETE", removePath, user, nil); status != 200 {
		t.Fatalf("hapus ulang = %d, mau 200", status)
	}

	_, queue = do(t, "GET", "/api/me/player/queue", user, nil)
	if queue["total"].(float64) != initial {
		t.Fatalf("total antrean setelah hapus = %v, mau %v", queue["total"], initial)
	}
}

// TestPlayerRequiresAuth mengunci guard: semua endpoint /me butuh login.
func TestPlayerRequiresAuth(t *testing.T) {
	status, body := do(t, "GET", "/api/me/player", "", nil)
	if status != 401 || body["code"] != "UNAUTHORIZED" {
		t.Fatalf("tanpa token = %d %v, mau 401 UNAUTHORIZED", status, body)
	}
}
