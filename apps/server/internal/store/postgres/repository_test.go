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
			*target = row.values[index].(string)
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

func (executor *recordingExecutor) QueryRowContext(context.Context, string, ...any) Row {
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
	executor := &recordingExecutor{row: recordingRow{values: []any{
		"ver_1", "draft", int64(2), []byte(`{"schemaVersion":"1.0.0"}`), "sha256:test", false,
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

func TestAssetRepositoryUpdateUsesStatusCompareAndSwap(t *testing.T) {
	executor := &recordingExecutor{result: recordingResult{affected: 1}}
	asset := domain.AssetRecord{ID: "asset_1", Status: domain.AssetReady, Metadata: []byte(`{}`), Rights: []byte(`{}`)}
	if err := NewAssetRepository(executor).UpdateAsset(context.Background(), asset, domain.AssetPending); err != nil {
		t.Fatalf("update asset: %v", err)
	}
	if len(executor.execQueries) != 1 || !strings.Contains(executor.execQueries[0], "WHERE id = $6 AND status = $7") {
		t.Fatalf("expected status compare-and-swap query, got %#v", executor.execQueries)
	}
	if executor.execArgs[0][6] != domain.AssetPending {
		t.Fatalf("unexpected expected status %#v", executor.execArgs[0])
	}
}

func TestAssetRepositoryMapsStaleStatusToConflict(t *testing.T) {
	now := time.Date(2026, 8, 29, 0, 0, 0, 0, time.UTC)
	executor := &recordingExecutor{
		result: recordingResult{affected: 0},
		row: recordingRow{values: []any{
			"asset_1", "assets/asset_1/source.webp", "deleted", []byte(`{}`), []byte(`{}`),
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
		"pub_1", "idem-1", "publish", "ver_old", "rel_old", "snapshots/rel/sha.json", "sha256:test", "build-1", "succeeded", "", "publisher", now, now,
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
