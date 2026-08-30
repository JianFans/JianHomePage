package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"yujian.me/server/internal/assets"
	"yujian.me/server/internal/auth"
	"yujian.me/server/internal/domain"
)

func testRouterWithAssets(t *testing.T, content ContentService, assetService AssetService, publish PublishService) http.Handler {
	t.Helper()
	middleware, err := auth.NewMiddleware(auth.MiddlewareOptions{AllowDevIdentity: true})
	if err != nil {
		t.Fatalf("create auth middleware: %v", err)
	}
	return NewRouter(RouterOptions{
		Content:      content,
		Assets:       assetService,
		Publish:      publish,
		Middleware:   middleware,
		MaxBodyBytes: 1024,
		RequestID:    func(*http.Request) string { return "req-routes" },
	})
}

func TestGetVersionReturnsVersionAndETag(t *testing.T) {
	content := &contentServiceStub{getFn: func(_ context.Context, actor domain.Principal, id string) (domain.ContentVersion, error) {
		if actor.Subject != "editor-1" || id != "ver_1" {
			t.Fatalf("unexpected service input: actor=%q id=%q", actor.Subject, id)
		}
		return testVersion(), nil
	}}
	recorder := httptest.NewRecorder()
	request := authenticatedRequest(http.MethodGet, "/api/v1/versions/ver_1", "", "editor")

	testRouter(t, content, &publishServiceStub{}).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if recorder.Header().Get("ETag") != `"1"` {
		t.Fatalf("unexpected ETag %q", recorder.Header().Get("ETag"))
	}
	var response versionResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode version response: %v", err)
	}
	if response.ID != "ver_1" || response.Revision != 1 || response.Checksum != "sha256:test" {
		t.Fatalf("unexpected version response %#v", response)
	}
}

func TestReviewRoutesForwardRevisionAndReason(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		body    string
		role    string
		content *contentServiceStub
	}{
		{
			name: "submit",
			path: "/api/v1/versions/ver_1/review",
			body: `{"revision":7}`,
			role: "editor",
			content: &contentServiceStub{submitFn: func(_ context.Context, actor domain.Principal, id string, revision int64) (domain.ContentVersion, error) {
				if actor.Subject != "editor-1" || id != "ver_1" || revision != 7 {
					t.Fatalf("unexpected submit input: actor=%q id=%q revision=%d", actor.Subject, id, revision)
				}
				version := testVersion()
				version.Revision = 8
				version.Status = domain.StatusInReview
				return version, nil
			}},
		},
		{
			name: "approve",
			path: "/api/v1/versions/ver_1/approve",
			body: `{"revision":8}`,
			role: "reviewer",
			content: &contentServiceStub{approveFn: func(_ context.Context, actor domain.Principal, id string, revision int64) (domain.ContentVersion, error) {
				if actor.Subject != "editor-1" || id != "ver_1" || revision != 8 {
					t.Fatalf("unexpected approve input: actor=%q id=%q revision=%d", actor.Subject, id, revision)
				}
				version := testVersion()
				version.Revision = 9
				version.ReviewApproved = true
				return version, nil
			}},
		},
		{
			name: "reject",
			path: "/api/v1/versions/ver_1/reject",
			body: `{"revision":8,"reason":"missing rights evidence"}`,
			role: "reviewer",
			content: &contentServiceStub{rejectFn: func(_ context.Context, actor domain.Principal, id string, revision int64, reason string) (domain.ContentVersion, error) {
				if actor.Subject != "editor-1" || id != "ver_1" || revision != 8 || reason != "missing rights evidence" {
					t.Fatalf("unexpected reject input: actor=%q id=%q revision=%d reason=%q", actor.Subject, id, revision, reason)
				}
				version := testVersion()
				version.Revision = 9
				return version, nil
			}},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := authenticatedRequest(http.MethodPost, test.path, test.body, test.role)

			testRouter(t, test.content, &publishServiceStub{}).ServeHTTP(recorder, request)

			if recorder.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d body=%s", recorder.Code, recorder.Body.String())
			}
			if recorder.Header().Get("ETag") == "" {
				t.Fatal("review response must include an ETag")
			}
		})
	}
}

func TestAssetLifecycleRoutesReturnStableRepresentations(t *testing.T) {
	deletedAt := time.Date(2026, 8, 30, 2, 0, 0, 0, time.UTC)
	asset := domain.AssetRecord{
		ID:        "asset_1",
		SourceURL: "https://media.yujian.me/assets/asset_1/source.webp",
		Status:    domain.AssetReady,
		Metadata:  json.RawMessage(`{"width":1200}`),
		Rights:    json.RawMessage(`{"source":{"zh-CN":"authorized"}}`),
		DeletedAt: &deletedAt,
	}
	assetService := &assetServiceStub{
		completeFn: func(_ context.Context, actor domain.Principal, id string) (domain.AssetRecord, error) {
			if actor.Subject != "editor-1" || id != "asset_1" {
				t.Fatalf("unexpected complete input: actor=%q id=%q", actor.Subject, id)
			}
			return asset, nil
		},
		deleteFn: func(_ context.Context, actor domain.Principal, id string) error {
			if actor.Subject != "editor-1" || id != "asset_1" {
				t.Fatalf("unexpected delete input: actor=%q id=%q", actor.Subject, id)
			}
			return nil
		},
	}
	router := testRouterWithAssets(t, &contentServiceStub{}, assetService, &publishServiceStub{})

	completeRecorder := httptest.NewRecorder()
	completeRequest := authenticatedRequest(http.MethodPost, "/api/v1/assets/asset_1/complete", "", "editor")
	router.ServeHTTP(completeRecorder, completeRequest)
	if completeRecorder.Code != http.StatusOK {
		t.Fatalf("expected complete status 200, got %d body=%s", completeRecorder.Code, completeRecorder.Body.String())
	}
	var response assetResponse
	if err := json.Unmarshal(completeRecorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode asset response: %v", err)
	}
	if response.ID != asset.ID || response.Src != asset.SourceURL || response.DeletedAt == nil {
		t.Fatalf("unexpected asset response %#v", response)
	}

	deleteRecorder := httptest.NewRecorder()
	deleteRequest := authenticatedRequest(http.MethodDelete, "/api/v1/assets/asset_1", "", "admin")
	router.ServeHTTP(deleteRecorder, deleteRequest)
	if deleteRecorder.Code != http.StatusNoContent || deleteRecorder.Body.Len() != 0 {
		t.Fatalf("expected empty 204 response, got %d body=%q", deleteRecorder.Code, deleteRecorder.Body.String())
	}
}

func TestPublishRoutesForwardIdentifiersAndReturnJob(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		role       string
		configure  func(*publishServiceStub)
		wantStatus int
	}{
		{
			name:       "create",
			method:     http.MethodPost,
			path:       "/api/v1/publishes",
			body:       `{"versionId":"ver_1"}`,
			role:       "publisher",
			wantStatus: http.StatusAccepted,
			configure: func(stub *publishServiceStub) {
				stub.publishFn = func(_ context.Context, actor domain.Principal, versionID, key string) (domain.PublishJob, error) {
					if actor.Subject != "editor-1" || versionID != "ver_1" || key != "idem-1234" {
						t.Fatalf("unexpected publish input: actor=%q version=%q key=%q", actor.Subject, versionID, key)
					}
					return testJob(), nil
				}
			},
		},
		{
			name:       "get",
			method:     http.MethodGet,
			path:       "/api/v1/publishes/pub_1",
			role:       "publisher",
			wantStatus: http.StatusOK,
			configure: func(stub *publishServiceStub) {
				stub.getFn = func(_ context.Context, actor domain.Principal, id string) (domain.PublishJob, error) {
					if actor.Subject != "editor-1" || id != "pub_1" {
						t.Fatalf("unexpected get input: actor=%q id=%q", actor.Subject, id)
					}
					return testJob(), nil
				}
			},
		},
		{
			name:       "refresh",
			method:     http.MethodPost,
			path:       "/api/v1/publishes/pub_1/refresh",
			role:       "publisher",
			wantStatus: http.StatusOK,
			configure: func(stub *publishServiceStub) {
				stub.refreshFn = func(_ context.Context, actor domain.Principal, id string) (domain.PublishJob, error) {
					if actor.Subject != "editor-1" || id != "pub_1" {
						t.Fatalf("unexpected refresh input: actor=%q id=%q", actor.Subject, id)
					}
					return testJob(), nil
				}
			},
		},
		{
			name:       "rollback",
			method:     http.MethodPost,
			path:       "/api/v1/rollbacks",
			body:       `{"versionId":"ver_1"}`,
			role:       "publisher",
			wantStatus: http.StatusAccepted,
			configure: func(stub *publishServiceStub) {
				stub.rollbackFn = func(_ context.Context, actor domain.Principal, versionID, key string) (domain.PublishJob, error) {
					if actor.Subject != "editor-1" || versionID != "ver_1" || key != "idem-1234" {
						t.Fatalf("unexpected rollback input: actor=%q version=%q key=%q", actor.Subject, versionID, key)
					}
					return testJob(), nil
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			publishService := &publishServiceStub{}
			test.configure(publishService)
			recorder := httptest.NewRecorder()
			request := authenticatedRequest(test.method, test.path, test.body, test.role)
			if test.method == http.MethodPost {
				request.Header.Set("Idempotency-Key", " idem-1234 ")
			}

			testRouter(t, &contentServiceStub{}, publishService).ServeHTTP(recorder, request)

			if recorder.Code != test.wantStatus {
				t.Fatalf("expected %d, got %d body=%s", test.wantStatus, recorder.Code, recorder.Body.String())
			}
			var response publishResponse
			if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
				t.Fatalf("decode publish response: %v", err)
			}
			if response.ID != "pub_1" || response.VersionID != "ver_1" || response.SnapshotChecksum != "sha256:test" {
				t.Fatalf("unexpected publish response %#v", response)
			}
		})
	}
}

func TestDomainErrorsUseStableHTTPContract(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{name: "forbidden", err: domain.ErrForbidden, wantStatus: http.StatusForbidden, wantCode: "forbidden"},
		{name: "not found", err: domain.ErrNotFound, wantStatus: http.StatusNotFound, wantCode: "not_found"},
		{name: "conflict", err: domain.ErrConflict, wantStatus: http.StatusConflict, wantCode: "conflict"},
		{name: "invalid transition", err: domain.ErrInvalidTransition, wantStatus: http.StatusConflict, wantCode: "invalid_state"},
		{name: "invalid input", err: domain.ErrInvalidInput, wantStatus: http.StatusBadRequest, wantCode: "invalid_request"},
		{name: "unexpected", err: errors.New("storage unavailable"), wantStatus: http.StatusInternalServerError, wantCode: "internal_error"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			content := &contentServiceStub{getFn: func(context.Context, domain.Principal, string) (domain.ContentVersion, error) {
				return domain.ContentVersion{}, test.err
			}}
			recorder := httptest.NewRecorder()
			request := authenticatedRequest(http.MethodGet, "/api/v1/versions/ver_missing", "", "editor")

			testRouter(t, content, &publishServiceStub{}).ServeHTTP(recorder, request)

			if recorder.Code != test.wantStatus {
				t.Fatalf("expected %d, got %d body=%s", test.wantStatus, recorder.Code, recorder.Body.String())
			}
			response := decodeError(t, recorder)
			if response["code"] != test.wantCode || response["requestId"] != "req-test" {
				t.Fatalf("unexpected error response %#v", response)
			}
		})
	}
}

func TestMutationRoutesPropagateServiceErrors(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		role       string
		headers    map[string]string
		content    ContentService
		assets     AssetService
		publish    PublishService
		wantStatus int
		wantCode   string
	}{
		{
			name: "submit review invalid transition", method: http.MethodPost, path: "/api/v1/versions/ver_1/review",
			body: `{"revision":2}`, role: "editor", wantStatus: http.StatusConflict, wantCode: "invalid_state",
			content: &contentServiceStub{submitFn: func(context.Context, domain.Principal, string, int64) (domain.ContentVersion, error) {
				return domain.ContentVersion{}, domain.ErrInvalidTransition
			}},
		},
		{
			name: "approve stale revision", method: http.MethodPost, path: "/api/v1/versions/ver_1/approve",
			body: `{"revision":2}`, role: "reviewer", wantStatus: http.StatusConflict, wantCode: "conflict",
			content: &contentServiceStub{approveFn: func(context.Context, domain.Principal, string, int64) (domain.ContentVersion, error) {
				return domain.ContentVersion{}, domain.ErrConflict
			}},
		},
		{
			name: "reject invalid reason", method: http.MethodPost, path: "/api/v1/versions/ver_1/reject",
			body: `{"revision":2,"reason":""}`, role: "reviewer", wantStatus: http.StatusBadRequest, wantCode: "invalid_request",
			content: &contentServiceStub{rejectFn: func(context.Context, domain.Principal, string, int64, string) (domain.ContentVersion, error) {
				return domain.ContentVersion{}, domain.ErrInvalidInput
			}},
		},
		{
			name: "create asset invalid metadata", method: http.MethodPost, path: "/api/v1/assets/uploads",
			body: `{"fileName":"cover.webp","contentType":"image/webp","size":100,"checksum":"sha256:test","rights":{"source":{"zh-CN":"authorized"}}}`,
			role: "editor", wantStatus: http.StatusBadRequest, wantCode: "invalid_request",
			assets: &assetServiceStub{createFn: func(context.Context, domain.Principal, assets.CreateUploadInput) (assets.CreateUploadResult, error) {
				return assets.CreateUploadResult{}, domain.ErrInvalidInput
			}},
		},
		{
			name: "complete missing asset", method: http.MethodPost, path: "/api/v1/assets/asset_missing/complete",
			role: "editor", wantStatus: http.StatusNotFound, wantCode: "not_found",
			assets: &assetServiceStub{completeFn: func(context.Context, domain.Principal, string) (domain.AssetRecord, error) {
				return domain.AssetRecord{}, domain.ErrNotFound
			}},
		},
		{
			name: "delete active asset", method: http.MethodDelete, path: "/api/v1/assets/asset_1",
			role: "admin", wantStatus: http.StatusConflict, wantCode: "conflict",
			assets: &assetServiceStub{deleteFn: func(context.Context, domain.Principal, string) error {
				return domain.ErrConflict
			}},
		},
		{
			name: "create publish invalid transition", method: http.MethodPost, path: "/api/v1/publishes",
			body: `{"versionId":"ver_1"}`, role: "publisher", headers: map[string]string{"Idempotency-Key": "idem-1234"},
			wantStatus: http.StatusConflict, wantCode: "invalid_state",
			publish: &publishServiceStub{publishFn: func(context.Context, domain.Principal, string, string) (domain.PublishJob, error) {
				return domain.PublishJob{}, domain.ErrInvalidTransition
			}},
		},
		{
			name: "refresh missing publish", method: http.MethodPost, path: "/api/v1/publishes/pub_missing/refresh",
			role: "publisher", wantStatus: http.StatusNotFound, wantCode: "not_found",
			publish: &publishServiceStub{refreshFn: func(context.Context, domain.Principal, string) (domain.PublishJob, error) {
				return domain.PublishJob{}, domain.ErrNotFound
			}},
		},
		{
			name: "rollback forbidden", method: http.MethodPost, path: "/api/v1/rollbacks",
			body: `{"versionId":"ver_1"}`, role: "publisher", headers: map[string]string{"Idempotency-Key": "idem-1234"},
			wantStatus: http.StatusForbidden, wantCode: "forbidden",
			publish: &publishServiceStub{rollbackFn: func(context.Context, domain.Principal, string, string) (domain.PublishJob, error) {
				return domain.PublishJob{}, domain.ErrForbidden
			}},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := authenticatedRequest(test.method, test.path, test.body, test.role)
			for name, value := range test.headers {
				request.Header.Set(name, value)
			}

			testRouterWithAssets(t, test.content, test.assets, test.publish).ServeHTTP(recorder, request)

			if recorder.Code != test.wantStatus {
				t.Fatalf("expected %d, got %d body=%s", test.wantStatus, recorder.Code, recorder.Body.String())
			}
			if response := decodeError(t, recorder); response["code"] != test.wantCode || response["requestId"] != "req-routes" {
				t.Fatalf("unexpected error response %#v", response)
			}
		})
	}
}

func TestJSONDecoderRejectsTrailingValue(t *testing.T) {
	called := false
	content := &contentServiceStub{createFn: func(context.Context, domain.Principal, json.RawMessage) (domain.ContentVersion, error) {
		called = true
		return testVersion(), nil
	}}
	recorder := httptest.NewRecorder()
	request := authenticatedRequest(http.MethodPost, "/api/v1/versions", `{"snapshot":{}} {}`, "editor")

	testRouter(t, content, &publishServiceStub{}).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", recorder.Code, recorder.Body.String())
	}
	if called {
		t.Fatal("service must not receive a request with multiple JSON values")
	}
}

func TestConfiguredRouteWithoutServiceReturns500(t *testing.T) {
	tests := []struct {
		name    string
		method  string
		path    string
		body    string
		role    string
		headers map[string]string
	}{
		{name: "create version", method: http.MethodPost, path: "/api/v1/versions", body: `{"snapshot":{}}`, role: "editor"},
		{name: "complete asset", method: http.MethodPost, path: "/api/v1/assets/asset_1/complete", role: "editor"},
		{name: "get publish", method: http.MethodGet, path: "/api/v1/publishes/pub_1", role: "publisher"},
		{name: "rollback", method: http.MethodPost, path: "/api/v1/rollbacks", body: `{"versionId":"ver_1"}`, role: "publisher", headers: map[string]string{"Idempotency-Key": "idem-1234"}},
	}

	router := testRouterWithAssets(t, nil, nil, nil)
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := authenticatedRequest(test.method, test.path, test.body, test.role)
			for name, value := range test.headers {
				request.Header.Set(name, value)
			}

			router.ServeHTTP(recorder, request)

			if recorder.Code != http.StatusInternalServerError {
				t.Fatalf("expected 500, got %d body=%s", recorder.Code, recorder.Body.String())
			}
			if response := decodeError(t, recorder); response["code"] != "internal_error" {
				t.Fatalf("unexpected error response %#v", response)
			}
		})
	}
}

func TestRequestIDMiddlewareFallsBackWhenGeneratorReturnsBlank(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set("X-Request-ID", "  ")
	handler := requestIDMiddleware(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("X-Request-ID") != "req-unknown" {
			t.Fatalf("unexpected request ID in downstream handler %q", request.Header.Get("X-Request-ID"))
		}
		writer.WriteHeader(http.StatusNoContent)
	}), func(*http.Request) string { return "  " })

	handler.ServeHTTP(recorder, request)

	if recorder.Header().Get("X-Request-ID") != "req-unknown" {
		t.Fatalf("unexpected response request ID %q", recorder.Header().Get("X-Request-ID"))
	}
}

func TestHealthHandlerRejectsNonGETMethod(t *testing.T) {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/healthz", nil)
	request.Header.Set("X-Request-ID", "req-health")

	healthHandler(recorder, request)

	if recorder.Code != http.StatusMethodNotAllowed || recorder.Header().Get("Allow") != http.MethodGet {
		t.Fatalf("expected GET-only 405 response, got %d allow=%q", recorder.Code, recorder.Header().Get("Allow"))
	}
	if response := decodeError(t, recorder); response["requestId"] != "req-health" || response["code"] != "method_not_allowed" {
		t.Fatalf("unexpected health error response %#v", response)
	}
}
