package auth

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"yujian.me/server/internal/domain"
)

const (
	defaultOIDCTokenBytes    = 16 * 1024
	defaultOIDCResponseBytes = 256 * 1024
	defaultOIDCKeyTTL        = 5 * time.Minute
)

type OIDCConfig struct {
	Issuer       string
	Audience     string
	HTTPClient   *http.Client
	Now          func() time.Time
	ClockSkew    time.Duration
	MaxTokenSize int
	RequireHTTPS bool
}

type OIDCProvider struct {
	issuer       string
	audience     string
	httpClient   *http.Client
	now          func() time.Time
	clockSkew    time.Duration
	maxTokenSize int
	requireHTTPS bool

	mu          sync.Mutex
	jwksURL     string
	keys        map[string]*rsa.PublicKey
	keysFetched time.Time
}

type oidcDiscovery struct {
	Issuer  string `json:"issuer"`
	JWKSURI string `json:"jwks_uri"`
}

type jwksDocument struct {
	Keys []jwk `json:"keys"`
}

type jwk struct {
	KTY string `json:"kty"`
	Use string `json:"use"`
	Alg string `json:"alg"`
	Kid string `json:"kid"`
	N   string `json:"n"`
	E   string `json:"e"`
}

func NewOIDCProvider(config OIDCConfig) (*OIDCProvider, error) {
	issuer := strings.TrimRight(strings.TrimSpace(config.Issuer), "/")
	parsed, err := url.Parse(issuer)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") {
		return nil, errors.New("OIDC issuer must be an absolute HTTP(S) URL")
	}
	if config.RequireHTTPS && parsed.Scheme != "https" {
		return nil, errors.New("OIDC issuer must use HTTPS")
	}
	if strings.TrimSpace(config.Audience) == "" {
		return nil, errors.New("OIDC audience is required")
	}
	httpClient := config.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}
	now := config.Now
	if now == nil {
		now = time.Now
	}
	clockSkew := config.ClockSkew
	if clockSkew < 0 {
		return nil, errors.New("OIDC clock skew cannot be negative")
	}
	if clockSkew == 0 {
		clockSkew = 30 * time.Second
	}
	maxTokenSize := config.MaxTokenSize
	if maxTokenSize <= 0 {
		maxTokenSize = defaultOIDCTokenBytes
	}
	return &OIDCProvider{
		issuer:       issuer,
		audience:     strings.TrimSpace(config.Audience),
		httpClient:   httpClient,
		now:          now,
		clockSkew:    clockSkew,
		maxTokenSize: maxTokenSize,
		requireHTTPS: config.RequireHTTPS,
		keys:         make(map[string]*rsa.PublicKey),
	}, nil
}

func (provider *OIDCProvider) Authenticate(ctx context.Context, token string) (domain.Principal, error) {
	token = strings.TrimSpace(token)
	if token == "" || len(token) > provider.maxTokenSize {
		return domain.Principal{}, errors.New("invalid bearer token")
	}
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return domain.Principal{}, errors.New("invalid bearer token")
	}
	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return domain.Principal{}, errors.New("invalid bearer token")
	}
	var header struct {
		Algorithm string `json:"alg"`
		KeyID     string `json:"kid"`
	}
	if err := json.Unmarshal(headerBytes, &header); err != nil || header.Algorithm != "RS256" || header.KeyID == "" {
		return domain.Principal{}, errors.New("unsupported bearer token")
	}
	publicKey, err := provider.key(ctx, header.KeyID)
	if err != nil {
		return domain.Principal{}, err
	}
	signed := parts[0] + "." + parts[1]
	digest := sha256Digest([]byte(signed))
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || rsa.VerifyPKCS1v15(publicKey, crypto.SHA256, digest, signature) != nil {
		return domain.Principal{}, errors.New("invalid bearer token")
	}
	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return domain.Principal{}, errors.New("invalid bearer token")
	}
	var claims map[string]any
	decoder := json.NewDecoder(strings.NewReader(string(payloadBytes)))
	decoder.UseNumber()
	if err := decoder.Decode(&claims); err != nil {
		return domain.Principal{}, errors.New("invalid bearer token")
	}
	if err := provider.validateClaims(claims); err != nil {
		return domain.Principal{}, err
	}
	roles, err := rolesFromClaims(claims["roles"])
	if err != nil {
		return domain.Principal{}, err
	}
	subject, _ := claims["sub"].(string)
	principal := domain.Principal{Subject: subject, Roles: roles}
	if value, ok := claims["email"].(string); ok {
		principal.Email = value
	}
	if value, ok := claims["name"].(string); ok {
		principal.Name = value
	}
	return principal, nil
}

func (provider *OIDCProvider) validateClaims(claims map[string]any) error {
	issuer, _ := claims["iss"].(string)
	if issuer != provider.issuer {
		return errors.New("invalid token issuer")
	}
	subject, _ := claims["sub"].(string)
	if strings.TrimSpace(subject) == "" {
		return errors.New("token subject is required")
	}
	if !audienceContains(claims["aud"], provider.audience) {
		return errors.New("invalid token audience")
	}
	exp, ok := numericClaim(claims["exp"])
	if !ok || provider.now().After(time.Unix(int64(exp), 0).Add(provider.clockSkew)) {
		return errors.New("token is expired")
	}
	if nbf, ok := numericClaim(claims["nbf"]); ok && provider.now().Before(time.Unix(int64(nbf), 0).Add(-provider.clockSkew)) {
		return errors.New("token is not active")
	}
	return nil
}

func (provider *OIDCProvider) key(ctx context.Context, keyID string) (*rsa.PublicKey, error) {
	provider.mu.Lock()
	defer provider.mu.Unlock()
	if key, ok := provider.keys[keyID]; ok && provider.now().Before(provider.keysFetched.Add(defaultOIDCKeyTTL)) {
		return key, nil
	}
	if err := provider.refreshKeysLocked(ctx); err != nil {
		return nil, err
	}
	key, ok := provider.keys[keyID]
	if !ok {
		return nil, errors.New("signing key not found")
	}
	return key, nil
}

func (provider *OIDCProvider) refreshKeysLocked(ctx context.Context) error {
	discoveryURL := provider.issuer + "/.well-known/openid-configuration"
	body, err := provider.fetch(ctx, discoveryURL)
	if err != nil {
		return err
	}
	var discovery oidcDiscovery
	if err := json.Unmarshal(body, &discovery); err != nil || discovery.JWKSURI == "" || discovery.Issuer != provider.issuer {
		return errors.New("invalid OIDC discovery document")
	}
	parsed, err := url.Parse(discovery.JWKSURI)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return errors.New("invalid OIDC JWKS URL")
	}
	if provider.requireHTTPS && parsed.Scheme != "https" {
		return errors.New("OIDC JWKS URL must use HTTPS")
	}
	body, err = provider.fetch(ctx, discovery.JWKSURI)
	if err != nil {
		return err
	}
	var document jwksDocument
	if err := json.Unmarshal(body, &document); err != nil {
		return errors.New("invalid OIDC JWKS document")
	}
	keys := make(map[string]*rsa.PublicKey, len(document.Keys))
	for _, value := range document.Keys {
		if value.KTY != "RSA" || value.Kid == "" || value.N == "" || value.E == "" {
			continue
		}
		modulus, err := base64.RawURLEncoding.DecodeString(value.N)
		if err != nil || len(modulus) < 256 {
			continue
		}
		exponentBytes, err := base64.RawURLEncoding.DecodeString(value.E)
		if err != nil || len(exponentBytes) == 0 || len(exponentBytes) > 4 {
			continue
		}
		exponent := 0
		for _, part := range exponentBytes {
			exponent = exponent<<8 | int(part)
		}
		if exponent < 3 {
			continue
		}
		keys[value.Kid] = &rsa.PublicKey{N: new(big.Int).SetBytes(modulus), E: exponent}
	}
	if len(keys) == 0 {
		return errors.New("OIDC JWKS contains no usable RSA keys")
	}
	provider.jwksURL = discovery.JWKSURI
	provider.keys = keys
	provider.keysFetched = provider.now()
	return nil
}

func (provider *OIDCProvider) fetch(ctx context.Context, endpoint string) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, errors.New("OIDC request could not be created")
	}
	request.Header.Set("Accept", "application/json")
	response, err := provider.httpClient.Do(request)
	if err != nil {
		return nil, errors.New("OIDC provider unavailable")
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, defaultOIDCResponseBytes+1))
	if err != nil || len(body) > defaultOIDCResponseBytes {
		return nil, errors.New("OIDC provider response invalid")
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("OIDC provider returned status %d", response.StatusCode)
	}
	return body, nil
}

func rolesFromClaims(value any) ([]domain.Role, error) {
	var values []string
	switch typed := value.(type) {
	case []any:
		for _, item := range typed {
			role, ok := item.(string)
			if !ok {
				return nil, errors.New("invalid token roles")
			}
			values = append(values, role)
		}
	case []string:
		values = append(values, typed...)
	case string:
		values = strings.Fields(typed)
	default:
		return nil, errors.New("token roles are required")
	}
	roles, err := ParseRoles(values)
	if err != nil || len(roles) == 0 {
		return nil, errors.New("token roles are invalid")
	}
	return roles, nil
}

func audienceContains(value any, expected string) bool {
	switch typed := value.(type) {
	case string:
		return typed == expected
	case []any:
		for _, item := range typed {
			if item, ok := item.(string); ok && item == expected {
				return true
			}
		}
	case []string:
		for _, item := range typed {
			if item == expected {
				return true
			}
		}
	}
	return false
}

func numericClaim(value any) (float64, bool) {
	switch typed := value.(type) {
	case json.Number:
		result, err := typed.Float64()
		return result, err == nil
	case float64:
		return typed, true
	case int64:
		return float64(typed), true
	default:
		return 0, false
	}
}

func sha256Digest(value []byte) []byte {
	digest := sha256.Sum256(value)
	return digest[:]
}
