//go:build integration

package integration

import (
	"fmt"
	"testing"
)

// missingUserID adalah UUID yang tidak dibuat oleh fixture mana pun.
const missingUserID = "00000000-0000-4000-8000-000000000000"

// TestPublicProfile mengunci bentuk profil publik: data aman (tanpa email),
// statistik sosial, dan is_following yang bergantung pada viewer.
func TestPublicProfile(t *testing.T) {
	admin := login(t, "admin@test.local", "Password123!")
	userToken := login(t, "user@test.local", "Password123!")

	profilePath := fmt.Sprintf("/api/users/%s", fixtureUserID)

	// Anonim: profil tetap bisa dibuka, is_following false.
	status, profile := do(t, "GET", profilePath, "", nil)
	if status != 200 {
		t.Fatalf("profil anonim = %d %v", status, profile)
	}
	if profile["name"] != "User Test" {
		t.Fatalf("nama profil = %v", profile["name"])
	}
	if _, leaked := profile["email"]; leaked {
		t.Fatal("profil publik membocorkan email")
	}
	if profile["is_following"] != false {
		t.Fatalf("is_following anonim = %v, mau false", profile["is_following"])
	}
	if _, ok := profile["followers"].(float64); !ok {
		t.Fatalf("followers bukan angka: %v", profile["followers"])
	}

	// Viewer adalah user itu sendiri: is_following tetap false.
	_, own := do(t, "GET", profilePath, userToken, nil)
	if own["is_following"] != false {
		t.Fatalf("is_following diri sendiri = %v, mau false", own["is_following"])
	}

	// Admin mengikuti user -> is_following true untuk viewer admin.
	followPath := fmt.Sprintf("/api/users/%s/follow", fixtureUserID)
	t.Cleanup(func() {
		do(t, "DELETE", followPath, admin, nil)
	})

	if status, _ := do(t, "PUT", followPath, admin, nil); status != 200 {
		t.Fatalf("follow = %d, mau 200", status)
	}

	status, asAdmin := do(t, "GET", profilePath, admin, nil)
	if status != 200 || asAdmin["is_following"] != true {
		t.Fatalf("is_following viewer admin = %v", asAdmin["is_following"])
	}
	if asAdmin["followers"].(float64) < 1 {
		t.Fatalf("followers = %v, mau >= 1", asAdmin["followers"])
	}
}

// TestFollowFlow mengunci alur follow: idempoten, menolak follow diri sendiri,
// menolak user yang tidak ada, dan unfollow yang juga idempoten.
func TestFollowFlow(t *testing.T) {
	admin := login(t, "admin@test.local", "Password123!")
	userToken := login(t, "user@test.local", "Password123!")

	followPath := fmt.Sprintf("/api/users/%s/follow", fixtureUserID)
	t.Cleanup(func() {
		do(t, "DELETE", followPath, admin, nil)
	})

	if status, body := do(t, "PUT", followPath, admin, nil); status != 200 {
		t.Fatalf("follow = %d %v", status, body)
	}
	// Idempoten.
	if status, body := do(t, "PUT", followPath, admin, nil); status != 200 {
		t.Fatalf("follow ulang = %d %v, mau 200", status, body)
	}

	// Daftar following milik admin memuat user.
	_, following := do(t, "GET", fmt.Sprintf("/api/users/%s/following?limit=50", fixtureAdminID), "", nil)
	found := false
	for _, item := range items(t, following) {
		if item["name"] == "User Test" {
			found = true
		}
		if _, leaked := item["email"]; leaked {
			t.Fatal("daftar following membocorkan email")
		}
	}
	if !found {
		t.Fatalf("following admin tidak memuat user: %v", following)
	}

	// Daftar followers milik user memuat admin.
	_, followers := do(t, "GET", fmt.Sprintf("/api/users/%s/followers?limit=50", fixtureUserID), "", nil)
	foundAdmin := false
	for _, item := range items(t, followers) {
		if item["name"] == "Admin Test" {
			foundAdmin = true
		}
	}
	if !foundAdmin {
		t.Fatalf("followers user tidak memuat admin: %v", followers)
	}

	// Follow diri sendiri ditolak 422.
	status, body := do(t, "PUT", fmt.Sprintf("/api/users/%s/follow", fixtureAdminID), admin, nil)
	if status != 422 || body["code"] != "VALIDATION_FAILED" {
		t.Fatalf("follow diri sendiri = %d %v, mau 422", status, body)
	}

	// User yang tidak ada dibalas 404.
	status, body = do(t, "PUT", fmt.Sprintf("/api/users/%s/follow", missingUserID), admin, nil)
	if status != 404 || body["code"] != "NOT_FOUND" {
		t.Fatalf("follow user tak ada = %d %v, mau 404", status, body)
	}

	// UUID tidak valid dibalas 400.
	status, _ = do(t, "GET", "/api/users/not-a-uuid", "", nil)
	if status != 400 {
		t.Fatalf("uuid tidak valid = %d, mau 400", status)
	}

	// Unfollow idempoten.
	if status, body := do(t, "DELETE", followPath, admin, nil); status != 200 {
		t.Fatalf("unfollow = %d %v", status, body)
	}
	if status, body := do(t, "DELETE", followPath, admin, nil); status != 200 {
		t.Fatalf("unfollow ulang = %d %v, mau 200", status, body)
	}

	_, profile := do(t, "GET", fmt.Sprintf("/api/users/%s", fixtureUserID), admin, nil)
	if profile["is_following"] != false {
		t.Fatalf("is_following setelah unfollow = %v", profile["is_following"])
	}

	// User biasa boleh mengikuti siapa pun (memakai token user, target admin).
	otherPath := fmt.Sprintf("/api/users/%s/follow", fixtureAdminID)
	if status, body := do(t, "PUT", otherPath, userToken, nil); status != 200 {
		t.Fatalf("follow oleh user biasa = %d %v", status, body)
	}
	do(t, "DELETE", otherPath, userToken, nil)
}

// TestProfilePlaylists mengunci daftar playlist publik di profil: hanya yang
// publik yang tampil.
func TestProfilePlaylists(t *testing.T) {
	_, page := do(t, "GET", fmt.Sprintf("/api/users/%s/playlists?limit=50", fixtureUserID), "", nil)
	playlists := items(t, page)

	if len(playlists) == 0 {
		t.Fatal("profil tidak menampilkan playlist publik milik user")
	}
	for _, playlist := range playlists {
		if playlist["name"] == "Public Mix" {
			return
		}
	}
	t.Fatalf("playlist fixture tidak ditemukan: %v", playlists)
}

// TestOptionalAuth mengunci perilaku middleware: token salah tetap 401 walau
// endpointnya publik.
func TestOptionalAuth(t *testing.T) {
	status, body := do(t, "GET", fmt.Sprintf("/api/users/%s", fixtureUserID), "token-palsu", nil)
	if status != 401 || body["code"] != "UNAUTHORIZED" {
		t.Fatalf("token palsu di endpoint opsional = %d %v, mau 401", status, body)
	}
}
