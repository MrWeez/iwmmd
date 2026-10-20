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

// EnqueueJobInput describes a durable job.
type EnqueueJobInput struct {
	Kind        string
	PayloadJSON string
	RunAt       time.Time
	MaxAttempts int
	UniqueKey   string
}

// EnqueueJob inserts a job unless an active job with the same unique key exists.
func (s *Store) EnqueueJob(ctx context.Context, in EnqueueJobInput) (Job, bool, error) {
	if in.MaxAttempts <= 0 {
		in.MaxAttempts = 5
	}
	if in.RunAt.IsZero() {
		in.RunAt = time.Now().UTC()
	}
	if in.PayloadJSON == "" {
		in.PayloadJSON = "{}"
	}
	now := time.Now().UTC()
	job := Job{
		ID:          ids.New(),
		Kind:        in.Kind,
		PayloadJSON: in.PayloadJSON,
		State:       "pending",
		RunAt:       in.RunAt.UTC(),
		MaxAttempts: in.MaxAttempts,
		UniqueKey:   in.UniqueKey,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	_, err := s.Writer.ExecContext(ctx, `
		INSERT INTO jobs (id, kind, payload_json, state, run_at, max_attempts, unique_key, created_at, updated_at)
		VALUES (?, ?, ?, 'pending', ?, ?, ?, ?, ?)`,
		job.ID, job.Kind, job.PayloadJSON, formatTime(job.RunAt), job.MaxAttempts,
		nullString(job.UniqueKey), formatTime(now), formatTime(now))
	if err == nil {
		return job, true, nil
	}
	if !strings.Contains(err.Error(), "UNIQUE constraint failed") {
		return Job{}, false, fmt.Errorf("enqueue job: %w", err)
	}
	if in.UniqueKey == "" {
		return Job{}, false, fmt.Errorf("enqueue job: %w", err)
	}
	existing, err := s.findActiveJobByKey(ctx, in.UniqueKey)
	if err != nil {
		return Job{}, false, err
	}
	return existing, false, nil
}

// ClaimJob leases the oldest ready job.
func (s *Store) ClaimJob(ctx context.Context, workerID string, now time.Time, lease time.Duration) (*Job, error) {
	if lease <= 0 {
		lease = 5 * time.Minute
	}
	if err := s.RecoverExpiredLeases(ctx, now); err != nil {
		return nil, err
	}
	tx, err := s.Writer.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var id string
	err = tx.QueryRowContext(ctx, `
		SELECT id FROM jobs WHERE state = 'pending' AND run_at <= ?
		ORDER BY run_at ASC, created_at ASC LIMIT 1`, formatTime(now)).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	lockedUntil := now.Add(lease)
	result, err := tx.ExecContext(ctx, `
		UPDATE jobs SET state = 'running', locked_by = ?, locked_until = ?,
			attempts = attempts + 1, updated_at = ?
		WHERE id = ? AND state = 'pending'`,
		workerID, formatTime(lockedUntil), formatTime(now), id)
	if err != nil {
		return nil, err
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return nil, nil
	}
	row := tx.QueryRowContext(ctx, `SELECT `+jobColumns+` FROM jobs WHERE id = ?`, id)
	job, err := scanJob(row)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &job, nil
}

// CompleteJob marks a job succeeded.
func (s *Store) CompleteJob(ctx context.Context, id string, at time.Time) error {
	result, err := s.Writer.ExecContext(ctx, `
		UPDATE jobs SET state = 'succeeded', locked_by = NULL, locked_until = NULL,
			finished_at = ?, updated_at = ? WHERE id = ? AND state = 'running'`,
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

// FailJob records an error and schedules a retry or marks the job dead.
func (s *Store) FailJob(ctx context.Context, id, message string, retryAt time.Time, at time.Time) error {
	tx, err := s.Writer.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var attempts, maxAttempts int
	if err := tx.QueryRowContext(ctx, `SELECT attempts, max_attempts FROM jobs WHERE id = ?`, id).
		Scan(&attempts, &maxAttempts); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	state := "pending"
	finished := any(nil)
	if attempts >= maxAttempts {
		state = "dead"
		finished = formatTime(at)
	}
	_, err = tx.ExecContext(ctx, `
		UPDATE jobs SET state = ?, run_at = ?, locked_by = NULL, locked_until = NULL,
			last_error = ?, updated_at = ?, finished_at = COALESCE(?, finished_at)
		WHERE id = ?`,
		state, formatTime(retryAt), nullString(message), formatTime(at), finished, id)
	if err != nil {
		return err
	}
	return tx.Commit()
}

// RecoverExpiredLeases returns abandoned running jobs to the queue.
func (s *Store) RecoverExpiredLeases(ctx context.Context, now time.Time) error {
	if _, err := s.Writer.ExecContext(ctx, `
		UPDATE jobs SET state = CASE WHEN attempts >= max_attempts THEN 'dead' ELSE 'pending' END,
			locked_by = NULL, locked_until = NULL, last_error = 'worker lease expired',
			finished_at = CASE WHEN attempts >= max_attempts THEN ? ELSE finished_at END,
			updated_at = ?
		WHERE state = 'running' AND locked_until IS NOT NULL AND locked_until < ?`,
		formatTime(now), formatTime(now), formatTime(now)); err != nil {
		return err
	}
	return nil
}

// CancelJobsByKey cancels active jobs matching a unique key.
func (s *Store) CancelJobsByKey(ctx context.Context, uniqueKey string, at time.Time) error {
	_, err := s.Writer.ExecContext(ctx, `
		UPDATE jobs SET state = 'canceled', locked_by = NULL, locked_until = NULL,
			finished_at = ?, updated_at = ?
		WHERE unique_key = ? AND state IN ('pending', 'running')`,
		formatTime(at), formatTime(at), uniqueKey)
	return err
}

// ListJobs returns recent jobs.
func (s *Store) ListJobs(ctx context.Context, limit int) ([]Job, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := s.Reader.QueryContext(ctx, `SELECT `+jobColumns+` FROM jobs
		ORDER BY created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var jobs []Job
	for rows.Next() {
		job, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	return jobs, rows.Err()
}

// FindActiveJob returns the active job for a unique key.
func (s *Store) FindActiveJob(ctx context.Context, key string) (Job, bool, error) {
	job, err := s.findActiveJobByKey(ctx, key)
	if errors.Is(err, ErrNotFound) || errors.Is(err, sql.ErrNoRows) {
		return Job{}, false, nil
	}
	if err != nil {
		return Job{}, false, err
	}
	return job, true, nil
}

func (s *Store) findActiveJobByKey(ctx context.Context, key string) (Job, error) {
	row := s.Reader.QueryRowContext(ctx, `SELECT `+jobColumns+` FROM jobs
		WHERE unique_key = ? AND state IN ('pending', 'running') ORDER BY created_at DESC LIMIT 1`, key)
	return scanJob(row)
}

const jobColumns = `
	id, kind, payload_json, state, run_at, attempts, max_attempts, COALESCE(locked_by, ''),
	locked_until, COALESCE(last_error, ''), COALESCE(unique_key, ''), created_at, updated_at, finished_at`

func scanJob(scanner rowScanner) (Job, error) {
	var job Job
	var lockedBy, lastError, uniqueKey, lockedUntil, finishedAt sql.NullString
	var runAt, createdAt, updatedAt string
	err := scanner.Scan(&job.ID, &job.Kind, &job.PayloadJSON, &job.State, &runAt,
		&job.Attempts, &job.MaxAttempts, &lockedBy, &lockedUntil, &lastError, &uniqueKey,
		&createdAt, &updatedAt, &finishedAt)
	if err != nil {
		return Job{}, err
	}
	job.LockedBy = scanString(lockedBy)
	job.LastError = scanString(lastError)
	job.UniqueKey = scanString(uniqueKey)
	job.RunAt = parseTime(runAt)
	job.LockedUntil = scanTime(lockedUntil)
	job.CreatedAt = parseTime(createdAt)
	job.UpdatedAt = parseTime(updatedAt)
	job.FinishedAt = scanTime(finishedAt)
	return job, nil
}
