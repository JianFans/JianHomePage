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
	S3Endpoint          string
	S3Region            string
	S3Bucket            string
	S3AccessKeyID       string
	S3SecretAccessKey   string
	S3SessionToken      string
	S3UsePathStyle      bool
	EdgeOneTriggerURL   string
	EdgeOneStatusURL    string
	EdgeOneToken        string
	AllowDevIdentity    bool
	ShutdownTimeout     time.Duration
}

func Load(getenv func(string) string) (Config, error) {
	config := Config{
		Environment:       valueOrDefault(strings.TrimSpace(getenv("APP_ENV")), "development"),
		Address:           valueOrDefault(strings.TrimSpace(getenv("HTTP_ADDRESS")), "127.0.0.1:8080"),
		DatabaseURL:       strings.TrimSpace(getenv("DATABASE_URL")),
		OIDCIssuer:        strings.TrimSpace(getenv("OIDC_ISSUER")),
		OIDCAudience:      strings.TrimSpace(getenv("OIDC_AUDIENCE")),
		S3Endpoint:        strings.TrimSpace(getenv("S3_ENDPOINT")),
		S3Region:          strings.TrimSpace(getenv("S3_REGION")),
		S3Bucket:          strings.TrimSpace(getenv("S3_BUCKET")),
		S3AccessKeyID:     strings.TrimSpace(getenv("S3_ACCESS_KEY_ID")),
		S3SecretAccessKey: strings.TrimSpace(getenv("S3_SECRET_ACCESS_KEY")),
		S3SessionToken:    strings.TrimSpace(getenv("S3_SESSION_TOKEN")),
		EdgeOneTriggerURL: strings.TrimSpace(getenv("EDGEONE_TRIGGER_URL")),
		EdgeOneStatusURL:  strings.TrimSpace(getenv("EDGEONE_STATUS_URL")),
		EdgeOneToken:      strings.TrimSpace(getenv("EDGEONE_TOKEN")),
		ShutdownTimeout:   10 * time.Second,
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
	if raw := strings.TrimSpace(getenv("S3_USE_PATH_STYLE")); raw != "" {
		usePathStyle, err := strconv.ParseBool(raw)
		if err != nil {
			return Config{}, fmt.Errorf("%w: S3_USE_PATH_STYLE: %v", ErrInvalidConfig, err)
		}
		config.S3UsePathStyle = usePathStyle
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
		if config.DatabaseURL == "" || config.OIDCIssuer == "" || config.OIDCAudience == "" ||
			len(config.AllowedAdminOrigins) == 0 || config.S3Endpoint == "" || config.S3Region == "" ||
			config.S3Bucket == "" || config.S3AccessKeyID == "" || config.S3SecretAccessKey == "" ||
			config.EdgeOneTriggerURL == "" || config.EdgeOneStatusURL == "" || config.EdgeOneToken == "" {
			return Config{}, ErrMissingProductionConfig
		}
	}
	for name, endpoint := range map[string]string{
		"S3_ENDPOINT":         config.S3Endpoint,
		"EDGEONE_TRIGGER_URL": config.EdgeOneTriggerURL,
		"EDGEONE_STATUS_URL":  config.EdgeOneStatusURL,
	} {
		if endpoint != "" {
			if err := validateEndpoint(endpoint, config.Environment == "production"); err != nil {
				return Config{}, fmt.Errorf("%w: %s: %v", ErrInvalidConfig, name, err)
			}
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

func validateEndpoint(value string, requireHTTPS bool) error {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.User != nil {
		return errors.New("must be an absolute HTTP URL without credentials")
	}
	if requireHTTPS && parsed.Scheme != "https" {
		return errors.New("must use HTTPS in production")
	}
	return nil
}
