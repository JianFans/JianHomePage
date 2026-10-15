package auth_test

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"yujian.me/server/internal/auth"
	"yujian.me/server/internal/store/memory"
)

// TestLoginRejectsUntrustedCallbacks exercises real signed ID tokens against
// browser state tampering, token claim violations and failed code exchanges.
func TestLoginRejectsUntrustedCallbacks(t *testing.T) {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	now := time.Now().UTC()
	var issuer, nonce, scenario string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			json.NewEncoder(w).Encode(map[string]string{"issuer": issuer, "jwks_uri": issuer + "/keys", "authorization_endpoint": issuer + "/authorize", "token_endpoint": issuer + "/token"})
		case "/keys":
			json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{"kty": "RSA", "kid": "test-key", "n": base64.RawURLEncoding.EncodeToString(key.N.Bytes()), "e": "AQAB"}}})
		case "/token":
			if scenario == "token-error" {
				w.WriteHeader(400)
				return
			}
			if scenario == "token-redirect" {
				http.Redirect(w, r, issuer+"/stolen", 302)
				return
			}
			if scenario == "invalid-response" {
				w.Write([]byte("invalid"))
				return
			}
			claims := map[string]any{"iss": issuer, "aud": "client", "sub": "editor", "roles": []string{"editor"}, "exp": now.Add(time.Hour).Unix(), "nonce": nonce}
			switch scenario {
			case "nonce":
				claims["nonce"] = "wrong"
			case "issuer":
				claims["iss"] = "http://evil.local"
			case "audience":
				claims["aud"] = "evil-client"
			case "expired":
				claims["exp"] = now.Add(-time.Hour).Unix()
			case "roles":
				claims["roles"] = []string{"root"}
			case "azp":
				claims["azp"] = "evil-client"
			case "multi-audience":
				claims["aud"] = []string{"client", "other"}
			}
			token := signJWT(t, key, claims)
			if scenario == "signature" {
				parts := strings.Split(token, ".")
				parts[2] = base64.RawURLEncoding.EncodeToString(make([]byte, 256))
				token = strings.Join(parts, ".")
			}
			json.NewEncoder(w).Encode(map[string]string{"id_token": token})
		case "/stolen":
			t.Error("credentials were redirected")
		}
	}))
	defer server.Close()
	issuer = server.URL
	manager, err := auth.NewSessions(auth.SessionOptions{Issuer: issuer, ClientID: "client", RedirectURL: "http://api.local/api/v1/auth/callback", ReturnURL: "http://admin.local/", AllowedOrigins: []string{"http://admin.local"}, Key: make([]byte, 32), Store: memory.NewSessionStore(), Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []string{"state", "duplicate-state", "duplicate-code", "cookie", "cookie-short", "missing-cookie", "expired-flow", "idp-error", "missing-code", "nonce", "issuer", "audience", "expired", "roles", "azp", "multi-audience", "signature", "token-error", "token-redirect", "invalid-response"} {
		t.Run(test, func(t *testing.T) {
			scenario = test
			login := httptest.NewRecorder()
			manager.ServeHTTP(login, httptest.NewRequest("GET", "/api/v1/auth/login", nil))
			location, _ := url.Parse(login.Header().Get("Location"))
			nonce = location.Query().Get("nonce")
			query := url.Values{"state": {location.Query().Get("state")}, "code": {"valid"}}
			cookie := login.Result().Cookies()[0]
			switch test {
			case "duplicate-state":
				query.Add("state", query.Get("state"))
			case "duplicate-code":
				query.Add("code", "second-code")
			case "state":
				query.Set("state", "wrong")
			case "cookie":
				cookie.Value = strings.Repeat("A", 100)
			case "cookie-short":
				cookie.Value = "AA"
			case "idp-error":
				query.Set("error", "access_denied")
			case "missing-code":
				query.Del("code")
			case "expired-flow":
				now = now.Add(11 * time.Minute)
			}
			callback := httptest.NewRequest("GET", "/api/v1/auth/callback?"+query.Encode(), nil)
			if test != "missing-cookie" {
				callback.AddCookie(cookie)
			}
			response := httptest.NewRecorder()
			manager.ServeHTTP(response, callback)
			if response.Code != 401 {
				t.Fatalf("unsafe callback accepted: %d %s", response.Code, response.Body.String())
			}
			for _, cookie := range response.Result().Cookies() {
				if cookie.Name == "yujian_session" && cookie.MaxAge > 0 {
					t.Fatal("rejected callback created session")
				}
			}
		})
	}
}

// TestSessionConfigurationAndUnavailableRoutes checks disabled login and strict
// constructor validation without contacting an external identity provider.
func TestSessionConfigurationAndUnavailableRoutes(t *testing.T) {
	base := auth.SessionOptions{Issuer: "http://issuer.local", ClientID: "client", RedirectURL: "http://api.local/api/v1/auth/callback", ReturnURL: "http://admin.local/", AllowedOrigins: []string{"http://admin.local"}, Key: make([]byte, 32), Store: memory.NewSessionStore()}
	for _, change := range []func(*auth.SessionOptions){func(o *auth.SessionOptions) { o.Key = nil }, func(o *auth.SessionOptions) { o.Store = nil }, func(o *auth.SessionOptions) { o.ReturnURL = "http://evil.local/" }, func(o *auth.SessionOptions) { o.RedirectURL = "ftp://api.local" }, func(o *auth.SessionOptions) { o.RedirectURL = "http://api.local/#fragment" }, func(o *auth.SessionOptions) { o.Secure = true }, func(o *auth.SessionOptions) { o.Issuer = "invalid" }} {
		options := base
		change(&options)
		if _, err := auth.NewSessions(options); err == nil {
			t.Fatal("unsafe configuration accepted")
		}
	}
	manager, _ := auth.NewSessions(auth.SessionOptions{})
	for _, path := range []string{"/api/v1/auth/login", "/api/v1/auth/callback", "/api/v1/auth/unknown"} {
		response := httptest.NewRecorder()
		manager.ServeHTTP(response, httptest.NewRequest("GET", path, nil))
		if response.Code < 400 {
			t.Fatal("disabled login accepted")
		}
	}
	response := httptest.NewRecorder()
	manager.ServeHTTP(response, httptest.NewRequest("POST", "/api/v1/auth/logout", nil))
	if response.Code != 401 {
		t.Fatal("anonymous logout accepted")
	}
}
