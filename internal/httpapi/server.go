package httpapi

import (
	"bytes"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/0x464e/opnsense-ip-invites/internal/service"
	"github.com/0x464e/opnsense-ip-invites/internal/store"
)

type Server struct {
	svc      *service.Service
	secret   string
	assets   fs.FS
	trusted  []netip.Prefix
	limitsMu sync.Mutex
	limits   map[string]bucket
}
type bucket struct {
	count int
	until time.Time
}

func New(svc *service.Service, secret string, assets fs.FS) (*Server, error) {
	if len(secret) < 32 {
		return nil, errors.New("INVITES_ADMIN_PROXY_SECRET must contain at least 32 characters")
	}
	s := &Server{svc: svc, secret: secret, assets: assets, limits: map[string]bucket{}}
	for _, p := range svc.Config.Server.TrustedProxies {
		s.trusted = append(s.trusted, netip.MustParsePrefix(p))
	}
	return s, nil
}
func (s *Server) trustedIP(ip netip.Addr) bool {
	for _, p := range s.trusted {
		if p.Contains(ip.Unmap()) {
			return true
		}
	}
	return false
}
func peer(r *http.Request) (netip.Addr, error) {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return netip.Addr{}, err
	}
	return netip.ParseAddr(host)
}
func (s *Server) visitor(r *http.Request) (string, error) {
	ip, err := peer(r)
	if err != nil {
		return "", errors.New("invalid connection address")
	}
	ip = ip.Unmap()
	header := s.svc.Config.Server.ClientIPHeader
	if header == "" || !s.trustedIP(ip) {
		return ip.String(), nil
	}
	values := r.Header.Values(header)
	if len(values) != 1 || strings.TrimSpace(values[0]) == "" {
		return "", errors.New("trusted proxy did not supply a single visitor address header")
	}
	if header != "X-Forwarded-For" {
		addr, err := netip.ParseAddr(strings.TrimSpace(values[0]))
		if err != nil {
			return "", errors.New("invalid visitor address header")
		}
		return addr.Unmap().String(), nil
	}
	parts := strings.Split(values[0], ",")
	if len(parts) > 32 {
		return "", errors.New("proxy chain is too long")
	}
	// Walk from the verified peer toward the client, stopping at the first untrusted hop.
	for i := len(parts) - 1; i >= 0 && s.trustedIP(ip); i-- {
		ip, err = netip.ParseAddr(strings.TrimSpace(parts[i]))
		if err != nil {
			return "", errors.New("invalid forwarded address chain")
		}
		ip = ip.Unmap()
	}
	return ip.String(), nil
}
func (s *Server) originOK(r *http.Request) bool {
	v := r.Header.Get("Origin")
	return v != "" && (v == s.svc.Config.Server.PublicURL || v == s.svc.Config.Server.IPv4PublicURL)
}
func (s *Server) authenticatedProxy(r *http.Request) bool {
	ip, err := peer(r)
	values := r.Header.Values("X-Invites-Admin-Secret")
	return err == nil && s.trustedIP(ip) && len(values) == 1 && subtle.ConstantTimeCompare([]byte(values[0]), []byte(s.secret)) == 1
}
func (s *Server) admin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.authenticatedProxy(r) {
			problem(w, 403, "admin_protected", "Admin access requires the configured authenticated reverse proxy.")
			return
		}
		if r.Method != "GET" && r.Method != "HEAD" && !s.originOK(r) {
			problem(w, 403, "invalid_origin", "Request origin is not allowed.")
			return
		}
		next(w, r)
	}
}
func (s *Server) allow(key string) bool {
	s.limitsMu.Lock()
	defer s.limitsMu.Unlock()
	now := time.Now()
	if len(s.limits) > 10000 {
		for k, b := range s.limits {
			if !now.Before(b.until) {
				delete(s.limits, k)
			}
		}
	}
	b, exists := s.limits[key]
	if !exists && len(s.limits) > 10000 {
		return false
	}
	if !now.Before(b.until) {
		b = bucket{until: now.Add(time.Minute)}
	}
	b.count++
	s.limits[key] = b
	return b.count <= 30
}
func (s *Server) public(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		address, err := s.visitor(r)
		if err != nil {
			address = r.RemoteAddr
		}
		if !s.allow(address) {
			w.Header().Set("Retry-After", "60")
			problem(w, 429, "rate_limited", "Too many requests. Try again in a minute.")
			return
		}
		if len(r.PathValue("token")) != 43 {
			problem(w, 404, "not_found", "This invitation could not be found.")
			return
		}
		if r.Method == "POST" && !s.originOK(r) {
			problem(w, 403, "invalid_origin", "Request origin is not allowed.")
			return
		}
		next(w, r)
	}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) { write(w, 200, map[string]string{"status": "ok"}) })
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		if !s.svc.Ready() {
			write(w, 503, map[string]string{"status": "not_ready"})
			return
		}
		write(w, 200, map[string]string{"status": "ready"})
	})
	mux.HandleFunc("GET /api/public/invites/{token}", s.public(s.preview))
	mux.HandleFunc("POST /api/public/invites/{token}/authorize", s.public(s.authorize))
	mux.HandleFunc("GET /api/admin/bootstrap", s.admin(s.bootstrap))
	mux.HandleFunc("POST /api/admin/lists", s.admin(s.createList))
	mux.HandleFunc("POST /api/admin/lists/{id}", s.admin(s.updateList))
	mux.HandleFunc("POST /api/admin/lists/{id}/delete", s.admin(s.deleteList))
	mux.HandleFunc("GET /api/admin/links", s.admin(s.links))
	mux.HandleFunc("POST /api/admin/links", s.admin(s.createLink))
	mux.HandleFunc("POST /api/admin/links/{id}/revoke", s.admin(s.revokeLink))
	mux.HandleFunc("GET /api/admin/grants", s.admin(s.grants))
	mux.HandleFunc("POST /api/admin/grants/{id}/revoke", s.admin(s.revokeGrant))
	mux.HandleFunc("POST /api/admin/sync", s.admin(s.synchronize))
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) { problem(w, 404, "not_found", "API endpoint not found.") })
	mux.HandleFunc("/admin", s.admin(s.static))
	mux.HandleFunc("/admin/", s.admin(s.static))
	mux.HandleFunc("/", s.static)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		mux.ServeHTTP(w, r)
	})
}
func write(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func problem(w http.ResponseWriter, status int, code, message string) {
	write(w, status, map[string]string{"code": code, "message": message})
}
func body(w http.ResponseWriter, r *http.Request, v any) bool {
	if strings.Split(r.Header.Get("Content-Type"), ";")[0] != "application/json" {
		problem(w, 415, "invalid_body", "Send an application/json request.")
		return false
	}
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 8192))
	if err != nil || len(bytes.TrimSpace(data)) == 0 || bytes.TrimSpace(data)[0] != '{' {
		problem(w, 400, "invalid_body", "Send exactly one JSON object.")
		return false
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		problem(w, 400, "invalid_body", "Invalid JSON request.")
		return false
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		problem(w, 400, "invalid_body", "Send exactly one JSON object.")
		return false
	}
	return true
}
func (s *Server) fail(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		problem(w, 404, "not_found", "This invitation or access grant could not be found.")
	case errors.Is(err, store.ErrExpired):
		problem(w, 410, "expired", err.Error())
	case errors.Is(err, store.ErrRevoked):
		problem(w, 410, "revoked", err.Error())
	case errors.Is(err, store.ErrCapacity):
		problem(w, 409, "capacity", err.Error())
	case errors.Is(err, service.ErrList):
		problem(w, 410, "list_removed", err.Error())
	case errors.Is(err, service.ErrSync):
		problem(w, 503, "sync_pending", err.Error())
	default:
		slog.Error("request failed", "error", err)
		problem(w, 500, "internal_error", "The request could not be completed. Try again shortly.")
	}
}
func (s *Server) preview(w http.ResponseWriter, r *http.Request) {
	l, list, err := s.svc.Preview(r.Context(), r.PathValue("token"))
	if err != nil {
		s.fail(w, err)
		return
	}
	raw, ipErr := s.visitor(r)
	ip := ""
	if ipErr == nil {
		var a netip.Addr
		a, ipErr = service.PublicIPv4(raw)
		if ipErr == nil {
			ip = a.String()
		}
	}
	message := ""
	if ipErr != nil {
		message = ipErr.Error()
	}
	fallback := ""
	if s.svc.Config.Server.IPv4PublicURL != "" {
		fallback = s.svc.Config.Server.IPv4PublicURL + "/i/" + url.PathEscape(r.PathValue("token"))
	}
	write(w, 200, map[string]any{"name": list.Name, "description": list.Description, "link_expires_at": l.ExpiresAt, "access_seconds": l.AccessSeconds, "ipv4": ip, "ip_error": message, "ipv4_url": fallback})
}
func (s *Server) authorize(w http.ResponseWriter, r *http.Request) {
	var input struct{}
	if !body(w, r, &input) {
		return
	}
	ip, err := s.visitor(r)
	if err != nil {
		problem(w, 422, "ip_unavailable", err.Error())
		return
	}
	v, err := service.PublicIPv4(ip)
	if err != nil {
		problem(w, 422, "ip_unavailable", err.Error())
		return
	}
	g, err := s.svc.Authorize(r.Context(), r.PathValue("token"), v.String())
	if err != nil {
		s.fail(w, err)
		return
	}
	write(w, 200, map[string]any{"status": "authorized", "ip": g.IP, "expires_at": g.ExpiresAt, "effective_expires_at": g.EffectiveExpiresAt})
}
func (s *Server) bootstrap(w http.ResponseWriter, r *http.Request) {
	c := s.svc.Config
	lists, err := s.svc.DB.Lists(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	write(w, 200, map[string]any{"lists": lists, "statuses": s.svc.Statuses(), "defaults": map[string]any{"link_validity_seconds": int64(c.Defaults.LinkValidity.Seconds()), "access_seconds": int64(c.Defaults.AccessDuration.Seconds()), "max_ips": c.Defaults.MaxIPs}})
}
func pagination(w http.ResponseWriter, r *http.Request) (string, int, int, bool) {
	status := r.URL.Query().Get("status")
	if status == "" {
		status = "active"
	}
	switch status {
	case "active", "expired", "revoked", "all":
	default:
		problem(w, 400, "invalid_filter", "Invalid status filter.")
		return "", 0, 0, false
	}
	page, size := 1, 25
	for name, target := range map[string]*int{"page": &page, "page_size": &size} {
		if v := r.URL.Query().Get(name); v != "" {
			n, e := strconv.Atoi(v)
			if e != nil || n < 1 || n > 1000000 {
				problem(w, 400, "invalid_page", "Invalid page.")
				return "", 0, 0, false
			}
			*target = n
		}
	}
	if size > 100 {
		size = 100
	}
	return status, page, size, true
}
func (s *Server) links(w http.ResponseWriter, r *http.Request) {
	status, page, size, ok := pagination(w, r)
	if !ok {
		return
	}
	p, err := s.svc.DB.Links(r.Context(), status, page, size, s.svc.Now())
	if err != nil {
		s.fail(w, err)
		return
	}
	write(w, 200, p)
}
func (s *Server) grants(w http.ResponseWriter, r *http.Request) {
	status, page, size, ok := pagination(w, r)
	if !ok {
		return
	}
	p, err := s.svc.DB.Grants(r.Context(), status, page, size, s.svc.Now())
	if err != nil {
		s.fail(w, err)
		return
	}
	write(w, 200, p)
}
func (s *Server) createLink(w http.ResponseWriter, r *http.Request) {
	var in struct {
		ListID        string `json:"list_id"`
		Label         string `json:"label"`
		ExpiresAt     *int64 `json:"expires_at"`
		AccessSeconds *int64 `json:"access_seconds"`
		MaxIPs        *int   `json:"max_ips"`
	}
	if !body(w, r, &in) {
		return
	}
	c := s.svc.Config
	expiry := s.svc.Now().Add(c.Defaults.LinkValidity.Duration).Unix()
	access := int64(c.Defaults.AccessDuration.Seconds())
	maxIPs := c.Defaults.MaxIPs
	if in.ExpiresAt != nil {
		expiry = *in.ExpiresAt
	}
	if in.AccessSeconds != nil {
		access = *in.AccessSeconds
	}
	if in.MaxIPs != nil {
		maxIPs = *in.MaxIPs
	}
	l, token, err := s.svc.CreateLink(r.Context(), store.Link{ListID: in.ListID, Label: in.Label, ExpiresAt: expiry, AccessSeconds: access, MaxIPs: maxIPs})
	if err != nil {
		if errors.Is(err, service.ErrInput) || errors.Is(err, service.ErrList) {
			problem(w, 400, "invalid_invite", err.Error())
		} else {
			s.fail(w, err)
		}
		return
	}
	write(w, 201, map[string]any{"link": l, "url": c.Server.PublicURL + "/i/" + token})
}
func (s *Server) revokeLink(w http.ResponseWriter, r *http.Request) {
	var in struct {
		RevokeGrants *bool `json:"revoke_grants"`
	}
	if !body(w, r, &in) {
		return
	}
	grants := true
	if in.RevokeGrants != nil {
		grants = *in.RevokeGrants
	}
	err := s.svc.RevokeLink(r.Context(), r.PathValue("id"), grants)
	s.mutationResult(w, err)
}
func (s *Server) revokeGrant(w http.ResponseWriter, r *http.Request) {
	var in struct{}
	if !body(w, r, &in) {
		return
	}
	s.mutationResult(w, s.svc.RevokeGrant(r.Context(), r.PathValue("id")))
}
func (s *Server) mutationResult(w http.ResponseWriter, err error) {
	if err != nil && !errors.Is(err, service.ErrSync) {
		s.fail(w, err)
		return
	}
	if err != nil {
		slog.Warn("change saved; synchronization pending", "error", err)
		write(w, 202, map[string]string{"status": "pending", "message": "Change saved. Firewall synchronization is pending."})
		return
	}
	write(w, 200, map[string]string{"status": "synced"})
}
func (s *Server) synchronize(w http.ResponseWriter, r *http.Request) {
	var in struct{}
	if !body(w, r, &in) {
		return
	}
	if err := s.svc.Sync(r.Context()); err != nil {
		slog.Warn("manual synchronization incomplete", "error", err)
		write(w, 202, map[string]string{"status": "pending", "message": "Firewall synchronization is pending. Check the list status for details."})
		return
	}
	s.mutationResult(w, nil)
}
func (s *Server) static(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/")
	// Match the decoded path too: encoded slashes can reach the SPA fallback
	// without matching ServeMux's segment-based /admin/ pattern.
	if (path == "admin" || strings.HasPrefix(path, "admin/")) && !s.authenticatedProxy(r) {
		problem(w, 403, "admin_protected", "Admin access requires the configured authenticated reverse proxy.")
		return
	}
	if r.Method != "GET" && r.Method != "HEAD" {
		w.Header().Set("Allow", "GET, HEAD")
		problem(w, 405, "method_not_allowed", "Method not allowed.")
		return
	}
	if strings.HasPrefix(path, "assets/") || path == "favicon.svg" {
		if !fs.ValidPath(path) {
			http.NotFound(w, r)
			return
		}
		http.FileServer(http.FS(s.assets)).ServeHTTP(w, r)
		return
	}
	if path != "" && path != "admin" && !strings.HasPrefix(path, "admin/") && !strings.HasPrefix(path, "i/") {
		http.NotFound(w, r)
		return
	}
	data, err := fs.ReadFile(s.assets, "index.html")
	if err != nil {
		http.Error(w, "Frontend unavailable; build with make build or use the Vite development server.", 503)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if r.Method != "HEAD" {
		_, _ = w.Write(data)
	}
}
