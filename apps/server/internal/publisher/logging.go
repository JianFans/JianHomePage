package publisher

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"yujian.me/server/internal/observability"
)

// taskLog records only server-owned task identities and sanitized diagnostics;
// snapshots, signed URLs, idempotency keys and recovery notes are excluded.
func taskLog(ctx context.Context, level slog.Level, message, stage string, task Task, started time.Time, err error, attributes ...any) {
	values := []any{"component", "publisher", "stage", stage, "build_id", task.ID,
		"release_id", task.Request.ReleaseID, "deployment_id", task.DeploymentID, "phase", task.Phase}
	if !started.IsZero() {
		values = append(values, "duration_ms", time.Since(started).Milliseconds())
	}
	if err != nil {
		if stage == "build.generate" {
			cause := err
			if unwrapped := errors.Unwrap(err); unwrapped != nil {
				cause = unwrapped
			}
			values = append(values, "error_type", fmt.Sprintf("%T", cause))
		} else {
			values = append(values, observability.Error(err))
		}
	}
	values = append(values, attributes...)
	observability.Logger(ctx).Log(ctx, level, message, values...)
}

type buildFailure struct {
	stage string
	cause error
}

// Error provides a safe boundary label while retaining the original cause.
func (failure *buildFailure) Error() string { return "publisher failed at " + failure.stage }

// Unwrap preserves timeout, cancellation and domain sentinel classification.
func (failure *buildFailure) Unwrap() error { return failure.cause }

// buildError attaches a fixed stage without copying content into diagnostics.
func buildError(stage string, err error) error { return &buildFailure{stage: stage, cause: err} }

// buildStage identifies the innermost static-generation boundary.
func buildStage(err error) string {
	var failure *buildFailure
	if errors.As(err, &failure) {
		return failure.stage
	}
	return "build"
}
