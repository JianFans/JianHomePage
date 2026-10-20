package assets

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"
	"time"

	"yujian.me/server/internal/domain"
	"yujian.me/server/internal/ports"
)

type memoryRepository struct {
	assets       map[string]domain.AssetRecord
	audits       []domain.AuditEntry
	updateErr    error
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

// GetAsset exposes the fixture's current lifecycle state and models missing IDs
// separately from status conflicts injected by UpdateAsset.
func (repository *memoryRepository) GetAsset(_ context.Context, id string) (domain.AssetRecord, error) {
	asset, exists := repository.assets[id]
	if !exists {
		return domain.AssetRecord{}, domain.ErrNotFound
	}
	return asset, nil
}

// ListAssets models status filtering and the strict timestamp/ID boundary so
// service tests can verify cursor generation independently of a database.
func (repository *memoryRepository) ListAssets(_ context.Context, query ListQuery) ([]domain.AssetRecord, error) {
	items := make([]domain.AssetRecord, 0, len(repository.assets))
	for _, asset := range repository.assets {
		if !slices.Contains(query.Statuses, asset.Status) {
			continue
		}
		if query.BeforeCreatedAt != nil &&
			(asset.CreatedAt.After(*query.BeforeCreatedAt) ||
				(asset.CreatedAt.Equal(*query.BeforeCreatedAt) && asset.ID >= query.BeforeID)) {
			continue
		}
		items = append(items, asset)
	}
	slices.SortFunc(items, func(left, right domain.AssetRecord) int {
		if compared := right.CreatedAt.Compare(left.CreatedAt); compared != 0 {
			return compared
		}
		return -strings.Compare(left.ID, right.ID)
	})
	if len(items) > query.Limit {
		items = items[:query.Limit]
	}
	return items, nil
}

// UpdateAsset can inject a competing change before checking expected status,
// letting service tests exercise lost-update protection deterministically.
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

// AppendAudit captures service events so tests can distinguish idempotent retries
// from transitions that should create a new audit record.
func (repository *memoryRepository) AppendAudit(_ context.Context, entry domain.AuditEntry) error {
	repository.audits = append(repository.audits, entry)
	return nil
}

// EnsureAssetSourceURL models the narrow atomic repair contract for service tests.
func (repository *memoryRepository) EnsureAssetSourceURL(_ context.Context, id, sourceURL string) (string, error) {
	if sourceURL == "" {
		return "", domain.ErrInvalidInput
	}
	if repository.updateErr != nil {
		return "", repository.updateErr
	}
	asset, exists := repository.assets[id]
	if !exists {
		return "", domain.ErrNotFound
	}
	if asset.SourceURL == "" {
		asset.SourceURL = sourceURL
		repository.assets[id] = asset
	}
	return asset.SourceURL, nil
}

type blobStoreFake struct {
	uploads     []ports.UploadRequest
	metadata    ports.BlobMetadata
	deletedKeys []string
	publicURL   string
	publicCalls int
	publicErr   error
	createErr   error
	statErr     error
	statCalls   int
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
	store.statCalls++
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

// SignedReadURL supplies a deterministic temporary read URL independently of the
// stable PublicURL used by asset snapshots and legacy-address repair.
func (store *blobStoreFake) SignedReadURL(context.Context, string, time.Duration) (string, error) {
	return "https://read.example.com/signed", nil
}

// PublicURL records resolution attempts and injects provider failures or URL
// changes to test that persisted addresses are repaired once and then frozen.
func (store *blobStoreFake) PublicURL(_ context.Context, key string) (string, error) {
	store.publicCalls++
	if store.publicErr != nil {
		return "", store.publicErr
	}
	if store.publicURL != "" {
		return store.publicURL, nil
	}
	return "https://media.example.com/" + key, nil
}

func assetServiceForTest(repository *memoryRepository, blobs *blobStoreFake) *Service {
	return NewService(ServiceOptions{
		Repository: repository,
		BlobStore:  blobs,
		Now: func() time.Time {
			return time.Date(2026, 8, 29, 10, 0, 0, 0, time.UTC)
		},
		NewID: func(prefix string) string { return prefix + "550e8400-e29b-41d4-a716-446655440000" },
	})
}

func editor() domain.Principal {
	return domain.Principal{Subject: "editor-1", Roles: []domain.Role{domain.RoleEditor}}
}

// admin supplies the delete permission omitted from editor fixtures so asset
// lifecycle tests can verify both write roles without authentication machinery.
func admin() domain.Principal {
	return domain.Principal{Subject: "admin-1", Roles: []domain.Role{domain.RoleAdmin}}
}

// TestListAssetsUsesStableCursorAndDefaultStatuses covers equal-time ID ordering,
// deleted-record exclusion and a final page without a continuation cursor.
func TestListAssetsUsesStableCursorAndDefaultStatuses(t *testing.T) {
	createdAt := time.Date(2026, 10, 9, 8, 0, 0, 0, time.UTC)
	repository := newMemoryRepository()
	repository.assets = map[string]domain.AssetRecord{
		"asset_0f5179cb-849b-4786-84ae-ae17f29ffbda": {ID: "asset_0f5179cb-849b-4786-84ae-ae17f29ffbda", Status: domain.AssetPending, CreatedAt: createdAt.Add(-time.Minute)},
		"asset_47f68d47-98a9-4288-be7c-c76c3dbcad67": {ID: "asset_47f68d47-98a9-4288-be7c-c76c3dbcad67", Status: domain.AssetReady, CreatedAt: createdAt},
		"asset_f964af2b-2c43-4b26-99fe-2f3b32e0b3d6": {ID: "asset_f964af2b-2c43-4b26-99fe-2f3b32e0b3d6", Status: domain.AssetPending, CreatedAt: createdAt},
		"asset_0c6cac0a-8ed5-4da9-b2f5-13b400740070": {ID: "asset_0c6cac0a-8ed5-4da9-b2f5-13b400740070", Status: domain.AssetDeleted, CreatedAt: createdAt.Add(time.Minute)},
	}
	service := assetServiceForTest(repository, &blobStoreFake{})

	first, err := service.List(t.Context(), editor(), ListOptions{Limit: 2})
	if err != nil {
		t.Fatalf("list first page: %v", err)
	}
	if got := assetIDs(first.Items); !slices.Equal(got, []string{"asset_f964af2b-2c43-4b26-99fe-2f3b32e0b3d6", "asset_47f68d47-98a9-4288-be7c-c76c3dbcad67"}) {
		t.Fatalf("unexpected first page %v", got)
	}
	if first.NextCursor == "" {
		t.Fatal("expected next cursor")
	}

	second, err := service.List(t.Context(), editor(), ListOptions{Limit: 2, Cursor: first.NextCursor})
	if err != nil {
		t.Fatalf("list second page: %v", err)
	}
	if got := assetIDs(second.Items); !slices.Equal(got, []string{"asset_0f5179cb-849b-4786-84ae-ae17f29ffbda"}) {
		t.Fatalf("unexpected second page %v", got)
	}
	if second.NextCursor != "" {
		t.Fatalf("unexpected trailing cursor %q", second.NextCursor)
	}
}

// TestListRepairsLegacySources covers records written by old instances after
// startup migration, without replacing existing provider URLs or lookahead rows.
func TestListRepairsLegacySources(t *testing.T) {
	for _, status := range []domain.AssetStatus{domain.AssetPending, domain.AssetReady, domain.AssetDeleted} {
		t.Run(string(status), func(t *testing.T) {
			repository := newMemoryRepository()
			now := time.Now().UTC()
			for _, id := range []string{"asset_0f5179cb-849b-4786-84ae-ae17f29ffbda", "asset_47f68d47-98a9-4288-be7c-c76c3dbcad67", "asset_f964af2b-2c43-4b26-99fe-2f3b32e0b3d6"} {
				repository.assets[id] = domain.AssetRecord{ID: id, BlobKey: "assets/" + id + "/source.webp", Status: status, CreatedAt: now}
			}
			blobs := &blobStoreFake{}
			service := assetServiceForTest(repository, blobs)
			page, err := service.List(t.Context(), editor(), ListOptions{Status: status, Limit: 1})
			if err != nil {
				t.Fatal(err)
			}
			if len(page.Items) != 1 || page.Items[0].SourceURL != "https://media.example.com/assets/asset_f964af2b-2c43-4b26-99fe-2f3b32e0b3d6/source.webp" {
				t.Fatalf("legacy URL not repaired: %#v", page)
			}
			if repository.assets["asset_f964af2b-2c43-4b26-99fe-2f3b32e0b3d6"].SourceURL != page.Items[0].SourceURL || blobs.publicCalls != 1 || repository.assets["asset_47f68d47-98a9-4288-be7c-c76c3dbcad67"].SourceURL != "" {
				t.Fatal("repair must persist only returned page records")
			}
			blobs.publicURL = "https://other-provider.example/changed.webp"
			if _, err := service.List(t.Context(), editor(), ListOptions{Status: status, Limit: 1}); err != nil {
				t.Fatal(err)
			}
			if blobs.publicCalls != 1 {
				t.Fatal("persisted source URL was recalculated")
			}
		})
	}
}

// TestListLegacySourceRepairPropagatesErrors ensures neither URL resolution nor
// persistence failure returns a partially repaired page to callers.
func TestListLegacySourceRepairPropagatesErrors(t *testing.T) {
	repository := newMemoryRepository()
	repository.assets["asset_58d6e938-5806-4086-8a5d-f3908326255e"] = domain.AssetRecord{ID: "asset_58d6e938-5806-4086-8a5d-f3908326255e", BlobKey: "assets/legacy/source.webp", Status: domain.AssetReady, CreatedAt: time.Now()}
	providerErr := errors.New("provider unavailable")
	blobs := &blobStoreFake{publicErr: providerErr}
	page, err := assetServiceForTest(repository, blobs).List(t.Context(), editor(), ListOptions{})
	if !errors.Is(err, providerErr) || len(page.Items) != 0 || repository.assets["asset_58d6e938-5806-4086-8a5d-f3908326255e"].SourceURL != "" {
		t.Fatalf("provider failure must not return invalid page: page=%#v error=%v", page, err)
	}
	blobs.publicErr = nil
	repository.updateErr = errors.New("database unavailable")
	page, err = assetServiceForTest(repository, blobs).List(t.Context(), editor(), ListOptions{})
	if !errors.Is(err, repository.updateErr) || len(page.Items) != 0 {
		t.Fatalf("storage repair failure must propagate: page=%#v error=%v", page, err)
	}
}

// TestListAssetsSupportsExplicitDeletedStatus verifies that deleted records stay
// accessible through an explicit filter despite being hidden by the default.
func TestListAssetsSupportsExplicitDeletedStatus(t *testing.T) {
	repository := newMemoryRepository()
	repository.assets["asset_bc2e7dfb-aa59-4010-a1e9-3eea6242d9af"] = domain.AssetRecord{
		ID: "asset_bc2e7dfb-aa59-4010-a1e9-3eea6242d9af", Status: domain.AssetDeleted, CreatedAt: time.Now(),
	}
	service := assetServiceForTest(repository, &blobStoreFake{})

	page, err := service.List(t.Context(), editor(), ListOptions{Status: domain.AssetDeleted})
	if err != nil {
		t.Fatalf("list deleted assets: %v", err)
	}
	if got := assetIDs(page.Items); !slices.Equal(got, []string{"asset_bc2e7dfb-aa59-4010-a1e9-3eea6242d9af"}) {
		t.Fatalf("unexpected deleted assets %v", got)
	}
}

// TestListAssetsRejectsInvalidOptions covers status and page-size limits plus
// malformed, incomplete and NUL-bearing cursors at the service boundary.
func TestListAssetsRejectsInvalidOptions(t *testing.T) {
	service := assetServiceForTest(newMemoryRepository(), &blobStoreFake{})
	for _, options := range []ListOptions{
		{Status: domain.AssetStatus("unknown")},
		{Limit: -1},
		{Limit: 101},
		{Cursor: "not-base64"},
		{Cursor: "e30"},
		{Cursor: base64.RawURLEncoding.EncodeToString([]byte(`{"createdAt":"2026-10-09T00:00:00Z","id":"asset_\u0000"}`))},
	} {
		if _, err := service.List(t.Context(), editor(), options); !errors.Is(err, domain.ErrInvalidInput) {
			t.Fatalf("options %#v: expected invalid input, got %v", options, err)
		}
	}
}

// TestListAssetsRequiresCreatePermission prevents review-only identities from
// reading the asset library without the editor's create_asset permission.
func TestListAssetsRequiresCreatePermission(t *testing.T) {
	service := assetServiceForTest(newMemoryRepository(), &blobStoreFake{})
	actor := domain.Principal{Subject: "reviewer", Roles: []domain.Role{domain.RoleReviewer}}
	if _, err := service.List(t.Context(), actor, ListOptions{}); !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected forbidden, got %v", err)
	}
}

// assetIDs preserves page order while projecting records for pagination
// assertions, avoiding comparisons of unrelated metadata and timestamps.
func assetIDs(items []domain.AssetRecord) []string {
	ids := make([]string, len(items))
	for index, item := range items {
		ids[index] = item.ID
	}
	return ids
}

// TestCreateUploadValidatesTypeAndCreatesProviderIndependentKey checks that one
// reservation uses a stable asset key, public URL and bounded signature lifetime.
func TestCreateUploadValidatesTypeAndCreatesProviderIndependentKey(t *testing.T) {
	repository := newMemoryRepository()
	blobs := &blobStoreFake{}
	service := assetServiceForTest(repository, blobs)

	result, err := service.CreateUpload(context.Background(), editor(), CreateUploadInput{
		FileName:    "cover.webp",
		ContentType: "image/webp",
		Size:        1024,
		Checksum:    "sha256:" + string(bytes.Repeat([]byte{'a'}, 64)),
		Rights:      json.RawMessage(`{"source":{"zh-CN":"authorized"}}`),
	})
	if err != nil {
		t.Fatalf("create upload: %v", err)
	}
	if result.Asset.ID != "asset_550e8400-e29b-41d4-a716-446655440000" {
		t.Fatalf("unexpected asset id %q", result.Asset.ID)
	}
	if result.Asset.BlobKey != "assets/asset_550e8400-e29b-41d4-a716-446655440000/source.webp" {
		t.Fatalf("unexpected blob key %q", result.Asset.BlobKey)
	}
	if result.Upload.URL != "https://upload.example.com/signed" {
		t.Fatalf("unexpected upload URL %q", result.Upload.URL)
	}
	if result.Asset.SourceURL != "https://media.example.com/assets/asset_550e8400-e29b-41d4-a716-446655440000/source.webp" {
		t.Fatalf("unexpected public source URL %q", result.Asset.SourceURL)
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
		{FileName: "cover.webp", ContentType: "image/webp", Size: 1024, Checksum: "sha256:not-a-digest"},
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

func TestUploadFormatsMatchSnapshotContract(t *testing.T) {
	validChecksum := "sha256:" + string(bytes.Repeat([]byte{'a'}, 64))
	service := assetServiceForTest(newMemoryRepository(), &blobStoreFake{})
	if _, err := service.CreateUpload(context.Background(), editor(), CreateUploadInput{
		FileName: "animation.gif", ContentType: "image/gif", Size: 1024,
		Checksum: validChecksum, Rights: json.RawMessage(`{"source":{"zh-CN":"authorized"}}`),
	}); err != nil {
		t.Fatalf("snapshot-supported GIF should be uploadable: %v", err)
	}

	for _, input := range []CreateUploadInput{
		{FileName: "photo.avif", ContentType: "image/avif", Size: 1024, Checksum: validChecksum, Rights: json.RawMessage(`{"source":{"zh-CN":"authorized"}}`)},
		{FileName: "photo.jpg", ContentType: "image/jpeg", Size: 1024, Checksum: validChecksum, Rights: json.RawMessage(`{"source":{"zh-CN":"authorized"}}`)},
		{FileName: "clip.webm", ContentType: "video/webm", Size: 1024, Checksum: validChecksum, Rights: json.RawMessage(`{"source":{"zh-CN":"authorized"}}`)},
	} {
		if _, err := service.CreateUpload(context.Background(), editor(), input); !errors.Is(err, domain.ErrInvalidInput) {
			t.Fatalf("format outside snapshot contract should be rejected: %#v err=%v", input, err)
		}
	}
}

func TestCreateUploadRejectsNonCanonicalContentTypeBeforeBlobCall(t *testing.T) {
	for _, contentType := range []string{"Image/WebP", "image/WEBP"} {
		t.Run(contentType, func(t *testing.T) {
			blobs := &blobStoreFake{}
			service := assetServiceForTest(newMemoryRepository(), blobs)
			_, err := service.CreateUpload(context.Background(), editor(), CreateUploadInput{
				FileName: "cover.webp", ContentType: contentType, Size: 1024,
				Checksum: "sha256:" + string(bytes.Repeat([]byte{'a'}, 64)),
				Rights:   json.RawMessage(`{"source":{"zh-CN":"authorized"}}`),
			})
			if !errors.Is(err, domain.ErrInvalidInput) {
				t.Fatalf("expected non-canonical content type rejection, got %v", err)
			}
			if len(blobs.uploads) != 0 {
				t.Fatalf("non-canonical content type called blob store: %#v", blobs.uploads)
			}
		})
	}
}

func TestCreateUploadRequiresContentChecksum(t *testing.T) {
	blobs := &blobStoreFake{}
	service := assetServiceForTest(newMemoryRepository(), blobs)

	_, err := service.CreateUpload(context.Background(), editor(), CreateUploadInput{
		FileName: "cover.webp", ContentType: "image/webp", Size: 1024,
		Rights: json.RawMessage(`{"source":{"zh-CN":"authorized"}}`),
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("expected missing checksum rejection, got %v", err)
	}
	if len(blobs.uploads) != 0 {
		t.Fatal("missing checksum reached blob store")
	}
}

func TestCreateUploadNormalizesUppercaseChecksum(t *testing.T) {
	blobs := &blobStoreFake{}
	service := assetServiceForTest(newMemoryRepository(), blobs)
	result, err := service.CreateUpload(context.Background(), editor(), CreateUploadInput{
		FileName: "cover.webp", ContentType: "image/webp", Size: 1024,
		Checksum: "sha256:" + string(bytes.Repeat([]byte{'A'}, 64)),
		Rights:   json.RawMessage(`{"source":{"zh-CN":"authorized"}}`),
	})
	if err != nil {
		t.Fatalf("create upload: %v", err)
	}
	want := "sha256:" + string(bytes.Repeat([]byte{'a'}, 64))
	if len(blobs.uploads) != 1 || blobs.uploads[0].Checksum != want {
		t.Fatalf("checksum was not normalized before signing: %#v", blobs.uploads)
	}
	var metadata storedMetadata
	if err := json.Unmarshal(result.Asset.Metadata, &metadata); err != nil {
		t.Fatalf("decode metadata: %v", err)
	}
	if metadata.Checksum != want {
		t.Fatalf("checksum was not normalized before persistence: %q", metadata.Checksum)
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

func TestCreateUploadRejectsRightsOutsideSnapshotContract(t *testing.T) {
	checksum := "sha256:" + string(bytes.Repeat([]byte{'a'}, 64))
	for _, rights := range []json.RawMessage{
		json.RawMessage(`{}`),
		json.RawMessage(`{"source":"authorized"}`),
		json.RawMessage(`{"source":{}}`),
		json.RawMessage(`{"source":{"en":"authorized"}}`),
		json.RawMessage(`{"source":{"zh-CN":""}}`),
		json.RawMessage(`{"source":{"zh-CN":"authorized"},"unknown":true}`),
		json.RawMessage(`{"source":{"zh-CN":"authorized","unknown":"value"}}`),
		json.RawMessage(`{"source":{"zh-CN":"authorized","en":null}}`),
		json.RawMessage(`{"source":{"zh-CN":"authorized"},"credit":null}`),
		json.RawMessage(`{"source":{"zh-CN":"authorized"},"license":null}`),
	} {
		t.Run(string(rights), func(t *testing.T) {
			blobs := &blobStoreFake{}
			service := assetServiceForTest(newMemoryRepository(), blobs)
			_, err := service.CreateUpload(context.Background(), editor(), CreateUploadInput{
				FileName: "cover.webp", ContentType: "image/webp", Size: 1024,
				Checksum: checksum, Rights: rights,
			})
			if !errors.Is(err, domain.ErrInvalidInput) {
				t.Fatalf("rights %s: expected invalid input, got %v", rights, err)
			}
			if len(blobs.uploads) != 0 {
				t.Fatalf("rights %s called blob store", rights)
			}
		})
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
		Rights:      json.RawMessage(`{"source":{"zh-CN":"authorized"}}`),
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

func TestCompleteUploadCanRetryAfterReadyResponseIsLost(t *testing.T) {
	repository := newMemoryRepository()
	blobs := &blobStoreFake{}
	service := assetServiceForTest(repository, blobs)
	created, err := service.CreateUpload(context.Background(), editor(), CreateUploadInput{
		FileName: "cover.webp", ContentType: "image/webp", Size: 1024,
		Checksum: "sha256:" + string(bytes.Repeat([]byte{'b'}, 64)),
		Rights:   json.RawMessage(`{"source":{"zh-CN":"authorized"}}`),
	})
	if err != nil {
		t.Fatalf("create upload: %v", err)
	}
	blobs.metadata = ports.BlobMetadata{
		ContentType: "image/webp", Size: 1024,
		Checksum: "sha256:" + string(bytes.Repeat([]byte{'b'}, 64)),
	}

	first, err := service.CompleteUpload(context.Background(), editor(), created.Asset.ID)
	if err != nil {
		t.Fatalf("first complete: %v", err)
	}
	retried, err := service.CompleteUpload(context.Background(), editor(), created.Asset.ID)
	if err != nil {
		t.Fatalf("retry complete: %v", err)
	}
	if retried.Status != domain.AssetReady || retried.SourceURL != first.SourceURL {
		t.Fatalf("retry did not return ready asset: %#v", retried)
	}
	if blobs.statCalls != 1 || len(repository.audits) != 2 {
		t.Fatalf("retry repeated completion effects: stat=%d audits=%#v", blobs.statCalls, repository.audits)
	}
}

func TestCompleteUploadRetryPreservesPersistedSourceURL(t *testing.T) {
	repository := newMemoryRepository()
	repository.assets["asset_4c1ba641-5c3b-4477-91ea-7a24acc8aa17"] = domain.AssetRecord{
		ID: "asset_4c1ba641-5c3b-4477-91ea-7a24acc8aa17", BlobKey: "assets/asset_4c1ba641-5c3b-4477-91ea-7a24acc8aa17/source.webp",
		SourceURL: "https://media.yujian.me/assets/asset_4c1ba641-5c3b-4477-91ea-7a24acc8aa17/source.webp",
		Status:    domain.AssetReady,
	}
	blobs := &blobStoreFake{publicURL: "https://new-provider.example/assets/asset_4c1ba641-5c3b-4477-91ea-7a24acc8aa17/source.webp"}
	service := assetServiceForTest(repository, blobs)

	asset, err := service.CompleteUpload(t.Context(), editor(), "asset_4c1ba641-5c3b-4477-91ea-7a24acc8aa17")
	if err != nil {
		t.Fatalf("retry complete upload: %v", err)
	}
	if asset.SourceURL != repository.assets[asset.ID].SourceURL {
		t.Fatalf("persisted source URL was overwritten: %#v", asset)
	}
	if blobs.publicCalls != 0 {
		t.Fatalf("ready retry recalculated stable source URL %d times", blobs.publicCalls)
	}
}

func TestCompleteUploadPersistsSourceURLForLegacyPendingAsset(t *testing.T) {
	const checksum = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	const sourceURL = "https://media.yujian.me/assets/asset_58d6e938-5806-4086-8a5d-f3908326255e/source.webp"
	repository := newMemoryRepository()
	repository.assets["asset_58d6e938-5806-4086-8a5d-f3908326255e"] = domain.AssetRecord{
		ID: "asset_58d6e938-5806-4086-8a5d-f3908326255e", BlobKey: "assets/asset_58d6e938-5806-4086-8a5d-f3908326255e/source.webp", Status: domain.AssetPending,
		Metadata: json.RawMessage(`{"fileName":"cover.webp","contentType":"image/webp","declaredSize":1024,"checksum":"` + checksum + `"}`),
		Rights:   json.RawMessage(`{"source":{"zh-CN":"authorized"}}`),
	}
	blobs := &blobStoreFake{
		publicURL: sourceURL,
		metadata:  ports.BlobMetadata{ContentType: "image/webp", Size: 1024, Checksum: checksum},
	}
	service := assetServiceForTest(repository, blobs)

	asset, err := service.CompleteUpload(t.Context(), editor(), "asset_58d6e938-5806-4086-8a5d-f3908326255e")
	if err != nil {
		t.Fatalf("complete legacy pending upload: %v", err)
	}
	stored := repository.assets[asset.ID]
	if asset.SourceURL != sourceURL || stored.SourceURL != sourceURL || stored.Status != domain.AssetReady {
		t.Fatalf("legacy source URL was not persisted: response=%#v stored=%#v", asset, stored)
	}
	if blobs.publicCalls != 1 {
		t.Fatalf("legacy source URL resolved %d times", blobs.publicCalls)
	}
}

func TestCompleteUploadPersistsSourceURLForLegacyReadyAsset(t *testing.T) {
	const sourceURL = "https://media.yujian.me/assets/asset_58d6e938-5806-4086-8a5d-f3908326255e/source.webp"
	repository := newMemoryRepository()
	repository.assets["asset_58d6e938-5806-4086-8a5d-f3908326255e"] = domain.AssetRecord{
		ID: "asset_58d6e938-5806-4086-8a5d-f3908326255e", BlobKey: "assets/asset_58d6e938-5806-4086-8a5d-f3908326255e/source.webp", Status: domain.AssetReady,
	}
	blobs := &blobStoreFake{publicURL: sourceURL}
	service := assetServiceForTest(repository, blobs)

	asset, err := service.CompleteUpload(t.Context(), editor(), "asset_58d6e938-5806-4086-8a5d-f3908326255e")
	if err != nil {
		t.Fatalf("retry legacy ready upload: %v", err)
	}
	if asset.SourceURL != sourceURL || repository.assets[asset.ID].SourceURL != sourceURL {
		t.Fatalf("legacy ready source URL was not persisted: response=%#v stored=%#v", asset, repository.assets[asset.ID])
	}
	if blobs.publicCalls != 1 {
		t.Fatalf("legacy ready source URL resolved %d times", blobs.publicCalls)
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
		Checksum:    "sha256:" + string(bytes.Repeat([]byte{'d'}, 64)),
		Rights:      json.RawMessage(`{"source":{"zh-CN":"authorized"}}`),
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
		Checksum: "sha256:" + string(bytes.Repeat([]byte{'e'}, 64)),
		Rights:   json.RawMessage(`{"source":{"zh-CN":"authorized"}}`),
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
		Checksum: "sha256:" + string(bytes.Repeat([]byte{'f'}, 64)),
		Rights:   json.RawMessage(`{"source":{"zh-CN":"authorized"}}`),
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

func TestDeleteReadyAssetRetainsBlobForPublishedAndHistoricalSnapshots(t *testing.T) {
	repository := newMemoryRepository()
	checksum := "sha256:" + string(bytes.Repeat([]byte{'a'}, 64))
	blobs := &blobStoreFake{metadata: ports.BlobMetadata{
		ContentType: "image/webp", Size: 10, Checksum: checksum,
	}}
	service := assetServiceForTest(repository, blobs)
	created, err := service.CreateUpload(context.Background(), editor(), CreateUploadInput{
		FileName: "cover.webp", ContentType: "image/webp", Size: 10,
		Checksum: checksum, Rights: json.RawMessage(`{"source":{"zh-CN":"authorized"}}`),
	})
	if err != nil {
		t.Fatalf("create upload: %v", err)
	}
	if _, err := service.CompleteUpload(context.Background(), editor(), created.Asset.ID); err != nil {
		t.Fatalf("complete upload: %v", err)
	}
	if err := service.Delete(context.Background(), admin(), created.Asset.ID); err != nil {
		t.Fatalf("delete asset: %v", err)
	}
	if repository.assets[created.Asset.ID].Status != domain.AssetDeleted {
		t.Fatal("asset should be logically deleted")
	}
	if len(blobs.deletedKeys) != 0 {
		t.Fatalf("ready asset blob must be retained, deleted=%#v", blobs.deletedKeys)
	}
	if err := service.Delete(context.Background(), admin(), created.Asset.ID); err != nil {
		t.Fatalf("retry delete asset: %v", err)
	}
	if len(blobs.deletedKeys) != 0 {
		t.Fatalf("repeated delete must retain ready asset blob, deleted=%#v", blobs.deletedKeys)
	}
}

func TestCompleteUploadDoesNotRestoreConcurrentlyDeletedAsset(t *testing.T) {
	repository := newMemoryRepository()
	blobs := &blobStoreFake{}
	service := assetServiceForTest(repository, blobs)
	created, err := service.CreateUpload(context.Background(), editor(), CreateUploadInput{
		FileName: "cover.webp", ContentType: "image/webp", Size: 10,
		Checksum: "sha256:" + string(bytes.Repeat([]byte{'a'}, 64)),
		Rights:   json.RawMessage(`{"source":{"zh-CN":"authorized"}}`),
	})
	if err != nil {
		t.Fatalf("create upload: %v", err)
	}
	blobs.metadata = ports.BlobMetadata{
		ContentType: "image/webp", Size: 10,
		Checksum: "sha256:" + string(bytes.Repeat([]byte{'a'}, 64)),
	}
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
