package publish

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"yujian.me/server/internal/domain"
	"yujian.me/server/internal/observability"
	"yujian.me/server/internal/ports"
)

// publishLogContext captures structured diagnostics at every severity.
func publishLogContext(t *testing.T) (context.Context, *bytes.Buffer) {
	t.Helper()
	var output bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&output, &slog.HandlerOptions{Level: slog.LevelDebug}))
	return observability.WithLogger(t.Context(), logger), &output
}

// TestPublishLogsHandoffFailureWithoutConfusingItForCloudDeployment guards the
// boundary between durable task admission and the worker's later cloud request.
func TestPublishLogsHandoffFailureWithoutConfusingItForCloudDeployment(t *testing.T) {
	repository := newMemoryRepository()
	repository.versions["ver_new"] = approvedVersion("ver_new", "rel_new")
	trigger := &buildTriggerFake{triggerErr: errors.New("Authorization: Bearer private-token"), statuses: make(map[string]ports.BuildRun)}
	service := publishServiceForTest(repository, newBlobStoreFake(), trigger)
	ctx, output := publishLogContext(t)
	_, err := service.Publish(ctx, publisher(), "ver_new", "publish-handoff-failure")
	if err == nil {
		t.Fatal("expected task handoff failure")
	}
	event := requirePublishEvent(t, output, "publisher task handoff failed; inspect publisher logs before retry", "ERROR")
	if event["stage"] != "build.trigger" || event["error"] == nil || strings.Contains(output.String(), "private-token") {
		t.Fatalf("unsafe or incomplete handoff diagnostic: %#v output=%s", event, output.String())
	}
}

// TestExternalNotFoundIsLoggedAsDependencyFailure keeps provider and domain failures at distinct severities.
func TestExternalNotFoundIsLoggedAsDependencyFailure(t *testing.T) {
	ctx, output := publishLogContext(t)
	job := domain.PublishJob{ID: "pub_1", VersionID: "ver_new", Status: domain.PublishFailed}
	lifecycleError(ctx, publisher(), job, failureAt("asset.stat", domain.ErrNotFound))
	event := requirePublishEvent(t, output, "publish operation failed", "ERROR")
	if event["error_kind"] != "dependency" || event["stage"] != "asset.stat" {
		t.Fatalf("external not-found was misclassified: %#v", event)
	}
}

// TestStoredSnapshotIntegrityFailureKeepsActionablePublishDiagnostics verifies
// corrupted persisted content is logged as an internal integrity failure.
func TestStoredSnapshotIntegrityFailureKeepsActionablePublishDiagnostics(t *testing.T) {
	ctx, output := publishLogContext(t)
	job := domain.PublishJob{ID: "pub_1", VersionID: "ver_1", Status: domain.PublishFailed}
	err := failureAt("version.lookup", fmt.Errorf("stored snapshot checksum mismatch: %w", domain.ErrDataIntegrity))

	lifecycleError(ctx, publisher(), job, err)
	event := requirePublishEvent(t, output, "publish operation failed", "ERROR")
	if event["stage"] != "version.lookup" || event["error_kind"] != "data_integrity" || event["error"] == nil ||
		!strings.Contains(output.String(), "stored snapshot checksum mismatch") {
		t.Fatalf("stored snapshot integrity diagnostic is missing or misclassified: %#v output=%s", event, output.String())
	}
}

// requirePublishEvent verifies stable correlation fields without tying tests to
// text formatting or slog attribute order.
func requirePublishEvent(t *testing.T, output *bytes.Buffer, message, level string) map[string]any {
	t.Helper()
	for _, line := range strings.Split(strings.TrimSpace(output.String()), "\n") {
		var entry map[string]any
		if json.Unmarshal([]byte(line), &entry) == nil && entry["msg"] == message {
			if entry["level"] != level || entry["component"] != "publish" || entry["stage"] == "" || entry["version_id"] == "" {
				t.Fatalf("incomplete publish lifecycle event: %#v", entry)
			}
			return entry
		}
	}
	t.Fatalf("missing publish event %s/%s: %s", message, level, output.String())
	return nil
}

// TestPublishLifecycleLogsAdmissionReuseAndCompletion links the request and
// later status transition through the same immutable publish and build IDs.
func TestPublishLifecycleLogsAdmissionReuseAndCompletion(t *testing.T) {
	repository := newMemoryRepository()
	repository.versions["ver_new"] = approvedVersion("ver_new", "rel_new")
	trigger := &buildTriggerFake{statuses: make(map[string]ports.BuildRun)}
	service := publishServiceForTest(repository, newBlobStoreFake(), trigger)
	ctx, output := publishLogContext(t)

	job, err := service.Publish(ctx, publisher(), "ver_new", "publish-lifecycle")
	if err != nil || job.Status != domain.PublishBuilding {
		t.Fatalf("publish admission: %#v %v", job, err)
	}
	entry := requirePublishEvent(t, output, "publish job admitted", "INFO")
	if entry["job_id"] != job.ID || entry["stage"] != "job.create" {
		t.Fatalf("admission lost job correlation: %#v", entry)
	}
	entry = requirePublishEvent(t, output, "build submission accepted", "INFO")
	if entry["job_id"] != job.ID || entry["build_id"] != job.BuildID || entry["stage"] != "build.trigger" {
		t.Fatalf("build submission lost job/build correlation: %#v", entry)
	}

	if _, err := service.Publish(ctx, publisher(), "ver_new", "publish-lifecycle"); err != nil {
		t.Fatal(err)
	}
	entry = requirePublishEvent(t, output, "existing publish job reused", "INFO")
	if entry["job_id"] != job.ID {
		t.Fatalf("idempotency reuse changed job identity: %#v", entry)
	}

	trigger.statuses[job.BuildID] = ports.BuildRun{ID: job.BuildID, Status: domain.PublishBuilding}
	if _, err := service.RefreshStatus(ctx, publisher(), job.ID); err != nil {
		t.Fatal(err)
	}
	entry = requirePublishEvent(t, output, "publish build remains in progress", "DEBUG")
	if entry["job_id"] != job.ID || entry["build_status"] != string(domain.PublishBuilding) {
		t.Fatalf("unchanged polling event lost build state: %#v", entry)
	}
	trigger.statuses[job.BuildID] = ports.BuildRun{ID: job.BuildID, Status: domain.PublishSucceeded}
	if _, err := service.RefreshStatus(ctx, publisher(), job.ID); err != nil {
		t.Fatal(err)
	}
	entry = requirePublishEvent(t, output, "publish completed", "INFO")
	if entry["job_id"] != job.ID || entry["build_id"] != job.BuildID || entry["version_id"] != job.VersionID {
		t.Fatalf("completion lost lifecycle identity: %#v", entry)
	}
}

// TestPublishFailureLogsStageAndCauseWithoutChangingHTTPClassification records
// missing local content as a domain 404 rather than an external failure.
func TestPublishFailureLogsStageAndCauseWithoutChangingHTTPClassification(t *testing.T) {
	repository := newMemoryRepository()
	service := publishServiceForTest(repository, newBlobStoreFake(), &buildTriggerFake{statuses: make(map[string]ports.BuildRun)})
	ctx, output := publishLogContext(t)
	_, err := service.Publish(ctx, publisher(), "ver_missing", "publish-missing-version")
	if err == nil || !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("missing version classification changed: %v", err)
	}
	entry := requirePublishEvent(t, output, "publish operation failed", "WARN")
	if entry["stage"] != "version.lookup" || entry["error"] != nil || entry["error_kind"] != "domain_rejection" || entry["job_id"] != "" || entry["version_id"] != "ver_missing" {
		t.Fatalf("missing version diagnostic is inaccurate: %#v", entry)
	}
}

// TestPublishSnapshotValidationDoesNotLogSnapshotDetails keeps caller content out of publish diagnostics.
func TestPublishSnapshotValidationDoesNotLogSnapshotDetails(t *testing.T) {
	repository := newMemoryRepository()
	repository.versions["ver_private"] = approvedVersion("ver_private", "rel_private")
	service := publishServiceForTest(repository, newBlobStoreFake(), &buildTriggerFake{statuses: make(map[string]ports.BuildRun)})
	service.validator = validatorStub{err: errors.New("unknown field private-snapshot-title")}
	ctx, output := publishLogContext(t)
	_, err := service.Publish(ctx, publisher(), "ver_private", "publish-private")
	if err == nil || !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("snapshot validation error classification changed: %v", err)
	}
	entry := requirePublishEvent(t, output, "publish operation failed", "WARN")
	if entry["stage"] != "snapshot.validate" || entry["error"] != nil || strings.Contains(output.String(), "private-snapshot-title") {
		t.Fatalf("unsafe or incomplete snapshot validation diagnostic: %#v output=%s", entry, output.String())
	}
}

// TestAdmissionFailureLogsDoNotChangeReturnedJobs preserves empty error results while retaining diagnostic context.
func TestAdmissionFailureLogsDoNotChangeReturnedJobs(t *testing.T) {
	for _, operation := range []domain.PublishOperation{domain.PublishOperationPublish, domain.PublishOperationRollback} {
		for _, scenario := range []struct {
			name, key string
			actor     domain.Principal
			wantErr   error
		}{
			{"forbidden", "admission-forbidden", domain.Principal{Subject: "editor-1", Roles: []domain.Role{domain.RoleEditor}}, domain.ErrForbidden},
			{"invalid key", "bad", publisher(), domain.ErrInvalidInput},
			{"missing version", "admission-missing", publisher(), domain.ErrNotFound},
		} {
			t.Run(string(operation)+"/"+scenario.name, func(t *testing.T) {
				service := publishServiceForTest(newMemoryRepository(), newBlobStoreFake(), &buildTriggerFake{statuses: make(map[string]ports.BuildRun)})
				admit := service.Publish
				if operation == domain.PublishOperationRollback {
					admit = service.Rollback
				}
				ctx, output := publishLogContext(t)
				job, err := admit(ctx, scenario.actor, "ver_missing", scenario.key)
				if !errors.Is(err, scenario.wantErr) {
					t.Fatalf("failure classification changed: %v", err)
				}
				if job != (domain.PublishJob{}) {
					t.Errorf("logging populated an empty error result: %#v", job)
				}
				entry := requirePublishEvent(t, output, "publish operation failed", "WARN")
				if entry["version_id"] != "ver_missing" || entry["actor_subject"] != scenario.actor.Subject || entry["operation"] != string(operation) {
					t.Fatalf("failure log lost admission context: %#v", entry)
				}
			})
		}
	}
}

// TestPublishLogsDoNotEchoInvalidRequestedVersionIDs prevents caller paths from becoming log fields.
func TestPublishLogsDoNotEchoInvalidRequestedVersionIDs(t *testing.T) {
	service := publishServiceForTest(newMemoryRepository(), newBlobStoreFake(), &buildTriggerFake{statuses: make(map[string]ports.BuildRun)})
	ctx, output := publishLogContext(t)
	_, _ = service.Publish(ctx, publisher(), "private-token-value", "publish-invalid-id")
	if strings.Contains(output.String(), "private-token-value") {
		t.Fatalf("invalid caller-controlled version ID leaked into publish logs: %s", output.String())
	}
}

// TestRefreshFailureLogsRetainJobIdentity covers lookup fallback and loaded jobs lost by error returns.
func TestRefreshFailureLogsRetainJobIdentity(t *testing.T) {
	for _, scenario := range []string{"missing", "invalid-id", "build-status", "fail-persistence", "finalize"} {
		t.Run(scenario, func(t *testing.T) {
			repository := newMemoryRepository()
			trigger := &buildTriggerFake{statuses: make(map[string]ports.BuildRun)}
			service := publishServiceForTest(repository, newBlobStoreFake(), trigger)
			job := domain.PublishJob{ID: "pub_missing"}
			stage := "job.lookup"
			if scenario == "invalid-id" {
				job.ID = "private-token-value"
			} else if scenario != "missing" {
				repository.versions["ver_new"] = approvedVersion("ver_new", "rel_new")
				var err error
				job, err = service.Publish(t.Context(), publisher(), "ver_new", "refresh-log-identity")
				if err != nil {
					t.Fatal(err)
				}
				switch scenario {
				case "build-status":
					stage = "build.status"
					// No provider status exists, so the fake returns a lookup error.
				case "fail-persistence":
					stage = "job.fail"
					trigger.statuses[job.BuildID] = ports.BuildRun{Status: domain.PublishFailed, Error: "build failed"}
					repository.updateFailures = 1
					repository.updateErr = errors.New("database unavailable")
				case "finalize":
					stage = "job.finalize"
					trigger.statuses[job.BuildID] = ports.BuildRun{Status: domain.PublishSucceeded}
					delete(repository.versions, job.VersionID)
				}
			}
			ctx, output := publishLogContext(t)
			result, err := service.RefreshStatus(ctx, publisher(), job.ID)
			if err == nil || (scenario != "build-status" && result.ID != "") {
				t.Fatalf("unexpected refresh result: %#v, %v", result, err)
			}
			var event map[string]any
			for _, line := range strings.Split(strings.TrimSpace(output.String()), "\n") {
				var entry map[string]any
				if json.Unmarshal([]byte(line), &entry) == nil && entry["msg"] == "publish operation failed" {
					event = entry
				}
			}
			wantID := job.ID
			if scenario == "invalid-id" {
				wantID = ""
			}
			if event == nil || event["job_id"] != wantID || event["version_id"] != job.VersionID || event["build_id"] != job.BuildID || event["stage"] != stage || strings.Contains(output.String(), "private-token-value") {
				t.Fatalf("refresh failure lost safe job identity: %s", output.String())
			}
		})
	}
}

// TestUnclassifiedPublishFailuresDoNotReuseHTTPEventNames keeps service and request diagnostics distinguishable.
func TestUnclassifiedPublishFailuresDoNotReuseHTTPEventNames(t *testing.T) {
	ctx, output := publishLogContext(t)
	job := domain.PublishJob{VersionID: "ver_new"}
	lifecycleError(ctx, publisher(), job, errors.New("unexpected internal failure"))
	entry := requirePublishEvent(t, output, "publish operation failed", "ERROR")
	if entry["version_id"] != job.VersionID {
		t.Fatalf("unclassified failure lost version identity: %#v", entry)
	}
}
