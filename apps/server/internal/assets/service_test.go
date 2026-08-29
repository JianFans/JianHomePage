package assets

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"testing"
	"time"

	"yujian.me/server/internal/domain"
	"yujian.me/server/internal/ports"
)

type memoryRepository struct {
	assets    map[string]domain.AssetRecord
	audits    []domain.AuditEntry
	updateErr error
	beforeUpdate func(*memoryRepository)
}

func newMemoryRepository() *memoryRepository {
	return &memoryRepository{assets: make(map[string]domain.AssetRecord)}
}

func (repository *memoryRepository) WithinTransaction(ctx context.Context, run func(Repository) error) error {
	return run(repository)
}

func (repository *memoryRepository) CreateAsset(_ context.Context, asset domain.AssetRecord) error {
	repository.assets[asset.ID] = asset
	return nil
}

func (repository *memoryRepository) GetAsset(_ context.Context, id string) (domain.AssetRecord, error) {
	asset, exists := repository.assets[id]
	if !exists {
		return domain.AssetRecord{}, domain.ErrNotFound
	}
	return asset, nil
}

func (repository *memoryRepository) UpdateAsset(_ context.Context, asset domain.AssetRecord, expectedStatus domain.AssetStatus) error {
	if repository.updateErr != nil {
		return repository.updateErr
	}
	if repository.beforeUpdate != nil {
		repository.beforeUpdate(repository)
		repository.beforeUpdate = nil
	}
	current, exists := repository.assets[asset.ID]
	if !exists {
		return domain.ErrNotFound
	}
	if current.Status != expectedStatus {
		return domain.ErrConflict
	}
	repository.assets[asset.ID] = asset
	return nil
}

func (repository *memoryRepository) AppendAudit(_ context.Context, entry domain.AuditEntry) error {
	repository.audits = append(repository.audits, entry)
	return nil
}

type blobStoreFake struct {
	uploads     []ports.UploadRequest
	metadata    ports.BlobMetadata
	deletedKeys []string
	createErr   error
	statErr     error
	deleteErr   error
}

func (store *blobStoreFake) CreateUpload(_ context.Context, request ports.UploadRequest) (ports.SignedUpload, error) {
	if store.createErr != nil {
		return ports.SignedUpload{}, store.createErr
	}
	store.uploads = append(store.uploads, request)
	return ports.SignedUpload{
		URL:       "https://upload.example.com/signed",
		ExpiresAt: time.Date(2026, 8, 29, 10, 15, 0, 0, time.UTC),
	}, nil
}

func (store *blobStoreFake) Stat(context.Context, string) (ports.BlobMetadata, error) {
	return store.metadata, store.statErr
}

func (store *blobStoreFake) Put(context.Context, string, io.Reader, ports.BlobMetadata) error {
	return nil
}

func (store *blobStoreFake) Delete(_ context.Context, key string) error {
	if store.deleteErr != nil {
		return store.deleteErr
	}
	store.deletedKeys = append(store.deletedKeys, key)
	return nil
}

func (store *blobStoreFake) SignedReadURL(context.Context, string, time.Duration) (string, error) {
	return "https://read.example.com/signed", nil
}

func assetServiceForTest(repository *memoryRepository, blobs *blobStoreFake) *Service {
	return NewService(ServiceOptions{
		Repository: repository,
		BlobStore:  blobs,
		Now: func() time.Time {
			return time.Date(2026, 8, 29, 10, 0, 0, 0, time.UTC)
		},
		NewID: func(prefix string) string { return prefix + "fixed" },
	})
}

func editor() domain.Principal {
	return domain.Principal{Subject: "editor-1", Roles: []domain.Role{domain.RoleEditor}}
}

func admin() domain.Principal {
	return domain.Principal{Subject: "admin-1", Roles: []domain.Role{domain.RoleAdmin}}
}

func TestCreateUploadValidatesTypeAndCreatesProviderIndependentKey(t *testing.T) {
	repository := newMemoryRepository()
	blobs := &blobStoreFake{}
	service := assetServiceForTest(repository, blobs)

	result, err := service.CreateUpload(context.Background(), editor(), CreateUploadInput{
		FileName:    "cover.webp",
		ContentType: "image/webp",
		Size:        1024,
		Checksum:    "sha256:" + string(bytes.Repeat([]byte{'a'}, 64)),
		Rights:      json.RawMessage(`{"source":"authorized"}`),
	})
	if err != nil {
		t.Fatalf("create upload: %v", err)
	}
	if result.Asset.ID != "asset_fixed" {
		t.Fatalf("unexpected asset id %q", result.Asset.ID)
	}
	if result.Asset.BlobKey != "assets/asset_fixed/source.webp" {
		t.Fatalf("unexpected blob key %q", result.Asset.BlobKey)
	}
	if result.Upload.URL != "https://upload.example.com/signed" {
		t.Fatalf("unexpected upload URL %q", result.Upload.URL)
	}
	if len(blobs.uploads) != 1 || blobs.uploads[0].ExpiresIn != 15*time.Minute {
		t.Fatalf("unexpected upload request %#v", blobs.uploads)
	}
}

func TestCreateUploadRejectsMismatchAndOversizeBeforeBlobCall(t *testing.T) {
	tests := []CreateUploadInput{
		{FileName: "cover.jpg", ContentType: "image/webp", Size: 1024},
		{FileName: "video.mp4", ContentType: "video/mp4", Size: 2*1024*1024*1024 + 1},
		{FileName: "script.svg", ContentType: "image/svg+xml", Size: 1024},
	}

	for _, input := range tests {
		blobs := &blobStoreFake{}
		service := assetServiceForTest(newMemoryRepository(), blobs)
		if _, err := service.CreateUpload(context.Background(), editor(), input); !errors.Is(err, domain.ErrInvalidInput) {
			t.Fatalf("input %#v: expected invalid input, got %v", input, err)
		}
		if len(blobs.uploads) != 0 {
			t.Fatalf("input %#v called blob store", input)
		}
	}
}

func TestCreateUploadRejectsMissingOrNonObjectRightsBeforeBlobCall(t *testing.T) {
	for _, rights := range []json.RawMessage{nil, json.RawMessage(`null`), json.RawMessage(`[]`)} {
		blobs := &blobStoreFake{}
		service := assetServiceForTest(newMemoryRepository(), blobs)
		_, err := service.CreateUpload(context.Background(), editor(), CreateUploadInput{
			FileName: "cover.webp", ContentType: "image/webp", Size: 1024, Rights: rights,
		})
		if !errors.Is(err, domain.ErrInvalidInput) {
			t.Fatalf("rights %s: expected invalid input, got %v", rights, err)
		}
		if len(blobs.uploads) != 0 {
			t.Fatalf("rights %s called blob store", rights)
		}
	}
}

func TestCompleteUploadChecksActualMetadata(t *testing.T) {
	repository := newMemoryRepository()
	blobs := &blobStoreFake{}
	service := assetServiceForTest(repository, blobs)
	created, err := service.CreateUpload(context.Background(), editor(), CreateUploadInput{
		FileName:    "cover.webp",
		ContentType: "image/webp",
		Size:        1024,
		Checksum:    "sha256:" + string(bytes.Repeat([]byte{'b'}, 64)),
		Rights:      json.RawMessage(`{"source":"authorized"}`),
	})
	if err != nil {
		t.Fatalf("create upload: %v", err)
	}

	blobs.metadata = ports.BlobMetadata{
		ContentType: "image/webp",
		Size:        1024,
		Checksum:    "sha256:" + string(bytes.Repeat([]byte{'c'}, 64)),
		Width:       1200,
		Height:      1200,
	}
	if _, err := service.CompleteUpload(context.Background(), editor(), created.Asset.ID); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("expected checksum mismatch, got %v", err)
	}
	if repository.assets[created.Asset.ID].Status != domain.AssetPending {
		t.Fatal("mismatched upload must remain pending")
	}

	blobs.metadata.Checksum = "sha256:" + string(bytes.Repeat([]byte{'b'}, 64))
	completed, err := service.CompleteUpload(context.Background(), editor(), created.Asset.ID)
	if err != nil {
		t.Fatalf("complete upload: %v", err)
	}
	if completed.Status != domain.AssetReady {
		t.Fatalf("unexpected status %q", completed.Status)
	}
}

func TestDeleteAssetRequiresAdminAndWritesAudit(t *testing.T) {
	repository := newMemoryRepository()
	blobs := &blobStoreFake{}
	service := assetServiceForTest(repository, blobs)
	created, err := service.CreateUpload(context.Background(), editor(), CreateUploadInput{
		FileName:    "cover.webp",
		ContentType: "image/webp",
		Size:        1024,
		Rights:      json.RawMessage(`{"source":"authorized"}`),
	})
	if err != nil {
		t.Fatalf("create upload: %v", err)
	}

	if err := service.Delete(context.Background(), editor(), created.Asset.ID); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("editor must not delete: %v", err)
	}
	if err := service.Delete(context.Background(), admin(), created.Asset.ID); err != nil {
		t.Fatalf("delete asset: %v", err)
	}
	if repository.assets[created.Asset.ID].Status != domain.AssetDeleted {
		t.Fatal("asset should be marked deleted")
	}
	if len(blobs.deletedKeys) != 1 || len(repository.audits) != 2 {
		t.Fatalf("unexpected delete effects keys=%v audits=%v", blobs.deletedKeys, repository.audits)
	}
}

func TestDeleteAssetDoesNotDeleteBlobBeforeDatabaseCommit(t *testing.T) {
	repository := newMemoryRepository()
	blobs := &blobStoreFake{}
	service := assetServiceForTest(repository, blobs)
	created, err := service.CreateUpload(context.Background(), editor(), CreateUploadInput{
		FileName: "cover.webp", ContentType: "image/webp", Size: 1024,
		Rights: json.RawMessage(`{"source":"authorized"}`),
	})
	if err != nil {
		t.Fatalf("create upload: %v", err)
	}
	repository.updateErr = errors.New("database unavailable")

	if err := service.Delete(context.Background(), admin(), created.Asset.ID); !errors.Is(err, repository.updateErr) {
		t.Fatalf("expected database error, got %v", err)
	}
	if len(blobs.deletedKeys) != 0 {
		t.Fatalf("blob was deleted before durable state change: %#v", blobs.deletedKeys)
	}
}

func TestDeleteAssetCanRetryBlobCleanupAfterProviderFailure(t *testing.T) {
	repository := newMemoryRepository()
	blobs := &blobStoreFake{deleteErr: errors.New("object storage unavailable")}
	service := assetServiceForTest(repository, blobs)
	created, err := service.CreateUpload(context.Background(), editor(), CreateUploadInput{
		FileName: "cover.webp", ContentType: "image/webp", Size: 1024,
		Rights: json.RawMessage(`{"source":"authorized"}`),
	})
	if err != nil {
		t.Fatalf("create upload: %v", err)
	}

	if err := service.Delete(context.Background(), admin(), created.Asset.ID); !errors.Is(err, blobs.deleteErr) {
		t.Fatalf("expected provider error, got %v", err)
	}
	if repository.assets[created.Asset.ID].Status != domain.AssetDeleted {
		t.Fatal("asset must be durably hidden before blob cleanup")
	}
	blobs.deleteErr = nil
	if err := service.Delete(context.Background(), admin(), created.Asset.ID); err != nil {
		t.Fatalf("retry cleanup: %v", err)
	}
	if len(blobs.deletedKeys) != 1 {
		t.Fatalf("expected one successful cleanup, got %#v", blobs.deletedKeys)
	}
}

func TestCompleteUploadDoesNotRestoreConcurrentlyDeletedAsset(t *testing.T) {
	repository := newMemoryRepository()
	blobs := &blobStoreFake{}
	service := assetServiceForTest(repository, blobs)
	created, err := service.CreateUpload(context.Background(), editor(), CreateUploadInput{
		FileName: "cover.webp", ContentType: "image/webp", Size: 10, Rights: json.RawMessage(`{"owner":"team"}`),
	})
	if err != nil {
		t.Fatalf("create upload: %v", err)
	}
	blobs.metadata = ports.BlobMetadata{ContentType: "image/webp", Size: 10, Checksum: "sha256:test"}
	repository.beforeUpdate = func(repository *memoryRepository) {
		asset := repository.assets[created.Asset.ID]
		asset.Status = domain.AssetDeleted
		repository.assets[asset.ID] = asset
	}

	if _, err := service.CompleteUpload(context.Background(), editor(), created.Asset.ID); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("expected concurrent state conflict, got %v", err)
	}
	if repository.assets[created.Asset.ID].Status != domain.AssetDeleted {
		t.Fatalf("concurrent delete was overwritten: %#v", repository.assets[created.Asset.ID])
	}
}
