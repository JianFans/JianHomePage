package auth_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"yujian.me/server/internal/auth"
)

// TestOIDCProfileClaims preserves verified display metadata without accepting
// executable, credential-bearing or malformed avatar URLs.
func TestOIDCProfileClaims(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_700_000_000, 0)
	server := oidcTestServer(key, func() time.Time { return now })
	defer server.Close()
	provider, err := auth.NewOIDCProvider(auth.OIDCConfig{Issuer: server.URL, Audience: "admin", Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name                  string
		profile               map[string]any
		wantName, wantPicture string
	}{
		{"standard", map[string]any{"name": "Editor", "picture": "https://media.example/avatar.jpg"}, "Editor", "https://media.example/avatar.jpg"},
		{"casdoor", map[string]any{"name": "editor-id", "displayName": "编辑者", "avatar": "https://media.example/casdoor.jpg"}, "编辑者", "https://media.example/casdoor.jpg"},
		{"standard-picture-priority", map[string]any{"picture": "https://media.example/standard.jpg", "avatar": "https://media.example/alternate.jpg"}, "", "https://media.example/standard.jpg"},
		{"empty-display-name", map[string]any{"displayName": "  ", "name": "Editor"}, "Editor", ""},
		{"javascript", map[string]any{"picture": "javascript:alert(1)", "avatar": "https://media.example/alternate.jpg"}, "", ""},
		{"invalid-picture-type", map[string]any{"picture": true, "avatar": "https://media.example/alternate.jpg"}, "", ""},
		{"credentials", map[string]any{"picture": "https://user:secret@media.example/avatar.jpg"}, "", ""},
		{"relative", map[string]any{"picture": "/avatar.jpg"}, "", ""},
		{"bad-port", map[string]any{"picture": "https://media.example:65536/avatar.jpg"}, "", ""},
		{"development-http", map[string]any{"picture": "http://media.example/avatar.jpg"}, "", "http://media.example/avatar.jpg"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			claims := map[string]any{"iss": server.URL, "aud": "admin", "sub": "editor", "roles": []string{"editor"}, "exp": now.Add(time.Hour).Unix()}
			for name, value := range tc.profile {
				claims[name] = value
			}
			principal, err := provider.Authenticate(context.Background(), signJWT(t, key, claims))
			if err != nil {
				t.Fatal(err)
			}
			if principal.Name != tc.wantName || principal.Picture != tc.wantPicture {
				t.Fatalf("profile name=%q picture=%q; want name=%q picture=%q", principal.Name, principal.Picture, tc.wantName, tc.wantPicture)
			}
		})
	}
}

// TestOIDCProductionProfileRequiresHTTPS rejects insecure pictures while
// accepting valid HTTPS metadata from a verified production-style issuer.
func TestOIDCProductionProfileRequiresHTTPS(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_700_000_000, 0)
	var issuer string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			_ = json.NewEncoder(w).Encode(map[string]string{"issuer": issuer, "jwks_uri": issuer + "/keys"})
		case "/keys":
			_ = json.NewEncoder(w).Encode(map[string]any{"keys": []map[string]string{{"kty": "RSA", "kid": "test-key", "n": base64.RawURLEncoding.EncodeToString(key.N.Bytes()), "e": "AQAB"}}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	issuer = server.URL
	provider, err := auth.NewOIDCProvider(auth.OIDCConfig{Issuer: issuer, Audience: "admin", RequireHTTPS: true, HTTPClient: server.Client(), Now: func() time.Time { return now }})
	if err != nil {
		t.Fatal(err)
	}
	for _, picture := range []string{"http://media.example/avatar.jpg", "https://media.example/avatar.jpg"} {
		claims := map[string]any{"iss": issuer, "aud": "admin", "sub": "editor", "roles": []string{"editor"}, "exp": now.Add(time.Hour).Unix(), "picture": picture}
		principal, err := provider.Authenticate(t.Context(), signJWT(t, key, claims))
		if err != nil {
			t.Fatal(err)
		}
		want := picture
		if picture == "http://media.example/avatar.jpg" {
			want = ""
		}
		if principal.Picture != want {
			t.Fatalf("production picture=%q; want %q", principal.Picture, want)
		}
	}
}
