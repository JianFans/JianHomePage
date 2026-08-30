package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"yujian.me/server/internal/assets"
	"yujian.me/server/internal/auth"
	"yujian.me/server/internal/domain"
	"yujian.me/server/internal/ports"
)

type contentServiceStub struct {
	createFn  func(context.Context, domain.Principal, json.RawMessage) (domain.ContentVersion, error)
	getFn     func(context.Context, domain.Principal, string) (domain.ContentVersion, error)
	updateFn  func(context.Context, domain.Principal, string, int64, json.RawMessage) (domain.ContentVersion, error)
	submitFn  func(context.Context, domain.Principal, string, int64) (domain.ContentVersion, error)
	approveFn func(context.Context, domain.Principal, string, int64) (domain.ContentVersion, error)
	rejectFn  func(context.Context, domain.Principal, string, int64, string) (domain.ContentVersion, error)
}

func (stub *contentServiceStub) CreateDraft(ctx context.Context, actor domain.Principal, snapshot json.RawMessage) (domain.ContentVersion, error) {
	if stub.createFn == nil {
		return domain.ContentVersion{}, nil
	}
	return stub.createFn(ctx, actor, snapshot)
}

func (stub *contentServiceStub) GetVersion(ctx context.Context, actor domain.Principal, id string) (domain.ContentVersion, error) {
	if stub.getFn == nil {
		return domain.ContentVersion{}, nil
	}
	return stub.getFn(ctx, actor, id)
}

func (stub *contentServiceStub) UpdateDraft(ctx context.Context, actor domain.Principal, id string, revision int64, snapshot json.RawMessage) (domain.ContentVersion, error) {
	if stub.updateFn == nil {
		return domain.ContentVersion{}, nil
	}
	return stub.updateFn(ctx, actor, id, revision, snapshot)
}

func (stub *contentServiceStub) SubmitReview(ctx context.Context, actor domain.Principal, id string, revision int64) (domain.ContentVersion, error) {
	if stub.submitFn == nil {
		return domain.ContentVersion{}, nil
	}
	return stub.submitFn(ctx, actor, id, revision)
}

func (stub *contentServiceStub) ApproveReview(ctx context.Context, actor domain.Principal, id string, revision int64) (domain.ContentVersion, error) {
	if stub.approveFn == nil {
		return domain.ContentVersion{}, nil
	}
	return stub.approveFn(ctx, actor, id, revision)
}

func (stub *contentServiceStub) RejectReview(ctx context.Context, actor domain.Principal, id string, revision int64, reason string) (domain.ContentVersion, error) {
	if stub.rejectFn == nil {
		return domain.ContentVersion{}, nil
	}
	return stub.rejectFn(ctx, actor, id, revision, reason)
}

type publishServiceStub struct {
	publishFn  func(context.Context, domain.Principal, string, string) (domain.PublishJob, error)
	getFn      func(context.Context, domain.Principal, string) (domain.PublishJob, error)
	refreshFn  func(context.Context, domain.Principal, string) (domain.PublishJob, error)
	rollbackFn func(context.Context, domain.Principal, string, string) (domain.PublishJob, error)
}

type assetServiceStub struct {
	createFn   func(context.Context, domain.Principal, assets.CreateUploadInput) (assets.CreateUploadResult, error)
	completeFn func(context.Context, domain.Principal, string) (domain.AssetRecord, error)
	deleteFn   func(context.Context, domain.Principal, string) error
}

func (stub *assetServiceStub) CreateUpload(ctx context.Context, actor domain.Principal, input assets.CreateUploadInput) (assets.CreateUploadResult, error) {
	if stub.createFn == nil {
		return assets.CreateUploadResult{}, nil
	}
	return stub.createFn(ctx, actor, input)
}

func (stub *assetServiceStub) CompleteUpload(ctx context.Context, actor domain.Principal, id string) (domain.AssetRecord, error) {
	if stub.completeFn == nil {
		return domain.AssetRecord{}, nil
	}
	return stub.completeFn(ctx, actor, id)
}

func (stub *assetServiceStub) Delete(ctx context.Context, actor domain.Principal, id string) error {
	if stub.deleteFn == nil {
		return nil
	}
	return stub.deleteFn(ctx, actor, id)
}

func (stub *publishServiceStub) Publish(ctx context.Context, actor domain.Principal, versionID, key string) (domain.PublishJob, error) {
	if stub.publishFn == nil {
		return domain.PublishJob{}, nil
	}
	return stub.publishFn(ctx, actor, versionID, key)
}

func (stub *publishServiceStub) GetPublishJob(ctx context.Context, actor domain.Principal, id string) (domain.PublishJob, error) {
	if stub.getFn == nil {
		return domain.PublishJob{}, nil
	}
	return stub.getFn(ctx, actor, id)
}

func (stub *publishServiceStub) RefreshStatus(ctx context.Context, actor domain.Principal, id string) (domain.PublishJob, error) {
	if stub.refreshFn == nil {
		return domain.PublishJob{}, nil
	}
	return stub.refreshFn(ctx, actor, id)
}

func (stub *publishServiceStub) Rollback(ctx context.Context, actor domain.Principal, versionID, key string) (domain.PublishJob, error) {
	if stub.rollbackFn == nil {
		return domain.PublishJob{}, nil
	}
	return stub.rollbackFn(ctx, actor, versionID, key)
}

func testVersion() domain.ContentVersion {
	now := time.Date(2026, 8, 30, 0, 0, 0, 0, time.UTC)
	return domain.ContentVersion{
		ID:        "ver_1",
		Status:    domain.StatusDraft,
		Revision:  1,
		Snapshot:  json.RawMessage(`{"schemaVersion":"1.0.0"}`),
		Checksum:  "sha256:test",
		CreatedBy: "editor-1",
		UpdatedBy: "editor-1",
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func testJob() domain.PublishJob {
	return domain.PublishJob{
		ID:               "pub_1",
		VersionID:        "ver_1",
		SnapshotKey:      "snapshots/rel_1/sha256-test.json",
		SnapshotChecksum: "sha256:test",
		Status:           domain.PublishBuilding,
	}
}

func testRouter(t *testing.T, content ContentService, publish PublishService) http.Handler {
	t.Helper()
	middleware, err := auth.NewMiddleware(auth.MiddlewareOptions{AllowDevIdentity: true})
	if err != nil {
		t.Fatalf("create auth middleware: %v", err)
	}
	return NewRouter(RouterOptions{
		Content:      content,
		Publish:      publish,
		Middleware:   middleware,
		MaxBodyBytes: 1024,
		RequestID:    func(*http.Request) string { return "req-test" },
	})
}

func authenticatedRequest(method, path, body string, roles string) *http.Request {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("X-Dev-Subject", "editor-1")
	request.Header.Set("X-Dev-Roles", roles)
	return request
}

func decodeError(t *testing.T, recorder *httptest.ResponseRecorder) map[string]string {
	t.Helper()
	var value map[string]string
	if err := json.Unmarshal(recorder.Body.Bytes(), &value); err != nil {
		t.Fatalf("decode error body: %v; body=%q", err, recorder.Body.String())
	}
	return value
}

func TestRouterHealthzDoesNotRequireAuthentication(t *testing.T) {
	router := testRouter(t, &contentServiceStub{}, &publishServiceStub{})
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}
}

func TestRouterMountsLocalMediaHandler(t *testing.T) {
	localHandler := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/media/assets/cover.webp" {
			t.Fatalf("unexpected local read path %q", request.URL.Path)
		}
		writer.WriteHeader(http.StatusNoContent)
	})
	router := NewRouter(RouterOptions{LocalUploads: localHandler})
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/media/assets/cover.webp", nil))

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("expected local read handler response, got %d", recorder.Code)
	}
}

func TestRouterHandlesAllowedCORSPreflightBeforeAuthentication(t *testing.T) {
	router := NewRouter(RouterOptions{
		Content:        &contentServiceStub{},
		Publish:        &publishServiceStub{},
		AllowedOrigins: []string{"https://admin.yujian.me"},
	})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodOptions, "/api/v1/versions", nil)
	request.Header.Set("Origin", "https://admin.yujian.me")
	request.Header.Set("Access-Control-Request-Method", http.MethodPost)
	request.Header.Set("Access-Control-Request-Headers", "authorization, content-type")

	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if recorder.Header().Get("Access-Control-Allow-Origin") != "https://admin.yujian.me" {
		t.Fatalf("unexpected allow origin %q", recorder.Header().Get("Access-Control-Allow-Origin"))
	}
	if !strings.Contains(strings.ToLower(recorder.Header().Get("Access-Control-Allow-Headers")), "authorization") {
		t.Fatalf("authorization header was not allowed: %q", recorder.Header().Get("Access-Control-Allow-Headers"))
	}
	if !strings.Contains(strings.ToLower(recorder.Header().Get("Access-Control-Allow-Headers")), "x-yujian-checksum") {
		t.Fatalf("local upload checksum header was not allowed: %q", recorder.Header().Get("Access-Control-Allow-Headers"))
	}
}

func TestRouterRejectsUnknownCORSOrigin(t *testing.T) {
	router := NewRouter(RouterOptions{
		Content:        &contentServiceStub{},
		Publish:        &publishServiceStub{},
		AllowedOrigins: []string{"https://admin.yujian.me"},
	})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodOptions, "/api/v1/versions", nil)
	request.Header.Set("Origin", "https://evil.example")
	request.Header.Set("Access-Control-Request-Method", http.MethodPost)

	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", recorder.Code)
	}
}

func TestRouterDoesNotBypassCORSAllowlistForMatchingHost(t *testing.T) {
	router := NewRouter(RouterOptions{AllowedOrigins: []string{"https://admin.yujian.me"}})
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodOptions, "/api/v1/versions", nil)
	request.Host = "api.yujian.me"
	request.Header.Set("Origin", "http://api.yujian.me")
	request.Header.Set("Access-Control-Request-Method", http.MethodPost)

	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for origin outside allowlist, got %d", recorder.Code)
	}
}

func TestCreateAssetUploadRequiresRightsObject(t *testing.T) {
	called := false
	middleware, err := auth.NewMiddleware(auth.MiddlewareOptions{AllowDevIdentity: true})
	if err != nil {
		t.Fatalf("create auth middleware: %v", err)
	}
	router := NewRouter(RouterOptions{
		Assets: &assetServiceStub{createFn: func(context.Context, domain.Principal, assets.CreateUploadInput) (assets.CreateUploadResult, error) {
			called = true
			return assets.CreateUploadResult{Upload: ports.SignedUpload{}}, nil
		}},
		Middleware: middleware,
	})
	recorder := httptest.NewRecorder()
	request := authenticatedRequest(http.MethodPost, "/api/v1/assets/uploads", `{"fileName":"cover.webp","contentType":"image/webp","size":100}`, "editor")

	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if called {
		t.Fatal("asset service must not be called without a rights object")
	}
}

func TestCreateAssetUploadReturnsStableSourceURL(t *testing.T) {
	middleware, err := auth.NewMiddleware(auth.MiddlewareOptions{AllowDevIdentity: true})
	if err != nil {
		t.Fatalf("create auth middleware: %v", err)
	}
	router := NewRouter(RouterOptions{
		Assets: &assetServiceStub{createFn: func(context.Context, domain.Principal, assets.CreateUploadInput) (assets.CreateUploadResult, error) {
			return assets.CreateUploadResult{
				Asset: domain.AssetRecord{
					ID: "asset_1", SourceURL: "https://media.yujian.me/assets/asset_1/source.webp",
					Status: domain.AssetPending, Metadata: json.RawMessage(`{}`), Rights: json.RawMessage(`{"source":{"zh-CN":"authorized"}}`),
				},
				Upload: ports.SignedUpload{URL: "https://upload.example.test/signed", ExpiresAt: time.Now().UTC().Add(time.Minute)},
			}, nil
		}},
		Middleware: middleware,
	})
	recorder := httptest.NewRecorder()
	request := authenticatedRequest(
		http.MethodPost,
		"/api/v1/assets/uploads",
		`{"fileName":"cover.webp","contentType":"image/webp","size":100,"checksum":"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","rights":{"source":{"zh-CN":"authorized"}}}`,
		"editor",
	)
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Asset struct {
			Src string `json:"src"`
		} `json:"asset"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Asset.Src != "https://media.yujian.me/assets/asset_1/source.webp" {
		t.Fatalf("unexpected asset source %q", response.Asset.Src)
	}
}

func TestCreateVersionReturnsCreatedETagAndRequestID(t *testing.T) {
	content := &contentServiceStub{
		createFn: func(_ context.Context, actor domain.Principal, snapshot json.RawMessage) (domain.ContentVersion, error) {
			if actor.Subject != "editor-1" || string(snapshot) != `{"schemaVersion":"1.0.0"}` {
				return domain.ContentVersion{}, errors.New("unexpected service input")
			}
			return testVersion(), nil
		},
	}
	router := testRouter(t, content, &publishServiceStub{})
	recorder := httptest.NewRecorder()
	request := authenticatedRequest(http.MethodPost, "/api/v1/versions", `{"snapshot":{"schemaVersion":"1.0.0"}}`, "editor")
	request.Header.Set("X-Request-ID", "req-create")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if recorder.Header().Get("ETag") != `"1"` {
		t.Fatalf("expected ETag \"1\", got %q", recorder.Header().Get("ETag"))
	}
	if recorder.Header().Get("X-Request-ID") != "req-create" {
		t.Fatalf("expected request id header, got %q", recorder.Header().Get("X-Request-ID"))
	}
}

func TestUpdateVersionRequiresIfMatch(t *testing.T) {
	called := false
	content := &contentServiceStub{updateFn: func(context.Context, domain.Principal, string, int64, json.RawMessage) (domain.ContentVersion, error) {
		called = true
		return testVersion(), nil
	}}
	router := testRouter(t, content, &publishServiceStub{})
	recorder := httptest.NewRecorder()
	request := authenticatedRequest(http.MethodPut, "/api/v1/versions/ver_1", `{"snapshot":{"schemaVersion":"1.0.0"}}`, "editor")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusPreconditionRequired {
		t.Fatalf("expected 428, got %d", recorder.Code)
	}
	if called {
		t.Fatal("update service must not be called without If-Match")
	}
	if value := decodeError(t, recorder); value["code"] != "precondition_required" {
		t.Fatalf("unexpected error %#v", value)
	}
}

func TestUpdateVersionMapsRevisionConflictTo409(t *testing.T) {
	content := &contentServiceStub{updateFn: func(context.Context, domain.Principal, string, int64, json.RawMessage) (domain.ContentVersion, error) {
		return domain.ContentVersion{}, domain.ErrConflict
	}}
	router := testRouter(t, content, &publishServiceStub{})
	recorder := httptest.NewRecorder()
	request := authenticatedRequest(http.MethodPut, "/api/v1/versions/ver_1", `{"snapshot":{"schemaVersion":"1.0.0"}}`, "editor")
	request.Header.Set("If-Match", `"1"`)
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d", recorder.Code)
	}
	if value := decodeError(t, recorder); value["requestId"] == "" {
		t.Fatalf("error should contain request id: %#v", value)
	}
}

func TestPublishRequiresIdempotencyKey(t *testing.T) {
	called := false
	publish := &publishServiceStub{publishFn: func(context.Context, domain.Principal, string, string) (domain.PublishJob, error) {
		called = true
		return testJob(), nil
	}}
	router := testRouter(t, &contentServiceStub{}, publish)
	recorder := httptest.NewRecorder()
	request := authenticatedRequest(http.MethodPost, "/api/v1/publishes", `{"versionId":"ver_1"}`, "publisher")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", recorder.Code)
	}
	if called {
		t.Fatal("publish service must not be called without idempotency key")
	}
	if value := decodeError(t, recorder); value["code"] != "invalid_request" {
		t.Fatalf("unexpected error %#v", value)
	}
}

func TestPublishRejectsIdempotencyKeyOutsideContractLength(t *testing.T) {
	for _, key := range []string{"1234567", strings.Repeat("x", 129)} {
		t.Run(key[:1], func(t *testing.T) {
			called := false
			publish := &publishServiceStub{publishFn: func(context.Context, domain.Principal, string, string) (domain.PublishJob, error) {
				called = true
				return testJob(), nil
			}}
			router := testRouter(t, &contentServiceStub{}, publish)
			recorder := httptest.NewRecorder()
			request := authenticatedRequest(http.MethodPost, "/api/v1/publishes", `{"versionId":"ver_1"}`, "publisher")
			request.Header.Set("Idempotency-Key", key)

			router.ServeHTTP(recorder, request)

			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d", recorder.Code)
			}
			if called {
				t.Fatal("publish service must not receive an invalid idempotency key")
			}
		})
	}
}

func TestPublishMapsForbiddenRoleTo403(t *testing.T) {
	publish := &publishServiceStub{publishFn: func(context.Context, domain.Principal, string, string) (domain.PublishJob, error) {
		return domain.PublishJob{}, domain.ErrForbidden
	}}
	router := testRouter(t, &contentServiceStub{}, publish)
	recorder := httptest.NewRecorder()
	request := authenticatedRequest(http.MethodPost, "/api/v1/publishes", `{"versionId":"ver_1"}`, "editor")
	request.Header.Set("Idempotency-Key", "idem-1234")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", recorder.Code)
	}
}

func TestPublishRouteRejectsEditorBeforeCallingService(t *testing.T) {
	called := false
	publish := &publishServiceStub{publishFn: func(context.Context, domain.Principal, string, string) (domain.PublishJob, error) {
		called = true
		return testJob(), nil
	}}
	router := testRouter(t, &contentServiceStub{}, publish)
	recorder := httptest.NewRecorder()
	request := authenticatedRequest(http.MethodPost, "/api/v1/publishes", `{"versionId":"ver_1"}`, "editor")
	request.Header.Set("Idempotency-Key", "idem-1234")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", recorder.Code)
	}
	if called {
		t.Fatal("editor request must be rejected before publish service")
	}
}

func TestJSONDecoderRejectsUnknownFields(t *testing.T) {
	router := testRouter(t, &contentServiceStub{}, &publishServiceStub{})
	recorder := httptest.NewRecorder()
	request := authenticatedRequest(http.MethodPost, "/api/v1/versions", `{"snapshot":{"schemaVersion":"1.0.0"},"unexpected":true}`, "editor")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", recorder.Code)
	}
}

func TestRequestBodyLimitReturns413(t *testing.T) {
	router := testRouter(t, &contentServiceStub{}, &publishServiceStub{})
	recorder := httptest.NewRecorder()
	request := authenticatedRequest(http.MethodPost, "/api/v1/versions", `{"snapshot":"`+strings.Repeat("x", 2000)+`"}`, "editor")
	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d", recorder.Code)
	}
}

func TestMissingBearerAndDevIdentityReturns401(t *testing.T) {
	middleware, err := auth.NewMiddleware(auth.MiddlewareOptions{})
	if err != nil {
		t.Fatalf("create middleware: %v", err)
	}
	router := NewRouter(RouterOptions{Content: &contentServiceStub{}, Publish: &publishServiceStub{}, Middleware: middleware})
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/v1/versions", strings.NewReader(`{"snapshot":{}}`)))

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", recorder.Code)
	}
	if value := decodeError(t, recorder); value["code"] != "unauthorized" {
		t.Fatalf("unexpected error %#v", value)
	}
}
