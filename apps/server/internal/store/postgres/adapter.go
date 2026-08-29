package postgres

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	_ "github.com/jackc/pgx/v5/stdlib"
)

type sqlExecutor struct{ database *sql.DB }

type sqlTransaction struct{ transaction *sql.Tx }

type Database struct {
	executor Executor
	database *sql.DB
}

func Open(ctx context.Context, databaseURL string) (*Database, error) {
	if strings.TrimSpace(databaseURL) == "" {
		return nil, errors.New("database URL is required")
	}
	database, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return nil, err
	}
	if err := database.PingContext(ctx); err != nil {
		_ = database.Close()
		return nil, err
	}
	executor, err := NewExecutor(database)
	if err != nil {
		_ = database.Close()
		return nil, err
	}
	return &Database{executor: executor, database: database}, nil
}

func NewExecutor(database *sql.DB) (Executor, error) {
	if database == nil {
		return nil, errors.New("database is required")
	}
	return &sqlExecutor{database: database}, nil
}

func (database *Database) ExecContext(ctx context.Context, query string, args ...any) (ExecResult, error) {
	return database.executor.ExecContext(ctx, query, args...)
}

func (database *Database) QueryRowContext(ctx context.Context, query string, args ...any) Row {
	return database.executor.QueryRowContext(ctx, query, args...)
}

func (database *Database) QueryContext(ctx context.Context, query string, args ...any) (Rows, error) {
	return database.executor.QueryContext(ctx, query, args...)
}

func (database *Database) BeginTx(ctx context.Context) (Tx, error) {
	return database.executor.BeginTx(ctx)
}

func (database *Database) Close() error { return database.database.Close() }

func (executor *sqlExecutor) ExecContext(ctx context.Context, query string, args ...any) (ExecResult, error) {
	return executor.database.ExecContext(ctx, query, args...)
}

func (executor *sqlExecutor) QueryRowContext(ctx context.Context, query string, args ...any) Row {
	return executor.database.QueryRowContext(ctx, query, args...)
}

func (executor *sqlExecutor) QueryContext(ctx context.Context, query string, args ...any) (Rows, error) {
	return executor.database.QueryContext(ctx, query, args...)
}

func (executor *sqlExecutor) BeginTx(ctx context.Context) (Tx, error) {
	transaction, err := executor.database.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	return &sqlTransaction{transaction: transaction}, nil
}

func (transaction *sqlTransaction) ExecContext(ctx context.Context, query string, args ...any) (ExecResult, error) {
	return transaction.transaction.ExecContext(ctx, query, args...)
}

func (transaction *sqlTransaction) QueryRowContext(ctx context.Context, query string, args ...any) Row {
	return transaction.transaction.QueryRowContext(ctx, query, args...)
}

func (transaction *sqlTransaction) QueryContext(ctx context.Context, query string, args ...any) (Rows, error) {
	return transaction.transaction.QueryContext(ctx, query, args...)
}

func (*sqlTransaction) BeginTx(context.Context) (Tx, error) {
	return nil, errors.New("nested transactions are not supported")
}

func (transaction *sqlTransaction) Commit(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return transaction.transaction.Commit()
}

func (transaction *sqlTransaction) Rollback(context.Context) error {
	return transaction.transaction.Rollback()
}

var _ Executor = (*sqlExecutor)(nil)
var _ Executor = (*Database)(nil)
var _ Tx = (*sqlTransaction)(nil)
