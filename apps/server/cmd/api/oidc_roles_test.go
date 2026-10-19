package main

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"testing"
	"time"

	"yujian.me/server/internal/auth"
	"yujian.me/server/internal/config"
)

// TestBuildHandlerSharesOIDCRoleMapping verifies environment-to-router wiring
// for both signed Bearer tokens and an authorization-code browser session.
func TestBuildHandlerSharesOIDCRoleMapping(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	var issuer, nonce string
	sign := func(audience string, roles []string) string {
		claims, err := json.Marshal(map[string]any{
			"iss": issuer, "sub": "user-1", "aud": audience, "nonce": nonce,
			"exp": time.Now().Add(time.Hour).Unix(), "roles": roles,
		})
		if err != nil {
			t.Fatal(err)
		}
		header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","kid":"test-key"}`))
		input := header + "." + base64.RawURLEncoding.EncodeToString(claims)
		digest := sha256.Sum256([]byte(input))
		signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
		if err != nil {
			t.Fatal(err)
		}
		return input + "." + base64.RawURLEncoding.EncodeToString(signature)
	}
	externalRoles := []string{"other-app-admin", "site-editor", "site-review", "site-publish", "site-admin"}
	identity := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			_ = json.NewEncoder(w).Encode(map[string]string{
				"issuer": issuer, "jwks_uri": issuer + "/keys",
				"authorization_endpoint": issuer + "/authorize", "token_endpoint": issuer + "/token",
			})
		case "/keys":
			_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{
				"kty": "RSA", "kid": "test-key", "e": "AQAB", "n": base64.RawURLEncoding.EncodeToString(key.N.Bytes()),
			}}})
		case "/token":
			_ = json.NewEncoder(w).Encode(map[string]string{"id_token": sign("browser-client", externalRoles)})
		default:
			http.NotFound(w, r)
		}
	}))
	defer identity.Close()
	issuer = identity.URL
	env := map[string]string{
		"OIDC_ISSUER": issuer, "OIDC_AUDIENCE": "api-audience", "ADMIN_OIDC_CLIENT_ID": "browser-client",
		"ADMIN_OIDC_REDIRECT_URL": "http://admin.local/api/v1/auth/callback", "ADMIN_RETURN_URL": "http://admin.local/",
		"ADMIN_ALLOWED_ORIGINS": "http://admin.local", "ADMIN_SESSION_KEY": base64.StdEncoding.EncodeToString(make([]byte, 32)),
		"OIDC_ROLE_EDITOR": "site-editor", "OIDC_ROLE_REVIEWER": "site-review",
		"OIDC_ROLE_PUBLISHER": "site-publish", "OIDC_ROLE_ADMIN": "site-admin",
	}
	settings, err := config.Load(func(key string) string { return env[key] })
	if err != nil {
		t.Fatal(err)
	}
	dependencies := developmentDependencies()
	defer dependencies.Close()
	handler, err := buildHandler(settings, dependencies)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		roles  []string
		status int
	}{{externalRoles, 200}, {[]string{"admin"}, 401}, {[]string{"other-app-admin"}, 401}} {
		request := httptest.NewRequest("GET", "/api/v1/versions", nil)
		request.Header.Set("Authorization", "Bearer "+sign("api-audience", tc.roles))
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != tc.status {
			t.Fatalf("Bearer roles %v: status %d; want %d", tc.roles, response.Code, tc.status)
		}
	}
	login := httptest.NewRecorder()
	handler.ServeHTTP(login, httptest.NewRequest("GET", "/api/v1/auth/login", nil))
	location, err := url.Parse(login.Header().Get("Location"))
	if err != nil || login.Code != 302 {
		t.Fatalf("login: status %d, error %v", login.Code, err)
	}
	nonce = location.Query().Get("nonce")
	callback := httptest.NewRequest("GET", "/api/v1/auth/callback?code=valid&state="+url.QueryEscape(location.Query().Get("state")), nil)
	for _, cookie := range login.Result().Cookies() {
		callback.AddCookie(cookie)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, callback)
	if response.Code != 302 {
		t.Fatalf("callback: %d %s", response.Code, response.Body.String())
	}
	request := httptest.NewRequest("GET", "/api/v1/auth/session", nil)
	for _, cookie := range response.Result().Cookies() {
		request.AddCookie(cookie)
	}
	session := httptest.NewRecorder()
	handler.ServeHTTP(session, request)
	var body struct {
		Authenticated bool                  `json:"authenticated"`
		Principal     auth.SessionPrincipal `json:"principal"`
	}
	if err := json.Unmarshal(session.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if session.Code != 200 || !body.Authenticated || !reflect.DeepEqual(body.Principal.Roles, []string{"editor", "reviewer", "publisher", "admin"}) {
		t.Fatalf("noncanonical browser session: %s", session.Body.String())
	}
}
