package observability

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// TestAuthenticationTextRedaction removes complete credentials and header values while retaining following lines.
func TestAuthenticationTextRedaction(t *testing.T) {
	for _, input := range []string{
		"Authorization: Basic dXNlcjpwYXNz",
		"Proxy-Authorization: Basic dXNlcjpwYXNz",
		"Basic dXNlcjpwYXNz",
		"request failed: Authorization: Basic dXNlcjpwYXNz",
		"Cookie: session=private-session; theme=private-theme",
		"Set-Cookie: session=private-session; Path=/; Secure; another=private-theme",
		"authorization: Custom private-session private-theme",
	} {
		t.Run(input, func(t *testing.T) {
			got := newRedactor(nil).text(input + "\r\nX-Request-ID: public-id")
			for _, secret := range []string{"dXNlcjpwYXNz", "private-session", "private-theme"} {
				if strings.Contains(got, secret) {
					t.Fatalf("credential remains in sanitized text: %q", got)
				}
			}
			if !strings.Contains(got, "[REDACTED]") || !strings.Contains(got, "X-Request-ID: public-id") {
				t.Fatalf("redaction removed unrelated lines: %q", got)
			}
		})
	}
}

// TestCodeRedactionPreservesStatusDescriptions distinguishes diagnostics from OAuth authorization codes.
func TestCodeRedactionPreservesStatusDescriptions(t *testing.T) {
	redactor := newRedactor(nil)
	for _, tc := range []struct{ name, input, want string }{
		{"status", "status code: 503", "status code: 503"},
		{"numeric JSON", `{"code":503}`, `{"code":503}`},
		{"query", "code=private-code", "code=[REDACTED]"},
		{"quoted query", `code="private-code"`, "code=[REDACTED]"},
		{"single quoted query", "code='private-code'", "code=[REDACTED]"},
		{"relative URL", "/callback?code=private-code&state=public-state", "/callback?code=[REDACTED]&state=public-state"},
		{"JSON", `{"code":"private-code","status":503}`, `{"code":"[REDACTED]","status":503}`},
		{"escaped JSON", `{"code":"private-\"code-tail"}`, `{"code":"[REDACTED]"}`},
		{"query inside JSON", `{"code":"code=\"private-code\""}`, `{"code":"[REDACTED]"}`},
		{"credential inside JSON", `{"code":"token=\"private-code\""}`, `{"code":"[REDACTED]"}`},
		{"unrelated key", "decode=public-value", "decode=public-value"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := redactor.text(tc.input); got != tc.want {
				t.Fatalf("sanitized text = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestRedactorTextNormalizesUTF8BeforeTruncation preserves valid content after malformed input and avoids splitting runes.
func TestRedactorTextNormalizesUTF8BeforeTruncation(t *testing.T) {
	marker := "…[truncated]"
	redactor := newRedactor(nil)
	for _, tc := range []struct{ name, input, want string }{
		{"empty", "", ""},
		{"malformed short", "\xff\xfeok", "\uFFFDok"},
		{"malformed long", "\xff" + strings.Repeat("a", maxString+20), "\uFFFD" + strings.Repeat("a", maxString-3) + marker},
		{"normalization crosses limit", strings.Repeat("a", maxString-2) + "\xff", strings.Repeat("a", maxString-2) + marker},
		{"exact rune boundary", strings.Repeat("a", maxString-3) + "界tail", strings.Repeat("a", maxString-3) + "界" + marker},
		{"inside rune", strings.Repeat("a", maxString-1) + "界tail", strings.Repeat("a", maxString-1) + marker},
		{"four byte rune", strings.Repeat("a", maxString-2) + "🎵tail", strings.Repeat("a", maxString-2) + marker},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := redactor.text(tc.input)
			if got != tc.want || !utf8.ValidString(got) || len(got) > maxString+len(marker) {
				t.Fatalf("normalized text = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestRecordBudgetTruncatesNormalizedUTF8 keeps byte accounting and suffix behavior for exhausted and partial-rune budgets.
func TestRecordBudgetTruncatesNormalizedUTF8(t *testing.T) {
	marker := "…[truncated]"
	redactor := newRedactor(nil)
	for _, tc := range []struct {
		name, input, want string
		bytes, remaining  int
	}{
		{"empty", "", "", 8, 8},
		{"exhausted", "界abc", "", 0, 0},
		{"inside rune without suffix", "界abc", "", 2, 0},
		{"exact rune without suffix", "界abc", "界", 3, 0},
		{"exact fit", "界a", "界a", 4, 0},
		{"malformed short", "\xffok", "\uFFFDok", 8, 3},
		{"malformed long", "\xff" + strings.Repeat("a", 30), "\uFFFDaaa" + marker, len(marker) + 6, 0},
		{"suffix only", strings.Repeat("界", 10), marker, len(marker), 0},
		{"inside rune with suffix", strings.Repeat("界", 10), marker, len(marker) + 2, 0},
		{"exact rune with suffix", strings.Repeat("界", 10), "界" + marker, len(marker) + 3, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			budget := &recordBudget{bytes: tc.bytes}
			got := budget.take(redactor, tc.input)
			if got != tc.want || !utf8.ValidString(got) || len(got) > tc.bytes || budget.bytes != tc.remaining {
				t.Fatalf("budget result = %q, remaining = %d; want %q, %d", got, budget.bytes, tc.want, tc.remaining)
			}
		})
	}
}
