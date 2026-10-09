package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"yujian.me/server/internal/config"
	"yujian.me/server/internal/httpapi"
	"yujian.me/server/internal/ports"
	"yujian.me/server/internal/providers/edgeone"
	"yujian.me/server/internal/providers/local"
	providerS3 "yujian.me/server/internal/providers/s3"
	"yujian.me/server/internal/store/postgres"
)

// TestLocalUploadOutlivesAPIDeadlines exercises real socket deadlines through
// the production logging wrapper; ordinary requests keep the server limit.
func TestLocalUploadOutlivesAPIDeadlines(t *testing.T) {
	dependencies := developmentDependencies()
	t.Cleanup(func() { _ = dependencies.Close() })
	handler, err := buildHandler(config.Config{Environment: "development"}, dependencies)
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte("slow-video-upload")
	upload, err := dependencies.LocalUploads.(*local.BlobStore).CreateUpload(t.Context(), ports.UploadRequest{
		BlobKey: "assets/slow/source.mp4", ContentType: "video/mp4", Size: int64(len(payload)),
		Checksum: fmt.Sprintf("sha256:%x", sha256.Sum256(payload)), ExpiresIn: time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	signed, err := url.Parse(upload.URL)
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{signed.RequestURI(), "/deadline-control"} {
		t.Run(target, func(t *testing.T) {
			entered := make(chan struct{})
			endpoint := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				close(entered)
				if request.URL.Path == "/deadline-control" {
					_, _ = io.Copy(io.Discard, request.Body)
					writer.WriteHeader(http.StatusNoContent)
					return
				}
				handler.ServeHTTP(writer, request)
			})
			server := httptest.NewUnstartedServer(httpapi.LoggingMiddleware(endpoint, slog.New(slog.NewTextHandler(io.Discard, nil))))
			server.Config.ReadTimeout = 50 * time.Millisecond
			server.Config.WriteTimeout = 50 * time.Millisecond
			server.Start()
			defer server.Close()
			body, sender := io.Pipe()
			defer body.Close()
			defer sender.Close()
			request, err := http.NewRequestWithContext(t.Context(), http.MethodPut, server.URL+target, body)
			if err != nil {
				t.Fatal(err)
			}
			request.ContentLength = int64(len(payload))
			for key, value := range upload.Headers {
				request.Header.Set(key, value)
			}
			client := server.Client()
			client.Timeout = 3 * time.Second
			type result struct {
				response *http.Response
				err      error
			}
			finished := make(chan result, 1)
			go func() {
				response, err := client.Do(request)
				finished <- result{response, err}
			}()
			select {
			case <-entered:
			case <-time.After(3 * time.Second):
				t.Fatal("request did not reach handler")
			}
			// Exceed both default deadlines before delivering the reserved body.
			time.Sleep(150 * time.Millisecond)
			_, _ = sender.Write(payload)
			_ = sender.Close()
			got := <-finished
			if got.response != nil {
				defer got.response.Body.Close()
			}
			if target == "/deadline-control" {
				if got.err == nil {
					t.Fatal("ordinary request unexpectedly outlived server deadlines")
				}
			} else if got.err != nil || got.response.StatusCode != http.StatusNoContent {
				t.Fatalf("reserved slow upload failed: response=%v error=%v", got.response, got.err)
			}
		})
	}
}

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

func TestDevelopmentHandlerRunsAssetUploadRoundTrip(t *testing.T) {
	settings := config.Config{
		Environment:      "development",
		Address:          "127.0.0.1:0",
		AllowDevIdentity: true,
	}
	dependencies := developmentDependencies()
	t.Cleanup(func() {
		if err := dependencies.Close(); err != nil {
			t.Errorf("close development dependencies: %v", err)
		}
	})
	handler, err := buildHandler(settings, dependencies)
	if err != nil {
		t.Fatalf("build handler: %v", err)
	}
	payload, err := os.ReadFile("../../../web/public/media/cover-05.webp")
	if err != nil {
		t.Fatalf("read WebP fixture: %v", err)
	}
	checksum := fmt.Sprintf("sha256:%x", sha256.Sum256(payload))

	create := httptest.NewRecorder()
	createRequest := httptest.NewRequest(http.MethodPost, "/api/v1/assets/uploads", bytes.NewReader(mustJSON(map[string]any{
		"fileName": "cover.webp", "contentType": "image/webp", "size": len(payload), "checksum": checksum,
		"rights": map[string]any{"source": map[string]string{"zh-CN": "本地闭环测试"}},
	})))
	devHeaders(createRequest, "editor-1", "editor")
	handler.ServeHTTP(create, createRequest)
	if create.Code != http.StatusCreated {
		t.Fatalf("create asset upload: status=%d body=%s", create.Code, create.Body.String())
	}
	var created struct {
		Asset struct {
			ID     string `json:"id"`
			Src    string `json:"src"`
			Status string `json:"status"`
		} `json:"asset"`
		UploadURL string            `json:"uploadUrl"`
		Headers   map[string]string `json:"headers"`
	}
	if err := json.Unmarshal(create.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode asset upload: %v", err)
	}
	if created.Asset.Status != "pending" || created.Asset.Src == "" {
		t.Fatalf("unexpected pending asset %#v", created.Asset)
	}
	signedURL, err := url.Parse(created.UploadURL)
	if err != nil {
		t.Fatalf("parse signed upload URL: %v", err)
	}

	upload := httptest.NewRecorder()
	uploadRequest := httptest.NewRequest(http.MethodPut, signedURL.RequestURI(), bytes.NewReader(payload))
	for key, value := range created.Headers {
		uploadRequest.Header.Set(key, value)
	}
	handler.ServeHTTP(upload, uploadRequest)
	if upload.Code != http.StatusNoContent {
		t.Fatalf("upload asset blob: status=%d body=%s", upload.Code, upload.Body.String())
	}

	complete := httptest.NewRecorder()
	completeRequest := httptest.NewRequest(http.MethodPost, "/api/v1/assets/"+created.Asset.ID+"/complete", bytes.NewReader([]byte(`{}`)))
	devHeaders(completeRequest, "editor-1", "editor")
	handler.ServeHTTP(complete, completeRequest)
	if complete.Code != http.StatusOK {
		t.Fatalf("complete asset upload: status=%d body=%s", complete.Code, complete.Body.String())
	}
	var completed struct {
		ID     string `json:"id"`
		Src    string `json:"src"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal(complete.Body.Bytes(), &completed); err != nil {
		t.Fatalf("decode completed asset: %v", err)
	}
	if completed.ID != created.Asset.ID || completed.Src != created.Asset.Src || completed.Status != "ready" {
		t.Fatalf("unexpected completed asset %#v", completed)
	}

	media := httptest.NewRecorder()
	handler.ServeHTTP(media, httptest.NewRequest(http.MethodGet, completed.Src, nil))
	if media.Code != http.StatusOK || media.Header().Get("Content-Type") != "image/webp" || !bytes.Equal(media.Body.Bytes(), payload) {
		t.Fatalf("read stable media: status=%d type=%q size=%d", media.Code, media.Header().Get("Content-Type"), media.Body.Len())
	}

	list := httptest.NewRecorder()
	listRequest := httptest.NewRequest(http.MethodGet, "/api/v1/assets?status=ready", nil)
	devHeaders(listRequest, "editor-1", "editor")
	handler.ServeHTTP(list, listRequest)
	if list.Code != http.StatusOK || !bytes.Contains(list.Body.Bytes(), []byte(created.Asset.ID)) {
		t.Fatalf("list ready assets: status=%d body=%s", list.Code, list.Body.String())
	}
}

func TestDevelopmentDependenciesCloseLocalBlobStore(t *testing.T) {
	dependencies := developmentDependencies()
	store, ok := dependencies.LocalUploads.(*local.BlobStore)
	if !ok || dependencies.Close == nil {
		t.Fatalf("development dependencies do not expose local cleanup: %#v", dependencies)
	}
	payload := []byte("temporary-media")
	if err := store.Put(t.Context(), "assets/temporary/source.webp", bytes.NewReader(payload), ports.BlobMetadata{
		ContentType: "image/webp",
		Size:        int64(len(payload)),
		Checksum:    fmt.Sprintf("sha256:%x", sha256.Sum256(payload)),
	}); err != nil {
		t.Fatalf("put local object: %v", err)
	}

	if err := dependencies.Close(); err != nil {
		t.Fatalf("close development dependencies: %v", err)
	}
	if _, err := store.Stat(t.Context(), "assets/temporary/source.webp"); err == nil {
		t.Fatal("development cleanup retained local object")
	}
}

func TestBuildProductionDependenciesCreatesServicesAndClosesDatabase(t *testing.T) {
	database := &productionDatabaseFake{legacyAsset: true}
	var blobConfig providerS3.Config
	var buildConfig edgeone.Config
	var databaseURL string
	factory := productionFactory{
		openDatabase: func(_ context.Context, value string) (productionDatabase, error) {
			databaseURL = value
			return database, nil
		},
		newBlobStore: func(config providerS3.Config) (ports.BlobStore, error) {
			blobConfig = config
			return local.NewBlobStore(), nil
		},
		newBuildTrigger: func(config edgeone.Config) (ports.BuildTrigger, error) {
			buildConfig = config
			return local.NewBuildTrigger(), nil
		},
	}
	settings := productionSettings()
	dependencies, closeResources, err := buildProductionDependencies(context.Background(), settings, factory)
	if err != nil {
		t.Fatalf("build production dependencies: %v", err)
	}
	if dependencies.Content == nil || dependencies.Assets == nil || dependencies.Publish == nil || dependencies.PublishReconciler == nil {
		t.Fatalf("incomplete production dependencies %#v", dependencies)
	}
	if blobConfig.PublicBaseURL != "https://media.yujian.me" {
		t.Fatalf("unexpected production media base URL %q", blobConfig.PublicBaseURL)
	}
	if !blobConfig.RequireHTTPS {
		t.Fatal("production object storage must require HTTPS")
	}
	if databaseURL != settings.DatabaseURL {
		t.Fatalf("unexpected production database URL %q", databaseURL)
	}
	if buildConfig.TriggerURL != settings.EdgeOneTriggerURL || buildConfig.StatusURL != settings.EdgeOneStatusURL || !buildConfig.RequireHTTPS {
		t.Fatalf("unexpected EdgeOne production configuration %#v", buildConfig)
	}
	if database.beginCalls != 1 || database.tx == nil || !database.tx.committed {
		t.Fatalf("migrations did not commit in a database transaction: %#v", database)
	}
	assetURLBackfilled := false
	for index, query := range database.tx.execQueries {
		if strings.Contains(query, "UPDATE assets") && strings.Contains(query, "source_url") {
			args := database.tx.execArgs[index]
			assetURLBackfilled = len(args) == 2 && args[0] == "/media/assets/asset_legacy/source.webp" && args[1] == "asset_legacy"
		}
	}
	if !assetURLBackfilled {
		t.Fatalf("production migration did not use BlobStore.PublicURL: queries=%#v args=%#v", database.tx.execQueries, database.tx.execArgs)
	}
	handler, err := buildHandler(settings, dependencies)
	if err != nil {
		t.Fatalf("build production handler: %v", err)
	}
	health := httptest.NewRecorder()
	handler.ServeHTTP(health, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if health.Code != http.StatusOK {
		t.Fatalf("production health check: status=%d body=%s", health.Code, health.Body.String())
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

func TestPublishReconcilerLogsFailuresAndUsesDefaultInterval(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	reconciler := &publishReconcilerFake{
		calls: make(chan struct{}, 1),
		err:   errors.New("reconcile unavailable"),
	}
	logs := &notifyingWriter{wrote: make(chan struct{}, 1)}
	done := make(chan struct{})
	go func() {
		runPublishReconciler(ctx, 0, reconciler, slog.New(slog.NewTextHandler(logs, nil)))
		close(done)
	}()

	select {
	case <-reconciler.calls:
	case <-time.After(time.Second):
		t.Fatal("reconciler did not run immediately")
	}
	select {
	case <-logs.wrote:
	case <-time.After(time.Second):
		t.Fatal("reconcile failure was not logged")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("reconciler did not stop with context")
	}
	if !bytes.Contains(logs.Bytes(), []byte("reconcile unavailable")) {
		t.Fatalf("unexpected reconcile log: %q", logs.String())
	}
}

// TestStartPublishReconcilerWaitsForActiveRunBeforeStopping protects resource
// shutdown from racing an in-flight reconciliation.
func TestStartPublishReconcilerWaitsForActiveRunBeforeStopping(t *testing.T) {
	reconciler := &blockingPublishReconciler{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	stop := startPublishReconciler(context.Background(), time.Hour, reconciler, slog.New(slog.NewTextHandler(io.Discard, nil)))
	<-reconciler.started

	stopped := make(chan struct{})
	go func() {
		stop()
		close(stopped)
	}()
	select {
	case <-stopped:
		t.Fatal("reconciler stop returned before the active run completed")
	case <-time.After(25 * time.Millisecond):
	}
	close(reconciler.release)
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("reconciler stop did not wait for completion")
	}
}

func TestRunDevelopmentStopsWithCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	settings := config.Config{
		Environment:      "development",
		Address:          "127.0.0.1:0",
		AllowDevIdentity: true,
		ShutdownTimeout:  time.Second,
	}

	if err := run(ctx, settings, slog.New(slog.NewTextHandler(io.Discard, nil))); err != nil {
		t.Fatalf("run development server: %v", err)
	}
}

func TestRunReturnsHandlerConfigurationError(t *testing.T) {
	settings := config.Config{
		Environment:      "development",
		OIDCIssuer:       "://invalid",
		OIDCAudience:     "admin",
		AllowDevIdentity: true,
	}

	if err := run(context.Background(), settings, slog.New(slog.NewTextHandler(io.Discard, nil))); err == nil {
		t.Fatal("expected invalid OIDC configuration to stop startup")
	}
}

func TestRunProductionReturnsDatabaseConfigurationError(t *testing.T) {
	settings := productionSettings()
	settings.DatabaseURL = ""

	err := run(context.Background(), settings, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err == nil {
		t.Fatal("expected missing production database URL to stop startup")
	}
}

func TestDefaultProductionFactoryReturnsAdapterConfigurationErrors(t *testing.T) {
	factory := defaultProductionFactory()
	if factory.openDatabase == nil || factory.newBlobStore == nil || factory.newBuildTrigger == nil {
		t.Fatalf("incomplete default production factory %#v", factory)
	}
	if _, err := factory.openDatabase(context.Background(), ""); err == nil {
		t.Fatal("expected empty database URL to fail")
	}
	if _, err := factory.newBlobStore(providerS3.Config{}); err == nil {
		t.Fatal("expected empty object storage configuration to fail")
	}
	if _, err := factory.newBuildTrigger(edgeone.Config{}); err == nil {
		t.Fatal("expected empty build trigger configuration to fail")
	}
}

func TestRunServerReturnsListenError(t *testing.T) {
	settings := config.Config{
		Address:         "127.0.0.1:-1",
		ShutdownTimeout: time.Second,
	}
	err := runServer(context.Background(), settings, slog.New(slog.NewTextHandler(io.Discard, nil)), http.HandlerFunc(healthHandler))
	if err == nil {
		t.Fatal("expected invalid listen address to fail")
	}
}

func TestRunServerShutsDownWhenContextIsCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	settings := config.Config{
		Address:         "127.0.0.1:0",
		ShutdownTimeout: time.Second,
	}

	if err := runServer(ctx, settings, slog.New(slog.NewTextHandler(io.Discard, nil)), http.HandlerFunc(healthHandler)); err != nil {
		t.Fatalf("shutdown server: %v", err)
	}
}

func TestBuildProductionDependenciesRejectsNonProductionEnvironment(t *testing.T) {
	_, closeResources, err := buildProductionDependencies(context.Background(), config.Config{Environment: "development"}, productionFactory{})
	if err == nil {
		t.Fatal("expected non-production environment to be rejected")
	}
	if closeResources != nil {
		t.Fatal("unexpected cleanup function after rejected configuration")
	}
}

func TestBuildProductionDependenciesReturnsDatabaseOpenError(t *testing.T) {
	sentinel := errors.New("database unavailable")
	factory := productionFactory{
		openDatabase: func(context.Context, string) (productionDatabase, error) { return nil, sentinel },
	}

	_, _, err := buildProductionDependencies(context.Background(), productionSettings(), factory)
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected database error, got %v", err)
	}
}

func TestBuildProductionDependenciesClosesDatabaseOnMigrationFailure(t *testing.T) {
	sentinel := errors.New("migration unavailable")
	database := &productionDatabaseFake{beginErr: sentinel}
	factory := productionFactory{
		openDatabase: func(context.Context, string) (productionDatabase, error) { return database, nil },
		newBlobStore: func(providerS3.Config) (ports.BlobStore, error) { return local.NewBlobStore(), nil },
	}

	_, _, err := buildProductionDependencies(context.Background(), productionSettings(), factory)
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected migration error, got %v", err)
	}
	if !database.closed {
		t.Fatal("database was not closed after migration failure")
	}
}

func TestBuildProductionDependenciesClosesDatabaseOnBuildTriggerFailure(t *testing.T) {
	database := &productionDatabaseFake{}
	sentinel := errors.New("build trigger unavailable")
	factory := productionFactory{
		openDatabase: func(context.Context, string) (productionDatabase, error) { return database, nil },
		newBlobStore: func(providerS3.Config) (ports.BlobStore, error) {
			return local.NewBlobStore(), nil
		},
		newBuildTrigger: func(edgeone.Config) (ports.BuildTrigger, error) { return nil, sentinel },
	}

	_, _, err := buildProductionDependencies(context.Background(), productionSettings(), factory)
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected build trigger error, got %v", err)
	}
	if !database.closed {
		t.Fatal("database was not closed after build trigger failure")
	}
}

func TestBuildHandlerRejectsIncompleteProductionServices(t *testing.T) {
	_, err := buildHandler(productionSettings(), ServiceDependencies{})
	if err == nil {
		t.Fatal("expected incomplete production services to be rejected")
	}
}

func TestBuildHandlerRejectsDevelopmentIdentityInProduction(t *testing.T) {
	settings := productionSettings()
	settings.OIDCIssuer = ""
	settings.OIDCAudience = ""
	settings.AllowDevIdentity = true

	_, err := buildHandler(settings, developmentDependencies())
	if !errors.Is(err, config.ErrUnsafeDevelopmentIdentity) {
		t.Fatalf("expected unsafe development identity error, got %v", err)
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
	tx          *productionTxFake
	beginCalls  int
	beginErr    error
	closed      bool
	legacyAsset bool
}

type publishReconcilerFake struct {
	calls chan struct{}
	err   error
}

type blockingPublishReconciler struct {
	started chan struct{}
	release chan struct{}
}

type notifyingWriter struct {
	bytes.Buffer
	wrote chan struct{}
}

func (writer *notifyingWriter) Write(value []byte) (int, error) {
	written, err := writer.Buffer.Write(value)
	select {
	case writer.wrote <- struct{}{}:
	default:
	}
	return written, err
}

func (reconciler *publishReconcilerFake) Reconcile(context.Context) error {
	reconciler.calls <- struct{}{}
	return reconciler.err
}

// Reconcile holds one run open until the lifecycle test releases it.
func (reconciler *blockingPublishReconciler) Reconcile(context.Context) error {
	close(reconciler.started)
	<-reconciler.release
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
	if database.beginErr != nil {
		return nil, database.beginErr
	}
	database.tx = &productionTxFake{legacyAsset: database.legacyAsset}
	return database.tx, nil
}
func (database *productionDatabaseFake) Close() error {
	database.closed = true
	return nil
}

type productionTxFake struct {
	committed   bool
	legacyAsset bool
	execQueries []string
	execArgs    [][]any
}

func (tx *productionTxFake) ExecContext(_ context.Context, query string, args ...any) (postgres.ExecResult, error) {
	tx.execQueries = append(tx.execQueries, query)
	tx.execArgs = append(tx.execArgs, args)
	return productionResultFake(1), nil
}
func (*productionTxFake) QueryRowContext(context.Context, string, ...any) postgres.Row {
	return productionRowFake{}
}
func (tx *productionTxFake) QueryContext(_ context.Context, query string, _ ...any) (postgres.Rows, error) {
	if tx.legacyAsset && strings.Contains(query, "FROM assets") {
		return &productionAssetRowsFake{}, nil
	}
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

type productionAssetRowsFake struct{ read bool }

func (rows *productionAssetRowsFake) Next() bool {
	if rows.read {
		return false
	}
	rows.read = true
	return true
}

func (*productionAssetRowsFake) Scan(dest ...any) error {
	if len(dest) != 2 {
		return errors.New("expected asset id and blob key")
	}
	id, idOK := dest[0].(*string)
	key, keyOK := dest[1].(*string)
	if !idOK || !keyOK {
		return errors.New("expected string asset destinations")
	}
	*id = "asset_legacy"
	*key = "assets/asset_legacy/source.webp"
	return nil
}

func (*productionAssetRowsFake) Err() error   { return nil }
func (*productionAssetRowsFake) Close() error { return nil }

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
