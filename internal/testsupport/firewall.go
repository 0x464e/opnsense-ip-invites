// Package testsupport contains in-memory firewall fixtures, never a live connection.
package testsupport

import (
	"context"
	"errors"
	"sync"

	"github.com/0x464e/opnsense-ip-invites/internal/firewall"
)

type Firewall struct {
	Mu              sync.Mutex
	Tables          map[string]map[string]bool
	Definitions     map[string]firewall.Alias
	Offline         bool
	IgnoreMutations bool
}

func NewFirewall(names ...string) *Firewall {
	f := &Firewall{Tables: map[string]map[string]bool{}, Definitions: map[string]firewall.Alias{}}
	for _, name := range names {
		f.Tables[name] = map[string]bool{}
		f.Definitions[name] = firewall.Alias{Name: name, Type: "external", Enabled: "1"}
	}
	return f
}
func (f *Firewall) Aliases(context.Context) (map[string]firewall.Alias, error) {
	f.Mu.Lock()
	defer f.Mu.Unlock()
	if f.Offline {
		return nil, errors.New("firewall offline")
	}
	r := map[string]firewall.Alias{}
	for k, v := range f.Definitions {
		r[k] = v
	}
	return r, nil
}
func (f *Firewall) List(_ context.Context, alias string) (map[string]bool, error) {
	f.Mu.Lock()
	defer f.Mu.Unlock()
	if f.Offline {
		return nil, errors.New("firewall offline")
	}
	r := map[string]bool{}
	for ip := range f.Tables[alias] {
		r[ip] = true
	}
	return r, nil
}
func (f *Firewall) Add(_ context.Context, alias, ip string) error {
	f.Mu.Lock()
	defer f.Mu.Unlock()
	if f.Offline {
		return errors.New("firewall offline")
	}
	if !f.IgnoreMutations {
		f.Tables[alias][ip] = true
	}
	return nil
}
func (f *Firewall) Delete(_ context.Context, alias, ip string) error {
	f.Mu.Lock()
	defer f.Mu.Unlock()
	if f.Offline {
		return errors.New("firewall offline")
	}
	if !f.IgnoreMutations {
		delete(f.Tables[alias], ip)
	}
	return nil
}

func (f *Firewall) CreateAlias(_ context.Context, name string) error {
	f.Mu.Lock()
	defer f.Mu.Unlock()
	if f.Offline {
		return errors.New("firewall offline")
	}
	if _, ok := f.Definitions[name]; ok {
		return errors.New("alias already exists")
	}
	if !f.IgnoreMutations {
		f.Definitions[name] = firewall.Alias{Name: name, Type: "external", Enabled: "1"}
	}
	return nil
}
func (f *Firewall) ActiveAliases(context.Context) (map[string]bool, error) {
	f.Mu.Lock()
	defer f.Mu.Unlock()
	if f.Offline {
		return nil, errors.New("firewall offline")
	}
	result := map[string]bool{}
	for name := range f.Tables {
		result[name] = true
	}
	return result, nil
}
