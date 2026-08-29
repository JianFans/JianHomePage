package domain

import (
	"encoding/json"
	"errors"
	"strings"
	"time"
)

var (
	ErrNotFound          = errors.New("not found")
	ErrConflict          = errors.New("conflict")
	ErrForbidden         = errors.New("forbidden")
	ErrInvalidInput      = errors.New("invalid input")
	ErrInvalidTransition = errors.New("invalid state transition")
	ErrDuplicateRequest  = errors.New("duplicate request")
)

type Role string

const (
	RoleEditor    Role = "editor"
	RoleReviewer  Role = "reviewer"
	RolePublisher Role = "publisher"
	RoleAdmin     Role = "admin"
)

type Principal struct {
	Subject string
	Email   string
	Name    string
	Roles   []Role
}

type ContentStatus string

const (
	StatusDraft     ContentStatus = "draft"
	StatusInReview  ContentStatus = "in_review"
	StatusPublished ContentStatus = "published"
	StatusArchived  ContentStatus = "archived"
)

func CanTransition(from, to ContentStatus, reviewApproved bool) bool {
	switch {
	case from == StatusDraft && to == StatusInReview:
		return true
	case from == StatusInReview && to == StatusDraft:
		return true
	case from == StatusInReview && to == StatusPublished:
		return reviewApproved
	case from == StatusPublished && to == StatusArchived:
		return true
	default:
		return false
	}
}

type ContentVersion struct {
	ID             string
	Status         ContentStatus
	Revision       int64
	Snapshot       json.RawMessage
	Checksum       string
	ReviewApproved bool
	CreatedBy      string
	UpdatedBy      string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type AssetStatus string

const (
	AssetPending AssetStatus = "pending"
	AssetReady   AssetStatus = "ready"
	AssetDeleted AssetStatus = "deleted"
)

type AssetRecord struct {
	ID        string
	BlobKey   string
	Status    AssetStatus
	Metadata  json.RawMessage
	Rights    json.RawMessage
	CreatedBy string
	CreatedAt time.Time
	DeletedAt *time.Time
}

type PublishStatus string

type PublishOperation string

const (
	PublishPending   PublishStatus = "pending"
	PublishBuilding  PublishStatus = "building"
	PublishSucceeded PublishStatus = "succeeded"
	PublishFailed    PublishStatus = "failed"

	PublishOperationPublish  PublishOperation = "publish"
	PublishOperationRollback PublishOperation = "rollback"
)

func NormalizeIdempotencyKey(value string) (string, bool) {
	normalized := strings.TrimSpace(value)
	return normalized, len(normalized) >= 8 && len(normalized) <= 128
}

type PublishJob struct {
	ID               string
	IdempotencyKey   string
	Operation        PublishOperation
	VersionID        string
	ReleaseID        string
	SnapshotKey      string
	SnapshotChecksum string
	BuildID          string
	Status           PublishStatus
	ErrorMessage     string
	RequestedBy      string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type PublishPointer struct {
	Slot             string
	VersionID        string
	SnapshotKey      string
	SnapshotChecksum string
	UpdatedAt        time.Time
}

type AuditEntry struct {
	ActorSubject string
	Action       string
	ResourceType string
	ResourceID   string
	Metadata     json.RawMessage
	CreatedAt    time.Time
}
