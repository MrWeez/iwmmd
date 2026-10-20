// Package rdap queries RDAP directory services for domain expiry data.
package rdap

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"

	"github.com/mrweez/iwmmd/internal/checker/model"
)

// Client resolves RDAP base URLs from the IANA bootstrap registry.
type Client struct {
	http         *http.Client
	dataDir      string
	bootstrapURL string

	mu       sync.Mutex
	bases    map[string]string
	loadedAt time.Time

	limiterMu sync.Mutex
	limiters  map[string]*rate.Limiter
	newLimit  func() *rate.Limiter
}

// New creates an RDAP client. dataDir is used for the bootstrap cache.
func New(dataDir string) *Client {
	c := &Client{
		http: &http.Client{
			Timeout: 20 * time.Second,
			Transport: &http.Transport{
				Proxy:                 http.ProxyFromEnvironment,
				MaxIdleConns:          20,
				IdleConnTimeout:       60 * time.Second,
				TLSHandshakeTimeout:   10 * time.Second,
				ResponseHeaderTimeout: 15 * time.Second,
			},
		},
		dataDir:      dataDir,
		bootstrapURL: "https://data.iana.org/rdap/dns.json",
		bases:        map[string]string{},
		limiters:     map[string]*rate.Limiter{},
		newLimit:     func() *rate.Limiter { return rate.NewLimiter(rate.Every(time.Second), 2) },
	}
	return c
}

// SetHTTPClient replaces the HTTP client, primarily for tests.
func (c *Client) SetHTTPClient(client *http.Client) { c.http = client }

// SetBootstrapURL replaces the IANA bootstrap URL, primarily for tests.
func (c *Client) SetBootstrapURL(url string) { c.bootstrapURL = url }

// SetRateLimiterFactory replaces the per-registry limiter, primarily for tests.
func (c *Client) SetRateLimiterFactory(factory func() *rate.Limiter) { c.newLimit = factory }

// Check looks up a domain.
func (c *Client) Check(ctx context.Context, name string) model.Result {
	started := time.Now()
	result := model.Result{Source: "rdap", Status: "error", Confidence: "unknown"}
	tld := name
	if idx := strings.LastIndex(name, "."); idx >= 0 {
		tld = name[idx+1:]
	}
	base, err := c.baseURL(ctx, tld)
	if err != nil {
		result.ErrorCode = "bootstrap"
		result.ErrorMessage = err.Error()
		result.Duration = time.Since(started)
		return result
	}
	if base == "" {
		result.Status = "unsupported"
		result.ErrorCode = "no_rdap"
		result.ErrorMessage = "no RDAP service is published for ." + tld
		result.Duration = time.Since(started)
		return result
	}
	if err := c.limiter(base).Wait(ctx); err != nil {
		result.ErrorCode = "canceled"
		result.ErrorMessage = err.Error()
		result.Duration = time.Since(started)
		return result
	}

	target := strings.TrimRight(base, "/") + "/domain/" + name
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		result.ErrorCode = "request"
		result.ErrorMessage = err.Error()
		result.Duration = time.Since(started)
		return result
	}
	req.Header.Set("Accept", "application/rdap+json, application/json")
	req.Header.Set("User-Agent", "iwmmd/1 (+https://github.com/mrweez/iwmmd)")
	resp, err := c.http.Do(req)
	result.Duration = time.Since(started)
	if err != nil {
		result.ErrorCode = "network"
		result.ErrorMessage = err.Error()
		return result
	}
	defer resp.Body.Close()
	status := resp.StatusCode
	result.HTTPStatus = &status
	result.ContentType = resp.Header.Get("Content-Type")
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		result.ErrorCode = "read"
		result.ErrorMessage = err.Error()
		return result
	}
	result.Raw = body
	if resp.StatusCode == http.StatusNotFound {
		result.Status = "unsupported"
		result.ErrorCode = "not_found"
		result.ErrorMessage = "registry returned 404 for this domain"
		return result
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		result.ErrorCode = "rate_limited"
		result.ErrorMessage = "registry rate limit reached"
		return result
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		result.ErrorCode = fmt.Sprintf("http_%d", resp.StatusCode)
		result.ErrorMessage = "registry returned HTTP " + resp.Status
		return result
	}
	parsed, err := Parse(body)
	if err != nil {
		result.ErrorCode = "parse"
		result.ErrorMessage = err.Error()
		return result
	}
	result.ExpiresAt = parsed.ExpiresAt
	result.Registrar = parsed.Registrar
	if parsed.ExpiresAt == nil {
		result.ErrorCode = "expiry_not_found"
		result.ErrorMessage = "RDAP response did not contain an expiration event"
		return result
	}
	result.Status = "ok"
	result.Confidence = "high"
	result.Duration = time.Since(started)
	return result
}

// Parsed is the subset of RDAP data IWMMD uses.
type Parsed struct {
	ExpiresAt   *time.Time
	Registrar   string
	Nameservers []string
}

// Parse extracts expiration and registrar data from an RDAP response.
func Parse(body []byte) (Parsed, error) {
	var doc struct {
		Events []struct {
			Action string `json:"eventAction"`
			Date   string `json:"eventDate"`
		} `json:"events"`
		Entities []struct {
			Roles      []string          `json:"roles"`
			VCardArray []json.RawMessage `json:"vcardArray"`
		} `json:"entities"`
		Nameservers []struct {
			LDHName string `json:"ldhName"`
		} `json:"nameservers"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return Parsed{}, fmt.Errorf("decode RDAP: %w", err)
	}
	var parsed Parsed
	for _, event := range doc.Events {
		if !strings.EqualFold(event.Action, "expiration") {
			continue
		}
		if t, err := parseDate(event.Date); err == nil {
			parsed.ExpiresAt = &t
			break
		}
	}
	for _, entity := range doc.Entities {
		if !hasRole(entity.Roles, "registrar") {
			continue
		}
		parsed.Registrar = vcardName(entity.VCardArray)
		if parsed.Registrar != "" {
			break
		}
	}
	for _, ns := range doc.Nameservers {
		if ns.LDHName != "" {
			parsed.Nameservers = append(parsed.Nameservers, ns.LDHName)
		}
	}
	return parsed, nil
}

func (c *Client) baseURL(ctx context.Context, tld string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.bases == nil {
		c.bases = map[string]string{}
	}
	if len(c.bases) == 0 {
		c.loadCacheLocked()
	}
	if len(c.bases) == 0 || time.Since(c.loadedAt) > 24*time.Hour {
		if err := c.refreshLocked(ctx); err != nil && len(c.bases) == 0 {
			return "", err
		}
	}
	return strings.ToLower(c.bases[strings.ToLower(tld)]), nil
}

func (c *Client) loadCacheLocked() {
	if c.dataDir == "" {
		return
	}
	path := filepath.Join(c.dataDir, "rdap-bootstrap.json")
	info, err := os.Stat(path)
	if err != nil {
		return
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return
	}
	bases, err := parseBootstrap(body)
	if err != nil {
		return
	}
	c.bases = bases
	c.loadedAt = info.ModTime()
}

func (c *Client) refreshLocked(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.bootstrapURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "iwmmd/1 (+https://github.com/mrweez/iwmmd)")
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("fetch RDAP bootstrap: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("fetch RDAP bootstrap: HTTP %s", resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return fmt.Errorf("read RDAP bootstrap: %w", err)
	}
	bases, err := parseBootstrap(body)
	if err != nil {
		return err
	}
	c.bases = bases
	c.loadedAt = time.Now()
	if c.dataDir != "" {
		path := filepath.Join(c.dataDir, "rdap-bootstrap.json")
		tmp := path + ".tmp"
		if err := os.WriteFile(tmp, body, 0o600); err == nil {
			_ = os.Rename(tmp, path)
		}
	}
	return nil
}

func parseBootstrap(body []byte) (map[string]string, error) {
	var doc struct {
		Services [][]json.RawMessage `json:"services"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("decode RDAP bootstrap: %w", err)
	}
	bases := make(map[string]string)
	for _, service := range doc.Services {
		if len(service) < 2 {
			continue
		}
		var tlds []string
		var urls []string
		if err := json.Unmarshal(service[0], &tlds); err != nil {
			continue
		}
		if err := json.Unmarshal(service[1], &urls); err != nil || len(urls) == 0 {
			continue
		}
		for _, tld := range tlds {
			bases[strings.ToLower(tld)] = urls[0]
		}
	}
	if len(bases) == 0 {
		return nil, errors.New("RDAP bootstrap contained no services")
	}
	return bases, nil
}

func (c *Client) limiter(base string) *rate.Limiter {
	c.limiterMu.Lock()
	defer c.limiterMu.Unlock()
	if l, ok := c.limiters[base]; ok {
		return l
	}
	l := c.newLimit()
	c.limiters[base] = l
	return l
}

func parseDate(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02", "2006-01-02T15:04:05"} {
		if t, err := time.Parse(layout, value); err == nil {
			return t.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("unsupported date %q", value)
}

func hasRole(roles []string, want string) bool {
	for _, role := range roles {
		if strings.EqualFold(role, want) {
			return true
		}
	}
	return false
}

func vcardName(raw []json.RawMessage) string {
	if len(raw) < 2 {
		return ""
	}
	var entries []json.RawMessage
	if err := json.Unmarshal(raw[1], &entries); err != nil {
		return ""
	}
	for _, entryRaw := range entries {
		var entry []json.RawMessage
		if err := json.Unmarshal(entryRaw, &entry); err != nil || len(entry) < 4 {
			continue
		}
		var key string
		if err := json.Unmarshal(entry[0], &key); err != nil || !strings.EqualFold(key, "fn") {
			continue
		}
		var value string
		if err := json.Unmarshal(entry[3], &value); err == nil {
			return value
		}
	}
	return ""
}
