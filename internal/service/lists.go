package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"

	"github.com/0x464e/opnsense-ip-invites/internal/store"
)

var aliasPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{0,31}$`)
var idPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)

func (s *Service) CreateList(ctx context.Context, l store.List) (store.List, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return l, err
	}
	l.ID = "list-" + hex.EncodeToString(b[:])
	l.Name = strings.TrimSpace(l.Name)
	l.Description = strings.TrimSpace(l.Description)
	l.Alias = strings.TrimSpace(l.Alias)
	if l.Alias == "" {
		l.Alias = "INVITES_" + strings.ToUpper(hex.EncodeToString(b[:]))
	}
	if !validList(l) {
		return l, fmt.Errorf("%w: provide a name (1–100 bytes), description up to 500 bytes, and a valid alias (letter followed by letters, digits or underscores; at most 32 characters)", ErrInput)
	}
	// Existing configured aliases must be suitable before a list can claim them.
	definitions, err := s.api.Aliases(ctx)
	if err != nil {
		return l, err
	}
	if a, ok := definitions[l.Alias]; ok && (a.Type != "external" || a.Enabled != "1") {
		return l, fmt.Errorf("%w: an existing alias must be enabled and have type External", ErrInput)
	}
	if _, exists := definitions[l.Alias]; exists {
		addresses, err := s.api.List(ctx, l.Alias)
		if err != nil {
			return l, err
		}
		if len(addresses) > 0 {
			return l, fmt.Errorf("%w: an existing alias must be empty before assigning it to this app", ErrInput)
		}
	}
	l, err = s.DB.CreateList(ctx, l)
	if err != nil {
		return l, err
	}
	if err = s.syncLocked(ctx, l.Alias); err != nil {
		return l, ErrSync
	}
	return l, nil
}
func validList(l store.List) bool {
	return idPattern.MatchString(l.ID) && aliasPattern.MatchString(l.Alias) && l.Name != "" && len(l.Name) <= 100 && len(l.Description) <= 500
}
func (s *Service) UpdateList(ctx context.Context, l store.List) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	old, err := s.DB.List(ctx, l.ID)
	if err != nil {
		return err
	}
	if l.Alias != "" && l.Alias != old.Alias {
		return fmt.Errorf("%w: a list's alias cannot be changed", ErrInput)
	}
	l.Alias = old.Alias
	l.Name = strings.TrimSpace(l.Name)
	l.Description = strings.TrimSpace(l.Description)
	if !validList(l) {
		return fmt.Errorf("%w: invalid list name or description", ErrInput)
	}
	return s.DB.UpdateList(ctx, l)
}
func (s *Service) DeleteList(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.DB.DeleteList(ctx, id, s.Now()); err != nil {
		return err
	}
	if err := s.syncLocked(ctx, ""); err != nil {
		return ErrSync
	}
	return nil
}
