package auth_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"yujian.me/server/internal/auth"
	"yujian.me/server/internal/config"
	"yujian.me/server/internal/store/memory"
)

// TestUnconfiguredSessionDoesNotImpersonateDevelopmentUser keeps public session
// discovery anonymous even when a caller supplies development identity headers.
func TestUnconfiguredSessionDoesNotImpersonateDevelopmentUser(t *testing.T) {
	manager, err := auth.NewSessions(auth.SessionOptions{})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/auth/session", nil)
	request.Header.Set("X-Dev-Subject", "admin")
	response := httptest.NewRecorder()
	manager.ServeHTTP(response, request)
	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if response.Code != 200 || body["authenticated"] != false || body["loginAvailable"] != false {
		t.Fatalf("unexpected session: %d %s", response.Code, response.Body.String())
	}
	if response.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("session must not be cached")
	}
}

// TestAuthorizationCodePKCESessionLogoutAndExpiry follows a real signed ID-token
// exchange through login, protected writes, replay rejection and revocation.
func TestAuthorizationCodePKCESessionLogoutAndExpiry(t *testing.T) {
	clientID, clientSecret := "https://admin.example.com", "secret:+/% value"
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	now := time.Now().UTC().Truncate(time.Second)
	var issuer, nonce, challenge string
	var exchanges int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			json.NewEncoder(w).Encode(map[string]any{"issuer": issuer, "jwks_uri": issuer + "/keys", "authorization_endpoint": issuer + "/authorize", "token_endpoint": issuer + "/token"})
		case "/keys":
			json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{"kty": "RSA", "kid": "test-key", "n": base64.RawURLEncoding.EncodeToString(key.N.Bytes()), "e": "AQAB"}}})
		case "/token":
			clientID, secret, basic := r.BasicAuth()
			if !basic || clientID != url.QueryEscape("https://admin.example.com") || secret != url.QueryEscape(clientSecret) {
				t.Error("confidential client authentication missing")
			}
			r.ParseForm()
			digest := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
			if r.Form.Get("code") != "valid-code" || r.Form.Get("grant_type") != "authorization_code" || base64.RawURLEncoding.EncodeToString(digest[:]) != challenge {
				t.Error("invalid code exchange")
			}
			exchanges++
			json.NewEncoder(w).Encode(map[string]string{"id_token": signJWT(t, key, map[string]any{"iss": issuer, "aud": "https://admin.example.com", "sub": "editor-1", "roles": []string{"other-app-admin", "site-editor", "site-editor"}, "nonce": nonce, "exp": now.Add(time.Hour).Unix()})})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	issuer = server.URL
	store := memory.NewSessionStore()
	manager, err := auth.NewSessions(auth.SessionOptions{Issuer: issuer, ClientID: clientID, ClientSecret: clientSecret, RedirectURL: "http://api.local/api/v1/auth/callback", ReturnURL: "http://admin.local/", Key: make([]byte, 32), Store: store, AllowedOrigins: []string{"http://admin.local"}, Now: func() time.Time { return now }, RoleNames: config.OIDCRoleNames{Editor: "site-editor"}})
	if err != nil {
		t.Fatal(err)
	}
	login := httptest.NewRecorder()
	manager.ServeHTTP(login, httptest.NewRequest("GET", "/api/v1/auth/login", nil))
	location, _ := url.Parse(login.Header().Get("Location"))
	query := location.Query()
	nonce = query.Get("nonce")
	challenge = query.Get("code_challenge")
	if login.Code != 302 || query.Get("code_challenge_method") != "S256" || nonce == "" || query.Get("state") == "" || query.Get("response_type") != "code" {
		t.Fatalf("invalid login %d %s", login.Code, location)
	}
	flow := login.Result().Cookies()[0]
	callback := httptest.NewRequest("GET", "/api/v1/auth/callback?code=valid-code&state="+query.Get("state"), nil)
	callback.AddCookie(flow)
	response := httptest.NewRecorder()
	manager.ServeHTTP(response, callback)
	if response.Code != 302 || response.Header().Get("Location") != "http://admin.local/" {
		t.Fatalf("callback %d %s", response.Code, response.Body.String())
	}
	var cookie *http.Cookie
	for _, c := range response.Result().Cookies() {
		if c.Name == "yujian_session" {
			cookie = c
		}
	}
	if cookie == nil || !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode || strings.Contains(cookie.Value, "editor") {
		t.Fatalf("unsafe cookie: %#v", cookie)
	}
	sessionRequest := httptest.NewRequest("GET", "/api/v1/auth/session", nil)
	sessionRequest.AddCookie(cookie)
	session := httptest.NewRecorder()
	manager.ServeHTTP(session, sessionRequest)
	var body struct {
		Authenticated bool                  `json:"authenticated"`
		CSRF          string                `json:"csrfToken"`
		Principal     auth.SessionPrincipal `json:"principal"`
	}
	json.Unmarshal(session.Body.Bytes(), &body)
	if !body.Authenticated || body.CSRF == "" || len(body.Principal.Roles) != 1 || body.Principal.Roles[0] != "editor" {
		t.Fatalf("session %s", session.Body.String())
	}
	middleware, _ := auth.NewMiddleware(auth.MiddlewareOptions{Sessions: manager})
	protected := middleware.Authenticate(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	write := httptest.NewRequest("POST", "/api/v1/versions", nil)
	write.AddCookie(cookie)
	rejected := httptest.NewRecorder()
	protected.ServeHTTP(rejected, write)
	if rejected.Code != 403 {
		t.Fatalf("missing csrf accepted: %d", rejected.Code)
	}
	write.Header.Set("Origin", "http://admin.local")
	write.Header.Set("X-CSRF-Token", body.CSRF)
	accepted := httptest.NewRecorder()
	protected.ServeHTTP(accepted, write)
	if accepted.Code != 204 {
		t.Fatalf("valid csrf rejected: %d", accepted.Code)
	}
	for _, headers := range []map[string]string{{"Origin": "http://evil.local", "X-CSRF-Token": body.CSRF}, {"Origin": "http://admin.local", "X-CSRF-Token": "wrong"}, {"Origin": "http://admin.local"}} {
		request := httptest.NewRequest("POST", "/api/v1/auth/logout", nil)
		request.AddCookie(cookie)
		for key, value := range headers {
			request.Header.Set(key, value)
		}
		response := httptest.NewRecorder()
		manager.ServeHTTP(response, request)
		if response.Code != 403 {
			t.Fatalf("invalid logout verification accepted %d", response.Code)
		}
	}
	replay := httptest.NewRecorder()
	manager.ServeHTTP(replay, callback)
	if replay.Code != 401 || exchanges != 1 {
		t.Fatalf("flow replay accepted %d %d", replay.Code, exchanges)
	}
	logout := httptest.NewRequest("POST", "/api/v1/auth/logout", nil)
	logout.AddCookie(cookie)
	logout.Header = write.Header.Clone()
	loggedOut := httptest.NewRecorder()
	manager.ServeHTTP(loggedOut, logout)
	if loggedOut.Code != 204 {
		t.Fatalf("logout %d", loggedOut.Code)
	}
	old := httptest.NewRecorder()
	manager.ServeHTTP(old, sessionRequest)
	if strings.Contains(old.Body.String(), `"authenticated":true`) {
		t.Fatal("revoked cookie accepted")
	}
	// A persisted expired session must never authenticate, even before cleanup.
	store.CreateSession(context.Background(), auth.Session{TokenHash: auth.SessionTokenHash("expired"), Principal: auth.SessionPrincipal{Subject: "old", Roles: []string{"editor"}}, CSRFToken: "csrf", ExpiresAt: now.Add(-time.Second)})
	expired := httptest.NewRequest("GET", "/api/v1/auth/session", nil)
	expired.AddCookie(&http.Cookie{Name: "yujian_session", Value: "expired"})
	old = httptest.NewRecorder()
	manager.ServeHTTP(old, expired)
	if strings.Contains(old.Body.String(), `"authenticated":true`) {
		t.Fatal("expired cookie accepted")
	}
}
