package auth

import (
	"encoding/json"
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
}

type Middleware struct {
	allowDevIdentity bool
	identityProvider ports.IdentityProvider
}

func NewMiddleware(options MiddlewareOptions) (*Middleware, error) {
	if options.Environment == "production" && options.AllowDevIdentity {
		return nil, config.ErrUnsafeDevelopmentIdentity
	}
	return &Middleware{
		allowDevIdentity: options.AllowDevIdentity,
		identityProvider: options.IdentityProvider,
	}, nil
}

func (middleware *Middleware) Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		principal, err := middleware.authenticate(request)
		if err != nil {
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
			writeAuthError(writer, request, http.StatusUnauthorized, "unauthorized", "Authentication required.")
			return
		}
		if !HasPermission(principal, permission) {
			writeAuthError(writer, request, http.StatusForbidden, "forbidden", "Permission denied.")
			return
		}
		next.ServeHTTP(writer, request)
	})
}

func (middleware *Middleware) authenticate(request *http.Request) (domain.Principal, error) {
	if middleware.allowDevIdentity {
		if subject := strings.TrimSpace(request.Header.Get("X-Dev-Subject")); subject != "" {
			roles, err := parseRoleHeader(request.Header.Get("X-Dev-Roles"))
			if err != nil || len(roles) == 0 {
				return domain.Principal{}, domain.ErrForbidden
			}
			return domain.Principal{Subject: subject, Roles: roles}, nil
		}
	}

	token, ok := bearerToken(request.Header.Get("Authorization"))
	if !ok || middleware.identityProvider == nil {
		return domain.Principal{}, domain.ErrForbidden
	}
	principal, err := middleware.identityProvider.Authenticate(request.Context(), token)
	if err != nil || principal.Subject == "" || len(principal.Roles) == 0 {
		return domain.Principal{}, domain.ErrForbidden
	}
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
