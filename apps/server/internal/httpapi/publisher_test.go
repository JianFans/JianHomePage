package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"yujian.me/server/internal/auth"
	"yujian.me/server/internal/domain"
	"yujian.me/server/internal/publisher"
)

// TestLatestPublishIncludesSafeBuildStage restores a task without exposing its frozen request or audit records.
func TestLatestPublishIncludesSafeBuildStage(t *testing.T) {
	job := domain.PublishJob{ID: "pub_0cfc3422-0088-4fa6-869c-af64574d3509", VersionID: "ver_2c2fb0af-5f91-4cc4-be43-6e8859b1ed8d", BuildID: "build_18c66304-8e14-4453-9f71-5a0bd40ed814", Status: domain.PublishBuilding, Operation: domain.PublishOperationPublish}
	service := &publishServiceStub{latestFn: func(context.Context, domain.Principal) (domain.PublishJob, error) { return job, nil }}
	middleware, _ := auth.NewMiddleware(auth.MiddlewareOptions{AllowDevIdentity: true})
	router := NewRouter(RouterOptions{Publish: service, Publisher: &publisherStub{}, Middleware: middleware})
	w := httptest.NewRecorder()
	router.ServeHTTP(w, authenticatedRequest(http.MethodGet, "/api/v1/publishes/latest", "", "publisher"))
	if w.Code != http.StatusOK {
		t.Fatalf("latest job unavailable: %d %s", w.Code, w.Body)
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	build, ok := body["build"].(map[string]any)
	if !ok || build["phase"] != "submitting" || body["operation"] != "publish" {
		t.Fatalf("missing diagnostics: %s", w.Body)
	}
	for _, key := range []string{"target", "request", "reconciliation"} {
		if _, ok := build[key]; ok {
			t.Fatalf("exposed internal %s", key)
		}
	}
}

// TestLatestPublishEmptyAndForbidden keeps an empty history distinct from unauthorized access.
func TestLatestPublishEmptyAndForbidden(t *testing.T) {
	for _, tc := range []struct {
		role   string
		status int
	}{{"publisher", 204}, {"editor", 403}} {
		w := httptest.NewRecorder()
		testRouter(t, &contentServiceStub{}, &publishServiceStub{}).ServeHTTP(w, authenticatedRequest(http.MethodGet, "/api/v1/publishes/latest", "", tc.role))
		if w.Code != tc.status {
			t.Fatalf("role %s: %d %s", tc.role, w.Code, w.Body)
		}
	}
}

type publisherStub struct {
	calls int
	input publisher.RecoveryInput
	err   error
}

// Inspect counts calls to detect authentication paths reaching the domain service.
func (s *publisherStub) Inspect(context.Context, string) (publisher.Task, error) {
	s.calls++
	return publisher.Task{ID: "build_18c66304-8e14-4453-9f71-5a0bd40ed814", Phase: "submitting"}, s.err
}

// Recover retains the authenticated audit input for HTTP boundary assertions.
func (s *publisherStub) Recover(_ context.Context, id string, input publisher.RecoveryInput) (publisher.Task, error) {
	s.calls++
	s.input = input
	return publisher.Task{ID: id, Phase: "failed"}, s.err
}

// TestPublisherRecoveryRequiresAdministrator prevents ordinary publishing
// permission from granting the right to resolve an uncertain cloud operation.
func TestPublisherRecoveryRequiresAdministrator(t *testing.T) {
	for _, role := range []string{"editor", "reviewer", "publisher", "admin"} {
		t.Run(role, func(t *testing.T) {
			service := &publisherStub{}
			middleware, _ := auth.NewMiddleware(auth.MiddlewareOptions{AllowDevIdentity: true})
			router := NewRouter(RouterOptions{Publisher: service, Middleware: middleware})
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, authenticatedRequest(http.MethodPost, "/api/v1/publisher/builds/build_18c66304-8e14-4453-9f71-5a0bd40ed814/recovery", `{"confirmedAbsent":true,"note":"Checked cloud console"}`, role))
			want := http.StatusForbidden
			if role == "admin" {
				want = http.StatusOK
			}
			if recorder.Code != want {
				t.Fatalf("status %d want %d: %s", recorder.Code, want, recorder.Body)
			}
			if role == "admin" && service.calls != 1 {
				t.Fatal("missing recovery call")
			}
			if role == "admin" && service.input.ActorSubject != "editor-1" {
				t.Fatal("recovery audit must use authenticated subject")
			}
			if role != "admin" && service.calls != 0 {
				t.Fatal("unauthorized recovery reached service")
			}
		})
	}
}

// TestPublisherRecoveryRejectsMalformedBody prevents spoofed audit identities
// and ambiguous confirmations from reaching the recovery service.
func TestPublisherRecoveryRejectsMalformedBody(t *testing.T) {
	middleware, _ := auth.NewMiddleware(auth.MiddlewareOptions{AllowDevIdentity: true})
	for _, body := range []string{`null`, `{`, `{"confirmedAbsent":null}`, `{"confirmedAbsent":true,"actorSubject":"forged"}`, `{"confirmedAbsent":true,"confirmedAbsent":false}`} {
		t.Run(body, func(t *testing.T) {
			service := &publisherStub{}
			recorder := httptest.NewRecorder()
			NewRouter(RouterOptions{Publisher: service, Middleware: middleware}).ServeHTTP(recorder, authenticatedRequest(http.MethodPost, "/api/v1/publisher/builds/build_18c66304-8e14-4453-9f71-5a0bd40ed814/recovery", body, "admin"))
			if recorder.Code != http.StatusBadRequest || service.calls != 0 {
				t.Fatalf("status=%d calls=%d: %s", recorder.Code, service.calls, recorder.Body)
			}
		})
	}
}

// TestPublisherEndpointsEnforceAuthenticationAndMapDomainErrors exercises both
// routes at their HTTP boundary, including successful frozen-state inspection.
func TestPublisherEndpointsEnforceAuthenticationAndMapDomainErrors(t *testing.T) {
	middleware, _ := auth.NewMiddleware(auth.MiddlewareOptions{AllowDevIdentity: true})
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		target := "/api/v1/publisher/builds/build_18c66304-8e14-4453-9f71-5a0bd40ed814"
		if method == http.MethodPost {
			target += "/recovery"
		}
		for _, scenario := range []struct {
			name      string
			err       error
			status    int
			anonymous bool
		}{
			{"success", nil, http.StatusOK, false},
			{"anonymous", nil, http.StatusUnauthorized, true},
			{"missing", domain.ErrNotFound, http.StatusNotFound, false},
			{"conflict", domain.ErrConflict, http.StatusConflict, false},
			{"invalid", domain.ErrInvalidInput, http.StatusBadRequest, false},
		} {
			t.Run(method+"/"+scenario.name, func(t *testing.T) {
				service := &publisherStub{err: scenario.err}
				request := authenticatedRequest(method, target, `{"confirmedAbsent":true,"note":"verified"}`, "admin")
				if scenario.anonymous {
					request.Header.Del("X-Dev-Subject")
					request.Header.Del("X-Dev-Roles")
				}
				recorder := httptest.NewRecorder()
				NewRouter(RouterOptions{Publisher: service, Middleware: middleware}).ServeHTTP(recorder, request)
				if recorder.Code != scenario.status {
					t.Fatalf("status=%d want=%d: %s", recorder.Code, scenario.status, recorder.Body)
				}
				if scenario.anonymous && service.calls != 0 {
					t.Fatal("anonymous request reached publisher")
				}
			})
		}
	}
}

// TestPublisherInspectionAndUnavailableService verifies development mode reports
// the missing durable publisher instead of manufacturing a successful result.
func TestPublisherInspectionAndUnavailableService(t *testing.T) {
	middleware, _ := auth.NewMiddleware(auth.MiddlewareOptions{AllowDevIdentity: true})
	for _, method := range []string{http.MethodGet, http.MethodPost} {
		target := "/api/v1/publisher/builds/build_18c66304-8e14-4453-9f71-5a0bd40ed814"
		body := ""
		if method == http.MethodPost {
			target += "/recovery"
			body = `{"confirmedAbsent":true,"note":"checked"}`
		}
		recorder := httptest.NewRecorder()
		NewRouter(RouterOptions{Middleware: middleware}).ServeHTTP(recorder, authenticatedRequest(method, target, body, "admin"))
		if recorder.Code != http.StatusServiceUnavailable {
			t.Fatalf("status %d: %s", recorder.Code, recorder.Body)
		}
	}
}
