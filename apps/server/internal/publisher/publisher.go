// Package publisher coordinates durable static builds without retrying uncertain cloud writes.
package publisher

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"

	"yujian.me/server/internal/domain"
	"yujian.me/server/internal/ports"
	"yujian.me/server/internal/providers/pages"
)

// Target is frozen at enqueue time using the original Node publisher's JSON keys.
type Target struct {
	ProjectID    string `json:"projectId"`
	Region       string `json:"region"`
	CodeRevision string `json:"codeRevision"`
	PublicURL    string `json:"publicUrl"`
}

// RecoveryInput distinguishes omitted values from explicit operator confirmation.
type RecoveryInput struct {
	DeploymentID    *string `json:"deploymentId,omitempty"`
	ConfirmedAbsent *bool   `json:"confirmedAbsent,omitempty"`
	Note            string  `json:"note"`
	ActorSubject    string  `json:"-"`
}

// RecoveryRecord retains the operator's verification and immutable resolution time.
type RecoveryRecord struct {
	RecoveryInput
	At           time.Time `json:"at"`
	ActorSubject string    `json:"actorSubject,omitempty"`
}

// UnmarshalJSON reads both legacy Node audits and current authenticated records;
// persisted metadata is not subject to the stricter HTTP recovery-input decoder.
func (record *RecoveryRecord) UnmarshalJSON(raw []byte) error {
	type plainInput RecoveryInput
	var stored struct {
		plainInput
		At           time.Time `json:"at"`
		ActorSubject string    `json:"actorSubject"`
	}
	if err := json.Unmarshal(raw, &stored); err != nil {
		return err
	}
	*record = RecoveryRecord{RecoveryInput: RecoveryInput(stored.plainInput), At: stored.At, ActorSubject: stored.ActorSubject}
	return nil
}

// UnmarshalJSON rejects ambiguous confirmation, repeated fields and caller-supplied audit identity.
func (input *RecoveryInput) UnmarshalJSON(raw []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return domain.ErrInvalidInput
	}
	var parsed RecoveryInput
	seen := map[string]bool{}
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return domain.ErrInvalidInput
		}
		key, ok := token.(string)
		if !ok || seen[key] {
			return domain.ErrInvalidInput
		}
		seen[key] = true
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return domain.ErrInvalidInput
		}
		switch key {
		case "deploymentId":
			err = json.Unmarshal(value, &parsed.DeploymentID)
		case "confirmedAbsent":
			err = json.Unmarshal(value, &parsed.ConfirmedAbsent)
		case "note":
			err = json.Unmarshal(value, &parsed.Note)
		default:
			return domain.ErrInvalidInput
		}
		if err != nil {
			return domain.ErrInvalidInput
		}
	}
	if _, err := decoder.Token(); err != nil {
		return domain.ErrInvalidInput
	}
	*input = parsed
	return nil
}

// Task is the persisted state of one logical build across process restarts.
type Task struct {
	ID             string             `json:"id"`
	Request        ports.BuildRequest `json:"request"`
	Target         Target             `json:"target"`
	Phase          string             `json:"phase"`
	DeploymentID   string             `json:"deploymentId"`
	PreviewURL     string             `json:"previewUrl"`
	Error          string             `json:"error"`
	Reconciliation []RecoveryRecord   `json:"reconciliation"`
	CreatedAt      time.Time          `json:"createdAt"`
}

// Repository preserves atomic compare-and-swap transitions and serializes workers.
// Update must commit before returning; WithLock must not wrap the callback in a transaction.
type Repository interface {
	Enqueue(context.Context, string, ports.BuildRequest, Target) (Task, error)
	Get(context.Context, string) (Task, error)
	Active(context.Context) (Task, error)
	Update(context.Context, Task, string) (Task, error)
	WithLock(context.Context, func(Repository) error) error
}

// Pages is the cloud boundary; deployment creation is intentionally non-idempotent.
type Pages interface {
	Deploy(context.Context, string, map[string][]byte) (pages.Deployment, error)
	Get(context.Context, string, string) (pages.Deployment, error)
}

// Options requires explicit production dependencies rather than hidden fallbacks.
type Options struct {
	Repository   Repository
	Target       Target
	Pages        Pages
	Generate     func(context.Context, []byte) (map[string][]byte, error)
	ReadSnapshot func(context.Context, string) (io.ReadCloser, error)
	Now          func() time.Time
	BuildTimeout time.Duration
}

// Service owns the frozen publisher configuration and advances one locked task per tick.
type Service struct{ options Options }

// NewService constructs a publisher only when its explicit dependencies are present.
func NewService(options Options) (*Service, error) {
	if options.Repository == nil || options.Pages == nil || options.Generate == nil || options.ReadSnapshot == nil {
		return nil, errors.New("Publisher dependencies are required")
	}
	u, err := url.Parse(options.Target.PublicURL)
	if !identifier.MatchString(options.Target.ProjectID) || (options.Target.Region != "global" && options.Target.Region != "china") || !revision.MatchString(options.Target.CodeRevision) || err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return nil, errors.New("Invalid publisher target")
	}
	if options.Now == nil {
		options.Now = time.Now
	}
	if options.BuildTimeout == 0 {
		options.BuildTimeout = 10 * time.Minute
	}
	if options.BuildTimeout < 0 {
		return nil, errors.New("Invalid publisher build timeout")
	}
	return &Service{options: options}, nil
}

// Trigger durably queues one logical build without tying worker lifetime to HTTP.
func (s *Service) Trigger(ctx context.Context, request ports.BuildRequest) (ports.BuildRun, error) {
	key, ok := domain.NormalizeIdempotencyKey(request.IdempotencyKey)
	if !ok || !validRequest(request) {
		return ports.BuildRun{}, domain.ErrInvalidInput
	}
	request.IdempotencyKey = ""
	task, err := s.options.Repository.Enqueue(ctx, key, request, s.options.Target)
	return response(task), safeError(err)
}

// Status reads the durable build response without triggering a deployment.
func (s *Service) Status(ctx context.Context, id string) (ports.BuildRun, error) {
	task, err := s.Inspect(ctx, id)
	return response(task), err
}

// Inspect returns diagnostic task information for an authenticated administrator.
func (s *Service) Inspect(ctx context.Context, id string) (Task, error) {
	task, err := s.options.Repository.Get(ctx, id)
	return task, safeError(err)
}

// Reconcile advances a task while holding the cross-process publishing lock.
func (s *Service) Reconcile(ctx context.Context) error {
	return safeError(s.options.Repository.WithLock(ctx, func(repository Repository) error {
		task, err := repository.Active(ctx)
		if errors.Is(err, domain.ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if task.Target != s.options.Target {
			task.Error = "Frozen publisher configuration differs; restore the original configuration"
			_, err = repository.Update(ctx, task, task.Phase)
			return err
		}
		switch task.Phase {
		case "submitting":
			if task.Error != "" {
				return nil
			}
			task.Error = uncertainMessage
			_, err = repository.Update(ctx, task, "submitting")
			return err
		case "queued", "building":
			previous := task.Phase
			task.Phase = "building"
			task.Error = ""
			task, err = repository.Update(ctx, task, previous)
			if err != nil {
				return err
			}
			files, buildErr := s.build(ctx, task.Request)
			if buildErr != nil {
				task.Phase = "failed"
				task.Error = "Snapshot validation or static build failed"
				_, err = repository.Update(ctx, task, "building")
				return err
			}
			task.Phase = "submitting"
			task, err = repository.Update(ctx, task, "building")
			if err != nil {
				return err
			}
			deployment, cloudErr := s.options.Pages.Deploy(ctx, task.Target.ProjectID, files)
			if cloudErr != nil || !validDeployment(deployment, task.Target, "") {
				task.Error = uncertainMessage
				_, err = repository.Update(ctx, task, "submitting")
				return err
			}
			task.Phase = "deployed"
			task.DeploymentID = deployment.ID
			_, err = repository.Update(ctx, task, "submitting")
			return err
		case "deployed":
			deployment, cloudErr := s.options.Pages.Get(ctx, task.Target.ProjectID, task.DeploymentID)
			if cloudErr != nil || !validDeployment(deployment, task.Target, task.DeploymentID) || (deployment.Status != "building" && deployment.Status != "succeeded" && deployment.Status != "failed") {
				task.Error = "Deployment status unavailable; automatic reconciliation will continue"
				_, err = repository.Update(ctx, task, "deployed")
				return err
			}
			task.Error = ""
			task.PreviewURL = ""
			if deployment.Status != "building" {
				task.Phase = deployment.Status
			}
			if task.Phase == "failed" {
				task.Error = "Pages deployment failed"
			}
			if task.Phase == "succeeded" {
				task.PreviewURL = task.Target.PublicURL
			}
			_, err = repository.Update(ctx, task, "deployed")
			return err
		default:
			return domain.ErrConflict
		}
	}))
}

// Recover records verified operator resolution for an uncertain deployment.
func (s *Service) Recover(ctx context.Context, id string, input RecoveryInput) (Task, error) {
	var recovered Task
	if !validRecovery(input) {
		return recovered, domain.ErrInvalidInput
	}
	err := s.options.Repository.WithLock(ctx, func(repository Repository) error {
		task, err := repository.Get(ctx, id)
		if err != nil {
			return err
		}
		if task.Phase != "submitting" || task.Target != s.options.Target {
			return domain.ErrConflict
		}
		if input.DeploymentID != nil {
			deployment, err := s.options.Pages.Get(ctx, task.Target.ProjectID, *input.DeploymentID)
			if err != nil {
				return errors.New("Deployment status unavailable")
			}
			if !validDeployment(deployment, task.Target, *input.DeploymentID) {
				return domain.ErrConflict
			}
			task.Phase = "deployed"
			task.DeploymentID = *input.DeploymentID
			task.Error = ""
		} else {
			task.Phase = "failed"
			task.Error = "Operator confirmed that no deployment was created"
		}
		input.Note = strings.TrimSpace(input.Note)
		task.Reconciliation = append(task.Reconciliation, RecoveryRecord{RecoveryInput: input, At: s.options.Now().UTC(), ActorSubject: input.ActorSubject})
		recovered, err = repository.Update(ctx, task, "submitting")
		return err
	})
	if err == nil && recovered.ID == "" {
		err = domain.ErrConflict
	}
	return recovered, safeError(err)
}

const uncertainMessage = "Deployment outcome unknown; reconciliation required"
const maxSnapshotBytes = 8 * 1024 * 1024

var identifier = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,255}$`)
var revision = regexp.MustCompile(`^(?:[a-f0-9]{40}|[a-f0-9]{64})$`)
var release = regexp.MustCompile(`^rel_[a-z0-9_]+$`)
var checksum = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)

// validRequest accepts exactly the immutable object reference emitted by the publishing domain.
func validRequest(request ports.BuildRequest) bool {
	return release.MatchString(request.ReleaseID) && checksum.MatchString(request.SnapshotChecksum) && request.SnapshotKey == "snapshots/"+request.ReleaseID+"/"+request.SnapshotChecksum+".json"
}

// build verifies raw snapshot identity before handing canonical semantics to the generator.
func (s *Service) build(ctx context.Context, request ports.BuildRequest) (map[string][]byte, error) {
	if !validRequest(request) {
		return nil, domain.ErrInvalidInput
	}
	ctx, cancel := context.WithTimeout(ctx, s.options.BuildTimeout)
	defer cancel()
	body, err := s.options.ReadSnapshot(ctx, request.SnapshotKey)
	if err != nil {
		return nil, err
	}
	if body == nil {
		return nil, domain.ErrInvalidInput
	}
	defer body.Close()
	stop := context.AfterFunc(ctx, func() { _ = body.Close() })
	defer stop()
	raw, err := io.ReadAll(io.LimitReader(body, maxSnapshotBytes+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > maxSnapshotBytes || !utf8.Valid(raw) || fmt.Sprintf("sha256:%x", sha256.Sum256(raw)) != request.SnapshotChecksum {
		return nil, domain.ErrInvalidInput
	}
	var identity struct {
		ReleaseID string `json:"releaseId"`
		Site      struct {
			CanonicalURL string `json:"canonicalUrl"`
		} `json:"site"`
		Assets []struct {
			Src string `json:"src"`
		} `json:"assets"`
	}
	if err := json.Unmarshal(raw, &identity); err != nil || identity.ReleaseID != request.ReleaseID {
		return nil, domain.ErrInvalidInput
	}
	if !productionURL(identity.Site.CanonicalURL) || strings.TrimSuffix(identity.Site.CanonicalURL, "/") != strings.TrimSuffix(s.options.Target.PublicURL, "/") {
		return nil, domain.ErrInvalidInput
	}
	for _, asset := range identity.Assets {
		if !productionURL(asset.Src) {
			return nil, domain.ErrInvalidInput
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	files, err := s.options.Generate(ctx, raw)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, domain.ErrInvalidInput
	}
	return files, nil
}

// productionURL requires stable credential-free HTTPS addresses for published content.
func productionURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && u.Hostname() != "" && u.User == nil && u.RawQuery == "" && !u.ForceQuery && !strings.Contains(raw, "#")
}

// response maps diagnostic phases onto the existing BuildTrigger contract.
func response(task Task) ports.BuildRun {
	status := domain.PublishBuilding
	switch task.Phase {
	case "queued":
		status = domain.PublishPending
	case "succeeded":
		status = domain.PublishSucceeded
	case "failed":
		status = domain.PublishFailed
	}
	return ports.BuildRun{ID: task.ID, Status: status, PreviewURL: task.PreviewURL, Error: task.Error}
}

// validDeployment prevents binding another project or a preview deployment.
func validDeployment(deployment pages.Deployment, target Target, id string) bool {
	return identifier.MatchString(deployment.ID) && (id == "" || deployment.ID == id) && deployment.ProjectID == target.ProjectID && deployment.Environment == "Production"
}

// validRecovery preserves Node's UTF-16 note bound and mutually exclusive recovery choices.
func validRecovery(input RecoveryInput) bool {
	if !utf8.ValidString(input.ActorSubject) || strings.TrimSpace(input.ActorSubject) == "" || len(input.ActorSubject) > 1024 {
		return false
	}
	for _, r := range input.ActorSubject {
		if unicode.IsControl(r) {
			return false
		}
	}
	if !utf8.ValidString(input.Note) || strings.TrimSpace(input.Note) == "" || len(utf16.Encode([]rune(input.Note))) > 500 {
		return false
	}
	for _, r := range input.Note {
		if unicode.IsControl(r) {
			return false
		}
	}
	if input.DeploymentID != nil {
		return input.ConfirmedAbsent == nil && identifier.MatchString(*input.DeploymentID)
	}
	return input.ConfirmedAbsent != nil && *input.ConfirmedAbsent
}

// safeError keeps operational identity while withholding database/provider credentials.
func safeError(err error) error {
	if err == nil {
		return nil
	}
	for _, known := range []error{domain.ErrConflict, domain.ErrNotFound, domain.ErrInvalidInput, context.Canceled, context.DeadlineExceeded} {
		if errors.Is(err, known) {
			return known
		}
	}
	return errors.New("Publisher operation unavailable")
}

var _ ports.BuildTrigger = (*Service)(nil)
