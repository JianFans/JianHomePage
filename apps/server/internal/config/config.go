package config

import (
	"errors"
	"fmt"
	"net/url"
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
	Environment         string
	Address             string
	DatabaseURL         string
	OIDCIssuer          string
	OIDCAudience        string
	AllowedAdminOrigins []string
	AllowDevIdentity    bool
	ShutdownTimeout     time.Duration
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
	if !isEnvironment(config.Environment) {
		return Config{}, fmt.Errorf("%w: APP_ENV must be development, test, or production", ErrInvalidConfig)
	}

	origins, err := parseOrigins(getenv("ADMIN_ALLOWED_ORIGINS"), config.Environment == "production")
	if err != nil {
		return Config{}, err
	}
	config.AllowedAdminOrigins = origins

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

func isEnvironment(value string) bool {
	switch value {
	case "development", "test", "production":
		return true
	default:
		return false
	}
}

func parseOrigins(raw string, requireHTTPS bool) ([]string, error) {
	seen := make(map[string]struct{})
	origins := make([]string, 0)
	for _, candidate := range strings.Split(raw, ",") {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		parsed, err := url.Parse(candidate)
		if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") ||
			parsed.User != nil || (parsed.Path != "" && parsed.Path != "/") || parsed.RawQuery != "" || parsed.Fragment != "" {
			return nil, fmt.Errorf("%w: ADMIN_ALLOWED_ORIGINS contains an invalid origin", ErrInvalidConfig)
		}
		if requireHTTPS && parsed.Scheme != "https" {
			return nil, fmt.Errorf("%w: production admin origins must use HTTPS", ErrInvalidConfig)
		}
		origin := parsed.Scheme + "://" + parsed.Host
		if _, exists := seen[origin]; exists {
			continue
		}
		seen[origin] = struct{}{}
		origins = append(origins, origin)
	}
	return origins, nil
}
