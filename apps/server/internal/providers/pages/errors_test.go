package pages

import (
	"net/http"
	"strings"
	"testing"
)

// TestCOSRequestIDsAllowOnlyTerminalPadding preserves safe XML request identities for provider support.
func TestCOSRequestIDsAllowOnlyTerminalPadding(t *testing.T) {
	for _, tc := range []struct {
		id   string
		want string
	}{
		{"a", "a"},
		{"cos_req-123", "cos_req-123"},
		{"cos_req-123=", "cos_req-123="},
		{"cos_req-123==", "cos_req-123=="},
		{strings.Repeat("a", 126) + "==", strings.Repeat("a", 126) + "=="},
		{strings.Repeat("a", 127) + "==", ""},
		{strings.Repeat("a", 129), ""},
		{"", ""},
		{"=", ""},
		{"==", ""},
		{"cos===", ""},
		{"cos=req", ""},
		{"cos+req/", ""},
		{"cos req", ""},
	} {
		t.Run(tc.id, func(t *testing.T) {
			body := []byte("<Error><Code>AccessDenied</Code><RequestId>" + tc.id + "</RequestId></Error>")
			err := newCloudError("COSUpload", http.StatusForbidden, body, nil)
			if err.RequestID() != tc.want || err.SafeProviderCode() != "AccessDenied" || err.ProviderAction() != "COSUpload" || err.HTTPStatusCode() != http.StatusForbidden {
				t.Fatalf("unexpected COS diagnostic: id=%q code=%q action=%q status=%d", err.RequestID(), err.SafeProviderCode(), err.ProviderAction(), err.HTTPStatusCode())
			}
		})
	}
}
