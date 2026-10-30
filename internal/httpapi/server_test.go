package httpapi

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/0x464e/opnsense-ip-invites/internal/config"
	"github.com/0x464e/opnsense-ip-invites/internal/service"
	"github.com/0x464e/opnsense-ip-invites/internal/store"
	"github.com/0x464e/opnsense-ip-invites/internal/testsupport"
)

const testSecret = "test-only-admin-secret-with-at-least-32-characters"

func fixture(t *testing.T) (*Server, *testsupport.Firewall) {
	t.Helper()
	c, err := config.Load("../../examples/config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	c.Server.PublicURL = "https://invites.example.com"
	c.Server.IPv4PublicURL = "https://ipv4.example.com"
	c.Server.TrustedProxies = []string{"127.0.0.1/32", "192.0.2.0/24"}
	c.Server.ClientIPHeader = "X-Forwarded-For"
	c.Defaults.LinkValidity.Duration = 3 * time.Hour
	c.Defaults.AccessDuration.Duration = 17 * time.Minute
	c.Defaults.MaxIPs = 2
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	fw := testsupport.NewFirewall("INVITED_CLIENTS")
	svc, err := service.New(context.Background(), c, db, fw)
	if err != nil {
		t.Fatal(err)
	}
	svc.Now = func() time.Time { return time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC) }
	s, err := New(svc, testSecret, fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("test frontend")}})
	if err != nil {
		t.Fatal(err)
	}
	return s, fw
}
func request(s *Server, method, path, body, remote, forwarded, secret, origin string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.RemoteAddr = remote
	if forwarded != "" {
		r.Header.Set("X-Forwarded-For", forwarded)
	}
	if secret != "" {
		r.Header.Set("X-Invites-Admin-Secret", secret)
	}
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	if method == "POST" {
		r.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, r)
	return w
}
func TestAdminBoundariesAndCSRF(t *testing.T) {
	s, _ := fixture(t)
	for _, path := range []string{"/admin", "/admin/", "/admin/nested", "/admin%2Fsettings", "/%61dmin", "/api/admin/bootstrap", "/api/admin/links", "/api/admin/grants"} {
		for _, remote := range []string{"127.0.0.1:1234", "8.8.8.8:1234"} {
			secret := ""
			if remote == "8.8.8.8:1234" {
				secret = testSecret
			}
			w := request(s, "GET", path, "", remote, "", secret, "")
			if w.Code != 403 {
				t.Errorf("unprotected %s from %s: %d", path, remote, w.Code)
			}
		}
	}
	for _, origin := range []string{"", "https://evil.example"} {
		w := request(s, "POST", "/api/admin/links", `{}`, "127.0.0.1:1234", "", testSecret, origin)
		if w.Code != 403 {
			t.Fatal("cross-origin mutation allowed", w.Code)
		}
	}
	w := request(s, "GET", "/api/admin/bootstrap", "", "127.0.0.1:1234", "", testSecret, "")
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if !strings.Contains(w.Header().Get("Content-Security-Policy"), "frame-ancestors 'none'") || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("missing security headers")
	}
}
func TestForwardedAddressTrust(t *testing.T) {
	s, _ := fixture(t)
	for _, test := range []struct {
		peer, header, want string
		bad                bool
	}{
		{"8.8.8.8:1234", "1.1.1.1", "8.8.8.8", false},
		{"127.0.0.1:1234", "1.1.1.1, 8.8.8.8", "8.8.8.8", false},
		{"127.0.0.1:1234", "8.8.8.8, 192.0.2.5", "8.8.8.8", false},
		{"127.0.0.1:1234", "", "", true},
		{"127.0.0.1:1234", "not-an-address", "", true},
	} {
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = test.peer
		if test.header != "" {
			r.Header.Set("X-Forwarded-For", test.header)
		}
		got, err := s.visitor(r)
		if (err != nil) != test.bad || got != test.want {
			t.Errorf("visitor %s via %s = %q, %v", test.header, test.peer, got, err)
		}
	}
	s.svc.Config.Server.ClientIPHeader = "CF-Connecting-IP"
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "127.0.0.1:1234"
	r.Header.Set("CF-Connecting-IP", "8.8.8.8")
	r.Header.Set("X-Forwarded-For", "1.1.1.1")
	got, err := s.visitor(r)
	if err != nil || got != "8.8.8.8" {
		t.Fatal(got, err)
	}
	r.Header.Add("CF-Connecting-IP", "1.1.1.1")
	if _, err = s.visitor(r); err == nil {
		t.Fatal("ambiguous single-address header accepted")
	}
}
func TestCompleteAuthorizationFlowAndYAMLDefaults(t *testing.T) {
	s, fw := fixture(t)
	w := request(s, "POST", "/api/admin/links", `{"list_id":"service-access","label":"Friends"}`, "127.0.0.1:1234", "", testSecret, s.svc.Config.Server.PublicURL)
	if w.Code != 201 {
		t.Fatal(w.Code, w.Body.String())
	}
	var created struct {
		Link store.Link `json:"link"`
		URL  string     `json:"url"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.Link.AccessSeconds != 1020 || created.Link.MaxIPs != 2 || created.Link.ExpiresAt != s.svc.Now().Add(3*time.Hour).Unix() {
		t.Fatalf("ignored configured defaults: %+v", created.Link)
	}
	token := created.URL[strings.LastIndex(created.URL, "/")+1:]
	preview := "/api/public/invites/" + token
	w = request(s, "GET", preview, "", "127.0.0.1:1234", "8.8.8.8", "", "")
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	actual, _ := fw.List(context.Background(), "INVITED_CLIENTS")
	if len(actual) != 0 {
		t.Fatal("GET authorized access")
	}
	w = request(s, "POST", preview+"/authorize", `{"ip":"1.1.1.1"}`, "127.0.0.1:1234", "8.8.8.8", "", s.svc.Config.Server.PublicURL)
	if w.Code != 400 {
		t.Fatal("arbitrary IP body accepted")
	}
	w = request(s, "POST", preview+"/authorize", `{}`, "127.0.0.1:1234", "2001:db8::1", "", s.svc.Config.Server.PublicURL)
	if w.Code != 422 {
		t.Fatal("IPv6 accepted", w.Code)
	}
	w = request(s, "GET", preview, "", "127.0.0.1:1234", "2001:db8::1", "", "")
	if !strings.Contains(w.Body.String(), "https://ipv4.example.com/i/") {
		t.Fatal("missing IPv4 fallback")
	}
	w = request(s, "POST", preview+"/authorize", `{}`, "127.0.0.1:1234", "8.8.8.8", "", s.svc.Config.Server.PublicURL)
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	actual, _ = fw.List(context.Background(), "INVITED_CLIENTS")
	if !actual["8.8.8.8"] || actual["1.1.1.1"] {
		t.Fatal("wrong IP authorized", actual)
	}
	fw.Offline = true
	w = request(s, "POST", preview+"/authorize", `{}`, "127.0.0.1:1234", "8.8.4.4", "", s.svc.Config.Server.PublicURL)
	if w.Code != 503 || !strings.Contains(w.Body.String(), "sync_pending") {
		t.Fatal("false success during outage", w.Code, w.Body.String())
	}
	w = request(s, "POST", "/api/admin/links/"+created.Link.ID+"/revoke", `{}`, "127.0.0.1:1234", "", testSecret, s.svc.Config.Server.PublicURL)
	if w.Code != 202 {
		t.Fatal("saved revocation should be pending", w.Code)
	}
}
func TestPublicRateLimit(t *testing.T) {
	s, _ := fixture(t)
	for i := 0; i < 31; i++ {
		w := request(s, "GET", "/api/public/invites/"+strings.Repeat("x", 43), "", "8.8.8.8:1234", "", "", "")
		if i == 30 && w.Code != 429 {
			t.Fatal("rate limit missing")
		}
	}
}

func TestInvalidJSONAndDatabaseFailureAreNotSavedResponses(t *testing.T) {
	s, _ := fixture(t)
	for _, payload := range []string{"null", "[]", "{}{}"} {
		w := request(s, "POST", "/api/admin/links", payload, "127.0.0.1:1234", "", testSecret, s.svc.Config.Server.PublicURL)
		if w.Code != 400 {
			t.Errorf("accepted non-object or trailing JSON %s: %d", payload, w.Code)
		}
	}
	s.svc.DB.Close()
	w := request(s, "POST", "/api/admin/grants/missing/revoke", `{}`, "127.0.0.1:1234", "", testSecret, s.svc.Config.Server.PublicURL)
	if w.Code != 500 {
		t.Fatalf("database failure must not report saved/pending: %d", w.Code)
	}
}
