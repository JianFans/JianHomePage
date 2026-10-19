package postgres

import (
	"errors"
	"testing"
	"time"
	"yujian.me/server/internal/auth"
	"yujian.me/server/internal/content"
	"yujian.me/server/internal/domain"
	snapshotdata "yujian.me/server/internal/snapshot"
)

// cmsSnapshotValidator isolates storage orchestration from the separately
// tested full semantic validator while accepting canonical minimal fixtures.
type cmsSnapshotValidator struct{}

// Validate verifies that storage orchestration supplies canonical JSON.
func (cmsSnapshotValidator) Validate(value []byte) error {
	_, err := snapshotdata.CanonicalJSON(value)
	return err
}

// TestPostgresCMSLibraryAndSessions runs repeatable migrations, real JSONB
// cloning, keyset summaries, durable revocation and flow replay constraints.
func TestPostgresCMSLibraryAndSessions(t *testing.T) {
	db := assetIntegrationDatabase(t)
	ctx := t.Context()
	now := time.Now().UTC().Truncate(time.Microsecond)
	repo := NewContentRepository(db)
	raw := []byte(`{"schemaVersion":"1.0.0"}`)
	source := domain.ContentVersion{ID: "ver_4a1d69cc-c429-4747-bd2e-db2cf11cc6b3", Status: domain.StatusPublished, Revision: 4, Snapshot: raw, Checksum: snapshotdata.Checksum(raw), ReviewApproved: true, CreatedBy: "editor", UpdatedBy: "editor", CreatedAt: now.Add(-time.Second), UpdatedAt: now}
	if err := repo.CreateVersion(ctx, source); err != nil {
		t.Fatal(err)
	}
	got, err := repo.GetVersion(ctx, source.ID)
	if err != nil || got.Name != source.ID {
		t.Fatalf("legacy metadata fallback %#v %v", got, err)
	}
	service := content.NewService(content.ServiceOptions{Store: repo, Validator: cmsSnapshotValidator{}, Now: func() time.Time { return now }})
	actor := domain.Principal{Subject: "editor", Roles: []domain.Role{domain.RoleEditor}}
	clone, err := service.CloneVersion(ctx, actor, source.ID, "副本")
	if err != nil {
		t.Fatal(err)
	}
	if clone.SourceVersionID != source.ID || clone.Revision != 1 || clone.ReviewApproved || clone.Status != domain.StatusDraft {
		t.Fatalf("clone %#v", clone)
	}
	page, err := service.ListVersions(ctx, actor, content.ListOptions{Limit: 1})
	if err != nil || len(page.Items) != 1 || page.Items[0].ID != clone.ID || page.NextCursor == "" {
		t.Fatalf("first page %#v %v", page, err)
	}
	next, err := service.ListVersions(ctx, actor, content.ListOptions{Limit: 1, Cursor: page.NextCursor})
	if err != nil || len(next.Items) != 1 || next.Items[0].ID != source.ID || next.NextCursor != "" {
		t.Fatalf("next page %#v %v", next, err)
	}
	unchanged, err := repo.GetVersion(ctx, source.ID)
	if err != nil || unchanged.Revision != 4 || !unchanged.ReviewApproved || unchanged.Status != domain.StatusPublished {
		t.Fatalf("source mutated %#v %v", unchanged, err)
	}
	sessions := NewSessionStore(db)
	value := auth.Session{TokenHash: auth.SessionTokenHash("opaque"), Principal: auth.SessionPrincipal{Subject: "editor", Name: "编辑者", Picture: "https://media.example/avatar.jpg", Roles: []string{"editor"}}, CSRFToken: "csrf", ExpiresAt: now.Add(time.Hour)}
	if err := sessions.CreateSession(ctx, value); err != nil {
		t.Fatal(err)
	}
	session, err := sessions.GetSession(ctx, value.TokenHash)
	if err != nil || session.Principal.Subject != "editor" || session.Principal.Name != value.Principal.Name || session.Principal.Picture != value.Principal.Picture {
		t.Fatalf("persisted session %#v %v", session, err)
	}
	if err := sessions.DeleteSession(ctx, value.TokenHash); err != nil {
		t.Fatal(err)
	}
	if _, err := sessions.GetSession(ctx, value.TokenHash); !errors.Is(err, domain.ErrNotFound) {
		t.Fatal("revoked session restored")
	}
	value.TokenHash = auth.SessionTokenHash("expired")
	value.ExpiresAt = now.Add(-time.Second)
	if err := sessions.CreateSession(ctx, value); err != nil {
		t.Fatal(err)
	}
	if _, err := sessions.GetSession(ctx, value.TokenHash); !errors.Is(err, domain.ErrNotFound) {
		t.Fatal("expired session accepted")
	}
	if err := sessions.ConsumeLoginFlow(ctx, "state", now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := sessions.ConsumeLoginFlow(ctx, "state", now.Add(time.Minute)); !errors.Is(err, domain.ErrConflict) {
		t.Fatal("replayed login state accepted")
	}
}
