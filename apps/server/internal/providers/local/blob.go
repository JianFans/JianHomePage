package local

import (
	"context"
	"io"
	"net/url"
	"path"
	"sync"
	"time"

	"yujian.me/server/internal/domain"
	"yujian.me/server/internal/ports"
)

type BlobStore struct {
	mu      sync.RWMutex
	objects map[string]blobObject
}

type blobObject struct {
	metadata ports.BlobMetadata
	data     []byte
}

func NewBlobStore() *BlobStore { return &BlobStore{objects: make(map[string]blobObject)} }

func (store *BlobStore) CreateUpload(_ context.Context, request ports.UploadRequest) (ports.SignedUpload, error) {
	if request.BlobKey == "" {
		return ports.SignedUpload{}, domain.ErrInvalidInput
	}
	return ports.SignedUpload{
		URL:       "http://127.0.0.1:8080/local-upload/" + url.PathEscape(request.BlobKey),
		Headers:   map[string]string{"Content-Type": request.ContentType},
		ExpiresAt: time.Now().UTC().Add(request.ExpiresIn),
	}, nil
}

func (store *BlobStore) Stat(_ context.Context, key string) (ports.BlobMetadata, error) {
	store.mu.RLock()
	defer store.mu.RUnlock()
	object, exists := store.objects[key]
	if !exists {
		return ports.BlobMetadata{}, domain.ErrNotFound
	}
	return object.metadata, nil
}

func (store *BlobStore) Put(_ context.Context, key string, reader io.Reader, metadata ports.BlobMetadata) error {
	data, err := io.ReadAll(reader)
	if err != nil {
		return err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if current, exists := store.objects[key]; exists {
		if current.metadata.Checksum != metadata.Checksum || current.metadata.Size != metadata.Size {
			return domain.ErrConflict
		}
		return nil
	}
	store.objects[key] = blobObject{metadata: metadata, data: append([]byte(nil), data...)}
	return nil
}

func (store *BlobStore) Delete(_ context.Context, key string) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	if _, exists := store.objects[key]; !exists {
		return domain.ErrNotFound
	}
	delete(store.objects, key)
	return nil
}

func (store *BlobStore) SignedReadURL(_ context.Context, key string, expiresIn time.Duration) (string, error) {
	if key == "" || expiresIn <= 0 {
		return "", domain.ErrInvalidInput
	}
	return "http://127.0.0.1:8080/local-read/" + path.Clean("/"+key), nil
}

var _ ports.BlobStore = (*BlobStore)(nil)
