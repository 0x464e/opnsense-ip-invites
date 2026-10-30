package service_test

import (
	"context"
	"errors"
	"github.com/0x464e/opnsense-ip-invites/internal/config"
	"github.com/0x464e/opnsense-ip-invites/internal/service"
	"github.com/0x464e/opnsense-ip-invites/internal/store"
	"github.com/0x464e/opnsense-ip-invites/internal/testsupport"
	"path/filepath"
	"testing"
	"time"
)

func TestListsCreatePendingApplyEditAndDelete(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "lists.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	f := testsupport.NewFirewall()
	s, err := service.New(ctx, config.Config{}, db, f)
	if err != nil {
		t.Fatal(err)
	}
	if !s.Ready() {
		t.Fatal("an empty installation must be usable")
	}
	l, err := s.CreateList(ctx, store.List{Name: "New workspace"})
	if !errors.Is(err, service.ErrSync) || l.ID == "" {
		t.Fatal(l, err)
	}
	if f.Definitions[l.Alias].Type != "external" || s.Ready() {
		t.Fatal("missing alias not created, or unapplied table reported ready")
	}
	if err = s.Sync(ctx); err == nil {
		t.Fatal("unapplied table must remain pending")
	} // Repeated sync must not duplicate definitions.
	if len(f.Definitions) != 1 {
		t.Fatal("duplicate alias")
	}
	f.Tables[l.Alias] = map[string]bool{} // Operator references/applies firewall policy.
	syncNow(t, s)
	l.Name = "Renamed workspace"
	l.Description = "Shared access"
	if err = s.UpdateList(ctx, l); err != nil {
		t.Fatal(err)
	}
	link, token, err := s.CreateLink(ctx, store.Link{ListID: l.ID, Label: "Access", ExpiresAt: time.Now().Add(time.Hour).Unix(), AccessSeconds: 3600})
	if err != nil {
		t.Fatal(err)
	}
	authorize(t, s, token, "8.8.8.8")
	if err = s.DeleteList(ctx, l.ID); err != nil {
		t.Fatal(err)
	}
	has(t, f, l.Alias, "8.8.8.8", false)
	if _, err = db.List(ctx, l.ID); !errors.Is(err, store.ErrNotFound) {
		t.Fatal("deleted list remains available", err)
	}
	if _, _, err = s.Preview(ctx, token); !errors.Is(err, store.ErrRevoked) {
		t.Fatal("deleted list link remains usable", err)
	}
	links, _ := db.Links(ctx, "revoked", 1, 25, time.Now())
	if len(links.Items) != 1 || links.Items[0].ID != link.ID {
		t.Fatal(links)
	}
	if _, err = s.CreateList(ctx, store.List{Name: "Reuse", Alias: l.Alias}); !errors.Is(err, store.ErrConflict) {
		t.Fatal("retired alias reused", err)
	}
	if _, ok := f.Definitions[l.Alias]; !ok {
		t.Fatal("alias definition deleted")
	}
}
func TestListCreationOfflineAndWrongExistingAlias(t *testing.T) {
	s, f, _ := fixture(t)
	f.IgnoreMutations = true
	l, err := s.CreateList(ctx, store.List{Name: "Offline list"})
	if !errors.Is(err, service.ErrSync) || l.ID == "" {
		t.Fatal(err)
	}
	f.IgnoreMutations = false
	_ = s.Sync(ctx)
	if _, ok := f.Definitions[l.Alias]; !ok {
		t.Fatal("retry did not create missing alias")
	}
	a := f.Definitions["INVITED"]
	a.Type = "host"
	f.Definitions["INVITED"] = a
	if _, err = s.CreateList(ctx, store.List{Name: "Unsafe", Alias: "INVITED"}); !errors.Is(err, service.ErrInput) {
		t.Fatal(err)
	}
	a.Type = "external"
	f.Definitions["INVITED"] = a
	f.Tables["INVITED"]["8.8.8.8"] = true
	if _, err = s.CreateList(context.Background(), store.List{Name: "Occupied", Alias: "INVITED"}); !errors.Is(err, service.ErrInput) {
		t.Fatal(err)
	}
	if _, err = s.CreateList(ctx, store.List{Name: "Bad", Alias: "bad/../alias"}); !errors.Is(err, service.ErrInput) {
		t.Fatal(err)
	}
}
