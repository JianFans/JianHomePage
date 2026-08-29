package config

import (
	"errors"
	"testing"
	"time"
)

func TestLoadRejectsDevelopmentIdentityInProduction(t *testing.T) {
	env := map[string]string{
		"APP_ENV":            "production",
		"DATABASE_URL":       "postgres://example",
		"OIDC_ISSUER":        "https://id.example.com",
		"OIDC_AUDIENCE":      "yujian-admin",
		"ALLOW_DEV_IDENTITY": "true",
	}

	_, err := Load(func(key string) string { return env[key] })
	if !errors.Is(err, ErrUnsafeDevelopmentIdentity) {
		t.Fatalf("expected unsafe identity error, got %v", err)
	}
}

func TestLoadRequiresProductionDependencies(t *testing.T) {
	env := map[string]string{"APP_ENV": "production"}

	_, err := Load(func(key string) string { return env[key] })
	if !errors.Is(err, ErrMissingProductionConfig) {
		t.Fatalf("expected missing production config error, got %v", err)
	}
}

func TestLoadUsesSafeDevelopmentDefaults(t *testing.T) {
	config, err := Load(func(string) string { return "" })
	if err != nil {
		t.Fatalf("load defaults: %v", err)
	}

	if config.Environment != "development" {
		t.Fatalf("unexpected environment %q", config.Environment)
	}
	if config.Address != "127.0.0.1:8080" {
		t.Fatalf("unexpected address %q", config.Address)
	}
	if config.AllowDevIdentity {
		t.Fatal("development identity must require explicit opt-in")
	}
	if config.ShutdownTimeout != 10*time.Second {
		t.Fatalf("unexpected shutdown timeout %s", config.ShutdownTimeout)
	}
}

func TestLoadRejectsInvalidShutdownTimeout(t *testing.T) {
	env := map[string]string{"SHUTDOWN_TIMEOUT": "not-a-duration"}

	_, err := Load(func(key string) string { return env[key] })
	if !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("expected invalid config error, got %v", err)
	}
}

func TestLoadRejectsUnknownEnvironment(t *testing.T) {
	env := map[string]string{"APP_ENV": "prod"}

	_, err := Load(func(key string) string { return env[key] })
	if !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("expected invalid config error, got %v", err)
	}
}

func TestLoadParsesAllowedAdminOrigins(t *testing.T) {
	env := map[string]string{
		"ADMIN_ALLOWED_ORIGINS": " https://admin.yujian.me, http://127.0.0.1:3001,https://admin.yujian.me ",
	}

	settings, err := Load(func(key string) string { return env[key] })
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	want := []string{"https://admin.yujian.me", "http://127.0.0.1:3001"}
	if len(settings.AllowedAdminOrigins) != len(want) {
		t.Fatalf("unexpected origins %#v", settings.AllowedAdminOrigins)
	}
	for index := range want {
		if settings.AllowedAdminOrigins[index] != want[index] {
			t.Fatalf("unexpected origins %#v", settings.AllowedAdminOrigins)
		}
	}
}

func TestLoadRejectsInsecureProductionAdminOrigin(t *testing.T) {
	env := map[string]string{
		"APP_ENV":               "production",
		"DATABASE_URL":          "postgres://example",
		"OIDC_ISSUER":           "https://id.example.com",
		"OIDC_AUDIENCE":         "yujian-admin",
		"ADMIN_ALLOWED_ORIGINS": "http://admin.yujian.me",
	}

	_, err := Load(func(key string) string { return env[key] })
	if !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("expected invalid config error, got %v", err)
	}
}
