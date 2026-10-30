package store

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPersistenceTokenHashAndSingleInstance(t *testing.T) {
	path := filepath.Join(t.TempDir(), "invites.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if other, err := Open(path); err == nil {
		other.Close()
		t.Fatal("second instance acquired database")
	}
	ctx := context.Background()
	now := time.Now()
	l, token, err := s.CreateLink(ctx, Link{ListID: "list", Label: "Test", ExpiresAt: now.Add(time.Hour).Unix(), AccessSeconds: 600}, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(token) != 43 {
		t.Fatal("expected 256-bit token")
	}
	var hash string
	if err = s.db.QueryRow("SELECT token_hash FROM links WHERE id=?", l.ID).Scan(&hash); err != nil || hash == token || hash != tokenHash(token) {
		t.Fatal("raw token stored or hash incorrect")
	}
	if _, err = s.Authorize(ctx, l.ID, "8.8.8.8", now); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err = s.LookupToken(ctx, token); err != nil {
		t.Fatal(err)
	}
	desired, err := s.Desired(ctx, "list", now)
	if err != nil || !desired["8.8.8.8"] {
		t.Fatalf("lost persistent grant: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("database permissions must be 0600")
	}
}
