package httpapi

import (
	"net/http/httptest"
	"strings"
	"testing"
)

// TestSessionRouteIsPublicAndCORSAllowsCredentials checks anonymous capability
// discovery and exact-origin preflight support for cookie-authenticated writes.
func TestSessionRouteIsPublicAndCORSAllowsCredentials(t *testing.T) {
	router := NewRouter(RouterOptions{AllowedOrigins: []string{"http://admin.local"}})
	request := httptest.NewRequest("GET", "/api/v1/auth/session", nil)
	request.Header.Set("Origin", "http://admin.local")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != 200 || !strings.Contains(response.Body.String(), `"loginAvailable":false`) {
		t.Fatalf("session endpoint: %d %s", response.Code, response.Body.String())
	}
	if response.Header().Get("Access-Control-Allow-Credentials") != "true" {
		t.Fatal("credentialed CORS unavailable")
	}
	request = httptest.NewRequest("OPTIONS", "/api/v1/versions", nil)
	request.Header.Set("Origin", "http://admin.local")
	request.Header.Set("Access-Control-Request-Method", "POST")
	response = httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if !strings.Contains(response.Header().Get("Access-Control-Allow-Headers"), "X-CSRF-Token") {
		t.Fatal("csrf header not allowed")
	}
}
