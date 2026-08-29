package postgres

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"sort"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

// Migrate executes all embedded migrations in filename order in one database
// transaction. The advisory lock prevents two application instances from
// applying the same schema concurrently.
func Migrate(ctx context.Context, db Executor) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if db == nil {
		return errors.New("migration database is required")
	}
	tx, err := db.BeginTx(ctx)
	if err != nil {
		return fmt.Errorf("begin migration transaction: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback(context.Background())
		}
	}()
	if _, err := tx.ExecContext(ctx, "SELECT pg_advisory_xact_lock(865734219)"); err != nil {
		return fmt.Errorf("acquire migration lock: %w", err)
	}
	entries, err := fs.ReadDir(migrationFiles, "migrations")
	if err != nil {
		return fmt.Errorf("read migrations: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	for _, name := range names {
		statement, err := fs.ReadFile(migrationFiles, "migrations/"+name)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", name, err)
		}
		if _, err := tx.ExecContext(ctx, string(statement)); err != nil {
			return fmt.Errorf("execute migration %s: %w", name, err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit migrations: %w", err)
	}
	committed = true
	return nil
}
