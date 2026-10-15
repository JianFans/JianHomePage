package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"yujian.me/server/internal/auth"
	"yujian.me/server/internal/config"
)

// TestBuildHandlerWiresOpaqueSessions exercises configured browser sessions
// through the real application router rather than a standalone auth handler.
func TestBuildHandlerWiresOpaqueSessions(t *testing.T) {
	dependencies := developmentDependencies()
	defer dependencies.Close()
	settings := config.Config{Environment: "development", OIDCIssuer: "http://issuer.local", AdminOIDCClientID: "client", AdminOIDCRedirectURL: "http://api.local/api/v1/auth/callback", AdminReturnURL: "http://admin.local/", AdminSessionKey: make([]byte, 32), AllowedAdminOrigins: []string{"http://admin.local"}}
	handler, err := buildHandler(settings, dependencies)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest("GET", "/api/v1/auth/session", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != 200 || !strings.Contains(response.Body.String(), `"loginAvailable":true`) {
		t.Fatalf("configured session %d %s", response.Code, response.Body.String())
	}
	dependencies.Sessions.CreateSession(context.Background(), auth.Session{TokenHash: auth.SessionTokenHash("opaque-cookie"), Principal: auth.SessionPrincipal{Subject: "editor", Roles: []string{"editor"}}, CSRFToken: "csrf", ExpiresAt: time.Now().Add(time.Hour)})
	request = httptest.NewRequest("GET", "/api/v1/versions", nil)
	request.AddCookie(&http.Cookie{Name: "yujian_session", Value: "opaque-cookie"})
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != 200 || !strings.Contains(response.Body.String(), `"items":[]`) {
		t.Fatalf("session management request %d %s", response.Code, response.Body.String())
	}
	settings.AdminSessionKey = nil
	if _, err := buildHandler(settings, dependencies); err == nil {
		t.Fatal("partial login configuration accepted by buildHandler")
	}
}
