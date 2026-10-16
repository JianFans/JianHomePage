package postgres

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"sync/atomic"
	"time"
)

// publisherLockID must match the retired Node worker during rolling upgrades.
const publisherLockID int64 = 865734220

// WithPublisherLock pins a physical session across cloud calls while individual
// statements autocommit. Failed lock acquisition/unlock destroys the session so
// a server-side lock can never silently return to the connection pool.
func (database *Database) WithPublisherLock(ctx context.Context, run func(Executor) error) (result error) {
	if database.database == nil {
		return errors.New("Publisher requires a physical database connection")
	}
	connection, err := database.database.Conn(ctx)
	if err != nil {
		return err
	}
	guard := &publisherConnection{connection: connection}
	locked := false
	defer func() {
		if locked && !guard.lost.Load() {
			unlockCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			var unlocked bool
			err := connection.QueryRowContext(unlockCtx, `SELECT pg_advisory_unlock($1)`, publisherLockID).Scan(&unlocked)
			cancel()
			if err != nil || !unlocked {
				guard.lost.Store(true)
				result = errors.New("Publisher lock release failed")
			}
		}
		if guard.lost.Load() {
			_ = connection.Raw(func(any) error { return driver.ErrBadConn })
		}
		_ = connection.Close()
	}()
	if err := connection.QueryRowContext(ctx, `SELECT pg_try_advisory_lock($1)`, publisherLockID).Scan(&locked); err != nil {
		guard.lost.Store(true)
		return err
	}
	if !locked {
		return nil
	}
	return run(guard)
}

// publisherConnection refuses further operations after any SQL session error;
// sql.Conn itself never swaps in another pooled connection after disconnect.
type publisherConnection struct {
	connection *sql.Conn
	lost       atomic.Bool
}

// ExecContext executes autocommit writes only while the lock session is usable.
func (c *publisherConnection) ExecContext(ctx context.Context, query string, args ...any) (ExecResult, error) {
	if c.lost.Load() {
		return nil, errors.New("Publisher lock lost")
	}
	result, err := c.connection.ExecContext(ctx, query, args...)
	if err != nil {
		c.lost.Store(true)
	}
	return result, err
}

// QueryRowContext guards both query dispatch and delayed Scan failures.
func (c *publisherConnection) QueryRowContext(ctx context.Context, query string, args ...any) Row {
	if c.lost.Load() {
		return publisherLockRow{err: errors.New("Publisher lock lost")}
	}
	return publisherLockRow{row: c.connection.QueryRowContext(ctx, query, args...), owner: c}
}

// QueryContext returns rows only from the same exclusive session.
func (c *publisherConnection) QueryContext(ctx context.Context, query string, args ...any) (Rows, error) {
	if c.lost.Load() {
		return nil, errors.New("Publisher lock lost")
	}
	rows, err := c.connection.QueryContext(ctx, query, args...)
	if err != nil {
		c.lost.Store(true)
		return nil, err
	}
	return &publisherLockRows{Rows: rows, owner: c}, nil
}

// BeginTx rejects a long-lived transaction that could span non-idempotent cloud calls.
func (*publisherConnection) BeginTx(context.Context) (Tx, error) {
	return nil, errors.New("Publisher lock scopes must use autocommit")
}

type publisherLockRow struct {
	row   Row
	owner *publisherConnection
	err   error
}

// Scan records late database errors so subsequent mutations cannot escape a lost lock.
func (r publisherLockRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	err := r.row.Scan(dest...)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		r.owner.lost.Store(true)
	}
	return err
}

type publisherLockRows struct {
	Rows
	owner *publisherConnection
}

// Err invalidates a session when streaming exposes a late transport error.
func (r *publisherLockRows) Err() error {
	err := r.Rows.Err()
	if err != nil {
		r.owner.lost.Store(true)
	}
	return err
}

// Scan invalidates a session when a streamed row cannot be read safely.
func (r *publisherLockRows) Scan(dest ...any) error {
	err := r.Rows.Scan(dest...)
	if err != nil {
		r.owner.lost.Store(true)
	}
	return err
}

// Close propagates any final transport failure before the session is pooled.
func (r *publisherLockRows) Close() error {
	err := r.Rows.Close()
	if err != nil {
		r.owner.lost.Store(true)
	}
	return err
}

var _ Executor = (*publisherConnection)(nil)
var _ PublisherLocker = (*Database)(nil)
