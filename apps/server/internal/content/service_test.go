package content

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"yujian.me/server/internal/domain"
)

type memoryStore struct {
	versions map[string]domain.ContentVersion
	audits   []domain.AuditEntry
}

func newMemoryStore() *memoryStore {
	return &memoryStore{versions: make(map[string]domain.ContentVersion)}
}

func (store *memoryStore) WithinTransaction(ctx context.Context, run func(Repository) error) error {
	return run(store)
}

func (store *memoryStore) CreateVersion(_ context.Context, version domain.ContentVersion) error {
	if _, exists := store.versions[version.ID]; exists {
		return domain.ErrConflict
	}
	store.versions[version.ID] = version
	return nil
}

func (store *memoryStore) GetVersion(_ context.Context, id string) (domain.ContentVersion, error) {
	version, exists := store.versions[id]
	if !exists {
		return domain.ContentVersion{}, domain.ErrNotFound
	}
	return version, nil
}

func (store *memoryStore) UpdateVersion(_ context.Context, version domain.ContentVersion, expectedRevision int64) error {
	current, exists := store.versions[version.ID]
	if !exists {
		return domain.ErrNotFound
	}
	if current.Revision != expectedRevision {
		return domain.ErrConflict
	}
	store.versions[version.ID] = version
	return nil
}

func (store *memoryStore) AppendAudit(_ context.Context, entry domain.AuditEntry) error {
	store.audits = append(store.audits, entry)
	return nil
}

type validatorStub struct {
	err error
}

func (validator validatorStub) Validate([]byte) error {
	return validator.err
}

func newServiceForTest(store *memoryStore, validator SnapshotValidator) *Service {
	return NewService(ServiceOptions{
		Store:     store,
		Validator: validator,
		Now: func() time.Time {
			return time.Date(2026, 8, 29, 10, 0, 0, 0, time.UTC)
		},
		NewID: func(prefix string) string { return prefix + "fixed" },
	})
}

func principal(subject string, roles ...domain.Role) domain.Principal {
	return domain.Principal{Subject: subject, Roles: roles}
}

func validSnapshot() json.RawMessage {
	return json.RawMessage(`{"schemaVersion":"1.0.0"}`)
}

func TestCreateDraftValidatesSnapshotAndWritesAudit(t *testing.T) {
	store := newMemoryStore()
	service := newServiceForTest(store, validatorStub{})

	version, err := service.CreateDraft(
		context.Background(),
		principal("editor-1", domain.RoleEditor),
		validSnapshot(),
	)
	if err != nil {
		t.Fatalf("create draft: %v", err)
	}
	if version.Status != domain.StatusDraft || version.Revision != 1 {
		t.Fatalf("unexpected version %#v", version)
	}
	if version.CreatedBy != "editor-1" || version.UpdatedBy != "editor-1" {
		t.Fatalf("unexpected actors %#v", version)
	}
	if len(store.audits) != 1 || store.audits[0].Action != "content.create_draft" {
		t.Fatalf("unexpected audit log %#v", store.audits)
	}
}

func TestCreateDraftStoresCanonicalSnapshotWithMatchingChecksum(t *testing.T) {
	store := newMemoryStore()
	service := newServiceForTest(store, validatorStub{})
	raw := json.RawMessage("{\n  \"z\": 1.0e0, \"schemaVersion\": \"1.0.0\", \"a\": true\n}")

	version, err := service.CreateDraft(
		context.Background(),
		principal("editor-1", domain.RoleEditor),
		raw,
	)
	if err != nil {
		t.Fatalf("create draft: %v", err)
	}
	expected := json.RawMessage(`{"a":true,"schemaVersion":"1.0.0","z":1}`)
	if string(version.Snapshot) != string(expected) {
		t.Fatalf("snapshot was not canonicalized: %s", version.Snapshot)
	}
	digest := sha256.Sum256(expected)
	if version.Checksum != fmt.Sprintf("sha256:%x", digest) {
		t.Fatalf("checksum %q does not match snapshot", version.Checksum)
	}
}

func TestCreateDraftRejectsInvalidSnapshot(t *testing.T) {
	store := newMemoryStore()
	service := newServiceForTest(store, validatorStub{err: errors.New("invalid snapshot")})

	_, err := service.CreateDraft(
		context.Background(),
		principal("editor-1", domain.RoleEditor),
		validSnapshot(),
	)
	if err == nil {
		t.Fatal("expected validation error")
	}
	if len(store.versions) != 0 {
		t.Fatal("invalid snapshot must not be stored")
	}
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("expected invalid input classification, got %v", err)
	}
}

func TestUpdateDraftUsesExpectedRevision(t *testing.T) {
	store := newMemoryStore()
	service := newServiceForTest(store, validatorStub{})
	actor := principal("editor-1", domain.RoleEditor)
	draft, err := service.CreateDraft(context.Background(), actor, validSnapshot())
	if err != nil {
		t.Fatalf("create draft: %v", err)
	}

	_, err = service.UpdateDraft(context.Background(), actor, draft.ID, draft.Revision+1, validSnapshot())
	if !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("expected conflict, got %v", err)
	}
}

func TestReviewRequiresReviewerAndValidTransition(t *testing.T) {
	store := newMemoryStore()
	service := newServiceForTest(store, validatorStub{})
	ctx := context.Background()
	editor := principal("editor-1", domain.RoleEditor)
	reviewer := principal("reviewer-1", domain.RoleReviewer)
	draft, err := service.CreateDraft(ctx, editor, validSnapshot())
	if err != nil {
		t.Fatalf("create draft: %v", err)
	}
	submitted, err := service.SubmitReview(ctx, editor, draft.ID, draft.Revision)
	if err != nil {
		t.Fatalf("submit review: %v", err)
	}

	if _, err := service.ApproveReview(ctx, editor, submitted.ID, submitted.Revision); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("editor must not approve: %v", err)
	}

	approved, err := service.ApproveReview(ctx, reviewer, submitted.ID, submitted.Revision)
	if err != nil {
		t.Fatalf("approve review: %v", err)
	}
	if approved.Status != domain.StatusInReview || !approved.ReviewApproved {
		t.Fatalf("unexpected approved version %#v", approved)
	}

	rejected, err := service.RejectReview(ctx, reviewer, approved.ID, approved.Revision, "needs changes")
	if err != nil {
		t.Fatalf("reject review: %v", err)
	}
	if rejected.Status != domain.StatusDraft || rejected.ReviewApproved {
		t.Fatalf("unexpected rejected version %#v", rejected)
	}
}

func TestRejectReviewEnforcesReasonContract(t *testing.T) {
	store := newMemoryStore()
	service := newServiceForTest(store, validatorStub{})
	ctx := context.Background()
	editor := principal("editor-1", domain.RoleEditor)
	reviewer := principal("reviewer-1", domain.RoleReviewer)
	draft, err := service.CreateDraft(ctx, editor, validSnapshot())
	if err != nil {
		t.Fatalf("create draft: %v", err)
	}
	submitted, err := service.SubmitReview(ctx, editor, draft.ID, draft.Revision)
	if err != nil {
		t.Fatalf("submit review: %v", err)
	}

	for _, reason := range []string{"   ", strings.Repeat("x", 1001)} {
		if _, err := service.RejectReview(ctx, reviewer, submitted.ID, submitted.Revision, reason); !errors.Is(err, domain.ErrInvalidInput) {
			t.Fatalf("expected invalid reason length %d, got %v", len(reason), err)
		}
	}
}

func TestRejectReviewCannotMutatePublishingVersion(t *testing.T) {
	store := newMemoryStore()
	version := domain.ContentVersion{
		ID: "ver_publishing", Status: domain.StatusPublishing, Revision: 4,
		Snapshot: validSnapshot(), ReviewApproved: true,
	}
	store.versions[version.ID] = version
	service := newServiceForTest(store, validatorStub{})

	_, err := service.RejectReview(
		context.Background(),
		principal("reviewer-1", domain.RoleReviewer),
		version.ID,
		version.Revision,
		"late review change",
	)
	if !errors.Is(err, domain.ErrInvalidTransition) {
		t.Fatalf("expected publishing version to be frozen, got %v", err)
	}
	if stored := store.versions[version.ID]; stored.Status != domain.StatusPublishing || stored.Revision != version.Revision {
		t.Fatalf("publishing version was mutated: %#v", stored)
	}
}

func TestDraftMutationRejectsWrongRole(t *testing.T) {
	service := newServiceForTest(newMemoryStore(), validatorStub{})

	_, err := service.CreateDraft(
		context.Background(),
		principal("publisher-1", domain.RolePublisher),
		validSnapshot(),
	)
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected forbidden, got %v", err)
	}
}

func TestGetVersionReturnsStoredVersionForAuthenticatedEditor(t *testing.T) {
	store := newMemoryStore()
	service := newServiceForTest(store, validatorStub{})
	actor := principal("editor-1", domain.RoleEditor)
	created, err := service.CreateDraft(context.Background(), actor, validSnapshot())
	if err != nil {
		t.Fatalf("create draft: %v", err)
	}

	got, err := service.GetVersion(context.Background(), actor, created.ID)
	if err != nil {
		t.Fatalf("get version: %v", err)
	}
	if got.ID != created.ID || got.Revision != created.Revision {
		t.Fatalf("unexpected version %#v", got)
	}
}
