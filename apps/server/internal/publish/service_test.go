package publish

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"yujian.me/server/internal/domain"
	"yujian.me/server/internal/ports"
)

type memoryRepository struct {
	versions            map[string]domain.ContentVersion
	jobs                map[string]domain.PublishJob
	idempotency         map[string]string
	pointers            map[string]domain.PublishPointer
	audits              []domain.AuditEntry
	successfulByVersion map[string]string
	updateErr           error
	updateFailures      int
}

func newMemoryRepository() *memoryRepository {
	return &memoryRepository{
		versions:            make(map[string]domain.ContentVersion),
		jobs:                make(map[string]domain.PublishJob),
		idempotency:         make(map[string]string),
		pointers:            make(map[string]domain.PublishPointer),
		successfulByVersion: make(map[string]string),
	}
}

func (repository *memoryRepository) WithinTransaction(ctx context.Context, run func(Repository) error) error {
	return run(repository)
}

func (repository *memoryRepository) GetVersion(_ context.Context, id string) (domain.ContentVersion, error) {
	version, exists := repository.versions[id]
	if !exists {
		return domain.ContentVersion{}, domain.ErrNotFound
	}
	return version, nil
}

func (repository *memoryRepository) UpdateVersion(_ context.Context, version domain.ContentVersion, expectedRevision int64) error {
	current, exists := repository.versions[version.ID]
	if !exists {
		return domain.ErrNotFound
	}
	if current.Revision != expectedRevision {
		return domain.ErrConflict
	}
	repository.versions[version.ID] = version
	return nil
}

func (repository *memoryRepository) CreatePublishJob(_ context.Context, job domain.PublishJob) error {
	if _, exists := repository.idempotency[job.IdempotencyKey]; exists {
		return domain.ErrConflict
	}
	repository.jobs[job.ID] = job
	repository.idempotency[job.IdempotencyKey] = job.ID
	return nil
}

func (repository *memoryRepository) GetPublishJob(_ context.Context, id string) (domain.PublishJob, error) {
	job, exists := repository.jobs[id]
	if !exists {
		return domain.PublishJob{}, domain.ErrNotFound
	}
	return job, nil
}

func (repository *memoryRepository) GetPublishJobByIdempotencyKey(_ context.Context, key string) (domain.PublishJob, error) {
	id, exists := repository.idempotency[key]
	if !exists {
		return domain.PublishJob{}, domain.ErrNotFound
	}
	return repository.jobs[id], nil
}

func (repository *memoryRepository) GetSuccessfulPublishByVersion(_ context.Context, versionID string) (domain.PublishJob, error) {
	id, exists := repository.successfulByVersion[versionID]
	if !exists {
		return domain.PublishJob{}, domain.ErrNotFound
	}
	return repository.jobs[id], nil
}

func (repository *memoryRepository) UpdatePublishJob(_ context.Context, job domain.PublishJob) error {
	if repository.updateFailures > 0 {
		repository.updateFailures--
		return repository.updateErr
	}
	if _, exists := repository.jobs[job.ID]; !exists {
		return domain.ErrNotFound
	}
	repository.jobs[job.ID] = job
	if job.Status == domain.PublishSucceeded {
		repository.successfulByVersion[job.VersionID] = job.ID
	}
	return nil
}

func (repository *memoryRepository) GetPublishPointer(_ context.Context, slot string) (domain.PublishPointer, error) {
	pointer, exists := repository.pointers[slot]
	if !exists {
		return domain.PublishPointer{}, domain.ErrNotFound
	}
	return pointer, nil
}

func (repository *memoryRepository) SetPublishPointer(_ context.Context, pointer domain.PublishPointer) error {
	repository.pointers[pointer.Slot] = pointer
	return nil
}

func (repository *memoryRepository) AppendAudit(_ context.Context, entry domain.AuditEntry) error {
	repository.audits = append(repository.audits, entry)
	return nil
}

type blobStoreFake struct {
	objects  map[string]ports.BlobMetadata
	putCalls int
}

func newBlobStoreFake() *blobStoreFake {
	return &blobStoreFake{objects: make(map[string]ports.BlobMetadata)}
}

func (store *blobStoreFake) CreateUpload(context.Context, ports.UploadRequest) (ports.SignedUpload, error) {
	panic("not used")
}

func (store *blobStoreFake) Stat(_ context.Context, key string) (ports.BlobMetadata, error) {
	metadata, exists := store.objects[key]
	if !exists {
		return ports.BlobMetadata{}, domain.ErrNotFound
	}
	return metadata, nil
}

func (store *blobStoreFake) Put(_ context.Context, key string, reader io.Reader, metadata ports.BlobMetadata) error {
	if _, err := io.ReadAll(reader); err != nil {
		return err
	}
	store.putCalls++
	store.objects[key] = metadata
	return nil
}

func (store *blobStoreFake) Delete(context.Context, string) error {
	panic("not used")
}

func (store *blobStoreFake) SignedReadURL(context.Context, string, time.Duration) (string, error) {
	panic("not used")
}

type buildTriggerFake struct {
	triggerCalls int
	triggerErr   error
	statuses     map[string]ports.BuildRun
	requests     []ports.BuildRequest
}

func (trigger *buildTriggerFake) Trigger(_ context.Context, request ports.BuildRequest) (ports.BuildRun, error) {
	trigger.triggerCalls++
	trigger.requests = append(trigger.requests, request)
	if trigger.triggerErr != nil {
		return ports.BuildRun{}, trigger.triggerErr
	}
	return ports.BuildRun{ID: "build-1", Status: domain.PublishBuilding}, nil
}

func (trigger *buildTriggerFake) Status(_ context.Context, id string) (ports.BuildRun, error) {
	status, exists := trigger.statuses[id]
	if !exists {
		return ports.BuildRun{}, domain.ErrNotFound
	}
	return status, nil
}

type validatorStub struct {
	err error
}

func (validator validatorStub) Validate([]byte) error {
	return validator.err
}

func publishServiceForTest(
	repository *memoryRepository,
	blobs *blobStoreFake,
	trigger *buildTriggerFake,
) *Service {
	ids := []string{"pub_1", "pub_2", "pub_3"}
	return NewService(ServiceOptions{
		Repository:   repository,
		BlobStore:    blobs,
		BuildTrigger: trigger,
		Validator:    validatorStub{},
		Now: func() time.Time {
			return time.Date(2026, 8, 29, 10, 0, 0, 0, time.UTC)
		},
		NewID: func(string) string {
			id := ids[0]
			ids = ids[1:]
			return id
		},
	})
}

func publisher() domain.Principal {
	return domain.Principal{Subject: "publisher-1", Roles: []domain.Role{domain.RolePublisher}}
}

func approvedVersion(id, releaseID string) domain.ContentVersion {
	return domain.ContentVersion{
		ID:             id,
		Status:         domain.StatusInReview,
		Revision:       3,
		Snapshot:       []byte(`{"schemaVersion":"1.0.0","releaseId":"` + releaseID + `"}`),
		ReviewApproved: true,
	}
}

func TestPublishIsIdempotentAndWritesImmutableSnapshotOnce(t *testing.T) {
	repository := newMemoryRepository()
	repository.versions["ver_new"] = approvedVersion("ver_new", "rel_test")
	blobs := newBlobStoreFake()
	trigger := &buildTriggerFake{statuses: make(map[string]ports.BuildRun)}
	service := publishServiceForTest(repository, blobs, trigger)

	first, err := service.Publish(context.Background(), publisher(), "ver_new", "idem-1")
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	second, err := service.Publish(context.Background(), publisher(), "ver_new", "idem-1")
	if err != nil {
		t.Fatalf("repeat publish: %v", err)
	}

	if first.ID != second.ID {
		t.Fatalf("expected same job, got %q and %q", first.ID, second.ID)
	}
	if blobs.putCalls != 1 || trigger.triggerCalls != 1 {
		t.Fatalf("unexpected calls put=%d trigger=%d", blobs.putCalls, trigger.triggerCalls)
	}
	if first.SnapshotKey == "" || first.SnapshotChecksum == "" {
		t.Fatalf("missing immutable snapshot data %#v", first)
	}
}

func TestPublishRejectsIdempotencyKeyReusedForDifferentRequest(t *testing.T) {
	repository := newMemoryRepository()
	repository.versions["ver_one"] = approvedVersion("ver_one", "rel_one")
	repository.versions["ver_two"] = approvedVersion("ver_two", "rel_two")
	blobs := newBlobStoreFake()
	trigger := &buildTriggerFake{statuses: make(map[string]ports.BuildRun)}
	service := publishServiceForTest(repository, blobs, trigger)

	if _, err := service.Publish(context.Background(), publisher(), "ver_one", "idem-shared"); err != nil {
		t.Fatalf("first publish: %v", err)
	}
	if _, err := service.Publish(context.Background(), publisher(), "ver_two", "idem-shared"); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("expected version conflict, got %v", err)
	}
	other := domain.Principal{Subject: "publisher-2", Roles: []domain.Role{domain.RolePublisher}}
	if _, err := service.Publish(context.Background(), other, "ver_one", "idem-shared"); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("expected actor conflict, got %v", err)
	}
	if _, err := service.Rollback(context.Background(), publisher(), "ver_one", "idem-shared"); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("expected operation conflict, got %v", err)
	}
	if trigger.triggerCalls != 1 {
		t.Fatalf("mismatched retries triggered %d builds", trigger.triggerCalls)
	}
}

func TestPublishRetriesPendingTriggerWithProviderIdempotencyKey(t *testing.T) {
	repository := newMemoryRepository()
	repository.versions["ver_new"] = approvedVersion("ver_new", "rel_retry")
	repository.updateErr = errors.New("database write unavailable")
	repository.updateFailures = 1
	trigger := &buildTriggerFake{statuses: make(map[string]ports.BuildRun)}
	service := publishServiceForTest(repository, newBlobStoreFake(), trigger)

	if _, err := service.Publish(context.Background(), publisher(), "ver_new", "idem-retry"); err == nil {
		t.Fatal("expected first publish to report the state write failure")
	}
	retried, err := service.Publish(context.Background(), publisher(), "ver_new", "idem-retry")
	if err != nil {
		t.Fatalf("retry publish: %v", err)
	}
	if retried.Status != domain.PublishBuilding || retried.BuildID != "build-1" {
		t.Fatalf("pending publish was not resumed: %#v", retried)
	}
	if trigger.triggerCalls != 2 {
		t.Fatalf("expected two provider calls with one logical operation, got %d", trigger.triggerCalls)
	}
	for _, request := range trigger.requests {
		if request.IdempotencyKey != "idem-retry" {
			t.Fatalf("provider request lost idempotency key: %#v", request)
		}
	}
}

func TestAmbiguousTriggerFailureStaysRetryableAndKeepsCurrentPointer(t *testing.T) {
	repository := newMemoryRepository()
	repository.versions["ver_new"] = approvedVersion("ver_new", "rel_test")
	repository.pointers["production"] = domain.PublishPointer{Slot: "production", VersionID: "ver_old"}
	blobs := newBlobStoreFake()
	trigger := &buildTriggerFake{
		triggerErr: errors.New("build unavailable"),
		statuses:   make(map[string]ports.BuildRun),
	}
	service := publishServiceForTest(repository, blobs, trigger)

	job, err := service.Publish(context.Background(), publisher(), "ver_new", "idem-fail")
	if err == nil || job.Status != domain.PublishPending {
		t.Fatalf("expected retryable pending job and error, job=%#v err=%v", job, err)
	}
	if repository.pointers["production"].VersionID != "ver_old" {
		t.Fatal("failed build changed production pointer")
	}
	trigger.triggerErr = nil
	retried, err := service.Publish(context.Background(), publisher(), "ver_new", "idem-fail")
	if err != nil || retried.Status != domain.PublishBuilding || retried.ErrorMessage != "" {
		t.Fatalf("retry trigger: job=%#v err=%v", retried, err)
	}
}

func TestRefreshSuccessAtomicallySwitchesPointerAndArchivesOldVersion(t *testing.T) {
	repository := newMemoryRepository()
	old := approvedVersion("ver_old", "rel_old")
	old.Status = domain.StatusPublished
	newVersion := approvedVersion("ver_new", "rel_new")
	repository.versions[old.ID] = old
	repository.versions[newVersion.ID] = newVersion
	repository.pointers["production"] = domain.PublishPointer{Slot: "production", VersionID: old.ID}
	blobs := newBlobStoreFake()
	trigger := &buildTriggerFake{statuses: make(map[string]ports.BuildRun)}
	service := publishServiceForTest(repository, blobs, trigger)
	job, err := service.Publish(context.Background(), publisher(), newVersion.ID, "idem-success")
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	trigger.statuses[job.BuildID] = ports.BuildRun{ID: job.BuildID, Status: domain.PublishSucceeded}

	completed, err := service.RefreshStatus(context.Background(), publisher(), job.ID)
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if completed.Status != domain.PublishSucceeded {
		t.Fatalf("unexpected job status %q", completed.Status)
	}
	if repository.pointers["production"].VersionID != newVersion.ID {
		t.Fatal("production pointer did not switch")
	}
	if repository.versions[old.ID].Status != domain.StatusArchived {
		t.Fatal("old version was not archived")
	}
	if repository.versions[newVersion.ID].Status != domain.StatusPublished {
		t.Fatal("new version was not published")
	}
}

func TestRollbackReusesHistoricalSnapshotWithoutBlobRewrite(t *testing.T) {
	repository := newMemoryRepository()
	target := approvedVersion("ver_old", "rel_old")
	target.Status = domain.StatusArchived
	repository.versions[target.ID] = target
	historical := domain.PublishJob{
		ID:               "pub_history",
		VersionID:        target.ID,
		SnapshotKey:      "snapshots/rel_old/sha256-old.json",
		SnapshotChecksum: "sha256:old",
		Status:           domain.PublishSucceeded,
	}
	repository.jobs[historical.ID] = historical
	repository.successfulByVersion[target.ID] = historical.ID
	blobs := newBlobStoreFake()
	trigger := &buildTriggerFake{statuses: make(map[string]ports.BuildRun)}
	service := publishServiceForTest(repository, blobs, trigger)

	job, err := service.Rollback(context.Background(), publisher(), target.ID, "idem-rollback")
	if err != nil {
		t.Fatalf("rollback: %v", err)
	}
	if job.SnapshotKey != historical.SnapshotKey || blobs.putCalls != 0 {
		t.Fatalf("rollback rewrote snapshot job=%#v puts=%d", job, blobs.putCalls)
	}
	if trigger.triggerCalls != 1 {
		t.Fatalf("expected one build trigger, got %d", trigger.triggerCalls)
	}
}

func TestRollbackRevalidatesHistoricalSnapshot(t *testing.T) {
	repository := newMemoryRepository()
	target := approvedVersion("ver_old", "rel_old")
	target.Status = domain.StatusArchived
	repository.versions[target.ID] = target
	historical := domain.PublishJob{
		ID:               "pub_history",
		VersionID:        target.ID,
		SnapshotKey:      "snapshots/rel_old/sha256-old.json",
		SnapshotChecksum: "sha256:old",
		Status:           domain.PublishSucceeded,
	}
	repository.jobs[historical.ID] = historical
	repository.successfulByVersion[target.ID] = historical.ID
	blobs := newBlobStoreFake()
	trigger := &buildTriggerFake{statuses: make(map[string]ports.BuildRun)}
	service := NewService(ServiceOptions{
		Repository:   repository,
		BlobStore:    blobs,
		BuildTrigger: trigger,
		Validator:    validatorStub{err: errors.New("invalid historical snapshot")},
	})

	_, err := service.Rollback(context.Background(), publisher(), target.ID, "idem-rollback-invalid")
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("expected invalid input, got %v", err)
	}
	if trigger.triggerCalls != 0 {
		t.Fatal("invalid historical snapshot must not trigger a build")
	}
}

func TestCanonicalSnapshotIgnoresObjectKeyOrder(t *testing.T) {
	first, err := canonicalSnapshot([]byte(`{"releaseId":"rel_test","schemaVersion":"1.0.0"}`))
	if err != nil {
		t.Fatalf("canonicalize first: %v", err)
	}
	second, err := canonicalSnapshot([]byte(`{"schemaVersion":"1.0.0","releaseId":"rel_test"}`))
	if err != nil {
		t.Fatalf("canonicalize second: %v", err)
	}
	if !bytes.Equal(first.JSON, second.JSON) || first.Checksum != second.Checksum {
		t.Fatalf("canonical snapshots differ: %#v %#v", first, second)
	}
}

func TestGetPublishJobReturnsJobForPublisher(t *testing.T) {
	repository := newMemoryRepository()
	repository.jobs["pub_existing"] = domain.PublishJob{ID: "pub_existing", Status: domain.PublishBuilding}
	blobs := newBlobStoreFake()
	trigger := &buildTriggerFake{statuses: make(map[string]ports.BuildRun)}
	service := publishServiceForTest(repository, blobs, trigger)

	job, err := service.GetPublishJob(context.Background(), publisher(), "pub_existing")
	if err != nil {
		t.Fatalf("get publish job: %v", err)
	}
	if job.ID != "pub_existing" {
		t.Fatalf("unexpected job %#v", job)
	}
}
