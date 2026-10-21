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

// TestUserInfoEnrichesOnlyVerifiedIdentity exercises the full signed login flow;
// optional profile failures must neither grant roles nor prevent authentication.
func TestUserInfoEnrichesOnlyVerifiedIdentity(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, body, wantName, wantPicture string
		status                            int
		endpoint, accessToken, tokenType  string
		secure                            bool
	}{
		{name: "standard", body: `{"sub":"user-1","name":"Display name","picture":"https://media.example/portrait.webp","roles":["admin"]}`, wantName: "Display name", wantPicture: "https://media.example/portrait.webp"},
		{name: "casdoor", body: `{"sub":"user-1","name":"login-id","displayName":"小健","avatar":"https://media.example/casdoor.webp"}`, wantName: "小健", wantPicture: "https://media.example/casdoor.webp"},
		{name: "different-subject", body: `{"sub":"user-2","name":"Wrong user","picture":"https://media.example/wrong.webp"}`},
		{name: "missing-subject", body: `{"name":"Wrong user"}`},
		{name: "bad-json", body: `{`},
		{name: "oversized", body: strings.Repeat(" ", 256*1024+1)},
		{name: "provider-failed", status: 503},
		{name: "network-failed", status: -1},
		{name: "redirect", status: 302},
		{name: "no-access-token", accessToken: "absent"},
		{name: "no-endpoint", endpoint: "absent"},
		{name: "unsafe-endpoint", endpoint: "https://user:password@identity.example/profile"},
		{name: "insecure-endpoint", endpoint: "http://identity.example/profile", secure: true},
		{name: "wrong-token-type", tokenType: "Basic"},
		{name: "unsafe-picture", body: `{"sub":"user-1","name":"New name","picture":"javascript:alert(1)"}`, wantName: "New name"},
		{name: "blank-fields", body: `{"sub":"user-1","name":" ","picture":""}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			now := time.Now().UTC().Truncate(time.Second)
			var issuer, nonce string
			profileReads, credentialLeaks := 0, 0
			server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/.well-known/openid-configuration":
					endpoint := issuer + "/userinfo"
					if tc.endpoint == "absent" {
						endpoint = ""
					} else if tc.endpoint != "" {
						endpoint = tc.endpoint
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"issuer": issuer, "jwks_uri": issuer + "/keys", "authorization_endpoint": issuer + "/authorize", "token_endpoint": issuer + "/token", "userinfo_endpoint": endpoint})
				case "/keys":
					_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{"kty": "RSA", "kid": "test-key", "n": base64.RawURLEncoding.EncodeToString(key.N.Bytes()), "e": "AQAB"}}})
				case "/token":
					tokens := map[string]string{"id_token": signJWT(t, key, map[string]any{"iss": issuer, "aud": "admin", "sub": "user-1", "name": "Signed name", "picture": "https://media.example/signed.webp", "roles": []string{"editor"}, "nonce": nonce, "exp": now.Add(time.Hour).Unix()}), "token_type": "Bearer"}
					if tc.accessToken != "absent" {
						tokens["access_token"] = "profile-only-token"
					}
					if tc.tokenType != "" {
						tokens["token_type"] = tc.tokenType
					}
					_ = json.NewEncoder(w).Encode(tokens)
				case "/userinfo":
					profileReads++
					if r.Method != http.MethodGet || r.Header.Get("Authorization") != "Bearer profile-only-token" || r.URL.RawQuery != "" {
						t.Error("UserInfo must use an authenticated GET without query credentials")
					}
					if tc.status == 302 {
						w.Header().Set("Location", issuer+"/leak")
					}
					if tc.status == -1 {
						w.Header().Set("Content-Length", "1000")
						_, _ = w.Write([]byte("{"))
						return
					}
					if tc.status != 0 {
						w.WriteHeader(tc.status)
					}
					_, _ = w.Write([]byte(tc.body))
				case "/leak":
					credentialLeaks++
				default:
					http.NotFound(w, r)
				}
			}))
			if tc.secure {
				server.StartTLS()
			} else {
				server.Start()
			}
			defer server.Close()
			issuer = server.URL
			manager, err := auth.NewSessions(auth.SessionOptions{Issuer: issuer, ClientID: "admin", RedirectURL: "https://api.local/api/v1/auth/callback", ReturnURL: "https://admin.local/", Key: make([]byte, 32), Store: memory.NewSessionStore(), AllowedOrigins: []string{"https://admin.local"}, Secure: tc.secure, HTTPClient: server.Client()})
			if err != nil {
				t.Fatal(err)
			}
			login := httptest.NewRecorder()
			manager.ServeHTTP(login, httptest.NewRequest("GET", "/api/v1/auth/login", nil))
			location, _ := url.Parse(login.Header().Get("Location"))
			nonce = location.Query().Get("nonce")
			callback := httptest.NewRequest("GET", "/api/v1/auth/callback?code=valid&state="+location.Query().Get("state"), nil)
			callback.AddCookie(login.Result().Cookies()[0])
			response := httptest.NewRecorder()
			manager.ServeHTTP(response, callback)
			if response.Code != 302 {
				t.Fatalf("optional profile prevented login: %d", response.Code)
			}
			request := httptest.NewRequest("GET", "/api/v1/auth/session", nil)
			for _, cookie := range response.Result().Cookies() {
				if cookie.Name == "yujian_session" {
					request.AddCookie(cookie)
				}
			}
			session := httptest.NewRecorder()
			manager.ServeHTTP(session, request)
			var result struct {
				Principal auth.SessionPrincipal `json:"principal"`
			}
			if err := json.Unmarshal(session.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			wantName, wantPicture := tc.wantName, tc.wantPicture
			if wantName == "" {
				wantName = "Signed name"
			}
			if wantPicture == "" {
				wantPicture = "https://media.example/signed.webp"
			}
			if result.Principal.Name != wantName || result.Principal.Picture != wantPicture {
				t.Fatalf("profile = %q %q, want %q %q", result.Principal.Name, result.Principal.Picture, wantName, wantPicture)
			}
			if result.Principal.Subject != "user-1" || len(result.Principal.Roles) != 1 || result.Principal.Roles[0] != "editor" {
				t.Fatal("UserInfo changed the verified identity or permissions")
			}
			wantReads := 1
			if tc.accessToken == "absent" || tc.endpoint != "" || tc.tokenType != "" {
				wantReads = 0
			}
			if profileReads != wantReads || credentialLeaks != 0 || strings.Contains(session.Body.String(), "profile-only-token") {
				t.Fatalf("unexpected profile reads=%d (want %d), redirect requests=%d or leaked credential", profileReads, wantReads, credentialLeaks)
			}
		})
	}
}
