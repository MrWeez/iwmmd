package httpapi

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mrweez/iwmmd/internal/config"
)

func TestRealIP(t *testing.T) {
	trusted, err := config.ParseTrustedProxies([]string{"10.0.0.0/8", "2001:db8::/32"})
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		remote string
		xff    string
		realIP string
		want   string
	}{
		{
			name:   "single trusted proxy",
			remote: "10.1.2.3:4567",
			xff:    "203.0.113.5",
			want:   "203.0.113.5",
		},
		{
			name:   "walks past trusted proxies",
			remote: "10.1.2.3:4567",
			xff:    "198.51.100.9, 203.0.113.5, 10.2.3.4",
			want:   "203.0.113.5",
		},
		{
			name:   "ignores spoofed leftmost value",
			remote: "10.1.2.3:4567",
			xff:    "198.51.100.9, 203.0.113.5",
			want:   "203.0.113.5",
		},
		{
			name:   "untrusted peer ignores headers",
			remote: "192.0.2.10:1234",
			xff:    "203.0.113.5",
			want:   "192.0.2.10:1234",
		},
		{
			name:   "x-real-ip fallback",
			remote: "10.1.2.3:4567",
			realIP: "203.0.113.6",
			want:   "203.0.113.6",
		},
		{
			name:   "invalid x-forwarded-for falls back",
			remote: "10.1.2.3:4567",
			xff:    "not-an-ip",
			realIP: "203.0.113.6",
			want:   "203.0.113.6",
		},
		{
			name:   "forwarded address with port",
			remote: "10.1.2.3:4567",
			xff:    "203.0.113.5:5555",
			want:   "203.0.113.5",
		},
		{
			name:   "trusted IPv6 peer",
			remote: "[2001:db8::2]:443",
			xff:    "203.0.113.7",
			want:   "203.0.113.7",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := serveRealIP(trusted, tt.remote, tt.xff, tt.realIP); got != tt.want {
				t.Fatalf("remote addr = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRealIPDisabledWithoutTrustedProxies(t *testing.T) {
	const remote = "192.0.2.10:1234"
	if got := serveRealIP(nil, remote, "203.0.113.5", ""); got != remote {
		t.Fatalf("remote addr = %q, want %q", got, remote)
	}
}

func serveRealIP(trusted []*net.IPNet, remote, xff, xRealIP string) string {
	var got string
	handler := realIP(trusted)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.RemoteAddr
		w.WriteHeader(http.StatusNoContent)
	}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = remote
	if xff != "" {
		req.Header.Set("X-Forwarded-For", xff)
	}
	if xRealIP != "" {
		req.Header.Set("X-Real-IP", xRealIP)
	}
	handler.ServeHTTP(httptest.NewRecorder(), req)
	return got
}
