package auth

import (
	"errors"
	"log/slog"
	"net/http"

	"yujian.me/server/internal/observability"
)

// authenticationError carries a fixed diagnostic reason while keeping the
// public error safe and preserving the cause for structured diagnostics.
type authenticationError struct {
	reason  string
	message string
	cause   error
	level   slog.Level
	attrs   []slog.Attr
}

// Error never renders token claims, provider response bodies or storage errors.
func (err *authenticationError) Error() string { return err.message }

// Unwrap allows infrastructure classifiers to recover timeout and SQLSTATE data.
func (err *authenticationError) Unwrap() error { return err.cause }

// rejected records an expected token or browser-flow rejection.
func rejected(reason, message string) error {
	return &authenticationError{reason: reason, message: message, level: slog.LevelWarn}
}

// rejectedCause marks a caller-controlled authentication failure while
// preserving the domain error used by existing HTTP status handling.
func rejectedCause(reason, message string, cause error) error {
	return &authenticationError{reason: reason, message: message, cause: cause, level: slog.LevelWarn}
}

// rejectedWith attaches only application-owned, verified-configuration fields.
func rejectedWith(reason, message string, attrs ...slog.Attr) error {
	return &authenticationError{reason: reason, message: message, level: slog.LevelWarn, attrs: attrs}
}

// unavailable retains operational causes before the HTTP layer hides them.
func unavailable(reason, message string, cause error) error {
	return &authenticationError{reason: reason, message: message, cause: cause, level: slog.LevelError}
}

// authReason selects a fixed reason and severity rather than parsing error text.
func authReason(err error, fallback string, level slog.Level) (string, slog.Level) {
	var detail *authenticationError
	if errors.As(err, &detail) {
		return detail.reason, detail.level
	}
	return fallback, level
}

// logAuthentication records only verified subjects and application-owned
// stage/reason values. Callers must never pass token or browser flow material.
func logAuthentication(r *http.Request, level slog.Level, stage, reason, subject string, err error) {
	attributes := []any{"component", "auth", "stage", stage, "reason", reason}
	if !observability.HasLogger(r.Context()) {
		attributes = append(attributes, "request_id", r.Header.Get("X-Request-ID"))
	}
	if subject != "" {
		attributes = append(attributes, "actor_subject", subject)
	}
	var detail *authenticationError
	if errors.As(err, &detail) {
		for _, attr := range detail.attrs {
			attributes = append(attributes, attr.Key, attr.Value.Any())
		}
	}
	if err != nil {
		attributes = append(attributes, observability.Error(err))
	}
	observability.Logger(r.Context()).Log(r.Context(), level, "authentication event", attributes...)
}
