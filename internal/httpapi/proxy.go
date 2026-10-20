package httpapi

import (
	"net"
	"net/http"
	"strings"
)

// realIP resolves the client address from forwarded headers when the
// immediate peer is a trusted proxy.
func realIP(trusted []*net.IPNet) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if len(trusted) == 0 {
				next.ServeHTTP(w, r)
				return
			}
			peer := parseRemoteIP(r.RemoteAddr)
			if peer == nil || !ipInNetworks(peer, trusted) {
				next.ServeHTTP(w, r)
				return
			}
			if client := forwardedClientIP(r, trusted); client != nil {
				r.RemoteAddr = client.String()
			}
			next.ServeHTTP(w, r)
		})
	}
}

func forwardedClientIP(r *http.Request, trusted []*net.IPNet) net.IP {
	var chain []net.IP
	for _, header := range r.Header.Values("X-Forwarded-For") {
		for _, part := range strings.Split(header, ",") {
			if ip := parseForwardedIP(part); ip != nil {
				chain = append(chain, ip)
			}
		}
	}
	// Walk from the proxy side so a spoofed leftmost value is ignored.
	for i := len(chain) - 1; i >= 0; i-- {
		if !ipInNetworks(chain[i], trusted) {
			return chain[i]
		}
	}
	if len(chain) > 0 {
		return chain[0]
	}
	if ip := parseForwardedIP(r.Header.Get("X-Real-IP")); ip != nil {
		return ip
	}
	return nil
}

func parseRemoteIP(value string) net.IP {
	if host, _, err := net.SplitHostPort(value); err == nil {
		return net.ParseIP(host)
	}
	return net.ParseIP(strings.Trim(value, "[]"))
}

func parseForwardedIP(value string) net.IP {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	if host, _, err := net.SplitHostPort(value); err == nil {
		value = host
	}
	return net.ParseIP(strings.Trim(value, "[]"))
}

func ipInNetworks(ip net.IP, networks []*net.IPNet) bool {
	for _, network := range networks {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}
