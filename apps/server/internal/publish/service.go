package publish

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"yujian.me/server/internal/auth"
	"yujian.me/server/internal/domain"
	"yujian.me/server/internal/ports"
)

const productionSlot = "production"

type SnapshotValidator interface {
	Validate([]byte) error
}

type Repository interface {
	WithinTransaction(context.Context, func(Repository) error) error
	GetVersion(context.Context, string) (domain.ContentVersion, error)
	UpdateVersion(context.Context, domain.ContentVersion, int64) error
	CreatePublishJob(context.Context, domain.PublishJob) error
	GetPublishJob(context.Context, string) (domain.PublishJob, error)
	GetPublishJobByIdempotencyKey(context.Context, string) (domain.PublishJob, error)
	GetSuccessfulPublishByVersion(context.Context, string) (domain.PublishJob, error)
	UpdatePublishJob(context.Context, domain.PublishJob) error
	GetPublishPointer(context.Context, string) (domain.PublishPointer, error)
	SetPublishPointer(context.Context, domain.PublishPointer) error
	AppendAudit(context.Context, domain.AuditEntry) error
}

type ServiceOptions struct {
	Repository   Repository
	BlobStore    ports.BlobStore
	BuildTrigger ports.BuildTrigger
	Notifier     ports.Notifier
	Validator    SnapshotValidator
	Now          func() time.Time
	NewID        func(string) string
}

type Service struct {
	repository   Repository
	blobStore    ports.BlobStore
	buildTrigger ports.BuildTrigger
	notifier     ports.Notifier
	validator    SnapshotValidator
	now          func() time.Time
	newID        func(string) string
}

type canonicalResult struct {
	JSON      []byte
	Checksum  string
	ReleaseID string
}

func NewService(options ServiceOptions) *Service {
	now := options.Now
	if now == nil {
		now = time.Now
	}
	newID := options.NewID
	if newID == nil {
		newID = randomID
	}
	return &Service{
		repository:   options.Repository,
		blobStore:    options.BlobStore,
		buildTrigger: options.BuildTrigger,
		notifier:     options.Notifier,
		validator:    options.Validator,
		now:          now,
		newID:        newID,
	}
}

// GetPublishJob reads a publish task for a publisher or administrator.
func (service *Service) GetPublishJob(
	ctx context.Context,
	actor domain.Principal,
	id string,
) (domain.PublishJob, error) {
	if !hasPermission(actor, auth.PermissionPublish) {
		return domain.PublishJob{}, domain.ErrForbidden
	}
	return service.repository.GetPublishJob(ctx, id)
}

func (service *Service) Publish(
	ctx context.Context,
	actor domain.Principal,
	versionID string,
	idempotencyKey string,
) (domain.PublishJob, error) {
	if !hasPermission(actor, auth.PermissionPublish) {
		return domain.PublishJob{}, domain.ErrForbidden
	}
	if idempotencyKey == "" {
		return domain.PublishJob{}, domain.ErrInvalidInput
	}
	if existing, found, err := service.existingJob(ctx, actor, domain.PublishOperationPublish, versionID, idempotencyKey); err != nil {
		return domain.PublishJob{}, err
	} else if found {
		return service.resumeExisting(ctx, existing)
	}

	version, err := service.repository.GetVersion(ctx, versionID)
	if err != nil {
		return domain.PublishJob{}, err
	}
	if version.Status != domain.StatusInReview || !version.ReviewApproved {
		return domain.PublishJob{}, domain.ErrInvalidTransition
	}
	if err := service.validator.Validate(version.Snapshot); err != nil {
		return domain.PublishJob{}, fmt.Errorf("%w: validate publish snapshot: %v", domain.ErrInvalidInput, err)
	}
	canonical, err := canonicalSnapshot(version.Snapshot)
	if err != nil {
		return domain.PublishJob{}, err
	}
	snapshotKey := fmt.Sprintf("snapshots/%s/%s.json", canonical.ReleaseID, canonical.Checksum)
	if err := service.ensureSnapshot(ctx, snapshotKey, canonical); err != nil {
		return domain.PublishJob{}, err
	}

	job := service.newJob(actor, domain.PublishOperationPublish, versionID, canonical.ReleaseID, idempotencyKey, snapshotKey, canonical.Checksum)
	if err := service.createJob(ctx, actor, &job, "publish.requested"); err != nil {
		if errors.Is(err, domain.ErrConflict) {
			if existing, found, lookupErr := service.existingJob(ctx, actor, domain.PublishOperationPublish, versionID, idempotencyKey); lookupErr != nil {
				return domain.PublishJob{}, lookupErr
			} else if found {
				return service.resumeExisting(ctx, existing)
			}
		}
		return domain.PublishJob{}, err
	}

	return service.trigger(ctx, job, canonical.ReleaseID)
}

func (service *Service) Rollback(
	ctx context.Context,
	actor domain.Principal,
	versionID string,
	idempotencyKey string,
) (domain.PublishJob, error) {
	if !hasPermission(actor, auth.PermissionRollback) {
		return domain.PublishJob{}, domain.ErrForbidden
	}
	if idempotencyKey == "" {
		return domain.PublishJob{}, domain.ErrInvalidInput
	}
	if existing, found, err := service.existingJob(ctx, actor, domain.PublishOperationRollback, versionID, idempotencyKey); err != nil {
		return domain.PublishJob{}, err
	} else if found {
		return service.resumeExisting(ctx, existing)
	}

	version, err := service.repository.GetVersion(ctx, versionID)
	if err != nil {
		return domain.PublishJob{}, err
	}
	if version.Status != domain.StatusArchived && version.Status != domain.StatusPublished {
		return domain.PublishJob{}, domain.ErrInvalidTransition
	}
	historical, err := service.repository.GetSuccessfulPublishByVersion(ctx, versionID)
	if err != nil {
		return domain.PublishJob{}, err
	}
	if err := service.validator.Validate(version.Snapshot); err != nil {
		return domain.PublishJob{}, fmt.Errorf("%w: validate rollback snapshot: %v", domain.ErrInvalidInput, err)
	}
	canonical, err := canonicalSnapshot(version.Snapshot)
	if err != nil {
		return domain.PublishJob{}, err
	}

	job := service.newJob(
		actor,
		domain.PublishOperationRollback,
		versionID,
		canonical.ReleaseID,
		idempotencyKey,
		historical.SnapshotKey,
		historical.SnapshotChecksum,
	)
	if err := service.createJob(ctx, actor, &job, "rollback.requested"); err != nil {
		if errors.Is(err, domain.ErrConflict) {
			if existing, found, lookupErr := service.existingJob(ctx, actor, domain.PublishOperationRollback, versionID, idempotencyKey); lookupErr != nil {
				return domain.PublishJob{}, lookupErr
			} else if found {
				return service.resumeExisting(ctx, existing)
			}
		}
		return domain.PublishJob{}, err
	}
	return service.trigger(ctx, job, canonical.ReleaseID)
}

func (service *Service) RefreshStatus(
	ctx context.Context,
	actor domain.Principal,
	jobID string,
) (domain.PublishJob, error) {
	if !hasPermission(actor, auth.PermissionPublish) {
		return domain.PublishJob{}, domain.ErrForbidden
	}
	job, err := service.repository.GetPublishJob(ctx, jobID)
	if err != nil {
		return domain.PublishJob{}, err
	}
	if job.Status != domain.PublishBuilding {
		return job, nil
	}
	run, err := service.buildTrigger.Status(ctx, job.BuildID)
	if err != nil {
		return job, err
	}
	if run.Status == domain.PublishFailed {
		job.Status = domain.PublishFailed
		job.ErrorMessage = run.Error
		job.UpdatedAt = service.now().UTC()
		if err := service.repository.UpdatePublishJob(ctx, job); err != nil {
			return domain.PublishJob{}, err
		}
		service.notify(ctx, job, domain.PublishPointer{}, false)
		return job, nil
	}
	if run.Status != domain.PublishSucceeded {
		return job, nil
	}

	var pointer domain.PublishPointer
	err = service.repository.WithinTransaction(ctx, func(repository Repository) error {
		target, err := repository.GetVersion(ctx, job.VersionID)
		if err != nil {
			return err
		}
		currentPointer, pointerErr := repository.GetPublishPointer(ctx, productionSlot)
		if pointerErr != nil && !errors.Is(pointerErr, domain.ErrNotFound) {
			return pointerErr
		}
		if pointerErr == nil && currentPointer.VersionID != target.ID {
			current, err := repository.GetVersion(ctx, currentPointer.VersionID)
			if err != nil {
				return err
			}
			currentRevision := current.Revision
			current.Status = domain.StatusArchived
			current.Revision++
			current.UpdatedBy = actor.Subject
			current.UpdatedAt = service.now().UTC()
			if err := repository.UpdateVersion(ctx, current, currentRevision); err != nil {
				return err
			}
		}

		targetRevision := target.Revision
		if target.Status == domain.StatusInReview {
			if !domain.CanTransition(target.Status, domain.StatusPublished, target.ReviewApproved) {
				return domain.ErrInvalidTransition
			}
		} else if target.Status != domain.StatusArchived && target.Status != domain.StatusPublished {
			return domain.ErrInvalidTransition
		}
		target.Status = domain.StatusPublished
		target.Revision++
		target.UpdatedBy = actor.Subject
		target.UpdatedAt = service.now().UTC()
		if err := repository.UpdateVersion(ctx, target, targetRevision); err != nil {
			return err
		}

		pointer = domain.PublishPointer{
			Slot:             productionSlot,
			VersionID:        target.ID,
			SnapshotKey:      job.SnapshotKey,
			SnapshotChecksum: job.SnapshotChecksum,
			UpdatedAt:        service.now().UTC(),
		}
		if err := repository.SetPublishPointer(ctx, pointer); err != nil {
			return err
		}
		job.Status = domain.PublishSucceeded
		job.ErrorMessage = ""
		job.UpdatedAt = service.now().UTC()
		if err := repository.UpdatePublishJob(ctx, job); err != nil {
			return err
		}
		return repository.AppendAudit(ctx, publishAudit(actor, "publish.succeeded", job.ID, service.now().UTC()))
	})
	if err != nil {
		return domain.PublishJob{}, err
	}
	service.notify(ctx, job, pointer, true)
	return job, nil
}

func (service *Service) ensureSnapshot(
	ctx context.Context,
	key string,
	canonical canonicalResult,
) error {
	metadata, err := service.blobStore.Stat(ctx, key)
	if err == nil {
		if metadata.Checksum != canonical.Checksum {
			return domain.ErrConflict
		}
		return nil
	}
	if !errors.Is(err, domain.ErrNotFound) {
		return err
	}
	return service.blobStore.Put(ctx, key, bytes.NewReader(canonical.JSON), ports.BlobMetadata{
		ContentType: "application/json",
		Size:        int64(len(canonical.JSON)),
		Checksum:    canonical.Checksum,
	})
}

func (service *Service) createJob(
	ctx context.Context,
	actor domain.Principal,
	job *domain.PublishJob,
	action string,
) error {
	return service.repository.WithinTransaction(ctx, func(repository Repository) error {
		if err := repository.CreatePublishJob(ctx, *job); err != nil {
			return err
		}
		return repository.AppendAudit(ctx, publishAudit(actor, action, job.ID, job.CreatedAt))
	})
}

func (service *Service) trigger(
	ctx context.Context,
	job domain.PublishJob,
	releaseID string,
) (domain.PublishJob, error) {
	run, err := service.buildTrigger.Trigger(ctx, ports.BuildRequest{
		IdempotencyKey:   job.IdempotencyKey,
		ReleaseID:        releaseID,
		SnapshotKey:      job.SnapshotKey,
		SnapshotChecksum: job.SnapshotChecksum,
	})
	job.UpdatedAt = service.now().UTC()
	if err != nil {
		job.Status = domain.PublishPending
		job.ErrorMessage = err.Error()
		if updateErr := service.repository.UpdatePublishJob(ctx, job); updateErr != nil {
			return domain.PublishJob{}, errors.Join(err, updateErr)
		}
		return job, err
	}
	job.BuildID = run.ID
	job.Status = domain.PublishBuilding
	job.ErrorMessage = ""
	if err := service.repository.UpdatePublishJob(ctx, job); err != nil {
		return domain.PublishJob{}, err
	}
	return job, nil
}

func (service *Service) resumeExisting(ctx context.Context, job domain.PublishJob) (domain.PublishJob, error) {
	if job.Status != domain.PublishPending {
		return job, nil
	}
	return service.trigger(ctx, job, job.ReleaseID)
}

func (service *Service) newJob(
	actor domain.Principal,
	operation domain.PublishOperation,
	versionID string,
	releaseID string,
	idempotencyKey string,
	snapshotKey string,
	snapshotChecksum string,
) domain.PublishJob {
	now := service.now().UTC()
	return domain.PublishJob{
		ID:               service.newID("pub_"),
		IdempotencyKey:   idempotencyKey,
		Operation:        operation,
		VersionID:        versionID,
		ReleaseID:        releaseID,
		SnapshotKey:      snapshotKey,
		SnapshotChecksum: snapshotChecksum,
		Status:           domain.PublishPending,
		RequestedBy:      actor.Subject,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
}

func (service *Service) existingJob(
	ctx context.Context,
	actor domain.Principal,
	operation domain.PublishOperation,
	versionID string,
	idempotencyKey string,
) (domain.PublishJob, bool, error) {
	existing, err := service.repository.GetPublishJobByIdempotencyKey(ctx, idempotencyKey)
	if errors.Is(err, domain.ErrNotFound) {
		return domain.PublishJob{}, false, nil
	}
	if err != nil {
		return domain.PublishJob{}, false, err
	}
	if existing.Operation != operation || existing.VersionID != versionID || existing.RequestedBy != actor.Subject {
		return domain.PublishJob{}, false, domain.ErrConflict
	}
	return existing, true, nil
}

func (service *Service) notify(
	ctx context.Context,
	job domain.PublishJob,
	pointer domain.PublishPointer,
	succeeded bool,
) {
	if service.notifier == nil {
		return
	}
	_ = service.notifier.PublishCompleted(ctx, ports.PublishResult{
		Job:       job,
		Pointer:   pointer,
		Succeeded: succeeded,
	})
}

func canonicalSnapshot(raw []byte) (canonicalResult, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return canonicalResult{}, fmt.Errorf("decode snapshot: %w", err)
	}
	canonicalJSON, err := json.Marshal(value)
	if err != nil {
		return canonicalResult{}, fmt.Errorf("encode canonical snapshot: %w", err)
	}
	var envelope struct {
		ReleaseID string `json:"releaseId"`
	}
	if err := json.Unmarshal(canonicalJSON, &envelope); err != nil {
		return canonicalResult{}, err
	}
	if envelope.ReleaseID == "" {
		return canonicalResult{}, domain.ErrInvalidInput
	}
	sum := sha256.Sum256(canonicalJSON)
	return canonicalResult{
		JSON:      canonicalJSON,
		Checksum:  "sha256:" + hex.EncodeToString(sum[:]),
		ReleaseID: envelope.ReleaseID,
	}, nil
}

func hasPermission(actor domain.Principal, permission auth.Permission) bool {
	for _, role := range actor.Roles {
		if auth.Can(role, permission) {
			return true
		}
	}
	return false
}

func publishAudit(actor domain.Principal, action, id string, createdAt time.Time) domain.AuditEntry {
	return domain.AuditEntry{
		ActorSubject: actor.Subject,
		Action:       action,
		ResourceType: "publish_job",
		ResourceID:   id,
		Metadata:     json.RawMessage(`{}`),
		CreatedAt:    createdAt,
	}
}

func randomID(prefix string) string {
	buffer := make([]byte, 12)
	if _, err := rand.Read(buffer); err != nil {
		panic("secure random source unavailable")
	}
	return prefix + hex.EncodeToString(buffer)
}
