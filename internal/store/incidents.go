package store

import (
	"context"
	"database/sql"
	"errors"
)

// ListIncidents returns incidents, optionally filtered by domain and state.
func (s *Store) ListIncidents(ctx context.Context, domainID, state string, limit int) ([]Incident, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	where := "1 = 1"
	args := []any{}
	if domainID != "" {
		where += " AND domain_id = ?"
		args = append(args, domainID)
	}
	if state == "open" || state == "resolved" {
		where += " AND state = ?"
		args = append(args, state)
	}
	args = append(args, limit)
	rows, err := s.Reader.QueryContext(ctx, `
		SELECT id, domain_id, kind, state, opened_at, resolved_at, last_seen_at,
			details_json, created_at, updated_at
		FROM incidents WHERE `+where+` ORDER BY state ASC, last_seen_at DESC LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var incidents []Incident
	for rows.Next() {
		inc, err := scanIncident(rows)
		if err != nil {
			return nil, err
		}
		incidents = append(incidents, inc)
	}
	return incidents, rows.Err()
}

// GetIncident returns one incident.
func (s *Store) GetIncident(ctx context.Context, id string) (Incident, error) {
	row := s.Reader.QueryRowContext(ctx, `
		SELECT id, domain_id, kind, state, opened_at, resolved_at, last_seen_at,
			details_json, created_at, updated_at FROM incidents WHERE id = ?`, id)
	inc, err := scanIncident(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Incident{}, ErrNotFound
	}
	return inc, err
}
