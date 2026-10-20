package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/mrweez/iwmmd/internal/platform/ids"
)

// CheckRecord is the persistence input for one domain check.
type CheckRecord struct {
	DomainID       string
	Source         string
	Status         string
	HTTPStatus     *int
	ExpiresAt      *time.Time
	Registrar      string
	Confidence     string
	ErrorCode      string
	ErrorMessage   string
	DurationMS     int
	StartedAt      time.Time
	FinishedAt     time.Time
	Raw            []byte
	RawContentType string
	RawExpiresAt   *time.Time
	NextCheckAt    *time.Time
	IncidentKind   string
}

// CheckOutcome reports the state transition caused by a check.
type CheckOutcome struct {
	Domain         Domain
	PreviousExpiry *time.Time
	ExpiryChanged  bool
	Opened         []Incident
	Resolved       []Incident
}

// RecordCheck persists a check, updates the domain, and reconciles incidents.
func (s *Store) RecordCheck(ctx context.Context, rec CheckRecord) (CheckOutcome, error) {
	tx, err := s.Writer.BeginTx(ctx, nil)
	if err != nil {
		return CheckOutcome{}, err
	}
	defer tx.Rollback()

	row := tx.QueryRowContext(ctx, `SELECT `+domainColumns+` FROM domains WHERE id = ?`, rec.DomainID)
	domain, err := scanDomain(row)
	if errors.Is(err, sql.ErrNoRows) {
		return CheckOutcome{}, ErrNotFound
	}
	if err != nil {
		return CheckOutcome{}, err
	}

	checkID := ids.New()
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO domain_checks (
			id, domain_id, source, status, http_status, expires_at, registrar,
			confidence, error_code, error_message, duration_ms, started_at, finished_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		checkID, rec.DomainID, rec.Source, rec.Status, nullInt(rec.HTTPStatus),
		formatTimePtr(rec.ExpiresAt), nullString(rec.Registrar), rec.Confidence,
		nullString(rec.ErrorCode), nullString(rec.ErrorMessage), rec.DurationMS,
		formatTime(rec.StartedAt), formatTime(rec.FinishedAt),
	); err != nil {
		return CheckOutcome{}, fmt.Errorf("insert check: %w", err)
	}
	if len(rec.Raw) > 0 {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO check_raw_responses (check_id, encoding, body, content_type, expires_at)
			VALUES (?, 'identity', ?, ?, ?)`,
			checkID, rec.Raw, nullString(rec.RawContentType), formatTimePtr(rec.RawExpiresAt),
		); err != nil {
			return CheckOutcome{}, fmt.Errorf("insert raw response: %w", err)
		}
	}

	newExpiry := domain.ExpiresAt
	newConfidence := domain.ExpiryConfidence
	consecutive := domain.ConsecutiveFailures
	status := rec.Status
	lastError := rec.ErrorMessage
	if rec.Status == "ok" {
		if rec.ExpiresAt != nil {
			newExpiry = rec.ExpiresAt
			newConfidence = rec.Confidence
		} else {
			newConfidence = "unknown"
		}
		consecutive = 0
	} else {
		consecutive++
		if rec.IncidentKind == "parse_failure" || rec.IncidentKind == "expiry_unknown" {
			newConfidence = "unknown"
		}
	}

	expiryChanged := false
	if !sameDate(domain.ExpiresAt, newExpiry) {
		expiryChanged = true
	}

	_, err = tx.ExecContext(ctx, `
		UPDATE domains SET
			registrar = CASE WHEN ? = '' THEN registrar ELSE ? END,
			expires_at = ?, expiry_confidence = ?, consecutive_failures = ?,
			last_checked_at = ?, last_check_status = ?, last_check_error = ?,
			next_check_at = ?, version = version + 1, updated_at = ?
		WHERE id = ?`,
		rec.Registrar, rec.Registrar, formatTimePtr(newExpiry), newConfidence, consecutive,
		formatTime(rec.FinishedAt), status, nullString(lastError), formatTimePtr(rec.NextCheckAt),
		formatTime(rec.FinishedAt), rec.DomainID,
	)
	if err != nil {
		return CheckOutcome{}, fmt.Errorf("update domain check state: %w", err)
	}

	if expiryChanged && domain.ExpiresAt != nil {
		newDate := ""
		if newExpiry != nil {
			newDate = newExpiry.UTC().Format("2006-01-02")
		}
		if _, err := tx.ExecContext(ctx, `
			UPDATE notifications SET state = 'canceled', updated_at = ?
			WHERE domain_id = ? AND kind = 'expiry' AND state IN ('pending', 'sending')
				AND (expiry_date_utc IS NULL OR expiry_date_utc != ?)`,
			formatTime(rec.FinishedAt), rec.DomainID, newDate,
		); err != nil {
			return CheckOutcome{}, err
		}
	}

	outcome := CheckOutcome{Domain: domain, PreviousExpiry: domain.ExpiresAt, ExpiryChanged: expiryChanged}
	if rec.Status == "ok" && rec.ExpiresAt != nil {
		resolved, err := resolveIncidentsTx(ctx, tx, rec.DomainID, rec.FinishedAt,
			"check_failure", "parse_failure", "expiry_unknown")
		if err != nil {
			return CheckOutcome{}, err
		}
		outcome.Resolved = resolved
	} else if rec.Status == "ok" && rec.ExpiresAt == nil {
		if _, err := resolveIncidentsTx(ctx, tx, rec.DomainID, rec.FinishedAt, "check_failure"); err != nil {
			return CheckOutcome{}, err
		}
		inc, opened, err := ensureIncidentTx(ctx, tx, rec.DomainID, "expiry_unknown",
			rec.FinishedAt, rec.ErrorMessage)
		if err != nil {
			return CheckOutcome{}, err
		}
		if opened {
			outcome.Opened = append(outcome.Opened, inc)
		}
	} else {
		kind := rec.IncidentKind
		if kind == "" {
			kind = "check_failure"
		}
		if kind == "check_failure" && consecutive < 3 {
			// Failure threshold not reached.
		} else {
			inc, opened, err := ensureIncidentTx(ctx, tx, rec.DomainID, kind,
				rec.FinishedAt, rec.ErrorMessage)
			if err != nil {
				return CheckOutcome{}, err
			}
			if opened {
				outcome.Opened = append(outcome.Opened, inc)
			}
		}
		if consecutive >= 3 && kind != "check_failure" {
			inc, opened, err := ensureIncidentTx(ctx, tx, rec.DomainID, "check_failure",
				rec.FinishedAt, rec.ErrorMessage)
			if err != nil {
				return CheckOutcome{}, err
			}
			if opened {
				outcome.Opened = append(outcome.Opened, inc)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return CheckOutcome{}, err
	}
	domain.ExpiresAt = newExpiry
	domain.ExpiryConfidence = newConfidence
	domain.ConsecutiveFailures = consecutive
	domain.NextCheckAt = rec.NextCheckAt
	domain.LastCheckedAt = &rec.FinishedAt
	domain.LastCheckStatus = status
	domain.LastCheckError = lastError
	outcome.Domain = domain
	return outcome, nil
}

func sameDate(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.UTC().Format("2006-01-02") == b.UTC().Format("2006-01-02")
}

// ListChecks returns recent checks for a domain.
func (s *Store) ListChecks(ctx context.Context, domainID string, limit int) ([]DomainCheck, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.Reader.QueryContext(ctx, `
		SELECT id, domain_id, source, status, http_status, expires_at, COALESCE(registrar, ''),
			confidence, COALESCE(error_code, ''), COALESCE(error_message, ''), duration_ms,
			started_at, finished_at, created_at
		FROM domain_checks WHERE domain_id = ? ORDER BY started_at DESC LIMIT ?`, domainID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var checks []DomainCheck
	for rows.Next() {
		var c DomainCheck
		var httpStatus sql.NullInt64
		var expiresAt, registrar, errCode, errMsg, finished sql.NullString
		var startedAt, createdAt string
		if err := rows.Scan(&c.ID, &c.DomainID, &c.Source, &c.Status, &httpStatus, &expiresAt,
			&registrar, &c.Confidence, &errCode, &errMsg, &c.DurationMS, &startedAt, &finished, &createdAt); err != nil {
			return nil, err
		}
		c.HTTPStatus = scanNullInt(httpStatus)
		c.ExpiresAt = scanTime(expiresAt)
		c.Registrar = scanString(registrar)
		c.ErrorCode = scanString(errCode)
		c.ErrorMessage = scanString(errMsg)
		c.StartedAt = parseTime(startedAt)
		c.FinishedAt = scanTime(finished)
		c.CreatedAt = parseTime(createdAt)
		checks = append(checks, c)
	}
	return checks, rows.Err()
}

// GetRawCheck returns a stored raw check response.
func (s *Store) GetRawCheck(ctx context.Context, checkID string) ([]byte, string, error) {
	return s.GetRawCheckForDomain(ctx, "", checkID)
}

// GetRawCheckForDomain returns a raw response, optionally scoped to a domain.
func (s *Store) GetRawCheckForDomain(ctx context.Context, domainID, checkID string) ([]byte, string, error) {
	var body []byte
	var contentType sql.NullString
	query := `SELECT r.body, r.content_type FROM check_raw_responses r
		JOIN domain_checks c ON c.id = r.check_id WHERE r.check_id = ?`
	args := []any{checkID}
	if domainID != "" {
		query += ` AND c.domain_id = ?`
		args = append(args, domainID)
	}
	err := s.Reader.QueryRowContext(ctx, query, args...).Scan(&body, &contentType)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, "", ErrNotFound
	}
	return body, scanString(contentType), err
}

func ensureIncidentTx(ctx context.Context, tx *sql.Tx, domainID, kind string, at time.Time, detail string) (Incident, bool, error) {
	row := tx.QueryRowContext(ctx, `
		SELECT id, domain_id, kind, state, opened_at, resolved_at, last_seen_at,
			details_json, created_at, updated_at
		FROM incidents WHERE domain_id = ? AND kind = ? AND state = 'open'`, domainID, kind)
	inc, err := scanIncident(row)
	if err == nil {
		_, err = tx.ExecContext(ctx, `
			UPDATE incidents SET last_seen_at = ?, details_json = ?, updated_at = ?
			WHERE id = ?`, formatTime(at), incidentDetails(detail), formatTime(at), inc.ID)
		return inc, false, err
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return Incident{}, false, err
	}
	inc = Incident{
		ID:          fmt.Sprintf("inc_%d_%s_%s", at.UnixNano(), domainID, kind),
		DomainID:    domainID,
		Kind:        kind,
		State:       "open",
		OpenedAt:    at,
		LastSeenAt:  at,
		DetailsJSON: incidentDetails(detail),
		CreatedAt:   at,
		UpdatedAt:   at,
	}
	_, err = tx.ExecContext(ctx, `
		INSERT INTO incidents (id, domain_id, kind, state, opened_at, last_seen_at, details_json, created_at, updated_at)
		VALUES (?, ?, ?, 'open', ?, ?, ?, ?, ?)`,
		inc.ID, domainID, kind, formatTime(at), formatTime(at), inc.DetailsJSON,
		formatTime(at), formatTime(at))
	if err != nil {
		return Incident{}, false, err
	}
	return inc, true, nil
}

func resolveIncidentsTx(ctx context.Context, tx *sql.Tx, domainID string, at time.Time, kinds ...string) ([]Incident, error) {
	if len(kinds) == 0 {
		return nil, nil
	}
	placeholders := ""
	args := []any{}
	for i, kind := range kinds {
		if i > 0 {
			placeholders += ","
		}
		placeholders += "?"
		args = append(args, kind)
	}
	args = append([]any{domainID}, args...)
	rows, err := tx.QueryContext(ctx, `
		SELECT id, domain_id, kind, state, opened_at, resolved_at, last_seen_at,
			details_json, created_at, updated_at
		FROM incidents WHERE domain_id = ? AND state = 'open' AND kind IN (`+placeholders+`)`, args...)
	if err != nil {
		return nil, err
	}
	var incidents []Incident
	for rows.Next() {
		inc, err := scanIncident(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		incidents = append(incidents, inc)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	for i := range incidents {
		if _, err := tx.ExecContext(ctx, `
			UPDATE incidents SET state = 'resolved', resolved_at = ?, updated_at = ?
			WHERE id = ?`, formatTime(at), formatTime(at), incidents[i].ID); err != nil {
			return nil, err
		}
		incidents[i].State = "resolved"
		incidents[i].ResolvedAt = &at
		incidents[i].UpdatedAt = at
	}
	return incidents, nil
}

func incidentDetails(detail string) string {
	if detail == "" {
		return "{}"
	}
	body, err := json.Marshal(map[string]string{"message": detail})
	if err != nil {
		return "{}"
	}
	return string(body)
}

func scanIncident(scanner rowScanner) (Incident, error) {
	var inc Incident
	var resolvedAt sql.NullString
	var openedAt, lastSeen, createdAt, updatedAt string
	err := scanner.Scan(&inc.ID, &inc.DomainID, &inc.Kind, &inc.State, &openedAt,
		&resolvedAt, &lastSeen, &inc.DetailsJSON, &createdAt, &updatedAt)
	if err != nil {
		return Incident{}, err
	}
	inc.OpenedAt = parseTime(openedAt)
	inc.ResolvedAt = scanTime(resolvedAt)
	inc.LastSeenAt = parseTime(lastSeen)
	inc.CreatedAt = parseTime(createdAt)
	inc.UpdatedAt = parseTime(updatedAt)
	return inc, nil
}
