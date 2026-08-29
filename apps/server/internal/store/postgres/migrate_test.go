package postgres

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type execFake struct {
	tx         *migrationTxFake
	beginCalls int
}

func (fake *execFake) ExecContext(context.Context, string, ...any) (ExecResult, error) {
	return nil, errors.New("migration executed outside transaction")
}
func (fake *execFake) QueryRowContext(context.Context, string, ...any) Row { return recordingRow{} }
func (fake *execFake) QueryContext(context.Context, string, ...any) (Rows, error) {
	return &recordingRows{}, nil
}
func (fake *execFake) BeginTx(context.Context) (Tx, error) {
	fake.beginCalls++
	if fake.tx == nil {
		fake.tx = &migrationTxFake{}
	}
	return fake.tx, nil
}

type migrationTxFake struct {
	calls      []string
	failOn     int
	failErr    error
	committed  bool
	rolledBack bool
}

func (fake *migrationTxFake) ExecContext(_ context.Context, statement string, _ ...any) (ExecResult, error) {
	fake.calls = append(fake.calls, statement)
	if fake.failOn > 0 && len(fake.calls) == fake.failOn {
		return nil, fake.failErr
	}
	return recordingResult{}, nil
}
func (fake *migrationTxFake) QueryRowContext(context.Context, string, ...any) Row {
	return recordingRow{}
}
func (fake *migrationTxFake) QueryContext(context.Context, string, ...any) (Rows, error) {
	return &recordingRows{}, nil
}
func (fake *migrationTxFake) BeginTx(context.Context) (Tx, error) {
	return nil, errors.New("nested transaction")
}
func (fake *migrationTxFake) Commit(context.Context) error {
	fake.committed = true
	return nil
}
func (fake *migrationTxFake) Rollback(context.Context) error {
	fake.rolledBack = true
	return nil
}

func TestMigrateLocksTransactionAndRunsInitialSchema(t *testing.T) {
	fake := &execFake{}
	if err := Migrate(context.Background(), fake); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if fake.beginCalls != 1 || fake.tx == nil {
		t.Fatalf("expected one database transaction, got %#v", fake)
	}
	if len(fake.tx.calls) < 2 || !strings.Contains(fake.tx.calls[0], "pg_advisory_xact_lock") {
		t.Fatalf("unexpected migration sequence %#v", fake.tx.calls)
	}
	if !strings.Contains(strings.Join(fake.tx.calls, "\n"), "CREATE TABLE IF NOT EXISTS content_versions") {
		t.Fatal("initial migration did not create content_versions")
	}
	if !fake.tx.committed || fake.tx.rolledBack {
		t.Fatalf("expected committed transaction, got %#v", fake.tx)
	}
}

func TestMigrateRollsBackWhenStatementFails(t *testing.T) {
	fake := &execFake{tx: &migrationTxFake{failOn: 2, failErr: errors.New("database unavailable")}}
	if err := Migrate(context.Background(), fake); !errors.Is(err, fake.tx.failErr) {
		t.Fatalf("expected migration error, got %v", err)
	}
	if !fake.tx.rolledBack || fake.tx.committed {
		t.Fatalf("expected rollback, tx=%#v", fake.tx)
	}
}

func TestMigrateStopsWhenContextIsCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := Migrate(ctx, &execFake{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context cancellation, got %v", err)
	}
}
