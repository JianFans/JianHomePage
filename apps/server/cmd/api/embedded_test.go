package main

import (
	"context"
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"yujian.me/server/internal/ports"
	"yujian.me/server/internal/providers/local"
	"yujian.me/server/internal/providers/pages"
	providerS3 "yujian.me/server/internal/providers/s3"
	"yujian.me/server/internal/publisher"
)

// TestEmbeddedManagementUsesTheAPIServer verifies page and API sharing one handler.
func TestEmbeddedManagementUsesTheAPIServer(t *testing.T) {
	dependencies := developmentDependencies()
	t.Cleanup(func() { _ = dependencies.Close() })
	dependencies.AdminFiles = managementFiles()
	handler, err := buildHandler(productionSettings(), dependencies)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		path, body string
		status     int
	}{
		{"/", "management", 200}, {"/preview", "preview", 200},
		{"/api/v1/missing", "Authentication required", 401}, {"/healthz", "ok", 200},
	} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, tc.path, nil))
		if response.Code != tc.status || !strings.Contains(response.Body.String(), tc.body) {
			t.Fatalf("%s: %d %s", tc.path, response.Code, response.Body.String())
		}
	}
}

// TestProductionRejectsIncompleteEmbeddedAssets verifies failure closes the DB.
func TestProductionRejectsIncompleteEmbeddedAssets(t *testing.T) {
	for _, boundary := range []string{"renderer", "admin"} {
		t.Run(boundary, func(t *testing.T) {
			database := &productionDatabaseFake{}
			sentinel := errors.New("missing built resources")
			factory := productionFactory{
				openDatabase: func(context.Context, string) (productionDatabase, error) { return database, nil },
				newBlobStore: func(providerS3.Config) (ports.BlobStore, error) { return local.NewBlobStore(), nil },
				newPages:     func(config pages.Config) (publisher.Pages, error) { return pages.NewClient(config) },
				newGenerator: func() (siteGenerator, error) {
					if boundary == "renderer" {
						return nil, sentinel
					}
					return generatorFake{}, nil
				},
				newAdmin: func() (fs.FS, error) { return nil, sentinel },
			}
			if _, _, err := buildProductionDependencies(t.Context(), productionSettings(), factory); !errors.Is(err, sentinel) {
				t.Fatalf("error=%v", err)
			}
			if !database.closed {
				t.Fatal("database leaked")
			}
		})
	}
}

// TestReconciliationRunsWorkerBeforePublication preserves task completion ordering.
func TestReconciliationRunsWorkerBeforePublication(t *testing.T) {
	var calls []string
	workerErr := errors.New("worker unavailable")
	chain := serviceReconcilers{worker: reconcilerFunc(func(context.Context) error { calls = append(calls, "worker"); return workerErr }), publish: reconcilerFunc(func(context.Context) error { calls = append(calls, "publish"); return nil })}
	if err := chain.Reconcile(t.Context()); !errors.Is(err, workerErr) {
		t.Fatalf("error=%v", err)
	}
	if strings.Join(calls, ",") != "worker,publish" {
		t.Fatalf("order=%v", calls)
	}
	if err := (serviceReconcilers{}).Reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}
}

// reconcilerFunc adapts a deterministic callback to the lifecycle boundary.
type reconcilerFunc func(context.Context) error

// Reconcile invokes one lifecycle callback.
func (f reconcilerFunc) Reconcile(ctx context.Context) error { return f(ctx) }

// TestDefaultSnapshotReaderRefusesUnsupportedStores avoids hidden HTTP fallbacks.
func TestDefaultSnapshotReaderRefusesUnsupportedStores(t *testing.T) {
	store := local.NewBlobStore()
	t.Cleanup(func() { _ = store.Close() })
	reader, err := defaultProductionFactory().readSnapshot(t.Context(), store, "snapshots/file.json")
	if reader != nil {
		_ = reader.Close()
	}
	if err == nil {
		t.Fatal("accepted object store without direct snapshot reads")
	}
}
