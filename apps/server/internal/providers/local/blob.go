package local

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"

	"yujian.me/server/internal/domain"
	"yujian.me/server/internal/ports"
)

type BlobStore struct {
	mu           sync.RWMutex
	objects      map[string]blobObject
	reservations map[string]uploadReservation
	now          func() time.Time
}

type blobObject struct {
	metadata ports.BlobMetadata
	data     []byte
}

type uploadReservation struct {
	request   ports.UploadRequest
	token     string
	expiresAt time.Time
}

func NewBlobStore() *BlobStore {
	return &BlobStore{
		objects:      make(map[string]blobObject),
		reservations: make(map[string]uploadReservation),
		now:          time.Now,
	}
}

func (store *BlobStore) CreateUpload(_ context.Context, request ports.UploadRequest) (ports.SignedUpload, error) {
	if request.BlobKey == "" || request.ContentType == "" || request.Size <= 0 || request.ExpiresIn <= 0 {
		return ports.SignedUpload{}, domain.ErrInvalidInput
	}
	token, err := randomToken()
	if err != nil {
		return ports.SignedUpload{}, err
	}
	expiresAt := store.now().UTC().Add(request.ExpiresIn)
	store.mu.Lock()
	store.reservations[request.BlobKey] = uploadReservation{request: request, token: token, expiresAt: expiresAt}
	store.mu.Unlock()
	headers := map[string]string{"Content-Type": request.ContentType}
	if request.Checksum != "" {
		headers["X-Yujian-Checksum"] = request.Checksum
	}
	return ports.SignedUpload{
		URL:       "http://127.0.0.1:8080/local-upload/" + escapeKeyPath(request.BlobKey) + "?token=" + url.QueryEscape(token),
		Headers:   headers,
		ExpiresAt: expiresAt,
	}, nil
}

func (store *BlobStore) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	if strings.HasPrefix(request.URL.Path, "/media/") {
		store.serveRead(writer, request)
		return
	}
	if request.Method != http.MethodPut {
		writer.Header().Set("Allow", http.MethodPut)
		http.Error(writer, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}
	key := strings.TrimPrefix(request.URL.Path, "/local-upload/")
	if key == "" || key == request.URL.Path {
		http.NotFound(writer, request)
		return
	}
	store.mu.RLock()
	reservation, exists := store.reservations[key]
	store.mu.RUnlock()
	if !exists || !secureEqual(reservation.token, request.URL.Query().Get("token")) {
		http.NotFound(writer, request)
		return
	}
	if !store.now().Before(reservation.expiresAt) {
		http.Error(writer, http.StatusText(http.StatusGone), http.StatusGone)
		return
	}
	if request.Header.Get("Content-Type") != reservation.request.ContentType {
		http.Error(writer, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	if request.ContentLength > reservation.request.Size {
		http.Error(writer, http.StatusText(http.StatusRequestEntityTooLarge), http.StatusRequestEntityTooLarge)
		return
	}
	request.Body = http.MaxBytesReader(writer, request.Body, reservation.request.Size+1)
	data, err := io.ReadAll(request.Body)
	if err != nil || int64(len(data)) > reservation.request.Size {
		http.Error(writer, http.StatusText(http.StatusRequestEntityTooLarge), http.StatusRequestEntityTooLarge)
		return
	}
	if int64(len(data)) != reservation.request.Size {
		http.Error(writer, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	checksum := checksumFor(data)
	if reservation.request.Checksum != "" &&
		(request.Header.Get("X-Yujian-Checksum") != reservation.request.Checksum || checksum != reservation.request.Checksum) {
		http.Error(writer, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	if err := store.Put(request.Context(), key, bytes.NewReader(data), ports.BlobMetadata{
		ContentType: reservation.request.ContentType,
		Size:        int64(len(data)),
		Checksum:    checksum,
	}); err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, domain.ErrConflict) {
			status = http.StatusConflict
		}
		http.Error(writer, http.StatusText(status), status)
		return
	}
	store.mu.Lock()
	delete(store.reservations, key)
	store.mu.Unlock()
	writer.WriteHeader(http.StatusNoContent)
}

func (store *BlobStore) serveRead(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet && request.Method != http.MethodHead {
		writer.Header().Set("Allow", http.MethodGet+", "+http.MethodHead)
		http.Error(writer, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}
	key := strings.TrimPrefix(request.URL.Path, "/media/")
	if validateKey(key) != nil || key == request.URL.Path {
		http.NotFound(writer, request)
		return
	}
	store.mu.RLock()
	object, exists := store.objects[key]
	if exists {
		object.data = append([]byte(nil), object.data...)
	}
	store.mu.RUnlock()
	if !exists {
		http.NotFound(writer, request)
		return
	}
	writer.Header().Set("Content-Type", object.metadata.ContentType)
	if object.metadata.Checksum != "" {
		writer.Header().Set("ETag", `"`+object.metadata.Checksum+`"`)
	}
	http.ServeContent(writer, request, path.Base(key), time.Time{}, bytes.NewReader(object.data))
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
	if expiresIn <= 0 {
		return "", domain.ErrInvalidInput
	}
	publicURL, err := store.PublicURL(context.Background(), key)
	if err != nil {
		return "", err
	}
	return "http://127.0.0.1:8080" + publicURL, nil
}

func (store *BlobStore) PublicURL(_ context.Context, key string) (string, error) {
	if err := validateKey(key); err != nil {
		return "", domain.ErrInvalidInput
	}
	return "/media/" + escapeKeyPath(key), nil
}

var _ ports.BlobStore = (*BlobStore)(nil)
var _ http.Handler = (*BlobStore)(nil)

func randomToken() (string, error) {
	buffer := make([]byte, 24)
	if _, err := rand.Read(buffer); err != nil {
		return "", fmt.Errorf("create upload token: %w", err)
	}
	return hex.EncodeToString(buffer), nil
}

func escapeKeyPath(key string) string {
	parts := strings.Split(key, "/")
	for index := range parts {
		parts[index] = url.PathEscape(parts[index])
	}
	return strings.Join(parts, "/")
}

func validateKey(key string) error {
	if key == "" || strings.HasPrefix(key, "/") || strings.Contains(key, "\\") || path.Clean(key) != key || key == "." {
		return domain.ErrInvalidInput
	}
	return nil
}

func secureEqual(left, right string) bool {
	if len(left) != len(right) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(left), []byte(right)) == 1
}

func checksumFor(data []byte) string {
	return fmt.Sprintf("sha256:%x", sha256.Sum256(data))
}
