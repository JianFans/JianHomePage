package config_test

import (
	"encoding/base64"
	"testing"
	"yujian.me/server/internal/config"
)

// TestAdminLoginRejectsPartialConfiguration prevents silently disabled login
// after incomplete deployment configuration or an invalid encryption key.
func TestAdminLoginRejectsPartialConfiguration(t *testing.T) {
	for _, values := range []map[string]string{
		{"ADMIN_OIDC_CLIENT_ID": "client"},
		{"ADMIN_OIDC_CLIENT_SECRET": "secret"},
		{"ADMIN_OIDC_CLIENT_ID": "client", "OIDC_ISSUER": "http://issuer.local", "ADMIN_OIDC_REDIRECT_URL": "http://api.local/api/v1/auth/callback", "ADMIN_RETURN_URL": "http://admin.local/", "ADMIN_SESSION_KEY": "bad", "ADMIN_ALLOWED_ORIGINS": "http://admin.local"},
	} {
		if _, err := config.Load(func(key string) string { return values[key] }); err == nil {
			t.Fatal("partial admin login configuration accepted")
		}
	}
}

// TestAdminLoginConfiguration checks key decoding and the fixed allowed return origin.
func TestAdminLoginConfiguration(t *testing.T) {
	values := map[string]string{"ADMIN_OIDC_CLIENT_ID": "client", "OIDC_ISSUER": "http://issuer.local", "ADMIN_OIDC_REDIRECT_URL": "http://api.local/api/v1/auth/callback", "ADMIN_RETURN_URL": "http://admin.local/", "ADMIN_SESSION_KEY": base64.StdEncoding.EncodeToString(make([]byte, 32)), "ADMIN_ALLOWED_ORIGINS": "http://admin.local"}
	settings, err := config.Load(func(key string) string { return values[key] })
	if err != nil {
		t.Fatal(err)
	}
	if settings.AdminOIDCClientID != "client" || len(settings.AdminSessionKey) != 32 {
		t.Fatal("login configuration lost")
	}
	values["ADMIN_RETURN_URL"] = "http://evil.local/"
	if _, err := config.Load(func(key string) string { return values[key] }); err == nil {
		t.Fatal("untrusted return origin accepted")
	}
}
