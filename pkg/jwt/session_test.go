package jwt

import (
	"github.com/google/uuid"
	"testing"
)

func TestSessionIDSharedByTokensAndPreservedDuringRotation(t *testing.T) {
	manager := newManager()
	userID, sid := uuid.New(), uuid.New()
	access, err := manager.GenerateAccessToken(userID, "user", sid)
	if err != nil {
		t.Fatal(err)
	}
	refresh, err := manager.GenerateRefreshToken(userID, "user", sid)
	if err != nil {
		t.Fatal(err)
	}
	a, err := manager.ParseAccessToken(access)
	if err != nil {
		t.Fatal(err)
	}
	b, err := manager.ParseRefreshToken(refresh)
	if err != nil {
		t.Fatal(err)
	}
	if a.SessionID != sid.String() || b.SessionID != sid.String() || a.ID == b.ID {
		t.Fatal("session ID or independent token IDs lost")
	}
	rotated, _ := manager.GenerateRefreshToken(userID, "user", sid)
	c, _ := manager.ParseRefreshToken(rotated)
	if c.SessionID != sid.String() || c.ID == b.ID {
		t.Fatal("rotation changed session or reused token ID")
	}
}
