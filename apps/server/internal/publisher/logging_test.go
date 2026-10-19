package publisher

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"github.com/aws/smithy-go"
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
