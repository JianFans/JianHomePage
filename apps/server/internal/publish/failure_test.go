package publish

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"yujian.me/server/internal/domain"
)

// TestPublishReportsJobPersistenceFailureAtCorrectStage covers both accepted
// and uncertain build submissions whose subsequent task update cannot persist.
func TestPublishReportsJobPersistenceFailureAtCorrectStage(t *testing.T) {
	for _, triggerErr := range []error{nil, errors.New("build submission timed out")} {
		repository := newMemoryRepository()
		repository.versions["ver_85ffbe4c-6a0a-40c9-93a7-4acb67a8c526"] = approvedVersion("ver_85ffbe4c-6a0a-40c9-93a7-4acb67a8c526", "rel_f91d3d69-e774-4cf1-a493-744009d9aa0d")
		updateErr := errors.New("job persistence unavailable")
		repository.updateErr = updateErr
		repository.updateFailures = 1
		trigger := &buildTriggerFake{triggerErr: triggerErr}
		service := publishServiceForTest(repository, newBlobStoreFake(), trigger)
		_, err := service.Publish(context.Background(), publisher(), "ver_85ffbe4c-6a0a-40c9-93a7-4acb67a8c526", "publish-persistence-failure")
		var failure *Failure
		if !errors.As(err, &failure) || failure.Stage() != "job.update" || !errors.Is(err, updateErr) {
			t.Fatalf("incorrect job-update diagnostics: failure=%v stage=%v", err, failure)
		}
	}
}

// TestFailureRetainsDomainClassificationsWithoutExposingCauses protects RBAC,
// optimistic locking, and missing-resource semantics across diagnostic wrapping.
func TestFailureRetainsDomainClassificationsWithoutExposingCauses(t *testing.T) {
	for _, sentinel := range []error{domain.ErrForbidden, domain.ErrNotFound, domain.ErrConflict, domain.ErrInvalidInput, domain.ErrInvalidTransition} {
		err := failureAt("version.lookup", fmt.Errorf("private-provider-detail: %w", sentinel))
		if !errors.Is(err, sentinel) {
			t.Fatalf("lost original classification %v", sentinel)
		}
		var failure *Failure
		if !errors.As(err, &failure) || failure.Stage() != "version.lookup" || failure.External() {
			t.Fatalf("unexpected failure stage: %v", err)
		}
		if strings.Contains(err.Error(), "private-provider-detail") {
			t.Fatal("failure text exposed its underlying cause")
		}
	}
	if failureAt("unused", nil) != nil {
		t.Fatal("wrapping a successful operation produced a failure")
	}
}

// TestFailureKeepsInnermostExternalStage ensures outer admission and retry
// boundaries preserve the specific failed dependency rather than hiding it.
func TestFailureKeepsInnermostExternalStage(t *testing.T) {
	inner := failureAt("snapshot.put", domain.ErrNotFound)
	err := failureAt("job.create", inner)
	var failure *Failure
	if err != inner || !errors.As(err, &failure) || failure.Stage() != "snapshot.put" || !failure.External() {
		t.Fatalf("lost inner storage failure: %v", err)
	}
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatal("diagnostic wrapping changed the dependency sentinel")
	}
}

type diagnosticProviderError struct {
	code   string
	status int
}

// Error includes a sentinel that must never become a diagnostic field.
func (diagnosticProviderError) Error() string { return "private-provider-detail" }

// ErrorCode emulates SDK errors without importing provider types into the service.
func (err diagnosticProviderError) ErrorCode() string { return err.code }

// HTTPStatusCode emulates the transport metadata attached to SDK failures.
func (err diagnosticProviderError) HTTPStatusCode() int { return err.status }

// TestFailureDiagnosticsDiscardUntrustedProviderFields checks both the static
// provider-code allowlist and the allowed HTTP failure-status range.
func TestFailureDiagnosticsDiscardUntrustedProviderFields(t *testing.T) {
	for _, test := range []struct {
		code, expectedCode     string
		status, expectedStatus int
	}{
		{"NoSuchBucket", "NoSuchBucket", 404, 404},
		{"AccessDenied", "AccessDenied", 403, 403},
		{"provider-private-detail", "Unknown", 500, 500},
		{"SignatureDoesNotMatch", "SignatureDoesNotMatch", 200, 0},
		{"", "Unknown", 1000, 0},
	} {
		var failure *Failure
		err := failureAt("snapshot.put", diagnosticProviderError{code: test.code, status: test.status})
		if !errors.As(err, &failure) || failure.ProviderCode() != test.expectedCode || failure.ProviderHTTPStatus() != test.expectedStatus {
			t.Fatalf("unsafe or inaccurate provider diagnostics for %#v", test)
		}
	}
	var failure *Failure
	if !errors.As(failureAt("snapshot.stat", errors.New("network failure")), &failure) || failure.ProviderCode() != "Unknown" || failure.ProviderHTTPStatus() != 0 {
		t.Fatal("network failure produced provider metadata")
	}
}
