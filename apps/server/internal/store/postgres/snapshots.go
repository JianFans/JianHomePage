package postgres

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"unicode/utf8"

	"yujian.me/server/internal/domain"
	"yujian.me/server/internal/ports"
	snapshotdata "yujian.me/server/internal/snapshot"
)

var storedReleaseID = regexp.MustCompile(`^rel_[a-z0-9_]+$`)
var storedSnapshotChecksum = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)

// SnapshotStore keeps immutable site inputs in PostgreSQL, separate from media objects.
type SnapshotStore struct{ exec Executor }

// NewSnapshotStore binds immutable publish snapshots to the production database.
func NewSnapshotStore(exec Executor) *SnapshotStore { return &SnapshotStore{exec: exec} }

// PutSnapshot persists canonical bytes once; conflicting content for the same key is rejected.
func (store *SnapshotStore) PutSnapshot(ctx context.Context, key string, content []byte) error {
	releaseID, checksum, err := validateStoredSnapshot(key, content)
	if err != nil {
		return err
	}
	result, err := store.exec.ExecContext(ctx, `INSERT INTO publish_snapshots (snapshot_key,release_id,checksum,content)
VALUES ($1,$2,$3,$4) ON CONFLICT (snapshot_key) DO NOTHING`, key, releaseID, checksum, content)
	if err != nil {
		return err
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if inserted == 1 {
		return nil
	}
	reader, err := store.ReadSnapshot(ctx, key)
	if err != nil {
		return err
	}
	existing, readErr := io.ReadAll(reader)
	closeErr := reader.Close()
	if readErr != nil {
		return readErr
	}
	if closeErr != nil {
		return closeErr
	}
	if !bytes.Equal(existing, content) {
		return domain.ErrConflict
	}
	return nil
}

// ReadSnapshot verifies stored release identity, canonical bytes and checksum before returning them.
func (store *SnapshotStore) ReadSnapshot(ctx context.Context, key string) (io.ReadCloser, error) {
	releaseID, checksum, err := parseStoredSnapshotKey(key)
	if err != nil {
		return nil, err
	}
	var storedRelease, storedChecksum string
	var content []byte
	err = store.exec.QueryRowContext(ctx, `SELECT release_id,checksum,content FROM publish_snapshots WHERE snapshot_key = $1`, key).
		Scan(&storedRelease, &storedChecksum, &content)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	actualRelease, actualChecksum, err := validateStoredSnapshot(key, content)
	if err != nil || storedRelease != releaseID || storedChecksum != checksum || actualRelease != storedRelease || actualChecksum != storedChecksum {
		return nil, fmt.Errorf("%w: stored publish snapshot identity mismatch", domain.ErrConflict)
	}
	return io.NopCloser(bytes.NewReader(content)), nil
}

// validateStoredSnapshot checks key identity, canonical JSON, size, checksum, and release ID.
func validateStoredSnapshot(key string, content []byte) (string, string, error) {
	releaseID, checksum, err := parseStoredSnapshotKey(key)
	if err != nil || len(content) == 0 || len(content) > ports.MaxPublishSnapshotBytes || !utf8.Valid(content) {
		return "", "", domain.ErrInvalidInput
	}
	canonical, err := snapshotdata.CanonicalJSON(content)
	if err != nil || !bytes.Equal(canonical, content) || snapshotdata.Checksum(content) != checksum {
		return "", "", domain.ErrInvalidInput
	}
	var identity struct {
		ReleaseID string `json:"releaseId"`
	}
	if json.Unmarshal(content, &identity) != nil || identity.ReleaseID != releaseID {
		return "", "", domain.ErrInvalidInput
	}
	return releaseID, checksum, nil
}

// parseStoredSnapshotKey extracts the release ID and digest from a strict snapshot key.
func parseStoredSnapshotKey(key string) (string, string, error) {
	if !strings.HasPrefix(key, "snapshots/") {
		return "", "", domain.ErrInvalidInput
	}
	parts := strings.Split(key, "/")
	if len(parts) != 3 || parts[0] != "snapshots" || !storedReleaseID.MatchString(parts[1]) {
		return "", "", domain.ErrInvalidInput
	}
	checksum := strings.TrimSuffix(parts[2], ".json")
	if !storedSnapshotChecksum.MatchString(checksum) || key != "snapshots/"+parts[1]+"/"+checksum+".json" {
		return "", "", domain.ErrInvalidInput
	}
	return parts[1], checksum, nil
}

var _ ports.SnapshotStore = (*SnapshotStore)(nil)
