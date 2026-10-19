package httpapi

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLoggingMiddlewareRecordsImplicitOKAndAnonymousActor(t *testing.T) {
	var output bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&output, nil))
	handler := LoggingMiddleware(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		if _, err := writer.Write([]byte("ok")); err != nil {
			t.Fatalf("write response: %v", err)
		}
		writer.WriteHeader(http.StatusCreated)
	}), logger)
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/anonymous", nil))

	if recorder.Code != http.StatusOK || recorder.Body.String() != "ok" {
		t.Fatalf("unexpected response status=%d body=%q", recorder.Code, recorder.Body.String())
	}
	for _, fragment := range []string{`"actor_subject":""`, `"status":200`, `"route":"unmatched"`} {
		if !strings.Contains(output.String(), fragment) {
			t.Fatalf("log missing %q: %s", fragment, output.String())
		}
	}
}

// TestUnmatchedRequestDoesNotLogCallerControlledPath keeps credentials in arbitrary paths out of logs.
func TestUnmatchedRequestDoesNotLogCallerControlledPath(t *testing.T) {
	var output bytes.Buffer
	handler := LoggingMiddleware(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusNotFound)
	}), slog.New(slog.NewJSONHandler(&output, nil)))
	request := httptest.NewRequest(http.MethodGet, "/api/v1/auth/callback/private-code?state=private-state", nil)
	handler.ServeHTTP(httptest.NewRecorder(), request)

	entries := logEntries(t, &output)
	if len(entries) != 1 || entries[0]["route"] != "unmatched" {
		t.Fatalf("unexpected unmatched route diagnostics: %s", output.String())
	}
	for _, secret := range []string{"private-code", "private-state"} {
		if strings.Contains(output.String(), secret) {
			t.Fatalf("caller-controlled path leaked into logs: %s", output.String())
		}
	}
}

func TestStatusWriterDefaultsToOKBeforeWriting(t *testing.T) {
	recorder := httptest.NewRecorder()
	writer := &statusWriter{ResponseWriter: recorder}

	if writer.statusCode() != http.StatusOK {
		t.Fatalf("expected default status 200, got %d", writer.statusCode())
	}
}
