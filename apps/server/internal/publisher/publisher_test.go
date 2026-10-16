package publisher

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"yujian.me/server/internal/domain"
	"yujian.me/server/internal/ports"
	"yujian.me/server/internal/providers/pages"
)

// taskMemory stores real transitions while substituting only the external persistence boundary.
type taskMemory struct {
	task             Task
	key              string
	locked           bool
	updateErrorPhase string
}

// Enqueue models the unique key and single active slot used by PostgreSQL.
func (m *taskMemory) Enqueue(_ context.Context, key string, r ports.BuildRequest, target Target) (Task, error) {
	r.IdempotencyKey = ""
	if m.task.ID != "" {
		if key == m.key && r == m.task.Request {
			return m.task, nil
		}
		return Task{}, domain.ErrConflict
	}
	m.key = key
	m.task = Task{ID: fmt.Sprintf("build_%x", sha256.Sum256([]byte(key))), Request: r, Target: target, Phase: "queued"}
	return m.task, nil
}

// Get distinguishes missing tasks from persisted state.
func (m *taskMemory) Get(_ context.Context, id string) (Task, error) {
	if m.task.ID != id {
		return Task{}, domain.ErrNotFound
	}
	return m.task, nil
}

// Active mirrors the database partial-index predicate.
func (m *taskMemory) Active(context.Context) (Task, error) {
	switch m.task.Phase {
	case "queued", "building", "submitting", "deployed":
		return m.task, nil
	}
	return Task{}, domain.ErrNotFound
}

// Update checks both the lock and the expected phase before committing a transition.
func (m *taskMemory) Update(_ context.Context, task Task, phase string) (Task, error) {
	if !m.locked || m.task.Phase != phase {
		return Task{}, domain.ErrConflict
	}
	if task.Phase == m.updateErrorPhase {
		return Task{}, errors.New("database password=secret")
	}
	m.task = task
	return task, nil
}

// WithLock scopes exclusive mutation ownership to the supplied operation.
func (m *taskMemory) WithLock(_ context.Context, run func(Repository) error) error {
	if m.locked {
		return nil
	}
	m.locked = true
	defer func() { m.locked = false }()
	return run(m)
}

type testPages struct {
	memory                *taskMemory
	deployCount, getCount int
	deployment            pages.Deployment
	err                   error
}

// Deploy asserts that the irreversible cloud call observes a committed submitting state.
func (p *testPages) Deploy(_ context.Context, project string, files map[string][]byte) (pages.Deployment, error) {
	if p.memory.task.Phase != "submitting" || !p.memory.locked || project != "project" || string(files["index.html"]) != "<html>real</html>" {
		return pages.Deployment{}, errors.New("unsafe deployment boundary")
	}
	p.deployCount++
	return p.deployment, p.err
}

// Get represents a side-effect-free provider status check.
func (p *testPages) Get(context.Context, string, string) (pages.Deployment, error) {
	p.getCount++
	return p.deployment, p.err
}

// publisherFixture provides deterministic bytes and observable cloud operations.
func publisherFixture(t *testing.T) (*Service, *taskMemory, *testPages, ports.BuildRequest) {
	t.Helper()
	raw := `{"releaseId":"rel_test","site":{"canonicalUrl":"https://site.example/"},"assets":[{"src":"https://media.example/image.webp"}]}`
	r := ports.BuildRequest{IdempotencyKey: "opaque-key", ReleaseID: "rel_test", SnapshotChecksum: fmt.Sprintf("sha256:%x", sha256.Sum256([]byte(raw)))}
	r.SnapshotKey = "snapshots/" + r.ReleaseID + "/" + r.SnapshotChecksum + ".json"
	m := &taskMemory{}
	p := &testPages{memory: m, deployment: pages.Deployment{ID: "deploy-1", ProjectID: "project", Environment: "Production", Status: "succeeded"}}
	s, err := NewService(Options{Repository: m, Target: Target{ProjectID: "project", Region: "global", CodeRevision: strings.Repeat("a", 40), PublicURL: "https://site.example/"}, Pages: p,
		Generate: func(context.Context, []byte) (map[string][]byte, error) {
			return map[string][]byte{"index.html": []byte("<html>real</html>")}, nil
		},
		ReadSnapshot: func(context.Context, string) (io.ReadCloser, error) { return io.NopCloser(strings.NewReader(raw)), nil }, Now: func() time.Time { return time.Date(2026, 10, 16, 0, 0, 0, 0, time.UTC) }})
	if err != nil {
		t.Fatal(err)
	}
	return s, m, p, r
}

// TestDurableBuild verifies submission, restart recovery and terminal URL publication.
func TestDurableBuild(t *testing.T) {
	s, m, p, r := publisherFixture(t)
	run, err := s.Trigger(t.Context(), r)
	if err != nil || run.Status != domain.PublishPending {
		t.Fatalf("enqueue: %#v %v", run, err)
	}
	if err := s.Reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}
	if m.task.Phase != "deployed" || p.deployCount != 1 {
		t.Fatalf("submission %#v calls=%d", m.task, p.deployCount)
	}
	restarted, err := NewService(s.options)
	if err != nil {
		t.Fatal(err)
	}
	if err := restarted.Reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}
	status, err := restarted.Status(t.Context(), run.ID)
	if err != nil || status.Status != domain.PublishSucceeded || status.PreviewURL != "https://site.example/" || p.deployCount != 1 || p.getCount != 1 {
		t.Fatalf("terminal %#v %v calls=%d/%d", status, err, p.deployCount, p.getCount)
	}
}

// TestUncertainCreationNeverRetries retains an active task after a cloud timeout and restart.
func TestUncertainCreationNeverRetries(t *testing.T) {
	s, m, p, r := publisherFixture(t)
	p.err = errors.New("Authorization: Bearer secret")
	if _, err := s.Trigger(t.Context(), r); err != nil {
		t.Fatal(err)
	}
	for range 3 {
		if err := s.Reconcile(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
	if m.task.Phase != "submitting" || p.deployCount != 1 || strings.Contains(m.task.Error, "secret") || m.task.Error == "" {
		t.Fatalf("unsafe retry %#v calls=%d", m.task, p.deployCount)
	}
}

// TestSubmissionCommitFailureNeverCallsCloud proves the database/cloud ordering boundary.
func TestSubmissionCommitFailureNeverCallsCloud(t *testing.T) {
	s, m, p, r := publisherFixture(t)
	m.updateErrorPhase = "submitting"
	if _, err := s.Trigger(t.Context(), r); err != nil {
		t.Fatal(err)
	}
	if err := s.Reconcile(t.Context()); err == nil {
		t.Fatal("expected storage failure")
	}
	if p.deployCount != 0 || m.task.Phase != "building" {
		t.Fatalf("cloud called before commit: %#v", m.task)
	}
}

// TestFrozenTargetBlocksAllCloudOperations prevents a changed binary identity from rebuilding.
func TestFrozenTargetBlocksAllCloudOperations(t *testing.T) {
	s, m, p, r := publisherFixture(t)
	if _, err := s.Trigger(t.Context(), r); err != nil {
		t.Fatal(err)
	}
	s.options.Target.CodeRevision = strings.Repeat("b", 40)
	if err := s.Reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}
	if m.task.Phase != "queued" || m.task.Error == "" || p.deployCount != 0 || p.getCount != 0 {
		t.Fatalf("target drift %#v", m.task)
	}
}

// TestCompiledIdentityAcceptsSHA256 binds embedded render resources rather than a Git checkout.
func TestCompiledIdentityAcceptsSHA256(t *testing.T) {
	s, _, _, _ := publisherFixture(t)
	options := s.options
	options.Target.CodeRevision = strings.Repeat("a", 64)
	if _, err := NewService(options); err != nil {
		t.Fatalf("compiled SHA-256 identity rejected: %v", err)
	}
}

// TestTriggerValidatesImmutableIdentity preserves opaque keys and rejects mutable object references.
func TestTriggerValidatesImmutableIdentity(t *testing.T) {
	for _, name := range []string{"short-key", "long-key", "invalid-utf8", "control", "release", "checksum", "path"} {
		t.Run(name, func(t *testing.T) {
			s, _, _, request := publisherFixture(t)
			switch name {
			case "short-key":
				request.IdempotencyKey = "short"
			case "long-key":
				request.IdempotencyKey = strings.Repeat("x", 129)
			case "invalid-utf8":
				request.IdempotencyKey = "opaque-\xff"
			case "control":
				request.IdempotencyKey = "opaque-\x00key"
			case "release":
				request.ReleaseID = "../rel_test"
			case "checksum":
				request.SnapshotChecksum = "wrong"
			case "path":
				request.SnapshotKey = "snapshots/latest.json"
			}
			if _, err := s.Trigger(t.Context(), request); !errors.Is(err, domain.ErrInvalidInput) {
				t.Fatalf("invalid request accepted: %v", err)
			}
		})
	}
	s, m, _, r := publisherFixture(t)
	r.IdempotencyKey = "  发布-opaque-key  "
	first, err := s.Trigger(t.Context(), r)
	if err != nil {
		t.Fatal(err)
	}
	s.options.Target.ProjectID = "other"
	second, err := s.Trigger(t.Context(), r)
	if err != nil || first.ID != second.ID || m.key != "发布-opaque-key" || m.task.Target.ProjectID != "project" {
		t.Fatalf("opaque idempotency %#v %v", second, err)
	}
	r.SnapshotChecksum = "sha256:" + strings.Repeat("b", 64)
	r.SnapshotKey = "snapshots/" + r.ReleaseID + "/" + r.SnapshotChecksum + ".json"
	if _, err := s.Trigger(t.Context(), r); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("changed immutable body accepted: %v", err)
	}
	r.IdempotencyKey = "another-key"
	if _, err := s.Trigger(t.Context(), r); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("second active task accepted: %v", err)
	}
}

// TestInterruptedBuildingRebuilds keeps repeatable local work restartable.
func TestInterruptedBuildingRebuilds(t *testing.T) {
	s, m, p, r := publisherFixture(t)
	if _, err := s.Trigger(t.Context(), r); err != nil {
		t.Fatal(err)
	}
	m.task.Phase = "building"
	if err := s.Reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}
	if m.task.Phase != "deployed" || p.deployCount != 1 {
		t.Fatalf("building did not resume: %#v", m.task)
	}
}

// TestLostDeploymentPersistenceStaysUncertain never repeats a successfully returned cloud creation.
func TestLostDeploymentPersistenceStaysUncertain(t *testing.T) {
	s, m, p, r := publisherFixture(t)
	m.updateErrorPhase = "deployed"
	if _, err := s.Trigger(t.Context(), r); err != nil {
		t.Fatal(err)
	}
	if err := s.Reconcile(t.Context()); err == nil || strings.Contains(err.Error(), "secret") {
		t.Fatalf("storage error leaked or lost: %v", err)
	}
	m.updateErrorPhase = ""
	if err := s.Reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}
	if m.task.Phase != "submitting" || p.deployCount != 1 || m.task.Error != uncertainMessage {
		t.Fatalf("unsafe retry %#v", m.task)
	}
}

// TestDeploymentQueriesPreserveSafeStates rejects mismatched identities and unknown provider status.
func TestDeploymentQueriesPreserveSafeStates(t *testing.T) {
	for _, name := range []string{"building", "failed", "unknown", "wrong-id", "wrong-project", "preview", "network"} {
		t.Run(name, func(t *testing.T) {
			s, m, p, r := publisherFixture(t)
			if _, err := s.Trigger(t.Context(), r); err != nil {
				t.Fatal(err)
			}
			m.task.Phase = "deployed"
			m.task.DeploymentID = "deploy-1"
			switch name {
			case "building", "failed", "unknown":
				p.deployment.Status = name
			case "wrong-id":
				p.deployment.ID = "other"
			case "wrong-project":
				p.deployment.ProjectID = "other"
			case "preview":
				p.deployment.Environment = "Preview"
			case "network":
				p.err = errors.New("token=secret")
			}
			if err := s.Reconcile(t.Context()); err != nil {
				t.Fatal(err)
			}
			if p.deployCount != 0 || p.getCount != 1 {
				t.Fatal("deployed task triggered cloud creation")
			}
			if name == "failed" {
				if m.task.Phase != "failed" || m.task.Error != "Pages deployment failed" {
					t.Fatalf("failed %#v", m.task)
				}
			} else if m.task.Phase != "deployed" {
				t.Fatalf("unverified terminal phase: %#v", m.task)
			}
			if name != "building" && name != "failed" && (m.task.Error == "" || strings.Contains(m.task.Error, "secret")) {
				t.Fatalf("unsafe diagnostic %#v", m.task)
			}
		})
	}
}

// TestSnapshotValidationPrecedesGeneration validates raw bytes independently of generator semantics.
func TestSnapshotValidationPrecedesGeneration(t *testing.T) {
	for _, name := range []string{"checksum", "release", "json", "utf8", "oversize", "read", "generator", "empty"} {
		t.Run(name, func(t *testing.T) {
			s, m, p, r := publisherFixture(t)
			raw := []byte(`{"releaseId":"rel_test","site":{"canonicalUrl":"https://site.example/"},"assets":[{"src":"https://media.example/image.webp"}]}`)
			switch name {
			case "release":
				raw = []byte(`{"releaseId":"rel_other"}`)
			case "json":
				raw = []byte(`{"releaseId":`)
			case "utf8":
				raw = []byte("{\"releaseId\":\"rel_test\",\"value\":\"\xff\"}")
			case "oversize":
				raw = []byte(strings.Repeat(" ", maxSnapshotBytes+1))
			}
			if name != "checksum" {
				r.SnapshotChecksum = fmt.Sprintf("sha256:%x", sha256.Sum256(raw))
				r.SnapshotKey = "snapshots/" + r.ReleaseID + "/" + r.SnapshotChecksum + ".json"
			} else {
				raw = append(raw, ' ')
			}
			generated := 0
			s.options.ReadSnapshot = func(context.Context, string) (io.ReadCloser, error) {
				if name == "read" {
					return nil, errors.New("S3 secret")
				}
				return io.NopCloser(strings.NewReader(string(raw))), nil
			}
			s.options.Generate = func(context.Context, []byte) (map[string][]byte, error) {
				generated++
				if name == "empty" {
					return nil, nil
				}
				return nil, errors.New("generator secret")
			}
			if _, err := s.Trigger(t.Context(), r); err != nil {
				t.Fatal(err)
			}
			if err := s.Reconcile(t.Context()); err != nil {
				t.Fatal(err)
			}
			if m.task.Phase != "failed" || m.task.Error != "Snapshot validation or static build failed" || p.deployCount != 0 {
				t.Fatalf("invalid build deployed: %#v", m.task)
			}
			if name != "generator" && name != "empty" && generated != 0 {
				t.Fatal("invalid identity reached generator")
			}
			if (name == "generator" || name == "empty") && generated != 1 {
				t.Fatal("generator failure path was not exercised")
			}
		})
	}
}

// TestRecoverVerifiedAbsenceRecordsResolution releases only an explicitly resolved uncertain task.
func TestRecoverVerifiedAbsenceRecordsResolution(t *testing.T) {
	s, m, p, r := publisherFixture(t)
	if _, err := s.Trigger(t.Context(), r); err != nil {
		t.Fatal(err)
	}
	m.task.Phase = "submitting"
	absent := true
	task, err := s.Recover(t.Context(), m.task.ID, RecoveryInput{ConfirmedAbsent: &absent, Note: "  Verified in cloud console  ", ActorSubject: "admin-1"})
	if err != nil || task.Phase != "failed" || len(task.Reconciliation) != 1 || task.Reconciliation[0].Note != "Verified in cloud console" || task.Reconciliation[0].ActorSubject != "admin-1" || !task.Reconciliation[0].At.Equal(s.options.Now()) || p.getCount != 0 || p.deployCount != 0 {
		t.Fatalf("absence recovery %#v %v", task, err)
	}
}

// TestRecoverBindsOnlyVerifiedProductionDeployment checks real provider identity before binding.
func TestRecoverBindsOnlyVerifiedProductionDeployment(t *testing.T) {
	for _, name := range []string{"production", "project", "environment", "id", "network", "target", "phase"} {
		t.Run(name, func(t *testing.T) {
			s, m, p, r := publisherFixture(t)
			if _, err := s.Trigger(t.Context(), r); err != nil {
				t.Fatal(err)
			}
			m.task.Phase = "submitting"
			switch name {
			case "project":
				p.deployment.ProjectID = "other"
			case "environment":
				p.deployment.Environment = "Preview"
			case "id":
				p.deployment.ID = "other"
			case "network":
				p.err = errors.New("secret")
			case "target":
				s.options.Target.Region = "china"
			case "phase":
				m.task.Phase = "building"
			}
			id := "deploy-1"
			task, err := s.Recover(t.Context(), m.task.ID, RecoveryInput{DeploymentID: &id, Note: "Verified deployment identity", ActorSubject: "admin-1"})
			if name == "production" {
				if err != nil || task.Phase != "deployed" || task.DeploymentID != id || len(task.Reconciliation) != 1 {
					t.Fatalf("binding %#v %v", task, err)
				}
			} else if err == nil || strings.Contains(err.Error(), "secret") || len(m.task.Reconciliation) != 0 {
				t.Fatalf("unverified binding accepted/leaked: %#v %v", task, err)
			}
			if p.deployCount != 0 {
				t.Fatal("recovery must never create a deployment")
			}
		})
	}
}

// TestRecoverRejectsAmbiguousInputs validates existence, notes and UTF-16 length like Node.
func TestRecoverRejectsAmbiguousInputs(t *testing.T) {
	id := "deploy-1"
	badID := "../deployment"
	yes := true
	no := false
	for _, input := range []RecoveryInput{{Note: "note"}, {ConfirmedAbsent: &no, Note: "note"}, {ConfirmedAbsent: &yes, DeploymentID: &id, Note: "note"}, {DeploymentID: &badID, Note: "note"}, {ConfirmedAbsent: &yes, Note: " "}, {ConfirmedAbsent: &yes, Note: "note\n"}, {ConfirmedAbsent: &yes, Note: strings.Repeat("a", 501)}, {ConfirmedAbsent: &yes, Note: strings.Repeat("😀", 251)}, {ConfirmedAbsent: &yes, Note: "invalid\xff"}} {
		s, _, _, _ := publisherFixture(t)
		input.ActorSubject = "admin-1"
		if _, err := s.Recover(t.Context(), "missing", input); !errors.Is(err, domain.ErrInvalidInput) {
			t.Fatalf("accepted %#v: %v", input, err)
		}
	}
}

// TestPublisherConfigurationRejectsPartialDependencies refuses silent development fallbacks.
func TestPublisherConfigurationRejectsPartialDependencies(t *testing.T) {
	for _, name := range []string{"repository", "pages", "generator", "reader", "project", "region", "revision", "public-url", "timeout"} {
		t.Run(name, func(t *testing.T) {
			s, _, _, _ := publisherFixture(t)
			options := s.options
			switch name {
			case "repository":
				options.Repository = nil
			case "pages":
				options.Pages = nil
			case "generator":
				options.Generate = nil
			case "reader":
				options.ReadSnapshot = nil
			case "project":
				options.Target.ProjectID = "../project"
			case "region":
				options.Target.Region = "bad"
			case "revision":
				options.Target.CodeRevision = "mutable-main"
			case "public-url":
				options.Target.PublicURL = "https://user:secret@site.example/?token=secret"
			case "timeout":
				options.BuildTimeout = -time.Second
			}
			if _, err := NewService(options); err == nil || strings.Contains(err.Error(), "secret") {
				t.Fatalf("bad configuration accepted or leaked: %v", err)
			}
		})
	}
	s, _, _, _ := publisherFixture(t)
	options := s.options
	options.Now = nil
	options.BuildTimeout = 0
	configured, err := NewService(options)
	if err != nil || configured.options.Now == nil || configured.options.BuildTimeout != 10*time.Minute {
		t.Fatalf("missing bounded defaults %#v %v", configured, err)
	}
}

// TestPublisherIdentifiersMatchPages checks project configuration and recovery against the same cloud ID boundaries.
func TestPublisherIdentifiersMatchPages(t *testing.T) {
	for _, example := range []struct {
		name  string
		id    string
		valid bool
	}{
		{"single-letter", "A", true},
		{"single-digit", "0", true},
		{"mixed", "A0_b-c", true},
		{"maximum", "0" + strings.Repeat("a_-", 85), true},
		{"empty", "", false},
		{"underscore-prefix", "_project", false},
		{"hyphen-prefix", "-project", false},
		{"too-long", strings.Repeat("a", 257), false},
		{"path", "a/b", false},
		{"unicode", "项目", false},
		{"newline", "project\n", false},
	} {
		t.Run(example.name, func(t *testing.T) {
			s, m, p, request := publisherFixture(t)
			options := s.options
			options.Target.ProjectID = example.id
			if _, err := NewService(options); (err == nil) != example.valid {
				t.Fatalf("project ID validity = %v, want %v: %v", err == nil, example.valid, err)
			}
			if _, err := s.Trigger(t.Context(), request); err != nil {
				t.Fatal(err)
			}
			m.task.Phase = "submitting"
			p.deployment.ID = example.id
			got, err := s.Recover(t.Context(), m.task.ID, RecoveryInput{DeploymentID: &example.id, Note: "Verified in Pages", ActorSubject: "admin-1"})
			if example.valid {
				if err != nil || got.DeploymentID != example.id || got.Phase != "deployed" || p.getCount != 1 {
					t.Fatalf("valid deployment recovery failed: %#v %v", got, err)
				}
			} else if !errors.Is(err, domain.ErrInvalidInput) || p.getCount != 0 {
				t.Fatalf("invalid deployment ID reached provider: calls=%d error=%v", p.getCount, err)
			}
		})
	}
}

// TestBuildResponsePreservesGatewayStatus maps every durable phase without exposing a premature URL.
func TestBuildResponsePreservesGatewayStatus(t *testing.T) {
	for phase, want := range map[string]domain.PublishStatus{"queued": domain.PublishPending, "building": domain.PublishBuilding, "submitting": domain.PublishBuilding, "deployed": domain.PublishBuilding, "failed": domain.PublishFailed, "succeeded": domain.PublishSucceeded} {
		got := response(Task{ID: "build_id", Phase: phase, Error: "safe diagnostic"})
		if got.Status != want || got.ID != "build_id" || got.Error != "safe diagnostic" || got.PreviewURL != "" {
			t.Fatalf("response %s %#v", phase, got)
		}
	}
}

// TestRecoveryBusyOrInvalidActorCannotChangeTask validates authenticated identity and lock ownership.
func TestRecoveryBusyOrInvalidActorCannotChangeTask(t *testing.T) {
	for _, actor := range []string{"", " ", "invalid\xff", "control\n", strings.Repeat("a", 1025)} {
		s, m, _, r := publisherFixture(t)
		if _, err := s.Trigger(t.Context(), r); err != nil {
			t.Fatal(err)
		}
		m.task.Phase = "submitting"
		yes := true
		if _, err := s.Recover(t.Context(), m.task.ID, RecoveryInput{ConfirmedAbsent: &yes, Note: "verified", ActorSubject: actor}); !errors.Is(err, domain.ErrInvalidInput) {
			t.Fatalf("actor accepted: %q %v", actor, err)
		}
	}
	s, m, _, r := publisherFixture(t)
	if _, err := s.Trigger(t.Context(), r); err != nil {
		t.Fatal(err)
	}
	m.task.Phase = "submitting"
	m.locked = true
	yes := true
	if _, err := s.Recover(t.Context(), m.task.ID, RecoveryInput{ConfirmedAbsent: &yes, Note: "verified", ActorSubject: "admin-1"}); !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("busy lock recovery reported success: %v", err)
	}
}

// TestIdleWorkerDoesNotContactProvider makes an empty durable queue a cheap no-op.
func TestIdleWorkerDoesNotContactProvider(t *testing.T) {
	s, _, p, _ := publisherFixture(t)
	if err := s.Reconcile(t.Context()); err != nil || p.deployCount != 0 || p.getCount != 0 {
		t.Fatalf("idle worker called provider: %v", err)
	}
	if _, err := s.Status(t.Context(), "missing"); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("missing status %v", err)
	}
}

// TestRecoveryInputRoundTripAndTypes refuses wrong JSON types while accepting ordinary recovery bodies.
func TestRecoveryInputRoundTripAndTypes(t *testing.T) {
	for _, raw := range []string{`{"confirmedAbsent":"true","note":"verified"}`, `{"deploymentId":10,"note":"verified"}`, `{"confirmedAbsent":true,"note":10}`} {
		var input RecoveryInput
		if err := json.Unmarshal([]byte(raw), &input); err == nil {
			t.Fatalf("wrong JSON type accepted: %s", raw)
		}
	}
	var input RecoveryInput
	if err := json.Unmarshal([]byte(`{"confirmedAbsent":true,"note":"verified"}`), &input); err != nil || input.ConfirmedAbsent == nil || !*input.ConfirmedAbsent || input.Note != "verified" {
		t.Fatalf("normal request %#v %v", input, err)
	}
}

type blockedSnapshot struct {
	done chan struct{}
	once sync.Once
}

// Read simulates an HTTP response stalled until its body is closed.
func (s *blockedSnapshot) Read([]byte) (int, error) { <-s.done; return 0, errors.New("closed") }

// Close models net/http's cancellation contract for a blocked response stream.
func (s *blockedSnapshot) Close() error { s.once.Do(func() { close(s.done) }); return nil }

// TestBuildTimeoutClosesStalledSnapshot ensures a stuck body cannot hold the publishing lock forever.
func TestBuildTimeoutClosesStalledSnapshot(t *testing.T) {
	s, m, _, r := publisherFixture(t)
	body := &blockedSnapshot{done: make(chan struct{})}
	s.options.BuildTimeout = 20 * time.Millisecond
	s.options.ReadSnapshot = func(context.Context, string) (io.ReadCloser, error) { return body, nil }
	if _, err := s.Trigger(t.Context(), r); err != nil {
		t.Fatal(err)
	}
	if err := s.Reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}
	if m.task.Phase != "failed" {
		t.Fatalf("stalled snapshot %#v", m.task)
	}
}

// TestRecoveryJSONRejectsAmbiguousAndSpoofedFields preserves exact operator confirmation semantics.
func TestRecoveryJSONRejectsAmbiguousAndSpoofedFields(t *testing.T) {
	for _, raw := range []string{`{"deploymentId":"deploy-1","confirmedAbsent":null,"note":"verified"}`, `{"confirmedAbsent":true,"deploymentId":null,"note":"verified"}`, `{"confirmedAbsent":true,"actorSubject":"spoofed","note":"verified"}`, `{"confirmedAbsent":true,"confirmedAbsent":false,"note":"verified"}`, `{"confirmedAbsent":true,"note":"one","note":"two"}`, `{"confirmedAbsent":true,"unknown":0,"note":"verified"}`, `null`, `[]`} {
		var input RecoveryInput
		if err := json.Unmarshal([]byte(raw), &input); err == nil {
			t.Fatalf("ambiguous recovery JSON accepted: %s", raw)
		}
	}
}

// TestRecoveryRequiresAuthenticatedActor prevents an anonymous resolution audit.
func TestRecoveryRequiresAuthenticatedActor(t *testing.T) {
	s, m, _, r := publisherFixture(t)
	if _, err := s.Trigger(t.Context(), r); err != nil {
		t.Fatal(err)
	}
	m.task.Phase = "submitting"
	yes := true
	if _, err := s.Recover(t.Context(), m.task.ID, RecoveryInput{ConfirmedAbsent: &yes, Note: "verified absence"}); !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("anonymous recovery accepted: %v", err)
	}
}

// TestRecoveryRecordReadsHistoricalAndCurrentAudit separates persisted metadata from strict API inputs.
func TestRecoveryRecordReadsHistoricalAndCurrentAudit(t *testing.T) {
	for _, raw := range []string{`{"confirmedAbsent":true,"note":"verified","at":"2026-10-16T00:00:00.000Z"}`, `{"deploymentId":"deploy-1","note":"verified","at":"2026-10-16T00:00:00Z","actorSubject":"admin-1"}`} {
		var record RecoveryRecord
		if err := json.Unmarshal([]byte(raw), &record); err != nil || record.At.IsZero() || record.Note != "verified" {
			t.Fatalf("audit could not be read: %#v %v", record, err)
		}
	}
}

// TestProductionSnapshotMatchesFrozenPublicTarget rejects development media and cross-site builds.
func TestProductionSnapshotMatchesFrozenPublicTarget(t *testing.T) {
	for _, name := range []string{"local-media", "http-media", "media-credentials", "media-query", "different-site", "missing-site", "matching-trailing-slash", "https-media"} {
		t.Run(name, func(t *testing.T) {
			s, _, _, r := publisherFixture(t)
			canonical := "https://site.example"
			src := "https://media.example/image.webp"
			switch name {
			case "local-media":
				src = "/media/fixture.webp"
			case "http-media":
				src = "http://media.example/image.webp"
			case "media-credentials":
				src = "https://user:secret@media.example/image.webp"
			case "media-query":
				src = "https://media.example/image.webp?token=secret"
			case "different-site":
				canonical = "https://other.example/"
			case "missing-site":
				canonical = ""
			}
			raw := []byte(fmt.Sprintf(`{"releaseId":"rel_test","site":{"canonicalUrl":%q},"assets":[{"src":%q}]}`, canonical, src))
			r.SnapshotChecksum = fmt.Sprintf("sha256:%x", sha256.Sum256(raw))
			r.SnapshotKey = "snapshots/" + r.ReleaseID + "/" + r.SnapshotChecksum + ".json"
			s.options.ReadSnapshot = func(context.Context, string) (io.ReadCloser, error) {
				return io.NopCloser(strings.NewReader(string(raw))), nil
			}
			_, err := s.build(t.Context(), r)
			if name == "matching-trailing-slash" || name == "https-media" {
				if err != nil {
					t.Fatalf("production snapshot rejected: %v", err)
				}
			} else if err == nil {
				t.Fatal("unsafe production snapshot accepted")
			}
		})
	}
}
