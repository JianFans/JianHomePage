package postgres

import (
	"context"
	"strings"
	"testing"
	"time"
	"yujian.me/server/internal/content"
	"yujian.me/server/internal/domain"
	snapshotdata "yujian.me/server/internal/snapshot"
)

// TestVersionMetadataPersistenceAndSummaryQuery verifies SQL column order,
// metadata-only pagination and canonical checksum validation of full reads.
func TestVersionMetadataPersistenceAndSummaryQuery(t *testing.T) {
	now := time.Now().UTC()
	snapshot := []byte(`{"schemaVersion":"1.0.0"}`)
	exec := &recordingExecutor{result: recordingResult{affected: 1}, row: recordingRow{values: []any{"ver_new", "draft", int64(1), snapshot, snapshotdata.Checksum(snapshot), false, "editor", "editor", now, now, "新版本", "ver_source"}}}
	repo := NewContentRepository(exec)
	version, err := repo.GetVersion(context.Background(), "ver_new")
	if err != nil || version.Name != "新版本" || version.SourceVersionID != "ver_source" {
		t.Fatalf("metadata scan %#v %v", version, err)
	}
	if err := repo.CreateVersion(context.Background(), version); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(exec.execQueries[0], "source_version_id") || exec.execArgs[0][10] != "新版本" || exec.execArgs[0][11] != "ver_source" {
		t.Fatal("metadata not persisted")
	}
	exec.rows = &recordingRows{rows: []recordingRow{{values: []any{"ver_new", "新版本", "ver_source", "draft", int64(1), version.Checksum, false, now, now}}}}
	items, err := repo.ListVersions(context.Background(), content.ListQuery{Limit: 21, BeforeCreatedAt: &now, BeforeID: "ver_previous"})
	if err != nil || len(items) != 1 || items[0].Name != "新版本" {
		t.Fatalf("summary scan %#v %v", items, err)
	}
	query := exec.queries[0]
	if strings.Contains(query, "snapshot") || !strings.Contains(query, "ORDER BY created_at DESC, id DESC") || !strings.Contains(query, "(created_at,id)<($1,$2)") {
		t.Fatalf("unsafe list query %s", query)
	}
	if exec.queryArgs[0][1] != "ver_previous" || exec.queryArgs[0][2] != 21 {
		t.Fatal("paging arguments lost")
	}
	var _ domain.ContentVersion = version
}
