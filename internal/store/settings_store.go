package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// SettingRow is a raw runtime setting.
type SettingRow struct {
	Key        string
	ValueJSON  string
	Ciphertext string
	IsSecret   bool
	UpdatedBy  string
	UpdatedAt  time.Time
}

// GetSetting reads one setting.
func (s *Store) GetSetting(ctx context.Context, key string) (SettingRow, error) {
	var row SettingRow
	var value, ciphertext, updatedBy sql.NullString
	var isSecret int
	var updatedAt string
	err := s.Reader.QueryRowContext(ctx, `
		SELECT key, value_json, value_ciphertext, is_secret, COALESCE(updated_by, ''), updated_at
		FROM settings WHERE key = ?`, key).
		Scan(&row.Key, &value, &ciphertext, &isSecret, &updatedBy, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return SettingRow{}, ErrNotFound
	}
	if err != nil {
		return SettingRow{}, err
	}
	row.ValueJSON = scanString(value)
	row.Ciphertext = scanString(ciphertext)
	row.IsSecret = isSecret == 1
	row.UpdatedBy = scanString(updatedBy)
	row.UpdatedAt = parseTime(updatedAt)
	return row, nil
}

// ListSettings returns every stored setting.
func (s *Store) ListSettings(ctx context.Context) ([]SettingRow, error) {
	rows, err := s.Reader.QueryContext(ctx, `
		SELECT key, value_json, value_ciphertext, is_secret, COALESCE(updated_by, ''), updated_at
		FROM settings ORDER BY key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var settings []SettingRow
	for rows.Next() {
		var row SettingRow
		var value, ciphertext, updatedBy sql.NullString
		var isSecret int
		var updatedAt string
		if err := rows.Scan(&row.Key, &value, &ciphertext, &isSecret, &updatedBy, &updatedAt); err != nil {
			return nil, err
		}
		row.ValueJSON = scanString(value)
		row.Ciphertext = scanString(ciphertext)
		row.IsSecret = isSecret == 1
		row.UpdatedBy = scanString(updatedBy)
		row.UpdatedAt = parseTime(updatedAt)
		settings = append(settings, row)
	}
	return settings, rows.Err()
}

// PutSetting inserts or updates one setting.
func (s *Store) PutSetting(ctx context.Context, key, valueJSON, ciphertext string, secret bool, userID string, at time.Time) error {
	_, err := s.Writer.ExecContext(ctx, `
		INSERT INTO settings (key, value_json, value_ciphertext, is_secret, updated_by, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET
			value_json = excluded.value_json,
			value_ciphertext = excluded.value_ciphertext,
			is_secret = excluded.is_secret,
			updated_by = excluded.updated_by,
			updated_at = excluded.updated_at`,
		key, nullString(valueJSON), nullString(ciphertext), boolInt(secret), nullString(userID), formatTime(at))
	return err
}
