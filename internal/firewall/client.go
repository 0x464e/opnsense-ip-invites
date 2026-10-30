package firewall

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type Alias struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	Enabled string `json:"enabled"`
}
type API interface {
	Aliases(context.Context) (map[string]Alias, error)
	List(context.Context, string) (map[string]bool, error)
	Add(context.Context, string, string) error
	Delete(context.Context, string, string) error
}
type Client struct {
	base, key, secret string
	http              *http.Client
}

func New(base, key, secret, caFile string, timeout time.Duration) (*Client, error) {
	roots, err := x509.SystemCertPool()
	if err != nil {
		roots = x509.NewCertPool()
	}
	if caFile != "" {
		pem, err := os.ReadFile(caFile)
		if err != nil {
			return nil, err
		}
		if !roots.AppendCertsFromPEM(pem) {
			return nil, errors.New("CA file contains no certificates")
		}
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots}
	return &Client{base: strings.TrimRight(base, "/"), key: key, secret: secret, http: &http.Client{Transport: transport, Timeout: timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}
func (c *Client) call(ctx context.Context, method, path string, body any, out any) error {
	var data io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		data = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+"/api/firewall/"+path, data)
	if err != nil {
		return err
	}
	req.SetBasicAuth(c.key, c.secret)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := c.http.Do(req)
	if err != nil {
		return errors.New("OPNsense connection failed; check connectivity and certificate trust")
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("OPNsense returned HTTP %d", res.StatusCode)
	}
	d := json.NewDecoder(io.LimitReader(res.Body, 4<<20))
	if err = d.Decode(out); err != nil {
		return errors.New("OPNsense returned an invalid API response")
	}
	return nil
}

// Fetch every page: silently accepting truncated tables would make reconciliation unsafe.
func (c *Client) pages(ctx context.Context, path string, consume func(json.RawMessage) error) error {
	seen := 0
	expectedTotal := -1
	for page := 1; page <= 10000; page++ {
		var res struct {
			Rows  []json.RawMessage `json:"rows"`
			Total json.RawMessage   `json:"total"`
		}
		if err := c.call(ctx, "GET", path+"?rowCount=1000&current="+strconv.Itoa(page), nil, &res); err != nil {
			return err
		}
		if res.Rows == nil {
			return errors.New("OPNsense API response is missing rows")
		}
		for _, row := range res.Rows {
			if err := consume(row); err != nil {
				return err
			}
		}
		var total int
		if len(res.Total) > 0 {
			if err := json.Unmarshal(res.Total, &total); err != nil {
				var s string
				if json.Unmarshal(res.Total, &s) != nil {
					return errors.New("invalid OPNsense pagination")
				}
				var e error
				total, e = strconv.Atoi(s)
				if e != nil {
					return errors.New("invalid OPNsense pagination")
				}
			}
		} else {
			return errors.New("OPNsense API response is missing pagination total")
		}
		if total < 0 || (expectedTotal >= 0 && total != expectedTotal) {
			return errors.New("OPNsense table changed during pagination; retry synchronization")
		}
		expectedTotal = total
		seen += len(res.Rows)
		if seen > total || (seen < total && len(res.Rows) != 1000) {
			return errors.New("incomplete OPNsense pagination")
		}
		if seen == total {
			return nil
		}
		if len(res.Rows) == 0 {
			return errors.New("incomplete OPNsense pagination")
		}
	}
	return errors.New("OPNsense table exceeds supported pagination limit")
}
func (c *Client) Aliases(ctx context.Context) (map[string]Alias, error) {
	result := map[string]Alias{}
	err := c.pages(ctx, "alias/search_item", func(raw json.RawMessage) error {
		var a Alias
		if err := json.Unmarshal(raw, &a); err != nil {
			return errors.New("invalid OPNsense alias definition")
		}
		result[a.Name] = a
		return nil
	})
	return result, err
}
func canonicalIP(raw string) (string, error) {
	if p, err := netip.ParsePrefix(raw); err == nil {
		if p.Bits() == p.Addr().BitLen() {
			return p.Addr().Unmap().String(), nil
		}
		return p.Masked().String(), nil
	}
	a, err := netip.ParseAddr(raw)
	if err != nil {
		return "", errors.New("invalid address in OPNsense alias table")
	}
	return a.Unmap().String(), nil
}
func (c *Client) List(ctx context.Context, alias string) (map[string]bool, error) {
	result := map[string]bool{}
	err := c.pages(ctx, "alias_util/list/"+url.PathEscape(alias), func(raw json.RawMessage) error {
		var row struct {
			IP string `json:"ip"`
		}
		if err := json.Unmarshal(raw, &row); err != nil {
			return err
		}
		ip, err := canonicalIP(row.IP)
		if err != nil {
			return err
		}
		result[ip] = true
		return nil
	})
	return result, err
}
func (c *Client) mutate(ctx context.Context, action, alias, ip string) error {
	var res struct {
		Status string `json:"status"`
	}
	if err := c.call(ctx, "POST", "alias_util/"+action+"/"+url.PathEscape(alias), map[string]string{"address": ip}, &res); err != nil {
		return err
	}
	if res.Status != "done" {
		return errors.New("OPNsense did not accept the alias update")
	}
	return nil
}
func (c *Client) Add(ctx context.Context, alias, ip string) error {
	return c.mutate(ctx, "add", alias, ip)
}
func (c *Client) Delete(ctx context.Context, alias, ip string) error {
	return c.mutate(ctx, "delete", alias, ip)
}
