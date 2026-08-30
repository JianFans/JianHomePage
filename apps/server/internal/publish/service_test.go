package publish

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"yujian.me/server/internal/domain"
	"yujian.me/server/internal/ports"
	snapshotdata "yujian.me/server/internal/snapshot"
)

type memoryRepository struct {
	versions            map[string]domain.ContentVersion
	assets              map[string]domain.AssetRecord
	jobs                map[string]domain.PublishJob
	idempotency         map[string]string
	pointers            map[string]domain.PublishPointer
	audits              []domain.AuditEntry
	successfulByVersion map[string]string
	updateErr           error
	updateFailures      int
	beforeUpdate        func(*memoryRepository, domain.PublishJob)
	lockCalls           int
	lockHook            func(*memoryRepository)
}

func newMemoryRepository() *memoryRepository {
	return &memoryRepository{
		versions:            make(map[string]domain.ContentVersion),
		assets:              make(map[string]domain.AssetRecord),
		jobs:                make(map[string]domain.PublishJob),
		idempotency:         make(map[string]string),
		pointers:            make(map[string]domain.PublishPointer),
		successfulByVersion: make(map[string]string),
	}
}

func (repository *memoryRepository) GetAsset(_ context.Context, id string) (domain.AssetRecord, error) {
	asset, exists := repository.assets[id]
	if !exists {
		return domain.AssetRecord{}, domain.ErrNotFound
	}
	return asset, nil
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

func (repository *memoryRepository) GetActivePublishJob(context.Context) (domain.PublishJob, error) {
	for _, job := range repository.jobs {
		if job.Status == domain.PublishPending || job.Status == domain.PublishBuilding {
			return job, nil
		}
	}
	return domain.PublishJob{}, domain.ErrNotFound
}

func (repository *memoryRepository) GetSuccessfulPublishByVersion(_ context.Context, versionID string) (domain.PublishJob, error) {
	id, exists := repository.successfulByVersion[versionID]
	if !exists {
		return domain.PublishJob{}, domain.ErrNotFound
	}
	return repository.jobs[id], nil
}

func (repository *memoryRepository) UpdatePublishJob(_ context.Context, job domain.PublishJob, expectedStatus domain.PublishStatus) error {
	if repository.updateFailures > 0 {
		repository.updateFailures--
		return repository.updateErr
	}
	if _, exists := repository.jobs[job.ID]; !exists {
		return domain.ErrNotFound
	}
	if repository.beforeUpdate != nil {
		repository.beforeUpdate(repository, job)
		repository.beforeUpdate = nil
	}
	if repository.jobs[job.ID].Status != expectedStatus {
		return domain.ErrConflict
	}
	repository.jobs[job.ID] = job
	if job.Status == domain.PublishSucceeded {
		repository.successfulByVersion[job.VersionID] = job.ID
	}
	return nil
}

func TestLateTriggerFailureDoesNotDowngradeConcurrentBuildingJob(t *testing.T) {
	repository := newMemoryRepository()
	job := domain.PublishJob{
		ID: "pub_1", IdempotencyKey: "idem-concurrent-trigger", Operation: domain.PublishOperationPublish,
		VersionID: "ver_new", ReleaseID: "rel_new", Status: domain.PublishPending,
	}
	repository.jobs[job.ID] = job
	repository.beforeUpdate = func(repository *memoryRepository, _ domain.PublishJob) {
		concurrent := repository.jobs[job.ID]
		concurrent.Status = domain.PublishBuilding
		concurrent.BuildID = "build-concurrent"
		repository.jobs[job.ID] = concurrent
	}
	trigger := &buildTriggerFake{triggerErr: errors.New("late provider timeout"), statuses: make(map[string]ports.BuildRun)}
	service := publishServiceForTest(repository, newBlobStoreFake(), trigger)

	actual, err := service.trigger(context.Background(), job, job.ReleaseID)
	if err != nil {
		t.Fatalf("concurrent progress should win over late trigger failure: %v", err)
	}
	if actual.Status != domain.PublishBuilding || actual.BuildID != "build-concurrent" {
		t.Fatalf("late trigger result downgraded concurrent progress: %#v", actual)
	}
	if stored := repository.jobs[job.ID]; stored.Status != domain.PublishBuilding || stored.BuildID != "build-concurrent" {
		t.Fatalf("stored job was downgraded: %#v", stored)
	}
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

func (repository *memoryRepository) LockPublishSlot(context.Context, string) error {
	repository.lockCalls++
	if repository.lockHook != nil {
		repository.lockHook(repository)
		repository.lockHook = nil
	}
	return nil
}

func (repository *memoryRepository) AppendAudit(_ context.Context, entry domain.AuditEntry) error {
	repository.audits = append(repository.audits, entry)
	return nil
}

type blobStoreFake struct {
	objects     map[string]ports.BlobMetadata
	putCalls    int
	publicURL   string
	publicCalls int
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

func (store *blobStoreFake) PublicURL(_ context.Context, key string) (string, error) {
	store.publicCalls++
	if store.publicURL != "" {
		return store.publicURL, nil
	}
	return "/media/" + key, nil
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
	snapshot, err := snapshotdata.CanonicalJSON([]byte(`{"schemaVersion":"1.0.0","releaseId":"` + releaseID + `"}`))
	if err != nil {
		panic(err)
	}
	return domain.ContentVersion{
		ID:             id,
		Status:         domain.StatusInReview,
		Revision:       3,
		Snapshot:       snapshot,
		Checksum:       snapshotdata.Checksum(snapshot),
		ReviewApproved: true,
	}
}

func managedVersion(t *testing.T, status domain.ContentStatus) domain.ContentVersion {
	return managedVersionWithSource(t, status, "/media/assets/asset_managed/source.webp")
}

func managedVersionWithSource(t *testing.T, status domain.ContentStatus, source string) domain.ContentVersion {
	t.Helper()
	payloadChecksum := snapshotdata.Checksum([]byte("payload"))
	payload, err := json.Marshal(map[string]any{
		"schemaVersion": "1.0.0",
		"releaseId":     "rel_managed",
		"assets": []map[string]any{{
			"id": "asset_managed", "src": source, "mimeType": "image/webp",
			"byteSize": 7, "checksum": payloadChecksum, "rights": map[string]any{
				"source": map[string]string{"zh-CN": "authorized"},
			},
		}},
	})
	if err != nil {
		t.Fatalf("encode managed snapshot: %v", err)
	}
	snapshot, err := snapshotdata.CanonicalJSON(payload)
	if err != nil {
		t.Fatalf("canonicalize managed snapshot: %v", err)
	}
	return domain.ContentVersion{
		ID: "ver_managed", Status: status, Revision: 3,
		Snapshot: snapshot, Checksum: snapshotdata.Checksum(snapshot), ReviewApproved: true,
	}
}

func managedRights() json.RawMessage {
	return json.RawMessage(`{"source":{"zh-CN":"authorized"}}`)
}

func TestPublishRejectsBrowserNormalizedManagedAssetURLBypass(t *testing.T) {
	for _, source := range []string{
		`https://media.example.com/assets\asset_managed\source.webp`,
		`https://media.example.com/assets%5Casset_managed%5Csource.webp`,
		`https://media.example.com/assets/asset_managed/ignored/../source.webp`,
		`https://media.example.com/assets/asset_managed/ignored/%2e%2e/source.webp`,
		`https://media.example.com/assets/asset_managed/source.webp `,
		`https://media.example.com/assets/asset_managed/source.webp  `,
		`https:///assets/asset_managed/source.webp`,
		`https://media.example.com/assets%2Fasset_managed%2Fsource.webp`,
		`https://media.example.com/assets/asset_managed/source%2Ewebp`,
	} {
		t.Run(source, func(t *testing.T) {
			repository := newMemoryRepository()
			target := managedVersionWithSource(t, domain.StatusInReview, source)
			repository.versions[target.ID] = target
			repository.assets["asset_managed"] = domain.AssetRecord{
				ID: "asset_managed", BlobKey: "assets/asset_managed/source.webp", Status: domain.AssetReady,
				Rights: managedRights(),
			}
			blobs := newBlobStoreFake()
			blobs.objects["assets/asset_managed/source.webp"] = ports.BlobMetadata{
				ContentType: "image/webp", Size: 7, Checksum: snapshotdata.Checksum([]byte("payload")),
			}
			trigger := &buildTriggerFake{statuses: make(map[string]ports.BuildRun)}
			service := publishServiceForTest(repository, blobs, trigger)

			_, err := service.Publish(context.Background(), publisher(), target.ID, "idem-backslash-bypass")
			if !errors.Is(err, domain.ErrInvalidInput) {
				t.Fatalf("expected ambiguous managed URL rejection, got %v", err)
			}
			if trigger.triggerCalls != 0 {
				t.Fatalf("ambiguous managed URL triggered %d builds", trigger.triggerCalls)
			}
		})
	}
}

func TestPublishRejectsManagedAssetURLOutsideConfiguredProvider(t *testing.T) {
	for _, source := range []string{
		"https://attacker.example/assets/asset_managed/source.webp",
		"https://attacker.example/other/source.webp",
	} {
		t.Run(source, func(t *testing.T) {
			repository := newMemoryRepository()
			target := managedVersionWithSource(t, domain.StatusInReview, source)
			repository.versions[target.ID] = target
			repository.assets["asset_managed"] = domain.AssetRecord{
				ID: "asset_managed", BlobKey: "assets/asset_managed/source.webp", Status: domain.AssetReady,
				Rights: managedRights(),
			}
			blobs := newBlobStoreFake()
			blobs.objects["assets/asset_managed/source.webp"] = ports.BlobMetadata{
				ContentType: "image/webp", Size: 7, Checksum: snapshotdata.Checksum([]byte("payload")),
			}
			trigger := &buildTriggerFake{statuses: make(map[string]ports.BuildRun)}
			service := publishServiceForTest(repository, blobs, trigger)

			_, err := service.Publish(context.Background(), publisher(), target.ID, "idem-managed-provider")
			if !errors.Is(err, domain.ErrInvalidInput) {
				t.Fatalf("expected provider URL mismatch rejection, got %v", err)
			}
			if trigger.triggerCalls != 0 {
				t.Fatalf("provider URL mismatch triggered %d builds", trigger.triggerCalls)
			}
		})
	}
}

func TestPublishUsesPersistedStableAssetURL(t *testing.T) {
	const sourceURL = "https://media.yujian.me/assets/asset_managed/source.webp"
	repository := newMemoryRepository()
	target := managedVersionWithSource(t, domain.StatusInReview, sourceURL)
	repository.versions[target.ID] = target
	repository.assets["asset_managed"] = domain.AssetRecord{
		ID: "asset_managed", BlobKey: "assets/asset_managed/source.webp", SourceURL: sourceURL,
		Status: domain.AssetReady, Rights: managedRights(),
	}
	blobs := newBlobStoreFake()
	blobs.publicURL = "https://new-provider.example/assets/asset_managed/source.webp"
	blobs.objects["assets/asset_managed/source.webp"] = ports.BlobMetadata{
		ContentType: "image/webp", Size: 7, Checksum: snapshotdata.Checksum([]byte("payload")),
	}
	trigger := &buildTriggerFake{statuses: make(map[string]ports.BuildRun)}
	service := publishServiceForTest(repository, blobs, trigger)

	if _, err := service.Publish(t.Context(), publisher(), target.ID, "idem-stable-source-url"); err != nil {
		t.Fatalf("publish persisted stable asset URL: %v", err)
	}
	if blobs.publicCalls != 0 {
		t.Fatalf("publish recalculated stable source URL %d times", blobs.publicCalls)
	}
}

func TestPublishRejectsManagedAssetRightsMismatch(t *testing.T) {
	repository := newMemoryRepository()
	target := managedVersion(t, domain.StatusInReview)
	repository.versions[target.ID] = target
	repository.assets["asset_managed"] = domain.AssetRecord{
		ID: "asset_managed", BlobKey: "assets/asset_managed/source.webp", Status: domain.AssetReady,
		Rights: json.RawMessage(`{"source":{"zh-CN":"different"}}`),
	}
	blobs := newBlobStoreFake()
	blobs.objects["assets/asset_managed/source.webp"] = ports.BlobMetadata{
		ContentType: "image/webp", Size: 7, Checksum: snapshotdata.Checksum([]byte("payload")),
	}
	trigger := &buildTriggerFake{statuses: make(map[string]ports.BuildRun)}
	service := publishServiceForTest(repository, blobs, trigger)

	_, err := service.Publish(context.Background(), publisher(), target.ID, "idem-managed-rights")
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("expected rights mismatch rejection, got %v", err)
	}
	if trigger.triggerCalls != 0 {
		t.Fatalf("rights mismatch triggered %d builds", trigger.triggerCalls)
	}
}

func TestPublishFreezesApprovedVersionBeforeTrigger(t *testing.T) {
	repository := newMemoryRepository()
	target := approvedVersion("ver_new", "rel_new")
	repository.versions[target.ID] = target
	service := publishServiceForTest(repository, newBlobStoreFake(), &buildTriggerFake{statuses: make(map[string]ports.BuildRun)})

	job, err := service.Publish(context.Background(), publisher(), target.ID, "idem-freeze-target")
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	frozen := repository.versions[target.ID]
	if frozen.Status != domain.ContentStatus("publishing") || frozen.Revision != target.Revision+1 {
		t.Fatalf("publish target was not frozen: %#v", frozen)
	}
	if job.TargetRevision != frozen.Revision {
		t.Fatalf("job target revision %d does not match frozen revision %d", job.TargetRevision, frozen.Revision)
	}
}

func TestPublishValidatesManagedAssetLifecycleAndBlobMetadata(t *testing.T) {
	payloadChecksum := snapshotdata.Checksum([]byte("payload"))
	tests := []struct {
		name       string
		asset      *domain.AssetRecord
		metadata   ports.BlobMetadata
		shouldPass bool
	}{
		{
			name:       "ready matching asset",
			asset:      &domain.AssetRecord{ID: "asset_managed", BlobKey: "assets/asset_managed/source.webp", Status: domain.AssetReady},
			metadata:   ports.BlobMetadata{ContentType: "image/webp", Size: 7, Checksum: payloadChecksum},
			shouldPass: true,
		},
		{
			name:     "pending asset",
			asset:    &domain.AssetRecord{ID: "asset_managed", BlobKey: "assets/asset_managed/source.webp", Status: domain.AssetPending},
			metadata: ports.BlobMetadata{ContentType: "image/webp", Size: 7, Checksum: payloadChecksum},
		},
		{
			name:     "deleted asset",
			asset:    &domain.AssetRecord{ID: "asset_managed", BlobKey: "assets/asset_managed/source.webp", Status: domain.AssetDeleted},
			metadata: ports.BlobMetadata{ContentType: "image/webp", Size: 7, Checksum: payloadChecksum},
		},
		{
			name:     "missing asset record",
			metadata: ports.BlobMetadata{ContentType: "image/webp", Size: 7, Checksum: payloadChecksum},
		},
		{
			name:     "blob key mismatch",
			asset:    &domain.AssetRecord{ID: "asset_managed", BlobKey: "assets/asset_other/source.webp", Status: domain.AssetReady},
			metadata: ports.BlobMetadata{ContentType: "image/webp", Size: 7, Checksum: payloadChecksum},
		},
		{
			name:     "blob MIME mismatch",
			asset:    &domain.AssetRecord{ID: "asset_managed", BlobKey: "assets/asset_managed/source.webp", Status: domain.AssetReady},
			metadata: ports.BlobMetadata{ContentType: "image/gif", Size: 7, Checksum: payloadChecksum},
		},
		{
			name:     "blob size mismatch",
			asset:    &domain.AssetRecord{ID: "asset_managed", BlobKey: "assets/asset_managed/source.webp", Status: domain.AssetReady},
			metadata: ports.BlobMetadata{ContentType: "image/webp", Size: 8, Checksum: payloadChecksum},
		},
		{
			name:     "blob checksum mismatch",
			asset:    &domain.AssetRecord{ID: "asset_managed", BlobKey: "assets/asset_managed/source.webp", Status: domain.AssetReady},
			metadata: ports.BlobMetadata{ContentType: "image/webp", Size: 7, Checksum: "sha256:mismatch"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repository := newMemoryRepository()
			target := managedVersion(t, domain.StatusInReview)
			repository.versions[target.ID] = target
			if test.asset != nil {
				asset := *test.asset
				asset.Rights = managedRights()
				repository.assets[asset.ID] = asset
			}
			blobs := newBlobStoreFake()
			blobs.objects["assets/asset_managed/source.webp"] = test.metadata
			trigger := &buildTriggerFake{statuses: make(map[string]ports.BuildRun)}
			service := publishServiceForTest(repository, blobs, trigger)

			_, err := service.Publish(context.Background(), publisher(), target.ID, "idem-managed-asset")
			if test.shouldPass {
				if err != nil {
					t.Fatalf("publish ready managed asset: %v", err)
				}
				return
			}
			if !errors.Is(err, domain.ErrInvalidInput) {
				t.Fatalf("expected invalid managed asset, got %v", err)
			}
			if trigger.triggerCalls != 0 {
				t.Fatalf("invalid managed asset triggered %d builds", trigger.triggerCalls)
			}
		})
	}
}

func TestRollbackAllowsDeletedManagedAssetWhenRetainedBlobStillMatches(t *testing.T) {
	repository := newMemoryRepository()
	target := managedVersion(t, domain.StatusArchived)
	repository.versions[target.ID] = target
	payloadChecksum := snapshotdata.Checksum([]byte("payload"))
	repository.assets["asset_managed"] = domain.AssetRecord{
		ID: "asset_managed", BlobKey: "assets/asset_managed/source.webp", Status: domain.AssetDeleted,
		Rights: managedRights(),
	}
	historical := domain.PublishJob{
		ID: "pub_history", VersionID: target.ID, SnapshotKey: "snapshots/rel_managed/" + target.Checksum + ".json",
		SnapshotChecksum: target.Checksum, Status: domain.PublishSucceeded,
	}
	repository.jobs[historical.ID] = historical
	repository.successfulByVersion[target.ID] = historical.ID
	blobs := newBlobStoreFake()
	blobs.objects["assets/asset_managed/source.webp"] = ports.BlobMetadata{
		ContentType: "image/webp", Size: 7, Checksum: payloadChecksum,
	}
	trigger := &buildTriggerFake{statuses: make(map[string]ports.BuildRun)}
	service := publishServiceForTest(repository, blobs, trigger)

	if _, err := service.Rollback(context.Background(), publisher(), target.ID, "idem-deleted-managed-rollback"); err != nil {
		t.Fatalf("rollback retained managed asset: %v", err)
	}
	if trigger.triggerCalls != 1 {
		t.Fatalf("rollback did not trigger exactly one build: %d", trigger.triggerCalls)
	}
}

func TestRefreshFailsJobWhenFrozenTargetRevisionChanges(t *testing.T) {
	repository := newMemoryRepository()
	target := approvedVersion("ver_new", "rel_new")
	repository.versions[target.ID] = target
	trigger := &buildTriggerFake{statuses: make(map[string]ports.BuildRun)}
	service := publishServiceForTest(repository, newBlobStoreFake(), trigger)
	job, err := service.Publish(context.Background(), publisher(), target.ID, "idem-mutated-target")
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	mutated := repository.versions[target.ID]
	mutated.Revision++
	repository.versions[target.ID] = mutated
	trigger.statuses[job.BuildID] = ports.BuildRun{ID: job.BuildID, Status: domain.PublishSucceeded}

	completed, err := service.RefreshStatus(context.Background(), publisher(), job.ID)
	if err != nil {
		t.Fatalf("refresh should finalize the job as failed: %v", err)
	}
	if completed.Status != domain.PublishFailed || repository.jobs[job.ID].Status != domain.PublishFailed {
		t.Fatalf("mutated target did not fail publish job: %#v", completed)
	}
	if _, exists := repository.pointers[productionSlot]; exists {
		t.Fatal("mutated target changed the production pointer")
	}
	recovered := repository.versions[target.ID]
	if recovered.Status != domain.StatusDraft || recovered.ReviewApproved {
		t.Fatalf("mutated frozen target was not returned for review: %#v", recovered)
	}
}

func TestFailedBuildReleasesFrozenPublishTarget(t *testing.T) {
	repository := newMemoryRepository()
	target := approvedVersion("ver_new", "rel_new")
	repository.versions[target.ID] = target
	trigger := &buildTriggerFake{statuses: make(map[string]ports.BuildRun)}
	service := publishServiceForTest(repository, newBlobStoreFake(), trigger)
	job, err := service.Publish(context.Background(), publisher(), target.ID, "idem-release-target")
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	trigger.statuses[job.BuildID] = ports.BuildRun{ID: job.BuildID, Status: domain.PublishFailed, Error: "build failed"}

	completed, err := service.RefreshStatus(context.Background(), publisher(), job.ID)
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	released := repository.versions[target.ID]
	if completed.Status != domain.PublishFailed || released.Status != domain.StatusInReview || released.Revision != target.Revision+2 {
		t.Fatalf("failed build did not release frozen target: job=%#v version=%#v", completed, released)
	}
}

func TestPublishIsIdempotentAndWritesImmutableSnapshotOnce(t *testing.T) {
	repository := newMemoryRepository()
	repository.versions["ver_new"] = approvedVersion("ver_new", "rel_test")
	blobs := newBlobStoreFake()
	trigger := &buildTriggerFake{statuses: make(map[string]ports.BuildRun)}
	service := publishServiceForTest(repository, blobs, trigger)

	first, err := service.Publish(context.Background(), publisher(), "ver_new", "idem-0001")
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	second, err := service.Publish(context.Background(), publisher(), "ver_new", "idem-0001")
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

func TestPublishRejectsAnotherActiveProductionJob(t *testing.T) {
	repository := newMemoryRepository()
	repository.versions["ver_one"] = approvedVersion("ver_one", "rel_one")
	repository.versions["ver_two"] = approvedVersion("ver_two", "rel_two")
	trigger := &buildTriggerFake{statuses: make(map[string]ports.BuildRun)}
	service := publishServiceForTest(repository, newBlobStoreFake(), trigger)

	first, err := service.Publish(context.Background(), publisher(), "ver_one", "idem-active-one")
	if err != nil {
		t.Fatalf("first publish: %v", err)
	}
	if first.Status != domain.PublishBuilding {
		t.Fatalf("unexpected first job status %q", first.Status)
	}

	if _, err := service.Publish(context.Background(), publisher(), "ver_two", "idem-active-two"); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("expected active production job conflict, got %v", err)
	}
	if trigger.triggerCalls != 1 {
		t.Fatalf("second publish reached build trigger: %d calls", trigger.triggerCalls)
	}
}

func TestReconcileRecoversPendingJobAfterTriggerPersistenceFailure(t *testing.T) {
	repository := newMemoryRepository()
	repository.versions["ver_new"] = approvedVersion("ver_new", "rel_new")
	repository.updateFailures = 1
	repository.updateErr = errors.New("database unavailable")
	trigger := &buildTriggerFake{statuses: make(map[string]ports.BuildRun)}
	service := publishServiceForTest(repository, newBlobStoreFake(), trigger)

	if _, err := service.Publish(context.Background(), publisher(), "ver_new", "idem-reconcile-trigger"); err == nil {
		t.Fatal("expected initial publish persistence failure")
	}
	if trigger.triggerCalls != 1 {
		t.Fatalf("unexpected initial trigger calls %d", trigger.triggerCalls)
	}

	if err := service.Reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	job := repository.jobs["pub_1"]
	if job.Status != domain.PublishBuilding || job.BuildID != "build-1" {
		t.Fatalf("pending job was not recovered: %#v", job)
	}
	if trigger.triggerCalls != 2 || trigger.requests[0].IdempotencyKey != trigger.requests[1].IdempotencyKey {
		t.Fatalf("recovery did not reuse the provider idempotency key: %#v", trigger.requests)
	}
}

func TestReconcileFinalizesBuildingJobWithoutAdminRefresh(t *testing.T) {
	repository := newMemoryRepository()
	repository.versions["ver_new"] = approvedVersion("ver_new", "rel_new")
	trigger := &buildTriggerFake{statuses: make(map[string]ports.BuildRun)}
	service := publishServiceForTest(repository, newBlobStoreFake(), trigger)
	job, err := service.Publish(context.Background(), publisher(), "ver_new", "idem-reconcile-status")
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	trigger.statuses[job.BuildID] = ports.BuildRun{ID: job.BuildID, Status: domain.PublishSucceeded}

	if err := service.Reconcile(context.Background()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if repository.jobs[job.ID].Status != domain.PublishSucceeded || repository.pointers[productionSlot].VersionID != "ver_new" {
		t.Fatalf("building job was not finalized: job=%#v pointer=%#v", repository.jobs[job.ID], repository.pointers[productionSlot])
	}
}

func TestPublishRejectsIdempotencyKeyOutsideContractLength(t *testing.T) {
	repository := newMemoryRepository()
	repository.versions["ver_new"] = approvedVersion("ver_new", "rel_test")
	service := publishServiceForTest(repository, newBlobStoreFake(), &buildTriggerFake{statuses: make(map[string]ports.BuildRun)})

	for _, key := range []string{"1234567", strings.Repeat("x", 129)} {
		if _, err := service.Publish(context.Background(), publisher(), "ver_new", key); !errors.Is(err, domain.ErrInvalidInput) {
			t.Fatalf("expected invalid input for key length %d, got %v", len(key), err)
		}
	}
	if len(repository.jobs) != 0 {
		t.Fatalf("invalid keys created jobs %#v", repository.jobs)
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
	if repository.lockCalls != 2 {
		t.Fatalf("expected publish slot lock, got %d calls", repository.lockCalls)
	}
}

func TestRefreshSuccessRechecksJobAfterWaitingForPublishSlotLock(t *testing.T) {
	repository := newMemoryRepository()
	target := approvedVersion("ver_new", "rel_new")
	repository.versions[target.ID] = target
	trigger := &buildTriggerFake{statuses: make(map[string]ports.BuildRun)}
	service := publishServiceForTest(repository, newBlobStoreFake(), trigger)
	job, err := service.Publish(context.Background(), publisher(), target.ID, "idem-concurrent-refresh")
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	trigger.statuses[job.BuildID] = ports.BuildRun{ID: job.BuildID, Status: domain.PublishSucceeded}
	repository.lockHook = func(repository *memoryRepository) {
		persisted := repository.jobs[job.ID]
		persisted.Status = domain.PublishSucceeded
		repository.jobs[job.ID] = persisted
		published := repository.versions[target.ID]
		published.Status = domain.StatusPublished
		published.Revision++
		repository.versions[target.ID] = published
		repository.pointers[productionSlot] = domain.PublishPointer{Slot: productionSlot, VersionID: target.ID}
	}
	expectedRevision := target.Revision + 2
	expectedAudits := len(repository.audits)

	completed, err := service.RefreshStatus(context.Background(), publisher(), job.ID)
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if completed.Status != domain.PublishSucceeded {
		t.Fatalf("unexpected job status %q", completed.Status)
	}
	if repository.versions[target.ID].Revision != expectedRevision {
		t.Fatalf("completed publish was applied twice: %#v", repository.versions[target.ID])
	}
	if len(repository.audits) != expectedAudits {
		t.Fatalf("completed publish wrote duplicate audit entries: %#v", repository.audits)
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
		SnapshotChecksum: target.Checksum,
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

func TestRollbackRewritesLegacySnapshotWhenCanonicalChecksumChanged(t *testing.T) {
	repository := newMemoryRepository()
	canonical, err := canonicalSnapshot([]byte(`{"schemaVersion":"1.0.0","releaseId":"rel_old","value":0.50}`))
	if err != nil {
		t.Fatalf("canonicalize target: %v", err)
	}
	target := approvedVersion("ver_old", "rel_old")
	target.Status = domain.StatusArchived
	target.Snapshot = canonical.JSON
	target.Checksum = canonical.Checksum
	repository.versions[target.ID] = target
	legacyJSON := []byte(`{"releaseId":"rel_old","schemaVersion":"1.0.0","value":0.50}`)
	historical := domain.PublishJob{
		ID:               "pub_history",
		VersionID:        target.ID,
		SnapshotKey:      "snapshots/rel_old/legacy.json",
		SnapshotChecksum: snapshotdata.Checksum(legacyJSON),
		Status:           domain.PublishSucceeded,
	}
	repository.jobs[historical.ID] = historical
	repository.successfulByVersion[target.ID] = historical.ID
	blobs := newBlobStoreFake()
	trigger := &buildTriggerFake{statuses: make(map[string]ports.BuildRun)}
	service := publishServiceForTest(repository, blobs, trigger)

	job, err := service.Rollback(context.Background(), publisher(), target.ID, "idem-rollback-legacy-number")
	if err != nil {
		t.Fatalf("rollback: %v", err)
	}
	wantedKey := "snapshots/rel_old/" + canonical.Checksum + ".json"
	if job.SnapshotKey != wantedKey || job.SnapshotChecksum != canonical.Checksum {
		t.Fatalf("rollback did not use current canonical snapshot: %#v", job)
	}
	if blobs.putCalls != 1 || blobs.objects[wantedKey].Checksum != canonical.Checksum {
		t.Fatalf("canonical snapshot was not persisted: calls=%d objects=%#v", blobs.putCalls, blobs.objects)
	}
	if len(trigger.requests) != 1 || trigger.requests[0].SnapshotChecksum != canonical.Checksum {
		t.Fatalf("build trigger received stale checksum: %#v", trigger.requests)
	}
}

func TestRollbackSuccessWritesRollbackAudit(t *testing.T) {
	repository := newMemoryRepository()
	target := approvedVersion("ver_old", "rel_old")
	target.Status = domain.StatusArchived
	repository.versions[target.ID] = target
	historical := domain.PublishJob{
		ID:               "pub_history",
		VersionID:        target.ID,
		SnapshotKey:      "snapshots/rel_old/sha256-old.json",
		SnapshotChecksum: target.Checksum,
		Status:           domain.PublishSucceeded,
	}
	repository.jobs[historical.ID] = historical
	repository.successfulByVersion[target.ID] = historical.ID
	trigger := &buildTriggerFake{statuses: make(map[string]ports.BuildRun)}
	service := publishServiceForTest(repository, newBlobStoreFake(), trigger)

	job, err := service.Rollback(context.Background(), publisher(), target.ID, "idem-rollback-audit")
	if err != nil {
		t.Fatalf("rollback: %v", err)
	}
	trigger.statuses[job.BuildID] = ports.BuildRun{ID: job.BuildID, Status: domain.PublishSucceeded}
	if _, err := service.RefreshStatus(context.Background(), publisher(), job.ID); err != nil {
		t.Fatalf("refresh rollback: %v", err)
	}

	lastAudit := repository.audits[len(repository.audits)-1]
	if lastAudit.Action != "rollback.succeeded" {
		t.Fatalf("expected rollback success audit, got %q", lastAudit.Action)
	}
}

func TestRollbackFailureWritesRollbackAudit(t *testing.T) {
	repository := newMemoryRepository()
	target := approvedVersion("ver_old", "rel_old")
	target.Status = domain.StatusArchived
	repository.versions[target.ID] = target
	historical := domain.PublishJob{
		ID:               "pub_history",
		VersionID:        target.ID,
		SnapshotKey:      "snapshots/rel_old/sha256-old.json",
		SnapshotChecksum: target.Checksum,
		Status:           domain.PublishSucceeded,
	}
	repository.jobs[historical.ID] = historical
	repository.successfulByVersion[target.ID] = historical.ID
	trigger := &buildTriggerFake{statuses: make(map[string]ports.BuildRun)}
	service := publishServiceForTest(repository, newBlobStoreFake(), trigger)

	job, err := service.Rollback(context.Background(), publisher(), target.ID, "idem-rollback-failed-audit")
	if err != nil {
		t.Fatalf("rollback: %v", err)
	}
	trigger.statuses[job.BuildID] = ports.BuildRun{ID: job.BuildID, Status: domain.PublishFailed, Error: "build failed"}
	if _, err := service.RefreshStatus(context.Background(), publisher(), job.ID); err != nil {
		t.Fatalf("refresh rollback: %v", err)
	}

	lastAudit := repository.audits[len(repository.audits)-1]
	if lastAudit.Action != "rollback.failed" {
		t.Fatalf("expected rollback failure audit, got %q", lastAudit.Action)
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
