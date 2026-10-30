package config

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"
)

type Duration struct{ time.Duration }

func (d *Duration) UnmarshalYAML(n *yaml.Node) error {
	v, err := time.ParseDuration(n.Value)
	if err != nil {
		return fmt.Errorf("invalid duration %q (use e.g. 168h or 30m): %w", n.Value, err)
	}
	d.Duration = v
	return nil
}

type List struct {
	ID          string `yaml:"id" json:"id"`
	Name        string `yaml:"name" json:"name"`
	Description string `yaml:"description" json:"description"`
	Alias       string `yaml:"alias" json:"alias"`
}
type Config struct {
	Server struct {
		Listen         string   `yaml:"listen"`
		PublicURL      string   `yaml:"public_url"`
		IPv4PublicURL  string   `yaml:"ipv4_public_url"`
		TrustedProxies []string `yaml:"trusted_proxies"`
		ClientIPHeader string   `yaml:"client_ip_header"`
	} `yaml:"server"`
	Database string `yaml:"database"`
	Defaults struct {
		LinkValidity   Duration `yaml:"link_validity"`
		AccessDuration Duration `yaml:"access_duration"`
		MaxIPs         int      `yaml:"max_ips"`
	} `yaml:"defaults"`
	OPNsense struct {
		URL     string   `yaml:"url"`
		CAFile  string   `yaml:"ca_file"`
		Timeout Duration `yaml:"timeout"`
	} `yaml:"opnsense"`
	SyncInterval Duration `yaml:"sync_interval"`
	Lists        []List   `yaml:"lists"`
}

func Load(path string) (Config, error) {
	var c Config
	f, err := os.Open(path)
	if err != nil {
		return c, err
	}
	defer f.Close()
	d := yaml.NewDecoder(io.LimitReader(f, 1<<20))
	d.KnownFields(true)
	if err = d.Decode(&c); err != nil {
		return c, err
	}
	var extra any
	if err = d.Decode(&extra); err != io.EOF {
		return c, errors.New("configuration must contain one YAML document")
	}
	if err = c.Validate(); err != nil {
		return c, err
	}
	base, err := filepath.Abs(filepath.Dir(path))
	if err != nil {
		return c, err
	}
	if !filepath.IsAbs(c.Database) {
		c.Database = filepath.Join(base, c.Database)
	}
	if c.OPNsense.CAFile != "" && !filepath.IsAbs(c.OPNsense.CAFile) {
		c.OPNsense.CAFile = filepath.Join(base, c.OPNsense.CAFile)
	}
	return c, nil
}

func validURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Hostname() != "" && u.User == nil && u.RawQuery == "" && u.Fragment == "" && (u.Path == "" || u.Path == "/") && (u.Scheme == "https" || (u.Scheme == "http" && (u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" || u.Hostname() == "::1")))
}

func (c *Config) Validate() error {
	if _, _, err := net.SplitHostPort(c.Server.Listen); err != nil {
		return fmt.Errorf("server.listen must be host:port: %w", err)
	}
	if !validURL(c.Server.PublicURL) || (c.Server.IPv4PublicURL != "" && !validURL(c.Server.IPv4PublicURL)) {
		return errors.New("public URLs must be HTTPS origins (HTTP allowed only on loopback)")
	}
	c.Server.PublicURL = strings.TrimRight(c.Server.PublicURL, "/")
	c.Server.IPv4PublicURL = strings.TrimRight(c.Server.IPv4PublicURL, "/")
	if c.Database == "" {
		return errors.New("database is required")
	}
	if c.Defaults.LinkValidity.Duration < time.Minute || c.Defaults.AccessDuration.Duration < time.Minute || c.Defaults.MaxIPs < 0 {
		return errors.New("defaults.link_validity and defaults.access_duration must be at least 1m; max_ips must be nonnegative")
	}
	if c.Defaults.LinkValidity.Duration > 10*365*24*time.Hour || c.Defaults.AccessDuration.Duration > 10*365*24*time.Hour || c.Defaults.MaxIPs > 100000 {
		return errors.New("default durations must be within 10 years and max_ips no greater than 100000")
	}
	if c.SyncInterval.Duration < time.Second || c.OPNsense.Timeout.Duration < time.Second {
		return errors.New("sync_interval and opnsense.timeout must be at least 1s")
	}
	if !validURL(c.OPNsense.URL) {
		return errors.New("opnsense.url must be an HTTPS origin (HTTP allowed only on loopback for development)")
	}
	c.OPNsense.URL = strings.TrimRight(c.OPNsense.URL, "/")
	for _, s := range c.Server.TrustedProxies {
		if _, err := netip.ParsePrefix(s); err != nil {
			return fmt.Errorf("invalid trusted proxy CIDR %q", s)
		}
	}
	switch c.Server.ClientIPHeader {
	case "":
	case "X-Forwarded-For", "X-Real-IP", "CF-Connecting-IP":
		if len(c.Server.TrustedProxies) == 0 {
			return errors.New("a client_ip_header requires trusted_proxies")
		}
	default:
		return errors.New("client_ip_header must be X-Forwarded-For, X-Real-IP, CF-Connecting-IP, or empty")
	}
	if len(c.Lists) == 0 {
		return errors.New("configure at least one list")
	}
	ids, aliases := map[string]bool{}, map[string]bool{}
	idPattern, aliasPattern := regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`), regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{0,31}$`)
	for _, l := range c.Lists {
		if !idPattern.MatchString(l.ID) || !aliasPattern.MatchString(l.Alias) || strings.TrimSpace(l.Name) == "" || len(l.Name) > 100 || len(l.Description) > 500 {
			return fmt.Errorf("invalid list definition %q", l.ID)
		}
		if ids[l.ID] || aliases[l.Alias] {
			return errors.New("list IDs and aliases must be unique")
		}
		ids[l.ID], aliases[l.Alias] = true, true
	}
	return nil
}

// Secrets may be supplied directly in the environment or through a mounted file.
func Secret(name string) (string, error) {
	v, path := os.Getenv(name), os.Getenv(name+"_FILE")
	if v != "" && path != "" {
		return "", fmt.Errorf("set only one of %s and %s_FILE", name, name)
	}
	if path != "" {
		b, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("read %s_FILE: %w", name, err)
		}
		v = strings.TrimSpace(string(b))
	}
	if v == "" {
		return "", fmt.Errorf("%s or %s_FILE is required", name, name)
	}
	return v, nil
}
