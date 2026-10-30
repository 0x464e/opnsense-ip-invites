package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestListsPersistAndDeletionRevokesHistory(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "lists.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	l := List{ID: "workspace", Name: "Workspace", Alias: "WORKSPACE"}
	if _, err = db.CreateList(ctx, l); err != nil {
		t.Fatal(err)
	}
	link, token, err := db.CreateLink(ctx, Link{ListID: l.ID, Label: "Session", ExpiresAt: time.Now().Add(time.Hour).Unix(), AccessSeconds: 3600}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Authorize(ctx, link.ID, "8.8.8.8", time.Now()); err != nil {
		t.Fatal(err)
	}
	l.Name = "Updated"
	if err = db.UpdateList(ctx, l); err != nil {
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	saved, err := db.List(ctx, l.ID)
	if err != nil || saved.Name != "Updated" {
		t.Fatal(saved, err)
	}
	if err = db.DeleteList(ctx, l.ID, time.Now()); err != nil {
		t.Fatal(err)
	}
	lists, err := db.Lists(ctx)
	if err != nil || len(lists) != 0 {
		t.Fatal(lists, err)
	}
	desired, err := db.Desired(ctx, l.ID, time.Now())
	if err != nil || len(desired) != 0 {
		t.Fatal(desired, err)
	}
	old, err := db.LookupToken(ctx, token)
	if err != nil || old.RevokedAt == 0 {
		t.Fatal(old, err)
	}
	aliases, err := db.Aliases(ctx)
	if err != nil || len(aliases) != 1 || aliases[0] != l.Alias {
		t.Fatal(aliases, err)
	}
	if _, err = db.CreateList(ctx, List{ID: "replacement", Name: "Reuse", Alias: l.Alias}); !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
}
