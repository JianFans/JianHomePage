package postgres

import (
	"strings"
	"testing"
)

// TestPostgresSnapshotMigrationRequiresIdlePublisher verifies first-upgrade rejection,
// transactional rollback, safe retry, and subsequent restarts with active builds.
func TestPostgresSnapshotMigrationRequiresIdlePublisher(t *testing.T) {
	for _, phase := range []string{"queued", "building", "submitting", "deployed", "succeeded", "failed", "empty"} {
		t.Run(phase, func(t *testing.T) {
			db := assetIntegrationDatabase(t)
			ctx := t.Context()
			// Recreate the schema state immediately before the snapshot-store upgrade.
			if _, err := db.ExecContext(ctx, `
DELETE FROM schema_migrations WHERE version = '0009_publish_snapshots';
DROP TABLE publish_snapshots`); err != nil {
				t.Fatal(err)
			}
			if phase != "empty" {
				if _, err := db.ExecContext(ctx, `
INSERT INTO publisher_builds (id, idempotency_key, request, target, phase, deployment_id)
VALUES ('build_legacy', 'legacy-key', '{}'::jsonb, '{}'::jsonb, $1,
        CASE WHEN $1 IN ('deployed', 'succeeded') THEN 'deployment-legacy' ELSE NULL END)`, phase); err != nil {
					t.Fatal(err)
				}
			}
			active := phase == "queued" || phase == "building" || phase == "submitting" || phase == "deployed"
			err := Migrate(ctx, db)
			if active {
				if err == nil || !strings.Contains(err.Error(), "cannot upgrade publish snapshot storage with active publisher builds") {
					t.Fatalf("expected active-build migration rejection, got %v", err)
				}
			} else if err != nil {
				t.Fatalf("idle publisher migration: %v", err)
			}
			var applied, tableExists bool
			if err := db.QueryRowContext(ctx, `
SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = '0009_publish_snapshots'),
       EXISTS (SELECT 1 FROM information_schema.tables
               WHERE table_schema = current_schema() AND table_name = 'publish_snapshots')`).Scan(&applied, &tableExists); err != nil {
				t.Fatal(err)
			}
			if applied != !active || tableExists != !active {
				t.Fatalf("unexpected migration state: applied=%v table=%v active=%v", applied, tableExists, active)
			}
			if phase != "empty" {
				var storedPhase string
				if err := db.QueryRowContext(ctx, `SELECT phase FROM publisher_builds WHERE id = 'build_legacy'`).Scan(&storedPhase); err != nil || storedPhase != phase {
					t.Fatalf("migration altered legacy task: phase=%q err=%v", storedPhase, err)
				}
				if _, err := db.ExecContext(ctx, `UPDATE publisher_builds SET phase = 'failed' WHERE id = 'build_legacy'`); err != nil {
					t.Fatal(err)
				}
			}
			if err := Migrate(ctx, db); err != nil {
				t.Fatalf("retry after old task completes: %v", err)
			}
			if _, err := db.ExecContext(ctx, `
INSERT INTO publisher_builds (id, idempotency_key, request, target, phase)
VALUES ('build_new', 'new-key', '{}'::jsonb, '{}'::jsonb, 'queued')`); err != nil {
				t.Fatal(err)
			}
			if err := Migrate(ctx, db); err != nil {
				t.Fatalf("already-applied migration blocked normal restart: %v", err)
			}
		})
	}
}

// TestPostgresSnapshotMigrationRejectsPendingJobs covers the window before a
// publisher build exists, preserving terminal history and post-upgrade restarts.
func TestPostgresSnapshotMigrationRejectsPendingJobs(t *testing.T) {
	for _, operation := range []string{"publish", "rollback"} {
		for _, status := range []string{"pending", "succeeded", "failed"} {
			t.Run(operation+"/"+status, func(t *testing.T) {
				db := assetIntegrationDatabase(t)
				ctx := t.Context()
				if _, err := db.ExecContext(ctx, `
DELETE FROM schema_migrations WHERE version = '0009_publish_snapshots';
DROP TABLE publish_snapshots;
INSERT INTO content_versions
  (id, status, revision, snapshot, checksum, created_by, updated_by, created_at, updated_at)
VALUES ('ver_legacy', 'published', 1, '{"releaseId":"rel_legacy"}'::jsonb,
        'sha256:' || repeat('a', 64), 'test', 'test', now(), now())`); err != nil {
					t.Fatal(err)
				}
				if _, err := db.ExecContext(ctx, `
INSERT INTO publish_jobs
  (id, idempotency_key, operation, version_id, release_id, snapshot_key,
   snapshot_checksum, target_revision, status, requested_by, created_at, updated_at)
VALUES ('job_legacy', 'legacy-key', $1, 'ver_legacy', 'rel_legacy',
        'snapshots/legacy.json', 'sha256:' || repeat('a', 64), 1, $2, 'test', now(), now())`, operation, status); err != nil {
					t.Fatal(err)
				}
				err := Migrate(ctx, db)
				pending := status == "pending"
				if pending && (err == nil || !strings.Contains(err.Error(), "cannot upgrade publish snapshot storage")) {
					t.Fatalf("expected pending-job migration rejection without a build, got %v", err)
				}
				if !pending && err != nil {
					t.Fatalf("terminal job blocked migration: %v", err)
				}
				var applied, tableExists bool
				var storedStatus string
				if err := db.QueryRowContext(ctx, `
SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = '0009_publish_snapshots'),
       EXISTS (SELECT 1 FROM information_schema.tables
               WHERE table_schema = current_schema() AND table_name = 'publish_snapshots'),
       (SELECT status FROM publish_jobs WHERE id = 'job_legacy')`).Scan(&applied, &tableExists, &storedStatus); err != nil {
					t.Fatal(err)
				}
				if applied != !pending || tableExists != !pending || storedStatus != status {
					t.Fatalf("unexpected migration state: applied=%v table=%v status=%q", applied, tableExists, storedStatus)
				}
				if _, err := db.ExecContext(ctx, `UPDATE publish_jobs SET status = 'failed' WHERE id = 'job_legacy'`); err != nil {
					t.Fatal(err)
				}
				if err := Migrate(ctx, db); err != nil {
					t.Fatalf("retry after pending job completes: %v", err)
				}
				if _, err := db.ExecContext(ctx, `UPDATE publish_jobs SET status = 'pending' WHERE id = 'job_legacy'`); err != nil {
					t.Fatal(err)
				}
				if err := Migrate(ctx, db); err != nil {
					t.Fatalf("already-applied migration blocked pending job restart: %v", err)
				}
			})
		}
	}
}
