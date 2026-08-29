package config

import (
	"errors"
	"testing"
	"time"
)

func TestLoadRejectsDevelopmentIdentityInProduction(t *testing.T) {
	env := productionEnvironment()
	env["ALLOW_DEV_IDENTITY"] = "true"

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

func TestLoadRequiresEveryProductionProviderSetting(t *testing.T) {
	required := []string{
		"DATABASE_URL", "OIDC_ISSUER", "OIDC_AUDIENCE", "ADMIN_ALLOWED_ORIGINS",
		"S3_ENDPOINT", "S3_REGION", "S3_BUCKET", "S3_ACCESS_KEY_ID", "S3_SECRET_ACCESS_KEY",
		"EDGEONE_TRIGGER_URL", "EDGEONE_STATUS_URL", "EDGEONE_TOKEN",
	}
	for _, key := range required {
		t.Run(key, func(t *testing.T) {
			env := productionEnvironment()
			delete(env, key)
			_, err := Load(func(name string) string { return env[name] })
			if !errors.Is(err, ErrMissingProductionConfig) {
				t.Fatalf("expected missing production config for %s, got %v", key, err)
			}
		})
	}
}

func TestLoadParsesProviderConfiguration(t *testing.T) {
	env := productionEnvironment()
	env["S3_USE_PATH_STYLE"] = "true"

	settings, err := Load(func(key string) string { return env[key] })
	if err != nil {
		t.Fatalf("load production config: %v", err)
	}
	if settings.S3Endpoint != "https://cos.example.test" || settings.S3Region != "ap-singapore" ||
		settings.S3Bucket != "yujian-media" || !settings.S3UsePathStyle {
		t.Fatalf("unexpected S3 config %#v", settings)
	}
	if settings.EdgeOneTriggerURL == "" || settings.EdgeOneStatusURL == "" || settings.EdgeOneToken == "" {
		t.Fatalf("unexpected EdgeOne config %#v", settings)
	}
}

func TestLoadRejectsInsecureProductionProviderEndpoints(t *testing.T) {
	for _, key := range []string{"S3_ENDPOINT", "EDGEONE_TRIGGER_URL", "EDGEONE_STATUS_URL"} {
		t.Run(key, func(t *testing.T) {
			env := productionEnvironment()
			env[key] = "http://provider.example.test/path"
			_, err := Load(func(name string) string { return env[name] })
			if !errors.Is(err, ErrInvalidConfig) {
				t.Fatalf("expected invalid config for %s, got %v", key, err)
			}
		})
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
	env := productionEnvironment()
	env["ADMIN_ALLOWED_ORIGINS"] = "http://admin.yujian.me"

	_, err := Load(func(key string) string { return env[key] })
	if !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("expected invalid config error, got %v", err)
	}
}

func productionEnvironment() map[string]string {
	return map[string]string{
		"APP_ENV":               "production",
		"DATABASE_URL":          "postgres://example",
		"OIDC_ISSUER":           "https://id.example.com",
		"OIDC_AUDIENCE":         "yujian-admin",
		"ADMIN_ALLOWED_ORIGINS": "https://admin.yujian.me",
		"S3_ENDPOINT":           "https://cos.example.test",
		"S3_REGION":             "ap-singapore",
		"S3_BUCKET":             "yujian-media",
		"S3_ACCESS_KEY_ID":      "access-key",
		"S3_SECRET_ACCESS_KEY":  "secret-key",
		"EDGEONE_TRIGGER_URL":   "https://edgeone.example.test/trigger",
		"EDGEONE_STATUS_URL":    "https://edgeone.example.test/status",
		"EDGEONE_TOKEN":         "edgeone-token",
	}
}
