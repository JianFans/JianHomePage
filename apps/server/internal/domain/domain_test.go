package domain

import "testing"

func TestCanTransitionContentStatus(t *testing.T) {
	tests := []struct {
		from           ContentStatus
		to             ContentStatus
		reviewApproved bool
		allowed        bool
	}{
		{StatusDraft, StatusInReview, false, true},
		{StatusInReview, StatusDraft, false, true},
		{StatusInReview, StatusPublished, false, false},
		{StatusInReview, StatusPublished, true, true},
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
