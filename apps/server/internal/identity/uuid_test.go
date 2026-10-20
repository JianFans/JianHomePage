package identity

import (
	"regexp"
	"testing"
)

func TestNewReturnsPrefixedUUIDv4(t *testing.T) {
	got, err := New("req_")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if !regexp.MustCompile(`^req_[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`).MatchString(got) {
		t.Fatalf("New returned non-UUIDv4 identifier %q", got)
	}
	if !Valid(got, "req_") {
		t.Fatalf("Valid rejected generated identifier %q", got)
	}
}

func TestValidRequiresPrefixAndUUIDv4Bits(t *testing.T) {
	valid := "asset_550e8400-e29b-41d4-a716-446655440000"
	for _, value := range []string{
		valid,
		"550e8400-e29b-41d4-a716-446655440000",
		"release_550e8400-e29b-41d4-a716-446655440000",
		"asset_550e8400-e29b-11d4-a716-446655440000",
		"asset_550e8400-e29b-41d4-7716-446655440000",
		"asset_550e8400_e29b_41d4_a716_446655440000",
		"asset_550E8400-E29B-41D4-A716-446655440000",
	} {
		if got := Valid(value, "asset_"); got != (value == valid) {
			t.Errorf("Valid(%q) = %v", value, got)
		}
	}
}
