package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"yujian.me/server/internal/domain"
)

// logEntries decodes complete records so assertions cannot pass on fragments of unrelated events.
func logEntries(t *testing.T, output *bytes.Buffer) []map[string]any {
	t.Helper()
	var entries []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(output.String()), "\n") {
		if line == "" {
			continue
		}
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatal(err)
		}
		entries = append(entries, entry)
	}
	return entries
}

// TestRequestLogsClassifyStatusAndSuppressProbeNoise checks useful request context without query/header disclosure.
func TestRequestLogsClassifyStatusAndSuppressProbeNoise(t *testing.T) {
	for _, tc := range []struct {
		path   string
		status int
		level  string
	}{
		{"/api/v1/versions/ver_1?code=private-code", 200, "INFO"},
		{"/api/v1/versions/ver_1", 409, "WARN"},
		{"/api/v1/versions/ver_1", 503, "ERROR"},
		{"/healthz", 200, "DEBUG"},
		{"/_nuxt/entry-123.js", 200, "DEBUG"},
	} {
		t.Run(tc.path+tc.level, func(t *testing.T) {
			var output bytes.Buffer
			logger := slog.New(slog.NewJSONHandler(&output, &slog.HandlerOptions{Level: slog.LevelDebug}))
			handler := LoggingMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte("hello"))
			}), logger)
			r := httptest.NewRequest("GET", tc.path, nil)
			r.Header.Set("Cookie", "private-cookie")
			r.Header.Set("Authorization", "Bearer private-token")
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, r)
			entries := logEntries(t, &output)
			if len(entries) != 1 {
				t.Fatalf("unexpected logs: %s", output.String())
			}
			e := entries[0]
			if e["level"] != tc.level || e["method"] != "GET" || e["response_bytes"] != float64(5) || e["request_id"] == "" || e["request_id"] != w.Header().Get("X-Request-ID") {
				t.Fatalf("incomplete request diagnostics: %#v", e)
			}
			for _, secret := range []string{"private-code", "private-cookie", "private-token"} {
				if strings.Contains(output.String(), secret) {
					t.Fatalf("request data leaked: %s", secret)
				}
			}
		})
	}
}

func TestRequestLogsReplaceCookieShapedRequestIDs(t *testing.T) {
	cookie := strings.Repeat("a", 43)
	var output bytes.Buffer
	inner := requestIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}), func(*http.Request) string { return "req-generated" })
	handler := LoggingMiddleware(inner, slog.New(slog.NewJSONHandler(&output, nil)))
	request := httptest.NewRequest(http.MethodGet, "/api/v1/auth/session", nil)
	request.Header.Set("X-Request-ID", cookie)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, request)

	if got := w.Header().Get("X-Request-ID"); got != "req-generated" {
		t.Fatalf("response request ID = %q", got)
	}
	entries := logEntries(t, &output)
	if len(entries) != 1 || entries[0]["request_id"] != "req-generated" {
		t.Fatalf("unexpected request diagnostics: %s", output.String())
	}
	if strings.Contains(output.String(), cookie) {
		t.Fatalf("cookie-shaped request ID leaked to logs: %s", output.String())
	}
}

// TestInternalFailureLogsRetainCauseAndResource keeps public errors generic while recording the failed domain lookup.
func TestInternalFailureLogsRetainCauseAndResource(t *testing.T) {
	var output bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&output, nil))
	service := &contentServiceStub{getFn: func(context.Context, domain.Principal, string) (domain.ContentVersion, error) {
		return domain.ContentVersion{}, errors.New("read version: connection pool exhausted")
	}}
	handler := LoggingMiddleware(testRouter(t, service, nil), logger)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, authenticatedRequest("GET", "/api/v1/versions/ver_missing", "", "admin"))
	if w.Code != 500 || strings.Contains(w.Body.String(), "pool exhausted") {
		t.Fatalf("unsafe public response: %s", w.Body.String())
	}
	var diagnostic map[string]any
	for _, e := range logEntries(t, &output) {
		if e["msg"] == "management operation failed" {
			diagnostic = e
		}
	}
	if diagnostic == nil || diagnostic["level"] != "ERROR" || diagnostic["version_id"] != "ver_missing" || diagnostic["actor_subject"] != "editor-1" || !strings.Contains(output.String(), "connection pool exhausted") {
		t.Fatalf("missing failure context: %s", output.String())
	}
}

// TestExpectedInputErrorsDoNotLogCallerSnapshotDetails keeps validation payloads out of diagnostics.
func TestExpectedInputErrorsDoNotLogCallerSnapshotDetails(t *testing.T) {
	var output bytes.Buffer
	handler := LoggingMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		err := errors.Join(domain.ErrInvalidInput, errors.New("invalid snapshot field private-snapshot-title"))
		logOperation(r, err, http.StatusBadRequest, "invalid_request", "management operation failed")
		writeError(w, r, http.StatusBadRequest, "invalid_request", "Request data is invalid.")
	}), slog.New(slog.NewJSONHandler(&output, nil)))
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/api/v1/versions", nil))

	if strings.Contains(output.String(), "private-snapshot-title") || !strings.Contains(output.String(), `"error_kind":"invalid_request"`) {
		t.Fatalf("unsafe or incomplete input diagnostic: %s", output.String())
	}
}

// TestOperationAndRequestLogsShareTheConfiguredRequestID verifies middleware order preserves request correlation.
func TestOperationAndRequestLogsShareTheConfiguredRequestID(t *testing.T) {
	var output bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&output, nil))
	inner := requestIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logOperation(r, nil, http.StatusBadRequest, "invalid_request", "management request rejected")
		w.WriteHeader(http.StatusBadRequest)
	}), func(*http.Request) string { return "req-configured" })
	handler := LoggingMiddleware(inner, logger)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("GET", "/test", nil))
	if w.Header().Get("X-Request-ID") != "req-configured" {
		t.Fatalf("response request ID = %q", w.Header().Get("X-Request-ID"))
	}
	entries := logEntries(t, &output)
	if len(entries) != 2 || entries[0]["request_id"] != "req-configured" || entries[1]["request_id"] != "req-configured" {
		t.Fatalf("operation and request log IDs differ: %#v", entries)
	}
	if got := strings.Count(output.String(), `"request_id"`); got != 2 {
		t.Fatalf("request ID appears more than once per record: %s", output.String())
	}
}

// TestResourceFieldsOnlyLogApplicationOwnedIDs avoids copying arbitrary path credentials into diagnostics.
func TestResourceFieldsOnlyLogApplicationOwnedIDs(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.SetPathValue("versionId", "ver_valid-1")
	r.SetPathValue("assetId", "asset_token=private-value")
	fields := resourceFields(r)
	if len(fields) != 2 || fields[0] != "version_id" || fields[1] != "ver_valid-1" {
		t.Fatalf("resource diagnostics = %#v", fields)
	}
}

// TestRequestPanicRecordsStackAndSafeFailure checks recovery and preservation of already committed responses.
func TestRequestPanicRecordsStackAndSafeFailure(t *testing.T) {
	for _, panicValue := range []any{"private-panic-payload", http.ErrAbortHandler} {
		for _, committed := range []bool{false, true} {
			var output bytes.Buffer
			handler := LoggingMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if committed {
					_, _ = w.Write([]byte("partial"))
				}
				panic(panicValue)
			}), slog.New(slog.NewJSONHandler(&output, nil)))
			w := httptest.NewRecorder()
			var propagated any
			func() {
				defer func() { propagated = recover() }()
				handler.ServeHTTP(w, httptest.NewRequest("GET", "/panic", nil))
			}()
			if panicValue == http.ErrAbortHandler {
				if propagated != http.ErrAbortHandler || strings.Contains(output.String(), "http handler panicked") {
					t.Fatalf("abort semantics changed: %v %s", propagated, output.String())
				}
			} else if committed {
				if propagated != http.ErrAbortHandler || w.Body.String() != "partial" {
					t.Fatalf("committed response corrupted: %v %s", propagated, w.Body.String())
				}
			} else if propagated != nil || w.Code != 500 {
				t.Fatalf("panic not recovered: %v %d", propagated, w.Code)
			}
			if panicValue != http.ErrAbortHandler && (!strings.Contains(output.String(), "http handler panicked") || !strings.Contains(output.String(), "operational_logging_test.go") || strings.Contains(output.String(), "private-panic-payload")) {
				t.Fatalf("missing or unsafe panic trace: %s", output.String())
			}
			requestLogs := 0
			for _, entry := range logEntries(t, &output) {
				if entry["msg"] != "http request" {
					continue
				}
				requestLogs++
				if entry["status"] != float64(w.Code) || entry["response_bytes"] != float64(w.Body.Len()) || entry["method"] != "GET" || entry["request_id"] != w.Header().Get("X-Request-ID") {
					t.Fatalf("incomplete panic request log: %#v", entry)
				}
			}
			if requestLogs != 1 {
				t.Fatalf("expected one request log, got %d: %s", requestLogs, output.String())
			}
		}
	}
}

// TestDomainErrorDiagnosticsMatchResponses keeps the public contract and log
// status consistent, including errors that match more than one domain sentinel.
func TestDomainErrorDiagnosticsMatchResponses(t *testing.T) {
	for _, tc := range []struct {
		name    string
		err     error
		status  int
		code    string
		message string
	}{
		{"forbidden", domain.ErrForbidden, 403, "forbidden", "Permission denied."},
		{"missing", domain.ErrNotFound, 404, "not_found", "Resource not found."},
		{"conflict", domain.ErrConflict, 409, "conflict", "Resource was modified by another request."},
		{"transition", domain.ErrInvalidTransition, 409, "invalid_state", "The requested state transition is not allowed."},
		{"input", domain.ErrInvalidInput, 400, "invalid_request", "Request data is invalid."},
		{"data integrity", fmt.Errorf("stored snapshot checksum mismatch: %w", domain.ErrDataIntegrity), 500, "internal_error", "Service unavailable."},
		{"internal", errors.New("private failure"), 500, "internal_error", "Service unavailable."},
		{"joined", errors.Join(domain.ErrNotFound, domain.ErrConflict), 404, "not_found", "Resource not found."},
	} {
		for _, handler := range []struct {
			name  string
			write func(http.ResponseWriter, *http.Request, error)
		}{
			{"domain", writeDomainError},
			{"publish", writePublishError},
		} {
			t.Run(handler.name+"/"+tc.name, func(t *testing.T) {
				var output bytes.Buffer
				response := httptest.NewRecorder()
				LoggingMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					handler.write(w, r, tc.err)
				}), slog.New(slog.NewJSONHandler(&output, nil))).ServeHTTP(response, httptest.NewRequest("POST", "/api/v1/publishes", nil))
				var body errorResponse
				if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
					t.Fatal(err)
				}
				if response.Code != tc.status || body.Code != tc.code || body.Message != tc.message {
					t.Fatalf("unexpected error response: %d %#v", response.Code, body)
				}
				entries := logEntries(t, &output)
				if len(entries) != 2 {
					t.Fatalf("unexpected diagnostics: %s", output.String())
				}
				for _, entry := range entries {
					if entry["status"] != float64(tc.status) {
						t.Fatalf("diagnostic disagrees with response: %#v", entry)
					}
				}
				if errors.Is(tc.err, domain.ErrDataIntegrity) && (!strings.Contains(output.String(), "stored snapshot checksum mismatch") || entries[0]["error"] == nil || strings.Contains(response.Body.String(), "stored snapshot")) {
					t.Fatalf("data integrity failure was not retained in operation diagnostics: %s", output.String())
				}
			})
		}
	}
}

// TestStatusWriterAllowsFinalResponseAfterInformationalStatus mirrors net/http's 1xx handling.
func TestStatusWriterAllowsFinalResponseAfterInformationalStatus(t *testing.T) {
	w := &statusWriter{ResponseWriter: httptest.NewRecorder()}
	w.WriteHeader(http.StatusEarlyHints)
	w.WriteHeader(http.StatusCreated)
	if w.statusCode() != http.StatusCreated {
		t.Fatalf("final status lost: %d", w.statusCode())
	}
}
