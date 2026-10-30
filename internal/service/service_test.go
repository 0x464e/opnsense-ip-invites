package service_test

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/0x464e/opnsense-ip-invites/internal/config"
	"github.com/0x464e/opnsense-ip-invites/internal/service"
	"github.com/0x464e/opnsense-ip-invites/internal/store"
	"github.com/0x464e/opnsense-ip-invites/internal/testsupport"
)

var ctx = context.Background()

func fixture(t *testing.T) (*service.Service, *testsupport.Firewall, *time.Time) {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	var c config.Config
	c.Lists = []config.List{{ID: "access", Name: "Access", Alias: "INVITED"}, {ID: "other", Name: "Other", Alias: "OTHER"}}
	c.SyncInterval.Duration = time.Second
	f := testsupport.NewFirewall("INVITED", "OTHER")
	s, err := service.New(ctx, c, db, f)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)
	s.Now = func() time.Time { return now }
	return s, f, &now
}
func invite(t *testing.T, s *service.Service, access time.Duration, limit int) (store.Link, string) {
	t.Helper()
	l, token, err := s.CreateLink(ctx, store.Link{ListID: "access", Label: "Group", ExpiresAt: s.Now().Add(7 * 24 * time.Hour).Unix(), AccessSeconds: int64(access.Seconds()), MaxIPs: limit})
	if err != nil {
		t.Fatal(err)
	}
	return l, token
}
func authorize(t *testing.T, s *service.Service, token, ip string) store.Grant {
	t.Helper()
	g, err := s.Authorize(ctx, token, ip)
	if err != nil {
		t.Fatal(err)
	}
	return g
}
func has(t *testing.T, f *testsupport.Firewall, alias, ip string, want bool) {
	t.Helper()
	m, err := f.List(ctx, alias)
	if err != nil {
		t.Fatal(err)
	}
	if m[ip] != want {
		t.Fatalf("%s membership of %s = %v, want %v", alias, ip, m[ip], want)
	}
}
func syncNow(t *testing.T, s *service.Service) {
	t.Helper()
	if err := s.Sync(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestOverlappingGrantsSurviveOldExpiry(t *testing.T) {
	s, f, now := fixture(t)
	_, friday := invite(t, s, 24*time.Hour, 0)
	authorize(t, s, friday, "8.8.8.8")
	*now = now.Add(23 * time.Hour)
	_, saturday := invite(t, s, 24*time.Hour, 0)
	latest := authorize(t, s, saturday, "8.8.8.8")
	*now = now.Add(2 * time.Hour)
	syncNow(t, s)
	has(t, f, "INVITED", "8.8.8.8", true)
	p, err := s.DB.Grants(ctx, "active", 1, 25, *now)
	if err != nil || len(p.Items) != 1 || p.Items[0].EffectiveExpiresAt != latest.ExpiresAt {
		t.Fatalf("wrong effective grant: %+v, %v", p, err)
	}
	*now = now.Add(23 * time.Hour)
	syncNow(t, s)
	has(t, f, "INVITED", "8.8.8.8", false)
}
func TestRevocationDoesNotRemoveOtherLinkGrant(t *testing.T) {
	s, f, _ := fixture(t)
	old, a := invite(t, s, time.Hour, 0)
	_, b := invite(t, s, 2*time.Hour, 0)
	authorize(t, s, a, "8.8.8.8")
	g := authorize(t, s, b, "8.8.8.8")
	if err := s.RevokeLink(ctx, old.ID, true); err != nil {
		t.Fatal(err)
	}
	has(t, f, "INVITED", "8.8.8.8", true)
	if err := s.RevokeGrant(ctx, g.ID); err != nil {
		t.Fatal(err)
	}
	has(t, f, "INVITED", "8.8.8.8", false)
}
func TestIndependentExpiryAndRevokeWithoutGrants(t *testing.T) {
	s, f, now := fixture(t)
	l, token := invite(t, s, 3*time.Hour, 0)
	l.ExpiresAt = now.Add(time.Minute).Unix()
	// Exercise a link whose validity is shorter than its granted access.
	short, secret, err := s.CreateLink(ctx, l)
	if err != nil {
		t.Fatal(err)
	}
	authorize(t, s, secret, "8.8.8.8")
	*now = now.Add(2 * time.Minute)
	syncNow(t, s)
	has(t, f, "INVITED", "8.8.8.8", true)
	if _, _, err = s.Preview(ctx, secret); !errors.Is(err, store.ErrExpired) {
		t.Fatalf("expected expired invite, got %v", err)
	}
	authorize(t, s, token, "8.8.4.4")
	if err = s.RevokeLink(ctx, l.ID, false); err != nil {
		t.Fatal(err)
	}
	has(t, f, "INVITED", "8.8.4.4", true)
	if err = s.RevokeLink(ctx, short.ID, false); err != nil {
		t.Fatal(err)
	}
	has(t, f, "INVITED", "8.8.8.8", true)
}
func TestReplacementAndCapacityArePerLink(t *testing.T) {
	s, f, _ := fixture(t)
	_, personal := invite(t, s, time.Hour, 1)
	_, shared := invite(t, s, time.Hour, 2)
	authorize(t, s, personal, "8.8.8.8")
	authorize(t, s, shared, "8.8.8.8")
	authorize(t, s, personal, "8.8.4.4")
	has(t, f, "INVITED", "8.8.8.8", true)
	has(t, f, "INVITED", "8.8.4.4", true)
	authorize(t, s, shared, "1.1.1.1")
	if _, err := s.Authorize(ctx, shared, "9.9.9.9"); !errors.Is(err, store.ErrCapacity) {
		t.Fatalf("expected capacity error, got %v", err)
	}
	authorize(t, s, shared, "8.8.8.8") // Existing IP may renew even at capacity.
}
func TestRenewalAndDistinctLists(t *testing.T) {
	s, f, now := fixture(t)
	_, a := invite(t, s, time.Hour, 0)
	g1 := authorize(t, s, a, "8.8.8.8")
	*now = now.Add(time.Minute)
	g2 := authorize(t, s, a, "8.8.8.8")
	if g1.ID != g2.ID || g2.ExpiresAt <= g1.ExpiresAt {
		t.Fatal("renewal must reuse the grant and extend expiry")
	}
	l, token, err := s.CreateLink(ctx, store.Link{ListID: "other", Label: "Other", ExpiresAt: now.Add(time.Hour).Unix(), AccessSeconds: 3600})
	if err != nil {
		t.Fatal(err)
	}
	authorize(t, s, token, "8.8.8.8")
	if err = s.RevokeLink(ctx, l.ID, true); err != nil {
		t.Fatal(err)
	}
	has(t, f, "OTHER", "8.8.8.8", false)
	has(t, f, "INVITED", "8.8.8.8", true)
}
func TestFailurePendingRestartAndRetirement(t *testing.T) {
	s, f, _ := fixture(t)
	_, a := invite(t, s, time.Hour, 0)
	f.Offline = true
	if _, err := s.Authorize(ctx, a, "8.8.8.8"); !errors.Is(err, service.ErrSync) {
		t.Fatalf("expected pending, got %v", err)
	}
	if s.Ready() {
		t.Fatal("offline firewall cannot be ready")
	}
	f.Offline = false
	syncNow(t, s)
	has(t, f, "INVITED", "8.8.8.8", true)
	f.Tables["INVITED"] = map[string]bool{}
	syncNow(t, s)
	has(t, f, "INVITED", "8.8.8.8", true)
	c := s.Config
	c.Lists = c.Lists[1:]
	restarted, err := service.New(ctx, c, s.DB, f)
	if err != nil {
		t.Fatal(err)
	}
	restarted.Now = s.Now
	syncNow(t, restarted)
	has(t, f, "INVITED", "8.8.8.8", false)
	if _, _, err = restarted.Preview(ctx, a); !errors.Is(err, service.ErrList) {
		t.Fatalf("removed list usable: %v", err)
	}
}
func TestAcknowledgementWithoutMutationIsNotSuccess(t *testing.T) {
	s, f, _ := fixture(t)
	_, token := invite(t, s, time.Hour, 0)
	f.IgnoreMutations = true
	if _, err := s.Authorize(ctx, token, "8.8.8.8"); !errors.Is(err, service.ErrSync) {
		t.Fatalf("unconfirmed membership reported success: %v", err)
	}
}
func TestNonExternalAliasIsNeverChanged(t *testing.T) {
	s, f, _ := fixture(t)
	a := f.Definitions["INVITED"]
	a.Type = "host"
	f.Definitions["INVITED"] = a
	_, token := invite(t, s, time.Hour, 0)
	if _, err := s.Authorize(ctx, token, "8.8.8.8"); !errors.Is(err, service.ErrSync) {
		t.Fatal(err)
	}
	has(t, f, "INVITED", "8.8.8.8", false)
}
func TestConcurrentExpiryAndAuthorizations(t *testing.T) {
	s, f, now := fixture(t)
	_, old := invite(t, s, time.Minute, 0)
	authorize(t, s, old, "8.8.8.8")
	*now = now.Add(2 * time.Minute)
	_, fresh := invite(t, s, time.Hour, 0)
	var wg sync.WaitGroup
	errs := make(chan error, 20)
	for range 10 {
		wg.Go(func() { errs <- s.Sync(ctx) })
		wg.Go(func() { _, err := s.Authorize(ctx, fresh, "8.8.8.8"); errs <- err })
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	has(t, f, "INVITED", "8.8.8.8", true)
}
func TestPublicIPv4(t *testing.T) {
	for _, raw := range []string{"::1", "2001:db8::1", "127.0.0.1", "10.0.0.1", "100.64.0.2", "240.1.2.3", "0.0.0.0", "bad"} {
		if _, err := service.PublicIPv4(raw); err == nil {
			t.Errorf("accepted %s", raw)
		}
	}
	for _, raw := range []string{"8.8.8.8", "::ffff:8.8.8.8"} {
		if ip, err := service.PublicIPv4(raw); err != nil || ip.String() != "8.8.8.8" {
			t.Errorf("rejected public IPv4: %s, %v", raw, err)
		}
	}
}

func TestReadinessWaitsForEveryAlias(t *testing.T) {
	s, _, _ := fixture(t)
	if s.Ready() {
		t.Fatal("not-yet-synchronized aliases cannot be ready")
	}
	_, token := invite(t, s, time.Hour, 0)
	authorize(t, s, token, "8.8.8.8")
	if s.Ready() {
		t.Fatal("one synchronized list cannot hide an uninitialized list")
	}
	syncNow(t, s)
	if !s.Ready() {
		t.Fatal("all synchronized aliases should be ready")
	}
}
