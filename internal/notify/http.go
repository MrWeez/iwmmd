// Package notify delivers notifications to configured channels.
package notify

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// SafeHTTPClient returns a client that checks the resolved address at dial
// time to prevent SSRF and DNS rebinding.
func SafeHTTPClient(allowPrivate bool, timeout time.Duration) *http.Client {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	transport := &http.Transport{
		Proxy:                 nil,
		MaxIdleConns:          10,
		IdleConnTimeout:       30 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: timeout,
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(address)
			if err != nil {
				return nil, err
			}
			ips, err := resolveAllowed(ctx, host, allowPrivate)
			if err != nil {
				return nil, err
			}
			var lastErr error
			for _, ip := range ips {
				conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
				if err == nil {
					return conn, nil
				}
				lastErr = err
			}
			if lastErr == nil {
				lastErr = errors.New("no permitted address")
			}
			return nil, lastErr
		},
	}
	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 4 {
				return errors.New("too many redirects")
			}
			return ValidateOutboundURL(req.URL, allowPrivate)
		},
	}
}

// ValidateOutboundURL rejects unsafe webhook URLs.
func ValidateOutboundURL(u *url.URL, allowPrivate bool) error {
	if u == nil {
		return errors.New("URL is required")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return errors.New("URL must use http or https")
	}
	if u.Host == "" {
		return errors.New("URL host is required")
	}
	if u.User != nil {
		return errors.New("URL must not contain credentials")
	}
	if !allowPrivate && isLocalHostname(u.Hostname()) {
		return errors.New("URL must not target a private or local host")
	}
	return nil
}

func resolveAllowed(ctx context.Context, host string, allowPrivate bool) ([]net.IP, error) {
	if ip := net.ParseIP(host); ip != nil {
		if !ipAllowed(ip, allowPrivate) {
			return nil, fmt.Errorf("address %s is not permitted", ip)
		}
		return []net.IP{ip}, nil
	}
	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("resolve host: %w", err)
	}
	var allowed []net.IP
	for _, addr := range addrs {
		if ipAllowed(addr.IP, allowPrivate) {
			allowed = append(allowed, addr.IP)
		}
	}
	if len(allowed) == 0 {
		return nil, fmt.Errorf("host %s resolves only to blocked addresses", host)
	}
	return allowed, nil
}

func ipAllowed(ip net.IP, allowPrivate bool) bool {
	if ip == nil {
		return false
	}
	if ip.Equal(net.ParseIP("169.254.169.254")) {
		return false
	}
	if ip.IsUnspecified() || ip.IsMulticast() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return false
	}
	if ip.IsLoopback() || ip.IsPrivate() {
		return allowPrivate
	}
	if ip4 := ip.To4(); ip4 != nil {
		switch {
		case ip4[0] == 100 && ip4[1] >= 64 && ip4[1] <= 127:
			return allowPrivate
		case ip4[0] == 192 && ip4[1] == 0 && ip4[2] == 0:
			return allowPrivate
		case ip4[0] == 192 && ip4[1] == 0 && ip4[2] == 2:
			return false
		case ip4[0] == 198 && (ip4[1] == 18 || ip4[1] == 19):
			return allowPrivate
		case ip4[0] == 198 && ip4[1] == 51 && ip4[2] == 100:
			return false
		case ip4[0] == 203 && ip4[1] == 0 && ip4[2] == 113:
			return false
		case ip4[0] >= 240:
			return false
		}
	}
	return ip.IsGlobalUnicast()
}

func isLocalHostname(host string) bool {
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		return !ipAllowed(ip, false)
	}
	return false
}
