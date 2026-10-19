package publish

import "errors"

// Failure identifies the failed publish boundary without exposing repository or
// provider error text. Unwrap retains existing permission and concurrency checks.
type Failure struct {
	stage string
	cause error
}

// Error is safe for diagnostics even when the underlying error contains signed
// storage URLs or response bodies. Log Stage rather than the unwrapped error.
func (failure *Failure) Error() string { return "publish failed at " + failure.stage }

// Unwrap preserves the domain error classification for callers and reconcilers.
func (failure *Failure) Unwrap() error { return failure.cause }

// Stage returns a fixed application boundary, never a URL or user input.
func (failure *Failure) Stage() string { return failure.stage }

// External distinguishes infrastructure boundaries from local resource lookup.
// Missing snapshots during HEAD are handled before an error reaches this point.
func (failure *Failure) External() bool {
	switch failure.stage {
	case "snapshot.stat", "snapshot.put", "asset.stat", "asset.public_url", "build.trigger", "build.status":
		return true
	default:
		return false
	}
}

// ProviderCode exposes only recognized diagnostic codes. Unknown provider text
// is deliberately discarded because service responses can contain private data.
func (failure *Failure) ProviderCode() string {
	var coded interface{ ErrorCode() string }
	if errors.As(failure.cause, &coded) {
		switch code := coded.ErrorCode(); code {
		case "NoSuchBucket", "NoSuchKey", "NotFound", "AccessDenied", "Forbidden",
			"InvalidAccessKeyId", "SignatureDoesNotMatch", "ExpiredToken", "InvalidToken",
			"AuthorizationHeaderMalformed", "RequestTimeTooSkewed", "InvalidDigest", "BadDigest",
			"Conflict", "PreconditionFailed", "SlowDown", "ServiceUnavailable", "InternalError", "RequestTimeout":
			return code
		}
	}
	return "Unknown"
}

// ProviderHTTPStatus keeps an upstream HTTP failure distinguishable from the
// API response status without logging endpoints, request IDs, or response text.
func (failure *Failure) ProviderHTTPStatus() int {
	var status interface{ HTTPStatusCode() int }
	if errors.As(failure.cause, &status) {
		if code := status.HTTPStatusCode(); code >= 400 && code <= 599 {
			return code
		}
	}
	return 0
}

// failureAt retains the innermost failure so admission and retries report the
// specific storage or build step rather than its enclosing service operation.
func failureAt(stage string, err error) error {
	if err == nil {
		return nil
	}
	var existing *Failure
	if errors.As(err, &existing) {
		return err
	}
	return &Failure{stage: stage, cause: err}
}
