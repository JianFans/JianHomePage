package httpapi

import (
	"errors"
	"net/http"

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
	status, code, message := domainErrorResponse(err)
	kind := code
	// Adapter failures are dependency failures, even when they wrap a sentinel
	// used for internal business rejections.
	if external {
		status, code, message = domainErrorResponse(nil)
		kind = "external_dependency"
	}
	attributes := []any{"stage", stage}
	if external {
		attributes = append(attributes, "provider_error_code", failure.ProviderCode(), "provider_http_status", failure.ProviderHTTPStatus())
	}
	logOperation(request, err, status, kind, "publish request failed", attributes...)
	writeError(writer, request, status, code, message)
}
