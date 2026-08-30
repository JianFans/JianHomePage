package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"yujian.me/server/internal/assets"
	"yujian.me/server/internal/content"
	"yujian.me/server/internal/domain"
	"yujian.me/server/internal/publish"
	snapshotdata "yujian.me/server/internal/snapshot"
)

// ExecResult is the tiny command-result surface needed for optimistic locking.
// A pgx CommandTag can be adapted without importing pgx into this package.
type ExecResult interface {
	RowsAffected() (int64, error)
}

type Row interface {
	Scan(...any) error
}

type Rows interface {
	Next() bool
	Scan(...any) error
	Err() error
	Close() error
}

// Executor is implemented by a database pool and by its transaction wrapper.
type Executor interface {
	ExecContext(context.Context, string, ...any) (ExecResult, error)
	QueryRowContext(context.Context, string, ...any) Row
	QueryContext(context.Context, string, ...any) (Rows, error)
	BeginTx(context.Context) (Tx, error)
}

type Tx interface {
	Executor
	Commit(context.Context) error
	Rollback(context.Context) error
}

type ContentRepository struct{ exec Executor }

func NewContentRepository(exec Executor) *ContentRepository { return &ContentRepository{exec: exec} }

func (repository *ContentRepository) WithinTransaction(ctx context.Context, run func(content.Repository) error) error {
	tx, err := begin(ctx, repository.exec)
	if err != nil {
		return err
	}
	transaction := &ContentRepository{exec: tx}
	if err := run(transaction); err != nil {
		_ = tx.Rollback(context.Background())
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		_ = tx.Rollback(context.Background())
		return err
	}
	return nil
}

func (repository *ContentRepository) CreateVersion(ctx context.Context, version domain.ContentVersion) error {
	_, err := repository.exec.ExecContext(ctx, `
INSERT INTO content_versions
  (id, status, revision, snapshot, checksum, review_approved, created_by, updated_by, created_at, updated_at)
VALUES ($1, $2, $3, $4::jsonb, $5, $6, $7, $8, $9, $10)`,
		version.ID, version.Status, version.Revision, []byte(version.Snapshot), version.Checksum,
		version.ReviewApproved, version.CreatedBy, version.UpdatedBy, version.CreatedAt, version.UpdatedAt)
	return err
}

func (repository *ContentRepository) GetVersion(ctx context.Context, id string) (domain.ContentVersion, error) {
	return scanVersion(repository.exec.QueryRowContext(ctx, `
SELECT id, status, revision, snapshot, checksum, review_approved,
       created_by, updated_by, created_at, updated_at
FROM content_versions WHERE id = $1`, id))
}

func (repository *ContentRepository) UpdateVersion(ctx context.Context, version domain.ContentVersion, expectedRevision int64) error {
	result, err := repository.exec.ExecContext(ctx, `
UPDATE content_versions
SET status = $1, revision = $2, snapshot = $3::jsonb, checksum = $4,
    review_approved = $5, updated_by = $6, updated_at = $7
WHERE id = $8 AND revision = $9`,
		version.Status, version.Revision, []byte(version.Snapshot), version.Checksum,
		version.ReviewApproved, version.UpdatedBy, version.UpdatedAt, version.ID, expectedRevision)
	if err != nil {
		return err
	}
	return compareAndSwapResult(ctx, repository.exec, result, version.ID)
}

func (repository *ContentRepository) AppendAudit(ctx context.Context, entry domain.AuditEntry) error {
	_, err := repository.exec.ExecContext(ctx, `
INSERT INTO audit_log (actor_sub, action, resource_type, resource_id, metadata, created_at)
VALUES ($1, $2, $3, $4, $5::jsonb, $6)`,
		entry.ActorSubject, entry.Action, entry.ResourceType, entry.ResourceID, []byte(entry.Metadata), entry.CreatedAt)
	return err
}

type AssetRepository struct{ exec Executor }

func NewAssetRepository(exec Executor) *AssetRepository { return &AssetRepository{exec: exec} }

func (repository *AssetRepository) WithinTransaction(ctx context.Context, run func(assets.Repository) error) error {
	tx, err := begin(ctx, repository.exec)
	if err != nil {
		return err
	}
	transaction := &AssetRepository{exec: tx}
	if err := run(transaction); err != nil {
		_ = tx.Rollback(context.Background())
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		_ = tx.Rollback(context.Background())
		return err
	}
	return nil
}

func (repository *AssetRepository) CreateAsset(ctx context.Context, asset domain.AssetRecord) error {
	_, err := repository.exec.ExecContext(ctx, `
INSERT INTO assets (id, blob_key, source_url, status, metadata, rights, created_by, created_at, deleted_at)
VALUES ($1, $2, $3, $4, $5::jsonb, $6::jsonb, $7, $8, $9)`,
		asset.ID, asset.BlobKey, nullableAssetSourceURL(asset.SourceURL), asset.Status, []byte(asset.Metadata), []byte(asset.Rights), asset.CreatedBy, asset.CreatedAt, asset.DeletedAt)
	return err
}

func (repository *AssetRepository) GetAsset(ctx context.Context, id string) (domain.AssetRecord, error) {
	return scanAsset(repository.exec.QueryRowContext(ctx, `
SELECT id, blob_key, source_url, status, metadata, rights, created_by, created_at, deleted_at
FROM assets WHERE id = $1`, id))
}

func (repository *AssetRepository) UpdateAsset(ctx context.Context, asset domain.AssetRecord, expectedStatus domain.AssetStatus) error {
	result, err := repository.exec.ExecContext(ctx, `
UPDATE assets
SET blob_key = $1, source_url = $2, status = $3, metadata = $4::jsonb, rights = $5::jsonb, deleted_at = $6
WHERE id = $7 AND status = $8`,
		asset.BlobKey, nullableAssetSourceURL(asset.SourceURL), asset.Status, []byte(asset.Metadata), []byte(asset.Rights), asset.DeletedAt, asset.ID, expectedStatus)
	if err != nil {
		return err
	}
	return compareAndSwapAssetResult(ctx, repository.exec, result, asset.ID)
}

func nullableAssetSourceURL(sourceURL string) any {
	if sourceURL == "" {
		return nil
	}
	return sourceURL
}

func compareAndSwapAssetResult(ctx context.Context, exec Executor, result ExecResult, id string) error {
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected > 0 {
		return nil
	}
	_, err = scanAsset(exec.QueryRowContext(ctx, `
SELECT id, blob_key, source_url, status, metadata, rights, created_by, created_at, deleted_at
FROM assets WHERE id = $1`, id))
	if errors.Is(err, domain.ErrNotFound) {
		return domain.ErrNotFound
	}
	if err != nil {
		return err
	}
	return domain.ErrConflict
}

func (repository *AssetRepository) AppendAudit(ctx context.Context, entry domain.AuditEntry) error {
	_, err := repository.exec.ExecContext(ctx, `
INSERT INTO audit_log (actor_sub, action, resource_type, resource_id, metadata, created_at)
VALUES ($1, $2, $3, $4, $5::jsonb, $6)`,
		entry.ActorSubject, entry.Action, entry.ResourceType, entry.ResourceID, []byte(entry.Metadata), entry.CreatedAt)
	return err
}

type PublishRepository struct{ exec Executor }

func NewPublishRepository(exec Executor) *PublishRepository { return &PublishRepository{exec: exec} }

func (repository *PublishRepository) WithinTransaction(ctx context.Context, run func(publish.Repository) error) error {
	tx, err := begin(ctx, repository.exec)
	if err != nil {
		return err
	}
	transaction := &PublishRepository{exec: tx}
	if err := run(transaction); err != nil {
		_ = tx.Rollback(context.Background())
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		_ = tx.Rollback(context.Background())
		return err
	}
	return nil
}

func (repository *PublishRepository) GetVersion(ctx context.Context, id string) (domain.ContentVersion, error) {
	return scanVersion(repository.exec.QueryRowContext(ctx, `
SELECT id, status, revision, snapshot, checksum, review_approved,
       created_by, updated_by, created_at, updated_at
FROM content_versions WHERE id = $1`, id))
}

func (repository *PublishRepository) GetAsset(ctx context.Context, id string) (domain.AssetRecord, error) {
	return scanAsset(repository.exec.QueryRowContext(ctx, `
SELECT id, blob_key, source_url, status, metadata, rights, created_by, created_at, deleted_at
FROM assets WHERE id = $1`, id))
}

func (repository *PublishRepository) UpdateVersion(ctx context.Context, version domain.ContentVersion, expectedRevision int64) error {
	result, err := repository.exec.ExecContext(ctx, `
UPDATE content_versions
SET status = $1, revision = $2, snapshot = $3::jsonb, checksum = $4,
    review_approved = $5, updated_by = $6, updated_at = $7
WHERE id = $8 AND revision = $9`,
		version.Status, version.Revision, []byte(version.Snapshot), version.Checksum,
		version.ReviewApproved, version.UpdatedBy, version.UpdatedAt, version.ID, expectedRevision)
	if err != nil {
		return err
	}
	return compareAndSwapResult(ctx, repository.exec, result, version.ID)
}

func (repository *PublishRepository) CreatePublishJob(ctx context.Context, job domain.PublishJob) error {
	_, err := repository.exec.ExecContext(ctx, `
INSERT INTO publish_jobs
  (id, idempotency_key, operation, version_id, release_id, snapshot_key, snapshot_checksum, target_revision, build_id, status, error_message, requested_by, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NULLIF($9, ''), $10, NULLIF($11, ''), $12, $13, $14)`,
		job.ID, job.IdempotencyKey, job.Operation, job.VersionID, job.ReleaseID, job.SnapshotKey, job.SnapshotChecksum,
		job.TargetRevision, job.BuildID, job.Status, job.ErrorMessage, job.RequestedBy, job.CreatedAt, job.UpdatedAt)
	return mapConstraintError(err)
}

func (repository *PublishRepository) GetPublishJob(ctx context.Context, id string) (domain.PublishJob, error) {
	return scanPublishJob(repository.exec.QueryRowContext(ctx, publishJobQuery+" WHERE id = $1", id))
}

func (repository *PublishRepository) GetPublishJobByIdempotencyKey(ctx context.Context, key string) (domain.PublishJob, error) {
	return scanPublishJob(repository.exec.QueryRowContext(ctx, publishJobQuery+" WHERE idempotency_key = $1", key))
}

func (repository *PublishRepository) GetActivePublishJob(ctx context.Context) (domain.PublishJob, error) {
	return scanPublishJob(repository.exec.QueryRowContext(ctx, publishJobQuery+
		" WHERE status IN ('pending', 'building') ORDER BY created_at DESC LIMIT 1"))
}

func (repository *PublishRepository) GetSuccessfulPublishByVersion(ctx context.Context, versionID string) (domain.PublishJob, error) {
	return scanPublishJob(repository.exec.QueryRowContext(ctx, publishJobQuery+" WHERE version_id = $1 AND status = 'succeeded' ORDER BY updated_at DESC LIMIT 1", versionID))
}

func (repository *PublishRepository) UpdatePublishJob(ctx context.Context, job domain.PublishJob, expectedStatus domain.PublishStatus) error {
	result, err := repository.exec.ExecContext(ctx, `
UPDATE publish_jobs
SET idempotency_key = $1, operation = $2, version_id = $3, release_id = $4,
    snapshot_key = $5, snapshot_checksum = $6, build_id = NULLIF($7, ''),
    status = $8, error_message = NULLIF($9, ''), requested_by = $10, updated_at = $11
WHERE id = $12 AND status = $13`,
		job.IdempotencyKey, job.Operation, job.VersionID, job.ReleaseID, job.SnapshotKey, job.SnapshotChecksum,
		job.BuildID, job.Status, job.ErrorMessage, job.RequestedBy, job.UpdatedAt, job.ID, expectedStatus)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		_, lookupErr := repository.GetPublishJob(ctx, job.ID)
		if errors.Is(lookupErr, domain.ErrNotFound) {
			return domain.ErrNotFound
		}
		if lookupErr != nil {
			return lookupErr
		}
		return domain.ErrConflict
	}
	return nil
}

func (repository *PublishRepository) LockPublishSlot(ctx context.Context, slot string) error {
	_, err := repository.exec.ExecContext(ctx, `SELECT pg_advisory_xact_lock(865734220, hashtext($1))`, slot)
	return err
}

func (repository *PublishRepository) GetPublishPointer(ctx context.Context, slot string) (domain.PublishPointer, error) {
	var pointer domain.PublishPointer
	var slotValue, versionID, key, checksum string
	var updatedAt time.Time
	err := repository.exec.QueryRowContext(ctx, `
SELECT slot, version_id, snapshot_key, snapshot_checksum, updated_at
FROM publish_pointer WHERE slot = $1`, slot).Scan(&slotValue, &versionID, &key, &checksum, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.PublishPointer{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.PublishPointer{}, err
	}
	pointer = domain.PublishPointer{Slot: slotValue, VersionID: versionID, SnapshotKey: key, SnapshotChecksum: checksum, UpdatedAt: updatedAt}
	return pointer, nil
}

func (repository *PublishRepository) SetPublishPointer(ctx context.Context, pointer domain.PublishPointer) error {
	_, err := repository.exec.ExecContext(ctx, `
INSERT INTO publish_pointer (slot, version_id, snapshot_key, snapshot_checksum, updated_at)
VALUES ($1, $2, $3, $4, $5)
ON CONFLICT (slot) DO UPDATE SET version_id = EXCLUDED.version_id,
  snapshot_key = EXCLUDED.snapshot_key, snapshot_checksum = EXCLUDED.snapshot_checksum,
  updated_at = EXCLUDED.updated_at`,
		pointer.Slot, pointer.VersionID, pointer.SnapshotKey, pointer.SnapshotChecksum, pointer.UpdatedAt)
	return err
}

func (repository *PublishRepository) AppendAudit(ctx context.Context, entry domain.AuditEntry) error {
	_, err := repository.exec.ExecContext(ctx, `
INSERT INTO audit_log (actor_sub, action, resource_type, resource_id, metadata, created_at)
VALUES ($1, $2, $3, $4, $5::jsonb, $6)`,
		entry.ActorSubject, entry.Action, entry.ResourceType, entry.ResourceID, []byte(entry.Metadata), entry.CreatedAt)
	return err
}

const publishJobQuery = `
SELECT id, idempotency_key, operation, version_id, release_id, snapshot_key, snapshot_checksum,
       target_revision, COALESCE(build_id, ''), status, COALESCE(error_message, ''),
       requested_by, created_at, updated_at
FROM publish_jobs`

func begin(ctx context.Context, exec Executor) (Tx, error) {
	if exec == nil {
		return nil, errors.New("database executor is required")
	}
	return exec.BeginTx(ctx)
}

func compareAndSwapResult(ctx context.Context, exec Executor, result ExecResult, id string) error {
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected > 0 {
		return nil
	}
	_, err = scanVersion(exec.QueryRowContext(ctx, `SELECT id, status, revision, snapshot, checksum, review_approved, created_by, updated_by, created_at, updated_at FROM content_versions WHERE id = $1`, id))
	if errors.Is(err, domain.ErrNotFound) {
		return domain.ErrNotFound
	}
	if err != nil {
		return err
	}
	return domain.ErrConflict
}

func scanVersion(row Row) (domain.ContentVersion, error) {
	var version domain.ContentVersion
	var status string
	var snapshot []byte
	err := row.Scan(&version.ID, &status, &version.Revision, &snapshot, &version.Checksum,
		&version.ReviewApproved, &version.CreatedBy, &version.UpdatedBy, &version.CreatedAt, &version.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ContentVersion{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.ContentVersion{}, err
	}
	version.Status = domain.ContentStatus(status)
	canonical, err := snapshotdata.CanonicalJSON(snapshot)
	if err != nil {
		return domain.ContentVersion{}, fmt.Errorf("%w: canonicalize stored snapshot: %v", domain.ErrConflict, err)
	}
	if snapshotdata.Checksum(canonical) != version.Checksum {
		return domain.ContentVersion{}, fmt.Errorf("%w: stored snapshot checksum mismatch", domain.ErrConflict)
	}
	version.Snapshot = append(json.RawMessage(nil), canonical...)
	return version, nil
}

func scanAsset(row Row) (domain.AssetRecord, error) {
	var asset domain.AssetRecord
	var status string
	var metadata, rights []byte
	var sourceURL sql.NullString
	var deletedAt sql.NullTime
	err := row.Scan(&asset.ID, &asset.BlobKey, &sourceURL, &status, &metadata, &rights, &asset.CreatedBy, &asset.CreatedAt, &deletedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.AssetRecord{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.AssetRecord{}, err
	}
	if sourceURL.Valid {
		asset.SourceURL = sourceURL.String
	}
	asset.Status = domain.AssetStatus(status)
	asset.Metadata = append(json.RawMessage(nil), metadata...)
	asset.Rights = append(json.RawMessage(nil), rights...)
	if deletedAt.Valid {
		value := deletedAt.Time
		asset.DeletedAt = &value
	}
	return asset, nil
}

func scanPublishJob(row Row) (domain.PublishJob, error) {
	var job domain.PublishJob
	var operation, status string
	err := row.Scan(&job.ID, &job.IdempotencyKey, &operation, &job.VersionID, &job.ReleaseID, &job.SnapshotKey, &job.SnapshotChecksum,
		&job.TargetRevision, &job.BuildID, &status, &job.ErrorMessage, &job.RequestedBy, &job.CreatedAt, &job.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.PublishJob{}, domain.ErrNotFound
	}
	if err != nil {
		return domain.PublishJob{}, err
	}
	job.Operation = domain.PublishOperation(operation)
	job.Status = domain.PublishStatus(status)
	return job, nil
}

type sqlStateError interface {
	SQLState() string
}

func mapConstraintError(err error) error {
	if err == nil {
		return nil
	}
	var state sqlStateError
	if errors.As(err, &state) && state.SQLState() == "23505" {
		return errors.Join(domain.ErrConflict, err)
	}
	return err
}

var _ content.Repository = (*ContentRepository)(nil)
var _ assets.Repository = (*AssetRepository)(nil)
var _ publish.Repository = (*PublishRepository)(nil)
