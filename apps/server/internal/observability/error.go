package observability

import (
	"errors"
	"fmt"
	"log/slog"
	"net"

	"github.com/aws/smithy-go"
	"github.com/jackc/pgx/v5/pgconn"
)

// Error describes bounded error causes while excluding provider bodies, SQL and row data.
func Error(err error) slog.Attr {
	if err == nil {
		return slog.Any("error", nil)
	}
	return slog.Attr{Key: "error", Value: slog.GroupValue(errorAttrs(err, 0)...)}
}

// ProviderCode retains known diagnostic codes while discarding arbitrary provider text.
func ProviderCode(err error) string {
	var safe interface{ SafeProviderCode() string }
	if errors.As(err, &safe) {
		return safe.SafeProviderCode()
	}
	var coded interface{ ErrorCode() string }
	if !errors.As(err, &coded) {
		return "Unknown"
	}
	switch code := coded.ErrorCode(); code {
	case "NoSuchBucket", "NoSuchKey", "NotFound", "AccessDenied", "Forbidden",
		"InvalidAccessKeyId", "SignatureDoesNotMatch", "ExpiredToken", "InvalidToken",
		"AuthorizationHeaderMalformed", "RequestTimeTooSkewed", "InvalidDigest", "BadDigest",
		"Conflict", "PreconditionFailed", "SlowDown", "ServiceUnavailable", "InternalError", "RequestTimeout":
		return code
	default:
		return "Unknown"
	}
}

// errorAttrs examines the current cause before traversing children, never formatting sensitive SDK errors.
func errorAttrs(err error, depth int) []slog.Attr {
	if depth >= 8 {
		return []slog.Attr{slog.String("message", "[cause limit reached]")}
	}
	attrs := []slog.Attr{slog.String("type", fmt.Sprintf("%T", err))}
	var api smithy.APIError
	var database *pgconn.PgError
	var safe interface{ SafeProviderCode() string }
	sensitive := errors.As(err, &api) || errors.As(err, &database) || errors.As(err, &safe)
	switch typed := err.(type) {
	case interface{ SafeProviderCode() string }:
		attrs = append(attrs, slog.String("message", "provider request failed"), slog.String("provider_code", typed.SafeProviderCode()))
	case smithy.APIError:
		attrs = append(attrs, slog.String("message", "provider request failed"), slog.String("provider_code", ProviderCode(typed)), slog.String("provider_fault", typed.ErrorFault().String()))
	case *pgconn.PgError:
		attrs = append(attrs, slog.String("message", "database operation failed"), slog.String("sqlstate", typed.Code), slog.String("severity", typed.Severity))
	default:
		message := "wrapped dependency failure"
		if !sensitive {
			message = newRedactor(nil).sanitize(err.Error())
		}
		attrs = append(attrs, slog.String("message", message))
	}
	if typed, ok := err.(net.Error); ok {
		attrs = append(attrs, slog.Bool("timeout", typed.Timeout()))
	}
	if typed, ok := err.(interface{ HTTPStatusCode() int }); ok {
		if status := typed.HTTPStatusCode(); status >= 100 && status <= 599 {
			attrs = append(attrs, slog.Int("http_status", status))
		}
	}
	if typed, ok := err.(interface{ ProviderAction() string }); ok {
		if action := typed.ProviderAction(); action != "" {
			attrs = append(attrs, slog.String("provider_action", action))
		}
	}
	if typed, ok := err.(interface{ ServiceRequestID() string }); ok {
		if requestID := typed.ServiceRequestID(); requestID != "" {
			attrs = append(attrs, slog.String("provider_request_id", requestID))
		}
	}
	if typed, ok := err.(interface{ RequestID() string }); ok {
		if requestID := typed.RequestID(); requestID != "" {
			attrs = append(attrs, slog.String("provider_request_id", requestID))
		}
	}
	var children []error
	if typed, ok := err.(interface{ Unwrap() []error }); ok {
		children = typed.Unwrap()
	} else if child := errors.Unwrap(err); child != nil {
		children = []error{child}
	}
	if len(children) > 0 {
		causes := make([]slog.Attr, 0, min(len(children), 8))
		for i, child := range children[:min(len(children), 8)] {
			if child != nil {
				causes = append(causes, slog.Attr{Key: fmt.Sprint(i), Value: slog.GroupValue(errorAttrs(child, depth+1)...)})
			}
		}
		attrs = append(attrs, slog.Attr{Key: "causes", Value: slog.GroupValue(causes...)})
	}
	return attrs
}
