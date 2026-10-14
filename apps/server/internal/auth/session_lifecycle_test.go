package auth_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"yujian.me/server/internal/auth"
	"yujian.me/server/internal/store/memory"
)

type failedSessionStore struct {
	auth.SessionStore
	readErr, deleteErr error
}

// GetSession models storage unavailability without replacing valid identities.
func (store *failedSessionStore) GetSession(ctx context.Context, hash string) (auth.Session, error) {
	if store.readErr != nil {
		return auth.Session{}, store.readErr
	}
	return store.SessionStore.GetSession(ctx, hash)
}

// DeleteSession preserves the record when durable revocation fails.
func (store *failedSessionStore) DeleteSession(ctx context.Context, hash string) error {
	if store.deleteErr != nil {
		return store.deleteErr
	}
	return store.SessionStore.DeleteSession(ctx, hash)
}

// TestSessionStorageErrorsFailClosed checks unavailable lookup and revocation
// without logging or returning adapter errors that may contain private data.
func TestSessionStorageErrorsFailClosed(t *testing.T) {
	store := &failedSessionStore{SessionStore: memory.NewSessionStore()}
	store.CreateSession(context.Background(), auth.Session{TokenHash: auth.SessionTokenHash("cookie"), Principal: auth.SessionPrincipal{Subject: "editor", Roles: []string{"editor"}}, CSRFToken: "csrf", ExpiresAt: time.Now().Add(time.Hour)})
	manager, err := auth.NewSessions(auth.SessionOptions{Issuer: "https://issuer.test", ClientID: "client", RedirectURL: "https://api.test/api/v1/auth/callback", ReturnURL: "https://admin.test/", AllowedOrigins: []string{"https://admin.test"}, Key: make([]byte, 32), Store: store, Secure: true})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest("GET", "/api/v1/auth/session", nil)
	request.AddCookie(&http.Cookie{Name: "yujian_session", Value: "cookie"})
	store.readErr = errors.New("private database error")
	response := httptest.NewRecorder()
	manager.ServeHTTP(response, request)
	if response.Code != 503 || strings.Contains(response.Body.String(), "private") {
		t.Fatalf("storage error leaked: %d %s", response.Code, response.Body.String())
	}
	store.readErr = nil
	store.deleteErr = errors.New("private revocation error")
	logout := httptest.NewRequest("POST", "/api/v1/auth/logout", nil)
	logout.AddCookie(&http.Cookie{Name: "yujian_session", Value: "cookie"})
	logout.Header.Set("Origin", "https://admin.test")
	logout.Header.Set("X-CSRF-Token", "csrf")
	response = httptest.NewRecorder()
	manager.ServeHTTP(response, logout)
	if response.Code != 503 || len(response.Result().Cookies()) > 0 || strings.Contains(response.Body.String(), "private") {
		t.Fatal("failed revocation reported success")
	}
	response = httptest.NewRecorder()
	manager.ServeHTTP(response, request)
	if !strings.Contains(response.Body.String(), `"authenticated":true`) {
		t.Fatal("failed revocation unexpectedly lost session")
	}
}

// TestProductionLoginCookieRequiresSecureHTTPS exercises the actual TLS
// discovery transport and cookie flags selected by production configuration.
func TestProductionLoginCookieRequiresSecureHTTPS(t *testing.T) {
	var issuer string
	provider := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]string{"issuer": issuer, "authorization_endpoint": issuer + "/authorize", "token_endpoint": issuer + "/token"})
	}))
	defer provider.Close()
	issuer = provider.URL
	manager, err := auth.NewSessions(auth.SessionOptions{Issuer: issuer, ClientID: "client", RedirectURL: "https://api.test/api/v1/auth/callback", ReturnURL: "https://admin.test/", AllowedOrigins: []string{"https://admin.test"}, Key: make([]byte, 32), Store: memory.NewSessionStore(), Secure: true, HTTPClient: provider.Client()})
	if err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	manager.ServeHTTP(response, httptest.NewRequest("GET", "/api/v1/auth/login", nil))
	cookies := response.Result().Cookies()
	if response.Code != 302 || len(cookies) != 1 || !cookies[0].Secure || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteLaxMode || cookies[0].Domain != "" || cookies[0].MaxAge != 600 {
		t.Fatalf("unsafe production login cookies %#v", cookies)
	}
}
