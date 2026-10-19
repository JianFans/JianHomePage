package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLoadEnvironmentUsesWorkingDirectory verifies the default .env path without
// requiring environment export, and keeps environment overrides authoritative.
func TestLoadEnvironmentUsesWorkingDirectory(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("APP_ENV", "test")
	t.Setenv("HTTP_ADDRESS", "127.0.0.1:19090")
	t.Setenv("SHUTDOWN_TIMEOUT", "")
	if err := os.Unsetenv("SHUTDOWN_TIMEOUT"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(".env", []byte("APP_ENV=development\nHTTP_ADDRESS=127.0.0.1:18080\nSHUTDOWN_TIMEOUT=12s\n"), 0600); err != nil {
		t.Fatal(err)
	}
	settings, err := LoadEnvironment()
	if err != nil || settings.Environment != "test" || settings.Address != "127.0.0.1:19090" || settings.ShutdownTimeout.Seconds() != 12 {
		t.Fatalf("unexpected configuration: environment=%q address=%q error=%v", settings.Environment, settings.Address, err)
	}
}

// TestDotEnvFallbackAndOverrides covers real dotenv syntax and explicitly empty
// overrides; file values never mutate the process environment.
func TestDotEnvFallbackAndOverrides(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(path, []byte("# config\r\nAPP_ENV=test\r\nHTTP_ADDRESS='127.0.0.1:18080' # comment\r\nexport SHUTDOWN_TIMEOUT=\"12s\"\r\nS3_SECRET_ACCESS_KEY='literal$UNSET#value'\r\n"), 0600); err != nil {
		t.Fatal(err)
	}
	lookup := func(key string) (string, bool) { return "", key == "HTTP_ADDRESS" }
	settings, err := loadEnvironment(path, lookup)
	if err != nil || settings.Environment != "test" || settings.Address != "127.0.0.1:8080" || settings.ShutdownTimeout.Seconds() != 12 {
		t.Fatalf("file configuration or overrides ignored: %+v, %v", settings, err)
	}
	settings, err = loadEnvironment(path, func(string) (string, bool) { return "", false })
	if err != nil || settings.Address != "127.0.0.1:18080" || settings.S3SecretAccessKey != "literal$UNSET#value" {
		t.Fatalf("quoted file value ignored: %q, %v", settings.Address, err)
	}
}

// TestDotEnvDoesNotSearchParentDirectories prevents an unrelated parent .env
// from silently changing a server launched with its own working directory.
func TestDotEnvDoesNotSearchParentDirectories(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("SHUTDOWN_TIMEOUT=invalid\n"), 0600); err != nil {
		t.Fatal(err)
	}
	child := filepath.Join(root, "server")
	if err := os.Mkdir(child, 0700); err != nil {
		t.Fatal(err)
	}
	t.Chdir(child)
	t.Setenv("APP_ENV", "test")
	if _, err := LoadEnvironment(); err != nil {
		t.Fatalf("loaded parent .env: %v", err)
	}
}

// TestDotEnvMissingFileKeepsEnvironmentDeployment permits containers and local
// development to supply settings without a physical file.
func TestDotEnvMissingFileKeepsEnvironmentDeployment(t *testing.T) {
	settings, err := loadEnvironment(filepath.Join(t.TempDir(), ".env"), func(key string) (string, bool) {
		return "test", key == "APP_ENV"
	})
	if err != nil || settings.Environment != "test" {
		t.Fatalf("missing optional .env rejected: %v", err)
	}
}

// TestDotEnvFailuresAreRedacted rejects unreadable or malformed files before
// startup without exposing values from the dotenv parser's error messages.
func TestDotEnvFailuresAreRedacted(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ".env")
	if err := os.WriteFile(path, []byte("APP_ENV=test\nSECRET=\"sensitive-test-value\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, file := range []string{path, root} {
		_, err := loadEnvironment(file, func(string) (string, bool) { return "", false })
		if !errors.Is(err, ErrInvalidConfig) || strings.Contains(err.Error(), "sensitive-test-value") {
			t.Fatalf("expected redacted configuration error, got %v", err)
		}
	}
}
