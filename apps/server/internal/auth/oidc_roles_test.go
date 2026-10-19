package auth_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"reflect"
	"testing"
	"time"

	"yujian.me/server/internal/auth"
	"yujian.me/server/internal/config"
	"yujian.me/server/internal/domain"
)

// TestOIDCIgnoresUnrelatedRoles preserves site access for users who also hold
// roles in other applications, without granting permissions for those roles.
func TestOIDCIgnoresUnrelatedRoles(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_700_000_000, 0)
	server := oidcTestServer(key, func() time.Time { return now })
	defer server.Close()
	provider, err := auth.NewOIDCProvider(auth.OIDCConfig{
		Issuer: server.URL, Audience: "yujian-admin", Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, roles := range []any{
		[]string{"other-app-admin", "editor", "other-app-user", "reviewer", "editor"},
		"other-app-admin editor other-app-user reviewer editor",
	} {
		token := signJWT(t, key, map[string]any{
			"iss": server.URL, "sub": "editor-1", "aud": "yujian-admin",
			"exp": now.Add(time.Hour).Unix(), "roles": roles,
		})
		principal, err := provider.Authenticate(context.Background(), token)
		if err != nil {
			t.Fatalf("mixed roles rejected: %v", err)
		}
		if !reflect.DeepEqual(principal.Roles, []domain.Role{domain.RoleEditor, domain.RoleReviewer}) {
			t.Fatalf("unexpected permissions: %#v", principal.Roles)
		}
	}
}

// TestOIDCCustomRoleNames checks exact external-name matching and ensures that
// replacing a default name cannot leave an unintended administrator alias.
func TestOIDCCustomRoleNames(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_700_000_000, 0)
	server := oidcTestServer(key, func() time.Time { return now })
	defer server.Close()
	names := config.OIDCRoleNames{Editor: "org/编辑", Reviewer: "site-review", Publisher: "site-publish", Admin: "site-admin"}
	provider, err := auth.NewOIDCProvider(auth.OIDCConfig{
		Issuer: server.URL, Audience: "yujian-admin", Now: func() time.Time { return now }, RoleNames: names,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name  string
		claim any
		want  []domain.Role
	}{
		{"all mapped roles", []string{"unrelated", "org/编辑", "site-review", "site-publish", "site-admin", "site-admin"}, []domain.Role{domain.RoleEditor, domain.RoleReviewer, domain.RolePublisher, domain.RoleAdmin}},
		{"string claim", "unrelated site-admin", []domain.Role{domain.RoleAdmin}},
		{"no old administrator alias", []string{"admin", "org/编辑"}, []domain.Role{domain.RoleEditor}},
		{"old names rejected", []string{"editor", "reviewer", "publisher", "admin"}, nil},
		{"no recognized role", []string{"other-app-admin"}, nil},
		{"case sensitive", []string{"SITE-ADMIN"}, nil},
		{"malformed mixed array", []any{"site-admin", 1}, nil},
		{"missing roles", nil, nil},
		{"empty roles", []string{}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			token := signJWT(t, key, map[string]any{
				"iss": server.URL, "sub": "user-1", "aud": "yujian-admin",
				"exp": now.Add(time.Hour).Unix(), "roles": tc.claim,
			})
			principal, err := provider.Authenticate(context.Background(), token)
			if tc.want == nil {
				if err == nil {
					t.Fatalf("unexpected authentication: %#v", principal)
				}
				return
			}
			if err != nil || !reflect.DeepEqual(principal.Roles, tc.want) {
				t.Fatalf("roles = %#v, error = %v; want %#v", principal.Roles, err, tc.want)
			}
		})
	}
}

// TestOIDCRejectsAmbiguousRoleNames prevents a single external role from
// accidentally acquiring multiple internal permission levels.
func TestOIDCRejectsAmbiguousRoleNames(t *testing.T) {
	for _, names := range []config.OIDCRoleNames{
		{Editor: "admin"}, {Editor: "same", Admin: "same"}, {Admin: "site admin"},
		{Admin: "site\tadmin"}, {Admin: "site\u0000admin"}, {Admin: "site\u00a0admin"},
	} {
		if _, err := auth.NewOIDCProvider(auth.OIDCConfig{Issuer: "https://issuer.example", Audience: "client", RoleNames: names}); err == nil {
			t.Fatalf("invalid mapping accepted: %#v", names)
		}
	}
}
