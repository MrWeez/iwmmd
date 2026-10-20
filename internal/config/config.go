// Package config loads and validates IWMMD configuration from TOML and
// environment variables.
package config

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

const (
	defaultDataDir = "/data"
	secretKeyBytes = 32
)

// Config is the fully resolved application configuration.
type Config struct {
	BaseURL       string  `toml:"base_url"`
	HTTPAddr      string  `toml:"http_addr"`
	DataDir       string  `toml:"data_dir"`
	DBDSN         string  `toml:"db_dsn"`
	SecretKey     string  `toml:"secret_key"`
	SecretKeyFile string  `toml:"secret_key_file"`
	OIDC          OIDC    `toml:"oidc"`
	TLS           TLS     `toml:"tls"`
	Proxy         Proxy   `toml:"proxy"`
	Log           Log     `toml:"log"`
	Metrics       Metrics `toml:"metrics"`
	Demo          Demo    `toml:"demo"`

	// ConfigPath is the file that was loaded, or empty when defaults and
	// environment variables were used.
	ConfigPath string `toml:"-"`
	// MasterKey is the decoded 32-byte key used for authenticated encryption.
	MasterKey []byte `toml:"-"`
}

// OIDC contains the single sign-on provider settings.
type OIDC struct {
	Provider            string   `toml:"provider"`
	Issuer              string   `toml:"issuer"`
	ClientID            string   `toml:"client_id"`
	ClientSecret        string   `toml:"client_secret"`
	ClientSecretFile    string   `toml:"client_secret_file"`
	RedirectURL         string   `toml:"redirect_url"`
	Scopes              []string `toml:"scopes"`
	GroupsClaim         string   `toml:"groups_claim"`
	EmailClaim          string   `toml:"email_claim"`
	NameClaim           string   `toml:"name_claim"`
	AllowedGroups       []string `toml:"allowed_groups"`
	AdminGroups         []string `toml:"admin_groups"`
	AllowedEmailDomains []string `toml:"allowed_email_domains"`
	AllowedEmails       []string `toml:"allowed_emails"`
	AdminEmails         []string `toml:"admin_emails"`
	TrustEmailClaim     bool     `toml:"trust_email_claim"`
}

// TLS contains optional built-in TLS settings.
type TLS struct {
	Enabled      bool   `toml:"enabled"`
	CertFile     string `toml:"cert_file"`
	KeyFile      string `toml:"key_file"`
	Port         int    `toml:"port"`
	HTTPRedirect bool   `toml:"http_redirect"`
	HSTSMaxAge   string `toml:"hsts_max_age"`
}

// Proxy contains reverse-proxy trust settings.
type Proxy struct {
	TrustedProxies []string `toml:"trusted_proxies"`
}

// Log contains logging settings.
type Log struct {
	Level  string `toml:"level"`
	Format string `toml:"format"`
}

// Metrics contains the optional private metrics listener.
type Metrics struct {
	Addr string `toml:"addr"`
}

// Demo controls the disposable public demo mode.
type Demo struct {
	Enabled       bool   `toml:"enabled"`
	Role          string `toml:"role"`
	ResetInterval string `toml:"reset_interval"`
}

// Default returns the built-in configuration.
func Default() Config {
	return Config{
		BaseURL:  "http://localhost:8080",
		HTTPAddr: ":8080",
		DataDir:  defaultDataDir,
		OIDC: OIDC{
			Provider:    "generic",
			Scopes:      []string{"openid", "profile", "email", "groups"},
			GroupsClaim: "groups",
			EmailClaim:  "email",
			NameClaim:   "name",
		},
		TLS: TLS{
			Port:         8443,
			HTTPRedirect: true,
			HSTSMaxAge:   "31536000s",
		},
		Log: Log{
			Level:  "info",
			Format: "json",
		},
		Demo: Demo{
			Role:          "member",
			ResetInterval: "30m",
		},
	}
}

// Load reads configuration from path, environment variables, and defaults.
// An explicitly supplied path must exist; discovered files may be absent.
func Load(path string) (*Config, error) {
	cfg := Default()

	resolved, explicit, err := resolvePath(path)
	if err != nil {
		return nil, err
	}
	if resolved != "" {
		meta, err := toml.DecodeFile(resolved, &cfg)
		if err != nil {
			return nil, fmt.Errorf("read config %s: %w", resolved, err)
		}
		if undecoded := meta.Undecoded(); len(undecoded) > 0 {
			return nil, fmt.Errorf("config %s contains unknown keys: %s", resolved, joinKeys(undecoded))
		}
		cfg.ConfigPath = resolved
	} else if explicit {
		return nil, fmt.Errorf("config file %q does not exist", path)
	}

	if err := applyEnv(&cfg); err != nil {
		return nil, err
	}
	if err := resolveSecrets(&cfg); err != nil {
		return nil, err
	}
	normalize(&cfg)
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// Validate checks cross-field constraints.
func (c *Config) Validate() error {
	if c.BaseURL == "" {
		return errors.New("base_url is required")
	}
	base, err := url.Parse(c.BaseURL)
	if err != nil || base.Host == "" || (base.Scheme != "http" && base.Scheme != "https") {
		return fmt.Errorf("base_url must be an absolute http(s) URL, got %q", c.BaseURL)
	}
	if c.DataDir == "" {
		return errors.New("data_dir is required")
	}
	if c.HTTPAddr == "" && !c.TLS.Enabled {
		return errors.New("http_addr is required when TLS is disabled")
	}
	if c.TLS.Enabled {
		if c.TLS.CertFile == "" || c.TLS.KeyFile == "" {
			return errors.New("tls.cert_file and tls.key_file are required when TLS is enabled")
		}
		if c.TLS.Port < 1 || c.TLS.Port > 65535 {
			return fmt.Errorf("tls.port must be between 1 and 65535, got %d", c.TLS.Port)
		}
		if base.Scheme != "https" {
			return errors.New("base_url must use https when built-in TLS is enabled")
		}
		if c.HTTPAddr != "" {
			_, httpPort, err := net.SplitHostPort(c.HTTPAddr)
			if err == nil && httpPort == strconv.Itoa(c.TLS.Port) {
				return errors.New("http_addr and tls.port must not use the same port")
			}
		}
	}
	if c.TLS.HSTSMaxAge != "" {
		if _, err := time.ParseDuration(c.TLS.HSTSMaxAge); err != nil {
			return fmt.Errorf("tls.hsts_max_age: %w", err)
		}
	}
	if _, err := ParseTrustedProxies(c.Proxy.TrustedProxies); err != nil {
		return fmt.Errorf("proxy.trusted_proxies: %w", err)
	}
	switch c.Log.Level {
	case "debug", "info", "warn", "error":
	default:
		return fmt.Errorf("log.level must be debug, info, warn, or error, got %q", c.Log.Level)
	}
	switch c.Log.Format {
	case "json", "text":
	default:
		return fmt.Errorf("log.format must be json or text, got %q", c.Log.Format)
	}
	if !c.Demo.Enabled && c.OIDC.Issuer != "" {
		issuer, err := url.Parse(c.OIDC.Issuer)
		if err != nil || issuer.Host == "" || (issuer.Scheme != "https" && issuer.Scheme != "http") {
			return fmt.Errorf("oidc.issuer must be an absolute http(s) URL, got %q", c.OIDC.Issuer)
		}
		if issuer.Scheme == "http" && !isLoopback(issuer.Hostname()) {
			return errors.New("oidc.issuer must use https unless it is a loopback address")
		}
		if c.OIDC.ClientID == "" {
			return errors.New("oidc.client_id is required when oidc.issuer is set")
		}
		if c.OIDC.RedirectURL == "" {
			return errors.New("oidc.redirect_url is required when oidc.issuer is set")
		}
		redirect, err := url.Parse(c.OIDC.RedirectURL)
		if err != nil || redirect.Host == "" || (redirect.Scheme != "http" && redirect.Scheme != "https") {
			return fmt.Errorf("oidc.redirect_url must be an absolute http(s) URL, got %q", c.OIDC.RedirectURL)
		}
		if len(c.OIDC.AllowedGroups) == 0 && len(c.OIDC.AllowedEmailDomains) == 0 && len(c.OIDC.AllowedEmails) == 0 &&
			len(c.OIDC.AdminGroups) == 0 && len(c.OIDC.AdminEmails) == 0 {
			return errors.New("oidc requires at least one authorization source: allowed_groups, allowed_email_domains, allowed_emails, admin_groups, or admin_emails")
		}
		if len(c.OIDC.AdminGroups) == 0 && len(c.OIDC.AdminEmails) == 0 {
			return errors.New("oidc requires at least one admin source: admin_groups or admin_emails")
		}
	}
	if c.Demo.Enabled {
		switch c.Demo.Role {
		case "member", "admin":
		default:
			return fmt.Errorf("demo.role must be member or admin, got %q", c.Demo.Role)
		}
		interval, err := time.ParseDuration(c.Demo.ResetInterval)
		if err != nil {
			return fmt.Errorf("demo.reset_interval: %w", err)
		}
		if interval < time.Minute {
			return errors.New("demo.reset_interval must be at least 1m")
		}
	}
	if c.Metrics.Addr != "" {
		if _, _, err := splitHostPort(c.Metrics.Addr); err != nil {
			return fmt.Errorf("metrics.addr: %w", err)
		}
	}
	return nil
}

func resolveSecrets(cfg *Config) error {
	var err error
	cfg.OIDC.ClientSecret, err = readSecret(cfg.OIDC.ClientSecret, cfg.OIDC.ClientSecretFile)
	if err != nil {
		return fmt.Errorf("oidc client secret: %w", err)
	}

	raw, err := readSecret(cfg.SecretKey, cfg.SecretKeyFile)
	if err != nil {
		return fmt.Errorf("secret key: %w", err)
	}
	if raw == "" {
		return errors.New("secret key is required: set IWMMD_SECRET_KEY or IWMMD_SECRET_KEY_FILE (generate one with `iwmmd gen-key`)")
	}
	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(raw))
	if err != nil {
		return fmt.Errorf("secret key must be base64: %w", err)
	}
	if len(key) != secretKeyBytes {
		return fmt.Errorf("secret key must decode to %d bytes, got %d", secretKeyBytes, len(key))
	}
	cfg.MasterKey = key
	return nil
}

func readSecret(value, file string) (string, error) {
	if value != "" {
		return strings.TrimSpace(value), nil
	}
	if file == "" {
		return "", nil
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

func normalize(cfg *Config) {
	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")
	cfg.DataDir = filepath.Clean(cfg.DataDir)
	if cfg.OIDC.Provider == "" {
		cfg.OIDC.Provider = "generic"
	}
	if len(cfg.OIDC.Scopes) == 0 {
		cfg.OIDC.Scopes = []string{"openid", "profile", "email", "groups"}
	}
	if cfg.OIDC.GroupsClaim == "" {
		cfg.OIDC.GroupsClaim = "groups"
	}
	if cfg.OIDC.EmailClaim == "" {
		cfg.OIDC.EmailClaim = "email"
	}
	if cfg.OIDC.NameClaim == "" {
		cfg.OIDC.NameClaim = "name"
	}
	cfg.Demo.Role = strings.ToLower(strings.TrimSpace(cfg.Demo.Role))
	if cfg.Demo.Role == "" {
		cfg.Demo.Role = "member"
	}
	if cfg.Demo.ResetInterval == "" {
		cfg.Demo.ResetInterval = "30m"
	}
}

func resolvePath(path string) (string, bool, error) {
	if path != "" {
		if _, err := os.Stat(path); err != nil {
			return "", true, err
		}
		return path, true, nil
	}
	if path = os.Getenv("IWMMD_CONFIG"); path != "" {
		if _, err := os.Stat(path); err != nil {
			return "", true, err
		}
		return path, true, nil
	}

	var candidates []string
	candidates = append(candidates, "/etc/iwmmd/config.toml")
	if dir, err := os.UserConfigDir(); err == nil {
		candidates = append(candidates, filepath.Join(dir, "iwmmd", "config.toml"))
	}
	candidates = append(candidates, "config.toml")
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, false, nil
		}
	}
	return "", false, nil
}

func applyEnv(cfg *Config) error {
	var errs []error
	setString := func(key string, dst *string) {
		if v, ok := os.LookupEnv(key); ok {
			*dst = v
		}
	}
	setBool := func(key string, dst *bool) {
		if v, ok := os.LookupEnv(key); ok {
			parsed, err := strconv.ParseBool(v)
			if err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", key, err))
				return
			}
			*dst = parsed
		}
	}
	setInt := func(key string, dst *int) {
		if v, ok := os.LookupEnv(key); ok {
			parsed, err := strconv.Atoi(v)
			if err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", key, err))
				return
			}
			*dst = parsed
		}
	}
	setList := func(key string, dst *[]string) {
		if v, ok := os.LookupEnv(key); ok {
			*dst = splitList(v)
		}
	}

	setString("IWMMD_BASE_URL", &cfg.BaseURL)
	setString("IWMMD_HTTP_ADDR", &cfg.HTTPAddr)
	setString("IWMMD_DATA_DIR", &cfg.DataDir)
	setString("IWMMD_DB_DSN", &cfg.DBDSN)
	setString("IWMMD_SECRET_KEY", &cfg.SecretKey)
	setString("IWMMD_SECRET_KEY_FILE", &cfg.SecretKeyFile)

	setString("IWMMD_OIDC_PROVIDER", &cfg.OIDC.Provider)
	setString("IWMMD_OIDC_ISSUER", &cfg.OIDC.Issuer)
	setString("IWMMD_OIDC_CLIENT_ID", &cfg.OIDC.ClientID)
	setString("IWMMD_OIDC_CLIENT_SECRET", &cfg.OIDC.ClientSecret)
	setString("IWMMD_OIDC_CLIENT_SECRET_FILE", &cfg.OIDC.ClientSecretFile)
	setString("IWMMD_OIDC_REDIRECT_URL", &cfg.OIDC.RedirectURL)
	setList("IWMMD_OIDC_SCOPES", &cfg.OIDC.Scopes)
	setString("IWMMD_OIDC_GROUPS_CLAIM", &cfg.OIDC.GroupsClaim)
	setString("IWMMD_OIDC_EMAIL_CLAIM", &cfg.OIDC.EmailClaim)
	setString("IWMMD_OIDC_NAME_CLAIM", &cfg.OIDC.NameClaim)
	setList("IWMMD_OIDC_ALLOWED_GROUPS", &cfg.OIDC.AllowedGroups)
	setList("IWMMD_OIDC_ADMIN_GROUPS", &cfg.OIDC.AdminGroups)
	setList("IWMMD_OIDC_ALLOWED_EMAIL_DOMAINS", &cfg.OIDC.AllowedEmailDomains)
	setList("IWMMD_OIDC_ALLOWED_EMAILS", &cfg.OIDC.AllowedEmails)
	setList("IWMMD_OIDC_ADMIN_EMAILS", &cfg.OIDC.AdminEmails)
	setBool("IWMMD_OIDC_TRUST_EMAIL_CLAIM", &cfg.OIDC.TrustEmailClaim)

	setList("IWMMD_TRUSTED_PROXIES", &cfg.Proxy.TrustedProxies)

	setBool("IWMMD_TLS_ENABLED", &cfg.TLS.Enabled)
	setString("IWMMD_TLS_CERT_FILE", &cfg.TLS.CertFile)
	setString("IWMMD_TLS_KEY_FILE", &cfg.TLS.KeyFile)
	setInt("IWMMD_TLS_PORT", &cfg.TLS.Port)
	setBool("IWMMD_TLS_HTTP_REDIRECT", &cfg.TLS.HTTPRedirect)
	setString("IWMMD_TLS_HSTS_MAX_AGE", &cfg.TLS.HSTSMaxAge)

	setString("IWMMD_LOG_LEVEL", &cfg.Log.Level)
	setString("IWMMD_LOG_FORMAT", &cfg.Log.Format)
	setString("IWMMD_METRICS_ADDR", &cfg.Metrics.Addr)

	setBool("IWMMD_DEMO_MODE", &cfg.Demo.Enabled)
	setString("IWMMD_DEMO_ROLE", &cfg.Demo.Role)
	setString("IWMMD_DEMO_RESET_INTERVAL", &cfg.Demo.ResetInterval)

	return errors.Join(errs...)
}

func splitHostPort(addr string) (string, string, error) {
	return net.SplitHostPort(addr)
}

func splitList(v string) []string {
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func joinKeys(keys []toml.Key) string {
	parts := make([]string, len(keys))
	for i, key := range keys {
		parts[i] = key.String()
	}
	return strings.Join(parts, ", ")
}

func isLoopback(host string) bool {
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}
