// Package settings reads and writes runtime configuration stored in SQLite.
package settings

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/mail"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/mrweez/iwmmd/internal/calendar"
	"github.com/mrweez/iwmmd/internal/platform/secrets"
	"github.com/mrweez/iwmmd/internal/store"
)

const (
	keySMTPHost      = "smtp.host"
	keySMTPPort      = "smtp.port"
	keySMTPTLSMode   = "smtp.tls_mode"
	keySMTPUsername  = "smtp.username"
	keySMTPPassword  = "smtp.password"
	keySMTPFrom      = "smtp.from"
	keyAlertsTZ      = "alerts.timezone"
	keyAlertsTime    = "alerts.default_time"
	keyAlertsPaused  = "alerts.paused_until"
	keyRetRawDays    = "retention.raw_days"
	keyRetChecksDays = "retention.checks_days"
	keyRetNotifDays  = "retention.notifications_days"
	keyCheckInterval = "check.default_interval"
	keyCheckManual   = "check.manual_rate_limit"
)

// Service loads runtime settings and applies environment overrides.
type Service struct {
	store *store.Store
	box   *secrets.Box
}

// SMTP contains outbound email settings.
type SMTP struct {
	Host         string
	Port         int
	TLSMode      string
	Username     string
	Password     string
	PasswordSet  bool
	From         string
	ManagedByEnv bool
}

// Alerts contains default scheduling settings.
type Alerts struct {
	Timezone     string
	DefaultTime  string
	PausedUntil  *time.Time
	ManagedByEnv bool
}

// Retention contains history retention settings.
type Retention struct {
	RawDays           int  `json:"raw_days"`
	ChecksDays        int  `json:"checks_days"`
	NotificationsDays int  `json:"notifications_days"`
	ManagedByEnv      bool `json:"managed_by_env,omitempty"`
}

// Check contains scheduler settings.
type Check struct {
	DefaultIntervalSeconds int  `json:"default_interval"`
	ManualRateLimitSeconds int  `json:"manual_rate_limit"`
	ManagedByEnv           bool `json:"managed_by_env,omitempty"`
}

// Snapshot is the effective runtime configuration.
type Snapshot struct {
	SMTP      SMTP
	Alerts    Alerts
	Retention Retention
	Check     Check
}

// New creates a settings service.
func New(st *store.Store, box *secrets.Box) *Service {
	return &Service{store: st, box: box}
}

// Get returns effective settings.
func (s *Service) Get(ctx context.Context) (Snapshot, error) {
	snap := Snapshot{
		SMTP: SMTP{
			Port:    587,
			TLSMode: "starttls",
		},
		Alerts: Alerts{
			Timezone:    "UTC",
			DefaultTime: "09:00",
		},
		Retention: Retention{RawDays: 7, ChecksDays: 90, NotificationsDays: 180},
		Check:     Check{DefaultIntervalSeconds: 86400, ManualRateLimitSeconds: 300},
	}
	host, err := s.getString(ctx, keySMTPHost, snap.SMTP.Host, false)
	if err != nil {
		return snap, err
	}
	snap.SMTP.Host = host
	port, err := s.getInt(ctx, keySMTPPort, snap.SMTP.Port)
	if err != nil {
		return snap, err
	}
	snap.SMTP.Port = port
	tlsMode, err := s.getString(ctx, keySMTPTLSMode, snap.SMTP.TLSMode, false)
	if err != nil {
		return snap, err
	}
	snap.SMTP.TLSMode = tlsMode
	username, err := s.getString(ctx, keySMTPUsername, "", false)
	if err != nil {
		return snap, err
	}
	snap.SMTP.Username = username
	password, err := s.getString(ctx, keySMTPPassword, "", true)
	if err != nil {
		return snap, err
	}
	snap.SMTP.Password = password
	snap.SMTP.PasswordSet = password != ""
	from, err := s.getString(ctx, keySMTPFrom, "", false)
	if err != nil {
		return snap, err
	}
	snap.SMTP.From = from

	tz, err := s.getString(ctx, keyAlertsTZ, snap.Alerts.Timezone, false)
	if err != nil {
		return snap, err
	}
	snap.Alerts.Timezone = tz
	defaultTime, err := s.getString(ctx, keyAlertsTime, snap.Alerts.DefaultTime, false)
	if err != nil {
		return snap, err
	}
	snap.Alerts.DefaultTime = defaultTime
	paused, err := s.getString(ctx, keyAlertsPaused, "", false)
	if err != nil {
		return snap, err
	}
	if paused != "" {
		if t, err := time.Parse(time.RFC3339, paused); err == nil {
			snap.Alerts.PausedUntil = &t
		}
	}

	rawDays, err := s.getInt(ctx, keyRetRawDays, snap.Retention.RawDays)
	if err != nil {
		return snap, err
	}
	checksDays, err := s.getInt(ctx, keyRetChecksDays, snap.Retention.ChecksDays)
	if err != nil {
		return snap, err
	}
	notifDays, err := s.getInt(ctx, keyRetNotifDays, snap.Retention.NotificationsDays)
	if err != nil {
		return snap, err
	}
	snap.Retention = Retention{RawDays: rawDays, ChecksDays: checksDays, NotificationsDays: notifDays}

	interval, err := s.getInt(ctx, keyCheckInterval, snap.Check.DefaultIntervalSeconds)
	if err != nil {
		return snap, err
	}
	manual, err := s.getInt(ctx, keyCheckManual, snap.Check.ManualRateLimitSeconds)
	if err != nil {
		return snap, err
	}
	snap.Check = Check{DefaultIntervalSeconds: interval, ManualRateLimitSeconds: manual}

	applyEnv(&snap)
	return snap, nil
}

// Location resolves the configured alert timezone.
func (a Alerts) Location() (*time.Location, error) {
	return time.LoadLocation(a.Timezone)
}

// SMTPInput is an admin SMTP update. Nil Password keeps the current secret.
type SMTPInput struct {
	Host     string  `json:"host"`
	Port     int     `json:"port"`
	TLSMode  string  `json:"tls_mode"`
	Username string  `json:"username"`
	Password *string `json:"password,omitempty"`
	From     string  `json:"from"`
}

// SaveSMTP validates and stores SMTP settings.
func (s *Service) SaveSMTP(ctx context.Context, in SMTPInput, userID string, at time.Time) error {
	in.Host = strings.TrimSpace(in.Host)
	in.From = strings.TrimSpace(in.From)
	if in.Host == "" && in.From == "" {
		// Clear the configuration while keeping sane defaults.
		in.Port = 587
		in.TLSMode = "starttls"
	} else {
		if in.Host == "" {
			return errors.New("SMTP host is required")
		}
		if in.Port < 1 || in.Port > 65535 {
			return errors.New("SMTP port must be between 1 and 65535")
		}
		switch in.TLSMode {
		case "starttls", "implicit", "none":
		default:
			return errors.New("SMTP TLS mode must be starttls, implicit, or none")
		}
		if _, err := mail.ParseAddress(in.From); err != nil {
			return errors.New("SMTP from address is invalid")
		}
	}
	if err := s.putString(ctx, keySMTPHost, in.Host, false, userID, at); err != nil {
		return err
	}
	if err := s.putInt(ctx, keySMTPPort, in.Port, userID, at); err != nil {
		return err
	}
	if err := s.putString(ctx, keySMTPTLSMode, in.TLSMode, false, userID, at); err != nil {
		return err
	}
	if err := s.putString(ctx, keySMTPUsername, strings.TrimSpace(in.Username), false, userID, at); err != nil {
		return err
	}
	if in.Password != nil {
		if err := s.putString(ctx, keySMTPPassword, *in.Password, true, userID, at); err != nil {
			return err
		}
	}
	return s.putString(ctx, keySMTPFrom, in.From, false, userID, at)
}

// AlertsInput is an admin alert-settings update.
type AlertsInput struct {
	Timezone    string
	DefaultTime string
	PausedUntil *time.Time
}

// SaveAlerts validates and stores alert defaults.
func (s *Service) SaveAlerts(ctx context.Context, in AlertsInput, userID string, at time.Time) error {
	if _, err := time.LoadLocation(in.Timezone); err != nil {
		return fmt.Errorf("alert timezone is invalid: %w", err)
	}
	if _, _, err := calendar.ParseSendTime(in.DefaultTime); err != nil {
		return err
	}
	paused := ""
	if in.PausedUntil != nil {
		paused = in.PausedUntil.UTC().Format(time.RFC3339)
	}
	if err := s.putString(ctx, keyAlertsTZ, in.Timezone, false, userID, at); err != nil {
		return err
	}
	if err := s.putString(ctx, keyAlertsTime, in.DefaultTime, false, userID, at); err != nil {
		return err
	}
	return s.putString(ctx, keyAlertsPaused, paused, false, userID, at)
}

// SaveRetention stores retention days.
func (s *Service) SaveRetention(ctx context.Context, in Retention, userID string, at time.Time) error {
	if in.RawDays < 1 || in.ChecksDays < 1 || in.NotificationsDays < 1 {
		return errors.New("retention days must be at least 1")
	}
	if err := s.putInt(ctx, keyRetRawDays, in.RawDays, userID, at); err != nil {
		return err
	}
	if err := s.putInt(ctx, keyRetChecksDays, in.ChecksDays, userID, at); err != nil {
		return err
	}
	return s.putInt(ctx, keyRetNotifDays, in.NotificationsDays, userID, at)
}

// SaveCheck stores scheduler settings.
func (s *Service) SaveCheck(ctx context.Context, in Check, userID string, at time.Time) error {
	if in.DefaultIntervalSeconds < 300 {
		return errors.New("default check interval must be at least 300 seconds")
	}
	if in.ManualRateLimitSeconds < 60 {
		return errors.New("manual check rate limit must be at least 60 seconds")
	}
	if err := s.putInt(ctx, keyCheckInterval, in.DefaultIntervalSeconds, userID, at); err != nil {
		return err
	}
	return s.putInt(ctx, keyCheckManual, in.ManualRateLimitSeconds, userID, at)
}

func (s *Service) getString(ctx context.Context, key, fallback string, secret bool) (string, error) {
	row, err := s.store.GetSetting(ctx, key)
	if errors.Is(err, store.ErrNotFound) {
		return fallback, nil
	}
	if err != nil {
		return "", err
	}
	if row.IsSecret {
		if row.Ciphertext == "" {
			return fallback, nil
		}
		plaintext, err := s.box.Decrypt(row.Ciphertext)
		if err != nil {
			return "", fmt.Errorf("decrypt setting %s: %w", key, err)
		}
		return string(plaintext), nil
	}
	if row.ValueJSON == "" {
		return fallback, nil
	}
	var value string
	if err := json.Unmarshal([]byte(row.ValueJSON), &value); err != nil {
		return fallback, nil
	}
	return value, nil
}

func (s *Service) getInt(ctx context.Context, key string, fallback int) (int, error) {
	row, err := s.store.GetSetting(ctx, key)
	if errors.Is(err, store.ErrNotFound) {
		return fallback, nil
	}
	if err != nil {
		return 0, err
	}
	var value int
	if err := json.Unmarshal([]byte(row.ValueJSON), &value); err != nil {
		return fallback, nil
	}
	return value, nil
}

func (s *Service) putString(ctx context.Context, key, value string, secret bool, userID string, at time.Time) error {
	if secret {
		ciphertext, err := s.box.Encrypt([]byte(value))
		if err != nil {
			return err
		}
		return s.store.PutSetting(ctx, key, "", ciphertext, true, userID, at)
	}
	body, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return s.store.PutSetting(ctx, key, string(body), "", false, userID, at)
}

func (s *Service) putInt(ctx context.Context, key string, value int, userID string, at time.Time) error {
	body, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return s.store.PutSetting(ctx, key, string(body), "", false, userID, at)
}

func applyEnv(snap *Snapshot) {
	if value, ok := lookupEnv("IWMMD_SMTP_HOST"); ok {
		snap.SMTP.Host = value
		snap.SMTP.ManagedByEnv = true
	}
	if value, ok := lookupEnv("IWMMD_SMTP_PORT"); ok {
		if port, err := strconv.Atoi(value); err == nil {
			snap.SMTP.Port = port
			snap.SMTP.ManagedByEnv = true
		}
	}
	if value, ok := lookupEnv("IWMMD_SMTP_TLS_MODE"); ok {
		snap.SMTP.TLSMode = value
		snap.SMTP.ManagedByEnv = true
	}
	if value, ok := lookupEnv("IWMMD_SMTP_USERNAME"); ok {
		snap.SMTP.Username = value
		snap.SMTP.ManagedByEnv = true
	}
	if value, ok := lookupSecretEnv("IWMMD_SMTP_PASSWORD"); ok {
		snap.SMTP.Password = value
		snap.SMTP.PasswordSet = value != ""
		snap.SMTP.ManagedByEnv = true
	}
	if value, ok := lookupEnv("IWMMD_SMTP_FROM"); ok {
		snap.SMTP.From = value
		snap.SMTP.ManagedByEnv = true
	}
	if value, ok := lookupEnv("IWMMD_ALERTS_TIMEZONE"); ok {
		snap.Alerts.Timezone = value
		snap.Alerts.ManagedByEnv = true
	}
	if value, ok := lookupEnv("IWMMD_ALERTS_DEFAULT_TIME"); ok {
		snap.Alerts.DefaultTime = value
		snap.Alerts.ManagedByEnv = true
	}
	if value, ok := lookupEnv("IWMMD_ALERTS_PAUSED_UNTIL"); ok {
		if t, err := time.Parse(time.RFC3339, value); err == nil {
			snap.Alerts.PausedUntil = &t
			snap.Alerts.ManagedByEnv = true
		}
	}
	if value, ok := lookupEnv("IWMMD_RETENTION_RAW_DAYS"); ok {
		if n, err := strconv.Atoi(value); err == nil {
			snap.Retention.RawDays = n
			snap.Retention.ManagedByEnv = true
		}
	}
	if value, ok := lookupEnv("IWMMD_RETENTION_CHECKS_DAYS"); ok {
		if n, err := strconv.Atoi(value); err == nil {
			snap.Retention.ChecksDays = n
			snap.Retention.ManagedByEnv = true
		}
	}
	if value, ok := lookupEnv("IWMMD_RETENTION_NOTIFICATIONS_DAYS"); ok {
		if n, err := strconv.Atoi(value); err == nil {
			snap.Retention.NotificationsDays = n
			snap.Retention.ManagedByEnv = true
		}
	}
	if value, ok := lookupEnv("IWMMD_CHECK_DEFAULT_INTERVAL"); ok {
		if n, err := strconv.Atoi(value); err == nil {
			snap.Check.DefaultIntervalSeconds = n
			snap.Check.ManagedByEnv = true
		}
	}
	if value, ok := lookupEnv("IWMMD_CHECK_MANUAL_RATE_LIMIT"); ok {
		if n, err := strconv.Atoi(value); err == nil {
			snap.Check.ManualRateLimitSeconds = n
			snap.Check.ManagedByEnv = true
		}
	}
}

func lookupEnv(key string) (string, bool) {
	value, ok := os.LookupEnv(key)
	if !ok {
		return "", false
	}
	return value, true
}

func lookupSecretEnv(key string) (string, bool) {
	if file := os.Getenv(key + "_FILE"); file != "" {
		body, err := os.ReadFile(file)
		if err == nil {
			return strings.TrimSpace(string(body)), true
		}
	}
	return lookupEnv(key)
}
