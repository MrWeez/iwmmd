// Package demo seeds and resets disposable public demo data.
package demo

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/mrweez/iwmmd/internal/alert"
	"github.com/mrweez/iwmmd/internal/notify"
	"github.com/mrweez/iwmmd/internal/platform/secrets"
	"github.com/mrweez/iwmmd/internal/store"
)

const (
	// UserID is the stable identity used by anonymous demo requests.
	UserID = "00000000-0000-7000-8000-000000000001"
	// UserEmail is the demo account address shown in the interface.
	UserEmail = "demo@iwmmd.local"
	// UserName is the demo account display name.
	UserName = "Demo User"

	markerKey = "demo.marker"
)

// State holds immutable demo identity and the next reset deadline.
type State struct {
	User          store.User
	CSRF          string
	Role          string
	ResetInterval time.Duration

	nextReset atomic.Int64
}

// NewState builds demo identity and reset metadata.
func NewState(role string, interval time.Duration) *State {
	now := time.Now().UTC()
	if interval < time.Minute {
		interval = 30 * time.Minute
	}
	state := &State{
		User: store.User{
			ID:          UserID,
			Issuer:      "demo",
			Subject:     "demo",
			Email:       UserEmail,
			DisplayName: UserName,
			Role:        role,
			LastLoginAt: &now,
			CreatedAt:   now.AddDate(0, -3, 0),
			UpdatedAt:   now,
		},
		CSRF:          randomToken(),
		Role:          role,
		ResetInterval: interval,
	}
	state.SetNextReset(now.Add(interval))
	return state
}

// NextReset returns the next scheduled reset time.
func (s *State) NextReset() time.Time {
	value := s.nextReset.Load()
	if value == 0 {
		return time.Time{}
	}
	return time.Unix(0, value).UTC()
}

// SetNextReset updates the next scheduled reset time.
func (s *State) SetNextReset(at time.Time) {
	s.nextReset.Store(at.UTC().UnixNano())
}

// Seeder initializes and periodically replaces demo content.
type Seeder struct {
	Store  *store.Store
	Box    *secrets.Box
	State  *State
	Logger *slog.Logger
}

// NewSeeder creates a demo seeder.
func NewSeeder(st *store.Store, box *secrets.Box, state *State, logger *slog.Logger) *Seeder {
	if logger == nil {
		logger = slog.Default()
	}
	return &Seeder{Store: st, Box: box, State: state, Logger: logger}
}

// Initialize seeds an empty database or resets an existing demo database.
func (s *Seeder) Initialize(ctx context.Context, now time.Time) error {
	marker, err := s.hasMarker(ctx)
	if err != nil {
		return err
	}
	if !marker {
		empty, err := s.isEmpty(ctx)
		if err != nil {
			return err
		}
		if !empty {
			return errors.New("demo mode refuses to start: data directory contains existing data; use a separate empty data directory")
		}
	}
	return s.Reset(ctx, now)
}

// Run resets demo content on the configured interval.
func (s *Seeder) Run(ctx context.Context) {
	for {
		next := s.State.NextReset()
		wait := time.Until(next)
		if wait < 0 {
			wait = 0
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
			now := time.Now().UTC()
			if err := s.Reset(ctx, now); err != nil {
				if !errors.Is(err, context.Canceled) {
					s.Logger.Error("reset demo data", "error", err)
				}
				s.State.SetNextReset(now.Add(s.State.ResetInterval))
				continue
			}
			s.Logger.Info("reset demo data", "next_reset", s.State.NextReset())
		}
	}
}

// Reset replaces all demo content inside one transaction.
func (s *Seeder) Reset(ctx context.Context, now time.Time) error {
	now = now.UTC()
	tx, err := s.Store.Writer.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, table := range []string{
		"delivery_attempts",
		"notifications",
		"incidents",
		"alert_rule_channels",
		"alert_rules",
		"channels",
		"check_raw_responses",
		"domain_checks",
		"domains",
		"jobs",
		"sessions",
		"audit_log",
		"users",
	} {
		if _, err := tx.ExecContext(ctx, "DELETE FROM "+table); err != nil {
			return fmt.Errorf("clear %s: %w", table, err)
		}
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM settings WHERE key <> ?", markerKey); err != nil {
		return fmt.Errorf("clear settings: %w", err)
	}

	if err := s.seedUser(ctx, tx, now); err != nil {
		return err
	}
	if err := s.seedSettings(ctx, tx, now); err != nil {
		return err
	}
	domains := demoDomains()
	if err := s.seedDomains(ctx, tx, now, domains); err != nil {
		return err
	}
	if err := s.seedChecks(ctx, tx, now, domains); err != nil {
		return err
	}
	if err := s.seedChannels(ctx, tx, now); err != nil {
		return err
	}
	if err := s.seedRules(ctx, tx, now, domains); err != nil {
		return err
	}
	if err := s.seedIncidents(ctx, tx, now, domains); err != nil {
		return err
	}
	if err := s.seedNotifications(ctx, tx, now, domains); err != nil {
		return err
	}
	if err := s.seedAudit(ctx, tx, now); err != nil {
		return err
	}
	if err := seedMarker(ctx, tx, now); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}

	s.State.SetNextReset(now.Add(s.State.ResetInterval))
	return nil
}

func (s *Seeder) hasMarker(ctx context.Context) (bool, error) {
	var one int
	err := s.Store.Reader.QueryRowContext(ctx, "SELECT 1 FROM settings WHERE key = ? LIMIT 1", markerKey).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

func (s *Seeder) isEmpty(ctx context.Context) (bool, error) {
	var count int
	err := s.Store.Reader.QueryRowContext(ctx, `
		SELECT
			(SELECT COUNT(*) FROM users) +
			(SELECT COUNT(*) FROM sessions) +
			(SELECT COUNT(*) FROM domains) +
			(SELECT COUNT(*) FROM domain_checks) +
			(SELECT COUNT(*) FROM check_raw_responses) +
			(SELECT COUNT(*) FROM channels) +
			(SELECT COUNT(*) FROM alert_rules) +
			(SELECT COUNT(*) FROM alert_rule_channels) +
			(SELECT COUNT(*) FROM incidents) +
			(SELECT COUNT(*) FROM notifications) +
			(SELECT COUNT(*) FROM delivery_attempts) +
			(SELECT COUNT(*) FROM jobs) +
			(SELECT COUNT(*) FROM audit_log) +
			(SELECT COUNT(*) FROM settings)`).Scan(&count)
	if err != nil {
		return false, err
	}
	return count == 0, nil
}

func (s *Seeder) seedUser(ctx context.Context, tx *sql.Tx, now time.Time) error {
	created := now.AddDate(0, -3, 0)
	_, err := tx.ExecContext(ctx, `
		INSERT INTO users (id, oidc_issuer, oidc_subject, email, display_name, role,
			last_login_at, created_at, updated_at)
		VALUES (?, 'demo', 'demo', ?, ?, ?, ?, ?, ?)`,
		UserID, UserEmail, UserName, s.State.Role, dbTime(now), dbTime(created), dbTime(now))
	return err
}

func (s *Seeder) seedSettings(ctx context.Context, tx *sql.Tx, now time.Time) error {
	values := []struct {
		key   string
		value any
	}{
		{"smtp.host", "smtp.demo.invalid"},
		{"smtp.port", 587},
		{"smtp.tls_mode", "starttls"},
		{"smtp.username", "demo"},
		{"smtp.from", "alerts@demo.invalid"},
		{"alerts.timezone", "UTC"},
		{"alerts.default_time", "09:00"},
		{"retention.raw_days", 7},
		{"retention.checks_days", 90},
		{"retention.notifications_days", 180},
		{"check.default_interval", 86400},
		{"check.manual_rate_limit", 300},
	}
	updated := now.AddDate(0, 0, -30)
	for _, item := range values {
		body, err := json.Marshal(item.value)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO settings (key, value_json, is_secret, updated_by, updated_at)
			VALUES (?, ?, 0, ?, ?)`,
			item.key, string(body), UserID, dbTime(updated)); err != nil {
			return fmt.Errorf("seed setting %s: %w", item.key, err)
		}
	}
	return nil
}

type seedDomain struct {
	ID             string
	Name           string
	Registrar      string
	ExpiryDays     *int
	State          string
	Confidence     string
	LastStatus     string
	LastError      string
	Failures       int
	Notes          string
	InheritGlobal  bool
	Muted          bool
	CreatedDaysAgo int
	NextCheckIn    time.Duration
}

func demoDomains() []seedDomain {
	return []seedDomain{
		{
			ID: "00000000-0000-7000-8000-000000000101", Name: "example.com",
			Registrar: "Example Registrar", ExpiryDays: intPtr(6), State: "active",
			Confidence: "high", LastStatus: "ok", InheritGlobal: true,
			Notes: "Primary marketing domain.", CreatedDaysAgo: 120, NextCheckIn: 6 * time.Hour,
		},
		{
			ID: "00000000-0000-7000-8000-000000000102", Name: "example.net",
			Registrar: "Example Registrar", ExpiryDays: intPtr(21), State: "active",
			Confidence: "high", LastStatus: "ok", InheritGlobal: true, Muted: true,
			Notes: "Customer portal. Registrar migration in progress.", CreatedDaysAgo: 90, NextCheckIn: 12 * time.Hour,
		},
		{
			ID: "00000000-0000-7000-8000-000000000103", Name: "example.org",
			Registrar: "Example Foundation", ExpiryDays: intPtr(75), State: "active",
			Confidence: "medium", LastStatus: "ok", InheritGlobal: true,
			Notes: "Foundation site with an extra 60-day reminder.", CreatedDaysAgo: 200, NextCheckIn: 18 * time.Hour,
		},
		{
			ID: "00000000-0000-7000-8000-000000000104", Name: "northwind-demo.com",
			Registrar: "Northwind Domains", ExpiryDays: intPtr(-4), State: "active",
			Confidence: "high", LastStatus: "ok", InheritGlobal: true,
			Notes: "Expired during the last renewal cycle.", CreatedDaysAgo: 60, NextCheckIn: 24 * time.Hour,
		},
		{
			ID: "00000000-0000-7000-8000-000000000105", Name: "acme-demo.net",
			Registrar: "Acme Registry", State: "active", Confidence: "unknown",
			LastStatus: "unsupported", LastError: "No expiry date found in RDAP response",
			Failures: 2, InheritGlobal: true, Notes: "Registry does not expose an expiry event.",
			CreatedDaysAgo: 45, NextCheckIn: 12 * time.Hour,
		},
		{
			ID: "00000000-0000-7000-8000-000000000106", Name: "paused-demo.org",
			Registrar: "Demo Registrar", ExpiryDays: intPtr(120), State: "paused",
			Confidence: "high", LastStatus: "ok", InheritGlobal: true,
			Notes: "Paused while the owner confirms the renewal.", CreatedDaysAgo: 30,
		},
		{
			ID: "00000000-0000-7000-8000-000000000107", Name: "archived-demo.com",
			Registrar: "Demo Registrar", ExpiryDays: intPtr(180), State: "archived",
			Confidence: "high", LastStatus: "ok", InheritGlobal: true,
			Notes: "Archived after the project ended.", CreatedDaysAgo: 300,
		},
		{
			ID: "00000000-0000-7000-8000-000000000108", Name: "labs-demo.io",
			Registrar: "Demo Registrar", ExpiryDays: intPtr(2), State: "active",
			Confidence: "high", LastStatus: "ok", InheritGlobal: true,
			Notes: "Short-lived experimental domain.", CreatedDaysAgo: 14, NextCheckIn: 6 * time.Hour,
		},
	}
}

func (s *Seeder) seedDomains(ctx context.Context, tx *sql.Tx, now time.Time, domains []seedDomain) error {
	for _, d := range domains {
		var expires any
		if d.ExpiryDays != nil {
			expires = dbTime(demoDate(now, *d.ExpiryDays))
		}
		var nextCheck any
		if d.State == "active" && d.NextCheckIn > 0 {
			nextCheck = dbTime(now.Add(d.NextCheckIn))
		}
		var mutedUntil any
		var muteReason any
		if d.Muted {
			mutedUntil = dbTime(now.AddDate(0, 0, 2))
			muteReason = "Registrar migration window"
		}
		lastChecked := now.Add(-2 * time.Hour)
		if d.State == "paused" {
			lastChecked = now.AddDate(0, 0, -1)
		}
		if d.State == "archived" {
			lastChecked = now.AddDate(0, 0, -20)
		}
		var archivedAt any
		updatedAt := lastChecked
		if d.State == "archived" {
			archivedAt = dbTime(now.AddDate(0, 0, -10))
			updatedAt = now.AddDate(0, 0, -10)
		}
		lastError := any(nil)
		if d.LastError != "" {
			lastError = d.LastError
		}
		_, err := tx.ExecContext(ctx, `
			INSERT INTO domains (id, name_ace, name_unicode, registrar, expires_at, state,
				inherit_global_alerts, muted_until, mute_reason, check_interval_seconds,
				next_check_at, last_checked_at, last_check_status, last_check_error,
				expiry_confidence, consecutive_failures, version, notes, created_by,
				created_at, updated_at, archived_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 86400, ?, ?, ?, ?, ?, ?, 2, ?, ?, ?, ?, ?)`,
			d.ID, d.Name, d.Name, d.Registrar, expires, d.State, boolInt(d.InheritGlobal),
			mutedUntil, muteReason, nextCheck, dbTime(lastChecked), d.LastStatus, lastError,
			d.Confidence, d.Failures, d.Notes, UserID,
			dbTime(now.AddDate(0, 0, -d.CreatedDaysAgo)), dbTime(updatedAt), archivedAt)
		if err != nil {
			return fmt.Errorf("seed domain %s: %w", d.Name, err)
		}
	}
	return nil
}

func (s *Seeder) seedChecks(ctx context.Context, tx *sql.Tx, now time.Time, domains []seedDomain) error {
	for _, d := range domains {
		lastChecked := now.Add(-2 * time.Hour)
		if d.State == "paused" {
			lastChecked = now.AddDate(0, 0, -1)
		}
		if d.State == "archived" {
			lastChecked = now.AddDate(0, 0, -20)
		}
		for attempt := 0; attempt < 2; attempt++ {
			started := lastChecked.Add(-time.Duration(attempt) * 24 * time.Hour)
			checkID := fmt.Sprintf("%s-check-%d", d.ID, attempt+1)
			source := "rdap"
			if d.Name == "example.org" {
				source = "whois"
			}
			status := d.LastStatus
			if status == "" {
				status = "ok"
			}
			var expires any
			if d.ExpiryDays != nil {
				expires = dbTime(demoDate(now, *d.ExpiryDays))
			}
			var httpStatus any
			switch status {
			case "ok":
				httpStatus = 200
			case "error":
				httpStatus = 500
			}
			errorCode := any(nil)
			errorMessage := any(nil)
			if status != "ok" {
				errorCode = "expiry_not_found"
				errorMessage = d.LastError
				if d.LastError == "" {
					errorMessage = "No expiry date found"
				}
			}
			durationMS := 180 + attempt*60
			_, err := tx.ExecContext(ctx, `
				INSERT INTO domain_checks (id, domain_id, source, status, http_status, expires_at,
					registrar, confidence, error_code, error_message, duration_ms, started_at,
					finished_at, created_at)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				checkID, d.ID, source, status, httpStatus, expires, d.Registrar, d.Confidence,
				errorCode, errorMessage, durationMS, dbTime(started),
				dbTime(started.Add(time.Duration(durationMS)*time.Millisecond)), dbTime(started))
			if err != nil {
				return fmt.Errorf("seed check for %s: %w", d.Name, err)
			}
			if attempt == 0 {
				raw, err := json.Marshal(map[string]any{
					"demo": true, "domain": d.Name, "source": source, "status": status,
				})
				if err != nil {
					return err
				}
				if _, err := tx.ExecContext(ctx, `
					INSERT INTO check_raw_responses (check_id, encoding, body, content_type, created_at, expires_at)
					VALUES (?, 'identity', ?, ?, ?, ?)`,
					checkID, raw, "application/json", dbTime(started), dbTime(now.AddDate(0, 0, 7))); err != nil {
					return fmt.Errorf("seed raw response for %s: %w", d.Name, err)
				}
			}
		}
	}
	return nil
}

type seedChannel struct {
	ID      string
	Name    string
	Type    string
	Config  any
	Enabled bool
	System  bool
	Muted   bool
}

func (s *Seeder) seedChannels(ctx context.Context, tx *sql.Tx, now time.Time) error {
	channels := []seedChannel{
		{
			ID: "00000000-0000-7000-8000-000000000201", Name: "Ops email", Type: "email",
			Config:  notify.EmailConfig{Recipients: []string{"ops@example.com", "owner@example.com"}},
			Enabled: true, System: true,
		},
		{
			ID: "00000000-0000-7000-8000-000000000202", Name: "Status page webhook", Type: "webhook",
			Config:  notify.WebhookConfig{URL: "https://hooks.demo.invalid/iwmmd-demo", Secret: "demo-webhook-secret", Headers: map[string]string{"X-Demo": "true"}},
			Enabled: true,
		},
		{
			ID: "00000000-0000-7000-8000-000000000203", Name: "Team Discord", Type: "discord",
			Config:  notify.DiscordConfig{WebhookURL: "https://discord.com/api/webhooks/123456789012345678/demo-token", Username: "IWMMD Demo"},
			Enabled: true,
		},
		{
			ID: "00000000-0000-7000-8000-000000000204", Name: "On-call Telegram", Type: "telegram",
			Config:  notify.TelegramConfig{BotToken: "123456789:demo-token", ChatID: "-1001234567890"},
			Enabled: false, Muted: true,
		},
	}
	for _, channel := range channels {
		body, err := notify.EncodeConfig(channel.Config)
		if err != nil {
			return err
		}
		ciphertext, err := s.Box.Encrypt(body)
		if err != nil {
			return err
		}
		var mutedUntil any
		var muteReason any
		if channel.Muted {
			mutedUntil = dbTime(now.AddDate(0, 0, 7))
			muteReason = "On-call rotation paused"
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO channels (id, name, type, config_ciphertext, enabled, receive_system_alerts,
				muted_until, mute_reason, created_by, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			channel.ID, channel.Name, channel.Type, ciphertext, boolInt(channel.Enabled),
			boolInt(channel.System), mutedUntil, muteReason, UserID,
			dbTime(now.AddDate(0, 0, -60)), dbTime(now.AddDate(0, 0, -30))); err != nil {
			return fmt.Errorf("seed channel %s: %w", channel.Name, err)
		}
	}
	return nil
}

type seedRule struct {
	ID          string
	Name        string
	Scope       string
	DomainID    string
	OffsetValue int
	OffsetUnit  string
	Direction   string
	Channels    []string
}

func (s *Seeder) seedRules(ctx context.Context, tx *sql.Tx, now time.Time, domains []seedDomain) error {
	domainID := ""
	for _, d := range domains {
		if d.Name == "example.org" {
			domainID = d.ID
		}
	}
	rules := []seedRule{
		{
			ID: "00000000-0000-7000-8000-000000000301", Name: "30 days before expiry",
			Scope: "global", OffsetValue: 30, OffsetUnit: "days", Direction: "before",
			Channels: []string{"00000000-0000-7000-8000-000000000201", "00000000-0000-7000-8000-000000000202"},
		},
		{
			ID: "00000000-0000-7000-8000-000000000302", Name: "7 days before expiry",
			Scope: "global", OffsetValue: 7, OffsetUnit: "days", Direction: "before",
			Channels: []string{"00000000-0000-7000-8000-000000000201", "00000000-0000-7000-8000-000000000203"},
		},
		{
			ID: "00000000-0000-7000-8000-000000000303", Name: "1 day after expiry",
			Scope: "global", OffsetValue: 1, OffsetUnit: "days", Direction: "after",
			Channels: []string{"00000000-0000-7000-8000-000000000201"},
		},
		{
			ID: "00000000-0000-7000-8000-000000000304", Name: "60 days before expiry",
			Scope: "domain", DomainID: domainID, OffsetValue: 60, OffsetUnit: "days", Direction: "before",
			Channels: []string{"00000000-0000-7000-8000-000000000201"},
		},
	}
	for _, rule := range rules {
		var scopedDomain any
		if rule.DomainID != "" {
			scopedDomain = rule.DomainID
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO alert_rules (id, name, scope, domain_id, offset_value, offset_unit,
				direction, send_time_local, enabled, created_by, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, '09:00', 1, ?, ?, ?)`,
			rule.ID, rule.Name, rule.Scope, scopedDomain, rule.OffsetValue, rule.OffsetUnit,
			rule.Direction, UserID, dbTime(now.AddDate(0, 0, -45)), dbTime(now.AddDate(0, 0, -20))); err != nil {
			return fmt.Errorf("seed rule %s: %w", rule.Name, err)
		}
		for _, channelID := range rule.Channels {
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO alert_rule_channels (rule_id, channel_id) VALUES (?, ?)`,
				rule.ID, channelID); err != nil {
				return fmt.Errorf("seed rule channel %s: %w", rule.Name, err)
			}
		}
	}
	return nil
}

func (s *Seeder) seedIncidents(ctx context.Context, tx *sql.Tx, now time.Time, domains []seedDomain) error {
	byName := map[string]seedDomain{}
	for _, d := range domains {
		byName[d.Name] = d
	}
	incidents := []struct {
		ID       string
		Domain   string
		Kind     string
		State    string
		OpenedAt time.Time
		Resolved *time.Time
		LastSeen time.Time
		Details  string
	}{
		{
			ID: "00000000-0000-7000-8000-000000000401", Domain: "acme-demo.net",
			Kind: "parse_failure", State: "open", OpenedAt: now.AddDate(0, 0, -3),
			LastSeen: now.Add(-30 * time.Minute), Details: `{"source":"rdap","error_code":"expiry_not_found"}`,
		},
		{
			ID: "00000000-0000-7000-8000-000000000402", Domain: "acme-demo.net",
			Kind: "expiry_unknown", State: "open", OpenedAt: now.AddDate(0, 0, -3),
			LastSeen: now.Add(-25 * time.Minute), Details: `{"reason":"no_expiry_event"}`,
		},
		{
			ID: "00000000-0000-7000-8000-000000000403", Domain: "example.net",
			Kind: "check_failure", State: "resolved", OpenedAt: now.AddDate(0, 0, -12),
			Resolved: timePtr(now.AddDate(0, 0, -11)), LastSeen: now.AddDate(0, 0, -11),
			Details: `{"source":"rdap","error_code":"timeout"}`,
		},
	}
	for _, incident := range incidents {
		d, ok := byName[incident.Domain]
		if !ok {
			return fmt.Errorf("seed incident: missing domain %s", incident.Domain)
		}
		var resolved any
		if incident.Resolved != nil {
			resolved = dbTime(*incident.Resolved)
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO incidents (id, domain_id, kind, state, opened_at, resolved_at, last_seen_at,
				details_json, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			incident.ID, d.ID, incident.Kind, incident.State, dbTime(incident.OpenedAt),
			resolved, dbTime(incident.LastSeen), incident.Details, dbTime(incident.OpenedAt),
			dbTime(incident.LastSeen)); err != nil {
			return fmt.Errorf("seed incident %s: %w", incident.ID, err)
		}
	}
	return nil
}

type seedNotification struct {
	ID          string
	Kind        string
	Domain      string
	IncidentID  string
	RuleID      string
	ChannelID   string
	State       string
	ScheduledAt time.Time
	NextAttempt *time.Time
	Attempts    int
	LastError   string
	DedupeKey   string
	SentAt      *time.Time
	Message     string
}

func (s *Seeder) seedNotifications(ctx context.Context, tx *sql.Tx, now time.Time, domains []seedDomain) error {
	byName := map[string]seedDomain{}
	for _, d := range domains {
		byName[d.Name] = d
	}
	rules := map[string]string{
		"30d": "00000000-0000-7000-8000-000000000301",
		"7d":  "00000000-0000-7000-8000-000000000302",
		"1da": "00000000-0000-7000-8000-000000000303",
		"60d": "00000000-0000-7000-8000-000000000304",
	}
	channels := map[string]string{
		"email":   "00000000-0000-7000-8000-000000000201",
		"discord": "00000000-0000-7000-8000-000000000203",
	}
	incidentID := "00000000-0000-7000-8000-000000000401"
	exampleOrg := byName["example.org"]
	exampleNet := byName["example.net"]
	exampleCom := byName["example.com"]
	northwind := byName["northwind-demo.com"]
	paused := byName["paused-demo.org"]
	archived := byName["archived-demo.com"]
	labs := byName["labs-demo.io"]

	notifications := []seedNotification{
		{
			ID: "00000000-0000-7000-8000-000000000501", Kind: "expiry", Domain: exampleCom.Name,
			RuleID: rules["7d"], ChannelID: channels["email"], State: "sent",
			ScheduledAt: now.AddDate(0, 0, -1), Attempts: 1, DedupeKey: "demo-history-com-email",
			SentAt: timePtr(now.AddDate(0, 0, -1).Add(time.Minute)),
		},
		{
			ID: "00000000-0000-7000-8000-000000000502", Kind: "expiry", Domain: exampleCom.Name,
			RuleID: rules["7d"], ChannelID: channels["discord"], State: "sent",
			ScheduledAt: now.AddDate(0, 0, -1), Attempts: 1, DedupeKey: "demo-history-com-discord",
			SentAt: timePtr(now.AddDate(0, 0, -1).Add(2 * time.Minute)),
		},
		{
			ID: "00000000-0000-7000-8000-000000000503", Kind: "expiry", Domain: exampleNet.Name,
			RuleID: rules["30d"], ChannelID: channels["email"], State: "failed",
			ScheduledAt: now.AddDate(0, 0, -9), Attempts: 3,
			LastError: "SMTP connection timed out", DedupeKey: "demo-history-net-failed",
		},
		{
			ID: "00000000-0000-7000-8000-000000000504", Kind: "expiry", Domain: northwind.Name,
			RuleID: rules["1da"], ChannelID: channels["email"], State: "sent",
			ScheduledAt: now.AddDate(0, 0, -3), Attempts: 1, DedupeKey: "demo-history-northwind-email",
			SentAt: timePtr(now.AddDate(0, 0, -3).Add(time.Minute)),
		},
		{
			ID: "00000000-0000-7000-8000-000000000505", Kind: "system", Domain: byName["acme-demo.net"].Name,
			IncidentID: incidentID, ChannelID: channels["email"], State: "sent",
			ScheduledAt: now.AddDate(0, 0, -3), Attempts: 1, DedupeKey: "demo-history-acme-parse",
			SentAt:  timePtr(now.AddDate(0, 0, -3).Add(time.Minute)),
			Message: "Expiry lookup needs attention.",
		},
		{
			ID: "00000000-0000-7000-8000-000000000506", Kind: "expiry", Domain: paused.Name,
			RuleID: rules["30d"], ChannelID: channels["email"], State: "suppressed",
			ScheduledAt: now.AddDate(0, 0, -1), DedupeKey: "demo-history-paused-suppressed",
			LastError: "Domain paused",
		},
		{
			ID: "00000000-0000-7000-8000-000000000507", Kind: "expiry", Domain: archived.Name,
			RuleID: rules["30d"], ChannelID: channels["email"], State: "canceled",
			ScheduledAt: now.AddDate(0, 0, -10), DedupeKey: "demo-history-archived-canceled",
			LastError: "Domain archived",
		},
		{
			ID: "00000000-0000-7000-8000-000000000508", Kind: "expiry", Domain: exampleOrg.Name,
			RuleID: rules["60d"], ChannelID: channels["email"], State: "pending",
			ScheduledAt: now.AddDate(0, 0, 15), NextAttempt: timePtr(now.AddDate(0, 0, 15)),
			DedupeKey: "demo-history-org-60d",
		},
		{
			ID: "00000000-0000-7000-8000-000000000509", Kind: "expiry", Domain: exampleOrg.Name,
			RuleID: rules["30d"], ChannelID: channels["email"], State: "pending",
			ScheduledAt: now.AddDate(0, 0, 45), NextAttempt: timePtr(now.AddDate(0, 0, 45)),
			DedupeKey: "demo-history-org-30d",
		},
		{
			ID: "00000000-0000-7000-8000-000000000510", Kind: "expiry", Domain: labs.Name,
			RuleID: rules["7d"], ChannelID: channels["discord"], State: "sent",
			ScheduledAt: now.AddDate(0, 0, -5), Attempts: 1, DedupeKey: "demo-history-labs-discord",
			SentAt: timePtr(now.AddDate(0, 0, -5).Add(time.Minute)),
		},
		{
			ID: "00000000-0000-7000-8000-000000000511", Kind: "expiry", Domain: exampleNet.Name,
			RuleID: rules["7d"], ChannelID: channels["email"], State: "pending",
			ScheduledAt: now.AddDate(0, 0, -1), NextAttempt: timePtr(now.AddDate(0, 0, 2)),
			DedupeKey: "demo-history-net-muted",
		},
	}

	for _, item := range notifications {
		d := byName[item.Domain]
		payload := alert.Payload{
			Event: "domain.expiring", Kind: "expiry", DomainID: d.ID, Domain: d.Name,
			URL: "https://demo.iwmmd.local/domains/" + d.ID, Rule: ruleName(rules, item.RuleID),
		}
		if d.ExpiryDays != nil {
			expiry := demoDate(now, *d.ExpiryDays)
			payload.Expiry = expiry.Format("2006-01-02")
			payload.DaysLeft = calendarDays(now, expiry)
		}
		if item.Kind == "system" {
			payload.Kind = "system"
			payload.Event = "system.incident"
			payload.IncidentID = item.IncidentID
			payload.IncidentKind = "parse_failure"
			payload.Message = item.Message
		}
		payloadJSON, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		var expiryDate any
		if payload.Expiry != "" {
			expiryDate = payload.Expiry
		}
		var incident any
		if item.IncidentID != "" {
			incident = item.IncidentID
		}
		var ruleID any
		if item.RuleID != "" {
			ruleID = item.RuleID
		}
		var channelID any
		if item.ChannelID != "" {
			channelID = item.ChannelID
		}
		var nextAttempt any
		if item.NextAttempt != nil {
			nextAttempt = dbTime(*item.NextAttempt)
		}
		var sentAt any
		if item.SentAt != nil {
			sentAt = dbTime(*item.SentAt)
		}
		var lastError any
		if item.LastError != "" {
			lastError = item.LastError
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO notifications (id, kind, domain_id, incident_id, rule_id, channel_id,
				expiry_date_utc, dedupe_key, state, scheduled_at, next_attempt_at, attempts,
				last_error, payload_json, created_at, updated_at, sent_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			item.ID, item.Kind, d.ID, incident, ruleID, channelID, expiryDate,
			item.DedupeKey, item.State, dbTime(item.ScheduledAt), nextAttempt, item.Attempts,
			lastError, string(payloadJSON), dbTime(notificationCreated(now, item)), dbTime(notificationUpdated(now, item)), sentAt); err != nil {
			return fmt.Errorf("seed notification %s: %w", item.ID, err)
		}
		if item.Attempts > 0 {
			for attempt := 1; attempt <= item.Attempts; attempt++ {
				status := "success"
				errorText := any(nil)
				responseCode := any(202)
				if item.State == "failed" {
					responseCode = 500
					if attempt < item.Attempts {
						status = "retry"
					} else {
						status = "failed"
						errorText = item.LastError
					}
				}
				started := item.ScheduledAt.Add(time.Duration(attempt) * time.Minute)
				if _, err := tx.ExecContext(ctx, `
					INSERT INTO delivery_attempts (id, notification_id, attempt_no, started_at,
						finished_at, status, response_code, error, created_at)
					VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
					fmt.Sprintf("%s-attempt-%d", item.ID, attempt), item.ID, attempt,
					dbTime(started), dbTime(started.Add(time.Second)), status, responseCode,
					errorText, dbTime(started)); err != nil {
					return fmt.Errorf("seed delivery attempt %s: %w", item.ID, err)
				}
			}
		}
	}
	return nil
}

func (s *Seeder) seedAudit(ctx context.Context, tx *sql.Tx, now time.Time) error {
	entries := []struct {
		ID       string
		Action   string
		Entity   string
		EntityID string
		Before   string
		After    string
		At       time.Time
	}{
		{
			ID: "00000000-0000-7000-8000-000000000601", Action: "domain.create", Entity: "domain",
			EntityID: "00000000-0000-7000-8000-000000000101", After: `{"name":"example.com"}`, At: now.AddDate(0, 0, -120),
		},
		{
			ID: "00000000-0000-7000-8000-000000000602", Action: "domain.update", Entity: "domain",
			EntityID: "00000000-0000-7000-8000-000000000101", Before: `{"notes":""}`,
			After: `{"notes":"Primary marketing domain."}`, At: now.AddDate(0, 0, -30),
		},
		{
			ID: "00000000-0000-7000-8000-000000000603", Action: "alert_rule.create", Entity: "alert_rule",
			EntityID: "00000000-0000-7000-8000-000000000302", After: `{"name":"7 days before expiry"}`, At: now.AddDate(0, 0, -45),
		},
		{
			ID: "00000000-0000-7000-8000-000000000604", Action: "channel.create", Entity: "channel",
			EntityID: "00000000-0000-7000-8000-000000000201", After: `{"name":"Ops email","type":"email"}`, At: now.AddDate(0, 0, -60),
		},
		{
			ID: "00000000-0000-7000-8000-000000000605", Action: "settings.smtp", Entity: "settings",
			After: `{"host":"smtp.demo.invalid"}`, At: now.AddDate(0, 0, -30),
		},
		{
			ID: "00000000-0000-7000-8000-000000000606", Action: "auth.login", Entity: "user",
			EntityID: UserID, After: `{"email":"demo@iwmmd.local"}`, At: now.Add(-2 * time.Hour),
		},
	}
	for _, entry := range entries {
		var entityID any
		if entry.EntityID != "" {
			entityID = entry.EntityID
		}
		var before any
		if entry.Before != "" {
			before = entry.Before
		}
		var after any
		if entry.After != "" {
			after = entry.After
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO audit_log (id, actor_user_id, action, entity_type, entity_id,
				before_json, after_json, ip, user_agent, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, '203.0.113.42', 'Mozilla/5.0 (Demo)', ?)`,
			entry.ID, UserID, entry.Action, entry.Entity, entityID, before, after, dbTime(entry.At)); err != nil {
			return fmt.Errorf("seed audit %s: %w", entry.ID, err)
		}
	}
	return nil
}

func seedMarker(ctx context.Context, tx *sql.Tx, now time.Time) error {
	body, err := json.Marshal(map[string]int{"version": 1})
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO settings (key, value_json, is_secret, updated_by, updated_at)
		VALUES (?, ?, 0, ?, ?)
		ON CONFLICT(key) DO UPDATE SET value_json = excluded.value_json,
			updated_by = excluded.updated_by, updated_at = excluded.updated_at`,
		markerKey, string(body), UserID, dbTime(now))
	return err
}

func randomToken() string {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("demo-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(buf)
}

func demoDate(now time.Time, days int) time.Time {
	base := time.Date(now.Year(), now.Month(), now.Day(), 12, 0, 0, 0, time.UTC)
	return base.AddDate(0, 0, days)
}

func calendarDays(now, target time.Time) int {
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	day := time.Date(target.Year(), target.Month(), target.Day(), 0, 0, 0, 0, time.UTC)
	return int(day.Sub(today).Hours() / 24)
}

func notificationCreated(now time.Time, item seedNotification) time.Time {
	if item.ScheduledAt.After(now) {
		return now
	}
	return item.ScheduledAt
}

func notificationUpdated(now time.Time, item seedNotification) time.Time {
	if item.SentAt != nil {
		return *item.SentAt
	}
	if item.ScheduledAt.After(now) {
		return now
	}
	if item.Attempts > 0 {
		return item.ScheduledAt.Add(time.Duration(item.Attempts)*time.Minute + time.Second)
	}
	return item.ScheduledAt
}

func ruleName(rules map[string]string, id string) string {
	for key, value := range rules {
		if value != id {
			continue
		}
		switch key {
		case "30d":
			return "30 days before expiry"
		case "7d":
			return "7 days before expiry"
		case "1da":
			return "1 day after expiry"
		case "60d":
			return "60 days before expiry"
		}
	}
	return ""
}

func dbTime(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func intPtr(value int) *int { return &value }

func timePtr(value time.Time) *time.Time { return &value }
