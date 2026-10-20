package local

import (
	"bytes"
	"context"
	"io"
	"sync"

	"yujian.me/server/internal/domain"
	"yujian.me/server/internal/ports"
)

// SnapshotStore keeps immutable development snapshots separate from media files.
type SnapshotStore struct {
	mu      sync.RWMutex
	objects map[string][]byte
}

// NewSnapshotStore creates an in-memory store for local-only publish workflows.
func NewSnapshotStore() *SnapshotStore {
	return &SnapshotStore{objects: make(map[string][]byte)}
}

// PutSnapshot stores the first value for a key and rejects attempts to replace it.
func (store *SnapshotStore) PutSnapshot(_ context.Context, key string, content []byte) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	if current, exists := store.objects[key]; exists {
		if !bytes.Equal(current, content) {
			return domain.ErrConflict
		}
		return nil
	}
	store.objects[key] = append([]byte(nil), content...)
	return nil
}

// ReadSnapshot returns an independent reader over the saved snapshot bytes.
func (store *SnapshotStore) ReadSnapshot(_ context.Context, key string) (io.ReadCloser, error) {
	store.mu.RLock()
	defer store.mu.RUnlock()
	content, exists := store.objects[key]
	if !exists {
		return nil, domain.ErrNotFound
	}
	return io.NopCloser(bytes.NewReader(append([]byte(nil), content...))), nil
}

var _ ports.SnapshotStore = (*SnapshotStore)(nil)
