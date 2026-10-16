package postgres

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"yujian.me/server/internal/domain"
	"yujian.me/server/internal/ports"
	"yujian.me/server/internal/publisher"
)

// PublisherLocker supplies an exclusive pinned connection without opening a transaction.
type PublisherLocker interface {
	WithPublisherLock(context.Context, func(Executor) error) error
}

// PublisherRepository persists the historical publisher_builds contract.
type PublisherRepository struct {
	exec   Executor
	locker PublisherLocker
}

// NewPublisherRepository binds SQL persistence and cross-process worker serialization.
func NewPublisherRepository(exec Executor, locker PublisherLocker) *PublisherRepository {
	return &PublisherRepository{exec: exec, locker: locker}
}

// Enqueue retains one frozen task per idempotency key.
func (r *PublisherRepository) Enqueue(ctx context.Context, key string, request ports.BuildRequest, target publisher.Target) (publisher.Task, error) {
	request.IdempotencyKey = ""
	requestJSON, err := json.Marshal(request)
	if err != nil {
		return publisher.Task{}, err
	}
	targetJSON, err := json.Marshal(target)
	if err != nil {
		return publisher.Task{}, err
	}
	id := fmt.Sprintf("build_%x", sha256.Sum256([]byte(key)))
	task, err := scanPublisherTask(r.exec.QueryRowContext(ctx, `INSERT INTO publisher_builds (id,idempotency_key,request,target,phase)
VALUES ($1,$2,$3::jsonb,$4::jsonb,'queued') ON CONFLICT (idempotency_key) DO NOTHING RETURNING `+publisherColumns, id, key, requestJSON, targetJSON))
	if err != nil && !errors.Is(err, domain.ErrNotFound) && !errors.Is(mapConstraintError(err), domain.ErrConflict) {
		return publisher.Task{}, err
	}
	if err != nil {
		task, err = scanPublisherTask(r.exec.QueryRowContext(ctx, publisherSelect+` WHERE idempotency_key = $1`, key))
	}
	if errors.Is(err, domain.ErrNotFound) {
		return publisher.Task{}, domain.ErrConflict
	}
	if err != nil {
		return publisher.Task{}, err
	}
	if task.Request != request {
		return publisher.Task{}, domain.ErrConflict
	}
	return task, nil
}

// Get reads one task, including its immutable request and target.
func (r *PublisherRepository) Get(ctx context.Context, id string) (publisher.Task, error) {
	return scanPublisherTask(r.exec.QueryRowContext(ctx, publisherSelect+` WHERE id = $1`, id))
}

// Active reads the partial unique-index slot.
func (r *PublisherRepository) Active(ctx context.Context) (publisher.Task, error) {
	return scanPublisherTask(r.exec.QueryRowContext(ctx, publisherSelect+` WHERE phase IN ('queued','building','submitting','deployed') LIMIT 1`))
}

// Update commits a phase-checked state transition before returning.
func (r *PublisherRepository) Update(ctx context.Context, task publisher.Task, expected string) (publisher.Task, error) {
	if task.Reconciliation == nil {
		task.Reconciliation = []publisher.RecoveryRecord{}
	}
	records, err := json.Marshal(task.Reconciliation)
	if err != nil {
		return publisher.Task{}, err
	}
	updated, err := scanPublisherTask(r.exec.QueryRowContext(ctx, `UPDATE publisher_builds
SET phase = $3,deployment_id = NULLIF($4,''),preview_url = $5,error = $6,reconciliation = $7::jsonb,updated_at = now()
WHERE id = $1 AND phase = $2 RETURNING `+publisherColumns, task.ID, expected, task.Phase, task.DeploymentID, task.PreviewURL, task.Error, records))
	if errors.Is(err, domain.ErrNotFound) {
		return publisher.Task{}, domain.ErrConflict
	}
	return updated, mapConstraintError(err)
}

// WithLock routes every callback read and write through the pinned lock session.
func (r *PublisherRepository) WithLock(ctx context.Context, run func(publisher.Repository) error) error {
	if r.locker == nil {
		return errors.New("Publisher lock connection is required")
	}
	return r.locker.WithPublisherLock(ctx, func(exec Executor) error { return run(NewPublisherRepository(exec, nil)) })
}

// Health verifies the historical migration exists without modifying task state.
func (r *PublisherRepository) Health(ctx context.Context) error {
	rows, err := r.exec.QueryContext(ctx, `SELECT id,phase,target,reconciliation FROM publisher_builds LIMIT 0`)
	if err != nil {
		return err
	}
	defer rows.Close()
	return rows.Err()
}

const publisherColumns = `id,request,target,phase,COALESCE(deployment_id,''),preview_url,error,reconciliation,created_at`
const publisherSelect = `SELECT ` + publisherColumns + ` FROM publisher_builds`

// scanPublisherTask decodes JSONB without relying on serialized object key order.
func scanPublisherTask(row Row) (publisher.Task, error) {
	var task publisher.Task
	var request, target, records []byte
	err := row.Scan(&task.ID, &request, &target, &task.Phase, &task.DeploymentID, &task.PreviewURL, &task.Error, &records, &task.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return publisher.Task{}, domain.ErrNotFound
	}
	if err != nil {
		return publisher.Task{}, err
	}
	if err := json.Unmarshal(request, &task.Request); err != nil {
		return publisher.Task{}, err
	}
	if err := json.Unmarshal(target, &task.Target); err != nil {
		return publisher.Task{}, err
	}
	if err := json.Unmarshal(records, &task.Reconciliation); err != nil {
		return publisher.Task{}, err
	}
	return task, nil
}

var _ publisher.Repository = (*PublisherRepository)(nil)
