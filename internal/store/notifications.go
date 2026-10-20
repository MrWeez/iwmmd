package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/mrweez/iwmmd/internal/platform/ids"
)

// NotificationInput describes a queued delivery.
type NotificationInput struct {
	Kind          string
	DomainID      string
	IncidentID    string
	RuleID        string
	ChannelID     string
	ExpiryDate    string
	DedupeKey     string
	ScheduledAt   time.Time
	PayloadJSON   string
	UpdatePending bool
	Reactivate    bool
}

// EnqueueNotification inserts a notification idempotently.
func (s *Store) EnqueueNotification(ctx context.Context, in NotificationInput) (Notification, bool, error) {
	if in.ScheduledAt.IsZero() {
		in.ScheduledAt = time.Now().UTC()
	}
	in.ScheduledAt = in.ScheduledAt.UTC()
	if in.PayloadJSON == "" {
		in.PayloadJSON = "{}"
	}
	now := time.Now().UTC()
	n := Notification{
		ID:          ids.New(),
		Kind:        in.Kind,
		DomainID:    in.DomainID,
		IncidentID:  in.IncidentID,
		RuleID:      in.RuleID,
		ChannelID:   in.ChannelID,
		ExpiryDate:  in.ExpiryDate,
		DedupeKey:   in.DedupeKey,
		State:       "pending",
		ScheduledAt: in.ScheduledAt.UTC(),
		NextAttempt: ptrTime(in.ScheduledAt.UTC()),
		PayloadJSON: in.PayloadJSON,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	_, err := s.Writer.ExecContext(ctx, `
		INSERT INTO notifications (id, kind, domain_id, incident_id, rule_id, channel_id,
			expiry_date_utc, dedupe_key, state, scheduled_at, next_attempt_at, payload_json,
			created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'pending', ?, ?, ?, ?, ?)`,
		n.ID, n.Kind, nullString(n.DomainID), nullString(n.IncidentID), nullString(n.RuleID),
		nullString(n.ChannelID), nullString(n.ExpiryDate), n.DedupeKey, formatTime(n.ScheduledAt),
		formatTime(n.ScheduledAt), n.PayloadJSON, formatTime(now), formatTime(now))
	if err == nil {
		return n, true, nil
	}
	if !strings.Contains(err.Error(), "UNIQUE constraint failed") {
		return Notification{}, false, err
	}
	if in.UpdatePending {
		// A claimed delivery is owned by the dispatcher. Reactivating it would
		// reset its state mid-send and allow a duplicate delivery.
		states := "state = 'pending'"
		if in.Reactivate {
			states = "state IN ('pending', 'canceled', 'suppressed', 'failed')"
		}
		_, _ = s.Writer.ExecContext(ctx, `
			UPDATE notifications SET state = 'pending', scheduled_at = ?, next_attempt_at = ?,
				payload_json = ?, updated_at = ?
			WHERE dedupe_key = ? AND `+states,
			formatTime(in.ScheduledAt), formatTime(in.ScheduledAt), in.PayloadJSON,
			formatTime(now), in.DedupeKey)
	}
	existing, err := s.notificationByDedupe(ctx, in.DedupeKey)
	if err != nil {
		return Notification{}, false, err
	}
	return existing, false, nil
}

// ClaimDueNotifications leases due deliveries.
func (s *Store) ClaimDueNotifications(ctx context.Context, now time.Time, limit int, lease time.Duration) ([]Notification, error) {
	if limit <= 0 {
		limit = 10
	}
	if lease <= 0 {
		lease = 2 * time.Minute
	}
	if _, err := s.Writer.ExecContext(ctx, `
		UPDATE notifications SET state = 'pending', updated_at = ?
		WHERE state = 'sending' AND next_attempt_at IS NOT NULL AND next_attempt_at < ?`,
		formatTime(now), formatTime(now)); err != nil {
		return nil, err
	}
	tx, err := s.Writer.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `SELECT id FROM notifications
		WHERE state = 'pending' AND next_attempt_at IS NOT NULL AND next_attempt_at <= ?
		ORDER BY next_attempt_at ASC LIMIT ?`, formatTime(now), limit)
	if err != nil {
		return nil, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, nil
	}
	leaseUntil := now.Add(lease)
	var claimed []Notification
	for _, id := range ids {
		result, err := tx.ExecContext(ctx, `
			UPDATE notifications SET state = 'sending', attempts = attempts + 1,
				next_attempt_at = ?, updated_at = ?
			WHERE id = ? AND state = 'pending'`, formatTime(leaseUntil), formatTime(now), id)
		if err != nil {
			return nil, err
		}
		affected, _ := result.RowsAffected()
		if affected == 0 {
			continue
		}
		row := tx.QueryRowContext(ctx, `SELECT `+notificationColumns+` FROM notifications n WHERE n.id = ?`, id)
		n, err := scanNotification(row)
		if err != nil {
			return nil, err
		}
		claimed = append(claimed, n)
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return claimed, nil
}

// MarkNotificationSent finalizes a successful delivery.
func (s *Store) MarkNotificationSent(ctx context.Context, id string, at time.Time) error {
	result, err := s.Writer.ExecContext(ctx, `
		UPDATE notifications SET state = 'sent', sent_at = ?, next_attempt_at = NULL,
			updated_at = ? WHERE id = ? AND state = 'sending'`,
		formatTime(at), formatTime(at), id)
	if err != nil {
		return err
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

// RetryNotification schedules a failed delivery.
func (s *Store) RetryNotification(ctx context.Context, id, message string, retryAt, at time.Time) error {
	_, err := s.Writer.ExecContext(ctx, `
		UPDATE notifications SET state = 'pending', next_attempt_at = ?, last_error = ?,
			updated_at = ? WHERE id = ? AND state = 'sending'`,
		formatTime(retryAt), nullString(message), formatTime(at), id)
	return err
}

// FailNotification permanently fails a delivery.
func (s *Store) FailNotification(ctx context.Context, id, message string, at time.Time) error {
	_, err := s.Writer.ExecContext(ctx, `
		UPDATE notifications SET state = 'failed', last_error = ?, updated_at = ?
		WHERE id = ? AND state IN ('pending', 'sending')`,
		nullString(message), formatTime(at), id)
	return err
}

// SuppressNotification marks a delivery superseded by a newer due alert.
func (s *Store) SuppressNotification(ctx context.Context, id string, at time.Time) error {
	_, err := s.Writer.ExecContext(ctx, `
		UPDATE notifications SET state = 'suppressed', updated_at = ? WHERE id = ?`,
		formatTime(at), id)
	return err
}

// RescheduleNotification moves a pending delivery, such as when a mute or
// global pause ends.
func (s *Store) RescheduleNotification(ctx context.Context, id string, at, next time.Time) error {
	_, err := s.Writer.ExecContext(ctx, `
		UPDATE notifications SET state = 'pending', next_attempt_at = ?, updated_at = ?
		WHERE id = ? AND state IN ('pending', 'sending')`,
		formatTime(next), formatTime(at), id)
	return err
}

// CancelNotification cancels a pending delivery.
func (s *Store) CancelNotification(ctx context.Context, id string, at time.Time) error {
	_, err := s.Writer.ExecContext(ctx, `
		UPDATE notifications SET state = 'canceled', updated_at = ?
		WHERE id = ? AND state IN ('pending', 'sending')`, formatTime(at), id)
	return err
}

// NewerDueNotification reports whether a newer expiry alert for the same
// domain and channel is already due.
func (s *Store) NewerDueNotification(ctx context.Context, n Notification, now time.Time) (bool, error) {
	if n.Kind != "expiry" {
		return false, nil
	}
	var one int
	err := s.Reader.QueryRowContext(ctx, `
		SELECT 1 FROM notifications
		WHERE kind = 'expiry' AND domain_id = ? AND channel_id = ? AND id != ?
			AND state IN ('pending', 'sending') AND scheduled_at > ? AND scheduled_at <= ?
		LIMIT 1`, n.DomainID, n.ChannelID, n.ID, formatTime(n.ScheduledAt), formatTime(now)).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

// CancelPendingExpiryAll cancels every pending expiry notification.
func (s *Store) CancelPendingExpiryAll(ctx context.Context, at time.Time) error {
	_, err := s.Writer.ExecContext(ctx, `
		UPDATE notifications SET state = 'canceled', updated_at = ?
		WHERE kind = 'expiry' AND state IN ('pending', 'sending')`, formatTime(at))
	return err
}

// ListNotifications returns notifications with domain and channel names.
func (s *Store) ListNotifications(ctx context.Context, domainID, channelID, state string, limit int) ([]Notification, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	where := []string{"1 = 1"}
	args := []any{}
	if domainID != "" {
		where = append(where, "n.domain_id = ?")
		args = append(args, domainID)
	}
	if channelID != "" {
		where = append(where, "n.channel_id = ?")
		args = append(args, channelID)
	}
	if state != "" {
		where = append(where, "n.state = ?")
		args = append(args, state)
	}
	args = append(args, limit)
	rows, err := s.Reader.QueryContext(ctx, `SELECT `+notificationColumns+`,
		COALESCE(d.name_unicode, ''), COALESCE(c.name, '')
		FROM notifications n
		LEFT JOIN domains d ON d.id = n.domain_id
		LEFT JOIN channels c ON c.id = n.channel_id
		WHERE `+strings.Join(where, " AND ")+`
		ORDER BY n.created_at DESC LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var notifications []Notification
	for rows.Next() {
		n, err := scanNotificationWithNames(rows)
		if err != nil {
			return nil, err
		}
		notifications = append(notifications, n)
	}
	return notifications, rows.Err()
}

// GetNotification returns one notification.
func (s *Store) GetNotification(ctx context.Context, id string) (Notification, error) {
	row := s.Reader.QueryRowContext(ctx, `SELECT `+notificationColumns+` FROM notifications n WHERE n.id = ?`, id)
	n, err := scanNotification(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Notification{}, ErrNotFound
	}
	return n, err
}

// RecordDeliveryAttempt stores an attempt result.
func (s *Store) RecordDeliveryAttempt(ctx context.Context, a DeliveryAttempt) error {
	_, err := s.Writer.ExecContext(ctx, `
		INSERT INTO delivery_attempts (id, notification_id, attempt_no, started_at, finished_at,
			status, response_code, error, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.ID, a.NotificationID, a.AttemptNo, formatTime(a.StartedAt), formatTimePtr(a.FinishedAt),
		a.Status, nullInt(a.ResponseCode), nullString(a.Error), formatTime(a.CreatedAt))
	return err
}

// NewDeliveryAttemptID returns an attempt identifier.
func NewDeliveryAttemptID() string { return ids.New() }

func (s *Store) notificationByDedupe(ctx context.Context, key string) (Notification, error) {
	row := s.Reader.QueryRowContext(ctx, `SELECT `+notificationColumns+`
		FROM notifications n WHERE n.dedupe_key = ?`, key)
	return scanNotification(row)
}

const notificationColumns = `
	n.id, n.kind, COALESCE(n.domain_id, ''), COALESCE(n.incident_id, ''), COALESCE(n.rule_id, ''),
	COALESCE(n.channel_id, ''), COALESCE(n.expiry_date_utc, ''), n.dedupe_key, n.state,
	n.scheduled_at, n.next_attempt_at, n.attempts, COALESCE(n.last_error, ''), n.payload_json,
	n.created_at, n.updated_at, n.sent_at`

func scanNotification(scanner rowScanner) (Notification, error) {
	return scanNotificationValues(scanner, false)
}

func scanNotificationWithNames(scanner rowScanner) (Notification, error) {
	return scanNotificationValues(scanner, true)
}

func scanNotificationValues(scanner rowScanner, withNames bool) (Notification, error) {
	var n Notification
	var scheduledAt, createdAt, updatedAt string
	var nextAttempt, sentAt sql.NullString
	if withNames {
		err := scanner.Scan(&n.ID, &n.Kind, &n.DomainID, &n.IncidentID, &n.RuleID, &n.ChannelID,
			&n.ExpiryDate, &n.DedupeKey, &n.State, &scheduledAt, &nextAttempt, &n.Attempts,
			&n.LastError, &n.PayloadJSON, &createdAt, &updatedAt, &sentAt, &n.DomainName, &n.ChannelName)
		if err != nil {
			return Notification{}, err
		}
	} else {
		err := scanner.Scan(&n.ID, &n.Kind, &n.DomainID, &n.IncidentID, &n.RuleID, &n.ChannelID,
			&n.ExpiryDate, &n.DedupeKey, &n.State, &scheduledAt, &nextAttempt, &n.Attempts,
			&n.LastError, &n.PayloadJSON, &createdAt, &updatedAt, &sentAt)
		if err != nil {
			return Notification{}, err
		}
	}
	n.ScheduledAt = parseTime(scheduledAt)
	n.NextAttempt = scanTime(nextAttempt)
	n.CreatedAt = parseTime(createdAt)
	n.UpdatedAt = parseTime(updatedAt)
	n.SentAt = scanTime(sentAt)
	return n, nil
}

func ptrTime(t time.Time) *time.Time { return &t }
