package store

import (
	"context"
	"time"
)

// ExtendSession rolls a session expiry forward and records activity.
func (s *Store) ExtendSession(ctx context.Context, id string, expires, at time.Time) error {
	_, err := s.Writer.ExecContext(ctx, `
		UPDATE sessions SET expires_at = ?, last_seen_at = ?
		WHERE id = ? AND revoked_at IS NULL`, formatTime(expires), formatTime(at), id)
	return err
}
