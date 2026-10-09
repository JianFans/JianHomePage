package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"sync"
	"testing"
	"time"

	"yujian.me/server/internal/domain"
)

// assetIntegrationDatabase isolates real migrations and repository checks in a
// fresh schema; it never reads or mutates existing application tables.
func assetIntegrationDatabase(t *testing.T) *Database {
	t.Helper()
	connection := os.Getenv("YUJIAN_TEST_POSTGRES_URL")
	if connection == "" {
		t.Skip("set YUJIAN_TEST_POSTGRES_URL to run real PostgreSQL asset checks")
	}
	base, err := Open(t.Context(), connection)
	if err != nil {
		t.Fatal("open integration database failed")
	}
	t.Cleanup(func() { _ = base.Close() })
	schema := fmt.Sprintf("asset_review_%x", time.Now().UnixNano())
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
	for range 2 {
		if err := Migrate(t.Context(), database); err != nil {
			t.Fatalf("run real migrations: %v", err)
		}
	}
	return database
}

// TestPostgresAssetSourceRepair preserves lifecycle fields and the winning URL
// across actual concurrent UPDATEs, including old-instance NULL records.
func TestPostgresAssetSourceRepair(t *testing.T) {
	database := assetIntegrationDatabase(t)
	repository := NewAssetRepository(database)
	deletedAt := time.Now().UTC().Truncate(time.Microsecond)
	asset := domain.AssetRecord{
		ID: "asset_legacy", BlobKey: "assets/legacy/source.webp", Status: domain.AssetDeleted,
		Metadata: json.RawMessage(`{"retainBlob":true}`), Rights: json.RawMessage(`{"source":{"zh-CN":"authorized"}}`),
		CreatedBy: "review", CreatedAt: deletedAt, DeletedAt: &deletedAt,
	}
	if err := repository.CreateAsset(t.Context(), asset); err != nil {
		t.Fatal(err)
	}
	var workers sync.WaitGroup
	results := make(chan string, 2)
	for _, candidate := range []string{"https://media-a.example/source.webp", "https://media-b.example/source.webp"} {
		workers.Go(func() {
			stored, err := repository.EnsureAssetSourceURL(t.Context(), asset.ID, candidate)
			if err != nil {
				t.Error(err)
			}
			results <- stored
		})
	}
	workers.Wait()
	close(results)
	stored, err := repository.GetAsset(t.Context(), asset.ID)
	if err != nil {
		t.Fatal(err)
	}
	for result := range results {
		if result == "" || result != stored.SourceURL {
			t.Fatal("concurrent repair changed the frozen source URL")
		}
	}
	var metadata map[string]bool
	if err := json.Unmarshal(stored.Metadata, &metadata); err != nil {
		t.Fatal(err)
	}
	if stored.Status != asset.Status || stored.DeletedAt == nil || !stored.DeletedAt.Equal(deletedAt) || !metadata["retainBlob"] {
		t.Fatalf("repair changed lifecycle fields: %#v", stored)
	}
}
