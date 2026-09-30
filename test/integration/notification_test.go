//go:build integration

package integration

import (
	"fmt"
	"testing"
)

// TestNotificationsOnFollow mengunci alur notifikasi: follow baru membuat satu
// notifikasi untuk penerima, follow berulang tidak menambah duplikat, actor
// tampil tanpa email, dan menandai dibaca bersifat idempoten.
func TestNotificationsOnFollow(t *testing.T) {
	user := login(t, "user@test.local", "Password123!")
	admin := login(t, "admin@test.local", "Password123!")

	followPath := fmt.Sprintf("/api/users/%s/follow", fixtureAdminID)

	// State bersih: pastikan tidak ada follow dan semua notifikasi terdahulu
	// sudah dibaca, supaya hitungannya deterministik.
	do(t, "DELETE", followPath, user, nil)
	do(t, "POST", "/api/me/notifications/read", admin, nil)
	t.Cleanup(func() {
		do(t, "DELETE", followPath, user, nil)
		do(t, "POST", "/api/me/notifications/read", admin, nil)
	})

	_, before := do(t, "GET", "/api/me/notifications?limit=50", admin, nil)
	unreadBefore := before["unread"].(float64)

	// Follow baru → satu notifikasi untuk admin.
	if status, body := do(t, "PUT", followPath, user, nil); status != 200 {
		t.Fatalf("follow = %d %v", status, body)
	}
	// Follow diulang (idempoten) tidak boleh menambah notifikasi kedua.
	if status, body := do(t, "PUT", followPath, user, nil); status != 200 {
		t.Fatalf("follow ulang = %d %v", status, body)
	}

	status, page := do(t, "GET", "/api/me/notifications?limit=50", admin, nil)
	if status != 200 {
		t.Fatalf("notifications = %d %v", status, page)
	}
	if page["unread"].(float64) != unreadBefore+1 {
		t.Fatalf("unread = %v, mau %v", page["unread"], unreadBefore+1)
	}

	entries := items(t, page)
	if len(entries) == 0 {
		t.Fatal("tidak ada notifikasi setelah follow")
	}

	first := entries[0]
	if first["type"] != "user_followed" {
		t.Fatalf("tipe notifikasi = %v, mau user_followed", first["type"])
	}
	if first["read_at"] != nil {
		t.Fatalf("notifikasi baru tidak boleh sudah bertanda dibaca: %v", first["read_at"])
	}

	actor, _ := first["actor"].(map[string]any)
	if actor == nil || actor["name"] != "User Test" {
		t.Fatalf("actor notifikasi salah: %v", first["actor"])
	}
	if _, leaked := actor["email"]; leaked {
		t.Fatal("actor notifikasi membocorkan email")
	}

	// Tandai dibaca, dua kali (idempoten) → unread jadi 0.
	if status, _ := do(t, "POST", "/api/me/notifications/read", admin, nil); status != 200 {
		t.Fatalf("mark read = %d, mau 200", status)
	}
	do(t, "POST", "/api/me/notifications/read", admin, nil)

	_, after := do(t, "GET", "/api/me/notifications?limit=50", admin, nil)
	if after["unread"].(float64) != 0 {
		t.Fatalf("unread setelah read = %v, mau 0", after["unread"])
	}

	// Endpoint butuh login.
	status, body := do(t, "GET", "/api/me/notifications", "", nil)
	if status != 401 || body["code"] != "UNAUTHORIZED" {
		t.Fatalf("notifications tanpa token = %d %v, mau 401", status, body)
	}
}
