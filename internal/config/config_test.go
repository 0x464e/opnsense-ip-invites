package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestYAMLDefaultsAndStrictValidation(t *testing.T) {
	original, err := os.ReadFile("../../examples/config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name, from, to string
		valid          bool
	}{
		{"example", "", "", true},
		{"custom defaults", `link_validity: "168h"`, `link_validity: "72h"`, true},
		{"missing default", "link_validity: \"168h\"", "", false},
		{"unknown key", "sync_interval:", "sync_intervall:", false},
		{"insecure public URL", "https://invites.example.com", "http://invites.example.com", false},
		{"untrusted header", "trusted_proxies: [\"127.0.0.1/32\"]", "trusted_proxies: []", false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := string(original)
			if tt.from != "" {
				s = strings.Replace(s, tt.from, tt.to, 1)
			}
			p := filepath.Join(t.TempDir(), "config.yaml")
			if err := os.WriteFile(p, []byte(s), 0600); err != nil {
				t.Fatal(err)
			}
			c, err := Load(p)
			if (err == nil) != tt.valid {
				t.Fatalf("valid=%v, got %v", tt.valid, err)
			}
			if err == nil && !filepath.IsAbs(c.Database) {
				t.Fatal("database path not resolved")
			}
			if tt.name == "custom defaults" && c.Defaults.LinkValidity.Hours() != 72 {
				t.Fatal("default did not come from YAML")
			}
		})
	}
}
func TestSecretFileAndAmbiguity(t *testing.T) {
	p := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(p, []byte("file-value\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TEST_SECRET", "")
	t.Setenv("TEST_SECRET_FILE", p)
	v, err := Secret("TEST_SECRET")
	if err != nil || v != "file-value" {
		t.Fatal(v, err)
	}
	t.Setenv("TEST_SECRET", "environment-value")
	if _, err = Secret("TEST_SECRET"); err == nil {
		t.Fatal("ambiguous secrets accepted")
	}
}
