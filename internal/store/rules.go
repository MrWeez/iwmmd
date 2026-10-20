package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/mrweez/iwmmd/internal/platform/ids"
)

// CreateAlertRule inserts a rule and its channels.
func (s *Store) CreateAlertRule(ctx context.Context, r AlertRule) error {
	tx, err := s.Writer.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(ctx, `
		INSERT INTO alert_rules (id, name, scope, domain_id, offset_value, offset_unit,
			direction, send_time_local, enabled, created_by, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ID, r.Name, r.Scope, nullString(r.DomainID), r.OffsetValue, r.OffsetUnit,
		r.Direction, r.SendTimeLocal, boolInt(r.Enabled), nullString(r.CreatedBy),
		formatTime(r.CreatedAt), formatTime(r.UpdatedAt))
	if err != nil {
		return err
	}
	if err := replaceRuleChannelsTx(ctx, tx, r.ID, r.ChannelIDs); err != nil {
		return err
	}
	return tx.Commit()
}

// GetAlertRule returns a rule with channel IDs.
func (s *Store) GetAlertRule(ctx context.Context, id string) (AlertRule, error) {
	row := s.Reader.QueryRowContext(ctx, `SELECT `+ruleColumns+` FROM alert_rules WHERE id = ?`, id)
	r, err := scanRule(row)
	if errors.Is(err, sql.ErrNoRows) {
		return AlertRule{}, ErrNotFound
	}
	if err != nil {
		return AlertRule{}, err
	}
	r.ChannelIDs, err = s.ruleChannels(ctx, r.ID)
	return r, err
}

// ListAlertRules returns non-archived rules, optionally filtered by scope and
// domain.
func (s *Store) ListAlertRules(ctx context.Context, scope, domainID string) ([]AlertRule, error) {
	where := "archived_at IS NULL"
	args := []any{}
	if scope != "" {
		where += " AND scope = ?"
		args = append(args, scope)
	}
	if domainID != "" {
		where += " AND domain_id = ?"
		args = append(args, domainID)
	}
	rows, err := s.Reader.QueryContext(ctx, `SELECT `+ruleColumns+` FROM alert_rules WHERE `+where+`
		ORDER BY scope, offset_value DESC, name`, args...)
	if err != nil {
		return nil, err
	}
	var rules []AlertRule
	for rows.Next() {
		r, err := scanRule(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		rules = append(rules, r)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	for i := range rules {
		rules[i].ChannelIDs, err = s.ruleChannels(ctx, rules[i].ID)
		if err != nil {
			return nil, err
		}
	}
	return rules, nil
}

// RulesForDomain returns enabled global and domain rules for a domain.
func (s *Store) RulesForDomain(ctx context.Context, domainID string, inheritGlobal bool) ([]AlertRule, error) {
	where := "archived_at IS NULL AND enabled = 1 AND scope = 'domain' AND domain_id = ?"
	args := []any{domainID}
	if inheritGlobal {
		where = "archived_at IS NULL AND enabled = 1 AND ((scope = 'global') OR (scope = 'domain' AND domain_id = ?))"
	}
	rows, err := s.Reader.QueryContext(ctx, `SELECT `+ruleColumns+` FROM alert_rules WHERE `+where+`
		ORDER BY offset_value DESC, name`, args...)
	if err != nil {
		return nil, err
	}
	var rules []AlertRule
	for rows.Next() {
		r, err := scanRule(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		rules = append(rules, r)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	for i := range rules {
		rules[i].ChannelIDs, err = s.ruleChannels(ctx, rules[i].ID)
		if err != nil {
			return nil, err
		}
	}
	return rules, nil
}

// UpdateAlertRule updates a rule and replaces its channel set.
func (s *Store) UpdateAlertRule(ctx context.Context, r AlertRule) error {
	tx, err := s.Writer.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `
		UPDATE alert_rules SET name = ?, offset_value = ?, offset_unit = ?, direction = ?,
			send_time_local = ?, enabled = ?, updated_at = ?
		WHERE id = ? AND archived_at IS NULL`,
		r.Name, r.OffsetValue, r.OffsetUnit, r.Direction, r.SendTimeLocal,
		boolInt(r.Enabled), formatTime(r.UpdatedAt), r.ID)
	if err != nil {
		return err
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return ErrNotFound
	}
	if err := replaceRuleChannelsTx(ctx, tx, r.ID, r.ChannelIDs); err != nil {
		return err
	}
	if !r.Enabled {
		if _, err := tx.ExecContext(ctx, `
			UPDATE notifications SET state = 'canceled', updated_at = ?
			WHERE rule_id = ? AND state IN ('pending', 'sending')`, formatTime(r.UpdatedAt), r.ID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ArchiveAlertRule archives a rule and cancels its scheduled notifications.
func (s *Store) ArchiveAlertRule(ctx context.Context, id string, at time.Time) error {
	tx, err := s.Writer.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `
		UPDATE alert_rules SET archived_at = ?, enabled = 0, updated_at = ?
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
		WHERE rule_id = ? AND state IN ('pending', 'sending')`, formatTime(at), id); err != nil {
		return err
	}
	return tx.Commit()
}

// NewAlertRuleID returns a rule identifier.
func NewAlertRuleID() string { return ids.New() }

const ruleColumns = `
	id, name, scope, COALESCE(domain_id, ''), offset_value, offset_unit, direction,
	send_time_local, enabled, archived_at, COALESCE(created_by, ''), created_at, updated_at`

func scanRule(scanner rowScanner) (AlertRule, error) {
	var r AlertRule
	var domainID, archived, createdBy sql.NullString
	var enabled int
	var createdAt, updatedAt string
	err := scanner.Scan(&r.ID, &r.Name, &r.Scope, &domainID, &r.OffsetValue, &r.OffsetUnit,
		&r.Direction, &r.SendTimeLocal, &enabled, &archived, &createdBy, &createdAt, &updatedAt)
	if err != nil {
		return AlertRule{}, err
	}
	r.DomainID = scanString(domainID)
	r.Enabled = enabled == 1
	r.ArchivedAt = scanTime(archived)
	r.CreatedBy = scanString(createdBy)
	r.CreatedAt = parseTime(createdAt)
	r.UpdatedAt = parseTime(updatedAt)
	return r, nil
}

func (s *Store) ruleChannels(ctx context.Context, ruleID string) ([]string, error) {
	rows, err := s.Reader.QueryContext(ctx, `SELECT channel_id FROM alert_rule_channels
		WHERE rule_id = ? ORDER BY channel_id`, ruleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func replaceRuleChannelsTx(ctx context.Context, tx *sql.Tx, ruleID string, channelIDs []string) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM alert_rule_channels WHERE rule_id = ?`, ruleID); err != nil {
		return err
	}
	for _, channelID := range channelIDs {
		if _, err := tx.ExecContext(ctx, `
			INSERT OR IGNORE INTO alert_rule_channels (rule_id, channel_id) VALUES (?, ?)`,
			ruleID, channelID); err != nil {
			return err
		}
	}
	return nil
}
