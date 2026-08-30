package memory

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"yujian.me/server/internal/assets"
	"yujian.me/server/internal/content"
	"yujian.me/server/internal/domain"
	"yujian.me/server/internal/publish"
)

func TestRepositoriesRejectCancelledTransactions(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	state := NewState()

	checks := []struct {
		name string
		run  func() error
	}{
		{name: "content", run: func() error {
			return NewContentRepository(state).WithinTransaction(ctx, func(content.Repository) error { return nil })
		}},
		{name: "asset", run: func() error {
			return NewAssetRepository(state).WithinTransaction(ctx, func(assets.Repository) error { return nil })
		}},
		{name: "publish", run: func() error {
			return NewPublishRepository(state).WithinTransaction(ctx, func(publish.Repository) error { return nil })
		}},
	}
	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			if err := check.run(); !errors.Is(err, context.Canceled) {
				t.Fatalf("expected cancelled transaction, got %v", err)
			}
		})
	}
}

func TestAssetRepositoryLifecycleAndCloneIsolation(t *testing.T) {
	state := NewState()
	repository := NewAssetRepository(state)
	asset := domain.AssetRecord{
		ID: "asset_1", Status: domain.AssetPending,
		Metadata: json.RawMessage(`{"kind":"image"}`), Rights: json.RawMessage(`{"holder":"artist"}`),
	}

	if err := repository.WithinTransaction(t.Context(), func(tx assets.Repository) error {
		if err := tx.CreateAsset(t.Context(), asset); err != nil {
			return err
		}
		loaded, err := tx.GetAsset(t.Context(), asset.ID)
		if err != nil {
			return err
		}
		loaded.Status = domain.AssetReady
		if err := tx.UpdateAsset(t.Context(), loaded, domain.AssetPending); err != nil {
			return err
		}
		return tx.AppendAudit(t.Context(), domain.AuditEntry{ResourceID: asset.ID, Metadata: json.RawMessage(`{"ok":true}`)})
	}); err != nil {
		t.Fatalf("asset transaction: %v", err)
	}

	loaded, err := repository.GetAsset(t.Context(), asset.ID)
	if err != nil || loaded.Status != domain.AssetReady {
		t.Fatalf("unexpected asset %#v err=%v", loaded, err)
	}
	loaded.Metadata[0] = '['
	again, err := repository.GetAsset(t.Context(), asset.ID)
	if err != nil || string(again.Metadata) != `{"kind":"image"}` {
		t.Fatalf("stored metadata was not isolated: %s err=%v", again.Metadata, err)
	}

	if err := repository.CreateAsset(t.Context(), asset); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("expected duplicate conflict, got %v", err)
	}
	if _, err := repository.GetAsset(t.Context(), "missing"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected missing asset, got %v", err)
	}
	if err := repository.UpdateAsset(t.Context(), loaded, domain.AssetPending); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("expected status conflict, got %v", err)
	}
	if err := repository.UpdateAsset(t.Context(), domain.AssetRecord{ID: "missing"}, domain.AssetPending); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected missing update, got %v", err)
	}
	if len(state.audits) != 1 || string(state.audits[0].Metadata) != `{"ok":true}` {
		t.Fatalf("unexpected audits %#v", state.audits)
	}
}

func TestPublishRepositoryLifecycleAndIndexes(t *testing.T) {
	state := NewState()
	contentRepository := NewContentRepository(state)
	assetRepository := NewAssetRepository(state)
	repository := NewPublishRepository(state)
	version := domain.ContentVersion{ID: "ver_1", Status: domain.StatusInReview, Revision: 2, Snapshot: json.RawMessage(`{"schemaVersion":"1.0.0"}`)}
	asset := domain.AssetRecord{ID: "asset_1", Status: domain.AssetReady, Metadata: json.RawMessage(`{"kind":"image"}`)}
	if err := contentRepository.CreateVersion(t.Context(), version); err != nil {
		t.Fatalf("seed version: %v", err)
	}
	if err := assetRepository.CreateAsset(t.Context(), asset); err != nil {
		t.Fatalf("seed asset: %v", err)
	}

	job := domain.PublishJob{ID: "pub_1", IdempotencyKey: "publish-key", VersionID: version.ID, Status: domain.PublishPending}
	if err := repository.WithinTransaction(t.Context(), func(tx publish.Repository) error {
		if err := tx.CreatePublishJob(t.Context(), job); err != nil {
			return err
		}
		if _, err := tx.GetVersion(t.Context(), version.ID); err != nil {
			return err
		}
		if _, err := tx.GetAsset(t.Context(), asset.ID); err != nil {
			return err
		}
		return tx.AppendAudit(t.Context(), domain.AuditEntry{ResourceID: job.ID})
	}); err != nil {
		t.Fatalf("publish transaction: %v", err)
	}

	if _, err := repository.GetPublishJob(t.Context(), job.ID); err != nil {
		t.Fatalf("get job: %v", err)
	}
	if found, err := repository.GetPublishJobByIdempotencyKey(t.Context(), job.IdempotencyKey); err != nil || found.ID != job.ID {
		t.Fatalf("get idempotent job: %#v err=%v", found, err)
	}
	if active, err := repository.GetActivePublishJob(t.Context()); err != nil || active.ID != job.ID {
		t.Fatalf("get active job: %#v err=%v", active, err)
	}
	if _, err := repository.GetSuccessfulPublishByVersion(t.Context(), version.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected no successful job, got %v", err)
	}

	if err := repository.CreatePublishJob(t.Context(), job); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("expected duplicate job conflict, got %v", err)
	}
	duplicateKey := job
	duplicateKey.ID = "pub_2"
	if err := repository.CreatePublishJob(t.Context(), duplicateKey); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("expected idempotency conflict, got %v", err)
	}
	if err := repository.UpdatePublishJob(t.Context(), domain.PublishJob{ID: "missing"}, domain.PublishPending); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected missing job, got %v", err)
	}
	if err := repository.UpdatePublishJob(t.Context(), job, domain.PublishBuilding); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("expected status conflict, got %v", err)
	}

	job.Status = domain.PublishSucceeded
	if err := repository.UpdatePublishJob(t.Context(), job, domain.PublishPending); err != nil {
		t.Fatalf("succeed job: %v", err)
	}
	if successful, err := repository.GetSuccessfulPublishByVersion(t.Context(), version.ID); err != nil || successful.ID != job.ID {
		t.Fatalf("get successful job: %#v err=%v", successful, err)
	}
	if _, err := repository.GetActivePublishJob(t.Context()); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected no active job, got %v", err)
	}

	if _, err := repository.GetPublishPointer(t.Context(), "production"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected missing pointer, got %v", err)
	}
	pointer := domain.PublishPointer{Slot: "production", VersionID: version.ID}
	if err := repository.SetPublishPointer(t.Context(), pointer); err != nil {
		t.Fatalf("set pointer: %v", err)
	}
	if found, err := repository.GetPublishPointer(t.Context(), pointer.Slot); err != nil || found.VersionID != version.ID {
		t.Fatalf("get pointer: %#v err=%v", found, err)
	}
	if err := repository.LockPublishSlot(t.Context(), pointer.Slot); err != nil {
		t.Fatalf("lock slot: %v", err)
	}
}
