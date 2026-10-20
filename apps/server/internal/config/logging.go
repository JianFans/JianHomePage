package config

import (
	"fmt"
	"strconv"
	"strings"
)

// LoggingConfig specifies console format and optional bounded file retention.
type LoggingConfig struct {
	Level, Format, File   string
	MaxSizeMB, MaxBackups int
}

// loadLogging reads operational logger settings before opening infrastructure.
func loadLogging(getenv func(string) string) (LoggingConfig, error) {
	settings := LoggingConfig{
		Level:  valueOrDefault(strings.TrimSpace(getenv("LOG_LEVEL")), "info"),
		Format: valueOrDefault(strings.TrimSpace(getenv("LOG_FORMAT")), "text"),
		File:   strings.TrimSpace(getenv("LOG_FILE")), MaxSizeMB: 20, MaxBackups: 5,
	}
	if settings.Level != "debug" && settings.Level != "info" && settings.Level != "warn" && settings.Level != "error" {
		return LoggingConfig{}, fmt.Errorf("%w: LOG_LEVEL must be debug, info, warn, or error", ErrInvalidConfig)
	}
	if settings.Format != "text" && settings.Format != "json" {
		return LoggingConfig{}, fmt.Errorf("%w: LOG_FORMAT must be text or json", ErrInvalidConfig)
	}
	if strings.ContainsAny(settings.File, "\x00\r\n") {
		return LoggingConfig{}, fmt.Errorf("%w: LOG_FILE contains invalid path characters", ErrInvalidConfig)
	}
	for _, setting := range []struct {
		key     string
		target  *int
		maximum int
	}{
		{"LOG_MAX_SIZE_MB", &settings.MaxSizeMB, 1024}, {"LOG_MAX_BACKUPS", &settings.MaxBackups, 100},
	} {
		if raw := strings.TrimSpace(getenv(setting.key)); raw != "" {
			value, err := strconv.Atoi(raw)
			if err != nil || value < 1 || value > setting.maximum {
				return LoggingConfig{}, fmt.Errorf("%w: %s must be an integer between 1 and %d", ErrInvalidConfig, setting.key, setting.maximum)
			}
			*setting.target = value
		}
	}
	return settings, nil
}
