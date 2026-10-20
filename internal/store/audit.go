package store

import (
	"context"
	"time"

	"github.com/mrweez/iwmmd/internal/platform/ids"
)

// AuditInput describes an audit event.
type AuditInput struct {
	ActorUserID string
	Action      string
	EntityType  string
	EntityID    string
	BeforeJSON  string
	AfterJSON   string
	IP          string
	UserAgent   string
}

// WriteAudit appends an audit record.
func (s *Store) WriteAudit(ctx context.Context, in AuditInput, at time.Time) error {
	_, err := s.Writer.ExecContext(ctx, `
		INSERT INTO audit_log (id, actor_user_id, action, entity_type, entity_id,
			before_json, after_json, ip, user_agent, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		ids.New(), nullString(in.ActorUserID), in.Action, in.EntityType,
		nullString(in.EntityID), nullString(in.BeforeJSON), nullString(in.AfterJSON),
		nullString(in.IP), nullString(in.UserAgent), formatTime(at))
	return err
}

// ListAudit returns recent audit records.
func (s *Store) ListAudit(ctx context.Context, limit int) ([]AuditEntry, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	rows, err := s.Reader.QueryContext(ctx, `
		SELECT id, COALESCE(actor_user_id, ''), action, entity_type, COALESCE(entity_id, ''),
			COALESCE(before_json, ''), COALESCE(after_json, ''), COALESCE(ip, ''),
			COALESCE(user_agent, ''), created_at
		FROM audit_log ORDER BY created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var entries []AuditEntry
	for rows.Next() {
		var a AuditEntry
		var createdAt string
		if err := rows.Scan(&a.ID, &a.ActorUserID, &a.Action, &a.EntityType, &a.EntityID,
			&a.BeforeJSON, &a.AfterJSON, &a.IP, &a.UserAgent, &createdAt); err != nil {
			return nil, err
		}
		a.CreatedAt = parseTime(createdAt)
		entries = append(entries, a)
	}
	return entries, rows.Err()
}
