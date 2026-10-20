package publish

import (
	"context"
	"errors"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"yujian.me/server/internal/domain"
	"yujian.me/server/internal/observability"
)

var safeLifecycleID = regexp.MustCompile(`^(?:ver|rel|pub)_[A-Za-z0-9_-]{1,128}$`)
var safeBuildID = regexp.MustCompile(`^build[_-][A-Za-z0-9_-]{1,128}$`)

// lifecycleLog records stable domain identities and a fixed stage, never
// snapshot data, idempotency keys, provider URLs, tokens, or error strings.
func lifecycleLog(ctx context.Context, level slog.Level, message, stage string, actor domain.Principal, job domain.PublishJob, started time.Time, err error, attributes ...any) {
	values := []any{"component", "publish", "stage", stage, "actor_subject", actor.Subject,
		"job_id", safeLifecycleResourceID(job.ID, "pub"), "version_id", safeLifecycleResourceID(job.VersionID, "ver"),
		"release_id", safeLifecycleResourceID(job.ReleaseID, "rel"), "build_id", safeLifecycleBuildID(job.BuildID),
		"operation", job.Operation, "status", job.Status}
	if !started.IsZero() {
		values = append(values, "duration_ms", time.Since(started).Milliseconds())
	}
	if err != nil {
		values = append(values, observability.Error(err))
	}
	values = append(values, attributes...)
	observability.Logger(ctx).Log(ctx, level, message, values...)
}

func safeLifecycleResourceID(value, prefix string) string {
	if strings.HasPrefix(value, prefix+"_") && safeLifecycleID.MatchString(value) {
		return value
	}
	return ""
}

func safeLifecycleBuildID(value string) string {
	if safeBuildID.MatchString(value) {
		return value
	}
	return ""
}

func lifecycleActorForJob(job domain.PublishJob) domain.Principal {
	return domain.Principal{Subject: job.RequestedBy}
}

func lifecycleError(ctx context.Context, actor domain.Principal, job domain.PublishJob, err error) {
	var failure *Failure
	if !errors.As(err, &failure) {
		lifecycleLog(ctx, slog.LevelError, "publish operation failed", "service", actor, job, time.Time{}, err)
		return
	}
	level := slog.LevelError
	kind := "internal"
	if failure.External() {
		kind = "dependency"
	} else if errors.Is(err, domain.ErrDataIntegrity) {
		kind = "data_integrity"
	} else if errors.Is(err, domain.ErrForbidden) || errors.Is(err, domain.ErrNotFound) || errors.Is(err, domain.ErrConflict) ||
		errors.Is(err, domain.ErrInvalidInput) || errors.Is(err, domain.ErrInvalidTransition) {
		level = slog.LevelWarn
		kind = "domain_rejection"
	}
	if failure.Stage() == "job.update" {
		level = slog.LevelError
		kind = "persistence_failure"
	}
	message := "publish operation failed"
	if failure.Stage() == "build.trigger" {
		message = "publisher task handoff failed; inspect publisher logs before retry"
	}
	var diagnostic error = err
	if kind == "domain_rejection" {
		diagnostic = nil
	}
	lifecycleLog(ctx, level, message, failure.Stage(), actor, job, time.Time{}, diagnostic, "error_kind", kind)
}
