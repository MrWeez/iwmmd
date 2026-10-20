package store

import (
	"context"
	"time"
)

// CountAdmins returns the number of admin users.
func (s *Store) CountAdmins(ctx context.Context) (int, error) {
	var count int
	err := s.Reader.QueryRowContext(ctx, `SELECT count(*) FROM users WHERE role = 'admin'`).Scan(&count)
	return count, err
}

// RequeueNotification resets a notification for a manual retry.
func (s *Store) RequeueNotification(ctx context.Context, id string, at time.Time) error {
	result, err := s.Writer.ExecContext(ctx, `
		UPDATE notifications SET state = 'pending', next_attempt_at = ?, attempts = 0,
			last_error = NULL, updated_at = ?
		WHERE id = ? AND state IN ('failed', 'suppressed', 'canceled')`,
		formatTime(at), formatTime(at), id)
	if err != nil {
		return err
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return ErrConflict
	}
	return nil
}
