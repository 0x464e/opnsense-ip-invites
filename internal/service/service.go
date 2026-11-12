package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/netip"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/0x464e/opnsense-ip-invites/internal/config"
	"github.com/0x464e/opnsense-ip-invites/internal/firewall"
	"github.com/0x464e/opnsense-ip-invites/internal/store"
)

var ErrSync = errors.New("authorization saved, but firewall membership could not be confirmed; retry shortly")
var ErrList = errors.New("this invite's list is no longer configured")
var ErrInput = errors.New("invalid invitation")

type Status struct {
	Alias       string `json:"alias"`
	Retired     bool   `json:"retired"`
	LastAttempt int64  `json:"last_attempt"`
	LastSuccess int64  `json:"last_success"`
	Desired     int    `json:"desired"`
	Actual      int    `json:"actual"`
	Error       string `json:"error"`
}
type Service struct {
	Config config.Config
	DB     *store.Store
	api    firewall.API
	// The single operation lock covers grant transactions AND remote updates. An
	// expiry worker cannot apply an old snapshot after a new grant is confirmed.
	mu       sync.Mutex
	statusMu sync.RWMutex
	statuses map[string]Status
	Now      func() time.Time
}

func New(ctx context.Context, c config.Config, db *store.Store, api firewall.API) (*Service, error) {
	known, err := db.Aliases(ctx)
	if err != nil {
		return nil, err
	}
	lists, err := db.Lists(ctx)
	if err != nil {
		return nil, err
	}
	statuses := map[string]Status{}
	for _, alias := range known {
		retired := true
		for _, list := range lists {
			if list.Alias == alias {
				retired = false
				break
			}
		}
		statuses[alias] = Status{Alias: alias, Retired: retired, Error: "Not yet synchronized"}
	}
	return &Service{Config: c, DB: db, api: api, statuses: statuses, Now: time.Now}, nil
}
func (s *Service) List(id string) (store.List, bool) {
	l, err := s.DB.List(context.Background(), id)
	return l, err == nil
}
func (s *Service) Preview(ctx context.Context, token string) (store.Link, store.List, error) {
	l, err := s.DB.LookupToken(ctx, token)
	if err != nil {
		return l, store.List{}, err
	}
	if err = store.ValidLink(l, s.Now()); err != nil {
		return l, store.List{}, err
	}
	list, ok := s.List(l.ListID)
	if !ok {
		return l, list, ErrList
	}
	return l, list, nil
}
func (s *Service) CreateLink(ctx context.Context, l store.Link) (store.Link, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.List(l.ListID); !ok {
		return l, "", ErrList
	}
	now := s.Now()
	l.Label = strings.TrimSpace(l.Label)
	if l.Label == "" || len(l.Label) > 100 || l.ExpiresAt <= now.Unix() || l.ExpiresAt > now.AddDate(10, 0, 0).Unix() || l.AccessSeconds < 60 || l.AccessSeconds > 315360000 || l.MaxIPs < 0 || l.MaxIPs > 100000 {
		return l, "", fmt.Errorf("%w: provide a label (1–100 bytes), future expiry within 10 years, access duration from 1 minute to 10 years, and an IP limit from 0 to 100000", ErrInput)
	}
	return s.DB.CreateLink(ctx, l, now)
}
func (s *Service) Authorize(ctx context.Context, token, ip string) (store.Grant, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	parsed, err := PublicIPv4(ip)
	if err != nil {
		return store.Grant{}, err
	}
	ip = parsed.String()
	l, list, err := s.Preview(ctx, token)
	if err != nil {
		return store.Grant{}, err
	}
	g, err := s.DB.Authorize(ctx, l.ID, ip, s.Now())
	if err != nil {
		return g, err
	}
	if err = s.syncLocked(ctx, list.Alias); err != nil {
		return g, ErrSync
	}
	return g, nil
}
func (s *Service) RevokeLink(ctx context.Context, id string, grants bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.DB.RevokeLink(ctx, id, grants, s.Now()); err != nil {
		return err
	}
	if err := s.syncLocked(ctx, ""); err != nil {
		return ErrSync
	}
	return nil
}
func (s *Service) RevokeGrant(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.DB.RevokeGrant(ctx, id, s.Now()); err != nil {
		return err
	}
	if err := s.syncLocked(ctx, ""); err != nil {
		return ErrSync
	}
	return nil
}
func (s *Service) ExtendGrant(ctx context.Context, id string, expiresAt int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.Now()
	if expiresAt <= now.Unix() || expiresAt > now.AddDate(10, 0, 0).Unix() {
		return fmt.Errorf("%w: choose a future expiry within 10 years", ErrInput)
	}
	alias, err := s.DB.ExtendGrant(ctx, id, expiresAt, now)
	if err != nil {
		return err
	}
	if err := s.syncLocked(ctx, alias); err != nil {
		return ErrSync
	}
	return nil
}

func (s *Service) Sync(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.syncLocked(ctx, "")
}
func (s *Service) Statuses() []Status {
	s.statusMu.RLock()
	defer s.statusMu.RUnlock()
	result := []Status{}
	for _, v := range s.statuses {
		result = append(result, v)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Alias < result[j].Alias })
	return result
}
func (s *Service) Ready() bool {
	statuses := s.Statuses()
	for _, v := range statuses {
		if v.Error != "" || v.LastSuccess == 0 {
			return false
		}
	}
	return true
}
func (s *Service) setStatus(v Status) {
	s.statusMu.Lock()
	s.statuses[v.Alias] = v
	s.statusMu.Unlock()
}
func (s *Service) previous(alias string) Status {
	s.statusMu.RLock()
	defer s.statusMu.RUnlock()
	return s.statuses[alias]
}

func (s *Service) syncLocked(ctx context.Context, only string) error {
	aliases, err := s.DB.Aliases(ctx)
	if err != nil {
		return err
	}
	lists, err := s.DB.Lists(ctx)
	if err != nil {
		return err
	}
	if len(aliases) == 0 {
		return nil
	}
	definitions, defErr := s.api.Aliases(ctx)
	active, activeErr := s.api.ActiveAliases(ctx)
	var failures []error
	for _, alias := range aliases {
		if only != "" && alias != only {
			continue
		}
		status := s.previous(alias)
		status.Alias = alias
		status.LastAttempt = s.Now().Unix()
		status.Error = ""
		status.Retired = true
		desired := map[string]bool{}
		var localErr error
		for _, l := range lists {
			if l.Alias == alias {
				status.Retired = false
				desired, localErr = s.DB.Desired(ctx, l.ID, s.Now())
				break
			}
		}
		status.Desired = len(desired)
		if localErr != nil {
			status.Error = "could not read desired membership"
		} else if defErr != nil {
			status.Error = defErr.Error()
		} else {
			definition, exists := definitions[alias]
			if !exists && status.Retired {
				status.Actual = 0
				status.LastSuccess = s.Now().Unix()
				s.setStatus(status)
				continue
			}
			if !exists && !status.Retired {
				if e := s.api.CreateAlias(ctx, alias); e != nil {
					status.Error = e.Error()
				} else {
					refreshed, e := s.api.Aliases(ctx)
					if e != nil {
						status.Error = e.Error()
					} else {
						definition, exists = refreshed[alias]
					}
				}
			}
			if status.Error != "" {
				// Leave a failed creation pending; retry on the next sync.
			} else if !exists || definition.Type != "external" || definition.Enabled != "1" {
				status.Error = "alias must exist, be enabled, and have type External"
			} else if activeErr != nil {
				status.Error = activeErr.Error()
			} else if !active[alias] && !status.Retired {
				status.Error = "Alias exists but its PF table is not active; reference it in an enabled firewall policy and apply changes in OPNsense"
			} else {
				actual, e := s.api.List(ctx, alias)
				if e == nil {
					// Add before deleting, so replacing an address doesn't introduce a needless gap.
					for _, ip := range sorted(desired) {
						if !actual[ip] {
							if e = s.api.Add(ctx, alias, ip); e != nil {
								break
							}
						}
					}
					if e == nil {
						for _, ip := range sorted(actual) {
							if !desired[ip] {
								if e = s.api.Delete(ctx, alias, ip); e != nil {
									break
								}
							}
						}
					}
				}
				if e == nil {
					actual, e = s.api.List(ctx, alias)
					if e == nil && !same(actual, desired) {
						e = errors.New("firewall membership differs from desired membership after update")
					}
				}
				status.Actual = len(actual)
				if e != nil {
					status.Error = e.Error()
				} else {
					status.LastSuccess = s.Now().Unix()
				}
			}
		}
		s.setStatus(status)
		if status.Error != "" {
			failures = append(failures, fmt.Errorf("alias %s: %s", alias, status.Error))
		}
	}
	return errors.Join(failures...)
}
func sorted(m map[string]bool) []string {
	keys := []string{}
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
func same(a, b map[string]bool) bool {
	if len(a) != len(b) {
		return false
	}
	for k := range a {
		if !b[k] {
			return false
		}
	}
	return true
}
func (s *Service) Run(ctx context.Context) {
	ticker := time.NewTicker(s.Config.SyncInterval.Duration)
	defer ticker.Stop()
	for {
		if err := s.Sync(ctx); err != nil && ctx.Err() == nil {
			slog.Warn("firewall reconciliation incomplete", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func PublicIPv4(raw string) (netip.Addr, error) {
	ip, err := netip.ParseAddr(raw)
	if err != nil {
		return netip.Addr{}, errors.New("could not detect a valid visitor address")
	}
	ip = ip.Unmap()
	if !ip.Is4() {
		return netip.Addr{}, errors.New("this invitation requires IPv4; this request arrived over IPv6")
	}
	if !ip.IsGlobalUnicast() || ip.IsPrivate() {
		return netip.Addr{}, errors.New("a public IPv4 address is required")
	}
	for _, p := range []string{"0.0.0.0/8", "100.64.0.0/10", "192.0.0.0/24", "198.18.0.0/15", "240.0.0.0/4"} {
		if netip.MustParsePrefix(p).Contains(ip) {
			return netip.Addr{}, errors.New("a public IPv4 address is required; synthetic and carrier-internal addresses are not accepted")
		}
	}
	return ip, nil
}
