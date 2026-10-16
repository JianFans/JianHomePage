package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"yujian.me/server/internal/domain"
	"yujian.me/server/internal/ports"
	"yujian.me/server/internal/publisher"
)

// publisherRow reproduces JSONB's object-key order independent of struct encoding.
func publisherRow() recordingRow {
	return recordingRow{values: []any{"build_id", []byte(`{"snapshotChecksum":"sha256:abc","snapshotKey":"key","releaseId":"rel_test"}`), []byte(`{"publicUrl":"https://site.example/","codeRevision":"revision","region":"global","projectId":"project"}`), "submitting", "", "", "", []byte(`[]`), time.Date(2026, 10, 16, 0, 0, 0, 0, time.UTC)}}
}

// TestPublisherReadsHistoricalJSONB preserves camelCase identity from Node-created records.
func TestPublisherReadsHistoricalJSONB(t *testing.T) {
	exec := &recordingExecutor{row: publisherRow()}
	task, err := NewPublisherRepository(exec, nil).Get(t.Context(), "build_id")
	if err != nil || task.Target.ProjectID != "project" || task.Request.ReleaseID != "rel_test" || task.Phase != "submitting" || task.Reconciliation == nil {
		t.Fatalf("legacy JSON %#v %v", task, err)
	}
	if strings.Contains(exec.rowQueries[0], "FOR UPDATE") {
		t.Fatal("ordinary reads must not open locking transactions")
	}
}

// TestPublisherEnqueueIsIdempotentAcrossTargetChanges keeps the first configuration on retries.
func TestPublisherEnqueueIsIdempotentAcrossTargetChanges(t *testing.T) {
	exec := &recordingExecutor{row: publisherRow()}
	repo := NewPublisherRepository(exec, nil)
	r := ports.BuildRequest{ReleaseID: "rel_test", SnapshotKey: "key", SnapshotChecksum: "sha256:abc"}
	task, err := repo.Enqueue(t.Context(), "opaque-key", r, publisher.Target{ProjectID: "changed"})
	if err != nil || task.Target.ProjectID != "project" {
		t.Fatalf("retry lost frozen target %#v %v", task, err)
	}
	r.ReleaseID = "rel_other"
	if _, err := repo.Enqueue(t.Context(), "opaque-key", r, publisher.Target{}); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("changed body accepted: %v", err)
	}
}

// TestPublisherCASRejectsStalePhase does not mask a lost or competing state transition.
func TestPublisherCASRejectsStalePhase(t *testing.T) {
	exec := &recordingExecutor{row: recordingRow{err: sql.ErrNoRows}}
	_, err := NewPublisherRepository(exec, nil).Update(t.Context(), publisher.Task{ID: "build_id", Phase: "submitting"}, "building")
	if !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("expected conflict, got %v", err)
	}
	if !strings.Contains(exec.rowQueries[0], "WHERE id = $1 AND phase = $2") || exec.begin != nil {
		t.Fatal("transition must be a single autocommit CAS")
	}
}

type publisherTestLocker struct{ exec Executor }

// WithPublisherLock substitutes the physical pinned connection for routing checks.
func (l publisherTestLocker) WithPublisherLock(ctx context.Context, run func(Executor) error) error {
	return run(l.exec)
}

// TestPublisherLockRoutesAllWrites proves callbacks never escape to the unlocked pool.
func TestPublisherLockRoutesAllWrites(t *testing.T) {
	pool := &recordingExecutor{row: recordingRow{err: errors.New("wrong connection")}}
	pinned := &recordingExecutor{row: publisherRow()}
	repo := NewPublisherRepository(pool, publisherTestLocker{exec: pinned})
	if err := repo.WithLock(t.Context(), func(locked publisher.Repository) error { _, err := locked.Active(t.Context()); return err }); err != nil {
		t.Fatal(err)
	}
	if len(pool.rowQueries) != 0 || len(pinned.rowQueries) != 1 {
		t.Fatal("callback escaped lock connection")
	}
}

// TestPublisherTaskJSONUsesHistoricalKeys verifies nested request and target wire compatibility.
func TestPublisherTaskJSONUsesHistoricalKeys(t *testing.T) {
	data, err := json.Marshal(publisher.Task{Request: ports.BuildRequest{IdempotencyKey: "do-not-store", ReleaseID: "rel_test"}, Target: publisher.Target{ProjectID: "project"}})
	if err != nil || !strings.Contains(string(data), `"projectId":"project"`) || !strings.Contains(string(data), `"releaseId":"rel_test"`) || strings.Contains(string(data), "do-not-store") {
		t.Fatalf("wire keys %s %v", data, err)
	}
}
