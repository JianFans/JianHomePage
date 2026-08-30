package httpurl

import "testing"

func TestParseAbsoluteRejectsUnsafeOrUnusableHTTPURLs(t *testing.T) {
	for _, value := range []string{
		"/relative",
		"mailto:admin@example.test",
		"https://:443/path",
		"https://user:pass@example.test/path",
		"https://example.test:0/path",
		"https://example.test:65536/path",
	} {
		t.Run(value, func(t *testing.T) {
			if _, err := ParseAbsolute(value); err == nil {
				t.Fatalf("expected invalid URL rejection for %q", value)
			}
		})
	}
}

func TestParseAbsoluteAcceptsHTTPHostsAndValidPorts(t *testing.T) {
	for _, value := range []string{
		"https://example.test/path",
		"http://127.0.0.1:8080/path",
		"https://[2001:db8::1]:443/path",
	} {
		t.Run(value, func(t *testing.T) {
			parsed, err := ParseAbsolute(value)
			if err != nil || parsed.String() != value {
				t.Fatalf("parse %q: parsed=%v err=%v", value, parsed, err)
			}
		})
	}
}
