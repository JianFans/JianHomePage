package main

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"yujian.me/server/internal/config"
)

// TestAssetListRejectsNULCursor verifies invalid text is rejected before storage
// access and uses the documented client-error response for malformed cursors.
func TestAssetListRejectsNULCursor(t *testing.T) {
	dependencies := developmentDependencies()
	t.Cleanup(func() { _ = dependencies.Close() })
	handler, err := buildHandler(config.Config{Environment: "development", AllowDevIdentity: true}, dependencies)
	if err != nil {
		t.Fatal(err)
	}
	cursor := base64.RawURLEncoding.EncodeToString([]byte(`{"createdAt":"2026-10-09T00:00:00Z","id":"asset_\u0000"}`))
	request := httptest.NewRequest(http.MethodGet, "/api/v1/assets?cursor="+cursor, nil)
	devHeaders(request, "editor-1", "editor")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest || response.Body.String() == "" {
		t.Fatalf("NUL cursor: status=%d body=%s", response.Code, response.Body.String())
	}
	var body struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Code != "invalid_request" {
		t.Fatalf("NUL cursor error code: %q", body.Code)
	}
}
