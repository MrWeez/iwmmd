package domain

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"strings"
	"time"

	"github.com/mrweez/iwmmd/internal/alert"
	"github.com/mrweez/iwmmd/internal/calendar"
	"github.com/mrweez/iwmmd/internal/checker/model"
	"github.com/mrweez/iwmmd/internal/platform/ids"
	"github.com/mrweez/iwmmd/internal/settings"
	"github.com/mrweez/iwmmd/internal/store"
)

var (
	// ErrRateLimited means a manual check was requested too soon.
	ErrRateLimited = errors.New("manual check rate limit reached")
	// ErrInvalidState means the requested domain state is not allowed.
	ErrInvalidState = errors.New("invalid domain state")
)

// Checker performs one domain lookup.
type Checker interface {
	Check(ctx context.Context, name string) model.Result
}

// Service owns domain lifecycle and check persistence.
type Service struct {
	Store    *store.Store
	Checker  Checker
	Settings *settings.Service
	Alerts   *alert.Engine
	Logger   *slog.Logger
}

// CreateInput is a domain create request.
type CreateInput struct {
	Name  string `json:"name"`
	Notes string `json:"notes,omitempty"`
}

// UpdateInput is a domain update request.
type UpdateInput struct {
	Version              int     `json:"version"`
	Registrar            *string `json:"registrar,omitempty"`
	Notes                *string `json:"notes,omitempty"`
	InheritGlobalAlerts  *bool   `json:"inherit_global_alerts,omitempty"`
	CheckIntervalSeconds int     `json:"check_interval_seconds,omitempty"`
	State                string  `json:"state,omitempty"`
}

// Create adds a domain and queues its first check.
func (s *Service) Create(ctx context.Context, in CreateInput, userID string, now time.Time) (store.Domain, error) {
	normalized, err := Normalize(in.Name)
	if err != nil {
		return store.Domain{}, err
	}
	snap, err := s.Settings.Get(ctx)
	if err != nil {
		return store.Domain{}, err
	}
	interval := snap.Check.DefaultIntervalSeconds
	if interval < 300 {
		interval = 86400
	}
	d := store.Domain{
		ID:                   ids.New(),
		NameACE:              normalized.ACE,
		NameUnicode:          normalized.Unicode,
		State:                "active",
		InheritGlobalAlerts:  true,
		CheckIntervalSeconds: interval,
		NextCheckAt:          &now,
		ExpiryConfidence:     "unknown",
		Version:              1,
		Notes:                strings.TrimSpace(in.Notes),
		CreatedBy:            userID,
		CreatedAt:            now,
		UpdatedAt:            now,
	}
	if err := s.Store.CreateDomain(ctx, d); err != nil {
		return store.Domain{}, err
	}
	_, _, _ = s.enqueueCheck(ctx, d.ID, now)
	return d, nil
}

// Update changes user-editable domain fields.
func (s *Service) Update(ctx context.Context, id string, in UpdateInput, now time.Time) (store.Domain, error) {
	current, err := s.Store.GetDomain(ctx, id)
	if err != nil {
		return store.Domain{}, err
	}
	if current.State == "archived" {
		return store.Domain{}, ErrInvalidState
	}
	if in.Version != 0 && in.Version != current.Version {
		return store.Domain{}, store.ErrConflict
	}
	if in.State != "" && in.State != "active" && in.State != "paused" {
		return store.Domain{}, ErrInvalidState
	}
	interval := current.CheckIntervalSeconds
	if in.CheckIntervalSeconds != 0 {
		if in.CheckIntervalSeconds < 300 {
			return store.Domain{}, errors.New("check interval must be at least 300 seconds")
		}
		interval = in.CheckIntervalSeconds
	}
	if in.Registrar != nil {
		current.Registrar = strings.TrimSpace(*in.Registrar)
	}
	if in.Notes != nil {
		current.Notes = strings.TrimSpace(*in.Notes)
	}
	current.CheckIntervalSeconds = interval
	if in.InheritGlobalAlerts != nil {
		current.InheritGlobalAlerts = *in.InheritGlobalAlerts
	}
	wasPaused := current.State == "paused"
	if in.State != "" {
		current.State = in.State
	}
	if current.State == "paused" {
		current.NextCheckAt = nil
	} else if wasPaused || current.NextCheckAt == nil {
		current.NextCheckAt = &now
	}
	if err := s.Store.UpdateDomain(ctx, current); err != nil {
		return store.Domain{}, err
	}
	updated, err := s.Store.GetDomain(ctx, id)
	if err != nil {
		return store.Domain{}, err
	}
	if !updated.InheritGlobalAlerts {
		if err := s.Store.CancelPendingGlobalNotificationsForDomain(ctx, id, now); err != nil {
			return store.Domain{}, err
		}
	}
	if err := s.Alerts.ReconcileDomain(ctx, updated, now); err != nil {
		return store.Domain{}, err
	}
	if updated.State == "active" {
		_, _, _ = s.enqueueCheck(ctx, id, now)
	}
	return updated, nil
}

// Archive archives a domain.
func (s *Service) Archive(ctx context.Context, id string, now time.Time) error {
	if err := s.Store.ArchiveDomain(ctx, id, now); err != nil {
		return err
	}
	return s.Store.CancelJobsByKey(ctx, "domain-check:"+id, now)
}

// Restore returns an archived domain to the active watchlist.
func (s *Service) Restore(ctx context.Context, id string, now time.Time) (store.Domain, error) {
	if err := s.Store.RestoreDomain(ctx, id, now); err != nil {
		return store.Domain{}, err
	}
	d, err := s.Store.GetDomain(ctx, id)
	if err != nil {
		return store.Domain{}, err
	}
	if err := s.Alerts.ReconcileDomain(ctx, d, now); err != nil {
		s.logger().Error("reconcile restored domain", "domain_id", id, "error", err)
	}
	if _, _, err := s.enqueueCheck(ctx, id, now); err != nil {
		s.logger().Error("enqueue restored domain check", "domain_id", id, "error", err)
	}
	return d, nil
}

// DeletePermanently removes an archived domain and all of its dependent data.
func (s *Service) DeletePermanently(ctx context.Context, id string) error {
	d, err := s.Store.GetDomain(ctx, id)
	if err != nil {
		return err
	}
	if d.State != "archived" {
		return ErrInvalidState
	}
	return s.Store.DeleteDomainPermanent(ctx, id)
}

// Mute holds expiry notifications for a domain. A nil until clears it.
func (s *Service) Mute(ctx context.Context, id string, until *time.Time, reason string, now time.Time) (store.Domain, error) {
	if until != nil && !until.After(now) {
		until = nil
	}
	if err := s.Store.MuteDomain(ctx, id, until, reason, now); err != nil {
		return store.Domain{}, err
	}
	d, err := s.Store.GetDomain(ctx, id)
	if err != nil {
		return store.Domain{}, err
	}
	if until == nil {
		if err := s.Alerts.ReconcileDomain(ctx, d, now); err != nil {
			return store.Domain{}, err
		}
	}
	return d, nil
}

// ManualCheck queues a check while enforcing the manual rate limit.
func (s *Service) ManualCheck(ctx context.Context, id string, now time.Time) (bool, error) {
	d, err := s.Store.GetDomain(ctx, id)
	if err != nil {
		return false, err
	}
	if d.State != "active" {
		return false, ErrInvalidState
	}
	snap, err := s.Settings.Get(ctx)
	if err != nil {
		return false, err
	}
	limit := time.Duration(snap.Check.ManualRateLimitSeconds) * time.Second
	if limit <= 0 {
		limit = 5 * time.Minute
	}
	if d.LastCheckedAt != nil && now.Sub(*d.LastCheckedAt) < limit {
		_, active, err := s.Store.FindActiveJob(ctx, "domain-check:"+id)
		if err != nil {
			return false, err
		}
		if active {
			return true, nil
		}
		return false, ErrRateLimited
	}
	_, created, err := s.enqueueCheck(ctx, id, now)
	if err != nil {
		return false, err
	}
	if !created {
		return true, nil
	}
	return true, nil
}

// CheckAndRecord performs one check and persists the result.
func (s *Service) CheckAndRecord(ctx context.Context, domainID string, now time.Time) (store.CheckOutcome, error) {
	d, err := s.Store.GetDomain(ctx, domainID)
	if err != nil {
		return store.CheckOutcome{}, err
	}
	if d.State != "active" {
		return store.CheckOutcome{}, ErrInvalidState
	}
	started := time.Now().UTC()
	result := s.Checker.Check(ctx, d.NameACE)
	finished := time.Now().UTC()
	return s.Record(ctx, d, result, started, finished)
}

// Record persists a checker result and updates alerts and incidents.
func (s *Service) Record(ctx context.Context, d store.Domain, result model.Result, started, finished time.Time) (store.CheckOutcome, error) {
	snap, err := s.Settings.Get(ctx)
	if err != nil {
		return store.CheckOutcome{}, err
	}
	source := "rdap"
	if result.Source == "whois" {
		source = "whois"
	}
	status := result.Status
	if status == "" {
		status = "error"
	}
	confidence := result.Confidence
	if confidence == "" {
		confidence = "unknown"
	}
	incidentKind := ""
	if status == "unsupported" {
		incidentKind = "expiry_unknown"
	} else if status == "error" {
		if result.ErrorCode == "parse" || result.ErrorCode == "expiry_not_found" {
			incidentKind = "parse_failure"
		} else {
			incidentKind = "check_failure"
		}
	}
	next := s.nextCheck(d, result, snap, finished)
	rawExpires := finished.AddDate(0, 0, snap.Retention.RawDays)
	rec := store.CheckRecord{
		DomainID:       d.ID,
		Source:         source,
		Status:         status,
		HTTPStatus:     result.HTTPStatus,
		ExpiresAt:      result.ExpiresAt,
		Registrar:      result.Registrar,
		Confidence:     confidence,
		ErrorCode:      result.ErrorCode,
		ErrorMessage:   result.ErrorMessage,
		DurationMS:     int(result.Duration / time.Millisecond),
		StartedAt:      started,
		FinishedAt:     finished,
		Raw:            result.Raw,
		RawContentType: result.ContentType,
		RawExpiresAt:   &rawExpires,
		NextCheckAt:    next,
		IncidentKind:   incidentKind,
	}
	outcome, err := s.Store.RecordCheck(ctx, rec)
	if err != nil {
		return store.CheckOutcome{}, err
	}
	if err := s.Alerts.GenerateDomain(ctx, outcome.Domain, finished); err != nil {
		s.logger().Error("generate alerts after check", "domain_id", d.ID, "error", err)
	}
	for _, incident := range outcome.Opened {
		message := fmt.Sprintf("%s incident opened for %s", incident.Kind, d.NameUnicode)
		if result.ErrorMessage != "" {
			message += ": " + result.ErrorMessage
		}
		if err := s.Alerts.SystemEvent(ctx, incident, outcome.Domain, "opened", message, finished); err != nil {
			s.logger().Error("queue system alert", "incident_id", incident.ID, "error", err)
		}
	}
	for _, incident := range outcome.Resolved {
		message := fmt.Sprintf("%s incident resolved for %s", incident.Kind, d.NameUnicode)
		if err := s.Alerts.SystemEvent(ctx, incident, outcome.Domain, "resolved", message, finished); err != nil {
			s.logger().Error("queue recovery alert", "incident_id", incident.ID, "error", err)
		}
	}
	return outcome, nil
}

func (s *Service) enqueueCheck(ctx context.Context, domainID string, now time.Time) (store.Job, bool, error) {
	payload, err := json.Marshal(map[string]string{"domain_id": domainID})
	if err != nil {
		return store.Job{}, false, err
	}
	return s.Store.EnqueueJob(ctx, store.EnqueueJobInput{
		Kind:        "domain.check",
		PayloadJSON: string(payload),
		RunAt:       now,
		MaxAttempts: 5,
		UniqueKey:   "domain-check:" + domainID,
	})
}

func (s *Service) nextCheck(d store.Domain, result model.Result, snap settings.Snapshot, now time.Time) *time.Time {
	next := now.Add(time.Duration(snap.Check.DefaultIntervalSeconds) * time.Second)
	if result.Status == "ok" && result.ExpiresAt != nil {
		days := calendar.CalendarDaysBetween(now, *result.ExpiresAt)
		switch {
		case days <= 7:
			next = now.Add(6 * time.Hour)
		case days <= 30:
			next = now.Add(12 * time.Hour)
		}
	} else if result.Status == "error" {
		failures := d.ConsecutiveFailures + 1
		if failures >= 3 {
			next = now.Add(6 * time.Hour)
		} else {
			next = now.Add(time.Duration(failures) * 30 * time.Minute)
		}
	}
	jitter := time.Duration(float64(next.Sub(now)) * 0.1)
	if jitter > 0 {
		next = next.Add(time.Duration(rand.Int64N(int64(jitter)*2+1)) - jitter)
	}
	return &next
}

func (s *Service) logger() *slog.Logger {
	if s.Logger != nil {
		return s.Logger
	}
	return slog.Default()
}
