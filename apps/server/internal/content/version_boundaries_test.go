package content_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"
	"time"
	"yujian.me/server/internal/content"
	"yujian.me/server/internal/domain"
	snapshotdata "yujian.me/server/internal/snapshot"
	"yujian.me/server/internal/store/memory"
)

// auditFailureRepository injects a real transactional audit failure without
// substituting clone logic, rollback or storage behavior.
type auditFailureRepository struct{ content.Repository }

// WithinTransaction preserves the underlying repository's rollback boundary.
func (repo auditFailureRepository) WithinTransaction(ctx context.Context, run func(content.Repository) error) error {
	return repo.Repository.WithinTransaction(ctx, func(tx content.Repository) error { return run(auditFailureRepository{Repository: tx}) })
}

// AppendAudit models a database error after creating the tentative draft.
func (auditFailureRepository) AppendAudit(context.Context, domain.AuditEntry) error {
	return errors.New("audit unavailable")
}

// TestCloneAuditFailureAndCursorValidation verifies transactional no-side-effect
// behavior and rejects malformed or ambiguous opaque cursor encodings.
func TestCloneAuditFailureAndCursorValidation(t *testing.T) {
	ctx := context.Background()
	repo := memory.NewContentRepository(memory.NewState())
	now := time.Now().UTC()
	raw := json.RawMessage(`{"schemaVersion":"1.0.0"}`)
	source := domain.ContentVersion{ID: "ver_source", Status: domain.StatusPublished, Revision: 3, Snapshot: raw, Checksum: snapshotdata.Checksum(raw), CreatedAt: now, UpdatedAt: now}
	repo.CreateVersion(ctx, source)
	service := content.NewService(content.ServiceOptions{Store: auditFailureRepository{Repository: repo}, Validator: acceptingValidator{}})
	editor := domain.Principal{Subject: "editor", Roles: []domain.Role{domain.RoleEditor}}
	if _, err := service.CloneVersion(ctx, editor, source.ID, "clone"); err == nil {
		t.Fatal("audit failure swallowed")
	}
	page, err := service.ListVersions(ctx, editor, content.ListOptions{})
	if err != nil || len(page.Items) != 1 || page.Items[0].Name != source.ID {
		t.Fatalf("failed clone left state: %#v %v", page, err)
	}
	timestamp := now.Format(time.RFC3339Nano)
	for _, raw := range []string{`{}`, `{"createdAt":"` + timestamp + `","id":"ver_a","unknown":true}`, `{"createdAt":"` + timestamp + `","id":"ver_a"} {}`, `{"createdAt":"` + timestamp + `","id":"invalid id"}`, `{"createdAt":"` + timestamp + `","id":"ver_a","id":"ver_b"}`} {
		cursor := base64.RawURLEncoding.EncodeToString([]byte(raw))
		if _, err := service.ListVersions(ctx, editor, content.ListOptions{Cursor: cursor}); !errors.Is(err, domain.ErrInvalidInput) {
			t.Fatalf("invalid cursor accepted: %s %v", raw, err)
		}
	}
}
