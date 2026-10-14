//go:build integration

package integration

import (
	"fmt"
	"testing"
)

// registerUser membuat akun baru untuk test ini sendiri, lalu login.
//
// Test cursor sengaja TIDAK memakai akun fixture `user@test.local`: riwayat
// dan state pemutaran yang ditinggalkan akan terbaca test lain (mis.
// TestPlayerStateAndPlay yang mengasumsikan state awalnya kosong). Akun
// sendiri membuat test ini tidak saling mengotori.
func registerUser(t *testing.T, email string) string {
	t.Helper()

	status, body := do(t, "POST", "/api/auth/register", "", map[string]any{
		"name":     "Cursor Tester",
		"email":    email,
		"password": "Password123!",
	})
	if status != 201 {
		t.Fatalf("register %s = %d %v", email, status, body)
	}

	return login(t, email, "Password123!")
}

// collectPages menelusuri seluruh halaman lewat next_cursor dan mengembalikan
// jumlah entri unik yang terkumpul. `key` menentukan identitas entri supaya
// duplikat antar halaman ketahuan.
func collectPages(t *testing.T, token, basePath string, first map[string]any, key func(map[string]any) string) int {
	t.Helper()

	seen := map[string]bool{}
	record := func(entries []map[string]any) {
		for _, entry := range entries {
			k := key(entry)
			if seen[k] {
				t.Fatalf("entri muncul dua kali antar halaman: %s", k)
			}
			seen[k] = true
		}
	}

	record(items(t, first))

	cursor, _ := first["next_cursor"].(string)
	pages := 1

	for cursor != "" && pages < 30 {
		status, page := do(t, "GET", fmt.Sprintf("%s&cursor=%s", basePath, cursor), token, nil)
		if status != 200 {
			t.Fatalf("halaman %d = %d %v", pages+1, status, page)
		}

		pageItems := items(t, page)
		if len(pageItems) == 0 {
			t.Fatalf("halaman %d kosong padahal next_cursor terisi", pages+1)
		}

		record(pageItems)
		cursor, _ = page["next_cursor"].(string)
		pages++
	}

	return len(seen)
}

// TestFeedCursorPagination mengunci keyset pagination feed: halaman pertama
// menyertakan next_cursor, menelusuri seluruh halaman menghasilkan jumlah yang
// sama dengan total tanpa duplikat, dan cursor rusak ditolak 400.
func TestFeedCursorPagination(t *testing.T) {
	admin := login(t, "admin@test.local", "Password123!")
	feedUser := registerUser(t, "cursor-feed@test.local")

	followPath := fmt.Sprintf("/api/users/%s/follow", fixtureAdminID)
	if status, _ := do(t, "PUT", followPath, feedUser, nil); status != 200 {
		t.Fatalf("follow = %d, mau 200", status)
	}
	t.Cleanup(func() { do(t, "DELETE", followPath, feedUser, nil) })

	// Tiga aktivitas supaya ada lebih dari satu halaman.
	for i := 0; i < 3; i++ {
		if status, body := do(t, "POST", "/api/me/player/play", admin, map[string]any{"song_id": fixtureSongID}); status != 200 {
			t.Fatalf("play ke-%d = %d %v", i+1, status, body)
		}
	}

	status, first := do(t, "GET", "/api/me/feed?limit=2", feedUser, nil)
	if status != 200 {
		t.Fatalf("feed = %d %v", status, first)
	}
	if len(items(t, first)) != 2 {
		t.Fatalf("halaman pertama = %d item, mau 2", len(items(t, first)))
	}

	cursor, ok := first["next_cursor"].(string)
	if !ok || cursor == "" {
		t.Fatalf("next_cursor kosong padahal masih ada halaman: %v", first)
	}

	total := int(first["total"].(float64))
	if total < 3 {
		t.Fatalf("total = %d, mau minimal 3", total)
	}

	key := func(entry map[string]any) string {
		objectID := "?"
		if song, ok := entry["song"].(map[string]any); ok {
			objectID = fmt.Sprint(song["id"])
		}
		if playlist, ok := entry["playlist"].(map[string]any); ok {
			objectID = fmt.Sprint(playlist["id"])
		}
		return fmt.Sprintf("%s|%s|%s", entry["type"], entry["created_at"], objectID)
	}

	collected := collectPages(t, feedUser, "/api/me/feed?limit=2", first, key)
	if collected != total {
		t.Fatalf("terkumpul %d entri, total %d", collected, total)
	}

	// Cursor rusak → 400 VALIDATION_FAILED.
	status, body := do(t, "GET", "/api/me/feed?cursor=abc", feedUser, nil)
	if status != 400 || body["code"] != "VALIDATION_FAILED" {
		t.Fatalf("cursor rusak = %d %v, mau 400 VALIDATION_FAILED", status, body)
	}
}

// TestHistoryCursorPagination mengunci keyset pagination riwayat, sekaligus
// memastikan mode offset lama tetap jalan dan tetap menyertakan next_cursor.
func TestHistoryCursorPagination(t *testing.T) {
	historyUser := registerUser(t, "cursor-history@test.local")

	for i := 0; i < 3; i++ {
		if status, body := do(t, "POST", "/api/me/player/play", historyUser, map[string]any{"song_id": fixtureSongID}); status != 200 {
			t.Fatalf("play ke-%d = %d %v", i+1, status, body)
		}
	}

	status, first := do(t, "GET", "/api/me/history?limit=2", historyUser, nil)
	if status != 200 {
		t.Fatalf("riwayat = %d %v", status, first)
	}
	if len(items(t, first)) != 2 {
		t.Fatalf("halaman pertama = %d item, mau 2", len(items(t, first)))
	}

	cursor, ok := first["next_cursor"].(string)
	if !ok || cursor == "" {
		t.Fatalf("next_cursor kosong padahal masih ada halaman: %v", first)
	}

	total := int(first["total"].(float64))
	if total < 3 {
		t.Fatalf("total = %d, mau minimal 3", total)
	}

	key := func(entry map[string]any) string {
		song, _ := entry["song"].(map[string]any)
		return fmt.Sprintf("%s|%v", entry["played_at"], song["id"])
	}

	collected := collectPages(t, historyUser, "/api/me/history?limit=2", first, key)
	if collected != total {
		t.Fatalf("terkumpul %d entri, total %d", collected, total)
	}

	// Mode offset tetap berperilaku seperti sebelumnya, dan kini ikut
	// menyertakan next_cursor supaya client bisa pindah mode kapan saja.
	status, offsetPage := do(t, "GET", "/api/me/history?limit=1&offset=0", historyUser, nil)
	if status != 200 || len(items(t, offsetPage)) != 1 {
		t.Fatalf("mode offset = %d %v", status, offsetPage)
	}
	if offsetPage["next_cursor"] == nil {
		t.Fatal("mode offset tidak menyertakan next_cursor")
	}
	if offsetPage["offset"].(float64) != 0 {
		t.Fatalf("offset = %v, mau 0", offsetPage["offset"])
	}

	// Cursor rusak → 400 VALIDATION_FAILED.
	status, body := do(t, "GET", "/api/me/history?cursor=abc", historyUser, nil)
	if status != 400 || body["code"] != "VALIDATION_FAILED" {
		t.Fatalf("cursor rusak = %d %v, mau 400 VALIDATION_FAILED", status, body)
	}
}
