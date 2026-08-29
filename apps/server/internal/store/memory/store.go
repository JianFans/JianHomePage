// Package memory provides a concurrency-safe in-memory implementation of the
// service repositories. It is intended for local development and tests only;
// production deployments must supply a durable repository.
package memory

import (
	"context"
	"encoding/json"
	"sync"

	"yujian.me/server/internal/assets"
	"yujian.me/server/internal/content"
	"yujian.me/server/internal/domain"
	"yujian.me/server/internal/publish"
)

type State struct {
	mu                  sync.RWMutex
	versions            map[string]domain.ContentVersion
	assets              map[string]domain.AssetRecord
	jobs                map[string]domain.PublishJob
	idempotency         map[string]string
	pointers            map[string]domain.PublishPointer
	audits              []domain.AuditEntry
	successfulByVersion map[string]string
}

func NewState() *State {
	return &State{
		versions:            make(map[string]domain.ContentVersion),
		assets:              make(map[string]domain.AssetRecord),
		jobs:                make(map[string]domain.PublishJob),
		idempotency:         make(map[string]string),
		pointers:            make(map[string]domain.PublishPointer),
		successfulByVersion: make(map[string]string),
	}
}

type ContentRepository struct {
	state  *State
	locked bool
}

func NewContentRepository(state *State) *ContentRepository { return &ContentRepository{state: state} }

func (repository *ContentRepository) WithinTransaction(ctx context.Context, run func(content.Repository) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	repository.state.mu.Lock()
	defer repository.state.mu.Unlock()
	tx := *repository
	tx.locked = true
	return run(&tx)
}

func (repository *ContentRepository) CreateVersion(_ context.Context, version domain.ContentVersion) error {
	return repository.withWrite(func() error {
		if _, exists := repository.state.versions[version.ID]; exists {
			return domain.ErrConflict
		}
		repository.state.versions[version.ID] = cloneVersion(version)
		return nil
	})
}

func (repository *ContentRepository) GetVersion(_ context.Context, id string) (domain.ContentVersion, error) {
	var value domain.ContentVersion
	err := repository.withRead(func() error {
		found, exists := repository.state.versions[id]
		if !exists {
			return domain.ErrNotFound
		}
		value = cloneVersion(found)
		return nil
	})
	return value, err
}

func (repository *ContentRepository) UpdateVersion(_ context.Context, version domain.ContentVersion, expectedRevision int64) error {
	return repository.withWrite(func() error {
		current, exists := repository.state.versions[version.ID]
		if !exists {
			return domain.ErrNotFound
		}
		if current.Revision != expectedRevision {
			return domain.ErrConflict
		}
		repository.state.versions[version.ID] = cloneVersion(version)
		return nil
	})
}

func (repository *ContentRepository) AppendAudit(_ context.Context, entry domain.AuditEntry) error {
	return repository.withWrite(func() error {
		repository.state.audits = append(repository.state.audits, cloneAudit(entry))
		return nil
	})
}

type AssetRepository struct {
	state  *State
	locked bool
}

func NewAssetRepository(state *State) *AssetRepository { return &AssetRepository{state: state} }

func (repository *AssetRepository) WithinTransaction(ctx context.Context, run func(assets.Repository) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	repository.state.mu.Lock()
	defer repository.state.mu.Unlock()
	tx := *repository
	tx.locked = true
	return run(&tx)
}

func (repository *AssetRepository) CreateAsset(_ context.Context, asset domain.AssetRecord) error {
	return repository.withWrite(func() error {
		if _, exists := repository.state.assets[asset.ID]; exists {
			return domain.ErrConflict
		}
		repository.state.assets[asset.ID] = cloneAsset(asset)
		return nil
	})
}

func (repository *AssetRepository) GetAsset(_ context.Context, id string) (domain.AssetRecord, error) {
	var value domain.AssetRecord
	err := repository.withRead(func() error {
		found, exists := repository.state.assets[id]
		if !exists {
			return domain.ErrNotFound
		}
		value = cloneAsset(found)
		return nil
	})
	return value, err
}

func (repository *AssetRepository) UpdateAsset(_ context.Context, asset domain.AssetRecord) error {
	return repository.withWrite(func() error {
		if _, exists := repository.state.assets[asset.ID]; !exists {
			return domain.ErrNotFound
		}
		repository.state.assets[asset.ID] = cloneAsset(asset)
		return nil
	})
}

func (repository *AssetRepository) AppendAudit(_ context.Context, entry domain.AuditEntry) error {
	return repository.withWrite(func() error {
		repository.state.audits = append(repository.state.audits, cloneAudit(entry))
		return nil
	})
}

type PublishRepository struct {
	state  *State
	locked bool
}

func NewPublishRepository(state *State) *PublishRepository { return &PublishRepository{state: state} }

func (repository *PublishRepository) WithinTransaction(ctx context.Context, run func(publish.Repository) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	repository.state.mu.Lock()
	defer repository.state.mu.Unlock()
	tx := *repository
	tx.locked = true
	return run(&tx)
}

func (repository *PublishRepository) GetVersion(_ context.Context, id string) (domain.ContentVersion, error) {
	var value domain.ContentVersion
	err := repository.withRead(func() error {
		found, exists := repository.state.versions[id]
		if !exists {
			return domain.ErrNotFound
		}
		value = cloneVersion(found)
		return nil
	})
	return value, err
}

func (repository *PublishRepository) UpdateVersion(_ context.Context, version domain.ContentVersion, expectedRevision int64) error {
	return repository.withWrite(func() error {
		current, exists := repository.state.versions[version.ID]
		if !exists {
			return domain.ErrNotFound
		}
		if current.Revision != expectedRevision {
			return domain.ErrConflict
		}
		repository.state.versions[version.ID] = cloneVersion(version)
		return nil
	})
}

func (repository *PublishRepository) CreatePublishJob(_ context.Context, job domain.PublishJob) error {
	return repository.withWrite(func() error {
		if _, exists := repository.state.jobs[job.ID]; exists {
			return domain.ErrConflict
		}
		if _, exists := repository.state.idempotency[job.IdempotencyKey]; exists {
			return domain.ErrConflict
		}
		repository.state.jobs[job.ID] = job
		repository.state.idempotency[job.IdempotencyKey] = job.ID
		return nil
	})
}

func (repository *PublishRepository) GetPublishJob(_ context.Context, id string) (domain.PublishJob, error) {
	var value domain.PublishJob
	err := repository.withRead(func() error {
		found, exists := repository.state.jobs[id]
		if !exists {
			return domain.ErrNotFound
		}
		value = found
		return nil
	})
	return value, err
}

func (repository *PublishRepository) GetPublishJobByIdempotencyKey(_ context.Context, key string) (domain.PublishJob, error) {
	var value domain.PublishJob
	err := repository.withRead(func() error {
		id, exists := repository.state.idempotency[key]
		if !exists {
			return domain.ErrNotFound
		}
		value = repository.state.jobs[id]
		return nil
	})
	return value, err
}

func (repository *PublishRepository) GetSuccessfulPublishByVersion(_ context.Context, versionID string) (domain.PublishJob, error) {
	var value domain.PublishJob
	err := repository.withRead(func() error {
		id, exists := repository.state.successfulByVersion[versionID]
		if !exists {
			return domain.ErrNotFound
		}
		value = repository.state.jobs[id]
		return nil
	})
	return value, err
}

func (repository *PublishRepository) UpdatePublishJob(_ context.Context, job domain.PublishJob) error {
	return repository.withWrite(func() error {
		if _, exists := repository.state.jobs[job.ID]; !exists {
			return domain.ErrNotFound
		}
		repository.state.jobs[job.ID] = job
		if job.Status == domain.PublishSucceeded {
			repository.state.successfulByVersion[job.VersionID] = job.ID
		}
		return nil
	})
}

func (repository *PublishRepository) GetPublishPointer(_ context.Context, slot string) (domain.PublishPointer, error) {
	var value domain.PublishPointer
	err := repository.withRead(func() error {
		found, exists := repository.state.pointers[slot]
		if !exists {
			return domain.ErrNotFound
		}
		value = found
		return nil
	})
	return value, err
}

func (repository *PublishRepository) SetPublishPointer(_ context.Context, pointer domain.PublishPointer) error {
	return repository.withWrite(func() error {
		repository.state.pointers[pointer.Slot] = pointer
		return nil
	})
}

func (repository *PublishRepository) AppendAudit(_ context.Context, entry domain.AuditEntry) error {
	return repository.withWrite(func() error {
		repository.state.audits = append(repository.state.audits, cloneAudit(entry))
		return nil
	})
}

func (repository *ContentRepository) withRead(run func() error) error {
	if repository.locked {
		return run()
	}
	repository.state.mu.RLock()
	defer repository.state.mu.RUnlock()
	return run()
}

func (repository *ContentRepository) withWrite(run func() error) error {
	if repository.locked {
		return run()
	}
	repository.state.mu.Lock()
	defer repository.state.mu.Unlock()
	return run()
}

func (repository *AssetRepository) withRead(run func() error) error {
	if repository.locked {
		return run()
	}
	repository.state.mu.RLock()
	defer repository.state.mu.RUnlock()
	return run()
}

func (repository *AssetRepository) withWrite(run func() error) error {
	if repository.locked {
		return run()
	}
	repository.state.mu.Lock()
	defer repository.state.mu.Unlock()
	return run()
}

func (repository *PublishRepository) withRead(run func() error) error {
	if repository.locked {
		return run()
	}
	repository.state.mu.RLock()
	defer repository.state.mu.RUnlock()
	return run()
}

func (repository *PublishRepository) withWrite(run func() error) error {
	if repository.locked {
		return run()
	}
	repository.state.mu.Lock()
	defer repository.state.mu.Unlock()
	return run()
}

func cloneVersion(value domain.ContentVersion) domain.ContentVersion {
	value.Snapshot = append(json.RawMessage(nil), value.Snapshot...)
	return value
}

func cloneAsset(value domain.AssetRecord) domain.AssetRecord {
	value.Metadata = append(json.RawMessage(nil), value.Metadata...)
	value.Rights = append(json.RawMessage(nil), value.Rights...)
	return value
}

func cloneAudit(value domain.AuditEntry) domain.AuditEntry {
	value.Metadata = append(json.RawMessage(nil), value.Metadata...)
	return value
}

var _ content.Repository = (*ContentRepository)(nil)
var _ assets.Repository = (*AssetRepository)(nil)
var _ publish.Repository = (*PublishRepository)(nil)
