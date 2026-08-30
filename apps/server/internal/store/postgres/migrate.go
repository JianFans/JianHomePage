package postgres

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"yujian.me/server/internal/domain"
	snapshotdata "yujian.me/server/internal/snapshot"
)

const publishFreezeMigration = "0003_publish_target_freeze.sql"
const assetSourceURLMigration = "0004_asset_source_url.sql"

type MigrationOptions struct {
	ResolveAssetSourceURL func(context.Context, string) (string, error)
}

//go:embed migrations/*.sql
var migrationFiles embed.FS

// Migrate executes all embedded migrations in filename order in one database
// transaction. The advisory lock prevents two application instances from
// applying the same schema concurrently.
func Migrate(ctx context.Context, db Executor, migrationOptions ...MigrationOptions) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if db == nil {
		return errors.New("migration database is required")
	}
	if len(migrationOptions) > 1 {
		return errors.New("only one migration options value is supported")
	}
	options := MigrationOptions{}
	if len(migrationOptions) == 1 {
		options = migrationOptions[0]
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
		migrationWasApplied := false
		if name == publishFreezeMigration || name == assetSourceURLMigration {
			applied, err := migrationApplied(ctx, tx, strings.TrimSuffix(name, ".sql"))
			if err != nil {
				return fmt.Errorf("check migration %s: %w", name, err)
			}
			migrationWasApplied = applied
			if name == publishFreezeMigration && !applied {
				if err := upgradeLegacyContentChecksums(ctx, tx); err != nil {
					return fmt.Errorf("upgrade legacy content checksums: %w", err)
				}
			}
		}
		if _, err := tx.ExecContext(ctx, string(statement)); err != nil {
			return fmt.Errorf("execute migration %s: %w", name, err)
		}
		if name == assetSourceURLMigration && !migrationWasApplied {
			if err := upgradeLegacyAssetSourceURLs(ctx, tx, options.ResolveAssetSourceURL); err != nil {
				return fmt.Errorf("upgrade legacy asset source URLs: %w", err)
			}
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit migrations: %w", err)
	}
	committed = true
	return nil
}

func upgradeLegacyAssetSourceURLs(
	ctx context.Context,
	tx Tx,
	resolve func(context.Context, string) (string, error),
) error {
	rows, err := tx.QueryContext(ctx, `
SELECT id, blob_key FROM assets
WHERE source_url IS NULL OR btrim(source_url) = ''
ORDER BY id FOR UPDATE`)
	if err != nil {
		return err
	}
	type legacyAsset struct {
		id      string
		blobKey string
	}
	assets := make([]legacyAsset, 0)
	for rows.Next() {
		var asset legacyAsset
		if err := rows.Scan(&asset.id, &asset.blobKey); err != nil {
			_ = rows.Close()
			return err
		}
		assets = append(assets, asset)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}

	if len(assets) > 0 && resolve == nil {
		return errors.New("asset source URL resolver is required for legacy records")
	}
	for _, asset := range assets {
		sourceURL, err := resolve(ctx, asset.blobKey)
		if err != nil {
			return fmt.Errorf("resolve asset %s source URL: %w", asset.id, err)
		}
		if sourceURL == "" || strings.TrimSpace(sourceURL) != sourceURL {
			return fmt.Errorf("resolve asset %s source URL: empty or ambiguous URL", asset.id)
		}
		result, err := tx.ExecContext(ctx, `
UPDATE assets SET source_url = $1
WHERE id = $2 AND (source_url IS NULL OR btrim(source_url) = '')`, sourceURL, asset.id)
		if err != nil {
			return err
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if affected != 1 {
			return domain.ErrConflict
		}
	}

	_, err = tx.ExecContext(ctx, `
ALTER TABLE assets DROP CONSTRAINT IF EXISTS assets_source_url_nonempty;
ALTER TABLE assets ADD CONSTRAINT assets_source_url_nonempty
CHECK (source_url IS NULL OR btrim(source_url) <> '')`)
	return err
}

func migrationApplied(ctx context.Context, tx Tx, version string) (bool, error) {
	var applied bool
	err := tx.QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = $1)`, version,
	).Scan(&applied)
	return applied, err
}

func upgradeLegacyContentChecksums(ctx context.Context, tx Tx) error {
	rows, err := tx.QueryContext(ctx,
		`SELECT id, snapshot, checksum FROM content_versions ORDER BY id FOR UPDATE`,
	)
	if err != nil {
		return err
	}
	type legacyVersion struct {
		id       string
		snapshot []byte
		checksum string
	}
	versions := make([]legacyVersion, 0)
	for rows.Next() {
		var version legacyVersion
		if err := rows.Scan(&version.id, &version.snapshot, &version.checksum); err != nil {
			_ = rows.Close()
			return err
		}
		versions = append(versions, version)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}

	for _, version := range versions {
		canonical, err := snapshotdata.CanonicalJSON(version.snapshot)
		if err != nil {
			return fmt.Errorf("canonicalize content version %s: %w", version.id, err)
		}
		checksum := snapshotdata.Checksum(canonical)
		if checksum == version.checksum {
			continue
		}
		result, err := tx.ExecContext(ctx, `
UPDATE content_versions SET checksum = $1
WHERE id = $2 AND checksum = $3 AND snapshot = $4::jsonb`,
			checksum, version.id, version.checksum, canonical)
		if err != nil {
			return err
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if affected != 1 {
			return domain.ErrConflict
		}
	}
	return nil
}
