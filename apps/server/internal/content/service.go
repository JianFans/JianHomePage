package content

import (
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
)

type SnapshotValidator interface {
	Validate([]byte) error
}

type Repository interface {
	WithinTransaction(context.Context, func(Repository) error) error
	CreateVersion(context.Context, domain.ContentVersion) error
	GetVersion(context.Context, string) (domain.ContentVersion, error)
	UpdateVersion(context.Context, domain.ContentVersion, int64) error
	AppendAudit(context.Context, domain.AuditEntry) error
}

type ServiceOptions struct {
	Store     Repository
	Validator SnapshotValidator
	Now       func() time.Time
	NewID     func(string) string
}

type Service struct {
	store     Repository
	validator SnapshotValidator
	now       func() time.Time
	newID     func(string) string
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
		store:     options.Store,
		validator: options.Validator,
		now:       now,
		newID:     newID,
	}
}

// GetVersion returns a content version to an authenticated management user.
// The HTTP layer is responsible for authentication; the service still checks
// that the principal carries at least one content-management role so callers
// cannot use an empty or forged principal.
func (service *Service) GetVersion(
	ctx context.Context,
	actor domain.Principal,
	id string,
) (domain.ContentVersion, error) {
	if actor.Subject == "" || !canRead(actor) {
		return domain.ContentVersion{}, domain.ErrForbidden
	}
	version, err := service.store.GetVersion(ctx, id)
	if err != nil {
		return domain.ContentVersion{}, err
	}
	version.Snapshot = cloneJSON(version.Snapshot)
	return version, nil
}

func (service *Service) CreateDraft(
	ctx context.Context,
	actor domain.Principal,
	snapshot json.RawMessage,
) (domain.ContentVersion, error) {
	if !hasPermission(actor, auth.PermissionEditDraft) {
		return domain.ContentVersion{}, domain.ErrForbidden
	}
	if err := service.validator.Validate(snapshot); err != nil {
		return domain.ContentVersion{}, fmt.Errorf("%w: validate snapshot: %v", domain.ErrInvalidInput, err)
	}

	now := service.now().UTC()
	version := domain.ContentVersion{
		ID:        service.newID("ver_"),
		Status:    domain.StatusDraft,
		Revision:  1,
		Snapshot:  cloneJSON(snapshot),
		Checksum:  checksum(snapshot),
		CreatedBy: actor.Subject,
		UpdatedBy: actor.Subject,
		CreatedAt: now,
		UpdatedAt: now,
	}

	err := service.store.WithinTransaction(ctx, func(repository Repository) error {
		if err := repository.CreateVersion(ctx, version); err != nil {
			return err
		}
		return repository.AppendAudit(ctx, audit(actor, "content.create_draft", version.ID, now, nil))
	})
	if err != nil {
		return domain.ContentVersion{}, err
	}
	return version, nil
}

func (service *Service) UpdateDraft(
	ctx context.Context,
	actor domain.Principal,
	id string,
	expectedRevision int64,
	snapshot json.RawMessage,
) (domain.ContentVersion, error) {
	if !hasPermission(actor, auth.PermissionEditDraft) {
		return domain.ContentVersion{}, domain.ErrForbidden
	}
	if err := service.validator.Validate(snapshot); err != nil {
		return domain.ContentVersion{}, fmt.Errorf("%w: validate snapshot: %v", domain.ErrInvalidInput, err)
	}

	return service.mutate(ctx, actor, id, expectedRevision, "content.update_draft", nil, func(version *domain.ContentVersion) error {
		if version.Status != domain.StatusDraft {
			return domain.ErrInvalidTransition
		}
		version.Snapshot = cloneJSON(snapshot)
		version.Checksum = checksum(snapshot)
		return nil
	})
}

func (service *Service) SubmitReview(
	ctx context.Context,
	actor domain.Principal,
	id string,
	expectedRevision int64,
) (domain.ContentVersion, error) {
	if !hasPermission(actor, auth.PermissionSubmitReview) {
		return domain.ContentVersion{}, domain.ErrForbidden
	}
	return service.mutate(ctx, actor, id, expectedRevision, "content.submit_review", nil, func(version *domain.ContentVersion) error {
		if !domain.CanTransition(version.Status, domain.StatusInReview, version.ReviewApproved) {
			return domain.ErrInvalidTransition
		}
		version.Status = domain.StatusInReview
		version.ReviewApproved = false
		return nil
	})
}

func (service *Service) ApproveReview(
	ctx context.Context,
	actor domain.Principal,
	id string,
	expectedRevision int64,
) (domain.ContentVersion, error) {
	if !hasPermission(actor, auth.PermissionReview) {
		return domain.ContentVersion{}, domain.ErrForbidden
	}
	return service.mutate(ctx, actor, id, expectedRevision, "content.approve_review", nil, func(version *domain.ContentVersion) error {
		if version.Status != domain.StatusInReview {
			return domain.ErrInvalidTransition
		}
		version.ReviewApproved = true
		return nil
	})
}

func (service *Service) RejectReview(
	ctx context.Context,
	actor domain.Principal,
	id string,
	expectedRevision int64,
	reason string,
) (domain.ContentVersion, error) {
	if !hasPermission(actor, auth.PermissionReview) {
		return domain.ContentVersion{}, domain.ErrForbidden
	}
	metadata, err := json.Marshal(map[string]string{"reason": reason})
	if err != nil {
		return domain.ContentVersion{}, err
	}
	return service.mutate(ctx, actor, id, expectedRevision, "content.reject_review", metadata, func(version *domain.ContentVersion) error {
		if !domain.CanTransition(version.Status, domain.StatusDraft, version.ReviewApproved) {
			return domain.ErrInvalidTransition
		}
		version.Status = domain.StatusDraft
		version.ReviewApproved = false
		return nil
	})
}

func (service *Service) mutate(
	ctx context.Context,
	actor domain.Principal,
	id string,
	expectedRevision int64,
	action string,
	metadata json.RawMessage,
	change func(*domain.ContentVersion) error,
) (domain.ContentVersion, error) {
	var updated domain.ContentVersion
	err := service.store.WithinTransaction(ctx, func(repository Repository) error {
		current, err := repository.GetVersion(ctx, id)
		if err != nil {
			return err
		}
		if current.Revision != expectedRevision {
			return domain.ErrConflict
		}
		if err := change(&current); err != nil {
			return err
		}

		now := service.now().UTC()
		current.Revision++
		current.UpdatedBy = actor.Subject
		current.UpdatedAt = now
		if err := repository.UpdateVersion(ctx, current, expectedRevision); err != nil {
			return err
		}
		if err := repository.AppendAudit(ctx, audit(actor, action, current.ID, now, metadata)); err != nil {
			return err
		}
		updated = current
		return nil
	})
	if err != nil {
		return domain.ContentVersion{}, err
	}
	return updated, nil
}

func hasPermission(actor domain.Principal, permission auth.Permission) bool {
	for _, role := range actor.Roles {
		if auth.Can(role, permission) {
			return true
		}
	}
	return false
}

func canRead(actor domain.Principal) bool {
	return hasPermission(actor, auth.PermissionEditDraft) ||
		hasPermission(actor, auth.PermissionReview) ||
		hasPermission(actor, auth.PermissionPublish) ||
		hasPermission(actor, auth.PermissionRollback)
}

func checksum(value []byte) string {
	sum := sha256.Sum256(value)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func cloneJSON(value []byte) json.RawMessage {
	return append(json.RawMessage(nil), value...)
}

func audit(
	actor domain.Principal,
	action string,
	resourceID string,
	createdAt time.Time,
	metadata json.RawMessage,
) domain.AuditEntry {
	if len(metadata) == 0 {
		metadata = json.RawMessage(`{}`)
	}
	return domain.AuditEntry{
		ActorSubject: actor.Subject,
		Action:       action,
		ResourceType: "content_version",
		ResourceID:   resourceID,
		Metadata:     metadata,
		CreatedAt:    createdAt,
	}
}

func randomID(prefix string) string {
	buffer := make([]byte, 12)
	if _, err := rand.Read(buffer); err != nil {
		panic(errors.New("secure random source unavailable"))
	}
	return prefix + hex.EncodeToString(buffer)
}
