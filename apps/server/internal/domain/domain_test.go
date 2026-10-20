package domain

import (
	"strings"
	"testing"
)

// TestNormalizeIdempotencyKey verifies the byte contract shared by Go and the publishing gateway.
func TestNormalizeIdempotencyKey(t *testing.T) {
	tests := []struct {
		name, input, want string
		valid             bool
	}{
		{"ascii minimum", "12345678", "12345678", true},
		{"ascii maximum", strings.Repeat("x", 128), strings.Repeat("x", 128), true},
		{"utf8 minimum", strings.Repeat("\u00e9", 4), strings.Repeat("\u00e9", 4), true},
		{"utf8 maximum", strings.Repeat("\u00e9", 64), strings.Repeat("\u00e9", 64), true},
		{"unicode whitespace", "\u0085\u00a0 publish-key \u3000", "publish-key", true},
		{"bom is opaque", "\ufeffpublish-key", "\ufeffpublish-key", true},
		{"short", "1234567", "1234567", false},
		{"utf8 overflow", strings.Repeat("\u00e9", 65), strings.Repeat("\u00e9", 65), false},
		{"tab", "publish\tkey", "publish\tkey", false},
		{"newline", "publish\nkey", "publish\nkey", false},
		{"del", "publish\x7fkey", "publish\x7fkey", false},
		{"c1 control", "publish\u0085key", "publish\u0085key", false},
		{"invalid utf8", "publish\xffkey", "publish\xffkey", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, valid := NormalizeIdempotencyKey(tt.input)
			if got != tt.want || valid != tt.valid {
				t.Fatalf("normalized %q valid=%v, want %q valid=%v", got, valid, tt.want, tt.valid)
			}
		})
	}
}

func TestCanTransitionContentStatus(t *testing.T) {
	tests := []struct {
		from           ContentStatus
		to             ContentStatus
		reviewApproved bool
		allowed        bool
	}{
		{StatusDraft, StatusInReview, false, true},
		{StatusInReview, StatusDraft, false, true},
		{StatusInReview, StatusDraft, true, false},
		{StatusInReview, StatusPublished, false, false},
		{StatusInReview, StatusPublished, true, true},
		{StatusInReview, StatusPublishing, false, false},
		{StatusInReview, StatusPublishing, true, true},
		{StatusPublishing, StatusDraft, true, false},
		{StatusPublishing, StatusInReview, true, true},
		{StatusPublishing, StatusPublished, true, true},
		{StatusPublished, StatusArchived, true, true},
		{StatusArchived, StatusDraft, true, false},
		{StatusDraft, StatusPublished, true, false},
	}

	for _, test := range tests {
		if got := CanTransition(test.from, test.to, test.reviewApproved); got != test.allowed {
			t.Fatalf("transition %s -> %s approved=%v: got %v, want %v", test.from, test.to, test.reviewApproved, got, test.allowed)
		}
	}
}
