package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"yujian.me/server/internal/config"
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
