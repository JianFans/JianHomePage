package assets

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
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
	UpdateAsset(context.Context, domain.AssetRecord, domain.AssetStatus) error
	AppendAudit(context.Context, domain.AuditEntry) error
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
	Width        int           `json:"width,omitempty"`
	Height       int           `json:"height,omitempty"`
	Duration     time.Duration `json:"duration,omitempty"`
}

type mediaRule struct {
	extension string
	limit     int64
}

var mediaRules = map[string]mediaRule{
	"image/avif": {extension: ".avif", limit: imageLimit},
	"image/webp": {extension: ".webp", limit: imageLimit},
	"image/jpeg": {extension: ".jpg", limit: imageLimit},
	"audio/mpeg": {extension: ".mp3", limit: audioLimit},
	"audio/wav":  {extension: ".wav", limit: audioLimit},
	"video/mp4":  {extension: ".mp4", limit: videoLimit},
	"video/webm": {extension: ".webm", limit: videoLimit},
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
		repository: options.Repository,
		blobStore:  options.BlobStore,
		now:        now,
		newID:      newID,
	}
}

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
	now := service.now().UTC()

	err = service.repository.WithinTransaction(ctx, func(repository Repository) error {
		current, err := repository.GetAsset(ctx, id)
		if err != nil {
			return err
		}
		if current.Status != domain.AssetPending {
			return domain.ErrInvalidTransition
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
	if err := service.blobStore.Delete(ctx, asset.BlobKey); err != nil && !errors.Is(err, domain.ErrNotFound) {
		return err
	}
	return nil
}

func validateUploadInput(input CreateUploadInput) (string, error) {
	rule, allowed := mediaRules[strings.ToLower(input.ContentType)]
	if !allowed || input.Size <= 0 || input.Size > rule.limit {
		return "", domain.ErrInvalidInput
	}
	var rights map[string]json.RawMessage
	if len(input.Rights) == 0 || json.Unmarshal(input.Rights, &rights) != nil || rights == nil {
		return "", domain.ErrInvalidInput
	}
	extension := strings.ToLower(filepath.Ext(filepath.Base(input.FileName)))
	if input.ContentType == "image/jpeg" && extension == ".jpeg" {
		extension = ".jpg"
	}
	if extension != rule.extension {
		return "", domain.ErrInvalidInput
	}
	if input.Checksum != "" {
		value := strings.TrimPrefix(input.Checksum, "sha256:")
		decoded, err := hex.DecodeString(value)
		if err != nil || len(decoded) != 32 || !strings.HasPrefix(input.Checksum, "sha256:") {
			return "", domain.ErrInvalidInput
		}
	}
	return rule.extension, nil
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
