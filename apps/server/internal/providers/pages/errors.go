package pages

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"regexp"
)

var safeRequestID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}={0,2}$`)

// preparationError proves CreatePagesDeployment was never attempted, even if a temporary artifact was uploaded.
type preparationError struct{ cause error }

// Error excludes provider response content, retaining diagnostics through Unwrap.
func (err *preparationError) Error() string { return "Pages deployment preparation failed" }

// Unwrap retains safe provider classifications and transport causes.
func (err *preparationError) Unwrap() error { return err.cause }

// DeploymentNotCreated permits ending a failed task without risking a duplicate deployment.
func (*preparationError) DeploymentNotCreated() bool { return true }

type cloudError struct {
	action, code, requestID string
	status                  int
	cause                   error
}

// Error omits provider response bodies, which may contain credentials or request data.
func (err *cloudError) Error() string { return "Pages cloud request failed" }

// Unwrap preserves cancellation and transport classification for callers.
func (err *cloudError) Unwrap() error { return err.cause }

// SafeProviderCode exposes only stable provider codes used for diagnostics.
func (err *cloudError) SafeProviderCode() string {
	switch err.code {
	case "AccessDenied", "AuthFailure", "AuthorizationHeaderMalformed", "BadDigest", "Conflict", "ExpiredToken",
		"FailedOperation", "Forbidden", "InternalError", "InvalidAccessKeyId", "InvalidAction", "InvalidDigest",
		"InvalidParameter", "InvalidParameterValue", "InvalidToken", "LimitExceeded", "NoSuchBucket", "NoSuchKey",
		"NotFound", "OperationDenied", "PreconditionFailed", "RequestLimitExceeded", "RequestTimeTooSkewed",
		"RequestTimeout", "ResourceNotFound", "ServiceUnavailable", "SignatureDoesNotMatch", "SlowDown",
		"UnauthorizedOperation", "UnsupportedOperation":
		return err.code
	default:
		return "Unknown"
	}
}

// HTTPStatusCode preserves the upstream status independently of the local API response.
func (err *cloudError) HTTPStatusCode() int { return err.status }

// RequestID exposes only bounded provider request identifiers.
func (err *cloudError) RequestID() string { return err.requestID }

// ProviderAction is constrained to operations issued by this adapter.
func (err *cloudError) ProviderAction() string {
	switch err.action {
	case "DescribePagesProjects", "DescribePagesCosTempToken", "CreatePagesDeployment", "DescribePagesDeployments", "COSUpload":
		return err.action
	default:
		return "unknown"
	}
}

// newCloudError retains diagnostic fields and the transport cause, never the
// provider response body, which may contain credentials or request data.
func newCloudError(action string, status int, body []byte, cause error) *cloudError {
	code, requestID := cloudErrorDetails(body)
	return &cloudError{action: action, code: code, requestID: requestID, status: status, cause: cause}
}

// cloudErrorDetails reads only code and request identity from bounded JSON or COS XML responses.
func cloudErrorDetails(body []byte) (string, string) {
	code, requestID := cloudJSONDetails(body, 0)
	if code != "" || requestID != "" {
		return code, requestID
	}
	var response struct {
		Code      string `xml:"Code"`
		RequestID string `xml:"RequestId"`
	}
	if xml.Unmarshal(body, &response) == nil {
		return response.Code, validRequestID(response.RequestID)
	}
	return "", ""
}

// cloudJSONDetails handles nested Pages response envelopes with bounded recursion,
// preferring nested error codes while retaining the outermost valid request ID.
func cloudJSONDetails(body []byte, depth int) (string, string) {
	if depth > 4 {
		return "", ""
	}
	var response struct {
		Code      json.RawMessage `json:"Code"`
		RequestID string          `json:"RequestId"`
		Error     json.RawMessage `json:"Error"`
		Response  json.RawMessage `json:"Response"`
		Data      struct {
			Response json.RawMessage `json:"Response"`
		} `json:"Data"`
	}
	if json.Unmarshal(body, &response) != nil {
		return "", ""
	}
	code := ""
	requestID := validRequestID(response.RequestID)
	for _, nested := range []json.RawMessage{response.Error, response.Response, response.Data.Response} {
		if len(nested) == 0 || bytes.Equal(bytes.TrimSpace(nested), []byte("null")) {
			continue
		}
		nestedCode, nestedRequestID := cloudJSONDetails(nested, depth+1)
		if code == "" {
			code = nestedCode
		}
		if requestID == "" {
			requestID = nestedRequestID
		}
	}
	if code == "" {
		code = rawCode(response.Code)
	}
	return code, requestID
}

// rawCode normalizes string and numeric provider codes without treating null as
// a failure code; callers still apply the diagnostic code allowlist.
func rawCode(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var value string
	if json.Unmarshal(raw, &value) == nil {
		return value
	}
	return string(raw)
}

// validRequestID rejects unsafe provider identities and enforces the total
// 128-byte limit, including any terminal base64 padding allowed by the pattern.
func validRequestID(value string) string {
	if len(value) <= 128 && safeRequestID.MatchString(value) {
		return value
	}
	return ""
}
