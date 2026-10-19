package config

import (
	"errors"
	"testing"
)

// TestLoadOIDCRoleNames keeps existing installations compatible and maps each
// optional environment setting to exactly one internal permission level.
func TestLoadOIDCRoleNames(t *testing.T) {
	env := map[string]string{}
	settings, err := Load(func(key string) string { return env[key] })
	if err != nil || settings.OIDCRoleNames != (OIDCRoleNames{"editor", "reviewer", "publisher", "admin"}) {
		t.Fatalf("unexpected defaults: %#v, %v", settings.OIDCRoleNames, err)
	}
	env = map[string]string{
		"OIDC_ROLE_EDITOR": "org/编辑", "OIDC_ROLE_REVIEWER": "site-review",
		"OIDC_ROLE_PUBLISHER": "site-publish", "OIDC_ROLE_ADMIN": "site-admin",
	}
	settings, err = Load(func(key string) string { return env[key] })
	if err != nil || settings.OIDCRoleNames != (OIDCRoleNames{"org/编辑", "site-review", "site-publish", "site-admin"}) {
		t.Fatalf("unexpected custom names: %#v, %v", settings.OIDCRoleNames, err)
	}
}

// TestLoadRejectsInvalidOIDCRoleNames makes configuration errors fail before
// either browser authentication or Bearer authentication is initialized.
func TestLoadRejectsInvalidOIDCRoleNames(t *testing.T) {
	for _, value := range []string{"editor", "site admin", "site\nadmin", "site\u0000admin", "site\u00a0admin"} {
		_, err := Load(func(key string) string {
			if key == "OIDC_ROLE_ADMIN" {
				return value
			}
			return ""
		})
		if !errors.Is(err, ErrInvalidConfig) {
			t.Fatalf("invalid name accepted: %q, %v", value, err)
		}
	}
}
