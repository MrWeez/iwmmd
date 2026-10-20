package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/mrweez/iwmmd/internal/platform/ids"
)

// UpsertUser creates or updates an OIDC user and returns the stored record.
func (s *Store) UpsertUser(ctx context.Context, u User, at time.Time) (User, error) {
	existing, err := s.findUserBySubject(ctx, u.Issuer, u.Subject)
	if err == nil {
		u.ID = existing.ID
		u.CreatedAt = existing.CreatedAt
		u.LastLoginAt = &at
		_, err = s.Writer.ExecContext(ctx, `
			UPDATE users SET email = ?, display_name = ?, role = ?, last_login_at = ?,
				updated_at = ? WHERE id = ?`,
			u.Email, nullString(u.DisplayName), u.Role, formatTime(at), formatTime(at), u.ID)
		if err != nil {
			return User{}, err
		}
		u.UpdatedAt = at
		return u, nil
	}
	if !errors.Is(err, ErrNotFound) {
		return User{}, err
	}
	u.ID = ids.New()
	u.CreatedAt = at
	u.UpdatedAt = at
	u.LastLoginAt = &at
	_, err = s.Writer.ExecContext(ctx, `
		INSERT INTO users (id, oidc_issuer, oidc_subject, email, display_name, role,
			last_login_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		u.ID, u.Issuer, u.Subject, u.Email, nullString(u.DisplayName), u.Role,
		formatTime(at), formatTime(at), formatTime(at))
	if err != nil {
		return User{}, err
	}
	return u, nil
}

// GetUser returns a user by ID.
func (s *Store) GetUser(ctx context.Context, id string) (User, error) {
	row := s.Reader.QueryRowContext(ctx, `SELECT `+userColumns+` FROM users WHERE id = ?`, id)
	return scanUser(row)
}

// ListUsers returns all users.
func (s *Store) ListUsers(ctx context.Context) ([]User, error) {
	rows, err := s.Reader.QueryContext(ctx, `SELECT `+userColumns+` FROM users ORDER BY email, display_name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var users []User
	for rows.Next() {
		user, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	return users, rows.Err()
}

// SetUserRole updates a user's role.
func (s *Store) SetUserRole(ctx context.Context, id, role string, at time.Time) error {
	result, err := s.Writer.ExecContext(ctx, `UPDATE users SET role = ?, updated_at = ? WHERE id = ?`,
		role, formatTime(at), id)
	if err != nil {
		return err
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return ErrNotFound
	}
	return nil
}

// CreateSession stores a hashed session token.
func (s *Store) CreateSession(ctx context.Context, session Session) error {
	_, err := s.Writer.ExecContext(ctx, `
		INSERT INTO sessions (id, user_id, created_at, expires_at, ip, user_agent)
		VALUES (?, ?, ?, ?, ?, ?)`,
		session.ID, session.UserID, formatTime(session.CreatedAt), formatTime(session.ExpiresAt),
		nullString(session.IP), nullString(session.UserAgent))
	return err
}

// GetSessionUser returns the session and user for a hashed token.
func (s *Store) GetSessionUser(ctx context.Context, id string, now time.Time) (Session, User, error) {
	row := s.Reader.QueryRowContext(ctx, `
		SELECT s.id, s.user_id, s.created_at, s.expires_at, s.last_seen_at,
			COALESCE(s.ip, ''), COALESCE(s.user_agent, ''), s.revoked_at,
			u.id, u.oidc_issuer, u.oidc_subject, u.email, COALESCE(u.display_name, ''),
			u.role, u.last_login_at, u.created_at, u.updated_at
		FROM sessions s JOIN users u ON u.id = s.user_id
		WHERE s.id = ? AND s.revoked_at IS NULL AND s.expires_at > ?`, id, formatTime(now))
	var session Session
	var user User
	var sessCreated, sessExpires, userCreated, userUpdated string
	var sessLastSeen, sessRevoked, userLastLogin sql.NullString
	var ip, agent, displayName sql.NullString
	err := row.Scan(&session.ID, &session.UserID, &sessCreated, &sessExpires, &sessLastSeen,
		&ip, &agent, &sessRevoked,
		&user.ID, &user.Issuer, &user.Subject, &user.Email, &displayName, &user.Role,
		&userLastLogin, &userCreated, &userUpdated)
	if errors.Is(err, sql.ErrNoRows) {
		return Session{}, User{}, ErrNotFound
	}
	if err != nil {
		return Session{}, User{}, err
	}
	session.CreatedAt = parseTime(sessCreated)
	session.ExpiresAt = parseTime(sessExpires)
	session.LastSeenAt = scanTime(sessLastSeen)
	session.IP = scanString(ip)
	session.UserAgent = scanString(agent)
	session.RevokedAt = scanTime(sessRevoked)
	user.DisplayName = scanString(displayName)
	user.LastLoginAt = scanTime(userLastLogin)
	user.CreatedAt = parseTime(userCreated)
	user.UpdatedAt = parseTime(userUpdated)
	return session, user, nil
}

// TouchSession updates last_seen_at.
func (s *Store) TouchSession(ctx context.Context, id string, at time.Time) error {
	_, err := s.Writer.ExecContext(ctx, `UPDATE sessions SET last_seen_at = ? WHERE id = ?`, formatTime(at), id)
	return err
}

// DeleteSession revokes one session.
func (s *Store) DeleteSession(ctx context.Context, id string, at time.Time) error {
	_, err := s.Writer.ExecContext(ctx, `UPDATE sessions SET revoked_at = ? WHERE id = ?`, formatTime(at), id)
	return err
}

// RevokeUserSessions revokes every active session for a user, optionally
// keeping one session ID.
func (s *Store) RevokeUserSessions(ctx context.Context, userID, keepSessionID string, at time.Time) error {
	_, err := s.Writer.ExecContext(ctx, `
		UPDATE sessions SET revoked_at = ? WHERE user_id = ? AND revoked_at IS NULL AND id != ?`,
		formatTime(at), userID, keepSessionID)
	return err
}

// PruneSessions removes expired and revoked sessions older than cutoff.
func (s *Store) PruneSessions(ctx context.Context, cutoff time.Time) error {
	_, err := s.Writer.ExecContext(ctx, `
		DELETE FROM sessions WHERE expires_at < ? OR (revoked_at IS NOT NULL AND revoked_at < ?)`,
		formatTime(cutoff), formatTime(cutoff))
	return err
}

const userColumns = `
	id, oidc_issuer, oidc_subject, email, COALESCE(display_name, ''), role,
	last_login_at, created_at, updated_at`

func scanUser(scanner rowScanner) (User, error) {
	var user User
	var displayName, lastLogin sql.NullString
	var createdAt, updatedAt string
	err := scanner.Scan(&user.ID, &user.Issuer, &user.Subject, &user.Email, &displayName,
		&user.Role, &lastLogin, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, ErrNotFound
	}
	if err != nil {
		return User{}, err
	}
	user.DisplayName = scanString(displayName)
	user.LastLoginAt = scanTime(lastLogin)
	user.CreatedAt = parseTime(createdAt)
	user.UpdatedAt = parseTime(updatedAt)
	return user, nil
}

func (s *Store) findUserBySubject(ctx context.Context, issuer, subject string) (User, error) {
	row := s.Reader.QueryRowContext(ctx, `SELECT `+userColumns+`
		FROM users WHERE oidc_issuer = ? AND oidc_subject = ?`, issuer, subject)
	return scanUser(row)
}
