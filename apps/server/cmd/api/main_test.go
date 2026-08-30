package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"yujian.me/server/internal/config"
	"yujian.me/server/internal/ports"
	"yujian.me/server/internal/providers/edgeone"
	"yujian.me/server/internal/providers/local"
	providerS3 "yujian.me/server/internal/providers/s3"
	"yujian.me/server/internal/store/postgres"
)

func TestHealthHandler(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/healthz", nil)

	healthHandler(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}
	if contentType := recorder.Header().Get("Content-Type"); contentType != "application/json" {
		t.Fatalf("unexpected content type %q", contentType)
	}
	if body := recorder.Body.String(); body != "{\"status\":\"ok\"}\n" {
		t.Fatalf("unexpected body %q", body)
	}
}

func TestHealthHandlerRejectsUnsupportedMethod(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/healthz", nil)

	healthHandler(recorder, request)

	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", recorder.Code)
	}
}

func TestDevelopmentHandlerRunsDraftReviewAndPublishLoop(t *testing.T) {
	settings := config.Config{
		Environment:      "development",
		Address:          "127.0.0.1:0",
		AllowDevIdentity: true,
	}
	handler, err := buildHandler(settings, ServiceDependencies{})
	if err != nil {
		t.Fatalf("build handler: %v", err)
	}
	snapshot, err := os.ReadFile("../../../../content/fixtures/homepage.json")
	if err != nil {
		t.Fatalf("read content fixture: %v", err)
	}

	create := httptest.NewRecorder()
	createRequest := httptest.NewRequest(http.MethodPost, "/api/v1/versions", bytes.NewReader(mustJSON(map[string]any{"snapshot": json.RawMessage(snapshot)})))
	devHeaders(createRequest, "editor-1", "editor")
	handler.ServeHTTP(create, createRequest)
	if create.Code != http.StatusCreated {
		t.Fatalf("create draft: status=%d body=%s", create.Code, create.Body.String())
	}
	var created struct {
		ID       string `json:"id"`
		Revision int64  `json:"revision"`
	}
	if err := json.Unmarshal(create.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode draft: %v", err)
	}

	submit := httptest.NewRecorder()
	submitRequest := httptest.NewRequest(http.MethodPost, "/api/v1/versions/"+created.ID+"/review", bytes.NewReader(mustJSON(map[string]any{"revision": created.Revision})))
	devHeaders(submitRequest, "editor-1", "editor")
	handler.ServeHTTP(submit, submitRequest)
	if submit.Code != http.StatusOK {
		t.Fatalf("submit review: status=%d body=%s", submit.Code, submit.Body.String())
	}
	created.Revision++

	approve := httptest.NewRecorder()
	approveRequest := httptest.NewRequest(http.MethodPost, "/api/v1/versions/"+created.ID+"/approve", bytes.NewReader(mustJSON(map[string]any{"revision": created.Revision})))
	devHeaders(approveRequest, "reviewer-1", "reviewer")
	handler.ServeHTTP(approve, approveRequest)
	if approve.Code != http.StatusOK {
		t.Fatalf("approve review: status=%d body=%s", approve.Code, approve.Body.String())
	}
	created.Revision++

	publish := httptest.NewRecorder()
	publishRequest := httptest.NewRequest(http.MethodPost, "/api/v1/publishes", bytes.NewReader(mustJSON(map[string]any{"versionId": created.ID})))
	devHeaders(publishRequest, "publisher-1", "publisher")
	publishRequest.Header.Set("Idempotency-Key", "local-publish-1")
	handler.ServeHTTP(publish, publishRequest)
	if publish.Code != http.StatusAccepted {
		t.Fatalf("publish: status=%d body=%s", publish.Code, publish.Body.String())
	}
}

func TestBuildProductionDependenciesCreatesServicesAndClosesDatabase(t *testing.T) {
	database := &productionDatabaseFake{}
	var blobConfig providerS3.Config
	factory := productionFactory{
		openDatabase: func(context.Context, string) (productionDatabase, error) { return database, nil },
		newBlobStore: func(config providerS3.Config) (ports.BlobStore, error) {
			blobConfig = config
			return local.NewBlobStore(), nil
		},
		newBuildTrigger: func(edgeone.Config) (ports.BuildTrigger, error) { return local.NewBuildTrigger(), nil },
	}
	dependencies, closeResources, err := buildProductionDependencies(context.Background(), productionSettings(), factory)
	if err != nil {
		t.Fatalf("build production dependencies: %v", err)
	}
	if dependencies.Content == nil || dependencies.Assets == nil || dependencies.Publish == nil || dependencies.PublishReconciler == nil {
		t.Fatalf("incomplete production dependencies %#v", dependencies)
	}
	if blobConfig.PublicBaseURL != "https://media.yujian.me" {
		t.Fatalf("unexpected production media base URL %q", blobConfig.PublicBaseURL)
	}
	if database.beginCalls != 1 || database.tx == nil || !database.tx.committed {
		t.Fatalf("migrations did not commit in a database transaction: %#v", database)
	}
	if err := closeResources(); err != nil {
		t.Fatalf("close resources: %v", err)
	}
	if !database.closed {
		t.Fatal("database was not closed")
	}
}

func TestPublishReconcilerRunsImmediatelyAndStopsWithContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	reconciler := &publishReconcilerFake{calls: make(chan struct{}, 1)}
	done := make(chan struct{})
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	go func() {
		runPublishReconciler(ctx, time.Hour, reconciler, logger)
		close(done)
	}()

	select {
	case <-reconciler.calls:
	case <-time.After(time.Second):
		t.Fatal("reconciler did not run immediately")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("reconciler did not stop with context")
	}
}

func TestBuildProductionDependenciesClosesDatabaseOnProviderFailure(t *testing.T) {
	database := &productionDatabaseFake{}
	sentinel := errors.New("object storage unavailable")
	factory := productionFactory{
		openDatabase:    func(context.Context, string) (productionDatabase, error) { return database, nil },
		newBlobStore:    func(providerS3.Config) (ports.BlobStore, error) { return nil, sentinel },
		newBuildTrigger: func(edgeone.Config) (ports.BuildTrigger, error) { return local.NewBuildTrigger(), nil },
	}
	_, _, err := buildProductionDependencies(context.Background(), productionSettings(), factory)
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected provider error, got %v", err)
	}
	if !database.closed {
		t.Fatal("database was not closed after provider failure")
	}
}

type productionDatabaseFake struct {
	tx         *productionTxFake
	beginCalls int
	closed     bool
}

type publishReconcilerFake struct{ calls chan struct{} }

func (reconciler *publishReconcilerFake) Reconcile(context.Context) error {
	reconciler.calls <- struct{}{}
	return nil
}

func (*productionDatabaseFake) ExecContext(context.Context, string, ...any) (postgres.ExecResult, error) {
	return productionResultFake(1), nil
}
func (*productionDatabaseFake) QueryRowContext(context.Context, string, ...any) postgres.Row {
	return productionRowFake{}
}
func (*productionDatabaseFake) QueryContext(context.Context, string, ...any) (postgres.Rows, error) {
	return &productionRowsFake{}, nil
}
func (database *productionDatabaseFake) BeginTx(context.Context) (postgres.Tx, error) {
	database.beginCalls++
	database.tx = &productionTxFake{}
	return database.tx, nil
}
func (database *productionDatabaseFake) Close() error {
	database.closed = true
	return nil
}

type productionTxFake struct {
	committed bool
}

func (*productionTxFake) ExecContext(context.Context, string, ...any) (postgres.ExecResult, error) {
	return productionResultFake(1), nil
}
func (*productionTxFake) QueryRowContext(context.Context, string, ...any) postgres.Row {
	return productionRowFake{}
}
func (*productionTxFake) QueryContext(context.Context, string, ...any) (postgres.Rows, error) {
	return &productionRowsFake{}, nil
}
func (*productionTxFake) BeginTx(context.Context) (postgres.Tx, error) {
	return nil, errors.New("nested transaction")
}
func (tx *productionTxFake) Commit(context.Context) error { tx.committed = true; return nil }
func (*productionTxFake) Rollback(context.Context) error  { return nil }

type productionResultFake int64

func (result productionResultFake) RowsAffected() (int64, error) { return int64(result), nil }

type productionRowFake struct{}

func (productionRowFake) Scan(dest ...any) error {
	if len(dest) == 1 {
		if value, ok := dest[0].(*bool); ok {
			*value = false
			return nil
		}
	}
	return errors.New("no rows")
}

type productionRowsFake struct{}

func (*productionRowsFake) Next() bool        { return false }
func (*productionRowsFake) Scan(...any) error { return errors.New("no rows") }
func (*productionRowsFake) Err() error        { return nil }
func (*productionRowsFake) Close() error      { return nil }

func productionSettings() config.Config {
	return config.Config{
		Environment:         "production",
		DatabaseURL:         "postgres://example",
		OIDCIssuer:          "https://id.example.test",
		OIDCAudience:        "admin",
		AllowedAdminOrigins: []string{"https://admin.yujian.me"},
		S3Endpoint:          "https://cos.example.test",
		S3Region:            "ap-singapore",
		S3Bucket:            "media",
		S3AccessKeyID:       "access",
		S3SecretAccessKey:   "secret",
		MediaPublicBaseURL:  "https://media.yujian.me",
		EdgeOneTriggerURL:   "https://edgeone.example.test/trigger",
		EdgeOneStatusURL:    "https://edgeone.example.test/status",
		EdgeOneToken:        "token",
	}
}

func devHeaders(request *http.Request, subject, roles string) {
	request.Header.Set("X-Dev-Subject", subject)
	request.Header.Set("X-Dev-Roles", roles)
}

func mustJSON(value any) []byte {
	raw, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return raw
}
