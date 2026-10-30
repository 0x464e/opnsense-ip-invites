package firewall

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestHTTPClientPaginationCanonicalAddressesAndMutations(t *testing.T) {
	table := map[string]bool{"8.8.8.8/32": true}
	added := false
	deleted := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key, secret, ok := r.BasicAuth()
		if !ok || key != "key" || secret != "secret" {
			t.Error("missing API auth")
			w.WriteHeader(401)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/firewall/alias/search_item":
			page, _ := strconv.Atoi(r.URL.Query().Get("current"))
			rows := []Alias{}
			if page == 1 {
				for i := range 1000 {
					rows = append(rows, Alias{Name: fmt.Sprintf("A%d", i), Type: "external", Enabled: "1"})
				}
			} else {
				rows = append(rows, Alias{Name: "INVITED", Type: "external", Enabled: "1"})
			}
			json.NewEncoder(w).Encode(map[string]any{"rows": rows, "total": "1001"})
		case "/api/firewall/alias_util/list/INVITED":
			rows := []map[string]string{}
			for ip := range table {
				rows = append(rows, map[string]string{"ip": ip})
			}
			json.NewEncoder(w).Encode(map[string]any{"rows": rows, "total": len(rows)})
		case "/api/firewall/alias_util/add/INVITED", "/api/firewall/alias_util/delete/INVITED":
			if r.Method != "POST" || r.Header.Get("Content-Type") != "application/json" {
				t.Error("invalid mutation request")
			}
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			if r.URL.Path == "/api/firewall/alias_util/add/INVITED" {
				table[body["address"]] = true
				added = true
			} else {
				delete(table, body["address"]+"/32")
				deleted = true
			}
			json.NewEncoder(w).Encode(map[string]string{"status": "done"})
		default:
			t.Errorf("unexpected endpoint %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	c, err := New(server.URL, "key", "secret", "", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	aliases, err := c.Aliases(context.Background())
	if err != nil || len(aliases) != 1001 || aliases["INVITED"].Type != "external" {
		t.Fatal("truncated alias definitions", len(aliases), err)
	}
	addresses, err := c.List(context.Background(), "INVITED")
	if err != nil || !addresses["8.8.8.8"] {
		t.Fatal(addresses, err)
	}
	if err = c.Add(context.Background(), "INVITED", "8.8.4.4"); err != nil {
		t.Fatal(err)
	}
	if err = c.Delete(context.Background(), "INVITED", "8.8.8.8"); err != nil {
		t.Fatal(err)
	}
	if !added || !deleted {
		t.Fatal("mutations not exercised")
	}
}
func TestMalformedPaginationAndRedirectsFailClosed(t *testing.T) {
	for _, test := range []struct {
		name    string
		handler http.HandlerFunc
	}{
		{"missing rows", func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `{"status":"ok"}`) }},
		{"missing total", func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `{"rows":[]}`) }},
		{"redirect", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/login", 302) }},
		{"HTML login", func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, "<html>login</html>") }},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(test.handler)
			defer server.Close()
			c, _ := New(server.URL, "key", "secret", "", time.Second)
			if _, err := c.List(context.Background(), "INVITED"); err == nil {
				t.Fatal("invalid API response accepted")
			}
		})
	}
}
func TestTLSVerificationAndCustomCA(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { fmt.Fprint(w, `{"rows":[],"total":0}`) }))
	defer server.Close()
	c, _ := New(server.URL, "key", "secret", "", time.Second)
	if _, err := c.Aliases(context.Background()); err == nil {
		t.Fatal("untrusted certificate accepted")
	}
	cert, err := x509.ParseCertificate(server.TLS.Certificates[0].Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(t.TempDir(), "ca.pem")
	if err = os.WriteFile(p, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw}), 0600); err != nil {
		t.Fatal(err)
	}
	c, err = New(server.URL, "key", "secret", p, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = c.Aliases(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestCreateExternalAliasAndActiveTables(t *testing.T) {
	for _, result := range []string{`{"result":"saved","uuid":"new-id"}`, `{"result":"failed"}`, `{"result":"saved"}`} {
		t.Run(result, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/firewall/alias/add_item":
					if r.Method != "POST" {
						t.Error("creation must be POST")
					}
					var in struct {
						Alias map[string]string `json:"alias"`
					}
					if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
						t.Fatal(err)
					}
					if in.Alias["name"] != "INVITED" || in.Alias["type"] != "external" || in.Alias["enabled"] != "1" || in.Alias["proto"] != "IPv4" || in.Alias["content"] != "" {
						t.Error(in)
					}
					fmt.Fprint(w, result)
				case "/api/firewall/alias_util/aliases":
					fmt.Fprint(w, `["INVITED"]`)
				default:
					t.Error("unexpected endpoint", r.URL.Path)
					w.WriteHeader(404)
				}
			}))
			defer server.Close()
			c, _ := New(server.URL, "key", "secret", "", time.Second)
			err := c.CreateAlias(context.Background(), "INVITED")
			if (err == nil) != (result == `{"result":"saved","uuid":"new-id"}`) {
				t.Fatal(err)
			}
			active, err := c.ActiveAliases(context.Background())
			if err != nil || !active["INVITED"] {
				t.Fatal(active, err)
			}
		})
	}
}
