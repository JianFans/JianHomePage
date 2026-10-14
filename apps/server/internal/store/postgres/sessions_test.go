package postgres

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"
	"yujian.me/server/internal/auth"
	"yujian.me/server/internal/domain"
)

// TestSessionRepositoryPersistsHashAndRevokes verifies durable lookup, revocation
// and one-time state semantics without exposing browser bearer values to SQL.
func TestSessionRepositoryPersistsHashAndRevokes(t *testing.T) {
	expiry := time.Now().Add(time.Hour)
	exec := &recordingExecutor{result: recordingResult{affected: 1}}
	repo := NewSessionStore(exec)
	session := auth.Session{TokenHash: "hashed-bearer", Principal: auth.SessionPrincipal{Subject: "editor", Roles: []string{"editor"}}, CSRFToken: "csrf", ExpiresAt: expiry}
	if err := repo.CreateSession(context.Background(), session); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(exec.execQueries[0], "admin_sessions") || exec.execArgs[0][0] != "hashed-bearer" {
		t.Fatal("session not persisted by hash")
	}
	exec.row = recordingRow{values: []any{"hashed-bearer", []byte(`{"subject":"editor","roles":["editor"]}`), "csrf", expiry}}
	got, err := repo.GetSession(context.Background(), "hashed-bearer")
	if err != nil || got.Principal.Subject != "editor" {
		t.Fatalf("session scan: %#v %v", got, err)
	}
	if err := repo.DeleteSession(context.Background(), "hashed-bearer"); err != nil {
		t.Fatal(err)
	}
	if err := repo.ConsumeLoginFlow(context.Background(), "state-hash", expiry); err != nil {
		t.Fatal(err)
	}
	exec.result = recordingResult{affected: 0}
	if !errors.Is(repo.ConsumeLoginFlow(context.Background(), "state-hash", expiry), domain.ErrConflict) {
		t.Fatal("replayed state accepted")
	}
	exec.row = recordingRow{err: sql.ErrNoRows}
	if _, err := repo.GetSession(context.Background(), "missing"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatal("missing session error")
	}
	exec.execErr = errors.New("database unavailable")
	if repo.CreateSession(context.Background(), session) == nil || repo.DeleteSession(context.Background(), "hash") == nil || repo.ConsumeLoginFlow(context.Background(), "hash", expiry) == nil {
		t.Fatal("database failure swallowed")
	}
}
