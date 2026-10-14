package memory

import (
	"context"
	"sync"
	"time"
	"yujian.me/server/internal/auth"
	"yujian.me/server/internal/domain"
)

// SessionStore is an explicitly selected development-only auth repository.
type SessionStore struct {
	mu       sync.Mutex
	sessions map[string]auth.Session
	flows    map[string]time.Time
}

// NewSessionStore creates isolated ephemeral authentication storage.
func NewSessionStore() *SessionStore {
	return &SessionStore{sessions: map[string]auth.Session{}, flows: map[string]time.Time{}}
}

// CreateSession persists an independent identity copy indexed by bearer hash.
func (s *SessionStore) CreateSession(ctx context.Context, value auth.Session) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	value.Principal.Roles = append([]string(nil), value.Principal.Roles...)
	s.sessions[value.TokenHash] = value
	return nil
}

// GetSession returns a copy so callers cannot mutate stored role grants.
func (s *SessionStore) GetSession(ctx context.Context, hash string) (auth.Session, error) {
	if err := ctx.Err(); err != nil {
		return auth.Session{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.sessions[hash]
	if !ok {
		return auth.Session{}, domain.ErrNotFound
	}
	value.Principal.Roles = append([]string(nil), value.Principal.Roles...)
	return value, nil
}

// DeleteSession revokes the bearer hash; repeated revocation is harmless.
func (s *SessionStore) DeleteSession(ctx context.Context, hash string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, hash)
	return nil
}

// ConsumeLoginFlow atomically rejects replay of a validated state cookie.
func (s *SessionStore) ConsumeLoginFlow(ctx context.Context, hash string, expiry time.Time) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for key, until := range s.flows {
		if time.Now().After(until) {
			delete(s.flows, key)
		}
	}
	if _, ok := s.flows[hash]; ok {
		return domain.ErrConflict
	}
	s.flows[hash] = expiry
	return nil
}
