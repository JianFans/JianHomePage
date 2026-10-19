package config

import (
	"encoding/base64"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"yujian.me/server/internal/httpurl"
)

var (
	ErrInvalidConfig             = errors.New("invalid configuration")
	ErrMissingProductionConfig   = errors.New("missing production configuration")
	ErrUnsafeDevelopmentIdentity = errors.New("development identity is forbidden in production")
)

type Config struct {
	Environment           string
	Address               string
	DatabaseURL           string
	OIDCIssuer            string
	OIDCAudience          string
	OIDCRoleNames         OIDCRoleNames
	AdminOIDCClientID     string
	AdminOIDCClientSecret string
	AdminOIDCRedirectURL  string
	AdminReturnURL        string
	AdminSessionKey       []byte
	AllowedAdminOrigins   []string
	S3Endpoint            string
	S3Region              string
	S3Bucket              string
	S3AccessKeyID         string
	S3SecretAccessKey     string
	S3SessionToken        string
	S3UsePathStyle        bool
	MediaPublicBaseURL    string
	PagesAPIToken         string
	PagesProjectID        string
	PagesRegion           string
	PagesPublicURL        string
	AllowDevIdentity      bool
	ShutdownTimeout       time.Duration
}

// Load validates explicit deployment dependencies before creating services.
func Load(getenv func(string) string) (Config, error) {
	config := Config{
		Environment:  valueOrDefault(strings.TrimSpace(getenv("APP_ENV")), "development"),
		Address:      valueOrDefault(strings.TrimSpace(getenv("HTTP_ADDRESS")), "127.0.0.1:8080"),
		DatabaseURL:  strings.TrimSpace(getenv("DATABASE_URL")),
		OIDCIssuer:   strings.TrimSpace(getenv("OIDC_ISSUER")),
		OIDCAudience: strings.TrimSpace(getenv("OIDC_AUDIENCE")),
		OIDCRoleNames: OIDCRoleNames{
			Editor: getenv("OIDC_ROLE_EDITOR"), Reviewer: getenv("OIDC_ROLE_REVIEWER"),
			Publisher: getenv("OIDC_ROLE_PUBLISHER"), Admin: getenv("OIDC_ROLE_ADMIN"),
		},
		AdminOIDCClientID:     strings.TrimSpace(getenv("ADMIN_OIDC_CLIENT_ID")),
		AdminOIDCClientSecret: strings.TrimSpace(getenv("ADMIN_OIDC_CLIENT_SECRET")),
		AdminOIDCRedirectURL:  strings.TrimSpace(getenv("ADMIN_OIDC_REDIRECT_URL")),
		AdminReturnURL:        strings.TrimSpace(getenv("ADMIN_RETURN_URL")),
		S3Endpoint:            strings.TrimSpace(getenv("S3_ENDPOINT")),
		S3Region:              strings.TrimSpace(getenv("S3_REGION")),
		S3Bucket:              strings.TrimSpace(getenv("S3_BUCKET")),
		S3AccessKeyID:         strings.TrimSpace(getenv("S3_ACCESS_KEY_ID")),
		S3SecretAccessKey:     strings.TrimSpace(getenv("S3_SECRET_ACCESS_KEY")),
		S3SessionToken:        strings.TrimSpace(getenv("S3_SESSION_TOKEN")),
		MediaPublicBaseURL:    strings.TrimSpace(getenv("MEDIA_PUBLIC_BASE_URL")),
		PagesAPIToken:         strings.TrimSpace(getenv("PAGES_API_TOKEN")),
		PagesProjectID:        strings.TrimSpace(getenv("PAGES_PROJECT_ID")),
		PagesRegion:           strings.TrimSpace(getenv("PAGES_REGION")),
		PagesPublicURL:        strings.TrimSpace(getenv("PAGES_PUBLIC_URL")),
		ShutdownTimeout:       10 * time.Second,
	}
	if !isEnvironment(config.Environment) {
		return Config{}, fmt.Errorf("%w: APP_ENV must be development, test, or production", ErrInvalidConfig)
	}
	roleNames, err := config.OIDCRoleNames.WithDefaults()
	if err != nil {
		return Config{}, err
	}
	config.OIDCRoleNames = roleNames

	origins, err := parseOrigins(getenv("ADMIN_ALLOWED_ORIGINS"), config.Environment == "production")
	if err != nil {
		return Config{}, err
	}
	config.AllowedAdminOrigins = origins
	if err := loadAdminLogin(&config, getenv("ADMIN_SESSION_KEY")); err != nil {
		return Config{}, err
	}

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
			config.MediaPublicBaseURL == "" ||
			config.PagesAPIToken == "" || config.PagesProjectID == "" || config.PagesRegion == "" || config.PagesPublicURL == "" {
			return Config{}, ErrMissingProductionConfig
		}
	}
	if config.OIDCIssuer != "" {
		if err := validateOIDCIssuer(config.OIDCIssuer, config.Environment == "production"); err != nil {
			return Config{}, fmt.Errorf("%w: OIDC_ISSUER: %v", ErrInvalidConfig, err)
		}
	}
	for name, endpoint := range map[string]string{
		"S3_ENDPOINT": config.S3Endpoint,
	} {
		if endpoint != "" {
			if err := validateEndpoint(endpoint, config.Environment == "production"); err != nil {
				return Config{}, fmt.Errorf("%w: %s: %v", ErrInvalidConfig, name, err)
			}
		}
	}
	if config.MediaPublicBaseURL != "" {
		if err := validatePublicBaseURL(config.MediaPublicBaseURL, config.Environment == "production"); err != nil {
			return Config{}, fmt.Errorf("%w: MEDIA_PUBLIC_BASE_URL: %v", ErrInvalidConfig, err)
		}
	}
	if config.PagesRegion != "" && config.PagesRegion != "china" && config.PagesRegion != "global" {
		return Config{}, fmt.Errorf("%w: PAGES_REGION must be china or global", ErrInvalidConfig)
	}
	if config.PagesProjectID != "" && !regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,255}$`).MatchString(config.PagesProjectID) {
		return Config{}, fmt.Errorf("%w: PAGES_PROJECT_ID is invalid", ErrInvalidConfig)
	}
	for _, value := range config.PagesAPIToken {
		if value <= 32 || value >= 127 {
			return Config{}, fmt.Errorf("%w: PAGES_API_TOKEN contains invalid characters", ErrInvalidConfig)
		}
	}
	if config.PagesPublicURL != "" {
		if err := validatePublicBaseURL(config.PagesPublicURL, config.Environment == "production"); err != nil {
			return Config{}, fmt.Errorf("%w: PAGES_PUBLIC_URL: %v", ErrInvalidConfig, err)
		}
		parsed, _ := httpurl.ParseAbsolute(config.PagesPublicURL)
		if parsed.ForceQuery {
			return Config{}, fmt.Errorf("%w: PAGES_PUBLIC_URL must not contain a query", ErrInvalidConfig)
		}
	}

	return config, nil
}

// loadAdminLogin permits a fully disabled browser login but rejects partial setup.
func loadAdminLogin(settings *Config, rawKey string) error {
	if settings.AdminOIDCClientID == "" && settings.AdminOIDCClientSecret == "" && settings.AdminOIDCRedirectURL == "" && settings.AdminReturnURL == "" && rawKey == "" {
		return nil
	}
	key, err := base64.StdEncoding.DecodeString(strings.TrimSpace(rawKey))
	if err != nil || len(key) != 32 || settings.AdminOIDCClientID == "" || settings.OIDCIssuer == "" || settings.AdminOIDCRedirectURL == "" || settings.AdminReturnURL == "" {
		return fmt.Errorf("%w: incomplete admin login configuration or invalid ADMIN_SESSION_KEY", ErrInvalidConfig)
	}
	for _, endpoint := range []string{settings.AdminOIDCRedirectURL, settings.AdminReturnURL} {
		parsed, err := httpurl.ParseAbsolute(endpoint)
		if err != nil || parsed.Fragment != "" || (settings.Environment == "production" && parsed.Scheme != "https") {
			return fmt.Errorf("%w: invalid admin login URL", ErrInvalidConfig)
		}
	}
	returned, _ := httpurl.ParseAbsolute(settings.AdminReturnURL)
	allowed := false
	for _, origin := range settings.AllowedAdminOrigins {
		if origin == returned.Scheme+"://"+returned.Host {
			allowed = true
		}
	}
	if !allowed {
		return fmt.Errorf("%w: ADMIN_RETURN_URL origin must be allowed", ErrInvalidConfig)
	}
	settings.AdminSessionKey = key
	return nil
}

// valueOrDefault supplies development defaults only for absent values.
func valueOrDefault(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

// isEnvironment restricts dependency modes to the supported deployment states.
func isEnvironment(value string) bool {
	switch value {
	case "development", "test", "production":
		return true
	default:
		return false
	}
}

// parseOrigins deduplicates exact origins while rejecting credentials and paths.
func parseOrigins(raw string, requireHTTPS bool) ([]string, error) {
	seen := make(map[string]struct{})
	origins := make([]string, 0)
	for _, candidate := range strings.Split(raw, ",") {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		parsed, err := httpurl.ParseAbsolute(candidate)
		if err != nil || (parsed.Path != "" && parsed.Path != "/") || parsed.RawQuery != "" || parsed.Fragment != "" {
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

// validateEndpoint accepts only absolute transport URLs for configured adapters.
func validateEndpoint(value string, requireHTTPS bool) error {
	parsed, err := httpurl.ParseAbsolute(value)
	if err != nil {
		return errors.New("must be an absolute HTTP URL without credentials")
	}
	if requireHTTPS && parsed.Scheme != "https" {
		return errors.New("must use HTTPS in production")
	}
	return nil
}

// validateOIDCIssuer prevents discovery targets containing query or fragments.
func validateOIDCIssuer(value string, requireHTTPS bool) error {
	parsed, err := httpurl.ParseAbsolute(value)
	if err != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return errors.New("must be an absolute HTTP URL without credentials, query, or fragment")
	}
	if requireHTTPS && parsed.Scheme != "https" {
		return errors.New("must use HTTPS in production")
	}
	return nil
}

// validatePublicBaseURL ensures published URLs remain stable and credential-free.
func validatePublicBaseURL(value string, requireHTTPS bool) error {
	parsed, err := httpurl.ParseAbsolute(value)
	if err != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return errors.New("must be an absolute HTTP URL without credentials, query, or fragment")
	}
	if requireHTTPS && parsed.Scheme != "https" {
		return errors.New("must use HTTPS in production")
	}
	return nil
}
