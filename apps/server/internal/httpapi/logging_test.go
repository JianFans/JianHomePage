package httpapi

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"yujian.me/server/internal/auth"
	"yujian.me/server/internal/domain"
)

func TestLoggingMiddlewareIncludesRequestActorRouteStatusAndDuration(t *testing.T) {
	var output bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&output, nil))
	next := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusNoContent)
	})
	middleware, err := auth.NewMiddleware(auth.MiddlewareOptions{AllowDevIdentity: true})
	if err != nil {
		t.Fatalf("create middleware: %v", err)
	}
	handler := LoggingMiddleware(middleware.Authenticate(next), logger)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/versions/ver_1", nil)
	request.Header.Set("X-Request-ID", "req-logging")
	request.Header.Set("X-Dev-Subject", "admin-1")
	request.Header.Set("X-Dev-Roles", "admin")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("unexpected status %d", recorder.Code)
	}
	for _, fragment := range []string{"request_id", "req-logging", "actor_subject", "admin-1", "route", "/api/v1/versions/ver_1", "status", "duration_ms"} {
		if !strings.Contains(output.String(), fragment) {
			t.Fatalf("log missing %q: %s", fragment, output.String())
		}
	}
	if strings.Contains(output.String(), "Authorization") || strings.Contains(output.String(), "token") {
		t.Fatalf("log contains sensitive token material: %s", output.String())
	}
}

var _ = domain.Principal{}
