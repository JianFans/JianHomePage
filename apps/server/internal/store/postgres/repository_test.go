package postgres

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"yujian.me/server/internal/content"
	"yujian.me/server/internal/domain"
	snapshotdata "yujian.me/server/internal/snapshot"
)

type recordingResult struct{ affected int64 }

func (result recordingResult) RowsAffected() (int64, error) { return result.affected, nil }

type recordingRow struct {
	values []any
	err    error
}

func (row recordingRow) Scan(dest ...any) error {
	if row.err != nil {
		return row.err
	}
	for index := range dest {
		if index >= len(row.values) {
			return errors.New("not enough row values")
		}
		switch target := dest[index].(type) {
		case *string:
			value, ok := row.values[index].(string)
			if !ok {
				return errors.New("cannot scan NULL into string")
			}
			*target = value
		case *sql.NullString:
			value, ok := row.values[index].(string)
			if !ok {
				*target = sql.NullString{}
				break
			}
			*target = sql.NullString{String: value, Valid: true}
		case *int64:
			*target = row.values[index].(int64)
		case *bool:
			*target = row.values[index].(bool)
		case *[]byte:
			*target = append((*target)[:0], row.values[index].([]byte)...)
		case *time.Time:
			*target = row.values[index].(time.Time)
		case *sql.NullTime:
			*target = row.values[index].(sql.NullTime)
		default:
			return errors.New("unsupported scan target")
		}
	}
	return nil
}

type recordingRows struct {
	rows  []recordingRow
	index int
}

func (rows *recordingRows) Next() bool {
	if rows.index >= len(rows.rows) {
		return false
	}
	rows.index++
	return true
}

func (rows *recordingRows) Scan(dest ...any) error { return rows.rows[rows.index-1].Scan(dest...) }
func (rows *recordingRows) Err() error             { return nil }
func (rows *recordingRows) Close() error           { return nil }

type recordingExecutor struct {
	execQueries []string
	execArgs    [][]any
	rowQueries  []string
	row         Row
	rows        Rows
	result      ExecResult
	begin       *recordingTx
	execErr     error
}

func (executor *recordingExecutor) ExecContext(_ context.Context, query string, args ...any) (ExecResult, error) {
	executor.execQueries = append(executor.execQueries, query)
	executor.execArgs = append(executor.execArgs, args)
	return executor.result, executor.execErr
}

func (executor *recordingExecutor) QueryRowContext(_ context.Context, query string, _ ...any) Row {
	executor.rowQueries = append(executor.rowQueries, query)
	return executor.row
}

func (executor *recordingExecutor) QueryContext(context.Context, string, ...any) (Rows, error) {
	return executor.rows, nil
}

func (executor *recordingExecutor) BeginTx(context.Context) (Tx, error) {
	executor.begin = &recordingTx{recordingExecutor: recordingExecutor{result: executor.result}}
	return executor.begin, nil
}

type recordingTx struct {
	recordingExecutor
	committed  bool
	rolledBack bool
}

func (tx *recordingTx) Commit(context.Context) error {
	tx.committed = true
	return nil
}

func (tx *recordingTx) Rollback(context.Context) error {
	tx.rolledBack = true
	return nil
}

func TestContentRepositoryGetVersionMapsRowAndNoRows(t *testing.T) {
	now := time.Date(2026, 8, 29, 0, 0, 0, 0, time.UTC)
	snapshot := []byte(`{"schemaVersion":"1.0.0"}`)
	executor := &recordingExecutor{row: recordingRow{values: []any{
		"ver_1", "draft", int64(2), snapshot, snapshotdata.Checksum(snapshot), false,
		"editor-1", "editor-1", now, now,
	}}}
	repository := NewContentRepository(executor)
	version, err := repository.GetVersion(context.Background(), "ver_1")
	if err != nil {
		t.Fatalf("get version: %v", err)
	}
	if version.ID != "ver_1" || version.Revision != 2 || version.Status != domain.StatusDraft {
		t.Fatalf("unexpected version %#v", version)
	}

	executor.row = recordingRow{err: sql.ErrNoRows}
	if _, err := repository.GetVersion(context.Background(), "missing"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected not found, got %v", err)
	}
}

func TestContentRepositoryCanonicalizesJSONBReadback(t *testing.T) {
	now := time.Date(2026, 8, 29, 0, 0, 0, 0, time.UTC)
	expected := []byte(`{"a":1.23,"schemaVersion":"1.0.0","z":100}`)
	executor := &recordingExecutor{row: recordingRow{values: []any{
		"ver_1", "draft", int64(2), []byte(`{"z":100.0,"schemaVersion":"1.0.0","a":1.2300}`), snapshotdata.Checksum(expected), false,
		"editor-1", "editor-1", now, now,
	}}}

	version, err := NewContentRepository(executor).GetVersion(context.Background(), "ver_1")
	if err != nil {
		t.Fatalf("get version: %v", err)
	}
	if string(version.Snapshot) != string(expected) {
		t.Fatalf("snapshot was not canonicalized: %s", version.Snapshot)
	}
}

func TestContentRepositoryRejectsSnapshotChecksumMismatch(t *testing.T) {
	now := time.Date(2026, 8, 29, 0, 0, 0, 0, time.UTC)
	executor := &recordingExecutor{row: recordingRow{values: []any{
		"ver_1", "draft", int64(2), []byte(`{"schemaVersion":"1.0.0"}`), "sha256:wrong", false,
		"editor-1", "editor-1", now, now,
	}}}

	if _, err := NewContentRepository(executor).GetVersion(context.Background(), "ver_1"); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("expected checksum conflict, got %v", err)
	}
}

func TestContentRepositoryUpdateUsesCompareAndSwap(t *testing.T) {
	version := domain.ContentVersion{ID: "ver_1", Status: domain.StatusDraft, Revision: 3, Snapshot: []byte(`{}`), Checksum: "sha256:test", CreatedBy: "editor", UpdatedBy: "editor"}
	executor := &recordingExecutor{result: recordingResult{affected: 1}}
	if err := NewContentRepository(executor).UpdateVersion(context.Background(), version, 2); err != nil {
		t.Fatalf("update version: %v", err)
	}
	if len(executor.execQueries) != 1 || !strings.Contains(executor.execQueries[0], "WHERE id = $8 AND revision = $9") {
		t.Fatalf("expected compare-and-swap query, got %#v", executor.execQueries)
	}
	if executor.execArgs[0][7] != "ver_1" || executor.execArgs[0][8] != int64(2) {
		t.Fatalf("unexpected update args %#v", executor.execArgs[0])
	}
}

func TestContentRepositoryPreservesLookupErrorsAfterZeroRowsUpdate(t *testing.T) {
	sentinel := errors.New("database connection lost")
	version := domain.ContentVersion{ID: "ver_1", Status: domain.StatusDraft, Revision: 3, Snapshot: []byte(`{}`), Checksum: "sha256:test", CreatedBy: "editor", UpdatedBy: "editor"}
	executor := &recordingExecutor{result: recordingResult{affected: 0}, row: recordingRow{err: sentinel}}
	if err := NewContentRepository(executor).UpdateVersion(context.Background(), version, 2); !errors.Is(err, sentinel) {
		t.Fatalf("expected lookup error, got %v", err)
	}
}

func TestAssetRepositoryPersistsStableSourceURL(t *testing.T) {
	now := time.Date(2026, 8, 30, 0, 0, 0, 0, time.UTC)
	asset := domain.AssetRecord{
		ID: "asset_1", BlobKey: "assets/asset_1/source.webp",
		SourceURL: "https://media.yujian.me/assets/asset_1/source.webp",
		Status:    domain.AssetPending, Metadata: []byte(`{"kind":"image"}`),
		Rights: []byte(`{"source":{"zh-CN":"authorized"}}`), CreatedBy: "editor-1", CreatedAt: now,
	}
	executor := &recordingExecutor{result: recordingResult{affected: 1}}
	repository := NewAssetRepository(executor)

	if err := repository.CreateAsset(t.Context(), asset); err != nil {
		t.Fatalf("create asset: %v", err)
	}
	if len(executor.execQueries) != 1 || !strings.Contains(executor.execQueries[0], "source_url") {
		t.Fatalf("asset insert does not persist source URL: %#v", executor.execQueries)
	}
	if args := executor.execArgs[0]; len(args) < 3 || args[2] != asset.SourceURL {
		t.Fatalf("asset insert lost source URL: %#v", args)
	}

	executor.row = recordingRow{values: []any{
		asset.ID, asset.BlobKey, asset.SourceURL, string(asset.Status), []byte(asset.Metadata), []byte(asset.Rights),
		asset.CreatedBy, asset.CreatedAt, sql.NullTime{},
	}}
	loaded, err := repository.GetAsset(t.Context(), asset.ID)
	if err != nil {
		t.Fatalf("get asset: %v", err)
	}
	if loaded.SourceURL != asset.SourceURL {
		t.Fatalf("asset read lost source URL: %#v", loaded)
	}

	loaded.Status = domain.AssetReady
	if err := repository.UpdateAsset(t.Context(), loaded, domain.AssetPending); err != nil {
		t.Fatalf("update asset: %v", err)
	}
	updateQuery := executor.execQueries[len(executor.execQueries)-1]
	updateArgs := executor.execArgs[len(executor.execArgs)-1]
	if !strings.Contains(updateQuery, "source_url") || len(updateArgs) < 2 || updateArgs[1] != asset.SourceURL {
		t.Fatalf("asset update lost source URL: query=%q args=%#v", updateQuery, updateArgs)
	}
}

func TestAssetRepositoryReadsLegacyNullSourceURL(t *testing.T) {
	now := time.Date(2026, 8, 30, 0, 0, 0, 0, time.UTC)
	executor := &recordingExecutor{row: recordingRow{values: []any{
		"asset_legacy", "assets/asset_legacy/source.webp", nil,
		"pending", []byte(`{}`), []byte(`{"source":{"zh-CN":"authorized"}}`),
		"editor-legacy", now, sql.NullTime{},
	}}}

	asset, err := NewAssetRepository(executor).GetAsset(t.Context(), "asset_legacy")
	if err != nil {
		t.Fatalf("read legacy asset with NULL source URL: %v", err)
	}
	if asset.SourceURL != "" || asset.Status != domain.AssetPending {
		t.Fatalf("unexpected legacy asset %#v", asset)
	}
}

func TestAssetRepositoryCreatesMissingSourceURLAsNull(t *testing.T) {
	executor := &recordingExecutor{result: recordingResult{affected: 1}}
	asset := domain.AssetRecord{
		ID: "asset_legacy", BlobKey: "assets/asset_legacy/source.webp", Status: domain.AssetPending,
		Metadata: []byte(`{}`), Rights: []byte(`{}`), CreatedAt: time.Now(),
	}

	if err := NewAssetRepository(executor).CreateAsset(t.Context(), asset); err != nil {
		t.Fatalf("create asset without source URL: %v", err)
	}
	if args := executor.execArgs[0]; len(args) < 3 || args[2] != nil {
		t.Fatalf("missing source URL must bind as SQL NULL: %#v", args)
	}
}

func TestAssetRepositoryUpdatesMissingSourceURLAsNull(t *testing.T) {
	executor := &recordingExecutor{result: recordingResult{affected: 1}}
	asset := domain.AssetRecord{
		ID: "asset_legacy", BlobKey: "assets/asset_legacy/source.webp", Status: domain.AssetDeleted,
		Metadata: []byte(`{}`), Rights: []byte(`{}`),
	}

	if err := NewAssetRepository(executor).UpdateAsset(t.Context(), asset, domain.AssetPending); err != nil {
		t.Fatalf("update asset without source URL: %v", err)
	}
	if args := executor.execArgs[0]; len(args) < 2 || args[1] != nil {
		t.Fatalf("missing source URL must bind as SQL NULL: %#v", args)
	}
}

func TestAssetRepositoryUpdateUsesStatusCompareAndSwap(t *testing.T) {
	executor := &recordingExecutor{result: recordingResult{affected: 1}}
	asset := domain.AssetRecord{ID: "asset_1", Status: domain.AssetReady, Metadata: []byte(`{}`), Rights: []byte(`{}`)}
	if err := NewAssetRepository(executor).UpdateAsset(context.Background(), asset, domain.AssetPending); err != nil {
		t.Fatalf("update asset: %v", err)
	}
	if len(executor.execQueries) != 1 || !strings.Contains(executor.execQueries[0], "WHERE id = $7 AND status = $8") {
		t.Fatalf("expected status compare-and-swap query, got %#v", executor.execQueries)
	}
	if executor.execArgs[0][7] != domain.AssetPending {
		t.Fatalf("unexpected expected status %#v", executor.execArgs[0])
	}
}

func TestAssetRepositoryMapsStaleStatusToConflict(t *testing.T) {
	now := time.Date(2026, 8, 29, 0, 0, 0, 0, time.UTC)
	executor := &recordingExecutor{
		result: recordingResult{affected: 0},
		row: recordingRow{values: []any{
			"asset_1", "assets/asset_1/source.webp", "https://media.yujian.me/assets/asset_1/source.webp",
			"deleted", []byte(`{}`), []byte(`{}`),
			"editor-1", now, sql.NullTime{},
		}},
	}
	asset := domain.AssetRecord{ID: "asset_1", Status: domain.AssetReady, Metadata: []byte(`{}`), Rights: []byte(`{}`)}
	if err := NewAssetRepository(executor).UpdateAsset(context.Background(), asset, domain.AssetPending); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("expected stale status conflict, got %v", err)
	}
}

func TestRepositoryTransactionRollsBackOnCallbackError(t *testing.T) {
	executor := &recordingExecutor{}
	sentinel := errors.New("callback failed")
	err := NewContentRepository(executor).WithinTransaction(context.Background(), func(content.Repository) error {
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected callback error, got %v", err)
	}
	if executor.begin == nil || !executor.begin.rolledBack || executor.begin.committed {
		t.Fatalf("expected rollback, tx=%#v", executor.begin)
	}
}

func TestPublishRepositoryReadsHistoricalSuccessfulJob(t *testing.T) {
	now := time.Date(2026, 8, 29, 0, 0, 0, 0, time.UTC)
	executor := &recordingExecutor{row: recordingRow{values: []any{
		"pub_1", "idem-1", "publish", "ver_old", "rel_old", "snapshots/rel/sha.json", "sha256:test", int64(4), "build-1", "succeeded", "", "publisher", now, now,
	}}}
	job, err := NewPublishRepository(executor).GetSuccessfulPublishByVersion(context.Background(), "ver_old")
	if err != nil {
		t.Fatalf("get historical job: %v", err)
	}
	if job.ID != "pub_1" || job.Status != domain.PublishSucceeded {
		t.Fatalf("unexpected job %#v", job)
	}
	if len(executor.execQueries) != 0 {
		t.Fatalf("query should use QueryRow, got exec calls %#v", executor.execQueries)
	}
}

func TestPublishRepositoryReadsActiveProductionJob(t *testing.T) {
	now := time.Date(2026, 8, 29, 0, 0, 0, 0, time.UTC)
	executor := &recordingExecutor{row: recordingRow{values: []any{
		"pub_active", "idem-active", "publish", "ver_new", "rel_new", "snapshots/rel/sha.json", "sha256:test", int64(4), "build-1", "building", "", "publisher", now, now,
	}}}
	job, err := NewPublishRepository(executor).GetActivePublishJob(context.Background())
	if err != nil {
		t.Fatalf("get active job: %v", err)
	}
	if job.ID != "pub_active" || job.Status != domain.PublishBuilding {
		t.Fatalf("unexpected active job %#v", job)
	}
	if len(executor.rowQueries) != 1 || !strings.Contains(executor.rowQueries[0], "status IN ('pending', 'building')") {
		t.Fatalf("expected active status query, got %#v", executor.rowQueries)
	}
}

func TestPublishRepositoryPersistsOperationAndReleaseID(t *testing.T) {
	executor := &recordingExecutor{result: recordingResult{affected: 1}}
	job := domain.PublishJob{
		ID: "pub_1", IdempotencyKey: "idem-1", Operation: domain.PublishOperationRollback,
		VersionID: "ver_1", ReleaseID: "rel_1", SnapshotKey: "snapshots/rel_1/sha.json",
		SnapshotChecksum: "sha256:test", Status: domain.PublishPending, RequestedBy: "publisher-1",
	}

	if err := NewPublishRepository(executor).CreatePublishJob(context.Background(), job); err != nil {
		t.Fatalf("create publish job: %v", err)
	}
	if len(executor.execArgs) != 1 || len(executor.execArgs[0]) < 6 {
		t.Fatalf("unexpected insert args %#v", executor.execArgs)
	}
	if executor.execArgs[0][2] != domain.PublishOperationRollback || executor.execArgs[0][4] != "rel_1" {
		t.Fatalf("operation and release were not persisted: %#v", executor.execArgs[0])
	}
}

func TestPublishRepositoryMapsUniqueViolationToConflict(t *testing.T) {
	executor := &recordingExecutor{execErr: sqlStateTestError{state: "23505"}}
	err := NewPublishRepository(executor).CreatePublishJob(context.Background(), domain.PublishJob{ID: "pub_1"})
	if !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("expected conflict, got %v", err)
	}
}

func TestPublishRepositoryUpdateUsesStatusCompareAndSwap(t *testing.T) {
	executor := &recordingExecutor{result: recordingResult{affected: 1}}
	job := domain.PublishJob{ID: "pub_1", Status: domain.PublishBuilding}
	if err := NewPublishRepository(executor).UpdatePublishJob(context.Background(), job, domain.PublishPending); err != nil {
		t.Fatalf("update publish job: %v", err)
	}
	if len(executor.execQueries) != 1 || !strings.Contains(executor.execQueries[0], "WHERE id = $12 AND status = $13") {
		t.Fatalf("expected status compare-and-swap query, got %#v", executor.execQueries)
	}
	if executor.execArgs[0][12] != domain.PublishPending {
		t.Fatalf("unexpected expected status %#v", executor.execArgs[0])
	}
}

func TestPublishRepositoryMapsStaleStatusToConflict(t *testing.T) {
	now := time.Date(2026, 8, 29, 0, 0, 0, 0, time.UTC)
	executor := &recordingExecutor{
		result: recordingResult{affected: 0},
		row: recordingRow{values: []any{
			"pub_1", "idem-1", "publish", "ver_new", "rel_new", "snapshots/rel/sha.json", "sha256:test", int64(4), "build-1", "succeeded", "", "publisher", now, now,
		}},
	}
	job := domain.PublishJob{ID: "pub_1", Status: domain.PublishBuilding}
	if err := NewPublishRepository(executor).UpdatePublishJob(context.Background(), job, domain.PublishPending); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("expected stale status conflict, got %v", err)
	}
}

func TestPublishRepositoryLocksSlotWithinTransaction(t *testing.T) {
	executor := &recordingExecutor{result: recordingResult{affected: 1}}
	if err := NewPublishRepository(executor).LockPublishSlot(context.Background(), "production"); err != nil {
		t.Fatalf("lock publish slot: %v", err)
	}
	if len(executor.execQueries) != 1 || !strings.Contains(executor.execQueries[0], "pg_advisory_xact_lock") {
		t.Fatalf("expected advisory lock query, got %#v", executor.execQueries)
	}
	if len(executor.execArgs) != 1 || len(executor.execArgs[0]) != 1 || executor.execArgs[0][0] != "production" {
		t.Fatalf("unexpected lock args %#v", executor.execArgs)
	}
}

type sqlStateTestError struct{ state string }

func (err sqlStateTestError) Error() string    { return "postgres error " + err.state }
func (err sqlStateTestError) SQLState() string { return err.state }
