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
	for _, fragment := range []string{`"actor_subject":""`, `"status":200`, `"route":"/anonymous"`} {
		if !strings.Contains(output.String(), fragment) {
			t.Fatalf("log missing %q: %s", fragment, output.String())
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
