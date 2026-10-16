package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"yujian.me/server/internal/auth"
	"yujian.me/server/internal/domain"
	"yujian.me/server/internal/publisher"
)

type publisherStub struct {
	calls int
	input publisher.RecoveryInput
	err   error
}

// Inspect counts calls to detect authentication paths reaching the domain service.
func (s *publisherStub) Inspect(context.Context, string) (publisher.Task, error) {
	s.calls++
	return publisher.Task{ID: "build_test", Phase: "submitting"}, s.err
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
			router.ServeHTTP(recorder, authenticatedRequest(http.MethodPost, "/api/v1/publisher/builds/build_test/recovery", `{"confirmedAbsent":true,"note":"Checked cloud console"}`, role))
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
			NewRouter(RouterOptions{Publisher: service, Middleware: middleware}).ServeHTTP(recorder, authenticatedRequest(http.MethodPost, "/api/v1/publisher/builds/build_test/recovery", body, "admin"))
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
		target := "/api/v1/publisher/builds/build_test"
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
		target := "/api/v1/publisher/builds/build_test"
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
