package postgres

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"slices"
	"sync"
	"testing"
	"time"

	"yujian.me/server/internal/assets"
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

// TestPostgresAssetListRejectsNULCursor rejects invalid PostgreSQL text at the
// service boundary instead of exposing a database encoding failure to the API.
func TestPostgresAssetListRejectsNULCursor(t *testing.T) {
	database := assetIntegrationDatabase(t)
	service := assets.NewService(assets.ServiceOptions{Repository: NewAssetRepository(database)})
	cursor := base64.RawURLEncoding.EncodeToString([]byte(`{"createdAt":"2026-10-09T00:00:00Z","id":"asset_\u0000"}`))
	_, err := service.List(t.Context(), domain.Principal{
		Subject: "editor-1", Roles: []domain.Role{domain.RoleEditor},
	}, assets.ListOptions{Cursor: cursor})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("expected invalid cursor before PostgreSQL query, got %v", err)
	}
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

// TestPostgresAssetTransactionSerializesSourceRepair exercises the read/update
// window shared by upload completion and deletion against a concurrent repair.
func TestPostgresAssetTransactionSerializesSourceRepair(t *testing.T) {
	database := assetIntegrationDatabase(t)
	repository := NewAssetRepository(database)
	for _, transition := range []struct{ initial, target domain.AssetStatus }{
		{domain.AssetPending, domain.AssetReady}, {domain.AssetReady, domain.AssetReady},
		{domain.AssetPending, domain.AssetDeleted}, {domain.AssetReady, domain.AssetDeleted},
	} {
		t.Run(string(transition.initial)+"_to_"+string(transition.target), func(t *testing.T) {
			id := "asset_" + string(transition.initial) + "_" + string(transition.target)
			asset := domain.AssetRecord{
				ID: id, BlobKey: "assets/" + id + "/source.webp", Status: transition.initial,
				Metadata: json.RawMessage(`{}`), Rights: json.RawMessage(`{}`),
				CreatedBy: "review", CreatedAt: time.Now().UTC(),
			}
			if err := repository.CreateAsset(t.Context(), asset); err != nil {
				t.Fatal(err)
			}
			const frozenURL = "https://media-a.example/source.webp"
			const competingURL = "https://media-b.example/source.webp"
			err := repository.WithinTransaction(t.Context(), func(tx assets.Repository) error {
				current, err := tx.GetAsset(t.Context(), asset.ID)
				if err != nil {
					return err
				}
				// A different connection must wait until this read/update transaction
				// finishes instead of freezing a URL that the stale update overwrites.
				ctx, cancel := context.WithTimeout(t.Context(), 150*time.Millisecond)
				_, repairErr := repository.EnsureAssetSourceURL(ctx, asset.ID, competingURL)
				cancel()
				if repairErr == nil || !errors.Is(ctx.Err(), context.DeadlineExceeded) {
					return fmt.Errorf("repair was not serialized with transaction: %v", repairErr)
				}
				current.SourceURL, current.Status = frozenURL, transition.target
				if transition.target == domain.AssetDeleted {
					deletedAt := time.Now().UTC()
					current.DeletedAt = &deletedAt
				}
				return tx.UpdateAsset(t.Context(), current, transition.initial)
			})
			if err != nil {
				t.Fatal(err)
			}
			winner, err := repository.EnsureAssetSourceURL(t.Context(), asset.ID, competingURL)
			if err != nil || winner != frozenURL {
				t.Fatalf("repair replaced the transaction's frozen URL: %q, %v", winner, err)
			}
			stored, err := repository.GetAsset(t.Context(), asset.ID)
			if err != nil || stored.SourceURL != frozenURL || stored.Status != transition.target ||
				(transition.target == domain.AssetDeleted && stored.DeletedAt == nil) {
				t.Fatalf("unexpected committed asset: %#v, %v", stored, err)
			}
		})
	}
}

// listPlanExecutor records the actual repository query so EXPLAIN checks cannot
// silently verify a hand-written query that differs from production.
type listPlanExecutor struct {
	Executor
	query string
	args  []any
}

func (executor *listPlanExecutor) QueryContext(ctx context.Context, query string, args ...any) (Rows, error) {
	executor.query, executor.args = query, args
	return executor.Executor.QueryContext(ctx, query, args...)
}

type assetQueryPlan struct {
	NodeType     string           `json:"Node Type"`
	IndexName    string           `json:"Index Name"`
	ActualRows   float64          `json:"Actual Rows"`
	FilteredRows float64          `json:"Rows Removed by Filter"`
	Plans        []assetQueryPlan `json:"Plans"`
}

// TestPostgresAssetListUsesBoundedIndexPlans exercises first and deep pages for
// all filters on 100,000 records, with real query plans and pagination results.
func TestPostgresAssetListUsesBoundedIndexPlans(t *testing.T) {
	database := assetIntegrationDatabase(t)
	_, err := database.ExecContext(t.Context(), `
INSERT INTO assets (id, blob_key, source_url, status, metadata, rights, created_by, created_at)
SELECT 'asset_' || lpad(g::text, 6, '0'), 'assets/' || g || '/source.webp',
       'https://media.example/assets/' || g || '/source.webp',
       CASE g % 3 WHEN 0 THEN 'ready' WHEN 1 THEN 'pending' ELSE 'deleted' END,
       '{}'::jsonb, '{"source":{"zh-CN":"authorized"}}'::jsonb, 'review',
       '2026-01-01T00:00:00Z'::timestamptz + (g / 10) * interval '1 second'
FROM generate_series(1, 100000) g`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.ExecContext(t.Context(), "ANALYZE assets"); err != nil {
		t.Fatal(err)
	}
	for _, statuses := range [][]domain.AssetStatus{
		{domain.AssetPending, domain.AssetReady}, {domain.AssetReady}, {domain.AssetPending}, {domain.AssetDeleted},
	} {
		for _, deep := range []bool{false, true} {
			t.Run(fmt.Sprintf("%v/deep=%v", statuses, deep), func(t *testing.T) {
				executor := &listPlanExecutor{Executor: database}
				query := assets.ListQuery{Statuses: statuses, Limit: 51}
				if deep {
					boundary := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).Add(1000 * time.Second)
					query.BeforeCreatedAt, query.BeforeID = &boundary, "asset_010005"
				}
				items, err := NewAssetRepository(executor).ListAssets(t.Context(), query)
				if err != nil || len(items) != query.Limit {
					t.Fatalf("list length=%d error=%v", len(items), err)
				}
				for index, item := range items {
					if !slices.Contains(statuses, item.Status) || (query.BeforeCreatedAt != nil &&
						(item.CreatedAt.After(*query.BeforeCreatedAt) || item.CreatedAt.Equal(*query.BeforeCreatedAt) && item.ID >= query.BeforeID)) {
						t.Fatal("returned record outside filter or cursor")
					}
					if index > 0 && (item.CreatedAt.After(items[index-1].CreatedAt) || item.CreatedAt.Equal(items[index-1].CreatedAt) && item.ID >= items[index-1].ID) {
						t.Fatal("page order is not strictly descending")
					}
				}
				var raw []byte
				if err := database.QueryRowContext(t.Context(), "EXPLAIN (ANALYZE, BUFFERS, FORMAT JSON) "+executor.query, executor.args...).Scan(&raw); err != nil {
					t.Fatal(err)
				}
				var explained []struct {
					Plan          assetQueryPlan
					ExecutionTime float64 `json:"Execution Time"`
				}
				if err := json.Unmarshal(raw, &explained); err != nil || len(explained) != 1 {
					t.Fatalf("decode plan: %v", err)
				}
				expectedIndexes := []string{"assets_status_created_id_idx"}
				if len(statuses) == 2 {
					expectedIndexes = []string{"assets_active_created_id_idx"}
				} else if statuses[0] != domain.AssetDeleted {
					// A custom plan may prefer the smaller active index. Both are
					// valid if scanned rows remain bounded independently of depth.
					expectedIndexes = append(expectedIndexes, "assets_active_created_id_idx")
				}
				if !checkAssetIndexPlan(t, explained[0].Plan, expectedIndexes, query.Limit) {
					t.Fatalf("query did not use %v", expectedIndexes)
				}
				t.Logf("execution=%.3fms", explained[0].ExecutionTime)
			})
		}
	}
}

// checkAssetIndexPlan rejects full scans and sorts anywhere below the LIMIT.
func checkAssetIndexPlan(t *testing.T, plan assetQueryPlan, expectedIndexes []string, limit int) bool {
	t.Helper()
	if plan.NodeType == "Seq Scan" || plan.NodeType == "Sort" || plan.NodeType == "Incremental Sort" {
		t.Errorf("unbounded %s in asset list plan", plan.NodeType)
	}
	if plan.ActualRows+plan.FilteredRows > float64(limit*4) {
		t.Errorf("%s scanned too many rows: %.0f", plan.NodeType, plan.ActualRows+plan.FilteredRows)
	}
	found := slices.Contains(expectedIndexes, plan.IndexName)
	for _, child := range plan.Plans {
		found = checkAssetIndexPlan(t, child, expectedIndexes, limit) || found
	}
	return found
}
