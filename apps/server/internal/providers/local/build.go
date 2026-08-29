package local

import (
	"context"
	"fmt"
	"sync"

	"yujian.me/server/internal/domain"
	"yujian.me/server/internal/ports"
)

type BuildTrigger struct {
	mu   sync.RWMutex
	runs map[string]ports.BuildRun
	next int
}

func NewBuildTrigger() *BuildTrigger { return &BuildTrigger{runs: make(map[string]ports.BuildRun)} }

func (trigger *BuildTrigger) Trigger(_ context.Context, _ ports.BuildRequest) (ports.BuildRun, error) {
	trigger.mu.Lock()
	defer trigger.mu.Unlock()
	trigger.next++
	id := fmt.Sprintf("local-build-%d", trigger.next)
	run := ports.BuildRun{ID: id, Status: domain.PublishSucceeded, PreviewURL: "http://127.0.0.1:8080/local-preview/" + id}
	trigger.runs[id] = run
	return run, nil
}

func (trigger *BuildTrigger) Status(_ context.Context, id string) (ports.BuildRun, error) {
	trigger.mu.RLock()
	defer trigger.mu.RUnlock()
	run, exists := trigger.runs[id]
	if !exists {
		return ports.BuildRun{}, domain.ErrNotFound
	}
	return run, nil
}

var _ ports.BuildTrigger = (*BuildTrigger)(nil)
