package auth_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"yujian.me/server/internal/auth"
	"yujian.me/server/internal/config"
	"yujian.me/server/internal/domain"
	"yujian.me/server/internal/observability"
	"yujian.me/server/internal/store/memory"
)

// captureAuthLogs captures all levels without letting tests log credentials to
// stderr; each test restores the process logger before returning.
func captureAuthLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(previous) })
	return &logs
}

// assertAuthLog checks structured reasons and request correlation independently
// of human-readable messages and excludes credential values from every event.
func assertAuthLog(t *testing.T, logs *bytes.Buffer, stage, reason, level string) {
	t.Helper()
	for _, line := range strings.Split(strings.TrimSpace(logs.String()), "\n") {
		var entry map[string]any
		if json.Unmarshal([]byte(line), &entry) == nil && entry["stage"] == stage && entry["reason"] == reason && entry["level"] == level && entry["request_id"] == "req-auth-test" {
			return
		}
	}
	t.Fatalf("missing %s %s/%s auth event: %s", level, stage, reason, logs.String())
}

// TestCallbackMissingCookieHasSafeReason preserves the generic 401 response
// while making the missing browser flow distinguishable in server diagnostics.
func TestCallbackMissingCookieHasSafeReason(t *testing.T) {
	logs := captureAuthLogs(t)
	manager, err := auth.NewSessions(auth.SessionOptions{Issuer: "https://issuer.test", ClientID: "client", RedirectURL: "https://api.test/api/v1/auth/callback", ReturnURL: "https://admin.test/", AllowedOrigins: []string{"https://admin.test"}, Key: make([]byte, 32), Store: memory.NewSessionStore(), Secure: true})
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("GET", "/api/v1/auth/callback?code=private-code&state=private-state", nil)
	r.Header.Set("X-Request-ID", "req-auth-test")
	w := httptest.NewRecorder()
	manager.ServeHTTP(w, r)
	if w.Code != 401 {
		t.Fatal(w.Code)
	}
	assertAuthLog(t, logs, "callback.cookie", "missing_cookie", "WARN")
	if strings.Contains(logs.String(), "private-code") || strings.Contains(logs.String(), "private-state") {
		t.Fatal("callback secrets leaked")
	}
}

// TestBearerClaimFailuresHaveDistinctReasons makes signed-token rejection
// actionable without logging the token or claims supplied by the caller.
func TestBearerClaimFailuresHaveDistinctReasons(t *testing.T) {
	logs := captureAuthLogs(t)
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_700_000_000, 0)
	server := oidcTestServer(key, func() time.Time { return now })
	defer server.Close()
	provider, err := auth.NewOIDCProvider(auth.OIDCConfig{Issuer: server.URL, Audience: "client", Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	middleware, err := auth.NewMiddleware(auth.MiddlewareOptions{IdentityProvider: provider})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		claim  string
		value  any
		reason string
	}{
		{"iss", "https://private-issuer.test", "issuer_mismatch"},
		{"aud", "private-audience", "audience_mismatch"},
		{"exp", now.Add(-time.Hour).Unix(), "token_expired"},
		{"roles", []string{"private-role"}, "roles_invalid"},
	} {
		t.Run(tc.reason, func(t *testing.T) {
			logs.Reset()
			claims := map[string]any{"iss": server.URL, "aud": "client", "sub": "private-unverified-sub", "exp": now.Add(time.Hour).Unix(), "roles": []string{"editor"}}
			claims[tc.claim] = tc.value
			token := signJWT(t, key, claims)
			r := httptest.NewRequest("GET", "/api/v1/versions", nil)
			r.Header.Set("Authorization", "Bearer "+token)
			r.Header.Set("X-Request-ID", "req-auth-test")
			w := httptest.NewRecorder()
			middleware.Authenticate(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Fatal("rejected token reached handler") })).ServeHTTP(w, r)
			if w.Code != 401 {
				t.Fatal(w.Code)
			}
			assertAuthLog(t, logs, "bearer.verify", tc.reason, "WARN")
			for _, secret := range []string{token, "private-unverified-sub", "private-issuer", "private-audience", "private-role"} {
				if strings.Contains(logs.String(), secret) {
					t.Fatalf("claim leaked: %s", secret)
				}
			}
		})
	}
}

// TestSessionPollingLogsAtDebug keeps routine anonymous checks out of INFO logs.
func TestSessionPollingLogsAtDebug(t *testing.T) {
	logs := captureAuthLogs(t)
	manager, err := auth.NewSessions(auth.SessionOptions{})
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("GET", "/api/v1/auth/session", nil)
	r.Header.Set("X-Request-ID", "req-auth-test")
	w := httptest.NewRecorder()
	manager.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
	assertAuthLog(t, logs, "session.read", "anonymous", "DEBUG")
}

// TestBoundRequestLoggerDoesNotDuplicateRequestID keeps JSON request correlation unambiguous.
func TestBoundRequestLoggerDoesNotDuplicateRequestID(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	manager, err := auth.NewSessions(auth.SessionOptions{})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest("GET", "/api/v1/auth/session", nil)
	request = request.WithContext(observability.WithLogger(request.Context(), logger.With("request_id", "req-auth-test")))
	response := httptest.NewRecorder()
	manager.ServeHTTP(response, request)
	if response.Code != http.StatusOK || strings.Count(logs.String(), `"request_id"`) != 1 {
		t.Fatalf("bound request ID was duplicated or lost: %s", logs.String())
	}
}

// TestMalformedSessionCookieHasSafeReason distinguishes corrupt cookies from
// normal anonymous session polling without logging the cookie value.
func TestMalformedSessionCookieHasSafeReason(t *testing.T) {
	logs := captureAuthLogs(t)
	manager, err := auth.NewSessions(auth.SessionOptions{Issuer: "https://issuer.test", ClientID: "client", RedirectURL: "https://api.test/api/v1/auth/callback", ReturnURL: "https://admin.test/", AllowedOrigins: []string{"https://admin.test"}, Key: make([]byte, 32), Store: memory.NewSessionStore(), Secure: true})
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest("GET", "/api/v1/auth/session", nil)
	r.AddCookie(&http.Cookie{Name: "yujian_session", Value: strings.Repeat("a", 129)})
	r.Header.Set("X-Request-ID", "req-auth-test")
	w := httptest.NewRecorder()
	manager.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatal(w.Code)
	}
	assertAuthLog(t, logs, "session.read", "cookie_invalid", "WARN")
}

// TestAuthorizationDenialLogsOnlyOAuthCode verifies callback errors are
// correlated to a known OAuth code after state validation, not descriptions.
func TestAuthorizationDenialLogsOnlyOAuthCode(t *testing.T) {
	logs := captureAuthLogs(t)
	manager, callbackURL, flowCookie, state := newLoggingTestFlow(t)
	callback := httptest.NewRequest("GET", callbackURL+"?error=access_denied&error_description=private-description&state="+url.QueryEscape(state), nil)
	callback.Header.Set("X-Request-ID", "req-auth-test")
	callback.AddCookie(flowCookie)
	response := httptest.NewRecorder()
	manager.ServeHTTP(response, callback)
	if response.Code != http.StatusUnauthorized {
		t.Fatal(response.Code)
	}
	assertAuthLog(t, logs, "callback.authorization", "authorization_access_denied", "WARN")
	for _, secret := range []string{"private-description", state, flowCookie.Value} {
		if strings.Contains(logs.String(), secret) {
			t.Fatalf("OAuth callback detail leaked: %s", secret)
		}
	}
}

// TestCallbackStateMismatchHasSafeReason distinguishes CSRF state mismatch from
// malformed callback parameters without recording either state value.
func TestCallbackStateMismatchHasSafeReason(t *testing.T) {
	logs := captureAuthLogs(t)
	manager, callbackURL, flowCookie, state := newLoggingTestFlow(t)
	callback := httptest.NewRequest("GET", callbackURL+"?code=private-code&state="+url.QueryEscape("wrong-"+state), nil)
	callback.Header.Set("X-Request-ID", "req-auth-test")
	callback.AddCookie(flowCookie)
	response := httptest.NewRecorder()
	manager.ServeHTTP(response, callback)
	if response.Code != http.StatusUnauthorized {
		t.Fatal(response.Code)
	}
	assertAuthLog(t, logs, "callback.state", "state_mismatch", "WARN")
	for _, secret := range []string{"private-code", state, flowCookie.Value} {
		if strings.Contains(logs.String(), secret) {
			t.Fatalf("callback state leaked: %s", secret)
		}
	}
}

// TestLoginDiscoveryFailureLogsValidatedConfigurationReason makes malformed
// provider metadata distinguishable from network failures.
func TestLoginDiscoveryFailureLogsValidatedConfigurationReason(t *testing.T) {
	logs := captureAuthLogs(t)
	issuer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{
			"issuer":                 "http://" + r.Host,
			"authorization_endpoint": "javascript:alert(1)",
			"token_endpoint":         "http://" + r.Host + "/token",
		})
	}))
	defer issuer.Close()
	manager, err := auth.NewSessions(auth.SessionOptions{
		Issuer: issuer.URL, ClientID: "client", RedirectURL: "http://api.test/api/v1/auth/callback",
		ReturnURL: "http://admin.test/", AllowedOrigins: []string{"http://admin.test"},
		Key: make([]byte, 32), Store: memory.NewSessionStore(),
	})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest("GET", "/api/v1/auth/login", nil)
	request.Header.Set("X-Request-ID", "req-auth-test")
	response := httptest.NewRecorder()
	manager.ServeHTTP(response, request)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatal(response.Code)
	}
	assertAuthLog(t, logs, "login.discovery", "authorization_endpoint_invalid", "ERROR")
}

// TestBearerRoleFailureLogsConfiguredNames keeps role-map mistakes diagnosable
// while excluding unrelated role values from the request token.
func TestBearerRoleFailureLogsConfiguredNames(t *testing.T) {
	logs := captureAuthLogs(t)
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_700_000_000, 0)
	server := oidcTestServer(key, func() time.Time { return now })
	defer server.Close()
	provider, err := auth.NewOIDCProvider(auth.OIDCConfig{Issuer: server.URL, Audience: "client", Now: func() time.Time { return now }, RoleNames: config.OIDCRoleNames{Editor: "site-editor", Reviewer: "site-reviewer", Publisher: "site-publisher", Admin: "site-admin"}})
	if err != nil {
		t.Fatal(err)
	}
	middleware, err := auth.NewMiddleware(auth.MiddlewareOptions{IdentityProvider: provider})
	if err != nil {
		t.Fatal(err)
	}
	token := signJWT(t, key, map[string]any{"iss": server.URL, "aud": "client", "sub": "private-subject", "exp": now.Add(time.Hour).Unix(), "roles": []string{"private-role"}})
	r := httptest.NewRequest("GET", "/api/v1/versions", nil)
	r.Header.Set("Authorization", "Bearer "+token)
	r.Header.Set("X-Request-ID", "req-auth-test")
	w := httptest.NewRecorder()
	middleware.Authenticate(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Fatal("rejected token reached handler") })).ServeHTTP(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Fatal(w.Code)
	}
	assertAuthLog(t, logs, "bearer.verify", "roles_invalid", "WARN")
	if !strings.Contains(logs.String(), `"expected_roles":["site-editor","site-reviewer","site-publisher","site-admin"]`) {
		t.Fatalf("configured role names absent from log: %s", logs.String())
	}
	for _, secret := range []string{token, "private-subject", "private-role"} {
		if strings.Contains(logs.String(), secret) {
			t.Fatalf("credential value leaked: %s", secret)
		}
	}
}

// TestSuccessfulCallbackLogsVerifiedSubject records identity only after token
// and nonce verification and never records transient OAuth secrets.
func TestSuccessfulCallbackLogsVerifiedSubject(t *testing.T) {
	logs := captureAuthLogs(t)
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	var issuer, nonce string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			_ = json.NewEncoder(w).Encode(map[string]string{"issuer": issuer, "jwks_uri": issuer + "/keys", "authorization_endpoint": issuer + "/authorize", "token_endpoint": issuer + "/token"})
		case "/keys":
			_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{"kty": "RSA", "kid": "test-key", "n": base64.RawURLEncoding.EncodeToString(key.N.Bytes()), "e": "AQAB"}}})
		case "/token":
			_ = json.NewEncoder(w).Encode(map[string]string{"id_token": signJWT(t, key, map[string]any{"iss": issuer, "aud": "client", "sub": "verified-editor", "roles": []string{"editor"}, "nonce": nonce, "exp": now.Add(time.Hour).Unix()})})
		}
	}))
	defer server.Close()
	issuer = server.URL
	manager, err := auth.NewSessions(auth.SessionOptions{Issuer: issuer, ClientID: "client", RedirectURL: "http://api.test/api/v1/auth/callback", ReturnURL: "http://admin.test/", AllowedOrigins: []string{"http://admin.test"}, Key: make([]byte, 32), Store: memory.NewSessionStore(), Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	login := httptest.NewRecorder()
	loginRequest := httptest.NewRequest("GET", "/api/v1/auth/login", nil)
	loginRequest.Header.Set("X-Request-ID", "req-auth-test")
	manager.ServeHTTP(login, loginRequest)
	location, _ := url.Parse(login.Header().Get("Location"))
	query := location.Query()
	nonce = query.Get("nonce")
	callback := httptest.NewRequest("GET", "/api/v1/auth/callback?code=private-code&state="+query.Get("state"), nil)
	callback.Header.Set("X-Request-ID", "req-auth-test")
	callback.AddCookie(login.Result().Cookies()[0])
	response := httptest.NewRecorder()
	manager.ServeHTTP(response, callback)
	if response.Code != http.StatusFound {
		t.Fatalf("callback: %d %s", response.Code, response.Body.String())
	}
	assertAuthLog(t, logs, "callback", "succeeded", "INFO")
	if !strings.Contains(logs.String(), "verified-editor") || strings.Contains(logs.String(), "private-code") || strings.Contains(logs.String(), nonce) || strings.Contains(logs.String(), query.Get("state")) {
		t.Fatal("callback log lacks verified subject or leaks transient credential")
	}
}

// TestSuccessfulLogoutLogsVerifiedSubject records successful revocation without
// including either the opaque session cookie or CSRF token.
func TestSuccessfulLogoutLogsVerifiedSubject(t *testing.T) {
	logs := captureAuthLogs(t)
	store := memory.NewSessionStore()
	csrf := "private-csrf"
	cookieValue := "private-session-cookie"
	if err := store.CreateSession(context.Background(), auth.Session{
		TokenHash: auth.SessionTokenHash(cookieValue),
		Principal: auth.SessionPrincipal{Subject: "verified-logout-user", Roles: []string{string(domain.RoleAdmin)}},
		CSRFToken: csrf,
		ExpiresAt: time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	manager, err := auth.NewSessions(auth.SessionOptions{Issuer: "https://issuer.test", ClientID: "client", RedirectURL: "https://api.test/api/v1/auth/callback", ReturnURL: "https://admin.test/", AllowedOrigins: []string{"https://admin.test"}, Key: make([]byte, 32), Store: store, Secure: true})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest("POST", "/api/v1/auth/logout", nil)
	request.AddCookie(&http.Cookie{Name: "yujian_session", Value: cookieValue})
	request.Header.Set("Origin", "https://admin.test")
	request.Header.Set("X-CSRF-Token", csrf)
	request.Header.Set("X-Request-ID", "req-auth-test")
	response := httptest.NewRecorder()
	manager.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatal(response.Code)
	}
	assertAuthLog(t, logs, "logout", "succeeded", "INFO")
	if !strings.Contains(logs.String(), "verified-logout-user") {
		t.Fatal("logout log lacks verified subject")
	}
	for _, secret := range []string{cookieValue, csrf} {
		if strings.Contains(logs.String(), secret) {
			t.Fatalf("logout credential leaked: %s", secret)
		}
	}
}

// TestCallbackExchangeFailuresKeepOAuthReasons distinguishes provider faults
// without copying untrusted error descriptions or unknown error codes to logs.
func TestCallbackExchangeFailuresKeepOAuthReasons(t *testing.T) {
	for _, tc := range []struct {
		code   string
		reason string
	}{
		{"server_error", "token_exchange_server_error"},
		{"temporarily_unavailable", "token_exchange_temporarily_unavailable"},
		{"private-provider-code", "token_exchange_rejected"},
	} {
		t.Run(tc.reason, func(t *testing.T) {
			logs := captureAuthLogs(t)
			manager, callbackURL, flowCookie, state := newLoggingTestFlowWithTokenHandler(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusServiceUnavailable)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": tc.code, "error_description": "private-description"})
			})
			callback := httptest.NewRequest("GET", callbackURL+"?code=private-code&state="+url.QueryEscape(state), nil)
			callback.Header.Set("X-Request-ID", "req-auth-test")
			callback.AddCookie(flowCookie)
			response := httptest.NewRecorder()
			manager.ServeHTTP(response, callback)
			if response.Code != http.StatusUnauthorized {
				t.Fatalf("callback status = %d", response.Code)
			}
			assertAuthLog(t, logs, "callback.exchange.response", tc.reason, "ERROR")
			for _, secret := range []string{"private-description", "private-provider-code", "private-code", state, flowCookie.Value} {
				if strings.Contains(logs.String(), secret) {
					t.Fatalf("OAuth exchange detail leaked: %s", secret)
				}
			}
		})
	}
}

func newLoggingTestFlow(t *testing.T) (*auth.Sessions, string, *http.Cookie, string) {
	t.Helper()
	return newLoggingTestFlowWithTokenHandler(t, http.NotFound)
}

// newLoggingTestFlowWithTokenHandler starts a real login flow with a controlled provider response.
func newLoggingTestFlowWithTokenHandler(t *testing.T, tokenHandler http.HandlerFunc) (*auth.Sessions, string, *http.Cookie, string) {
	t.Helper()
	issuer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/openid-configuration" {
			_ = json.NewEncoder(w).Encode(map[string]string{"issuer": "http://" + r.Host, "authorization_endpoint": "http://" + r.Host + "/authorize", "token_endpoint": "http://" + r.Host + "/token"})
			return
		}
		if r.URL.Path == "/token" {
			tokenHandler(w, r)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(issuer.Close)
	manager, err := auth.NewSessions(auth.SessionOptions{Issuer: issuer.URL, ClientID: "client", RedirectURL: "http://api.test/api/v1/auth/callback", ReturnURL: "http://admin.test/", AllowedOrigins: []string{"http://admin.test"}, Key: make([]byte, 32), Store: memory.NewSessionStore()})
	if err != nil {
		t.Fatal(err)
	}
	login := httptest.NewRecorder()
	manager.ServeHTTP(login, httptest.NewRequest("GET", "/api/v1/auth/login", nil))
	location, err := url.Parse(login.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	return manager, "http://api.test/api/v1/auth/callback", login.Result().Cookies()[0], location.Query().Get("state")
}
