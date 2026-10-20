package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mrweez/iwmmd/internal/platform/ids"
)

// CreateChannel inserts a channel.
func (s *Store) CreateChannel(ctx context.Context, c Channel) error {
	_, err := s.Writer.ExecContext(ctx, `
		INSERT INTO channels (id, name, type, config_ciphertext, enabled, receive_system_alerts,
			muted_until, mute_reason, archived_at, created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.ID, c.Name, c.Type, c.ConfigCiphertext, boolInt(c.Enabled), boolInt(c.ReceiveSystemAlerts),
		formatTimePtr(c.MutedUntil), nullString(c.MuteReason), formatTimePtr(c.ArchivedAt),
		nullString(c.CreatedBy), formatTime(c.CreatedAt), formatTime(c.UpdatedAt))
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return ErrConflict
		}
		return err
	}
	return nil
}

// GetChannel returns a channel by ID.
func (s *Store) GetChannel(ctx context.Context, id string) (Channel, error) {
	row := s.Reader.QueryRowContext(ctx, `SELECT `+channelColumns+` FROM channels WHERE id = ?`, id)
	c, err := scanChannel(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Channel{}, ErrNotFound
	}
	return c, err
}

// ListChannels returns channels ordered by name.
func (s *Store) ListChannels(ctx context.Context, includeArchived bool) ([]Channel, error) {
	where := "archived_at IS NULL"
	if includeArchived {
		where = "1 = 1"
	}
	rows, err := s.Reader.QueryContext(ctx, `SELECT `+channelColumns+` FROM channels WHERE `+where+` ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var channels []Channel
	for rows.Next() {
		c, err := scanChannel(rows)
		if err != nil {
			return nil, err
		}
		channels = append(channels, c)
	}
	return channels, rows.Err()
}

// SystemChannels returns enabled, non-archived channels opted into system alerts.
func (s *Store) SystemChannels(ctx context.Context) ([]Channel, error) {
	rows, err := s.Reader.QueryContext(ctx, `SELECT `+channelColumns+` FROM channels
		WHERE archived_at IS NULL AND enabled = 1 AND receive_system_alerts = 1 ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var channels []Channel
	for rows.Next() {
		c, err := scanChannel(rows)
		if err != nil {
			return nil, err
		}
		channels = append(channels, c)
	}
	return channels, rows.Err()
}

// UpdateChannel updates editable channel fields.
func (s *Store) UpdateChannel(ctx context.Context, c Channel) error {
	result, err := s.Writer.ExecContext(ctx, `
		UPDATE channels SET name = ?, config_ciphertext = ?, enabled = ?,
			receive_system_alerts = ?, updated_at = ?
		WHERE id = ? AND archived_at IS NULL`,
		c.Name, c.ConfigCiphertext, boolInt(c.Enabled), boolInt(c.ReceiveSystemAlerts),
		formatTime(c.UpdatedAt), c.ID)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return ErrConflict
		}
		return err
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

// ArchiveChannel archives a channel and cancels pending notifications.
func (s *Store) ArchiveChannel(ctx context.Context, id string, at time.Time) error {
	tx, err := s.Writer.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `
		UPDATE channels SET archived_at = ?, enabled = 0, updated_at = ?
		WHERE id = ? AND archived_at IS NULL`, formatTime(at), formatTime(at), id)
	if err != nil {
		return err
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return ErrNotFound
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE notifications SET state = 'canceled', updated_at = ?
		WHERE channel_id = ? AND state IN ('pending', 'sending')`, formatTime(at), id); err != nil {
		return err
	}
	return tx.Commit()
}

// MuteChannel holds deliveries until the given time. A nil until clears it.
func (s *Store) MuteChannel(ctx context.Context, id string, until *time.Time, reason string, at time.Time) error {
	result, err := s.Writer.ExecContext(ctx, `
		UPDATE channels SET muted_until = ?, mute_reason = ?, updated_at = ?
		WHERE id = ? AND archived_at IS NULL`, formatTimePtr(until), nullString(reason), formatTime(at), id)
	if err != nil {
		return err
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return ErrNotFound
	}
	if until == nil || until.Before(at) {
		_, err = s.Writer.ExecContext(ctx, `
			UPDATE notifications SET next_attempt_at = ?, updated_at = ?
			WHERE channel_id = ? AND state = 'pending'
				AND (next_attempt_at IS NULL OR next_attempt_at > ?)`,
			formatTime(at), formatTime(at), id, formatTime(at))
	}
	return err
}

const channelColumns = `
	id, name, type, config_ciphertext, enabled, receive_system_alerts, muted_until,
	COALESCE(mute_reason, ''), archived_at, COALESCE(created_by, ''), created_at, updated_at`

func scanChannel(scanner rowScanner) (Channel, error) {
	var c Channel
	var muted, archived, createdBy sql.NullString
	var enabled, system int
	var createdAt, updatedAt string
	err := scanner.Scan(&c.ID, &c.Name, &c.Type, &c.ConfigCiphertext, &enabled, &system,
		&muted, &c.MuteReason, &archived, &createdBy, &createdAt, &updatedAt)
	if err != nil {
		return Channel{}, err
	}
	c.Enabled = enabled == 1
	c.ReceiveSystemAlerts = system == 1
	c.MutedUntil = scanTime(muted)
	c.ArchivedAt = scanTime(archived)
	c.CreatedBy = scanString(createdBy)
	c.CreatedAt = parseTime(createdAt)
	c.UpdatedAt = parseTime(updatedAt)
	return c, nil
}

// NewChannelID returns a channel identifier.
func NewChannelID() string { return ids.New() }

// NewChannelNameError wraps a duplicate-name error.
func NewChannelNameError(name string) error {
	return fmt.Errorf("channel %q already exists", name)
}
