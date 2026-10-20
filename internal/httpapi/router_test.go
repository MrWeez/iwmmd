package httpapi

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mrweez/iwmmd/internal/config"
	"github.com/mrweez/iwmmd/internal/platform/metrics"
	"github.com/mrweez/iwmmd/internal/store"
)

func testRouter(t *testing.T) (http.Handler, *store.Store, *config.Config) {
	t.Helper()
	cfg := config.Default()
	cfg.DataDir = t.TempDir()
	st, err := store.Open(store.Options{DataDir: cfg.DataDir})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Migrate(); err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	m := metrics.New("test", "test")
	return NewRouter(Deps{Config: &cfg, Store: st, Metrics: m, Logger: logger}), st, &cfg
}

func TestHealthAndMeta(t *testing.T) {
	router, st, _ := testRouter(t)
	defer st.Close()

	for _, path := range []string{"/healthz", "/readyz"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status = %d body=%s", path, rec.Code, rec.Body.String())
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/meta", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("meta status = %d", rec.Code)
	}
	var meta map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &meta); err != nil {
		t.Fatal(err)
	}
	if meta["auth_configured"] != false {
		t.Fatalf("auth_configured = %v", meta["auth_configured"])
	}
	if _, ok := meta["version"]; !ok {
		t.Fatalf("meta = %v", meta)
	}
}

func TestProblemDetailsAndSecurityHeaders(t *testing.T) {
	router, st, _ := testRouter(t)
	defer st.Close()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/nope", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/problem+json" {
		t.Fatalf("Content-Type = %q", got)
	}
	if got := rec.Header().Get("Content-Security-Policy"); !strings.Contains(got, "default-src 'self'") {
		t.Fatalf("CSP = %q", got)
	}
	if got := rec.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q", got)
	}
}

func TestAuthPlaceholderAndSPAFallback(t *testing.T) {
	router, st, _ := testRouter(t)
	defer st.Close()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("/me status = %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/domains", nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("SPA fallback status = %d", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); !strings.HasPrefix(got, "text/html") {
		t.Fatalf("SPA fallback Content-Type = %q", got)
	}
}
