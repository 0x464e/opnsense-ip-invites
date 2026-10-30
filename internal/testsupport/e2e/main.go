//go:build e2e

// Local browser-test fixture. This command never contacts a real firewall and is
// excluded from normal builds. Its header injection is deliberately test-only.
package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/0x464e/opnsense-ip-invites/internal/config"
	"github.com/0x464e/opnsense-ip-invites/internal/firewall"
	"github.com/0x464e/opnsense-ip-invites/internal/httpapi"
	"github.com/0x464e/opnsense-ip-invites/internal/service"
	"github.com/0x464e/opnsense-ip-invites/internal/store"
	"github.com/0x464e/opnsense-ip-invites/internal/testsupport"
	frontend "github.com/0x464e/opnsense-ip-invites/web"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}
func run() error {
	root, err := os.MkdirTemp("", "ip-invites-e2e-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(root)
	fake := testsupport.NewFirewall("INVITED_CLIENTS", "PROJECT_MEMBERS")
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key, secret, ok := r.BasicAuth()
		if !ok || key != "test-key" || secret != "test-secret" {
			w.WriteHeader(401)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		parts := strings.Split(r.URL.Path, "/")
		if len(parts) < 5 {
			w.WriteHeader(404)
			return
		}
		switch parts[3] + "/" + parts[4] {
		case "alias_util/aliases":
			active, _ := fake.ActiveAliases(r.Context())
			names := []string{}
			for name := range active {
				names = append(names, name)
			}
			json.NewEncoder(w).Encode(names)
		case "alias/add_item":
			var in struct {
				Alias struct {
					Name string `json:"name"`
				} `json:"alias"`
			}
			if json.NewDecoder(r.Body).Decode(&in) != nil {
				w.WriteHeader(400)
				return
			}
			if err := fake.CreateAlias(r.Context(), in.Alias.Name); err != nil {
				json.NewEncoder(w).Encode(map[string]string{"result": "failed"})
				return
			}
			// The browser fixture simulates an operator applying the new policy.
			fake.Mu.Lock()
			fake.Tables[in.Alias.Name] = map[string]bool{}
			fake.Mu.Unlock()
			json.NewEncoder(w).Encode(map[string]string{"result": "saved", "uuid": "fixture-id"})
		case "alias/search_item":
			defs, _ := fake.Aliases(r.Context())
			rows := []firewall.Alias{}
			for _, a := range defs {
				rows = append(rows, a)
			}
			json.NewEncoder(w).Encode(map[string]any{"rows": rows, "total": len(rows)})
		case "alias_util/list":
			if len(parts) != 6 {
				w.WriteHeader(404)
				return
			}
			table, _ := fake.List(r.Context(), parts[5])
			rows := []map[string]string{}
			for ip := range table {
				rows = append(rows, map[string]string{"ip": ip})
			}
			json.NewEncoder(w).Encode(map[string]any{"rows": rows, "total": len(rows)})
		case "alias_util/add", "alias_util/delete":
			if len(parts) != 6 || r.Method != "POST" {
				w.WriteHeader(400)
				return
			}
			var input map[string]string
			if json.NewDecoder(r.Body).Decode(&input) != nil {
				w.WriteHeader(400)
				return
			}
			if parts[4] == "add" {
				_ = fake.Add(r.Context(), parts[5], input["address"])
			} else {
				_ = fake.Delete(r.Context(), parts[5], input["address"])
			}
			json.NewEncoder(w).Encode(map[string]string{"status": "done"})
		default:
			w.WriteHeader(404)
		}
	}))
	defer remote.Close()
	c, err := config.Load("examples/config.yaml")
	if err != nil {
		return err
	}
	c.Database = filepath.Join(root, "invites.db")
	c.Server.PublicURL = "http://127.0.0.1:5173"
	c.Server.IPv4PublicURL = "http://127.0.0.1:5173"
	c.Server.ClientIPHeader = "CF-Connecting-IP"
	c.Defaults.LinkValidity.Duration = 2 * time.Hour
	c.Defaults.AccessDuration.Duration = 30 * time.Minute
	db, err := store.Open(c.Database)
	if err != nil {
		return err
	}
	defer db.Close()
	for _, l := range []store.List{{ID: "service-access", Name: "Service access", Description: "Authorize this connection to access the shared service.", Alias: "INVITED_CLIENTS"}, {ID: "project-members", Name: "Project members", Description: "Access the project workspace.", Alias: "PROJECT_MEMBERS"}} {
		if _, err = db.CreateList(context.Background(), l); err != nil {
			return err
		}
	}
	client, err := firewall.New(remote.URL, "test-key", "test-secret", "", time.Second)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	svc, err := service.New(ctx, c, db, client)
	if err != nil {
		return err
	}
	seed, token, err := svc.CreateLink(ctx, store.Link{ListID: "service-access", Label: "Weekend group", ExpiresAt: time.Now().Add(48 * time.Hour).Unix(), AccessSeconds: 86400, MaxIPs: 0})
	if err != nil {
		return err
	}
	if _, err = svc.Authorize(ctx, token, "8.8.4.4"); err != nil {
		return err
	}
	_, _, err = svc.CreateLink(ctx, store.Link{ListID: "project-members", Label: "Project invitation", ExpiresAt: time.Now().Add(72 * time.Hour).Unix(), AccessSeconds: 3600, MaxIPs: 1})
	if err != nil {
		return err
	}
	app, err := httpapi.New(svc, "fixture-secret-never-use-this-in-production", frontend.Assets())
	if err != nil {
		return err
	}
	handler := app.Handler()
	server := &http.Server{Addr: "127.0.0.1:18080", ReadHeaderTimeout: 5 * time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/__fixture/invite" {
			json.NewEncoder(w).Encode(map[string]string{"token": token, "id": seed.ID})
			return
		}
		ip := r.Header.Get("X-Test-IP")
		if ip == "" {
			ip = "8.8.8.8"
		}
		r.Header.Set("CF-Connecting-IP", ip)
		if strings.HasPrefix(r.URL.Path, "/admin") || strings.HasPrefix(r.URL.Path, "/api/admin/") {
			r.Header.Set("X-Invites-Admin-Secret", "fixture-secret-never-use-this-in-production")
		}
		handler.ServeHTTP(w, r)
	})}
	done := make(chan struct{})
	go func() { defer close(done); svc.Run(ctx) }()
	go func() { <-ctx.Done(); _ = server.Close() }()
	log.Print("Mock-firewall fixture listening on 127.0.0.1:18080")
	err = server.ListenAndServe()
	stop()
	<-done
	if err == http.ErrServerClosed {
		return nil
	}
	return err
}
