package postgres

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

const publisherDriverName = "yujian-publisher-lock-test"

var publisherDriverOnce sync.Once
var publisherDriverStates sync.Map
var publisherDriverCounter atomic.Int64

type publisherDriverState struct {
	locked, unlock                   bool
	lockErr, unlockErr, operationErr error
	queries                          []string
	closes, execs                    int
}
type publisherDriver struct{}

// Open supplies an isolated physical connection state for each test database.
func (publisherDriver) Open(key string) (driver.Conn, error) {
	state, _ := publisherDriverStates.Load(key)
	return &publisherDriverConnection{state: state.(*publisherDriverState)}, nil
}

type publisherDriverConnection struct{ state *publisherDriverState }

// Prepare rejects statement APIs so tests exercise the production context-aware SQL path.
func (*publisherDriverConnection) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("unsupported")
}

// Close records destruction of a physical connection after a failed session lock.
func (c *publisherDriverConnection) Close() error { c.state.closes++; return nil }

// Begin detects accidental transactions spanning a worker callback.
func (*publisherDriverConnection) Begin() (driver.Tx, error) {
	return nil, errors.New("transactions forbidden")
}

// ExecContext counts physical mutations, enabling assertions that lost sessions refuse further writes.
func (c *publisherDriverConnection) ExecContext(context.Context, string, []driver.NamedValue) (driver.Result, error) {
	c.state.execs++
	return driver.RowsAffected(1), c.state.operationErr
}

// QueryContext models lock acquisition, unlock and subsequent session reads.
func (c *publisherDriverConnection) QueryContext(_ context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	c.state.queries = append(c.state.queries, query)
	if strings.Contains(query, "pg_try_advisory_lock") {
		if len(args) != 1 || args[0].Value != publisherLockID {
			return nil, errors.New("wrong shared lock ID")
		}
		if c.state.lockErr != nil {
			return nil, c.state.lockErr
		}
		return &publisherDriverRows{value: c.state.locked}, nil
	}
	if strings.Contains(query, "pg_advisory_unlock") {
		if c.state.unlockErr != nil {
			return nil, c.state.unlockErr
		}
		return &publisherDriverRows{value: c.state.unlock}, nil
	}
	if c.state.operationErr != nil {
		return nil, c.state.operationErr
	}
	return &publisherDriverRows{value: "ok"}, nil
}

type publisherDriverRows struct {
	value driver.Value
	read  bool
}

// Columns exposes the one-value query result used by lock and status reads.
func (*publisherDriverRows) Columns() []string { return []string{"value"} }

// Close implements the driver's stream cleanup contract.
func (*publisherDriverRows) Close() error { return nil }

// Next returns one row, followed by EOF, like PostgreSQL's lock SELECT.
func (r *publisherDriverRows) Next(dest []driver.Value) error {
	if r.read {
		return io.EOF
	}
	r.read = true
	dest[0] = r.value
	return nil
}

// publisherLockDatabase creates an isolated database/sql pool over the fault-injection driver.
func publisherLockDatabase(t *testing.T, state *publisherDriverState) *Database {
	t.Helper()
	publisherDriverOnce.Do(func() { sql.Register(publisherDriverName, publisherDriver{}) })
	key := fmt.Sprintf("state-%d", publisherDriverCounter.Add(1))
	publisherDriverStates.Store(key, state)
	t.Cleanup(func() { publisherDriverStates.Delete(key) })
	db, err := sql.Open(publisherDriverName, key)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	exec, err := NewExecutor(db)
	if err != nil {
		t.Fatal(err)
	}
	return &Database{database: db, executor: exec}
}

// TestPublisherLockRefusesWritesAfterSessionError simulates disconnect during a cloud operation.
func TestPublisherLockRefusesWritesAfterSessionError(t *testing.T) {
	state := &publisherDriverState{locked: true, unlock: true, operationErr: errors.New("transport disconnected")}
	db := publisherLockDatabase(t, state)
	if err := db.WithPublisherLock(t.Context(), func(exec Executor) error {
		if _, err := exec.ExecContext(t.Context(), "UPDATE task"); err == nil {
			t.Fatal("expected disconnect")
		}
		if _, err := exec.ExecContext(t.Context(), "UPDATE task again"); err == nil {
			t.Fatal("lost session allowed a second write")
		}
		var value string
		if err := exec.QueryRowContext(t.Context(), "SELECT task").Scan(&value); err == nil {
			t.Fatal("lost session allowed a read")
		}
		if _, err := exec.QueryContext(t.Context(), "SELECT tasks"); err == nil {
			t.Fatal("lost session allowed rows")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if state.execs != 1 || state.closes != 1 || len(state.queries) != 1 {
		t.Fatalf("session not destroyed or reused: %#v", state)
	}
}

// TestPublisherLockDestroysAmbiguousSessions never pools a session with a possibly retained server lock.
func TestPublisherLockDestroysAmbiguousSessions(t *testing.T) {
	for _, name := range []string{"lock-error", "unlock-error", "unlock-false"} {
		t.Run(name, func(t *testing.T) {
			state := &publisherDriverState{locked: true, unlock: true}
			switch name {
			case "lock-error":
				state.lockErr = errors.New("lock timeout")
			case "unlock-error":
				state.unlockErr = errors.New("unlock timeout")
			case "unlock-false":
				state.unlock = false
			}
			db := publisherLockDatabase(t, state)
			called := false
			err := db.WithPublisherLock(t.Context(), func(Executor) error { called = true; return nil })
			if err == nil || state.closes != 1 || called == (name == "lock-error") {
				t.Fatalf("unsafe cleanup %#v called=%v err=%v", state, called, err)
			}
		})
	}
}

// TestPublisherLockBusySkipsCallback lets a second worker tick return without cloud side effects.
func TestPublisherLockBusySkipsCallback(t *testing.T) {
	state := &publisherDriverState{locked: false, unlock: true}
	db := publisherLockDatabase(t, state)
	if err := db.WithPublisherLock(t.Context(), func(Executor) error { t.Fatal("busy lock ran callback"); return nil }); err != nil {
		t.Fatal(err)
	}
	if len(state.queries) != 1 || state.closes != 0 {
		t.Fatalf("busy connection mishandled: %#v", state)
	}
}

// TestPublisherLockSuccessUsesAutocommitAndUnlock releases the pinned session after callback errors too.
func TestPublisherLockSuccessUsesAutocommitAndUnlock(t *testing.T) {
	state := &publisherDriverState{locked: true, unlock: true}
	db := publisherLockDatabase(t, state)
	failed := errors.New("operation failure")
	err := db.WithPublisherLock(t.Context(), func(exec Executor) error {
		if _, err := exec.BeginTx(t.Context()); err == nil {
			t.Fatal("worker transaction allowed")
		}
		var value string
		if err := exec.QueryRowContext(t.Context(), "SELECT task").Scan(&value); err != nil || value != "ok" {
			t.Fatalf("pinned read %q %v", value, err)
		}
		rows, err := exec.QueryContext(t.Context(), "SELECT tasks")
		if err != nil {
			t.Fatal(err)
		}
		if !rows.Next() {
			t.Fatal("missing row")
		}
		if err := rows.Scan(&value); err != nil {
			t.Fatal(err)
		}
		if rows.Next() || rows.Err() != nil {
			t.Fatal("unexpected streaming error")
		}
		if err := rows.Close(); err != nil {
			t.Fatal(err)
		}
		return failed
	})
	if !errors.Is(err, failed) || state.closes != 0 || !strings.Contains(state.queries[len(state.queries)-1], "pg_advisory_unlock") {
		t.Fatalf("cleanup lost callback error: %#v %v", state, err)
	}
}

// TestPublisherScanFailureInvalidatesSession covers deferred database/sql QueryRow transport errors.
func TestPublisherScanFailureInvalidatesSession(t *testing.T) {
	state := &publisherDriverState{locked: true, unlock: true, operationErr: errors.New("read failed")}
	db := publisherLockDatabase(t, state)
	if err := db.WithPublisherLock(t.Context(), func(exec Executor) error {
		var value string
		if err := exec.QueryRowContext(t.Context(), "SELECT task").Scan(&value); err == nil {
			t.Fatal("expected row failure")
		}
		if _, err := exec.ExecContext(t.Context(), "UPDATE task"); err == nil {
			t.Fatal("write after row failure allowed")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if state.execs != 0 || state.closes != 1 {
		t.Fatalf("scan error failed to invalidate session %#v", state)
	}
}
