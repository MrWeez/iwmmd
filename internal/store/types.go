package store

import (
	"database/sql"
	"time"
)

const dbTimeFormat = "2006-01-02T15:04:05.000000000Z07:00"

// User is an authenticated OIDC principal.
type User struct {
	ID          string
	Issuer      string
	Subject     string
	Email       string
	DisplayName string
	Role        string
	LastLoginAt *time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Domain is a monitored domain.
type Domain struct {
	ID                   string
	NameACE              string
	NameUnicode          string
	Registrar            string
	ExpiresAt            *time.Time
	State                string
	InheritGlobalAlerts  bool
	MutedUntil           *time.Time
	MuteReason           string
	CheckIntervalSeconds int
	NextCheckAt          *time.Time
	LastCheckedAt        *time.Time
	LastCheckStatus      string
	LastCheckError       string
	ExpiryConfidence     string
	ConsecutiveFailures  int
	Version              int
	Notes                string
	CreatedBy            string
	CreatedAt            time.Time
	UpdatedAt            time.Time
	ArchivedAt           *time.Time
}

// DomainCheck is one check attempt.
type DomainCheck struct {
	ID           string
	DomainID     string
	Source       string
	Status       string
	HTTPStatus   *int
	ExpiresAt    *time.Time
	Registrar    string
	Confidence   string
	ErrorCode    string
	ErrorMessage string
	DurationMS   int
	StartedAt    time.Time
	FinishedAt   *time.Time
	CreatedAt    time.Time
}

// Channel is a notification destination.
type Channel struct {
	ID                  string
	Name                string
	Type                string
	ConfigCiphertext    string
	Enabled             bool
	ReceiveSystemAlerts bool
	MutedUntil          *time.Time
	MuteReason          string
	ArchivedAt          *time.Time
	CreatedBy           string
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

// AlertRule schedules notifications relative to expiry.
type AlertRule struct {
	ID            string
	Name          string
	Scope         string
	DomainID      string
	OffsetValue   int
	OffsetUnit    string
	Direction     string
	SendTimeLocal string
	Enabled       bool
	ArchivedAt    *time.Time
	CreatedBy     string
	CreatedAt     time.Time
	UpdatedAt     time.Time
	ChannelIDs    []string
}

// Incident is an open or resolved system problem.
type Incident struct {
	ID          string
	DomainID    string
	Kind        string
	State       string
	OpenedAt    time.Time
	ResolvedAt  *time.Time
	LastSeenAt  time.Time
	DetailsJSON string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Notification is a queued delivery.
type Notification struct {
	ID          string
	Kind        string
	DomainID    string
	IncidentID  string
	RuleID      string
	ChannelID   string
	ExpiryDate  string
	DedupeKey   string
	State       string
	ScheduledAt time.Time
	NextAttempt *time.Time
	Attempts    int
	LastError   string
	PayloadJSON string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	SentAt      *time.Time
	DomainName  string
	ChannelName string
}

// DeliveryAttempt records one channel send.
type DeliveryAttempt struct {
	ID             string
	NotificationID string
	AttemptNo      int
	StartedAt      time.Time
	FinishedAt     *time.Time
	Status         string
	ResponseCode   *int
	Error          string
	CreatedAt      time.Time
}

// Job is a durable queued task.
type Job struct {
	ID          string
	Kind        string
	PayloadJSON string
	State       string
	RunAt       time.Time
	Attempts    int
	MaxAttempts int
	LockedBy    string
	LockedUntil *time.Time
	LastError   string
	UniqueKey   string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	FinishedAt  *time.Time
}

// AuditEntry is an immutable admin/action record.
type AuditEntry struct {
	ID          string
	ActorUserID string
	Action      string
	EntityType  string
	EntityID    string
	BeforeJSON  string
	AfterJSON   string
	IP          string
	UserAgent   string
	CreatedAt   time.Time
}

// Session is a server-side login session.
type Session struct {
	ID         string
	UserID     string
	CreatedAt  time.Time
	ExpiresAt  time.Time
	LastSeenAt *time.Time
	IP         string
	UserAgent  string
	RevokedAt  *time.Time
}

type rowScanner interface {
	Scan(dest ...any) error
}

func formatTime(t time.Time) string {
	return t.UTC().Format(dbTimeFormat)
}

func formatTimePtr(t *time.Time) any {
	if t == nil || t.IsZero() {
		return nil
	}
	return formatTime(*t)
}

func parseTime(value string) time.Time {
	t, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}
	}
	return t.UTC()
}

func scanTime(value sql.NullString) *time.Time {
	if !value.Valid || value.String == "" {
		return nil
	}
	t := parseTime(value.String)
	if t.IsZero() {
		return nil
	}
	return &t
}

func scanString(value sql.NullString) string {
	if !value.Valid {
		return ""
	}
	return value.String
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func nullString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func nullInt(value *int) any {
	if value == nil {
		return nil
	}
	return *value
}

func scanNullInt(value sql.NullInt64) *int {
	if !value.Valid {
		return nil
	}
	n := int(value.Int64)
	return &n
}
