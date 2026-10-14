package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
	"yujian.me/server/internal/auth"
	"yujian.me/server/internal/domain"
)

// SessionStore shares durable authentication state across application instances.
type SessionStore struct{ exec Executor }

// NewSessionStore binds auth persistence to the configured PostgreSQL executor.
func NewSessionStore(exec Executor) *SessionStore { return &SessionStore{exec: exec} }

// CreateSession stores the opaque bearer hash and its server-owned identity.
func (s *SessionStore) CreateSession(ctx context.Context, value auth.Session) error {
	principal, err := json.Marshal(value.Principal)
	if err != nil {
		return err
	}
	_, err = s.exec.ExecContext(ctx, `INSERT INTO admin_sessions (token_hash, principal, csrf_token, expires_at) VALUES ($1, $2::jsonb, $3, $4)`, value.TokenHash, principal, value.CSRFToken, value.ExpiresAt)
	return err
}

// GetSession reads only unexpired records; callers also check their own clock.
func (s *SessionStore) GetSession(ctx context.Context, hash string) (auth.Session, error) {
	var value auth.Session
	var principal []byte
	err := s.exec.QueryRowContext(ctx, `SELECT token_hash, principal, csrf_token, expires_at FROM admin_sessions WHERE token_hash=$1 AND expires_at>now()`, hash).Scan(&value.TokenHash, &principal, &value.CSRFToken, &value.ExpiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return auth.Session{}, domain.ErrNotFound
	}
	if err != nil {
		return auth.Session{}, err
	}
	if err := json.Unmarshal(principal, &value.Principal); err != nil {
		return auth.Session{}, err
	}
	return value, nil
}

// DeleteSession permanently revokes the hash, including on concurrent instances.
func (s *SessionStore) DeleteSession(ctx context.Context, hash string) error {
	_, err := s.exec.ExecContext(ctx, `DELETE FROM admin_sessions WHERE token_hash=$1`, hash)
	return err
}

// ConsumeLoginFlow uses a unique state hash to atomically prevent callback replay.
func (s *SessionStore) ConsumeLoginFlow(ctx context.Context, hash string, expiry time.Time) error {
	result, err := s.exec.ExecContext(ctx, `INSERT INTO admin_login_flows (state_hash, expires_at) VALUES ($1,$2) ON CONFLICT (state_hash) DO NOTHING`, hash, expiry)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count != 1 {
		return domain.ErrConflict
	}
	return nil
}
