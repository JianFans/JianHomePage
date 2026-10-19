package content_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
	"yujian.me/server/internal/content"
	"yujian.me/server/internal/domain"
	snapshotdata "yujian.me/server/internal/snapshot"
	"yujian.me/server/internal/store/memory"
)

type acceptingValidator struct{}

// Validate permits minimal valid JSON fixtures to isolate version orchestration.
func (acceptingValidator) Validate(value []byte) error {
	if !json.Valid(value) {
		return errors.New("invalid JSON")
	}
	return nil
}

// TestVersionListAndClonePreserveSource verifies keyset ordering, independent
// draft state, metadata, checksums and roles through the real memory repository.
func TestVersionListAndClonePreserveSource(t *testing.T) {
	ctx := context.Background()
	state := memory.NewState()
	repo := memory.NewContentRepository(state)
	now := time.Now().UTC().Truncate(time.Microsecond)
	ids := []string{
		"ver_550e8400-e29b-41d4-a716-446655440001",
		"ver_550e8400-e29b-41d4-a716-446655440002",
	}
	service := content.NewService(content.ServiceOptions{
		Store:     repo,
		Validator: acceptingValidator{},
		Now:       func() time.Time { return now },
		NewID: func(prefix string) string {
			id := ids[0]
			ids = ids[1:]
			return prefix + strings.TrimPrefix(id, "ver_")
		},
	})
	editor := domain.Principal{Subject: "editor", Roles: []domain.Role{domain.RoleEditor}}
	source, err := service.CreateNamedDraft(ctx, editor, " 首个版本 ", json.RawMessage(`{"schemaVersion":"1.0.0"}`))
	if err != nil {
		t.Fatal(err)
	}
	if source.Name != "首个版本" {
		t.Fatal("name not normalized")
	}
	approved := source
	approved.Status = domain.StatusPublished
	approved.Revision = 5
	approved.ReviewApproved = true
	repo.UpdateVersion(ctx, approved, 1)
	clone, err := service.CloneVersion(ctx, editor, source.ID, "副本")
	if err != nil {
		t.Fatal(err)
	}
	if clone.ID == source.ID || clone.Name != "副本" || clone.SourceVersionID != source.ID || clone.Status != domain.StatusDraft || clone.Revision != 1 || clone.ReviewApproved || clone.Checksum != source.Checksum {
		t.Fatalf("invalid clone %#v", clone)
	}
	clone.Snapshot[0] = 'x'
	stored, _ := repo.GetVersion(ctx, clone.ID)
	if !json.Valid(stored.Snapshot) {
		t.Fatal("clone response aliases storage")
	}
	unchanged, _ := repo.GetVersion(ctx, source.ID)
	if unchanged.Status != domain.StatusPublished || unchanged.Revision != 5 || !unchanged.ReviewApproved {
		t.Fatal("clone changed source")
	}
	page, err := service.ListVersions(ctx, editor, content.ListOptions{Limit: 1})
	if err != nil || len(page.Items) != 1 || page.Items[0].ID != stored.ID || page.NextCursor == "" {
		t.Fatalf("page %#v %v", page, err)
	}
	encoded, _ := json.Marshal(page)
	if strings.Contains(string(encoded), "snapshot") {
		t.Fatal("list leaks content snapshot")
	}
	page, err = service.ListVersions(ctx, editor, content.ListOptions{Limit: 1, Cursor: page.NextCursor})
	if err != nil || len(page.Items) != 1 || page.Items[0].ID != source.ID || page.NextCursor != "" {
		t.Fatalf("next page %#v %v", page, err)
	}
	for _, actor := range []domain.Principal{{Subject: "unknown", Roles: []domain.Role{"unknown"}}, {Roles: []domain.Role{domain.RoleEditor}}} {
		if _, err := service.ListVersions(ctx, actor, content.ListOptions{}); !errors.Is(err, domain.ErrForbidden) {
			t.Fatal("forbidden list accepted")
		}
		if _, err := service.CloneVersion(ctx, actor, source.ID, "clone"); !errors.Is(err, domain.ErrForbidden) {
			t.Fatal("forbidden clone accepted")
		}
	}
	reviewer := domain.Principal{Subject: "reviewer", Roles: []domain.Role{domain.RoleReviewer}}
	if _, err := service.ListVersions(ctx, reviewer, content.ListOptions{}); err != nil {
		t.Fatal("reviewer cannot list")
	}
	if _, err := service.CloneVersion(ctx, reviewer, source.ID, "clone"); !errors.Is(err, domain.ErrForbidden) {
		t.Fatal("reviewer cloned")
	}
	if _, err := service.CloneVersion(ctx, editor, "missing", "clone"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatal("missing clone accepted")
	}
	corrupt := unchanged
	corrupt.Checksum = "sha256:wrong"
	repo.UpdateVersion(ctx, corrupt, corrupt.Revision)
	if _, err := service.CloneVersion(ctx, editor, source.ID, "clone"); !errors.Is(err, domain.ErrConflict) {
		t.Fatal("corrupt clone accepted")
	}
	corrupt.Snapshot = json.RawMessage(`invalid`)
	corrupt.Checksum = snapshotdata.Checksum(corrupt.Snapshot)
	repo.UpdateVersion(ctx, corrupt, corrupt.Revision)
	if _, err := service.CloneVersion(ctx, editor, source.ID, "clone"); err == nil {
		t.Fatal("invalid snapshot cloned")
	}
	page, _ = service.ListVersions(ctx, editor, content.ListOptions{})
	if len(page.Items) != 2 {
		t.Fatal("failed clone created a version")
	}
	for _, options := range []content.ListOptions{{Limit: -1}, {Limit: 101}, {Cursor: "bad"}, {Cursor: strings.Repeat("A", 4096)}} {
		if _, err := service.ListVersions(ctx, editor, options); !errors.Is(err, domain.ErrInvalidInput) {
			t.Fatal("invalid paging accepted")
		}
	}
	if _, err := service.CreateNamedDraft(ctx, editor, strings.Repeat("长", 121), json.RawMessage(`{}`)); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatal("oversized name accepted")
	}
}
