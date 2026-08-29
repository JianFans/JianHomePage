package postgres

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"sync"
	"testing"
)

const adapterTestDriverName = "yujian-postgres-adapter-test"

var (
	registerAdapterTestDriver sync.Once
	adapterState              = &adapterDriverState{}
)

type adapterDriverState struct {
	mu         sync.Mutex
	committed  bool
	rolledBack bool
}

type adapterTestDriver struct{}

func (adapterTestDriver) Open(string) (driver.Conn, error) {
	return &adapterTestConn{state: adapterState}, nil
}

type adapterTestConn struct{ state *adapterDriverState }

func (*adapterTestConn) Prepare(string) (driver.Stmt, error) { return nil, errors.New("not supported") }
func (*adapterTestConn) Close() error                        { return nil }
func (conn *adapterTestConn) Begin() (driver.Tx, error) {
	return &adapterTestTx{state: conn.state}, nil
}
func (conn *adapterTestConn) BeginTx(context.Context, driver.TxOptions) (driver.Tx, error) {
	return &adapterTestTx{state: conn.state}, nil
}
func (*adapterTestConn) Ping(context.Context) error { return nil }
func (*adapterTestConn) ExecContext(context.Context, string, []driver.NamedValue) (driver.Result, error) {
	return driver.RowsAffected(1), nil
}
func (*adapterTestConn) QueryContext(context.Context, string, []driver.NamedValue) (driver.Rows, error) {
	return &adapterTestRows{remaining: 1}, nil
}

type adapterTestTx struct{ state *adapterDriverState }

func (tx *adapterTestTx) Commit() error {
	tx.state.mu.Lock()
	defer tx.state.mu.Unlock()
	tx.state.committed = true
	return nil
}
func (tx *adapterTestTx) Rollback() error {
	tx.state.mu.Lock()
	defer tx.state.mu.Unlock()
	tx.state.rolledBack = true
	return nil
}

type adapterTestRows struct{ remaining int }

func (*adapterTestRows) Columns() []string { return []string{"value"} }
func (*adapterTestRows) Close() error      { return nil }
func (rows *adapterTestRows) Next(dest []driver.Value) error {
	if rows.remaining == 0 {
		return io.EOF
	}
	rows.remaining--
	dest[0] = "ok"
	return nil
}

func TestSQLExecutorAdaptsDatabaseAndTransaction(t *testing.T) {
	registerAdapterTestDriver.Do(func() { sql.Register(adapterTestDriverName, adapterTestDriver{}) })
	database, err := sql.Open(adapterTestDriverName, "test")
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	defer database.Close()

	executor, err := NewExecutor(database)
	if err != nil {
		t.Fatalf("new executor: %v", err)
	}
	result, err := executor.ExecContext(context.Background(), "UPDATE example")
	if err != nil {
		t.Fatalf("exec: %v", err)
	}
	if affected, err := result.RowsAffected(); err != nil || affected != 1 {
		t.Fatalf("unexpected result affected=%d err=%v", affected, err)
	}
	var value string
	if err := executor.QueryRowContext(context.Background(), "SELECT value").Scan(&value); err != nil || value != "ok" {
		t.Fatalf("unexpected query value=%q err=%v", value, err)
	}

	tx, err := executor.BeginTx(context.Background())
	if err != nil {
		t.Fatalf("begin transaction: %v", err)
	}
	if _, err := tx.ExecContext(context.Background(), "UPDATE example"); err != nil {
		t.Fatalf("transaction exec: %v", err)
	}
	if err := tx.Commit(context.Background()); err != nil {
		t.Fatalf("commit: %v", err)
	}
	adapterState.mu.Lock()
	committed := adapterState.committed
	adapterState.mu.Unlock()
	if !committed {
		t.Fatal("database transaction was not committed")
	}
}

func TestNewExecutorRejectsNilDatabase(t *testing.T) {
	if _, err := NewExecutor(nil); err == nil {
		t.Fatal("expected nil database error")
	}
}

func TestOpenRejectsEmptyDatabaseURL(t *testing.T) {
	if _, err := Open(context.Background(), ""); err == nil {
		t.Fatal("expected empty database URL error")
	}
}
