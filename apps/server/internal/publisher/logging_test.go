package publisher

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"strings"
	"testing"

	"github.com/aws/smithy-go"
	"yujian.me/server/internal/domain"
	"yujian.me/server/internal/observability"
	"yujian.me/server/internal/providers/pages"
)

// publisherLogContext captures every level without altering the process logger.
func publisherLogContext(t *testing.T) (context.Context, *bytes.Buffer) {
	t.Helper()
	var output bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&output, &slog.HandlerOptions{Level: slog.LevelDebug}))
	return observability.WithLogger(t.Context(), logger), &output
}

// TestPublisherFailureLogsRetainRequestedBuildID keeps safe lookup identities in diagnostics without changing error results.
func TestPublisherFailureLogsRetainRequestedBuildID(t *testing.T) {
	for _, operation := range []string{"inspect", "recover", "invalid-recovery"} {
		for _, identity := range []struct {
			name, value string
			safe        bool
		}{
			{"valid", "build_missing", true},
			{"single", "a", true},
			{"maximum", strings.Repeat("a", 256), true},
			{"unsafe", "build/private-token", false},
			{"invalid start", "_build", false},
			{"oversized", strings.Repeat("a", 257), false},
			{"empty", "", false},
		} {
			t.Run(operation+"/"+identity.name, func(t *testing.T) {
				service, repository, _, _ := publisherFixture(t)
				original := Task{ID: "build_existing"}
				repository.task = original
				ctx, output := publisherLogContext(t)
				var task Task
				var err error
				message, level, wantErr := "publisher task lookup failed", "WARN", domain.ErrNotFound
				if operation == "inspect" {
					task, err = service.Inspect(ctx, identity.value)
				} else {
					yes := true
					input := RecoveryInput{ConfirmedAbsent: &yes, Note: "verified absent", ActorSubject: "admin-1"}
					message, level = "publisher recovery failed", "ERROR"
					if operation == "invalid-recovery" {
						input = RecoveryInput{}
						message, level, wantErr = "publisher recovery rejected", "WARN", domain.ErrInvalidInput
					}
					task, err = service.Recover(ctx, identity.value, input)
				}
				if !errors.Is(err, wantErr) || !reflect.DeepEqual(task, Task{}) || !reflect.DeepEqual(repository.task, original) {
					t.Fatalf("failure logging changed task results or storage: %#v, %v", task, err)
				}
				var event map[string]any
				if err := json.NewDecoder(output).Decode(&event); err != nil {
					t.Fatal(err)
				}
				wantID := ""
				if identity.safe {
					wantID = identity.value
				}
				if event["msg"] != message || event["level"] != level || event["build_id"] != wantID {
					t.Fatalf("missing or unsafe failure identity: %#v", event)
				}
			})
		}
	}
}

type diagnosticPagesError struct {
	api    error
	status int
}

func (err diagnosticPagesError) Error() string       { return "Pages cloud request was rejected" }
func (err diagnosticPagesError) Unwrap() error       { return err.api }
func (err diagnosticPagesError) HTTPStatusCode() int { return err.status }

// TestPublisherKeepsProviderClassificationsButNeverResponseBodies proves that
// safeError happens after a useful, structured diagnostic has been recorded.
func TestPublisherKeepsProviderClassificationsButNeverResponseBodies(t *testing.T) {
	service, _, provider, request := publisherFixture(t)
	provider.err = diagnosticPagesError{api: &smithy.GenericAPIError{Code: "AccessDenied", Message: "client_secret=signing-secret"}, status: 403}
	ctx, output := publisherLogContext(t)
	if _, err := service.Trigger(ctx, request); err != nil {
		t.Fatal(err)
	}
	if err := service.Reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	event := findPublisherEvent(t, output, "deployment outcome unknown; verify Pages deployment before manual recovery", "WARN")
	errorField, ok := event["error"].(map[string]any)
	if !ok {
		t.Fatalf("structured provider cause missing: %#v", event)
	}
	if _, ok := errorField["causes"].(map[string]any); !ok {
		t.Fatalf("wrapped provider cause missing: %#v", errorField)
	}
	encoded := fmt.Sprint(errorField)
	if !strings.Contains(encoded, "AccessDenied") || errorField["http_status"] != float64(403) || strings.Contains(encoded, "signing-secret") {
		t.Fatalf("provider classification/body mismatch: %#v", errorField)
	}
}

// findPublisherEvent decodes structured events so tests exercise semantics and
// correlation instead of depending on console formatting or field order.
func findPublisherEvent(t *testing.T, output *bytes.Buffer, message, level string) map[string]any {
	t.Helper()
	for _, line := range strings.Split(strings.TrimSpace(output.String()), "\n") {
		var event map[string]any
		if json.Unmarshal([]byte(line), &event) != nil {
			continue
		}
		if event["msg"] == message {
			if event["level"] != level || event["component"] != "publisher" || event["build_id"] == "" || event["stage"] == "" {
				t.Fatalf("incomplete lifecycle event: %#v", event)
			}
			return event
		}
	}
	t.Fatalf("missing %s/%s event: %s", level, message, output.String())
	return nil
}

// TestPublisherLogsBuildCauseBeforePersistingPublicFailure protects the async
// diagnostic boundary where the worker intentionally returns a safe task error.
func TestPublisherLogsBuildCauseBeforePersistingPublicFailure(t *testing.T) {
	service, repository, _, request := publisherFixture(t)
	service.options.Generate = func(context.Context, []byte) (map[string][]byte, error) {
		return nil, errors.New("renderer failed on private-snapshot-title")
	}
	ctx, output := publisherLogContext(t)
	if _, err := service.Trigger(ctx, request); err != nil {
		t.Fatal(err)
	}
	if err := service.Reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	if repository.task.Phase != "failed" || repository.task.Error != "Snapshot validation or static build failed" {
		t.Fatalf("logging changed task failure: %#v", repository.task)
	}
	event := findPublisherEvent(t, output, "static site build failed", "ERROR")
	if event["stage"] != "build.generate" || event["error_type"] == nil || event["error"] != nil || event["duration_ms"] == nil || event["release_id"] != request.ReleaseID || strings.Contains(output.String(), "private-snapshot-title") {
		t.Fatalf("build failure lost diagnostic cause: %#v", event)
	}
}

// TestPublisherLogsUnknownSubmissionWithoutRetry exposes operational next steps
// while preserving the irreversible deployment boundary and public safe errors.
func TestPublisherLogsUnknownSubmissionWithoutRetry(t *testing.T) {
	service, repository, provider, request := publisherFixture(t)
	provider.err = errors.New("Authorization: Bearer private-token https://cloud.example/path?token=private-token")
	ctx, output := publisherLogContext(t)
	if _, err := service.Trigger(ctx, request); err != nil {
		t.Fatal(err)
	}
	for range 3 {
		if err := service.Reconcile(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if provider.deployCount != 1 || repository.task.Phase != "submitting" {
		t.Fatal("diagnostics changed uncertain-submission behavior")
	}
	event := findPublisherEvent(t, output, "deployment outcome unknown; verify Pages deployment before manual recovery", "WARN")
	if event["stage"] != "deployment.submit" || event["error"] == nil {
		t.Fatalf("unknown submission missing failure cause: %#v", event)
	}
	if strings.Contains(output.String(), "private-token") {
		t.Fatal("publisher logs exposed provider credentials")
	}
}

// TestPublisherSeparatesUnchangedPollingFromTerminalTransitions avoids INFO log
// spam while keeping successful deployment and operator recovery discoverable.
func TestPublisherSeparatesUnchangedPollingFromTerminalTransitions(t *testing.T) {
	service, repository, provider, request := publisherFixture(t)
	ctx, output := publisherLogContext(t)
	if _, err := service.Trigger(ctx, request); err != nil {
		t.Fatal(err)
	}
	if err := service.Reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	provider.deployment.Status = "building"
	if err := service.Reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	findPublisherEvent(t, output, "deployment still building", "DEBUG")
	provider.deployment.Status = "succeeded"
	if err := service.Reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	event := findPublisherEvent(t, output, "deployment succeeded", "INFO")
	if event["deployment_id"] != "deploy-1" || repository.task.Phase != "succeeded" {
		t.Fatalf("terminal deployment identity missing: %#v", event)
	}
	output.Reset()
	repository.task.Phase = "submitting"
	provider.deployment = pages.Deployment{ID: "deploy-1", ProjectID: "project", Environment: "Production", Status: "succeeded"}
	id := "deploy-1"
	if _, err := service.Recover(ctx, repository.task.ID, RecoveryInput{DeploymentID: &id, Note: "private operator note", ActorSubject: "operator"}); err != nil {
		t.Fatal(err)
	}
	findPublisherEvent(t, output, "publisher recovery recorded", "INFO")
	if strings.Contains(output.String(), "private operator note") {
		t.Fatal("recovery note leaked into logs")
	}
}
