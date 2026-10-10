package assets

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"yujian.me/server/internal/auth"
	"yujian.me/server/internal/domain"
	"yujian.me/server/internal/ports"
)

const (
	imageLimit = int64(20 * 1024 * 1024)
	audioLimit = int64(100 * 1024 * 1024)
	videoLimit = int64(2 * 1024 * 1024 * 1024)
)

type Repository interface {
	WithinTransaction(context.Context, func(Repository) error) error
	CreateAsset(context.Context, domain.AssetRecord) error
	GetAsset(context.Context, string) (domain.AssetRecord, error)
	ListAssets(context.Context, ListQuery) ([]domain.AssetRecord, error)
	EnsureAssetSourceURL(context.Context, string, string) (string, error)
	UpdateAsset(context.Context, domain.AssetRecord, domain.AssetStatus) error
	AppendAudit(context.Context, domain.AuditEntry) error
}

// ListQuery describes the repository-level asset filters and keyset boundary.
type ListQuery struct {
	Statuses        []domain.AssetStatus
	BeforeCreatedAt *time.Time
	BeforeID        string
	Limit           int
}

// ListOptions contains the public asset-list filters.
type ListOptions struct {
	Status domain.AssetStatus
	Limit  int
	Cursor string
}

// ListPage is one stable page of assets and its optional continuation cursor.
type ListPage struct {
	Items      []domain.AssetRecord
	NextCursor string
}

type listCursor struct {
	CreatedAt time.Time `json:"createdAt"`
	ID        string    `json:"id"`
}

type ServiceOptions struct {
	Repository Repository
	BlobStore  ports.BlobStore
	Now        func() time.Time
	NewID      func(string) string
}

type Service struct {
	repository Repository
	blobStore  ports.BlobStore
	now        func() time.Time
	newID      func(string) string
}

type CreateUploadInput struct {
	FileName    string
	ContentType string
	Size        int64
	Checksum    string
	Rights      json.RawMessage
}

type CreateUploadResult struct {
	Asset  domain.AssetRecord
	Upload ports.SignedUpload
}

type storedMetadata struct {
	FileName     string        `json:"fileName"`
	ContentType  string        `json:"contentType"`
	DeclaredSize int64         `json:"declaredSize"`
	Checksum     string        `json:"checksum,omitempty"`
	RetainBlob   bool          `json:"retainBlob,omitempty"`
	Width        int           `json:"width,omitempty"`
	Height       int           `json:"height,omitempty"`
	Duration     time.Duration `json:"duration,omitempty"`
}

type assetRights struct {
	Source  *localizedRightsSource `json:"source"`
	Credit  json.RawMessage        `json:"credit,omitempty"`
	License json.RawMessage        `json:"license,omitempty"`
}

type localizedRightsSource struct {
	ZhCN string          `json:"zh-CN"`
	En   json.RawMessage `json:"en,omitempty"`
}

type mediaRule struct {
	extension string
	limit     int64
}

var mediaRules = map[string]mediaRule{
	"image/webp": {extension: ".webp", limit: imageLimit},
	"image/gif":  {extension: ".gif", limit: imageLimit},
	"audio/mpeg": {extension: ".mp3", limit: audioLimit},
	"audio/wav":  {extension: ".wav", limit: audioLimit},
	"video/mp4":  {extension: ".mp4", limit: videoLimit},
}

// NewService binds provider-independent asset ports, defaulting the clock and
// secure ID generator while allowing deterministic replacements in tests.
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
		repository: options.Repository,
		blobStore:  options.BlobStore,
		now:        now,
		newID:      newID,
	}
}

// List returns assets ordered by creation time and ID in descending order.
func (service *Service) List(ctx context.Context, actor domain.Principal, options ListOptions) (ListPage, error) {
	if !hasPermission(actor, auth.PermissionCreateAsset) {
		return ListPage{}, domain.ErrForbidden
	}

	query, pageLimit, err := normalizeListOptions(options)
	if err != nil {
		return ListPage{}, err
	}
	items, err := service.repository.ListAssets(ctx, query)
	if err != nil {
		return ListPage{}, err
	}

	page := ListPage{Items: items}
	if len(items) > pageLimit {
		page.Items = items[:pageLimit]
		page.NextCursor, err = encodeListCursor(page.Items[len(page.Items)-1])
		if err != nil {
			return ListPage{}, err
		}
	}
	// Older instances can write NULL after the startup migration. Repair only
	// returned records and persist once, keeping their original list ordering.
	for index := range page.Items {
		asset := &page.Items[index]
		if asset.SourceURL != "" {
			continue
		}
		sourceURL, err := service.blobStore.PublicURL(ctx, asset.BlobKey)
		if err != nil {
			return ListPage{}, err
		}
		asset.SourceURL, err = service.repository.EnsureAssetSourceURL(ctx, asset.ID, sourceURL)
		if err != nil {
			return ListPage{}, err
		}
	}
	return page, nil
}

// normalizeListOptions applies the active-status and page-size defaults and
// requests one extra record so List can decide whether to issue a next cursor.
func normalizeListOptions(options ListOptions) (ListQuery, int, error) {
	limit := options.Limit
	if limit == 0 {
		limit = 50
	}
	if limit < 1 || limit > 100 {
		return ListQuery{}, 0, domain.ErrInvalidInput
	}

	statuses := []domain.AssetStatus{domain.AssetPending, domain.AssetReady}
	if options.Status != "" {
		switch options.Status {
		case domain.AssetPending, domain.AssetReady, domain.AssetDeleted:
			statuses = []domain.AssetStatus{options.Status}
		default:
			return ListQuery{}, 0, domain.ErrInvalidInput
		}
	}

	query := ListQuery{Statuses: statuses, Limit: limit + 1}
	if options.Cursor == "" {
		return query, limit, nil
	}
	cursor, err := decodeListCursor(options.Cursor)
	if err != nil {
		return ListQuery{}, 0, domain.ErrInvalidInput
	}
	query.BeforeCreatedAt = &cursor.CreatedAt
	query.BeforeID = cursor.ID
	return query, limit, nil
}

// encodeListCursor preserves the last item's UTC timestamp and ID as a
// Base64URL boundary for the shared descending keyset order.
func encodeListCursor(asset domain.AssetRecord) (string, error) {
	encoded, err := json.Marshal(listCursor{CreatedAt: asset.CreatedAt.UTC(), ID: asset.ID})
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(encoded), nil
}

// decodeListCursor accepts exactly one known-field JSON boundary and rejects
// zero timestamps, blank IDs and NUL bytes before they reach the repositories.
func decodeListCursor(value string) (listCursor, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return listCursor{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(decoded))
	decoder.DisallowUnknownFields()
	var cursor listCursor
	if err := decoder.Decode(&cursor); err != nil {
		return listCursor{}, err
	}
	if decoder.Decode(&struct{}{}) != io.EOF || cursor.CreatedAt.IsZero() || strings.TrimSpace(cursor.ID) == "" || strings.ContainsRune(cursor.ID, '\x00') {
		return listCursor{}, domain.ErrInvalidInput
	}
	return cursor, nil
}

// CreateUpload validates permissions and media declarations before reserving an
// upload, then persists its frozen public URL and creation audit atomically.
func (service *Service) CreateUpload(
	ctx context.Context,
	actor domain.Principal,
	input CreateUploadInput,
) (CreateUploadResult, error) {
	if !hasPermission(actor, auth.PermissionCreateAsset) {
		return CreateUploadResult{}, domain.ErrForbidden
	}

	extension, err := validateUploadInput(input)
	if err != nil {
		return CreateUploadResult{}, err
	}
	input.Checksum = "sha256:" + strings.ToLower(strings.TrimPrefix(input.Checksum, "sha256:"))
	id := service.newID("asset_")
	blobKey := "assets/" + id + "/source" + extension
	upload, err := service.blobStore.CreateUpload(ctx, ports.UploadRequest{
		BlobKey:     blobKey,
		ContentType: input.ContentType,
		Size:        input.Size,
		Checksum:    input.Checksum,
		ExpiresIn:   15 * time.Minute,
	})
	if err != nil {
		return CreateUploadResult{}, err
	}
	sourceURL, err := service.blobStore.PublicURL(ctx, blobKey)
	if err != nil {
		return CreateUploadResult{}, err
	}

	metadata, err := json.Marshal(storedMetadata{
		FileName:     filepath.Base(input.FileName),
		ContentType:  input.ContentType,
		DeclaredSize: input.Size,
		Checksum:     input.Checksum,
	})
	if err != nil {
		return CreateUploadResult{}, err
	}
	now := service.now().UTC()
	asset := domain.AssetRecord{
		ID:        id,
		BlobKey:   blobKey,
		SourceURL: sourceURL,
		Status:    domain.AssetPending,
		Metadata:  metadata,
		Rights:    append(json.RawMessage(nil), input.Rights...),
		CreatedBy: actor.Subject,
		CreatedAt: now,
	}

	err = service.repository.WithinTransaction(ctx, func(repository Repository) error {
		if err := repository.CreateAsset(ctx, asset); err != nil {
			return err
		}
		return repository.AppendAudit(ctx, assetAudit(actor, "asset.create_upload", asset.ID, now))
	})
	if err != nil {
		return CreateUploadResult{}, err
	}
	return CreateUploadResult{Asset: asset, Upload: upload}, nil
}

func (service *Service) CompleteUpload(
	ctx context.Context,
	actor domain.Principal,
	id string,
) (domain.AssetRecord, error) {
	if !hasPermission(actor, auth.PermissionCreateAsset) {
		return domain.AssetRecord{}, domain.ErrForbidden
	}
	asset, err := service.repository.GetAsset(ctx, id)
	if err != nil {
		return domain.AssetRecord{}, err
	}
	if asset.Status == domain.AssetReady {
		return service.persistMissingReadySourceURL(ctx, asset)
	}
	if asset.Status != domain.AssetPending {
		return domain.AssetRecord{}, domain.ErrInvalidTransition
	}

	var expected storedMetadata
	if err := json.Unmarshal(asset.Metadata, &expected); err != nil {
		return domain.AssetRecord{}, fmt.Errorf("decode asset metadata: %w", err)
	}
	actual, err := service.blobStore.Stat(ctx, asset.BlobKey)
	if err != nil {
		return domain.AssetRecord{}, err
	}
	if actual.ContentType != expected.ContentType || actual.Size != expected.DeclaredSize {
		return domain.AssetRecord{}, domain.ErrInvalidInput
	}
	if expected.Checksum != "" && actual.Checksum != expected.Checksum {
		return domain.AssetRecord{}, domain.ErrInvalidInput
	}

	expected.Checksum = actual.Checksum
	expected.Width = actual.Width
	expected.Height = actual.Height
	expected.Duration = actual.Duration
	metadata, err := json.Marshal(expected)
	if err != nil {
		return domain.AssetRecord{}, err
	}
	asset.Metadata = metadata
	asset.Status = domain.AssetReady
	if asset.SourceURL == "" {
		asset.SourceURL, err = service.blobStore.PublicURL(ctx, asset.BlobKey)
		if err != nil {
			return domain.AssetRecord{}, err
		}
	}
	now := service.now().UTC()

	err = service.repository.WithinTransaction(ctx, func(repository Repository) error {
		current, err := repository.GetAsset(ctx, id)
		if err != nil {
			return err
		}
		if current.Status == domain.AssetReady {
			if current.SourceURL == "" {
				current.SourceURL = asset.SourceURL
				if err := repository.UpdateAsset(ctx, current, current.Status); err != nil {
					return err
				}
			}
			asset = current
			return nil
		}
		if current.Status != domain.AssetPending {
			return domain.ErrInvalidTransition
		}
		if current.SourceURL != "" {
			asset.SourceURL = current.SourceURL
		}
		if err := repository.UpdateAsset(ctx, asset, current.Status); err != nil {
			return err
		}
		return repository.AppendAudit(ctx, assetAudit(actor, "asset.complete_upload", asset.ID, now))
	})
	if err != nil {
		return domain.AssetRecord{}, err
	}
	return asset, nil
}

func (service *Service) persistMissingReadySourceURL(
	ctx context.Context,
	asset domain.AssetRecord,
) (domain.AssetRecord, error) {
	if asset.SourceURL != "" {
		return asset, nil
	}
	sourceURL, err := service.blobStore.PublicURL(ctx, asset.BlobKey)
	if err != nil {
		return domain.AssetRecord{}, err
	}
	err = service.repository.WithinTransaction(ctx, func(repository Repository) error {
		current, err := repository.GetAsset(ctx, asset.ID)
		if err != nil {
			return err
		}
		if current.Status != domain.AssetReady {
			return domain.ErrInvalidTransition
		}
		if current.SourceURL == "" {
			current.SourceURL = sourceURL
			if err := repository.UpdateAsset(ctx, current, current.Status); err != nil {
				return err
			}
		}
		asset = current
		return nil
	})
	return asset, err
}

func (service *Service) Delete(ctx context.Context, actor domain.Principal, id string) error {
	if !hasPermission(actor, auth.PermissionDeleteAsset) {
		return domain.ErrForbidden
	}
	asset, err := service.repository.GetAsset(ctx, id)
	if err != nil {
		return err
	}
	if asset.Status != domain.AssetDeleted {
		now := service.now().UTC()
		err = service.repository.WithinTransaction(ctx, func(repository Repository) error {
			current, err := repository.GetAsset(ctx, id)
			if err != nil {
				return err
			}
			asset = current
			if current.Status == domain.AssetDeleted {
				return nil
			}
			var metadata storedMetadata
			if err := json.Unmarshal(current.Metadata, &metadata); err != nil {
				return fmt.Errorf("decode asset metadata: %w", err)
			}
			if current.Status == domain.AssetReady {
				metadata.RetainBlob = true
				current.Metadata, err = json.Marshal(metadata)
				if err != nil {
					return err
				}
			}
			asset = current
			asset.Status = domain.AssetDeleted
			asset.DeletedAt = &now
			if err := repository.UpdateAsset(ctx, asset, current.Status); err != nil {
				return err
			}
			return repository.AppendAudit(ctx, assetAudit(actor, "asset.delete", asset.ID, now))
		})
		if err != nil {
			return err
		}
	}
	var metadata storedMetadata
	if err := json.Unmarshal(asset.Metadata, &metadata); err != nil {
		return fmt.Errorf("decode asset metadata: %w", err)
	}
	if metadata.RetainBlob {
		return nil
	}
	if err := service.blobStore.Delete(ctx, asset.BlobKey); err != nil && !errors.Is(err, domain.ErrNotFound) {
		return err
	}
	return nil
}

func validateUploadInput(input CreateUploadInput) (string, error) {
	rule, allowed := mediaRules[input.ContentType]
	if input.ContentType != strings.ToLower(input.ContentType) || !allowed || input.Size <= 0 || input.Size > rule.limit {
		return "", domain.ErrInvalidInput
	}
	if !validAssetRights(input.Rights) {
		return "", domain.ErrInvalidInput
	}
	extension := strings.ToLower(filepath.Ext(filepath.Base(input.FileName)))
	if extension != rule.extension {
		return "", domain.ErrInvalidInput
	}
	value := strings.TrimPrefix(input.Checksum, "sha256:")
	decoded, err := hex.DecodeString(value)
	if err != nil || len(decoded) != 32 || !strings.HasPrefix(input.Checksum, "sha256:") {
		return "", domain.ErrInvalidInput
	}
	return rule.extension, nil
}

func validAssetRights(raw json.RawMessage) bool {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var rights assetRights
	if decoder.Decode(&rights) != nil || decoder.Decode(&struct{}{}) != io.EOF || rights.Source == nil || rights.Source.ZhCN == "" {
		return false
	}
	return validOptionalString(rights.Source.En, true) && validOptionalString(rights.Credit, false) && validOptionalString(rights.License, false)
}

func validOptionalString(raw json.RawMessage, requireNonEmpty bool) bool {
	if len(raw) == 0 {
		return true
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return false
	}
	var value string
	if json.Unmarshal(raw, &value) != nil {
		return false
	}
	return !requireNonEmpty || value != ""
}

func hasPermission(actor domain.Principal, permission auth.Permission) bool {
	for _, role := range actor.Roles {
		if auth.Can(role, permission) {
			return true
		}
	}
	return false
}

func assetAudit(actor domain.Principal, action, id string, createdAt time.Time) domain.AuditEntry {
	return domain.AuditEntry{
		ActorSubject: actor.Subject,
		Action:       action,
		ResourceType: "asset",
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
