package postgres

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type execFake struct {
	calls   []string
	failOn  int
	failErr error
}

func (fake *execFake) ExecContext(_ context.Context, statement string, _ ...any) error {
	fake.calls = append(fake.calls, statement)
	if fake.failOn > 0 && len(fake.calls) == fake.failOn {
		return fake.failErr
	}
	return nil
}

func TestMigrateLocksTransactionAndRunsInitialSchema(t *testing.T) {
	fake := &execFake{}
	if err := Migrate(context.Background(), fake); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if len(fake.calls) < 4 {
		t.Fatalf("expected transaction and migration statements, got %d", len(fake.calls))
	}
	if fake.calls[0] != "BEGIN" || !strings.Contains(fake.calls[1], "pg_advisory_xact_lock") || fake.calls[len(fake.calls)-1] != "COMMIT" {
		t.Fatalf("unexpected migration sequence %#v", fake.calls)
	}
	if !strings.Contains(strings.Join(fake.calls, "\n"), "CREATE TABLE IF NOT EXISTS content_versions") {
		t.Fatal("initial migration did not create content_versions")
	}
}

func TestMigrateRollsBackWhenStatementFails(t *testing.T) {
	fake := &execFake{failOn: 3, failErr: errors.New("database unavailable")}
	if err := Migrate(context.Background(), fake); !errors.Is(err, fake.failErr) {
		t.Fatalf("expected migration error, got %v", err)
	}
	if fake.calls[len(fake.calls)-1] != "ROLLBACK" {
		t.Fatalf("expected rollback, calls=%#v", fake.calls)
	}
}

func TestMigrateStopsWhenContextIsCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := Migrate(ctx, &execFake{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation, got %v", err)
	}
}
