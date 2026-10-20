package httpapi

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mrweez/iwmmd/internal/config"
	"github.com/mrweez/iwmmd/internal/demo"
	"github.com/mrweez/iwmmd/internal/platform/metrics"
	"github.com/mrweez/iwmmd/internal/store"
)

func TestDemoModeAnonymousAccess(t *testing.T) {
	cfg := config.Default()
	cfg.DataDir = t.TempDir()
	cfg.Demo.Enabled = true
	st, err := store.Open(store.Options{DataDir: cfg.DataDir})
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err := st.Migrate(); err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	state := demo.NewState("member", 30*time.Minute)
	router := NewRouter(Deps{
		Config: &cfg, Store: st, Metrics: metrics.New("test", "test"), Logger: logger, Demo: state,
	})

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
	if meta["demo_mode"] != true || meta["auth_configured"] != true {
		t.Fatalf("meta = %v", meta)
	}
	if meta["demo_reset_at"] == nil || meta["demo_reset_at"] == "" {
		t.Fatalf("demo_reset_at missing: %v", meta)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("me status = %d body=%s", rec.Code, rec.Body.String())
	}
	var me struct {
		User struct {
			Email string `json:"email"`
			Role  string `json:"role"`
		} `json:"user"`
		CSRF string `json:"csrf_token"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &me); err != nil {
		t.Fatal(err)
	}
	if me.User.Email != demo.UserEmail || me.User.Role != "member" || me.CSRF == "" {
		t.Fatalf("me = %#v", me)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/stats", nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("stats status = %d body=%s", rec.Code, rec.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("users status = %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("logout without csrf status = %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	req.Header.Set("X-CSRF-Token", state.CSRF)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("logout status = %d", rec.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/v1/auth/login?next=/domains", nil)
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/domains" {
		t.Fatalf("login status = %d location=%q", rec.Code, rec.Header().Get("Location"))
	}
}

func TestDemoModeSuppressesSMTPTest(t *testing.T) {
	cfg := config.Default()
	cfg.DataDir = t.TempDir()
	cfg.Demo.Enabled = true
	st, err := store.Open(store.Options{DataDir: cfg.DataDir})
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err := st.Migrate(); err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	state := demo.NewState("admin", 30*time.Minute)
	router := NewRouter(Deps{
		Config: &cfg, Store: st, Metrics: metrics.New("test", "test"), Logger: logger, Demo: state,
	})

	req := httptest.NewRequest(http.MethodPost, "/api/v1/settings/smtp/test", strings.NewReader(`{"to":[]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CSRF-Token", state.CSRF)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("smtp test status = %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "sent") {
		t.Fatalf("body = %s", rec.Body.String())
	}
}
