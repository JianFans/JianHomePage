package postgres

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"testing"
	"time"

	"yujian.me/server/internal/domain"
	"yujian.me/server/internal/identity"
	snapshotdata "yujian.me/server/internal/snapshot"
)

// legacyUUIDMigrationDatabase creates an isolated schema at migration 0008.
func legacyUUIDMigrationDatabase(t *testing.T) *Database {
	t.Helper()
	connection := os.Getenv("YUJIAN_TEST_POSTGRES_URL")
	if connection == "" {
		t.Skip("set YUJIAN_TEST_POSTGRES_URL to run real PostgreSQL migration checks")
	}
	base, err := Open(t.Context(), connection)
	if err != nil {
		t.Fatal("open integration database failed")
	}
	t.Cleanup(func() { _ = base.Close() })
	schema := fmt.Sprintf("uuid_migration_%x", time.Now().UnixNano())
	if _, err := base.ExecContext(t.Context(), "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := base.ExecContext(ctx, "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Error("clean integration schema failed")
		}
	})
	parsed, err := url.Parse(connection)
	if err != nil {
		t.Fatal("integration connection must be a PostgreSQL URL")
	}
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	database, err := Open(t.Context(), parsed.String())
	if err != nil {
		t.Fatal("open isolated integration schema failed")
	}
	t.Cleanup(func() { _ = database.Close() })

	tx, err := database.BeginTx(t.Context())
	if err != nil {
		t.Fatal("begin legacy schema setup failed")
	}
	for _, name := range []string{
		"0001_initial.sql",
		"0002_publish_job_identity.sql",
		"0003_publish_target_freeze.sql",
		"0004_asset_source_url.sql",
		"0005_asset_list_indexes.sql",
		"0006_admin_sessions.sql",
		"0007_version_library.sql",
		"0008_publisher_builds.sql",
	} {
		statement, err := migrationFiles.ReadFile("migrations/" + name)
		if err != nil {
			_ = tx.Rollback(context.Background())
			t.Fatalf("read legacy migration %s: %v", name, err)
		}
		if _, err := tx.ExecContext(t.Context(), string(statement)); err != nil {
			_ = tx.Rollback(context.Background())
			t.Fatalf("apply legacy migration %s: %v", name, err)
		}
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatal("commit legacy schema setup failed")
	}
	return database
}

// TestPostgresUUIDMigrationRejectsLegacyDataAndPreservesAudit verifies the
// upgrade boundary, transaction rollback, audit retention, and restart safety.
func TestPostgresUUIDMigrationRejectsLegacyDataAndPreservesAudit(t *testing.T) {
	database := legacyUUIDMigrationDatabase(t)
	ctx := t.Context()
	if _, err := database.ExecContext(ctx, `
INSERT INTO audit_log (actor_sub, action, resource_type, resource_id, metadata, created_at)
VALUES ('test', 'test.action', 'content', 'ver_legacy', '{}'::jsonb, now())`); err != nil {
		t.Fatal("insert legacy audit row: ", err)
	}
	snapshot := []byte(`{"schemaVersion":"1.0.0"}`)
	if _, err := database.ExecContext(ctx, `
INSERT INTO content_versions
	(id, status, revision, snapshot, checksum, review_approved, created_by, updated_by, created_at, updated_at, name)
VALUES ('ver_legacy', 'draft', 1, $1::jsonb, $2, false, 'test', 'test', now(), now(), 'legacy')`, string(snapshot), snapshotdata.Checksum(snapshot)); err != nil {
		t.Fatal("insert legacy content row: ", err)
	}

	if err := Migrate(ctx, database); err == nil {
		t.Fatal("UUID migration accepted existing business data")
	}
	var contentCount int
	var auditCount int
	var migrationApplied bool
	if err := database.QueryRowContext(ctx, `SELECT count(*) FROM content_versions WHERE id = 'ver_legacy'`).Scan(&contentCount); err != nil {
		t.Fatal("check retained content row: ", err)
	}
	if err := database.QueryRowContext(ctx, `SELECT count(*) FROM audit_log WHERE actor_sub = 'test'`).Scan(&auditCount); err != nil {
		t.Fatal("check retained audit row: ", err)
	}
	if err := database.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = '0009_uuid_ids')`).Scan(&migrationApplied); err != nil {
		t.Fatal("check failed migration marker: ", err)
	}
	if contentCount != 1 || auditCount != 1 || migrationApplied {
		t.Fatalf("failed migration changed data or marker: content=%d audit=%d applied=%v", contentCount, auditCount, migrationApplied)
	}
	if _, err := database.ExecContext(ctx, `DELETE FROM content_versions WHERE id = 'ver_legacy'`); err != nil {
		t.Fatal("remove isolated legacy content row: ", err)
	}
	if err := Migrate(ctx, database); err != nil {
		t.Fatalf("migrate empty business tables: %v", err)
	}

	var auditID string
	var auditIDType string
	if err := database.QueryRowContext(ctx, `SELECT id::text, pg_typeof(id)::text FROM audit_log WHERE actor_sub = 'test'`).Scan(&auditID, &auditIDType); err != nil {
		t.Fatal("read migrated audit row: ", err)
	}
	if auditIDType != "uuid" || !regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`).MatchString(auditID) {
		t.Fatalf("audit ID was not preserved as UUIDv4: type=%q id=%q", auditIDType, auditID)
	}

	if _, err := database.ExecContext(ctx, `
INSERT INTO content_versions
	(id, status, revision, snapshot, checksum, review_approved, created_by, updated_by, created_at, updated_at, name)
VALUES ('ver_550e8400-e29b-41d4-a716-446655440000', 'draft', 1, $1::jsonb, $2, false, 'test', 'test', now(), now(), 'current')`, string(snapshot), snapshotdata.Checksum(snapshot)); err != nil {
		t.Fatal("insert current content row: ", err)
	}
	if err := Migrate(ctx, database); err != nil {
		t.Fatalf("re-run migrations with current business data: %v", err)
	}
	var retainedAuditID string
	if err := database.QueryRowContext(ctx, `SELECT id::text FROM audit_log WHERE actor_sub = 'test'`).Scan(&retainedAuditID); err != nil || retainedAuditID != auditID {
		t.Fatalf("restart changed the migrated audit ID: %q %v", retainedAuditID, err)
	}
}

// TestPostgresUUIDPublishJobLifecycle exercises real SQL writes and reads with
// typed UUIDs, including the UUID default used for newly appended audit rows.
func TestPostgresUUIDPublishJobLifecycle(t *testing.T) {
	db := assetIntegrationDatabase(t)
	ctx := t.Context()
	now := time.Now().UTC().Truncate(time.Microsecond)
	raw := []byte(`{"schemaVersion":"1.0.0"}`)
	version := domain.ContentVersion{
		ID: identity.MustNew("ver_"), Status: domain.StatusDraft, Revision: 1,
		Snapshot: raw, Checksum: snapshotdata.Checksum(raw),
		CreatedBy: "test", UpdatedBy: "test", CreatedAt: now, UpdatedAt: now,
	}
	if err := NewContentRepository(db).CreateVersion(ctx, version); err != nil {
		t.Fatal("create UUID version: ", err)
	}
	repo := NewPublishRepository(db)
	job := domain.PublishJob{
		ID: identity.MustNew("pub_"), IdempotencyKey: "uuid-publish", Operation: domain.PublishOperationPublish,
		VersionID: version.ID, ReleaseID: identity.MustNew("rel_"), SnapshotKey: "snapshots/test.json",
		SnapshotChecksum: version.Checksum, TargetRevision: 1, Status: domain.PublishPending,
		RequestedBy: "test", CreatedAt: now, UpdatedAt: now,
	}
	if err := repo.CreatePublishJob(ctx, job); err != nil {
		t.Fatal("create UUID publish job: ", err)
	}
	job.BuildID = identity.MustNew("build_")
	job.Status = domain.PublishBuilding
	if err := repo.UpdatePublishJob(ctx, job, domain.PublishPending); err != nil {
		t.Fatal("update UUID publish job: ", err)
	}
	got, err := repo.GetPublishJob(ctx, job.ID)
	if err != nil || got.ID != job.ID || got.VersionID != version.ID || got.ReleaseID != job.ReleaseID || got.BuildID != job.BuildID || got.Status != domain.PublishBuilding {
		t.Fatalf("read UUID publish job: %#v %v", got, err)
	}
	if err := repo.AppendAudit(ctx, domain.AuditEntry{
		ActorSubject: "test", Action: "publish.test", ResourceType: "publish", ResourceID: job.ID,
		Metadata: []byte(`{}`), CreatedAt: now,
	}); err != nil {
		t.Fatal("append UUID audit row: ", err)
	}
	var auditID string
	if err := db.QueryRowContext(ctx, `SELECT id::text FROM audit_log WHERE resource_id = $1`, job.ID).Scan(&auditID); err != nil || !identity.Valid("audit_"+auditID, "audit_") {
		t.Fatalf("new audit default is not UUIDv4: %q %v", auditID, err)
	}
}
