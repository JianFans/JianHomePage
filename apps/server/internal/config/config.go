package config

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

var (
	ErrInvalidConfig             = errors.New("invalid configuration")
	ErrMissingProductionConfig   = errors.New("missing production configuration")
	ErrUnsafeDevelopmentIdentity = errors.New("development identity is forbidden in production")
)

type Config struct {
	Environment      string
	Address          string
	DatabaseURL      string
	OIDCIssuer       string
	OIDCAudience     string
	AllowDevIdentity bool
	ShutdownTimeout  time.Duration
}

func Load(getenv func(string) string) (Config, error) {
	config := Config{
		Environment:     valueOrDefault(strings.TrimSpace(getenv("APP_ENV")), "development"),
		Address:         valueOrDefault(strings.TrimSpace(getenv("HTTP_ADDRESS")), "127.0.0.1:8080"),
		DatabaseURL:     strings.TrimSpace(getenv("DATABASE_URL")),
		OIDCIssuer:      strings.TrimSpace(getenv("OIDC_ISSUER")),
		OIDCAudience:    strings.TrimSpace(getenv("OIDC_AUDIENCE")),
		ShutdownTimeout: 10 * time.Second,
	}

	if raw := strings.TrimSpace(getenv("ALLOW_DEV_IDENTITY")); raw != "" {
		allowed, err := strconv.ParseBool(raw)
		if err != nil {
			return Config{}, fmt.Errorf("%w: ALLOW_DEV_IDENTITY: %v", ErrInvalidConfig, err)
		}
		config.AllowDevIdentity = allowed
	}

	if raw := strings.TrimSpace(getenv("SHUTDOWN_TIMEOUT")); raw != "" {
		timeout, err := time.ParseDuration(raw)
		if err != nil || timeout <= 0 {
			return Config{}, fmt.Errorf("%w: SHUTDOWN_TIMEOUT must be a positive duration", ErrInvalidConfig)
		}
		config.ShutdownTimeout = timeout
	}

	if config.Environment == "production" {
		if config.AllowDevIdentity {
			return Config{}, ErrUnsafeDevelopmentIdentity
		}
		if config.DatabaseURL == "" || config.OIDCIssuer == "" || config.OIDCAudience == "" {
			return Config{}, ErrMissingProductionConfig
		}
	}

	return config, nil
}

func valueOrDefault(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
