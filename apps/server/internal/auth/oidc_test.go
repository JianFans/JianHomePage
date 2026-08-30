package auth_test

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"yujian.me/server/internal/auth"
	"yujian.me/server/internal/domain"
)

func TestOIDCProviderValidatesIssuerAudienceExpiryAndRoles(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	server := oidcTestServer(key, func() time.Time { return time.Unix(1_700_000_000, 0) })
	defer server.Close()

	provider, err := auth.NewOIDCProvider(auth.OIDCConfig{
		Issuer:     server.URL,
		Audience:   "yujian-admin",
		HTTPClient: server.Client(),
		Now:        func() time.Time { return time.Unix(1_700_000_000, 0) },
	})
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}
	token := signJWT(t, key, map[string]any{
		"iss":   server.URL,
		"sub":   "editor-1",
		"email": "editor@example.com",
		"name":  "Editor",
		"aud":   "yujian-admin",
		"exp":   1_700_000_300,
		"roles": []string{"editor"},
	})

	principal, err := provider.Authenticate(context.Background(), token)
	if err != nil {
		t.Fatalf("authenticate: %v", err)
	}
	if principal.Subject != "editor-1" || principal.Email != "editor@example.com" || len(principal.Roles) != 1 || principal.Roles[0] != domain.RoleEditor {
		t.Fatalf("unexpected principal %#v", principal)
	}
}

func TestOIDCProviderRejectsWrongAudienceExpiredTokenAndUnknownRole(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	server := oidcTestServer(key, func() time.Time { return time.Unix(1_700_000_000, 0) })
	defer server.Close()
	provider, err := auth.NewOIDCProvider(auth.OIDCConfig{Issuer: server.URL, Audience: "yujian-admin", HTTPClient: server.Client(), Now: func() time.Time { return time.Unix(1_700_000_000, 0) }})
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}

	cases := []map[string]any{
		{"iss": server.URL, "sub": "editor-1", "aud": "other", "exp": 1_700_000_300, "roles": []string{"editor"}},
		{"iss": server.URL, "sub": "editor-1", "aud": "yujian-admin", "exp": 1_699_999_900, "roles": []string{"editor"}},
		{"iss": server.URL, "sub": "editor-1", "aud": "yujian-admin", "exp": 1_700_000_300, "roles": []string{"root"}},
	}
	for index, claims := range cases {
		t.Run(fmt.Sprintf("case-%d", index), func(t *testing.T) {
			_, err := provider.Authenticate(context.Background(), signJWT(t, key, claims))
			if err == nil {
				t.Fatal("expected token rejection")
			}
		})
	}
}

func TestOIDCProviderDoesNotLeakTokenInErrors(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	server := oidcTestServer(key, func() time.Time { return time.Unix(1_700_000_000, 0) })
	defer server.Close()
	provider, err := auth.NewOIDCProvider(auth.OIDCConfig{Issuer: server.URL, Audience: "yujian-admin", HTTPClient: server.Client(), Now: func() time.Time { return time.Unix(1_700_000_000, 0) }})
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}
	token := "not-a-jwt-secret"
	_, err = provider.Authenticate(context.Background(), token)
	if err == nil || strings.Contains(err.Error(), token) {
		t.Fatalf("unexpected token leak: %v", err)
	}
}

func TestOIDCProviderCanRequireHTTPS(t *testing.T) {
	if _, err := auth.NewOIDCProvider(auth.OIDCConfig{Issuer: "http://issuer.example", Audience: "aud", RequireHTTPS: true}); err == nil {
		t.Fatal("expected HTTPS requirement error")
	}
}

func TestOIDCProviderRejectsIssuerWithoutHostnameOrValidPort(t *testing.T) {
	for _, issuer := range []string{"https://:443", "https://issuer.example:65536"} {
		t.Run(issuer, func(t *testing.T) {
			if _, err := auth.NewOIDCProvider(auth.OIDCConfig{Issuer: issuer, Audience: "aud", RequireHTTPS: true}); err == nil {
				t.Fatalf("expected invalid issuer rejection for %q", issuer)
			}
		})
	}
}

func TestOIDCProviderRejectsIssuerWithQueryOrFragment(t *testing.T) {
	for _, issuer := range []string{
		"https://issuer.example/tenant?client=yujian",
		"https://issuer.example/tenant#configuration",
	} {
		t.Run(issuer, func(t *testing.T) {
			if _, err := auth.NewOIDCProvider(auth.OIDCConfig{Issuer: issuer, Audience: "aud", RequireHTTPS: true}); err == nil {
				t.Fatalf("expected invalid issuer rejection for %q", issuer)
			}
		})
	}
}

func TestOIDCProviderThrottlesRefreshesAcrossUnknownKeyIDs(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	var discoveryCalls atomic.Int32
	var jwksCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/.well-known/openid-configuration":
			discoveryCalls.Add(1)
			_ = json.NewEncoder(writer).Encode(map[string]string{
				"issuer": requestHost(request), "jwks_uri": requestHost(request) + "/jwks",
			})
		case "/jwks":
			jwksCalls.Add(1)
			_ = json.NewEncoder(writer).Encode(map[string]any{"keys": []map[string]string{{
				"kty": "RSA", "kid": "test-key",
				"n": base64.RawURLEncoding.EncodeToString(key.PublicKey.N.Bytes()),
				"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.PublicKey.E)).Bytes()),
			}}})
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	now := time.Unix(1_700_000_000, 0)
	provider, err := auth.NewOIDCProvider(auth.OIDCConfig{
		Issuer: server.URL, Audience: "yujian-admin", HTTPClient: server.Client(), Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}
	claims := map[string]any{
		"iss": server.URL, "sub": "editor-1", "aud": "yujian-admin",
		"exp": 1_700_000_300, "roles": []string{"editor"},
	}
	if _, err := provider.Authenticate(context.Background(), signJWT(t, key, claims)); err != nil {
		t.Fatalf("prime signing key cache: %v", err)
	}
	for _, keyID := range []string{"unknown-one", "unknown-two"} {
		if _, err := provider.Authenticate(context.Background(), signJWTWithKeyID(t, key, keyID, claims)); err == nil {
			t.Fatalf("expected unknown key rejection for %q", keyID)
		}
	}
	if got := discoveryCalls.Load(); got != 2 {
		t.Fatalf("expected one initial fetch and one throttled refresh, got %d discovery requests", got)
	}
	if got := jwksCalls.Load(); got != 2 {
		t.Fatalf("expected one initial fetch and one throttled refresh, got %d JWKS requests", got)
	}
}

func TestOIDCProviderThrottlesUnknownKeyRefreshAfterProviderFailure(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	var discoveryCalls atomic.Int32
	var providerUnavailable atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/.well-known/openid-configuration":
			discoveryCalls.Add(1)
			if providerUnavailable.Load() {
				writer.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			_ = json.NewEncoder(writer).Encode(map[string]string{
				"issuer": requestHost(request), "jwks_uri": requestHost(request) + "/jwks",
			})
		case "/jwks":
			_ = json.NewEncoder(writer).Encode(map[string]any{"keys": []map[string]string{{
				"kty": "RSA", "kid": "test-key",
				"n": base64.RawURLEncoding.EncodeToString(key.PublicKey.N.Bytes()),
				"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.PublicKey.E)).Bytes()),
			}}})
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	now := time.Unix(1_700_000_000, 0)
	provider, err := auth.NewOIDCProvider(auth.OIDCConfig{
		Issuer: server.URL, Audience: "yujian-admin", HTTPClient: server.Client(), Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}
	claims := map[string]any{
		"iss": server.URL, "sub": "editor-1", "aud": "yujian-admin",
		"exp": 1_700_000_300, "roles": []string{"editor"},
	}
	if _, err := provider.Authenticate(context.Background(), signJWT(t, key, claims)); err != nil {
		t.Fatalf("prime signing key cache: %v", err)
	}
	providerUnavailable.Store(true)
	for _, keyID := range []string{"unknown-one", "unknown-two"} {
		if _, err := provider.Authenticate(context.Background(), signJWTWithKeyID(t, key, keyID, claims)); err == nil {
			t.Fatalf("expected unknown key rejection for %q", keyID)
		}
	}
	if got := discoveryCalls.Load(); got != 2 {
		t.Fatalf("expected failed unknown-key refresh to enter cooldown, got %d discovery requests", got)
	}
}

func TestOIDCProviderUsesCachedKnownKeyDuringFailedRefreshCooldown(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	var discoveryCalls atomic.Int32
	var providerUnavailable atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/.well-known/openid-configuration":
			discoveryCalls.Add(1)
			if providerUnavailable.Load() {
				writer.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			_ = json.NewEncoder(writer).Encode(map[string]string{
				"issuer": requestHost(request), "jwks_uri": requestHost(request) + "/jwks",
			})
		case "/jwks":
			_ = json.NewEncoder(writer).Encode(map[string]any{"keys": []map[string]string{{
				"kty": "RSA", "kid": "test-key",
				"n": base64.RawURLEncoding.EncodeToString(key.PublicKey.N.Bytes()),
				"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.PublicKey.E)).Bytes()),
			}}})
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	now := time.Unix(1_700_000_000, 0)
	provider, err := auth.NewOIDCProvider(auth.OIDCConfig{
		Issuer: server.URL, Audience: "yujian-admin", HTTPClient: server.Client(), Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}
	token := signJWT(t, key, map[string]any{
		"iss": server.URL, "sub": "editor-1", "aud": "yujian-admin",
		"exp": 1_700_003_600, "roles": []string{"editor"},
	})
	if _, err := provider.Authenticate(context.Background(), token); err != nil {
		t.Fatalf("prime signing key cache: %v", err)
	}

	providerUnavailable.Store(true)
	now = now.Add(5*time.Minute + time.Second)
	if _, err := provider.Authenticate(context.Background(), token); err != nil {
		t.Fatalf("expected cached key after failed refresh: %v", err)
	}
	if _, err := provider.Authenticate(context.Background(), token); err != nil {
		t.Fatalf("expected cached key during refresh cooldown: %v", err)
	}
	if got := discoveryCalls.Load(); got != 2 {
		t.Fatalf("expected one failed refresh during cooldown, got %d discovery requests", got)
	}

	now = now.Add(31 * time.Second)
	if _, err := provider.Authenticate(context.Background(), token); err != nil {
		t.Fatalf("expected cached key after retry failure: %v", err)
	}
	if got := discoveryCalls.Load(); got != 3 {
		t.Fatalf("expected refresh retry after cooldown, got %d discovery requests", got)
	}
}

func TestOIDCProviderRejectsHTTPSDowngradeRedirectBeforeSendingRequest(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	var insecureCalls atomic.Int32
	insecure := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		insecureCalls.Add(1)
		writer.WriteHeader(http.StatusBadGateway)
	}))
	defer insecure.Close()
	secure := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		http.Redirect(writer, request, insecure.URL, http.StatusTemporaryRedirect)
	}))
	defer secure.Close()
	provider, err := auth.NewOIDCProvider(auth.OIDCConfig{
		Issuer: secure.URL, Audience: "yujian-admin", HTTPClient: secure.Client(),
		Now: func() time.Time { return time.Unix(1_700_000_000, 0) }, RequireHTTPS: true,
	})
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}
	token := signJWT(t, key, map[string]any{
		"iss": secure.URL, "sub": "editor-1", "aud": "yujian-admin",
		"exp": 1_700_000_300, "roles": []string{"editor"},
	})

	if _, err := provider.Authenticate(context.Background(), token); err == nil {
		t.Fatal("expected HTTPS downgrade rejection")
	}
	if insecureCalls.Load() != 0 {
		t.Fatalf("insecure redirect target received %d requests", insecureCalls.Load())
	}
}

func TestOIDCProviderRequiresHTTPSForDiscoveredJWKS(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	issuer := "https://issuer.example"
	public := map[string]string{
		"kty": "RSA",
		"kid": "test-key",
		"n":   base64.RawURLEncoding.EncodeToString(key.PublicKey.N.Bytes()),
		"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.PublicKey.E)).Bytes()),
	}
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		var body any
		switch request.URL.String() {
		case issuer + "/.well-known/openid-configuration":
			body = map[string]string{"issuer": issuer, "jwks_uri": "http://keys.example/jwks"}
		case "http://keys.example/jwks":
			body = map[string]any{"keys": []map[string]string{public}}
		default:
			return &http.Response{StatusCode: http.StatusNotFound, Body: io.NopCloser(strings.NewReader("not found")), Header: make(http.Header)}, nil
		}
		encoded, marshalErr := json.Marshal(body)
		if marshalErr != nil {
			return nil, marshalErr
		}
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(string(encoded))), Header: make(http.Header)}, nil
	})}
	provider, err := auth.NewOIDCProvider(auth.OIDCConfig{
		Issuer: issuer, Audience: "yujian-admin", HTTPClient: client,
		Now: func() time.Time { return time.Unix(1_700_000_000, 0) }, RequireHTTPS: true,
	})
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}
	token := signJWT(t, key, map[string]any{
		"iss": issuer, "sub": "editor-1", "aud": "yujian-admin",
		"exp": 1_700_000_300, "roles": []string{"editor"},
	})

	if _, err := provider.Authenticate(context.Background(), token); err == nil {
		t.Fatal("expected insecure JWKS URL rejection")
	}
}

func TestOIDCProviderRejectsKeysDeclaredForAnotherUseOrAlgorithm(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/.well-known/openid-configuration":
			_ = json.NewEncoder(writer).Encode(map[string]string{
				"issuer": requestHost(request), "jwks_uri": requestHost(request) + "/jwks",
			})
		case "/jwks":
			_ = json.NewEncoder(writer).Encode(map[string]any{"keys": []map[string]string{{
				"kty": "RSA", "kid": "test-key", "use": "enc", "alg": "RS512",
				"n": base64.RawURLEncoding.EncodeToString(key.PublicKey.N.Bytes()),
				"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.PublicKey.E)).Bytes()),
			}}})
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	provider, err := auth.NewOIDCProvider(auth.OIDCConfig{
		Issuer: server.URL, Audience: "yujian-admin", HTTPClient: server.Client(),
		Now: func() time.Time { return time.Unix(1_700_000_000, 0) },
	})
	if err != nil {
		t.Fatalf("new provider: %v", err)
	}
	token := signJWT(t, key, map[string]any{
		"iss": server.URL, "sub": "editor-1", "aud": "yujian-admin",
		"exp": 1_700_000_300, "roles": []string{"editor"},
	})

	if _, err := provider.Authenticate(context.Background(), token); err == nil {
		t.Fatal("expected key use and algorithm rejection")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (roundTrip roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return roundTrip(request)
}

func oidcTestServer(key *rsa.PrivateKey, now func() time.Time) *httptest.Server {
	public := map[string]string{
		"kty": "RSA",
		"kid": "test-key",
		"n":   base64.RawURLEncoding.EncodeToString(key.PublicKey.N.Bytes()),
		"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.PublicKey.E)).Bytes()),
	}
	return httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		switch request.URL.Path {
		case "/.well-known/openid-configuration":
			_ = json.NewEncoder(writer).Encode(map[string]string{"issuer": requestHost(request), "jwks_uri": requestHost(request) + "/jwks"})
		case "/jwks":
			_ = json.NewEncoder(writer).Encode(map[string]any{"keys": []map[string]string{public}})
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
		_ = now
	}))
}

func requestHost(request *http.Request) string {
	return "http://" + request.Host
}

func signJWT(t *testing.T, key *rsa.PrivateKey, claims map[string]any) string {
	t.Helper()
	return signJWTWithKeyID(t, key, "test-key", claims)
}

func signJWTWithKeyID(t *testing.T, key *rsa.PrivateKey, keyID string, claims map[string]any) string {
	t.Helper()
	headerJSON, err := json.Marshal(map[string]string{"alg": "RS256", "kid": keyID, "typ": "JWT"})
	if err != nil {
		t.Fatalf("marshal header: %v", err)
	}
	header := base64.RawURLEncoding.EncodeToString(headerJSON)
	payload, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("marshal claims: %v", err)
	}
	payloadPart := base64.RawURLEncoding.EncodeToString(payload)
	message := header + "." + payloadPart
	digest := sha256.Sum256([]byte(message))
	signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatalf("sign JWT: %v", err)
	}
	return message + "." + base64.RawURLEncoding.EncodeToString(signature)
}
