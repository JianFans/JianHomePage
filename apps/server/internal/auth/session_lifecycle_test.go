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
	"yujian.me/server/internal/domain"
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

// TestCookieSessionReadFailuresRemainRetryable distinguishes authentication
// loss from infrastructure failures without disclosing storage error details.
func TestCookieSessionReadFailuresRemainRetryable(t *testing.T) {
	store := &failedSessionStore{SessionStore: memory.NewSessionStore()}
	if err := store.CreateSession(context.Background(), auth.Session{TokenHash: auth.SessionTokenHash("cookie"), Principal: auth.SessionPrincipal{Subject: "editor", Roles: []string{"editor"}}, CSRFToken: "csrf", ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	manager, err := auth.NewSessions(auth.SessionOptions{Issuer: "https://issuer.test", ClientID: "client", RedirectURL: "https://api.test/api/v1/auth/callback", ReturnURL: "https://admin.test/", AllowedOrigins: []string{"https://admin.test"}, Key: make([]byte, 32), Store: store, Secure: true})
	if err != nil {
		t.Fatal(err)
	}
	middleware, err := auth.NewMiddleware(auth.MiddlewareOptions{Sessions: manager})
	if err != nil {
		t.Fatal(err)
	}
	called := 0
	protected := middleware.Authenticate(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called++
		w.WriteHeader(http.StatusNoContent)
	}))
	request := httptest.NewRequest("GET", "/api/v1/versions", nil)
	request.AddCookie(&http.Cookie{Name: "yujian_session", Value: "cookie"})
	response := httptest.NewRecorder()
	protected.ServeHTTP(response, request.Clone(context.Background()))
	if response.Code != http.StatusNoContent || called != 1 {
		t.Fatal("valid session was rejected before the outage")
	}
	for _, test := range []struct {
		name   string
		err    error
		status int
	}{
		{"unavailable", errors.New("private database error"), http.StatusServiceUnavailable},
		{"cancelled", context.Canceled, http.StatusServiceUnavailable},
		{"revoked", domain.ErrNotFound, http.StatusUnauthorized},
		{"invalid", domain.ErrForbidden, http.StatusUnauthorized},
	} {
		t.Run(test.name, func(t *testing.T) {
			logs := captureAuthLogs(t)
			store.readErr = test.err
			for _, endpoint := range []string{"protected", "logout"} {
				var handler http.Handler = protected
				r := request.Clone(context.Background())
				if endpoint == "logout" {
					handler = manager
					r = httptest.NewRequest("POST", "/api/v1/auth/logout", nil)
					r.AddCookie(&http.Cookie{Name: "yujian_session", Value: "cookie"})
					r.Header.Set("Origin", "https://admin.test")
					r.Header.Set("X-CSRF-Token", "csrf")
				}
				response := httptest.NewRecorder()
				r.Header.Set("X-Request-ID", "req-auth-test")
				handler.ServeHTTP(response, r)
				if endpoint == "protected" {
					reason, level := "session_rejected", "WARN"
					if test.status == http.StatusServiceUnavailable {
						reason, level = "store_unavailable", "ERROR"
					}
					assertAuthLog(t, logs, "cookie.verify", reason, level)
				}
				if response.Code != test.status || strings.Contains(response.Body.String(), "private") || len(response.Result().Cookies()) != 0 {
					t.Errorf("%s: got %d %s, want safe %d without cookie changes", endpoint, response.Code, response.Body.String(), test.status)
				}
			}
		})
	}
	store.readErr = nil
	response = httptest.NewRecorder()
	protected.ServeHTTP(response, request.Clone(context.Background()))
	if response.Code != http.StatusNoContent || called != 2 {
		t.Fatal("session did not recover or a failed request reached the handler")
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
