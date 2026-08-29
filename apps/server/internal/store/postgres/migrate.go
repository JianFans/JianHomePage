package postgres

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"sort"
)

// Execer is deliberately smaller than a concrete database driver. A pgx
// pool, a transaction wrapper, or a deterministic test fake can implement it.
type Execer interface {
	ExecContext(context.Context, string, ...any) error
}

//go:embed migrations/*.sql
var migrationFiles embed.FS

// Migrate executes all embedded migrations in filename order in one database
// transaction. The advisory lock prevents two application instances from
// applying the same schema concurrently.
func Migrate(ctx context.Context, db Execer) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if db == nil {
		return errors.New("migration database is required")
	}
	if err := db.ExecContext(ctx, "BEGIN"); err != nil {
		return fmt.Errorf("begin migration transaction: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = db.ExecContext(context.Background(), "ROLLBACK")
		}
	}()
	if err := db.ExecContext(ctx, "SELECT pg_advisory_xact_lock(865734219)"); err != nil {
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
		if err := db.ExecContext(ctx, string(statement)); err != nil {
			return fmt.Errorf("execute migration %s: %w", name, err)
		}
	}
	if err := db.ExecContext(ctx, "COMMIT"); err != nil {
		return fmt.Errorf("commit migrations: %w", err)
	}
	committed = true
	return nil
}
