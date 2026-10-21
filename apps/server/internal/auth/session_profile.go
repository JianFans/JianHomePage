package auth

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"yujian.me/server/internal/domain"
	"yujian.me/server/internal/httpurl"
)

type sessionDiscovery struct {
	Issuer        string `json:"issuer"`
	Authorization string `json:"authorization_endpoint"`
	Token         string `json:"token_endpoint"`
	UserInfo      string `json:"userinfo_endpoint"`
}

// enrichProfile fetches optional display data after ID-token verification. It
// never persists access tokens, follows redirects or changes authenticated roles.
func (manager *Sessions) enrichProfile(r *http.Request, principal *domain.Principal, endpoint, accessToken, tokenType string) {
	if endpoint == "" || accessToken == "" {
		return
	}
	// Only fixed reasons are logged: provider URLs and response data can be private.
	fail := func(reason string) {
		logAuthentication(r, slog.LevelWarn, "callback.profile", reason, principal.Subject, nil)
	}
	parsed, err := httpurl.ParseAbsolute(endpoint)
	if err != nil || parsed.Fragment != "" || (manager.options.Secure && parsed.Scheme != "https") {
		fail("userinfo_endpoint_invalid")
		return
	}
	if !strings.EqualFold(tokenType, "Bearer") || len(accessToken) > defaultOIDCTokenBytes || strings.ContainsAny(accessToken, "\r\n") {
		fail("userinfo_token_invalid")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		fail("userinfo_request_invalid")
		return
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Authorization", "Bearer "+accessToken)
	client := *manager.verifier.httpClient
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	response, err := client.Do(request)
	if err != nil {
		fail("userinfo_unavailable")
		return
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		fail("userinfo_http_error")
		return
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, defaultOIDCResponseBytes+1))
	if err != nil || len(body) > defaultOIDCResponseBytes {
		fail("userinfo_response_invalid")
		return
	}
	var claims map[string]any
	if json.Unmarshal(body, &claims) != nil {
		fail("userinfo_response_invalid")
		return
	}
	if subject, _ := claims["sub"].(string); subject != principal.Subject {
		fail("userinfo_subject_mismatch")
		return
	}
	manager.verifier.applyProfile(principal, claims)
}
