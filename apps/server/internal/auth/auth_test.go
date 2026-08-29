package auth_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"yujian.me/server/internal/auth"
	"yujian.me/server/internal/config"
	"yujian.me/server/internal/domain"
)

type identityProviderStub struct {
	principal domain.Principal
	err       error
	tokens    []string
}

func (provider *identityProviderStub) Authenticate(_ context.Context, token string) (domain.Principal, error) {
	provider.tokens = append(provider.tokens, token)
	return provider.principal, provider.err
}

func TestCanEnforcesRoleBoundaries(t *testing.T) {
	tests := []struct {
		role       auth.Role
		permission auth.Permission
		allowed    bool
	}{
		{auth.RoleEditor, auth.PermissionEditDraft, true},
		{auth.RoleEditor, auth.PermissionSubmitReview, true},
		{auth.RoleEditor, auth.PermissionPublish, false},
		{auth.RoleReviewer, auth.PermissionReview, true},
		{auth.RoleReviewer, auth.PermissionEditDraft, false},
		{auth.RolePublisher, auth.PermissionPublish, true},
		{auth.RolePublisher, auth.PermissionRollback, true},
		{auth.RoleAdmin, auth.PermissionManageUsers, true},
		{auth.RoleAdmin, auth.PermissionDeleteAsset, true},
	}

	for _, test := range tests {
		if got := auth.Can(test.role, test.permission); got != test.allowed {
			t.Fatalf("role %s permission %s: got %v, want %v", test.role, test.permission, got, test.allowed)
		}
	}
}

func TestParseRolesRejectsUnknownValues(t *testing.T) {
	roles, err := auth.ParseRoles([]string{"editor", "root", "publisher"})
	if err == nil {
		t.Fatal("expected unknown role error")
	}
	if roles != nil {
		t.Fatalf("expected no roles on invalid input, got %v", roles)
	}
}

func TestParseRolesDeduplicatesKnownValues(t *testing.T) {
	roles, err := auth.ParseRoles([]string{"editor", "publisher", "editor"})
	if err != nil {
		t.Fatalf("parse roles: %v", err)
	}
	if len(roles) != 2 || roles[0] != auth.RoleEditor || roles[1] != auth.RolePublisher {
		t.Fatalf("unexpected roles %v", roles)
	}
}

func TestMiddlewareRequiresBearerToken(t *testing.T) {
	provider := &identityProviderStub{}
	middleware, err := auth.NewMiddleware(auth.MiddlewareOptions{IdentityProvider: provider})
	if err != nil {
		t.Fatalf("new middleware: %v", err)
	}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/versions", nil)

	middleware.Authenticate(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("unauthenticated request reached handler")
	})).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", recorder.Code)
	}
	assertErrorCode(t, recorder, "unauthorized")
}

func TestMiddlewareStoresPrincipalAndEnforcesPermission(t *testing.T) {
	provider := &identityProviderStub{principal: domain.Principal{
		Subject: "editor-1",
		Roles:   []domain.Role{domain.RoleEditor},
	}}
	middleware, err := auth.NewMiddleware(auth.MiddlewareOptions{IdentityProvider: provider})
	if err != nil {
		t.Fatalf("new middleware: %v", err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/publishes", nil)
	request.Header.Set("Authorization", "Bearer token-1")
	recorder := httptest.NewRecorder()

	middleware.Authenticate(middleware.Require(auth.PermissionPublish, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("editor reached publish handler")
	}))).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", recorder.Code)
	}
	if len(provider.tokens) != 1 || provider.tokens[0] != "token-1" {
		t.Fatalf("unexpected tokens %v", provider.tokens)
	}
}

func TestMiddlewareMakesPrincipalAvailableToHandler(t *testing.T) {
	provider := &identityProviderStub{principal: domain.Principal{
		Subject: "admin-1",
		Roles:   []domain.Role{domain.RoleAdmin},
	}}
	middleware, err := auth.NewMiddleware(auth.MiddlewareOptions{IdentityProvider: provider})
	if err != nil {
		t.Fatalf("new middleware: %v", err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/publishes", nil)
	request.Header.Set("Authorization", "Bearer token-admin")
	recorder := httptest.NewRecorder()

	middleware.Authenticate(middleware.Require(auth.PermissionPublish, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		principal, ok := auth.PrincipalFromContext(request.Context())
		if !ok || principal.Subject != "admin-1" {
			t.Fatalf("unexpected principal %#v ok=%v", principal, ok)
		}
		writer.WriteHeader(http.StatusNoContent)
	}))).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", recorder.Code)
	}
}

func TestDevelopmentIdentityRequiresExplicitNonProductionMode(t *testing.T) {
	_, err := auth.NewMiddleware(auth.MiddlewareOptions{
		Environment:      "production",
		AllowDevIdentity: true,
	})
	if !errors.Is(err, config.ErrUnsafeDevelopmentIdentity) {
		t.Fatalf("expected production safety error, got %v", err)
	}

	middleware, err := auth.NewMiddleware(auth.MiddlewareOptions{
		Environment:      "development",
		AllowDevIdentity: true,
	})
	if err != nil {
		t.Fatalf("new development middleware: %v", err)
	}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/versions", nil)
	request.Header.Set("X-Dev-Subject", "dev-editor")
	request.Header.Set("X-Dev-Roles", "editor,reviewer")
	recorder := httptest.NewRecorder()

	middleware.Authenticate(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		principal, ok := auth.PrincipalFromContext(request.Context())
		if !ok || principal.Subject != "dev-editor" || len(principal.Roles) != 2 {
			t.Fatalf("unexpected development principal %#v", principal)
		}
		writer.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", recorder.Code)
	}
}

func TestDevelopmentHeadersAreIgnoredWhenDisabled(t *testing.T) {
	middleware, err := auth.NewMiddleware(auth.MiddlewareOptions{})
	if err != nil {
		t.Fatalf("new middleware: %v", err)
	}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/versions", nil)
	request.Header.Set("X-Dev-Subject", "dev-editor")
	request.Header.Set("X-Dev-Roles", "editor")
	recorder := httptest.NewRecorder()

	middleware.Authenticate(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("disabled development identity reached handler")
	})).ServeHTTP(recorder, request)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", recorder.Code)
	}
}

func assertErrorCode(t *testing.T, recorder *httptest.ResponseRecorder, expected string) {
	t.Helper()
	var body struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error body: %v", err)
	}
	if body.Code != expected {
		t.Fatalf("unexpected error code %q", body.Code)
	}
}
