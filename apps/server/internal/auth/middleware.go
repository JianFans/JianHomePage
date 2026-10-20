package auth

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"yujian.me/server/internal/config"
	"yujian.me/server/internal/domain"
	"yujian.me/server/internal/ports"
)

type MiddlewareOptions struct {
	Environment      string
	AllowDevIdentity bool
	IdentityProvider ports.IdentityProvider
	Sessions         *Sessions
}

type Middleware struct {
	allowDevIdentity bool
	identityProvider ports.IdentityProvider
	sessions         *Sessions
}

func NewMiddleware(options MiddlewareOptions) (*Middleware, error) {
	if options.Environment == "production" && options.AllowDevIdentity {
		return nil, config.ErrUnsafeDevelopmentIdentity
	}
	return &Middleware{
		allowDevIdentity: options.AllowDevIdentity,
		identityProvider: options.IdentityProvider,
		sessions:         options.Sessions,
	}, nil
}

func (middleware *Middleware) Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		principal, err := middleware.authenticate(request)
		if err != nil {
			if errors.Is(err, errCSRF) {
				writeAuthError(writer, request, http.StatusForbidden, "forbidden", "Request verification failed.")
				return
			}
			if !errors.Is(err, domain.ErrForbidden) && !errors.Is(err, domain.ErrNotFound) {
				writeAuthError(writer, request, http.StatusServiceUnavailable, "unavailable", "Session service unavailable.")
				return
			}
			writeAuthError(writer, request, http.StatusUnauthorized, "unauthorized", "Authentication required.")
			return
		}
		// Update the request in place so outer observability middleware can read
		// the authenticated subject after the downstream handler returns.
		*request = *request.WithContext(withPrincipal(request.Context(), principal))
		next.ServeHTTP(writer, request)
	})
}

func (middleware *Middleware) Require(permission Permission, next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		principal, ok := PrincipalFromContext(request.Context())
		if !ok {
			logAuthentication(request, slog.LevelWarn, "rbac", "identity_missing", "", nil)
			writeAuthError(writer, request, http.StatusUnauthorized, "unauthorized", "Authentication required.")
			return
		}
		if !HasPermission(principal, permission) {
			logAuthentication(request, slog.LevelWarn, "rbac", "permission_denied", principal.Subject, nil)
			writeAuthError(writer, request, http.StatusForbidden, "forbidden", "Permission denied.")
			return
		}
		next.ServeHTTP(writer, request)
	})
}

func (middleware *Middleware) authenticate(request *http.Request) (domain.Principal, error) {
	if middleware.sessions != nil && request.Header.Get("Authorization") == "" {
		if _, err := request.Cookie(sessionCookieName); err == nil {
			principal, authErr := middleware.sessions.authenticateCookie(request)
			if authErr != nil {
				reason, level := "session_rejected", slog.LevelWarn
				if !errors.Is(authErr, domain.ErrForbidden) && !errors.Is(authErr, domain.ErrNotFound) && !errors.Is(authErr, errCSRF) {
					reason, level = "store_unavailable", slog.LevelError
				}
				reason, level = authReason(authErr, reason, level)
				logAuthentication(request, level, "cookie.verify", reason, "", authErr)
			} else {
				logAuthentication(request, slog.LevelDebug, "cookie.verify", "authenticated", principal.Subject, nil)
			}
			return principal, authErr
		}
	}
	if middleware.allowDevIdentity {
		if subject := strings.TrimSpace(request.Header.Get("X-Dev-Subject")); subject != "" {
			roles, err := parseRoleHeader(request.Header.Get("X-Dev-Roles"))
			if err != nil || len(roles) == 0 {
				logAuthentication(request, slog.LevelWarn, "development.verify", "roles_invalid", "", nil)
				return domain.Principal{}, domain.ErrForbidden
			}
			return domain.Principal{Subject: subject, Roles: roles}, nil
		}
	}

	token, ok := bearerToken(request.Header.Get("Authorization"))
	if !ok || middleware.identityProvider == nil {
		reason := "bearer_missing"
		if ok {
			reason = "provider_not_configured"
		}
		logAuthentication(request, slog.LevelWarn, "bearer.verify", reason, "", nil)
		return domain.Principal{}, domain.ErrForbidden
	}
	principal, err := middleware.identityProvider.Authenticate(request.Context(), token)
	if err != nil || principal.Subject == "" || len(principal.Roles) == 0 {
		reason, level := authReason(err, "identity_rejected", slog.LevelWarn)
		logAuthentication(request, level, "bearer.verify", reason, "", err)
		return domain.Principal{}, domain.ErrForbidden
	}
	logAuthentication(request, slog.LevelDebug, "bearer.verify", "authenticated", principal.Subject, nil)
	return principal, nil
}

func bearerToken(header string) (string, bool) {
	parts := strings.Fields(header)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || parts[1] == "" {
		return "", false
	}
	return parts[1], true
}

func parseRoleHeader(header string) ([]domain.Role, error) {
	if strings.TrimSpace(header) == "" {
		return nil, domain.ErrInvalidInput
	}
	values := strings.Split(header, ",")
	for index := range values {
		values[index] = strings.TrimSpace(values[index])
	}
	return ParseRoles(values)
}

func writeAuthError(
	writer http.ResponseWriter,
	request *http.Request,
	status int,
	code string,
	message string,
) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(map[string]string{
		"code":      code,
		"message":   message,
		"requestId": request.Header.Get("X-Request-ID"),
	})
}
