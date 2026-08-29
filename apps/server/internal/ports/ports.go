package ports

import (
	"context"
	"io"
	"time"

	"yujian.me/server/internal/domain"
)

type UploadRequest struct {
	BlobKey     string
	ContentType string
	Size        int64
	Checksum    string
	ExpiresIn   time.Duration
}

type SignedUpload struct {
	URL       string
	Headers   map[string]string
	ExpiresAt time.Time
}

type BlobMetadata struct {
	ContentType string
	Size        int64
	Checksum    string
	Width       int
	Height      int
	Duration    time.Duration
}

type BlobStore interface {
	CreateUpload(context.Context, UploadRequest) (SignedUpload, error)
	Stat(context.Context, string) (BlobMetadata, error)
	Put(context.Context, string, io.Reader, BlobMetadata) error
	Delete(context.Context, string) error
	SignedReadURL(context.Context, string, time.Duration) (string, error)
}

type BuildRequest struct {
	ReleaseID        string
	SnapshotKey      string
	SnapshotChecksum string
}

type BuildRun struct {
	ID         string
	Status     domain.PublishStatus
	PreviewURL string
	Error      string
}

type BuildTrigger interface {
	Trigger(context.Context, BuildRequest) (BuildRun, error)
	Status(context.Context, string) (BuildRun, error)
}

type PublishResult struct {
	Job       domain.PublishJob
	Pointer   domain.PublishPointer
	Succeeded bool
}

type Notifier interface {
	PublishCompleted(context.Context, PublishResult) error
}

type IdentityProvider interface {
	Authenticate(context.Context, string) (domain.Principal, error)
}

type Repository interface {
	CreateVersion(context.Context, domain.ContentVersion) error
	GetVersion(context.Context, string) (domain.ContentVersion, error)
	UpdateVersion(context.Context, domain.ContentVersion, int64) error
	CreateAsset(context.Context, domain.AssetRecord) error
	GetAsset(context.Context, string) (domain.AssetRecord, error)
	UpdateAsset(context.Context, domain.AssetRecord) error
	CreatePublishJob(context.Context, domain.PublishJob) error
	GetPublishJob(context.Context, string) (domain.PublishJob, error)
	GetPublishJobByIdempotencyKey(context.Context, string) (domain.PublishJob, error)
	UpdatePublishJob(context.Context, domain.PublishJob) error
	GetPublishPointer(context.Context, string) (domain.PublishPointer, error)
	SetPublishPointer(context.Context, domain.PublishPointer) error
	AppendAudit(context.Context, domain.AuditEntry) error
}

type Transactor interface {
	WithinTransaction(context.Context, func(Repository) error) error
}
