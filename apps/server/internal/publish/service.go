package publish

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"yujian.me/server/internal/auth"
	"yujian.me/server/internal/domain"
	"yujian.me/server/internal/ports"
	snapshotdata "yujian.me/server/internal/snapshot"
)

const productionSlot = "production"

type SnapshotValidator interface {
	Validate([]byte) error
}

type Repository interface {
	WithinTransaction(context.Context, func(Repository) error) error
	GetVersion(context.Context, string) (domain.ContentVersion, error)
	GetAsset(context.Context, string) (domain.AssetRecord, error)
	UpdateVersion(context.Context, domain.ContentVersion, int64) error
	CreatePublishJob(context.Context, domain.PublishJob) error
	GetPublishJob(context.Context, string) (domain.PublishJob, error)
	GetPublishJobByIdempotencyKey(context.Context, string) (domain.PublishJob, error)
	GetActivePublishJob(context.Context) (domain.PublishJob, error)
	GetSuccessfulPublishByVersion(context.Context, string) (domain.PublishJob, error)
	UpdatePublishJob(context.Context, domain.PublishJob, domain.PublishStatus) error
	LockPublishSlot(context.Context, string) error
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

type snapshotAsset struct {
	ID       string          `json:"id"`
	Source   string          `json:"src"`
	MIMEType string          `json:"mimeType"`
	ByteSize int64           `json:"byteSize"`
	Checksum string          `json:"checksum"`
	Rights   json.RawMessage `json:"rights"`
}

var managedAssetPath = regexp.MustCompile(`(?:^|/)assets/(asset_[A-Za-z0-9_-]+)/((?:source)\.[A-Za-z0-9]+)$`)

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

// Reconcile advances the single active production job without relying on an
// administrator keeping the management page open.
func (service *Service) Reconcile(ctx context.Context) error {
	job, err := service.repository.GetActivePublishJob(ctx)
	if errors.Is(err, domain.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if job.Status == domain.PublishPending {
		_, err = service.resumeExisting(ctx, job)
		return err
	}
	_, err = service.RefreshStatus(ctx, domain.Principal{
		Subject: "system:publish-reconciler",
		Roles:   []domain.Role{domain.RolePublisher},
	}, job.ID)
	return err
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
	var validKey bool
	idempotencyKey, validKey = domain.NormalizeIdempotencyKey(idempotencyKey)
	if !validKey {
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
	if version.Checksum != canonical.Checksum {
		return domain.PublishJob{}, domain.ErrConflict
	}
	if err := service.validateManagedAssets(ctx, canonical.JSON, false); err != nil {
		return domain.PublishJob{}, err
	}
	snapshotKey := fmt.Sprintf("snapshots/%s/%s.json", canonical.ReleaseID, canonical.Checksum)
	if err := service.ensureSnapshot(ctx, snapshotKey, canonical); err != nil {
		return domain.PublishJob{}, err
	}
	job := service.newJob(actor, domain.PublishOperationPublish, versionID, canonical.ReleaseID, idempotencyKey, snapshotKey, canonical.Checksum, version.Revision)
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
	var validKey bool
	idempotencyKey, validKey = domain.NormalizeIdempotencyKey(idempotencyKey)
	if !validKey {
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
	if err := service.validateManagedAssets(ctx, canonical.JSON, true); err != nil {
		return domain.PublishJob{}, err
	}
	snapshotKey := historical.SnapshotKey
	snapshotChecksum := historical.SnapshotChecksum
	if historical.SnapshotChecksum != canonical.Checksum {
		snapshotKey = fmt.Sprintf("snapshots/%s/%s.json", canonical.ReleaseID, canonical.Checksum)
		if err := service.ensureSnapshot(ctx, snapshotKey, canonical); err != nil {
			return domain.PublishJob{}, err
		}
		snapshotChecksum = canonical.Checksum
	}

	job := service.newJob(
		actor,
		domain.PublishOperationRollback,
		versionID,
		canonical.ReleaseID,
		idempotencyKey,
		snapshotKey,
		snapshotChecksum,
		version.Revision,
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

func (service *Service) validateManagedAssets(ctx context.Context, snapshot []byte, allowDeleted bool) error {
	var envelope struct {
		Assets []snapshotAsset `json:"assets"`
	}
	if err := json.Unmarshal(snapshot, &envelope); err != nil {
		return fmt.Errorf("%w: decode managed assets: %v", domain.ErrInvalidInput, err)
	}
	for _, reference := range envelope.Assets {
		asset, err := service.repository.GetAsset(ctx, reference.ID)
		if errors.Is(err, domain.ErrNotFound) {
			_, managed, parseErr := managedBlobKey(reference)
			if parseErr != nil {
				return parseErr
			}
			if managed {
				return fmt.Errorf("%w: managed asset %s does not exist", domain.ErrInvalidInput, reference.ID)
			}
			continue
		}
		if err != nil {
			return err
		}
		matches := managedAssetPath.FindStringSubmatch("/" + asset.BlobKey)
		if len(matches) == 0 || matches[1] != reference.ID || asset.BlobKey != "assets/"+matches[1]+"/"+matches[2] {
			return fmt.Errorf("%w: managed asset %s has an invalid blob key", domain.ErrInvalidInput, reference.ID)
		}
		blobKey := asset.BlobKey
		validStatus := asset.Status == domain.AssetReady || (allowDeleted && asset.Status == domain.AssetDeleted)
		if !validStatus {
			return fmt.Errorf("%w: managed asset %s is not publishable", domain.ErrInvalidInput, reference.ID)
		}
		expectedSource, err := service.blobStore.PublicURL(ctx, blobKey)
		if err != nil {
			return err
		}
		if expectedSource != reference.Source {
			return fmt.Errorf("%w: managed asset %s public URL does not match", domain.ErrInvalidInput, reference.ID)
		}
		storedRights, storedErr := snapshotdata.CanonicalJSON(asset.Rights)
		referenceRights, referenceErr := snapshotdata.CanonicalJSON(reference.Rights)
		if storedErr != nil || referenceErr != nil || !bytes.Equal(storedRights, referenceRights) {
			return fmt.Errorf("%w: managed asset %s rights do not match", domain.ErrInvalidInput, reference.ID)
		}
		metadata, err := service.blobStore.Stat(ctx, blobKey)
		if errors.Is(err, domain.ErrNotFound) {
			return fmt.Errorf("%w: managed asset %s blob does not exist", domain.ErrInvalidInput, reference.ID)
		}
		if err != nil {
			return err
		}
		if metadata.ContentType != reference.MIMEType || metadata.Size != reference.ByteSize || metadata.Checksum != reference.Checksum {
			return fmt.Errorf("%w: managed asset %s metadata does not match", domain.ErrInvalidInput, reference.ID)
		}
	}
	return nil
}

func managedBlobKey(reference snapshotAsset) (string, bool, error) {
	if strings.TrimSpace(reference.Source) != reference.Source {
		return "", false, fmt.Errorf("%w: ambiguous asset URL", domain.ErrInvalidInput)
	}
	parsed, err := url.Parse(reference.Source)
	if err != nil {
		return "", false, fmt.Errorf("%w: invalid managed asset URL", domain.ErrInvalidInput)
	}
	if parsed.Scheme == "https" && parsed.Host == "" {
		return "", false, fmt.Errorf("%w: HTTPS asset URL requires a host", domain.ErrInvalidInput)
	}
	if strings.ContainsRune(parsed.Path, '\\') {
		return "", false, fmt.Errorf("%w: ambiguous asset URL path", domain.ErrInvalidInput)
	}
	for _, segment := range strings.Split(parsed.Path, "/") {
		if segment == "." || segment == ".." {
			return "", false, fmt.Errorf("%w: ambiguous asset URL path", domain.ErrInvalidInput)
		}
	}
	matches := managedAssetPath.FindStringSubmatch(parsed.Path)
	if len(matches) == 0 {
		return "", false, nil
	}
	if parsed.RawPath != "" {
		return "", true, fmt.Errorf("%w: escaped managed asset URL path", domain.ErrInvalidInput)
	}
	if matches[1] != reference.ID {
		return "", true, fmt.Errorf("%w: managed asset URL does not match id %s", domain.ErrInvalidInput, reference.ID)
	}
	return "assets/" + matches[1] + "/" + matches[2], true, nil
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
		failed, changed, err := service.failBuildingJob(ctx, actor, job, run.Error, true)
		if err != nil {
			return domain.PublishJob{}, err
		}
		if changed {
			service.notify(ctx, failed, domain.PublishPointer{}, false)
		}
		return failed, nil
	}
	if run.Status != domain.PublishSucceeded {
		return job, nil
	}

	var pointer domain.PublishPointer
	alreadyFinalized := false
	finalizationFailed := false
	err = service.repository.WithinTransaction(ctx, func(repository Repository) error {
		if err := repository.LockPublishSlot(ctx, productionSlot); err != nil {
			return err
		}
		persisted, err := repository.GetPublishJob(ctx, job.ID)
		if err != nil {
			return err
		}
		job = persisted
		if job.Status != domain.PublishBuilding {
			alreadyFinalized = true
			return nil
		}
		target, err := repository.GetVersion(ctx, job.VersionID)
		if err != nil {
			return err
		}
		if !service.targetMatchesJob(target, job) {
			if err := service.releasePublishTarget(ctx, repository, actor, target, false); err != nil {
				return err
			}
			job.Status = domain.PublishFailed
			job.ErrorMessage = "publish target changed during build"
			job.UpdatedAt = service.now().UTC()
			if err := repository.UpdatePublishJob(ctx, job, domain.PublishBuilding); err != nil {
				return err
			}
			finalizationFailed = true
			return repository.AppendAudit(ctx, publishAudit(actor, failedAuditAction(job), job.ID, service.now().UTC()))
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
		if err := repository.UpdatePublishJob(ctx, job, domain.PublishBuilding); err != nil {
			return err
		}
		action := "publish.succeeded"
		if job.Operation == domain.PublishOperationRollback {
			action = "rollback.succeeded"
		}
		return repository.AppendAudit(ctx, publishAudit(actor, action, job.ID, service.now().UTC()))
	})
	if err != nil {
		return domain.PublishJob{}, err
	}
	if alreadyFinalized {
		return job, nil
	}
	if finalizationFailed {
		service.notify(ctx, job, domain.PublishPointer{}, false)
		return job, nil
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
		if err := repository.LockPublishSlot(ctx, productionSlot); err != nil {
			return err
		}
		if _, err := repository.GetActivePublishJob(ctx); err == nil {
			return domain.ErrConflict
		} else if !errors.Is(err, domain.ErrNotFound) {
			return err
		}
		target, err := repository.GetVersion(ctx, job.VersionID)
		if err != nil {
			return err
		}
		if target.Revision != job.TargetRevision || target.Checksum != job.SnapshotChecksum {
			return domain.ErrConflict
		}
		if job.Operation == domain.PublishOperationPublish {
			if !domain.CanTransition(target.Status, domain.StatusPublishing, target.ReviewApproved) {
				return domain.ErrInvalidTransition
			}
			targetRevision := target.Revision
			target.Status = domain.StatusPublishing
			target.Revision++
			target.UpdatedBy = actor.Subject
			target.UpdatedAt = service.now().UTC()
			if err := repository.UpdateVersion(ctx, target, targetRevision); err != nil {
				return err
			}
			job.TargetRevision = target.Revision
		} else if target.Status != domain.StatusArchived && target.Status != domain.StatusPublished {
			return domain.ErrInvalidTransition
		}
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
		if updateErr := service.repository.UpdatePublishJob(ctx, job, domain.PublishPending); updateErr != nil {
			if errors.Is(updateErr, domain.ErrConflict) {
				return service.repository.GetPublishJob(ctx, job.ID)
			}
			return domain.PublishJob{}, errors.Join(err, updateErr)
		}
		return job, err
	}
	job.BuildID = run.ID
	job.Status = domain.PublishBuilding
	job.ErrorMessage = ""
	if err := service.repository.UpdatePublishJob(ctx, job, domain.PublishPending); err != nil {
		if errors.Is(err, domain.ErrConflict) {
			return service.repository.GetPublishJob(ctx, job.ID)
		}
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
	targetRevision int64,
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
		TargetRevision:   targetRevision,
		Status:           domain.PublishPending,
		RequestedBy:      actor.Subject,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
}

func (service *Service) targetMatchesJob(target domain.ContentVersion, job domain.PublishJob) bool {
	if target.Revision != job.TargetRevision || target.Checksum != job.SnapshotChecksum {
		return false
	}
	if job.Operation == domain.PublishOperationPublish {
		return target.Status == domain.StatusPublishing && target.ReviewApproved
	}
	return target.Status == domain.StatusArchived || target.Status == domain.StatusPublished
}

func (service *Service) failBuildingJob(
	ctx context.Context,
	actor domain.Principal,
	job domain.PublishJob,
	errorMessage string,
	releaseTarget bool,
) (domain.PublishJob, bool, error) {
	changed := false
	err := service.repository.WithinTransaction(ctx, func(repository Repository) error {
		if err := repository.LockPublishSlot(ctx, productionSlot); err != nil {
			return err
		}
		persisted, err := repository.GetPublishJob(ctx, job.ID)
		if err != nil {
			return err
		}
		job = persisted
		if job.Status != domain.PublishBuilding {
			return nil
		}
		if releaseTarget && job.Operation == domain.PublishOperationPublish {
			target, err := repository.GetVersion(ctx, job.VersionID)
			if err != nil {
				return err
			}
			if target.Status == domain.StatusPublishing {
				if err := service.releasePublishTarget(ctx, repository, actor, target, service.targetMatchesJob(target, job)); err != nil {
					return err
				}
			}
		}
		job.Status = domain.PublishFailed
		job.ErrorMessage = errorMessage
		job.UpdatedAt = service.now().UTC()
		if err := repository.UpdatePublishJob(ctx, job, domain.PublishBuilding); err != nil {
			return err
		}
		changed = true
		return repository.AppendAudit(ctx, publishAudit(actor, failedAuditAction(job), job.ID, service.now().UTC()))
	})
	if err != nil {
		if errors.Is(err, domain.ErrConflict) {
			current, lookupErr := service.repository.GetPublishJob(ctx, job.ID)
			return current, false, lookupErr
		}
		return domain.PublishJob{}, false, err
	}
	return job, changed, nil
}

func (service *Service) releasePublishTarget(
	ctx context.Context,
	repository Repository,
	actor domain.Principal,
	target domain.ContentVersion,
	preserveApproval bool,
) error {
	if target.Status != domain.StatusPublishing {
		return nil
	}
	targetRevision := target.Revision
	if preserveApproval {
		target.Status = domain.StatusInReview
	} else {
		target.Status = domain.StatusDraft
		target.ReviewApproved = false
	}
	target.Revision++
	target.UpdatedBy = actor.Subject
	target.UpdatedAt = service.now().UTC()
	return repository.UpdateVersion(ctx, target, targetRevision)
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
	canonicalJSON, err := snapshotdata.CanonicalJSON(raw)
	if err != nil {
		return canonicalResult{}, fmt.Errorf("decode snapshot: %w", err)
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
	return canonicalResult{
		JSON:      canonicalJSON,
		Checksum:  snapshotdata.Checksum(canonicalJSON),
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

func failedAuditAction(job domain.PublishJob) string {
	if job.Operation == domain.PublishOperationRollback {
		return "rollback.failed"
	}
	return "publish.failed"
}

func randomID(prefix string) string {
	buffer := make([]byte, 12)
	if _, err := rand.Read(buffer); err != nil {
		panic("secure random source unavailable")
	}
	return prefix + hex.EncodeToString(buffer)
}
