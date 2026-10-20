package config

import (
	"bytes"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var envKeys = []string{
	"IWMMD_CONFIG",
	"IWMMD_BASE_URL",
	"IWMMD_HTTP_ADDR",
	"IWMMD_DATA_DIR",
	"IWMMD_DB_DSN",
	"IWMMD_SECRET_KEY",
	"IWMMD_SECRET_KEY_FILE",
	"IWMMD_OIDC_PROVIDER",
	"IWMMD_OIDC_ISSUER",
	"IWMMD_OIDC_CLIENT_ID",
	"IWMMD_OIDC_CLIENT_SECRET",
	"IWMMD_OIDC_CLIENT_SECRET_FILE",
	"IWMMD_OIDC_REDIRECT_URL",
	"IWMMD_OIDC_SCOPES",
	"IWMMD_OIDC_GROUPS_CLAIM",
	"IWMMD_OIDC_EMAIL_CLAIM",
	"IWMMD_OIDC_NAME_CLAIM",
	"IWMMD_OIDC_ALLOWED_GROUPS",
	"IWMMD_OIDC_ADMIN_GROUPS",
	"IWMMD_OIDC_ALLOWED_EMAIL_DOMAINS",
	"IWMMD_OIDC_ALLOWED_EMAILS",
	"IWMMD_OIDC_ADMIN_EMAILS",
	"IWMMD_OIDC_TRUST_EMAIL_CLAIM",
	"IWMMD_TRUSTED_PROXIES",
	"IWMMD_TLS_ENABLED",
	"IWMMD_TLS_CERT_FILE",
	"IWMMD_TLS_KEY_FILE",
	"IWMMD_TLS_PORT",
	"IWMMD_TLS_HTTP_REDIRECT",
	"IWMMD_TLS_HSTS_MAX_AGE",
	"IWMMD_LOG_LEVEL",
	"IWMMD_LOG_FORMAT",
	"IWMMD_METRICS_ADDR",
}

func testKey() string {
	return base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x42}, secretKeyBytes))
}

func clearEnv(t *testing.T) {
	t.Helper()
	for _, key := range envKeys {
		t.Setenv(key, "")
		os.Unsetenv(key)
	}
}

func emptyConfig(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadDefaults(t *testing.T) {
	clearEnv(t)
	t.Setenv("IWMMD_SECRET_KEY", testKey())

	cfg, err := Load(emptyConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.BaseURL != "http://localhost:8080" {
		t.Fatalf("BaseURL = %q", cfg.BaseURL)
	}
	if cfg.DataDir != defaultDataDir {
		t.Fatalf("DataDir = %q", cfg.DataDir)
	}
	if len(cfg.MasterKey) != secretKeyBytes {
		t.Fatalf("MasterKey length = %d", len(cfg.MasterKey))
	}
	if len(cfg.OIDC.Scopes) != 4 {
		t.Fatalf("scopes = %v", cfg.OIDC.Scopes)
	}
}

func TestLoadEnvironmentOverrides(t *testing.T) {
	clearEnv(t)
	t.Setenv("IWMMD_SECRET_KEY", testKey())
	t.Setenv("IWMMD_BASE_URL", "https://domains.example.com/")
	t.Setenv("IWMMD_DATA_DIR", "/srv/iwmmd/")
	t.Setenv("IWMMD_LOG_FORMAT", "text")
	t.Setenv("IWMMD_METRICS_ADDR", "127.0.0.1:9090")
	t.Setenv("IWMMD_OIDC_TRUST_EMAIL_CLAIM", "true")
	t.Setenv("IWMMD_OIDC_SCOPES", "openid, email ,groups")
	t.Setenv("IWMMD_TRUSTED_PROXIES", "10.0.0.0/8, 192.168.0.0/16")

	cfg, err := Load(emptyConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.BaseURL != "https://domains.example.com" {
		t.Fatalf("BaseURL = %q", cfg.BaseURL)
	}
	if cfg.DataDir != "/srv/iwmmd" {
		t.Fatalf("DataDir = %q", cfg.DataDir)
	}
	if cfg.Log.Format != "text" {
		t.Fatalf("Log.Format = %q", cfg.Log.Format)
	}
	if !cfg.OIDC.TrustEmailClaim {
		t.Fatal("TrustEmailClaim = false")
	}
	if got := strings.Join(cfg.OIDC.Scopes, ","); got != "openid,email,groups" {
		t.Fatalf("scopes = %q", got)
	}
	if len(cfg.Proxy.TrustedProxies) != 2 {
		t.Fatalf("trusted proxies = %v", cfg.Proxy.TrustedProxies)
	}
}

func TestLoadTOML(t *testing.T) {
	clearEnv(t)
	path := filepath.Join(t.TempDir(), "config.toml")
	content := `
base_url = "https://iwmmd.example.com"
data_dir = "/var/lib/iwmmd"
secret_key = "` + testKey() + `"
log = { level = "debug", format = "text" }
[oidc]
issuer = "https://id.example.com"
client_id = "iwmmd"
redirect_url = "https://iwmmd.example.com/auth/callback"
allowed_groups = ["domain-admins"]
admin_groups = ["domain-admins"]
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Log.Level != "debug" || cfg.Log.Format != "text" {
		t.Fatalf("log = %+v", cfg.Log)
	}
	if cfg.OIDC.Issuer != "https://id.example.com" {
		t.Fatalf("issuer = %q", cfg.OIDC.Issuer)
	}
}

func TestLoadUnknownKey(t *testing.T) {
	clearEnv(t)
	t.Setenv("IWMMD_SECRET_KEY", testKey())
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("nope = true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "unknown keys") {
		t.Fatalf("err = %v", err)
	}
}

func TestLoadMissingSecret(t *testing.T) {
	clearEnv(t)
	if _, err := Load(emptyConfig(t)); err == nil || !strings.Contains(err.Error(), "secret key is required") {
		t.Fatalf("err = %v", err)
	}
}

func TestLoadSecretFile(t *testing.T) {
	clearEnv(t)
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "key")
	if err := os.WriteFile(keyPath, []byte(testKey()+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("IWMMD_SECRET_KEY_FILE", keyPath)
	cfg, err := Load(emptyConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.MasterKey) != secretKeyBytes {
		t.Fatalf("MasterKey length = %d", len(cfg.MasterKey))
	}
}

func TestLoadInvalidEnvBool(t *testing.T) {
	clearEnv(t)
	t.Setenv("IWMMD_SECRET_KEY", testKey())
	t.Setenv("IWMMD_TLS_ENABLED", "yes-please")
	if _, err := Load(emptyConfig(t)); err == nil || !strings.Contains(err.Error(), "IWMMD_TLS_ENABLED") {
		t.Fatalf("err = %v", err)
	}
}

func TestValidateOIDCRequiresAdmin(t *testing.T) {
	clearEnv(t)
	path := filepath.Join(t.TempDir(), "config.toml")
	content := `
secret_key = "` + testKey() + `"
[oidc]
issuer = "https://id.example.com"
client_id = "iwmmd"
redirect_url = "https://iwmmd.example.com/auth/callback"
allowed_groups = ["domain-admins"]
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "admin source") {
		t.Fatalf("err = %v", err)
	}
}

func TestValidateTLSRequiresHTTPS(t *testing.T) {
	clearEnv(t)
	t.Setenv("IWMMD_SECRET_KEY", testKey())
	t.Setenv("IWMMD_TLS_ENABLED", "true")
	t.Setenv("IWMMD_TLS_CERT_FILE", "/tmp/cert.pem")
	t.Setenv("IWMMD_TLS_KEY_FILE", "/tmp/key.pem")
	if _, err := Load(emptyConfig(t)); err == nil || !strings.Contains(err.Error(), "https") {
		t.Fatalf("err = %v", err)
	}
}
