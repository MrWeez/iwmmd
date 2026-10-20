// Package alert evaluates expiry rules and queues notification deliveries.
package alert

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/mrweez/iwmmd/internal/calendar"
	"github.com/mrweez/iwmmd/internal/settings"
	"github.com/mrweez/iwmmd/internal/store"
)

// Engine turns alert rules into durable notifications.
type Engine struct {
	Store    *store.Store
	Settings *settings.Service
	BaseURL  string
	Logger   *slog.Logger
}

// Payload is stored with each notification for channel formatting.
type Payload struct {
	Event        string `json:"event"`
	Kind         string `json:"kind"`
	DomainID     string `json:"domain_id,omitempty"`
	Domain       string `json:"domain,omitempty"`
	Expiry       string `json:"expiry,omitempty"`
	DaysLeft     int    `json:"days_left,omitempty"`
	OffsetValue  int    `json:"offset_value,omitempty"`
	OffsetUnit   string `json:"offset_unit,omitempty"`
	Direction    string `json:"direction,omitempty"`
	DueAt        string `json:"due_at,omitempty"`
	Rule         string `json:"rule,omitempty"`
	IncidentID   string `json:"incident_id,omitempty"`
	IncidentKind string `json:"incident_kind,omitempty"`
	SystemEvent  string `json:"system_event,omitempty"`
	Message      string `json:"message,omitempty"`
	URL          string `json:"url,omitempty"`
}

// GenerateDomain schedules every applicable alert for a domain.
func (e *Engine) GenerateDomain(ctx context.Context, d store.Domain, now time.Time) error {
	if d.State != "active" || d.ExpiresAt == nil {
		return nil
	}
	snap, err := e.Settings.Get(ctx)
	if err != nil {
		return err
	}
	loc, err := snap.Alerts.Location()
	if err != nil {
		return fmt.Errorf("resolve alert timezone: %w", err)
	}
	rules, err := e.Store.RulesForDomain(ctx, d.ID, d.InheritGlobalAlerts)
	if err != nil {
		return err
	}
	channels, err := e.Store.ListChannels(ctx, false)
	if err != nil {
		return err
	}
	channelByID := make(map[string]store.Channel, len(channels))
	for _, channel := range channels {
		channelByID[channel.ID] = channel
	}
	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}
		sendTime := rule.SendTimeLocal
		if sendTime == "" {
			sendTime = snap.Alerts.DefaultTime
		}
		schedule, err := calendar.Due(*d.ExpiresAt, rule.OffsetValue, rule.OffsetUnit, rule.Direction, sendTime, loc)
		if err != nil {
			e.logger().Warn("skip alert rule", "rule_id", rule.ID, "error", err)
			continue
		}
		payload := Payload{
			Event:       "domain.expiring",
			Kind:        "expiry",
			DomainID:    d.ID,
			Domain:      d.NameUnicode,
			Expiry:      d.ExpiresAt.UTC().Format("2006-01-02"),
			DaysLeft:    calendar.CalendarDaysBetween(now, *d.ExpiresAt),
			OffsetValue: rule.OffsetValue,
			OffsetUnit:  rule.OffsetUnit,
			Direction:   rule.Direction,
			DueAt:       schedule.DueAt.Format(time.RFC3339),
			Rule:        rule.Name,
			URL:         e.BaseURL + "/domains/" + d.ID,
		}
		body, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		for _, channelID := range rule.ChannelIDs {
			channel, ok := channelByID[channelID]
			if !ok || !channel.Enabled {
				continue
			}
			key := expiryDedupeKey(d.ID, channelID, schedule.ExpiryUTC, rule.OffsetValue, rule.OffsetUnit, rule.Direction)
			n, _, err := e.Store.EnqueueNotification(ctx, store.NotificationInput{
				Kind:          "expiry",
				DomainID:      d.ID,
				RuleID:        rule.ID,
				ChannelID:     channelID,
				ExpiryDate:    schedule.ExpiryUTC,
				DedupeKey:     key,
				ScheduledAt:   schedule.DueAt,
				PayloadJSON:   string(body),
				UpdatePending: true,
				Reactivate:    true,
			})
			if err != nil {
				return err
			}
			if err := e.applyHold(ctx, n, d, channel, snap, now); err != nil {
				return err
			}
		}
	}
	return nil
}

// GenerateAll schedules alerts for every active domain.
func (e *Engine) GenerateAll(ctx context.Context, now time.Time) error {
	domains, err := e.Store.ActiveDomains(ctx)
	if err != nil {
		return err
	}
	for _, d := range domains {
		if err := e.GenerateDomain(ctx, d, now); err != nil {
			e.logger().Error("generate domain alerts", "domain_id", d.ID, "error", err)
		}
	}
	return nil
}

// ReconcileDomain cancels and regenerates pending alerts for one domain.
func (e *Engine) ReconcileDomain(ctx context.Context, d store.Domain, now time.Time) error {
	if err := e.Store.CancelPendingExpiryForDomain(ctx, d.ID, now); err != nil {
		return err
	}
	return e.GenerateDomain(ctx, d, now)
}

// ReconcileRule cancels affected pending alerts and regenerates them.
func (e *Engine) ReconcileRule(ctx context.Context, rule store.AlertRule, now time.Time) error {
	if err := e.Store.CancelPendingNotificationsForRule(ctx, rule.ID, now); err != nil {
		return err
	}
	if rule.ArchivedAt != nil || !rule.Enabled {
		return nil
	}
	domains, err := e.Store.ActiveDomains(ctx)
	if err != nil {
		return err
	}
	for _, d := range domains {
		if rule.Scope == "global" && !d.InheritGlobalAlerts {
			continue
		}
		if rule.Scope == "domain" && d.ID != rule.DomainID {
			continue
		}
		if err := e.GenerateDomain(ctx, d, now); err != nil {
			return err
		}
	}
	return nil
}

// ReconcileSettings cancels and regenerates all pending expiry alerts after a
// scheduling change.
func (e *Engine) ReconcileSettings(ctx context.Context, now time.Time) error {
	if err := e.Store.CancelPendingExpiryAll(ctx, now); err != nil {
		return err
	}
	return e.GenerateAll(ctx, now)
}

// SystemEvent queues open or recovery notices for an incident.
func (e *Engine) SystemEvent(ctx context.Context, incident store.Incident, d store.Domain, event string, message string, now time.Time) error {
	channels, err := e.Store.SystemChannels(ctx)
	if err != nil {
		return err
	}
	snap, err := e.Settings.Get(ctx)
	if err != nil {
		return err
	}
	payload := Payload{
		Event:        "system.incident",
		Kind:         "system",
		DomainID:     d.ID,
		Domain:       d.NameUnicode,
		IncidentID:   incident.ID,
		IncidentKind: incident.Kind,
		SystemEvent:  event,
		Message:      message,
		DueAt:        now.UTC().Format(time.RFC3339),
		URL:          e.BaseURL + "/domains/" + d.ID,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	for _, channel := range channels {
		key := systemDedupeKey(incident.ID, event, channel.ID)
		n, _, err := e.Store.EnqueueNotification(ctx, store.NotificationInput{
			Kind:        "system",
			DomainID:    d.ID,
			IncidentID:  incident.ID,
			ChannelID:   channel.ID,
			DedupeKey:   key,
			ScheduledAt: now,
			PayloadJSON: string(body),
		})
		if err != nil {
			return err
		}
		if err := e.applyHold(ctx, n, d, channel, snap, now); err != nil {
			return err
		}
	}
	return nil
}

func (e *Engine) applyHold(ctx context.Context, n store.Notification, d store.Domain, channel store.Channel, snap settings.Snapshot, now time.Time) error {
	if n.State != "pending" {
		return nil
	}
	hold := now
	if d.MutedUntil != nil && d.MutedUntil.After(hold) && n.Kind == "expiry" {
		hold = *d.MutedUntil
	}
	if channel.MutedUntil != nil && channel.MutedUntil.After(hold) {
		hold = *channel.MutedUntil
	}
	if snap.Alerts.PausedUntil != nil && snap.Alerts.PausedUntil.After(hold) {
		hold = *snap.Alerts.PausedUntil
	}
	if hold.After(now) && n.ScheduledAt.Before(hold) {
		return e.Store.RescheduleNotification(ctx, n.ID, now, hold)
	}
	return nil
}

func expiryDedupeKey(domainID, channelID, expiryDate string, value int, unit, direction string) string {
	return hashKey("expiry", domainID, channelID, expiryDate, fmt.Sprintf("%d", value), unit, direction)
}

func systemDedupeKey(incidentID, event, channelID string) string {
	return hashKey("system", incidentID, event, channelID)
}

func hashKey(parts ...string) string {
	h := sha256.New()
	for _, part := range parts {
		_, _ = h.Write([]byte(part))
		_, _ = h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

func (e *Engine) logger() *slog.Logger {
	if e.Logger != nil {
		return e.Logger
	}
	return slog.Default()
}
