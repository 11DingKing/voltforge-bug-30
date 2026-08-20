package storage

import (
	"context"
	"database/sql"
	"errors"
	"time"
	"voltforge/internal/auth"
)

var errAuthNotFound = errors.New("auth record not found")

func (s *sqliteStore) FindUser(ctx context.Context, username string) (auth.User, error) {
	var user auth.User
	err := s.db.QueryRowContext(ctx, `SELECT id, username, password_hash, role FROM auth_users WHERE username = ?`, username).
		Scan(&user.ID, &user.Username, &user.PasswordHash, &user.Role)
	if errors.Is(err, sql.ErrNoRows) {
		return auth.User{}, errAuthNotFound
	}
	return user, err
}

func (s *sqliteStore) SaveSession(ctx context.Context, session auth.Session) error {
	var revoked any
	if session.RevokedAt != nil {
		revoked = session.RevokedAt.Format(time.RFC3339Nano)
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO auth_sessions(id, user_id, role, expires_at, revoked_at, created_at) VALUES(?,?,?,?,?,?)`,
		session.ID, session.UserID, session.Role, session.ExpiresAt.Format(time.RFC3339Nano), revoked, s.clock.Now().Format(time.RFC3339Nano))
	return err
}

func (s *sqliteStore) FindSession(ctx context.Context, token string) (auth.Session, error) {
	var session auth.Session
	var expires string
	var revoked sql.NullString
	err := s.db.QueryRowContext(ctx, `SELECT id, user_id, role, expires_at, revoked_at FROM auth_sessions WHERE id = ?`, token).
		Scan(&session.ID, &session.UserID, &session.Role, &expires, &revoked)
	if errors.Is(err, sql.ErrNoRows) {
		return auth.Session{}, errAuthNotFound
	}
	if err != nil {
		return auth.Session{}, err
	}
	session.ExpiresAt, err = time.Parse(time.RFC3339Nano, expires)
	if err != nil {
		return auth.Session{}, err
	}
	if revoked.Valid {
		at, parseErr := time.Parse(time.RFC3339Nano, revoked.String)
		if parseErr != nil {
			return auth.Session{}, parseErr
		}
		session.RevokedAt = &at
	}
	return session, nil
}

func (s *sqliteStore) RevokeSession(ctx context.Context, token string, revokedAt time.Time) error {
	result, err := s.db.ExecContext(ctx, `UPDATE auth_sessions SET revoked_at = ? WHERE id = ? AND revoked_at IS NULL`, revokedAt.Format(time.RFC3339Nano), token)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil || count != 1 {
		return errAuthNotFound
	}
	return nil
}
