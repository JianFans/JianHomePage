package postgres

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"yujian.me/server/internal/domain"
	"yujian.me/server/internal/ports"
	"yujian.me/server/internal/providers/pages"
	"yujian.me/server/internal/publisher"
	snapshotdata "yujian.me/server/internal/snapshot"
)

type publisherIntegrationPages struct{ creates int }

// publisherIntegrationBuildInput shares canonical bytes between unit and database tests.
func publisherIntegrationBuildInput(t *testing.T) ([]byte, ports.BuildRequest) {
	t.Helper()
	raw, err := snapshotdata.CanonicalJSON([]byte(`{"releaseId":"rel_test","site":{"canonicalUrl":"https://site.example/"},"assets":[{"src":"https://media.example/image.webp"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	request := ports.BuildRequest{
		IdempotencyKey:   "opaque-key",
		ReleaseID:        "rel_test",
		SnapshotChecksum: snapshotdata.Checksum(raw),
	}
	request.SnapshotKey = "snapshots/" + request.ReleaseID + "/" + request.SnapshotChecksum + ".json"
	return raw, request
}

// TestPublisherIntegrationBuildInputIsCanonical validates the input before database setup.
func TestPublisherIntegrationBuildInputIsCanonical(t *testing.T) {
	raw, request := publisherIntegrationBuildInput(t)
	if _, _, err := validateStoredSnapshot(request.SnapshotKey, raw); err != nil {
		t.Fatalf("publisher integration input is not a valid stored snapshot: %v", err)
	}
}

// Deploy simulates an ambiguous successful HTTP submission whose response was lost.
func (p *publisherIntegrationPages) Deploy(context.Context, string, map[string][]byte) (pages.Deployment, error) {
	p.creates++
	return pages.Deployment{}, errors.New("connection closed after upload")
}

// Get returns a real production identity for explicit manual reconciliation.
func (*publisherIntegrationPages) Get(_ context.Context, project, id string) (pages.Deployment, error) {
	return pages.Deployment{ID: id, ProjectID: project, Environment: "Production", Status: "succeeded"}, nil
}

// TestPostgresPublisherRestartAndRecovery tests the real historical migration,
// global session lock, active-slot races and uncertain deployment recovery.
func TestPostgresPublisherRestartAndRecovery(t *testing.T) {
	db := assetIntegrationDatabase(t)
	ctx := t.Context()
	repo := NewPublisherRepository(db, db)
	if err := repo.Health(ctx); err != nil {
		t.Fatal(err)
	}
	raw, request := publisherIntegrationBuildInput(t)
	target := publisher.Target{ProjectID: "project", Region: "global", CodeRevision: strings.Repeat("a", 64), PublicURL: "https://site.example/"}
	cloud := &publisherIntegrationPages{}
	snapshots := NewSnapshotStore(db)
	if err := snapshots.PutSnapshot(ctx, request.SnapshotKey, raw); err != nil {
		t.Fatalf("persist immutable build input: %v", err)
	}
	options := publisher.Options{Repository: repo, Target: target, Pages: cloud, Generate: func(context.Context, []byte) (map[string][]byte, error) {
		return map[string][]byte{"index.html": []byte("rendered")}, nil
	}, ReadSnapshot: snapshots.ReadSnapshot}
	service, err := publisher.NewService(options)
	if err != nil {
		t.Fatal(err)
	}
	run, err := service.Trigger(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			retried, err := service.Trigger(ctx, request)
			if err != nil || retried.ID != run.ID {
				t.Errorf("idempotent race %#v %v", retried, err)
			}
		})
	}
	wg.Wait()
	other := request
	other.IdempotencyKey = "other-key"
	if _, err := service.Trigger(ctx, other); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("second active task accepted: %v", err)
	}
	if err := service.Reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	restarted, err := publisher.NewService(options)
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := restarted.Reconcile(ctx); err != nil {
			t.Fatal(err)
		}
	}
	task, err := restarted.Inspect(ctx, run.ID)
	if err != nil || task.Phase != "submitting" || cloud.creates != 1 {
		t.Fatalf("restart resubmitted %#v calls=%d %v", task, cloud.creates, err)
	}
	id := "deployment-verified"
	if _, err := restarted.Recover(ctx, run.ID, publisher.RecoveryInput{DeploymentID: &id, Note: "Confirmed in Pages console", ActorSubject: "admin-1"}); err != nil {
		t.Fatal(err)
	}
	if err := restarted.Reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	task, err = restarted.Inspect(ctx, run.ID)
	if err != nil || task.Phase != "succeeded" || len(task.Reconciliation) != 1 || task.PreviewURL != target.PublicURL {
		t.Fatalf("recovery not durable %#v %v", task, err)
	}
	if _, err := service.Trigger(ctx, other); err != nil {
		t.Fatalf("terminal task retained active slot: %v", err)
	}
}

// TestPostgresPublisherExclusiveLock proves another pool connection cannot run a worker concurrently.
func TestPostgresPublisherExclusiveLock(t *testing.T) {
	db := assetIntegrationDatabase(t)
	held := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- db.WithPublisherLock(t.Context(), func(Executor) error { close(held); <-release; return nil })
	}()
	<-held
	err := db.WithPublisherLock(t.Context(), func(Executor) error { t.Error("second worker acquired shared lock"); return nil })
	close(release)
	if err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	called := false
	if err := db.WithPublisherLock(t.Context(), func(Executor) error { called = true; return nil }); err != nil || !called {
		t.Fatalf("lock not released: %v", err)
	}
}

// TestPostgresPublisherLostSessionCannotMutate terminates only the test's pinned
// backend and proves database/sql cannot replace it with an unlocked connection.
func TestPostgresPublisherLostSessionCannotMutate(t *testing.T) {
	db := assetIntegrationDatabase(t)
	ctx := t.Context()
	repo := NewPublisherRepository(db, db)
	task, err := repo.Enqueue(ctx, "lost-session-key", ports.BuildRequest{ReleaseID: "rel_test"}, publisher.Target{ProjectID: "project"})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.WithPublisherLock(ctx, func(exec Executor) error {
		var pid int
		if err := exec.QueryRowContext(ctx, `SELECT pg_backend_pid()`).Scan(&pid); err != nil {
			return err
		}
		var killed bool
		if err := db.QueryRowContext(ctx, `SELECT pg_terminate_backend($1)`, pid).Scan(&killed); err != nil {
			return err
		}
		if !killed {
			t.Fatal("test backend was not terminated")
		}
		locked := NewPublisherRepository(exec, nil)
		next := task
		next.Phase = "submitting"
		if _, err := locked.Update(ctx, next, "queued"); err == nil {
			t.Fatal("dead lock session mutated task")
		}
		if _, err := locked.Update(ctx, next, "queued"); err == nil {
			t.Fatal("dead session reconnected without acquiring lock")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	stored, err := repo.Get(ctx, task.ID)
	if err != nil || stored.Phase != "queued" {
		t.Fatalf("task changed after lock loss %#v %v", stored, err)
	}
	called := false
	if err := db.WithPublisherLock(ctx, func(Executor) error { called = true; return nil }); err != nil || !called {
		t.Fatalf("dead backend retained lock: %v", err)
	}
}
