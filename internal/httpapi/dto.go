package httpapi

import (
	"context"
	"net/http"
	"time"

	"github.com/mrweez/iwmmd/internal/auth"
	"github.com/mrweez/iwmmd/internal/calendar"
	"github.com/mrweez/iwmmd/internal/store"
)

type userDTO struct {
	ID          string     `json:"id"`
	Email       string     `json:"email"`
	DisplayName string     `json:"display_name"`
	Role        string     `json:"role"`
	LastLoginAt *time.Time `json:"last_login_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

type domainDTO struct {
	ID                   string     `json:"id"`
	Name                 string     `json:"name"`
	NameACE              string     `json:"name_ace"`
	Registrar            string     `json:"registrar,omitempty"`
	ExpiresAt            *time.Time `json:"expires_at,omitempty"`
	DaysLeft             *int       `json:"days_left,omitempty"`
	State                string     `json:"state"`
	InheritGlobalAlerts  bool       `json:"inherit_global_alerts"`
	MutedUntil           *time.Time `json:"muted_until,omitempty"`
	MuteReason           string     `json:"mute_reason,omitempty"`
	CheckIntervalSeconds int        `json:"check_interval_seconds"`
	NextCheckAt          *time.Time `json:"next_check_at,omitempty"`
	LastCheckedAt        *time.Time `json:"last_checked_at,omitempty"`
	LastCheckStatus      string     `json:"last_check_status,omitempty"`
	LastCheckError       string     `json:"last_check_error,omitempty"`
	ExpiryConfidence     string     `json:"expiry_confidence"`
	ConsecutiveFailures  int        `json:"consecutive_failures"`
	Version              int        `json:"version"`
	Notes                string     `json:"notes,omitempty"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
	ArchivedAt           *time.Time `json:"archived_at,omitempty"`
}

type checkDTO struct {
	ID           string     `json:"id"`
	DomainID     string     `json:"domain_id"`
	Source       string     `json:"source"`
	Status       string     `json:"status"`
	HTTPStatus   *int       `json:"http_status,omitempty"`
	ExpiresAt    *time.Time `json:"expires_at,omitempty"`
	Registrar    string     `json:"registrar,omitempty"`
	Confidence   string     `json:"confidence"`
	ErrorCode    string     `json:"error_code,omitempty"`
	ErrorMessage string     `json:"error_message,omitempty"`
	DurationMS   int        `json:"duration_ms"`
	StartedAt    time.Time  `json:"started_at"`
	FinishedAt   *time.Time `json:"finished_at,omitempty"`
}

type ruleDTO struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	Scope         string    `json:"scope"`
	DomainID      string    `json:"domain_id,omitempty"`
	OffsetValue   int       `json:"offset_value"`
	OffsetUnit    string    `json:"offset_unit"`
	Direction     string    `json:"direction"`
	SendTimeLocal string    `json:"send_time_local"`
	Enabled       bool      `json:"enabled"`
	ChannelIDs    []string  `json:"channel_ids"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type incidentDTO struct {
	ID          string     `json:"id"`
	DomainID    string     `json:"domain_id"`
	Kind        string     `json:"kind"`
	State       string     `json:"state"`
	OpenedAt    time.Time  `json:"opened_at"`
	ResolvedAt  *time.Time `json:"resolved_at,omitempty"`
	LastSeenAt  time.Time  `json:"last_seen_at"`
	DetailsJSON string     `json:"details"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

type notificationDTO struct {
	ID          string     `json:"id"`
	Kind        string     `json:"kind"`
	DomainID    string     `json:"domain_id,omitempty"`
	DomainName  string     `json:"domain_name,omitempty"`
	IncidentID  string     `json:"incident_id,omitempty"`
	RuleID      string     `json:"rule_id,omitempty"`
	ChannelID   string     `json:"channel_id,omitempty"`
	ChannelName string     `json:"channel_name,omitempty"`
	ExpiryDate  string     `json:"expiry_date,omitempty"`
	State       string     `json:"state"`
	ScheduledAt time.Time  `json:"scheduled_at"`
	NextAttempt *time.Time `json:"next_attempt_at,omitempty"`
	Attempts    int        `json:"attempts"`
	LastError   string     `json:"last_error,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	SentAt      *time.Time `json:"sent_at,omitempty"`
}

func toUserDTO(user store.User) userDTO {
	return userDTO{
		ID:          user.ID,
		Email:       user.Email,
		DisplayName: user.DisplayName,
		Role:        user.Role,
		LastLoginAt: user.LastLoginAt,
		CreatedAt:   user.CreatedAt,
	}
}

func toDomainDTO(d store.Domain, now time.Time) domainDTO {
	var days *int
	if d.ExpiresAt != nil {
		value := calendar.CalendarDaysBetween(now, *d.ExpiresAt)
		days = &value
	}
	return domainDTO{
		ID:                   d.ID,
		Name:                 d.NameUnicode,
		NameACE:              d.NameACE,
		Registrar:            d.Registrar,
		ExpiresAt:            d.ExpiresAt,
		DaysLeft:             days,
		State:                d.State,
		InheritGlobalAlerts:  d.InheritGlobalAlerts,
		MutedUntil:           d.MutedUntil,
		MuteReason:           d.MuteReason,
		CheckIntervalSeconds: d.CheckIntervalSeconds,
		NextCheckAt:          d.NextCheckAt,
		LastCheckedAt:        d.LastCheckedAt,
		LastCheckStatus:      d.LastCheckStatus,
		LastCheckError:       d.LastCheckError,
		ExpiryConfidence:     d.ExpiryConfidence,
		ConsecutiveFailures:  d.ConsecutiveFailures,
		Version:              d.Version,
		Notes:                d.Notes,
		CreatedAt:            d.CreatedAt,
		UpdatedAt:            d.UpdatedAt,
		ArchivedAt:           d.ArchivedAt,
	}
}

func toCheckDTO(c store.DomainCheck) checkDTO {
	return checkDTO{
		ID:           c.ID,
		DomainID:     c.DomainID,
		Source:       c.Source,
		Status:       c.Status,
		HTTPStatus:   c.HTTPStatus,
		ExpiresAt:    c.ExpiresAt,
		Registrar:    c.Registrar,
		Confidence:   c.Confidence,
		ErrorCode:    c.ErrorCode,
		ErrorMessage: c.ErrorMessage,
		DurationMS:   c.DurationMS,
		StartedAt:    c.StartedAt,
		FinishedAt:   c.FinishedAt,
	}
}

func toRuleDTO(r store.AlertRule) ruleDTO {
	return ruleDTO{
		ID:            r.ID,
		Name:          r.Name,
		Scope:         r.Scope,
		DomainID:      r.DomainID,
		OffsetValue:   r.OffsetValue,
		OffsetUnit:    r.OffsetUnit,
		Direction:     r.Direction,
		SendTimeLocal: r.SendTimeLocal,
		Enabled:       r.Enabled,
		ChannelIDs:    r.ChannelIDs,
		CreatedAt:     r.CreatedAt,
		UpdatedAt:     r.UpdatedAt,
	}
}

func toIncidentDTO(inc store.Incident) incidentDTO {
	return incidentDTO{
		ID:          inc.ID,
		DomainID:    inc.DomainID,
		Kind:        inc.Kind,
		State:       inc.State,
		OpenedAt:    inc.OpenedAt,
		ResolvedAt:  inc.ResolvedAt,
		LastSeenAt:  inc.LastSeenAt,
		DetailsJSON: inc.DetailsJSON,
		UpdatedAt:   inc.UpdatedAt,
	}
}

func toNotificationDTO(n store.Notification) notificationDTO {
	return notificationDTO{
		ID:          n.ID,
		Kind:        n.Kind,
		DomainID:    n.DomainID,
		DomainName:  n.DomainName,
		IncidentID:  n.IncidentID,
		RuleID:      n.RuleID,
		ChannelID:   n.ChannelID,
		ChannelName: n.ChannelName,
		ExpiryDate:  n.ExpiryDate,
		State:       n.State,
		ScheduledAt: n.ScheduledAt,
		NextAttempt: n.NextAttempt,
		Attempts:    n.Attempts,
		LastError:   n.LastError,
		CreatedAt:   n.CreatedAt,
		SentAt:      n.SentAt,
	}
}

func (a *API) audit(r *http.Request, actor store.User, action, entityType, entityID, before, after string) {
	if a.deps.Store == nil {
		return
	}
	_ = a.deps.Store.WriteAudit(context.Background(), store.AuditInput{
		ActorUserID: actor.ID,
		Action:      action,
		EntityType:  entityType,
		EntityID:    entityID,
		BeforeJSON:  before,
		AfterJSON:   after,
		IP:          auth.SessionIP(r),
		UserAgent:   r.UserAgent(),
	}, now())
}

type auditDTO struct {
	ID               string    `json:"id"`
	ActorUserID      string    `json:"actor_user_id,omitempty"`
	ActorEmail       string    `json:"actor_email,omitempty"`
	ActorDisplayName string    `json:"actor_display_name,omitempty"`
	Action           string    `json:"action"`
	EntityType       string    `json:"entity_type"`
	EntityID         string    `json:"entity_id,omitempty"`
	BeforeJSON       string    `json:"before,omitempty"`
	AfterJSON        string    `json:"after,omitempty"`
	IP               string    `json:"ip,omitempty"`
	UserAgent        string    `json:"user_agent,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
}

type jobDTO struct {
	ID          string     `json:"id"`
	Kind        string     `json:"kind"`
	State       string     `json:"state"`
	RunAt       time.Time  `json:"run_at"`
	Attempts    int        `json:"attempts"`
	MaxAttempts int        `json:"max_attempts"`
	LastError   string     `json:"last_error,omitempty"`
	UniqueKey   string     `json:"unique_key,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	FinishedAt  *time.Time `json:"finished_at,omitempty"`
}

func toAuditDTOs(items []store.AuditEntry) []auditDTO {
	out := make([]auditDTO, 0, len(items))
	for _, item := range items {
		out = append(out, auditDTO{
			ID: item.ID, ActorUserID: item.ActorUserID, ActorEmail: item.ActorEmail,
			ActorDisplayName: item.ActorDisplayName, Action: item.Action,
			EntityType: item.EntityType, EntityID: item.EntityID, BeforeJSON: item.BeforeJSON,
			AfterJSON: item.AfterJSON, IP: item.IP, UserAgent: item.UserAgent, CreatedAt: item.CreatedAt,
		})
	}
	return out
}

func toJobDTOs(items []store.Job) []jobDTO {
	out := make([]jobDTO, 0, len(items))
	for _, item := range items {
		out = append(out, jobDTO{
			ID: item.ID, Kind: item.Kind, State: item.State, RunAt: item.RunAt,
			Attempts: item.Attempts, MaxAttempts: item.MaxAttempts, LastError: item.LastError,
			UniqueKey: item.UniqueKey, CreatedAt: item.CreatedAt, FinishedAt: item.FinishedAt,
		})
	}
	return out
}
