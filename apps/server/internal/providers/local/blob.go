package local

import (
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
	"os"
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
	rootOnce     sync.Once
	rootPath     string
	rootErr      error
	createRoot   func() (string, error)
	active       sync.WaitGroup
	closed       bool
}

type blobObject struct {
	metadata ports.BlobMetadata
	filePath string
}

type uploadReservation struct {
	request   ports.UploadRequest
	token     string
	expiresAt time.Time
}

var (
	errBlobStoreClosed = errors.New("local blob store is closed")
	errPayloadTooLarge = errors.New("payload exceeds declared size")
	errPayloadSize     = errors.New("payload size does not match declaration")
	errPayloadChecksum = errors.New("payload checksum does not match declaration")
)

// NewBlobStore creates development-only storage with a lazily allocated
// temporary directory; Close owns removal of all files in that directory.
func NewBlobStore() *BlobStore {
	return &BlobStore{
		objects:      make(map[string]blobObject),
		reservations: make(map[string]uploadReservation),
		now:          time.Now,
		createRoot: func() (string, error) {
			return os.MkdirTemp("", "yujian-local-blobs-*")
		},
	}
}

// CreateUpload reserves a validated local object key with a random, expiring
// token and prunes abandoned reservations without allocating a payload file.
func (store *BlobStore) CreateUpload(_ context.Context, request ports.UploadRequest) (ports.SignedUpload, error) {
	if validateKey(request.BlobKey) != nil || request.ContentType == "" || request.Size <= 0 || request.ExpiresIn <= 0 {
		return ports.SignedUpload{}, domain.ErrInvalidInput
	}
	token, err := randomToken()
	if err != nil {
		return ports.SignedUpload{}, err
	}
	expiresAt := store.now().UTC().Add(request.ExpiresIn)
	store.mu.Lock()
	if store.closed {
		store.mu.Unlock()
		return ports.SignedUpload{}, errBlobStoreClosed
	}
	for key, reservation := range store.reservations {
		if !store.now().Before(reservation.expiresAt) {
			delete(store.reservations, key)
		}
	}
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

// ServeHTTP serves local media reads and reserved PUT uploads. Only a verified
// upload can extend API deadlines; streaming validation enforces size and hash.
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
		store.mu.Lock()
		if current, ok := store.reservations[key]; ok && secureEqual(current.token, reservation.token) {
			delete(store.reservations, key)
		}
		store.mu.Unlock()
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
	if reservation.request.Checksum != "" && request.Header.Get("X-Yujian-Checksum") != reservation.request.Checksum {
		http.Error(writer, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
		return
	}
	// Only a validated reservation may extend the server's short API deadlines.
	// Bound both directions: WriteTimeout otherwise expires while reading a PUT.
	controller := http.NewResponseController(writer)
	deadline := time.Now().Add(15 * time.Minute)
	for _, setDeadline := range []func(time.Time) error{controller.SetReadDeadline, controller.SetWriteDeadline} {
		if err := setDeadline(deadline); err != nil && !errors.Is(err, http.ErrNotSupported) {
			http.Error(writer, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}
	}
	request.Body = http.MaxBytesReader(writer, request.Body, reservation.request.Size+1)
	err := store.put(request.Context(), key, request.Body, ports.BlobMetadata{
		ContentType: reservation.request.ContentType,
		Size:        reservation.request.Size,
		Checksum:    reservation.request.Checksum,
	})
	if err != nil {
		status := http.StatusInternalServerError
		switch {
		case errors.Is(err, errPayloadTooLarge):
			status = http.StatusRequestEntityTooLarge
		case errors.Is(err, errPayloadSize), errors.Is(err, errPayloadChecksum), errors.Is(err, domain.ErrInvalidInput):
			status = http.StatusBadRequest
		case errors.Is(err, domain.ErrConflict):
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

// serveRead opens a tracked media handle for GET, HEAD and Range responses.
// The handle permits an existing download to finish after the object is deleted.
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
	finish, err := store.beginOperation()
	if err != nil {
		http.NotFound(writer, request)
		return
	}
	defer finish()
	store.mu.RLock()
	object, exists := store.objects[key]
	if !exists {
		store.mu.RUnlock()
		http.NotFound(writer, request)
		return
	}
	file, err := openMediaFile(object.filePath)
	store.mu.RUnlock()
	if err != nil {
		http.NotFound(writer, request)
		return
	}
	defer file.Close()
	writer.Header().Set("Content-Type", object.metadata.ContentType)
	if object.metadata.Checksum != "" {
		writer.Header().Set("ETag", `"`+object.metadata.Checksum+`"`)
	}
	http.ServeContent(writer, request, path.Base(key), time.Time{}, file)
}

// Stat returns the metadata of a published local object without reading its
// payload; unfinished uploads and deleted keys report the domain not-found error.
func (store *BlobStore) Stat(_ context.Context, key string) (ports.BlobMetadata, error) {
	store.mu.RLock()
	defer store.mu.RUnlock()
	object, exists := store.objects[key]
	if !exists {
		return ports.BlobMetadata{}, domain.ErrNotFound
	}
	return object.metadata, nil
}

// Put writes a local object and maps streaming size or checksum failures to the
// domain error expected by snapshot and publishing callers.
func (store *BlobStore) Put(ctx context.Context, key string, reader io.Reader, metadata ports.BlobMetadata) error {
	err := store.put(ctx, key, reader, metadata)
	if errors.Is(err, errPayloadTooLarge) || errors.Is(err, errPayloadSize) || errors.Is(err, errPayloadChecksum) {
		return domain.ErrInvalidInput
	}
	return err
}

// put publishes only a fully validated temporary file. Matching size and hash
// make retries idempotent; competing or failed writes discard their own file.
func (store *BlobStore) put(ctx context.Context, key string, reader io.Reader, metadata ports.BlobMetadata) error {
	if validateKey(key) != nil || metadata.ContentType == "" || metadata.Size < 0 {
		return domain.ErrInvalidInput
	}
	finish, err := store.beginOperation()
	if err != nil {
		return err
	}
	defer finish()
	store.mu.RLock()
	current, exists := store.objects[key]
	store.mu.RUnlock()
	if exists {
		if current.metadata.Checksum != metadata.Checksum || current.metadata.Size != metadata.Size {
			return domain.ErrConflict
		}
		return nil
	}
	filePath, actualMetadata, err := store.writeTemporaryObject(ctx, reader, metadata)
	if err != nil {
		return err
	}
	keepFile := false
	defer func() {
		if !keepFile {
			_ = os.Remove(filePath)
		}
	}()
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.closed {
		return errBlobStoreClosed
	}
	if current, exists := store.objects[key]; exists {
		if current.metadata.Checksum != actualMetadata.Checksum || current.metadata.Size != actualMetadata.Size {
			return domain.ErrConflict
		}
		return nil
	}
	store.objects[key] = blobObject{metadata: actualMetadata, filePath: filePath}
	keepFile = true
	return nil
}

// Delete removes the payload before its lookup entry while holding the object
// lock, so failed filesystem deletion does not hide a still-readable object.
func (store *BlobStore) Delete(_ context.Context, key string) error {
	finish, err := store.beginOperation()
	if err != nil {
		return err
	}
	defer finish()
	store.mu.Lock()
	defer store.mu.Unlock()
	if _, exists := store.objects[key]; !exists {
		return domain.ErrNotFound
	}
	if err := os.Remove(store.objects[key].filePath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	delete(store.objects, key)
	return nil
}

// Close removes all temporary local objects and makes the store unavailable.
func (store *BlobStore) Close() error {
	store.mu.Lock()
	store.closed = true
	store.reservations = make(map[string]uploadReservation)
	store.mu.Unlock()
	store.active.Wait()
	store.rootOnce.Do(func() {})
	store.mu.Lock()
	rootPath := store.rootPath
	store.objects = make(map[string]blobObject)
	store.mu.Unlock()
	if rootPath == "" {
		return nil
	}
	return os.RemoveAll(rootPath)
}

// beginOperation prevents temporary files from being removed while a local
// read, write, or delete is still using them.
func (store *BlobStore) beginOperation() (func(), error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.closed {
		return nil, errBlobStoreClosed
	}
	store.active.Add(1)
	return store.active.Done, nil
}

// SignedReadURL validates the requested lifetime but returns an unsigned local
// development URL; it does not provide production access-control semantics.
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

// validateKey requires a canonical relative object path, rejecting traversal,
// absolute paths and Windows separators before resolving it under the local root.
func validateKey(key string) error {
	if key == "" || strings.HasPrefix(key, "/") || strings.Contains(key, "\\") || path.Clean(key) != key || key == "." {
		return domain.ErrInvalidInput
	}
	for _, segment := range strings.Split(key, "/") {
		if segment == ".." {
			return domain.ErrInvalidInput
		}
	}
	return nil
}

// secureEqual compares equal-length reservation tokens in constant time and
// rejects differing lengths without accepting a partial token match.
func secureEqual(left, right string) bool {
	if len(left) != len(right) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(left), []byte(right)) == 1
}

// writeTemporaryObject copies at most the declared size plus one sentinel byte
// into a temporary file while hashing, deleting it on read or validation failure.
func (store *BlobStore) writeTemporaryObject(
	ctx context.Context,
	reader io.Reader,
	metadata ports.BlobMetadata,
) (string, ports.BlobMetadata, error) {
	rootPath, err := store.localRoot()
	if err != nil {
		return "", ports.BlobMetadata{}, err
	}
	file, err := os.CreateTemp(rootPath, "object-*")
	if err != nil {
		return "", ports.BlobMetadata{}, err
	}
	filePath := file.Name()
	keepFile := false
	defer func() {
		_ = file.Close()
		if !keepFile {
			_ = os.Remove(filePath)
		}
	}()

	hash := sha256.New()
	limited := io.LimitReader(&contextReader{ctx: ctx, reader: reader}, metadata.Size+1)
	written, err := io.CopyBuffer(io.MultiWriter(file, hash), limited, make([]byte, 64*1024))
	if err != nil {
		return "", ports.BlobMetadata{}, err
	}
	if written > metadata.Size {
		return "", ports.BlobMetadata{}, errPayloadTooLarge
	}
	if written != metadata.Size {
		return "", ports.BlobMetadata{}, errPayloadSize
	}
	actualChecksum := fmt.Sprintf("sha256:%x", hash.Sum(nil))
	if metadata.Checksum != "" && metadata.Checksum != actualChecksum {
		return "", ports.BlobMetadata{}, errPayloadChecksum
	}
	if err := file.Close(); err != nil {
		return "", ports.BlobMetadata{}, err
	}
	keepFile = true
	return filePath, metadata, nil
}

// localRoot allocates the temporary directory once and rechecks shutdown after
// creation so a racing operation cannot publish files into a closed store.
func (store *BlobStore) localRoot() (string, error) {
	store.mu.RLock()
	closed := store.closed
	store.mu.RUnlock()
	if closed {
		return "", errBlobStoreClosed
	}
	store.rootOnce.Do(func() {
		rootPath, err := store.createRoot()
		store.mu.Lock()
		store.rootPath = rootPath
		store.rootErr = err
		store.mu.Unlock()
	})
	store.mu.RLock()
	defer store.mu.RUnlock()
	if store.closed {
		return "", errBlobStoreClosed
	}
	return store.rootPath, store.rootErr
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

// Read checks cancellation before each underlying read; a read already blocked
// on a network body still needs connection shutdown to unblock.
func (reader *contextReader) Read(buffer []byte) (int, error) {
	if err := reader.ctx.Err(); err != nil {
		return 0, err
	}
	return reader.reader.Read(buffer)
}

// checksumFor produces the domain's prefixed lowercase digest for small in-memory
// fixtures; uploaded payloads use the streaming hash in writeTemporaryObject.
func checksumFor(data []byte) string {
	return fmt.Sprintf("sha256:%x", sha256.Sum256(data))
}
