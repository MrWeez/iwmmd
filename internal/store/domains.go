package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrNotFound is returned when a requested record does not exist.
var ErrNotFound = errors.New("not found")

// ErrConflict is returned when optimistic concurrency or a unique constraint
// rejects a write.
var ErrConflict = errors.New("conflict")

// DomainFilter controls domain listing.
type DomainFilter struct {
	Search string
	State  string
	Expiry string
	Limit  int
	Offset int
}

const domainColumns = `
	id, name_ace, name_unicode, COALESCE(registrar, ''), expires_at, state,
	inherit_global_alerts, muted_until, COALESCE(mute_reason, ''),
	check_interval_seconds, next_check_at, last_checked_at,
	COALESCE(last_check_status, ''), COALESCE(last_check_error, ''),
	expiry_confidence, consecutive_failures, version, COALESCE(notes, ''),
	COALESCE(created_by, ''), created_at, updated_at, archived_at`

// CreateDomain inserts a domain.
func (s *Store) CreateDomain(ctx context.Context, d Domain) error {
	_, err := s.Writer.ExecContext(ctx, `
		INSERT INTO domains (
			id, name_ace, name_unicode, registrar, expires_at, state,
			inherit_global_alerts, muted_until, mute_reason, check_interval_seconds,
			next_check_at, last_checked_at, last_check_status, last_check_error,
			expiry_confidence, consecutive_failures, version, notes, created_by,
			created_at, updated_at, archived_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		d.ID, d.NameACE, d.NameUnicode, nullString(d.Registrar), formatTimePtr(d.ExpiresAt), d.State,
		boolInt(d.InheritGlobalAlerts), formatTimePtr(d.MutedUntil), nullString(d.MuteReason),
		d.CheckIntervalSeconds, formatTimePtr(d.NextCheckAt), formatTimePtr(d.LastCheckedAt),
		nullString(d.LastCheckStatus), nullString(d.LastCheckError), d.ExpiryConfidence,
		d.ConsecutiveFailures, d.Version, nullString(d.Notes), nullString(d.CreatedBy),
		formatTime(d.CreatedAt), formatTime(d.UpdatedAt), formatTimePtr(d.ArchivedAt),
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint failed") {
			return ErrConflict
		}
		return fmt.Errorf("insert domain: %w", err)
	}
	return nil
}

// GetDomain returns one domain by ID.
func (s *Store) GetDomain(ctx context.Context, id string) (Domain, error) {
	row := s.Reader.QueryRowContext(ctx, `SELECT `+domainColumns+` FROM domains WHERE id = ?`, id)
	d, err := scanDomain(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Domain{}, ErrNotFound
	}
	return d, err
}

// FindDomainByName returns one domain by canonical ASCII name.
func (s *Store) FindDomainByName(ctx context.Context, name string) (Domain, error) {
	row := s.Reader.QueryRowContext(ctx, `SELECT `+domainColumns+` FROM domains WHERE name_ace = ?`, name)
	d, err := scanDomain(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Domain{}, ErrNotFound
	}
	return d, err
}

// ListDomains returns a filtered page of domains.
func (s *Store) ListDomains(ctx context.Context, filter DomainFilter) ([]Domain, int, error) {
	if filter.Limit <= 0 {
		filter.Limit = 50
	}
	if filter.Limit > 200 {
		filter.Limit = 200
	}
	if filter.Offset < 0 {
		filter.Offset = 0
	}

	where := []string{"1 = 1"}
	args := []any{}
	if filter.Search != "" {
		like := "%" + strings.ToLower(filter.Search) + "%"
		where = append(where, "(lower(name_ace) LIKE ? OR lower(name_unicode) LIKE ? OR lower(COALESCE(registrar, '')) LIKE ?)")
		args = append(args, like, like, like)
	}
	switch filter.State {
	case "active", "paused", "archived":
		where = append(where, "state = ?")
		args = append(args, filter.State)
	default:
		where = append(where, "state != 'archived'")
	}
	now := formatTime(time.Now().UTC())
	switch filter.Expiry {
	case "expired":
		where = append(where, "expires_at IS NOT NULL AND expires_at < ?")
		args = append(args, now)
	case "7d":
		where = append(where, "expires_at IS NOT NULL AND expires_at < ?")
		args = append(args, formatTime(time.Now().UTC().AddDate(0, 0, 7)))
	case "30d":
		where = append(where, "expires_at IS NOT NULL AND expires_at < ?")
		args = append(args, formatTime(time.Now().UTC().AddDate(0, 0, 30)))
	case "90d":
		where = append(where, "expires_at IS NOT NULL AND expires_at < ?")
		args = append(args, formatTime(time.Now().UTC().AddDate(0, 0, 90)))
	case "unknown":
		where = append(where, "expires_at IS NULL")
	}

	whereSQL := strings.Join(where, " AND ")
	var total int
	if err := s.Reader.QueryRowContext(ctx, `SELECT count(*) FROM domains WHERE `+whereSQL, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count domains: %w", err)
	}
	queryArgs := append(append([]any{}, args...), filter.Limit, filter.Offset)
	rows, err := s.Reader.QueryContext(ctx, `SELECT `+domainColumns+` FROM domains WHERE `+whereSQL+`
		ORDER BY (expires_at IS NULL), expires_at ASC, name_ace ASC LIMIT ? OFFSET ?`, queryArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("list domains: %w", err)
	}
	defer rows.Close()
	items := make([]Domain, 0, filter.Limit)
	for rows.Next() {
		d, err := scanDomain(rows)
		if err != nil {
			return nil, 0, err
		}
		items = append(items, d)
	}
	return items, total, rows.Err()
}

// UpdateDomain applies user-editable fields with optimistic concurrency.
func (s *Store) UpdateDomain(ctx context.Context, d Domain) error {
	result, err := s.Writer.ExecContext(ctx, `
		UPDATE domains SET
			registrar = ?, state = ?, inherit_global_alerts = ?, check_interval_seconds = ?,
			next_check_at = ?, notes = ?, version = version + 1, updated_at = ?
		WHERE id = ? AND version = ?`,
		nullString(d.Registrar), d.State, boolInt(d.InheritGlobalAlerts), d.CheckIntervalSeconds,
		formatTimePtr(d.NextCheckAt), nullString(d.Notes), formatTime(time.Now().UTC()), d.ID, d.Version,
	)
	if err != nil {
		return fmt.Errorf("update domain: %w", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrConflict
	}
	return nil
}

// ArchiveDomain marks a domain archived and cancels its pending work.
func (s *Store) ArchiveDomain(ctx context.Context, id string, at time.Time) error {
	tx, err := s.Writer.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `
		UPDATE domains SET state = 'archived', archived_at = ?, next_check_at = NULL,
			version = version + 1, updated_at = ?
		WHERE id = ? AND state != 'archived'`, formatTime(at), formatTime(at), id)
	if err != nil {
		return err
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return ErrNotFound
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE notifications SET state = 'canceled', updated_at = ?
		WHERE domain_id = ? AND state IN ('pending', 'sending')`, formatTime(at), id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `
		UPDATE jobs SET state = 'canceled', updated_at = ?
		WHERE state IN ('pending', 'running') AND unique_key = ?`, formatTime(at), "domain-check:"+id); err != nil {
		return err
	}
	return tx.Commit()
}

// MuteDomain holds expiry deliveries until the given time. A nil until clears
// the mute.
func (s *Store) MuteDomain(ctx context.Context, id string, until *time.Time, reason string, at time.Time) error {
	result, err := s.Writer.ExecContext(ctx, `
		UPDATE domains SET muted_until = ?, mute_reason = ?, version = version + 1, updated_at = ?
		WHERE id = ? AND state != 'archived'`,
		formatTimePtr(until), nullString(reason), formatTime(at), id)
	if err != nil {
		return err
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return ErrNotFound
	}
	if until == nil || until.Before(at) {
		return s.wakeExpiryNotifications(ctx, id, at)
	}
	return nil
}

// DueDomains returns active domains whose next check is due.
func (s *Store) DueDomains(ctx context.Context, now time.Time, limit int) ([]Domain, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.Reader.QueryContext(ctx, `SELECT `+domainColumns+` FROM domains
		WHERE state = 'active' AND next_check_at IS NOT NULL AND next_check_at <= ?
		ORDER BY next_check_at ASC LIMIT ?`, formatTime(now), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var domains []Domain
	for rows.Next() {
		d, err := scanDomain(rows)
		if err != nil {
			return nil, err
		}
		domains = append(domains, d)
	}
	return domains, rows.Err()
}

// ActiveDomains returns all non-archived domains.
func (s *Store) ActiveDomains(ctx context.Context) ([]Domain, error) {
	rows, err := s.Reader.QueryContext(ctx, `SELECT `+domainColumns+` FROM domains WHERE state != 'archived' ORDER BY name_ace`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var domains []Domain
	for rows.Next() {
		d, err := scanDomain(rows)
		if err != nil {
			return nil, err
		}
		domains = append(domains, d)
	}
	return domains, rows.Err()
}

// DomainStats summarizes dashboard counts.
type DomainStats struct {
	Total         int
	Active        int
	Expired       int
	Expiring7     int
	Expiring30    int
	Expiring90    int
	Unknown       int
	Paused        int
	Archived      int
	OpenIncidents int
}

// Stats returns dashboard counts.
func (s *Store) Stats(ctx context.Context) (DomainStats, error) {
	var stats DomainStats
	now := time.Now().UTC()
	rows, err := s.Reader.QueryContext(ctx, `SELECT state, expires_at FROM domains`)
	if err != nil {
		return stats, err
	}
	defer rows.Close()
	for rows.Next() {
		var state string
		var expires sql.NullString
		if err := rows.Scan(&state, &expires); err != nil {
			return stats, err
		}
		stats.Total++
		switch state {
		case "active":
			stats.Active++
		case "paused":
			stats.Paused++
		case "archived":
			stats.Archived++
		}
		if state == "archived" {
			continue
		}
		if !expires.Valid {
			stats.Unknown++
			continue
		}
		expiry := parseTime(expires.String)
		if expiry.IsZero() {
			stats.Unknown++
			continue
		}
		if expiry.Before(now) {
			stats.Expired++
			continue
		}
		days := int(expiry.Sub(now).Hours() / 24)
		if days <= 7 {
			stats.Expiring7++
		}
		if days <= 30 {
			stats.Expiring30++
		}
		if days <= 90 {
			stats.Expiring90++
		}
	}
	if err := rows.Err(); err != nil {
		return stats, err
	}
	if err := s.Reader.QueryRowContext(ctx, `SELECT count(*) FROM incidents WHERE state = 'open'`).Scan(&stats.OpenIncidents); err != nil {
		return stats, err
	}
	return stats, nil
}

func scanDomain(scanner rowScanner) (Domain, error) {
	var d Domain
	var registrar, expiresAt, mutedUntil, muteReason, nextCheck, lastChecked sql.NullString
	var lastStatus, lastError, notes, createdBy, archivedAt sql.NullString
	var inherit int
	var createdAt, updatedAt string
	err := scanner.Scan(
		&d.ID, &d.NameACE, &d.NameUnicode, &registrar, &expiresAt, &d.State,
		&inherit, &mutedUntil, &muteReason, &d.CheckIntervalSeconds, &nextCheck, &lastChecked,
		&lastStatus, &lastError, &d.ExpiryConfidence, &d.ConsecutiveFailures, &d.Version, &notes,
		&createdBy, &createdAt, &updatedAt, &archivedAt,
	)
	if err != nil {
		return Domain{}, err
	}
	d.Registrar = scanString(registrar)
	d.ExpiresAt = scanTime(expiresAt)
	d.InheritGlobalAlerts = inherit == 1
	d.MutedUntil = scanTime(mutedUntil)
	d.MuteReason = scanString(muteReason)
	d.NextCheckAt = scanTime(nextCheck)
	d.LastCheckedAt = scanTime(lastChecked)
	d.LastCheckStatus = scanString(lastStatus)
	d.LastCheckError = scanString(lastError)
	d.Notes = scanString(notes)
	d.CreatedBy = scanString(createdBy)
	d.CreatedAt = parseTime(createdAt)
	d.UpdatedAt = parseTime(updatedAt)
	d.ArchivedAt = scanTime(archivedAt)
	return d, nil
}

func (s *Store) wakeExpiryNotifications(ctx context.Context, domainID string, at time.Time) error {
	_, err := s.Writer.ExecContext(ctx, `
		UPDATE notifications SET next_attempt_at = ?, updated_at = ?
		WHERE domain_id = ? AND kind = 'expiry' AND state = 'pending'
			AND (next_attempt_at IS NULL OR next_attempt_at > ?)`,
		formatTime(at), formatTime(at), domainID, formatTime(at))
	return err
}
