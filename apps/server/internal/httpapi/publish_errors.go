package httpapi

import (
	"errors"
	"log/slog"
	"net/http"

	"yujian.me/server/internal/auth"
	"yujian.me/server/internal/domain"
	"yujian.me/server/internal/publish"
)

// writePublishError records only a fixed stage and error classification. Provider
// messages may contain credentials, signed URLs, or private snapshot contents.
func writePublishError(writer http.ResponseWriter, request *http.Request, err error) {
	stage := "service"
	external := false
	var failure *publish.Failure
	if errors.As(err, &failure) {
		stage = failure.Stage()
		external = failure.External()
	}
	kind := "internal_error"
	switch {
	case errors.Is(err, domain.ErrForbidden):
		kind = "forbidden"
	case errors.Is(err, domain.ErrConflict):
		kind = "conflict"
	case errors.Is(err, domain.ErrInvalidTransition):
		kind = "invalid_state"
	case errors.Is(err, domain.ErrInvalidInput):
		kind = "invalid_request"
	case external:
		kind = "external_dependency"
	case errors.Is(err, domain.ErrNotFound):
		kind = "not_found"
	}
	logger, ok := request.Context().Value(requestLoggerKey{}).(*slog.Logger)
	if !ok {
		logger = slog.Default()
	}
	actor, _ := auth.PrincipalFromContext(request.Context())
	attributes := []any{
		"request_id", request.Header.Get("X-Request-ID"),
		"actor_subject", actor.Subject,
		"method", request.Method,
		"route", request.Pattern,
		"stage", stage,
		"error_kind", kind,
	}
	if external {
		attributes = append(attributes, "provider_error_code", failure.ProviderCode(), "provider_http_status", failure.ProviderHTTPStatus())
	}
	logger.ErrorContext(request.Context(), "publish request failed", attributes...)
	// A storage/build adapter's missing object is not a missing site version or
	// job, even if it uses the same sentinel as the domain repository.
	if kind == "external_dependency" {
		writeError(writer, request, http.StatusInternalServerError, "internal_error", "Service unavailable.")
		return
	}
	writeDomainError(writer, request, err)
}
