package config

import (
	"errors"
	"strings"
	"testing"
)

// TestLoggingDefaultsAndOverrides keeps explicit logging configuration validated at startup.
func TestLoggingDefaultsAndOverrides(t *testing.T) {
	settings, err := Load(func(string) string { return "" })
	if err != nil {
		t.Fatal(err)
	}
	if settings.Log != (LoggingConfig{Level: "info", Format: "text", MaxSizeMB: 20, MaxBackups: 5}) {
		t.Fatalf("defaults %#v", settings.Log)
	}
	env := map[string]string{"LOG_LEVEL": "debug", "LOG_FORMAT": "json", "LOG_FILE": "logs/server.jsonl", "LOG_MAX_SIZE_MB": "4", "LOG_MAX_BACKUPS": "2"}
	settings, err = Load(func(key string) string { return env[key] })
	if err != nil || settings.Log != (LoggingConfig{Level: "debug", Format: "json", File: "logs/server.jsonl", MaxSizeMB: 4, MaxBackups: 2}) {
		t.Fatalf("settings %#v error=%v", settings.Log, err)
	}
}

// TestLoggingRejectsInvalidConfiguration prevents silent level and retention fallbacks.
func TestLoggingRejectsInvalidConfiguration(t *testing.T) {
	for _, tc := range []struct{ key, value string }{
		{"LOG_LEVEL", "trace"}, {"LOG_FORMAT", "yaml"}, {"LOG_MAX_SIZE_MB", "0"}, {"LOG_MAX_SIZE_MB", "-1"}, {"LOG_MAX_SIZE_MB", "1025"},
		{"LOG_MAX_SIZE_MB", "1.5"}, {"LOG_MAX_BACKUPS", "0"}, {"LOG_MAX_BACKUPS", "101"}, {"LOG_FILE", "bad\x00path"},
	} {
		_, err := Load(func(key string) string {
			if key == tc.key {
				return tc.value
			}
			return ""
		})
		if !errors.Is(err, ErrInvalidConfig) {
			t.Fatalf("accepted %s=%q: %v", tc.key, tc.value, err)
		}
	}
}

// TestLoggingAcceptsRetentionBoundaries protects the documented inclusive limits.
func TestLoggingAcceptsRetentionBoundaries(t *testing.T) {
	for _, tc := range []struct{ key, value string }{{"LOG_MAX_SIZE_MB", "1"}, {"LOG_MAX_SIZE_MB", "1024"}, {"LOG_MAX_BACKUPS", "1"}, {"LOG_MAX_BACKUPS", "100"}} {
		env := map[string]string{tc.key: tc.value}
		if _, err := Load(func(key string) string { return env[key] }); err != nil {
			t.Fatalf("rejected %s=%s: %v", tc.key, tc.value, err)
		}
	}
}

// TestInvalidBooleanSettingsDoNotEchoEnvironmentValues prevents startup diagnostics from exposing accidental secrets.
func TestInvalidBooleanSettingsDoNotEchoEnvironmentValues(t *testing.T) {
	for _, key := range []string{"ALLOW_DEV_IDENTITY", "S3_USE_PATH_STYLE"} {
		t.Run(key, func(t *testing.T) {
			const secret = "private-config-value"
			_, err := Load(func(name string) string {
				if name == key {
					return secret
				}
				return ""
			})
			if err == nil || !strings.Contains(err.Error(), key) || strings.Contains(err.Error(), secret) {
				t.Fatalf("unsafe configuration diagnostic for %s: %v", key, err)
			}
		})
	}
}
