package postgres

import (
	"context"
	"errors"
	"strings"
	"testing"

	snapshotdata "yujian.me/server/internal/snapshot"
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
	args       [][]any
	queryCalls []string
	failOn     int
	failErr    error
	committed  bool
	rolledBack bool
	applied    bool
	rows       Rows
	assetRows  Rows
}

func (fake *migrationTxFake) ExecContext(_ context.Context, statement string, args ...any) (ExecResult, error) {
	fake.calls = append(fake.calls, statement)
	fake.args = append(fake.args, args)
	if fake.failOn > 0 && len(fake.calls) == fake.failOn {
		return nil, fake.failErr
	}
	return recordingResult{affected: 1}, nil
}
func (fake *migrationTxFake) QueryRowContext(context.Context, string, ...any) Row {
	return migrationBoolRow(fake.applied)
}
func (fake *migrationTxFake) QueryContext(_ context.Context, statement string, _ ...any) (Rows, error) {
	fake.queryCalls = append(fake.queryCalls, statement)
	if strings.Contains(statement, "FROM assets") {
		if fake.assetRows != nil {
			return fake.assetRows, nil
		}
		return &recordingRows{}, nil
	}
	if fake.rows != nil {
		return fake.rows, nil
	}
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

type migrationBoolRow bool

func (row migrationBoolRow) Scan(dest ...any) error {
	if len(dest) != 1 {
		return errors.New("expected one migration marker destination")
	}
	value, ok := dest[0].(*bool)
	if !ok {
		return errors.New("expected boolean migration marker destination")
	}
	*value = bool(row)
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
	if !strings.Contains(strings.Join(fake.tx.calls, "\n"), "conrelid = 'publish_jobs'::regclass") {
		t.Fatal("publish job constraint lookup is not scoped to publish_jobs")
	}
	statements := strings.Join(fake.tx.calls, "\n")
	if !strings.Contains(statements, "publishing") || !strings.Contains(statements, "target_revision") {
		t.Fatal("publish target freeze migration was not executed")
	}
	if !strings.Contains(statements, "status IN ('pending', 'building')") ||
		!strings.Contains(statements, "SET status = 'publishing'") ||
		!strings.Contains(statements, "RAISE EXCEPTION") {
		t.Fatal("active publish jobs are not safely upgraded or rejected")
	}
	if !strings.Contains(statements, "0009_uuid_ids requires empty content, asset, publish, and publisher tables") ||
		!strings.Contains(statements, "ALTER TABLE audit_log ALTER COLUMN id TYPE uuid USING gen_random_uuid()") ||
		!strings.Contains(statements, "ALTER TABLE audit_log ALTER COLUMN id SET DEFAULT gen_random_uuid()") {
		t.Fatal("UUID migration does not guard legacy business records and upgrade audit IDs")
	}
	if !fake.tx.committed || fake.tx.rolledBack {
		t.Fatalf("expected committed transaction, got %#v", fake.tx)
	}
}

func TestMigrateSkipsUUIDMigrationAfterItWasApplied(t *testing.T) {
	fake := &execFake{tx: &migrationTxFake{applied: true}}
	if err := Migrate(context.Background(), fake); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	statements := strings.Join(fake.tx.calls, "\n")
	if strings.Contains(statements, "0009_uuid_ids requires empty content") ||
		strings.Contains(statements, "ALTER TABLE audit_log ALTER COLUMN id TYPE uuid") {
		t.Fatal("already-applied UUID migration ran again")
	}
}

func TestUUIDMigrationPreservesAuditRowsAndNeverDeletesBusinessData(t *testing.T) {
	statement, err := migrationFiles.ReadFile("migrations/0009_uuid_ids.sql")
	if err != nil {
		t.Fatalf("read UUID migration: %v", err)
	}
	source := strings.ToLower(string(statement))
	for _, table := range []string{"content_versions", "assets", "publish_jobs", "publish_pointer", "publisher_builds"} {
		if !strings.Contains(source, "from "+table) {
			t.Errorf("UUID migration does not guard %s", table)
		}
	}
	for _, forbidden := range []string{"delete from", "truncate ", "drop table"} {
		if strings.Contains(source, forbidden) {
			t.Errorf("UUID migration contains destructive statement %q", forbidden)
		}
	}
	if !strings.Contains(source, "alter column id type uuid using gen_random_uuid()") {
		t.Fatal("audit IDs are not converted to PostgreSQL uuid values")
	}
}

func TestMigrateCanonicalizesLegacyContentChecksumsBeforePublishFreeze(t *testing.T) {
	jsonbReadback := []byte(`{"z": 100.0, "schemaVersion": "1.0.0", "a": 1.2300}`)
	canonical, err := snapshotdata.CanonicalJSON(jsonbReadback)
	if err != nil {
		t.Fatalf("canonicalize fixture: %v", err)
	}
	tx := &migrationTxFake{rows: &recordingRows{rows: []recordingRow{{values: []any{
		"ver_daf2a7f0-0912-4a1d-a5b3-e8aa5525734b", jsonbReadback, "sha256:legacy-raw-json",
	}}}}}
	fake := &execFake{tx: tx}

	if err := Migrate(context.Background(), fake); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	updateIndex := -1
	freezeIndex := -1
	for index, statement := range tx.calls {
		if strings.Contains(statement, "UPDATE content_versions SET checksum") {
			updateIndex = index
		}
		if strings.Contains(statement, "cannot safely upgrade active publish job") {
			freezeIndex = index
		}
	}
	if updateIndex < 0 {
		t.Fatal("legacy content checksum was not upgraded")
	}
	if freezeIndex < 0 || updateIndex >= freezeIndex {
		t.Fatalf("checksum upgrade must run before publish freeze migration: %#v", tx.calls)
	}
	if got := tx.args[updateIndex]; len(got) != 4 || got[0] != snapshotdata.Checksum(canonical) || got[1] != "ver_daf2a7f0-0912-4a1d-a5b3-e8aa5525734b" {
		t.Fatalf("unexpected checksum update args %#v", got)
	}
}

func TestMigrateBackfillsStableAssetURLsWithoutBlockingOldWriters(t *testing.T) {
	tx := &migrationTxFake{assetRows: &recordingRows{rows: []recordingRow{{values: []any{
		"asset_58d6e938-5806-4086-8a5d-f3908326255e", "assets/asset_58d6e938-5806-4086-8a5d-f3908326255e/source.webp",
	}}}}}
	fake := &execFake{tx: tx}
	resolvedKeys := make([]string, 0, 1)

	err := Migrate(context.Background(), fake, MigrationOptions{
		ResolveAssetSourceURL: func(_ context.Context, key string) (string, error) {
			resolvedKeys = append(resolvedKeys, key)
			return "https://media.yujian.me/" + key, nil
		},
	})
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if len(resolvedKeys) != 1 || resolvedKeys[0] != "assets/asset_58d6e938-5806-4086-8a5d-f3908326255e/source.webp" {
		t.Fatalf("unexpected URL resolution keys %#v", resolvedKeys)
	}

	addColumnIndex := -1
	backfillIndex := -1
	constraintIndex := -1
	for index, statement := range tx.calls {
		switch {
		case strings.Contains(statement, "ADD COLUMN IF NOT EXISTS source_url"):
			addColumnIndex = index
		case strings.Contains(statement, "UPDATE assets") && strings.Contains(statement, "source_url"):
			backfillIndex = index
			if args := tx.args[index]; len(args) != 2 || args[0] != "https://media.yujian.me/assets/asset_58d6e938-5806-4086-8a5d-f3908326255e/source.webp" || args[1] != "asset_58d6e938-5806-4086-8a5d-f3908326255e" {
				t.Fatalf("unexpected asset URL backfill args %#v", args)
			}
		case strings.Contains(statement, "ALTER COLUMN source_url SET NOT NULL"):
			t.Fatal("asset URL expansion migration must remain compatible with old writers")
		case strings.Contains(statement, "assets_source_url_nonempty"):
			constraintIndex = index
		}
	}
	if addColumnIndex < 0 || backfillIndex <= addColumnIndex || constraintIndex <= backfillIndex {
		t.Fatalf("unsafe asset URL migration sequence: %#v", tx.calls)
	}
	if !tx.committed || tx.rolledBack {
		t.Fatalf("expected committed URL migration, got %#v", tx)
	}
}

func TestMigrateRollsBackWhenLegacyAssetURLCannotBeResolved(t *testing.T) {
	sentinel := errors.New("media URL unavailable")
	tx := &migrationTxFake{assetRows: &recordingRows{rows: []recordingRow{{values: []any{
		"asset_58d6e938-5806-4086-8a5d-f3908326255e", "assets/asset_58d6e938-5806-4086-8a5d-f3908326255e/source.webp",
	}}}}}
	fake := &execFake{tx: tx}

	err := Migrate(context.Background(), fake, MigrationOptions{
		ResolveAssetSourceURL: func(context.Context, string) (string, error) { return "", sentinel },
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected URL resolver error, got %v", err)
	}
	if !tx.rolledBack || tx.committed {
		t.Fatalf("failed URL migration did not roll back: %#v", tx)
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
