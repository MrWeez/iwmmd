// Package whois queries port 43 registries when RDAP is unavailable.
package whois

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/mrweez/iwmmd/internal/checker/model"
)

// Client performs WHOIS lookups and caches registry referrals.
type Client struct {
	timeout time.Duration

	mu      sync.Mutex
	servers map[string]string
	dial    func(ctx context.Context, network, address string) (net.Conn, error)
}

// New creates a WHOIS client.
func New() *Client {
	return &Client{
		timeout: 12 * time.Second,
		servers: map[string]string{},
		dial:    (&net.Dialer{Timeout: 10 * time.Second}).DialContext,
	}
}

// SetDialer replaces the network dialer, primarily for tests.
func (c *Client) SetDialer(dial func(ctx context.Context, network, address string) (net.Conn, error)) {
	c.dial = dial
}

// SetServer overrides or seeds a TLD referral.
func (c *Client) SetServer(tld, server string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.servers == nil {
		c.servers = map[string]string{}
	}
	c.servers[strings.ToLower(tld)] = server
}

// Check looks up a domain via WHOIS.
func (c *Client) Check(ctx context.Context, name string) model.Result {
	started := time.Now()
	result := model.Result{Source: "whois", Status: "error", Confidence: "unknown"}
	tld := name
	if idx := strings.LastIndex(name, "."); idx >= 0 {
		tld = name[idx+1:]
	}
	server, err := c.server(ctx, tld)
	if err != nil {
		result.ErrorCode = "referral"
		result.ErrorMessage = err.Error()
		result.Duration = time.Since(started)
		return result
	}
	if server == "" {
		result.Status = "unsupported"
		result.ErrorCode = "no_whois"
		result.ErrorMessage = "no WHOIS server is published for ." + tld
		result.Duration = time.Since(started)
		return result
	}
	body, err := c.query(ctx, server, name)
	result.Raw = body
	result.Duration = time.Since(started)
	if err != nil {
		result.ErrorCode = "network"
		result.ErrorMessage = err.Error()
		return result
	}
	parsed := Parse(string(body))
	result.Registrar = parsed.Registrar
	result.ExpiresAt = parsed.ExpiresAt
	if parsed.ExpiresAt == nil {
		result.ErrorCode = "expiry_not_found"
		result.ErrorMessage = "WHOIS response did not contain a recognizable expiry date"
		return result
	}
	result.Status = "ok"
	result.Confidence = "low"
	return result
}

// Parsed is the subset of WHOIS data IWMMD uses.
type Parsed struct {
	ExpiresAt *time.Time
	Registrar string
}

// Parse extracts expiry and registrar fields from a WHOIS response.
func Parse(body string) Parsed {
	var parsed Parsed
	for _, rawLine := range strings.Split(body, "\n") {
		line := strings.TrimSpace(strings.TrimSuffix(rawLine, "\r"))
		if line == "" || strings.HasPrefix(line, "%") || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if parsed.ExpiresAt == nil && isExpiryKey(key) {
			if t, err := parseWhoisDate(value); err == nil {
				parsed.ExpiresAt = &t
			}
		}
		if parsed.Registrar == "" && (key == "registrar" || key == "sponsoring registrar") {
			parsed.Registrar = value
		}
	}
	return parsed
}

func (c *Client) server(ctx context.Context, tld string) (string, error) {
	tld = strings.ToLower(tld)
	c.mu.Lock()
	server, ok := c.servers[tld]
	c.mu.Unlock()
	if ok {
		return server, nil
	}
	body, err := c.query(ctx, "whois.iana.org", tld)
	if err != nil {
		return "", fmt.Errorf("query IANA referral: %w", err)
	}
	server = parseReferral(string(body))
	c.mu.Lock()
	if c.servers == nil {
		c.servers = map[string]string{}
	}
	c.servers[tld] = server
	c.mu.Unlock()
	return server, nil
}

func parseReferral(body string) string {
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		lower := strings.ToLower(line)
		if strings.HasPrefix(lower, "refer:") || strings.HasPrefix(lower, "whois:") {
			_, value, _ := strings.Cut(line, ":")
			value = strings.TrimSpace(value)
			value = strings.TrimPrefix(value, "whois://")
			if value != "" {
				return value
			}
		}
	}
	return ""
}

func (c *Client) query(ctx context.Context, server, query string) ([]byte, error) {
	address := server
	if _, _, err := net.SplitHostPort(address); err != nil {
		address = net.JoinHostPort(address, "43")
	}
	conn, err := c.dial(ctx, "tcp", address)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	deadline := time.Now().Add(c.timeout)
	if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(deadline) {
		deadline = ctxDeadline
	}
	_ = conn.SetDeadline(deadline)
	if _, err := io.WriteString(conn, query+"\r\n"); err != nil {
		return nil, err
	}
	body, err := io.ReadAll(io.LimitReader(conn, 1<<20))
	if err != nil {
		return body, err
	}
	return body, nil
}

func isExpiryKey(key string) bool {
	if strings.Contains(key, "updated") || strings.Contains(key, "created") || strings.Contains(key, "registration") {
		return false
	}
	for _, candidate := range []string{
		"registry expiry date", "expiry date", "expiration date", "expiration time",
		"expires", "expire", "paid-till", "renewal date", "valid until",
	} {
		if key == candidate || strings.Contains(key, candidate) {
			return true
		}
	}
	return false
}

func parseWhoisDate(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if idx := strings.Index(value, " ("); idx > 0 {
		value = strings.TrimSpace(value[:idx])
	}
	value = strings.TrimSuffix(value, " UTC")
	value = strings.TrimSpace(value)
	layouts := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05.0Z",
		"2006-01-02T15:04:05Z",
		"2006-01-02 15:04:05",
		"2006-01-02",
		"2006.01.02",
		"2006/01/02",
		"02-Jan-2006",
		"02.01.2006",
		"January 2 2006",
		"Jan 2 2006",
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, value); err == nil {
			return t.UTC(), nil
		}
	}
	return time.Time{}, errors.New("unrecognized date")
}
