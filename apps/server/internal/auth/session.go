package auth

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"yujian.me/server/internal/domain"
	"yujian.me/server/internal/httpurl"
)

const sessionCookieName = "yujian_session"
const flowCookieName = "yujian_login"

// SessionPrincipal is the explicit browser-visible identity contract.
type SessionPrincipal struct {
	Subject string   `json:"subject"`
	Name    string   `json:"name,omitempty"`
	Email   string   `json:"email,omitempty"`
	Roles   []string `json:"roles"`
}

// Session stores only a hash of the opaque bearer, never the browser cookie.
type Session struct {
	TokenHash string
	Principal SessionPrincipal
	CSRFToken string
	ExpiresAt time.Time
}

// SessionStore persists revocable sessions and atomically consumes login flows.
type SessionStore interface {
	CreateSession(context.Context, Session) error
	GetSession(context.Context, string) (Session, error)
	DeleteSession(context.Context, string) error
	ConsumeLoginFlow(context.Context, string, time.Time) error
}

// SessionOptions configures one fixed OIDC client and browser return target.
type SessionOptions struct {
	Issuer         string
	ClientID       string
	ClientSecret   string
	RedirectURL    string
	ReturnURL      string
	Key            []byte
	Store          SessionStore
	AllowedOrigins []string
	Secure         bool
	HTTPClient     *http.Client
	Now            func() time.Time
}

// Sessions owns authorization-code authentication and opaque browser sessions.
type Sessions struct {
	options  SessionOptions
	verifier *OIDCProvider
	aead     cipher.AEAD
}

type loginFlow struct {
	State     string    `json:"state"`
	Nonce     string    `json:"nonce"`
	Verifier  string    `json:"verifier"`
	ExpiresAt time.Time `json:"expiresAt"`
}

// NewSessions rejects partial login configuration and unsafe production URLs.
func NewSessions(options SessionOptions) (*Sessions, error) {
	if options.Now == nil {
		options.Now = time.Now
	}
	manager := &Sessions{options: options}
	if options.ClientID == "" && options.ClientSecret == "" && options.RedirectURL == "" && options.ReturnURL == "" && len(options.Key) == 0 {
		return manager, nil
	}
	if options.Issuer == "" || options.ClientID == "" || options.RedirectURL == "" || options.ReturnURL == "" || len(options.Key) != 32 || options.Store == nil {
		return nil, errors.New("incomplete admin login configuration")
	}
	for _, endpoint := range []string{options.RedirectURL, options.ReturnURL} {
		parsed, err := httpurl.ParseAbsolute(endpoint)
		if err != nil || parsed.Fragment != "" || (options.Secure && parsed.Scheme != "https") {
			return nil, errors.New("invalid admin login URL")
		}
	}
	returned, _ := url.Parse(options.ReturnURL)
	if !slices.Contains(options.AllowedOrigins, returned.Scheme+"://"+returned.Host) {
		return nil, errors.New("admin return URL origin is not allowed")
	}
	verifier, err := NewOIDCProvider(OIDCConfig{Issuer: options.Issuer, Audience: options.ClientID, RequireHTTPS: options.Secure, HTTPClient: options.HTTPClient, Now: options.Now})
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(options.Key)
	if err != nil {
		return nil, err
	}
	manager.aead, err = cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	manager.verifier = verifier
	return manager, nil
}

// SessionTokenHash derives the database lookup key without storing the bearer.
func SessionTokenHash(token string) string {
	digest := sha256.Sum256([]byte(token))
	return base64.RawURLEncoding.EncodeToString(digest[:])
}

// ServeHTTP exposes only the four public authentication routes.
func (manager *Sessions) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	switch r.Method + " " + r.URL.Path {
	case "GET /api/v1/auth/session":
		result := map[string]any{"authenticated": false, "loginAvailable": manager.verifier != nil}
		session, err := manager.readSession(r)
		if err == nil {
			result["authenticated"] = true
			result["principal"] = session.Principal
			result["csrfToken"] = session.CSRFToken
			result["expiresAt"] = session.ExpiresAt
		}
		if err != nil && !errors.Is(err, domain.ErrForbidden) && !errors.Is(err, domain.ErrNotFound) {
			writeAuthError(w, r, 503, "unavailable", "Session service unavailable.")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(result)
	case "GET /api/v1/auth/login":
		manager.login(w, r)
	case "GET /api/v1/auth/callback":
		manager.callback(w, r)
	case "POST /api/v1/auth/logout":
		session, err := manager.readSession(r)
		if err != nil {
			writeAuthError(w, r, 401, "unauthorized", "Authentication required.")
			return
		}
		if !manager.validCSRF(r, session) {
			writeAuthError(w, r, 403, "forbidden", "Request verification failed.")
			return
		}
		if err := manager.options.Store.DeleteSession(r.Context(), session.TokenHash); err != nil {
			writeAuthError(w, r, 503, "unavailable", "Session service unavailable.")
			return
		}
		manager.cookie(w, sessionCookieName, "", time.Time{}, -1, "/api/")
		w.WriteHeader(204)
	default:
		http.NotFound(w, r)
	}
}

// readSession rejects expired and revoked cookies on every request.
func (manager *Sessions) readSession(r *http.Request) (Session, error) {
	if manager.verifier == nil || manager.options.Store == nil {
		return Session{}, domain.ErrForbidden
	}
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || cookie.Value == "" || len(cookie.Value) > 128 {
		return Session{}, domain.ErrForbidden
	}
	session, err := manager.options.Store.GetSession(r.Context(), SessionTokenHash(cookie.Value))
	if err != nil {
		return Session{}, err
	}
	if !manager.options.Now().Before(session.ExpiresAt) {
		return Session{}, domain.ErrForbidden
	}
	return session, nil
}

// validCSRF binds every cookie-authenticated write to an exact trusted origin.
func (manager *Sessions) validCSRF(r *http.Request, session Session) bool {
	return slices.Contains(manager.options.AllowedOrigins, r.Header.Get("Origin")) && session.CSRFToken != "" && subtle.ConstantTimeCompare([]byte(r.Header.Get("X-CSRF-Token")), []byte(session.CSRFToken)) == 1
}

// authenticateCookie translates persisted identity without accepting dev headers.
func (manager *Sessions) authenticateCookie(r *http.Request) (domain.Principal, error) {
	session, err := manager.readSession(r)
	if err != nil {
		return domain.Principal{}, err
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions && !manager.validCSRF(r, session) {
		return domain.Principal{}, errCSRF
	}
	roles, err := ParseRoles(session.Principal.Roles)
	if err != nil || len(roles) == 0 || session.Principal.Subject == "" {
		return domain.Principal{}, domain.ErrForbidden
	}
	return domain.Principal{Subject: session.Principal.Subject, Email: session.Principal.Email, Name: session.Principal.Name, Roles: roles}, nil
}

var errCSRF = errors.New("invalid cookie request verification")

// discovery resolves endpoints from the configured issuer with transport checks.
func (manager *Sessions) discovery(ctx context.Context) (string, string, error) {
	body, err := manager.verifier.fetch(ctx, manager.verifier.issuer+"/.well-known/openid-configuration")
	if err != nil {
		return "", "", err
	}
	var doc struct {
		Issuer        string `json:"issuer"`
		Authorization string `json:"authorization_endpoint"`
		Token         string `json:"token_endpoint"`
	}
	if json.Unmarshal(body, &doc) != nil || doc.Issuer != manager.verifier.issuer {
		return "", "", errors.New("invalid discovery")
	}
	for _, endpoint := range []string{doc.Authorization, doc.Token} {
		parsed, err := httpurl.ParseAbsolute(endpoint)
		if err != nil || parsed.Fragment != "" || (manager.options.Secure && parsed.Scheme != "https") {
			return "", "", errors.New("invalid OIDC endpoint")
		}
	}
	return doc.Authorization, doc.Token, nil
}

// login protects state, nonce and the PKCE verifier in a short-lived AEAD cookie.
func (manager *Sessions) login(w http.ResponseWriter, r *http.Request) {
	if manager.verifier == nil {
		writeAuthError(w, r, 503, "login_unavailable", "Login is not configured.")
		return
	}
	authorization, _, err := manager.discovery(r.Context())
	if err != nil {
		writeAuthError(w, r, 503, "login_unavailable", "Identity provider unavailable.")
		return
	}
	flow := loginFlow{State: randomToken(), Nonce: randomToken(), Verifier: randomToken(), ExpiresAt: manager.options.Now().Add(10 * time.Minute)}
	if flow.State == "" || flow.Nonce == "" || flow.Verifier == "" {
		writeAuthError(w, r, 503, "unavailable", "Login unavailable.")
		return
	}
	body, _ := json.Marshal(flow)
	nonce := make([]byte, manager.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		writeAuthError(w, r, 503, "unavailable", "Login unavailable.")
		return
	}
	encrypted := manager.aead.Seal(nonce, nonce, body, []byte(flowCookieName))
	manager.cookie(w, flowCookieName, base64.RawURLEncoding.EncodeToString(encrypted), flow.ExpiresAt, 600, "/api/v1/auth/")
	endpoint, _ := url.Parse(authorization)
	query := endpoint.Query()
	digest := sha256.Sum256([]byte(flow.Verifier))
	query.Set("response_type", "code")
	query.Set("scope", "openid profile email")
	query.Set("client_id", manager.options.ClientID)
	query.Set("redirect_uri", manager.options.RedirectURL)
	query.Set("state", flow.State)
	query.Set("nonce", flow.Nonce)
	query.Set("code_challenge", base64.RawURLEncoding.EncodeToString(digest[:]))
	query.Set("code_challenge_method", "S256")
	endpoint.RawQuery = query.Encode()
	http.Redirect(w, r, endpoint.String(), 302)
}

// callback consumes one flow and accepts only a verified, nonce-bound ID token.
func (manager *Sessions) callback(w http.ResponseWriter, r *http.Request) {
	manager.cookie(w, flowCookieName, "", time.Time{}, -1, "/api/v1/auth/")
	fail := func() { writeAuthError(w, r, 401, "unauthorized", "Login verification failed.") }
	if manager.verifier == nil {
		fail()
		return
	}
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil || len(query["state"]) != 1 || len(query["code"]) != 1 || len(query["error"]) != 0 || len(query.Get("code")) > 4096 {
		fail()
		return
	}
	cookie, err := r.Cookie(flowCookieName)
	if err != nil || len(cookie.Value) > 4096 {
		fail()
		return
	}
	encrypted, err := base64.RawURLEncoding.DecodeString(cookie.Value)
	size := manager.aead.NonceSize()
	if err != nil || len(encrypted) < size {
		fail()
		return
	}
	body, err := manager.aead.Open(nil, encrypted[:size], encrypted[size:], []byte(flowCookieName))
	if err != nil {
		fail()
		return
	}
	var flow loginFlow
	if json.Unmarshal(body, &flow) != nil || !manager.options.Now().Before(flow.ExpiresAt) || flow.State == "" || subtle.ConstantTimeCompare([]byte(flow.State), []byte(query.Get("state"))) != 1 || query.Get("code") == "" {
		fail()
		return
	}
	if manager.options.Store.ConsumeLoginFlow(r.Context(), SessionTokenHash(flow.State), flow.ExpiresAt) != nil {
		fail()
		return
	}
	_, tokenEndpoint, err := manager.discovery(r.Context())
	if err != nil {
		fail()
		return
	}
	form := url.Values{"grant_type": {"authorization_code"}, "code": {query.Get("code")}, "client_id": {manager.options.ClientID}, "redirect_uri": {manager.options.RedirectURL}, "code_verifier": {flow.Verifier}}
	request, err := http.NewRequestWithContext(r.Context(), http.MethodPost, tokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		fail()
		return
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("Accept", "application/json")
	if manager.options.ClientSecret != "" {
		request.SetBasicAuth(url.QueryEscape(manager.options.ClientID), url.QueryEscape(manager.options.ClientSecret))
	}
	client := *manager.verifier.httpClient
	// Token endpoints must never redirect client credentials or authorization codes.
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	response, err := client.Do(request)
	if err != nil {
		fail()
		return
	}
	defer response.Body.Close()
	body, err = io.ReadAll(io.LimitReader(response.Body, defaultOIDCResponseBytes+1))
	if err != nil || len(body) > defaultOIDCResponseBytes || response.StatusCode != 200 {
		fail()
		return
	}
	var tokens struct {
		IDToken string `json:"id_token"`
	}
	if json.Unmarshal(body, &tokens) != nil {
		fail()
		return
	}
	principal, err := manager.verifier.Authenticate(r.Context(), tokens.IDToken)
	if err != nil {
		fail()
		return
	}
	parts := strings.Split(tokens.IDToken, ".")
	claimsBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		fail()
		return
	}
	var claims struct {
		Nonce           string `json:"nonce"`
		Expires         int64  `json:"exp"`
		AuthorizedParty string `json:"azp"`
		Audience        any    `json:"aud"`
	}
	if json.Unmarshal(claimsBytes, &claims) != nil || flow.Nonce == "" || subtle.ConstantTimeCompare([]byte(claims.Nonce), []byte(flow.Nonce)) != 1 || claims.Expires <= manager.options.Now().Unix() || (claims.AuthorizedParty != "" && claims.AuthorizedParty != manager.options.ClientID) {
		fail()
		return
	}
	if audiences, ok := claims.Audience.([]any); ok && len(audiences) > 1 && claims.AuthorizedParty != manager.options.ClientID {
		fail()
		return
	}
	expires := manager.options.Now().Add(8 * time.Hour)
	if tokenExpiry := time.Unix(claims.Expires, 0); tokenExpiry.Before(expires) {
		expires = tokenExpiry
	}
	token, csrf := randomToken(), randomToken()
	if token == "" || csrf == "" {
		fail()
		return
	}
	roles := make([]string, len(principal.Roles))
	for i, role := range principal.Roles {
		roles[i] = string(role)
	}
	session := Session{TokenHash: SessionTokenHash(token), Principal: SessionPrincipal{Subject: principal.Subject, Name: principal.Name, Email: principal.Email, Roles: roles}, CSRFToken: csrf, ExpiresAt: expires}
	// Replace an existing browser session so previously copied cookies are revoked.
	if old, err := r.Cookie(sessionCookieName); err == nil {
		if manager.options.Store.DeleteSession(r.Context(), SessionTokenHash(old.Value)) != nil {
			fail()
			return
		}
	}
	if manager.options.Store.CreateSession(r.Context(), session) != nil {
		fail()
		return
	}
	manager.cookie(w, sessionCookieName, token, expires, int(expires.Sub(manager.options.Now()).Seconds()), "/api/")
	http.Redirect(w, r, manager.options.ReturnURL, 302)
}

// cookie consistently scopes host-only authentication cookies to API routes.
func (manager *Sessions) cookie(w http.ResponseWriter, name, value string, expires time.Time, maxAge int, path string) {
	http.SetCookie(w, &http.Cookie{Name: name, Value: value, Path: path, Expires: expires, MaxAge: maxAge, HttpOnly: true, Secure: manager.options.Secure, SameSite: http.SameSiteLaxMode})
}

// randomToken returns 256 bits of CSPRNG entropy encoded for cookies and PKCE.
func randomToken() string {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(value)
}
