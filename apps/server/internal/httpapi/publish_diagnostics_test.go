package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"yujian.me/server/internal/domain"
	"yujian.me/server/internal/ports"
	storage "yujian.me/server/internal/providers/s3"
	"yujian.me/server/internal/publish"
	snapshotdata "yujian.me/server/internal/snapshot"
	"yujian.me/server/internal/store/memory"
)

// TestPublishSnapshotPersistenceHasSafeDiagnostics proves publication stores
// release data independently and never sends it to the configured media bucket.
func TestPublishSnapshotPersistenceHasSafeDiagnostics(t *testing.T) {
	for _, test := range []struct {
		name, stage, kind string
		versionExists     bool
		status            int
		snapshotError     bool
	}{
		{"missing version", "version.lookup", "not_found", false, http.StatusNotFound, false},
		{"snapshot database unavailable", "snapshot.save", "internal_error", true, http.StatusInternalServerError, true},
		{"new snapshot", "", "", true, http.StatusAccepted, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			objectRequests := 0
			provider := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				objectRequests++
				writer.WriteHeader(http.StatusOK)
			}))
			defer provider.Close()
			blobs, err := storage.NewBlobStore(storage.Config{Endpoint: provider.URL, PublicBaseURL: "https://media.example.test", Region: "test", Bucket: "private-bucket", AccessKeyID: "private-access", SecretAccessKey: "private-secret", UsePathStyle: true, HTTPClient: provider.Client()})
			if err != nil {
				t.Fatal(err)
			}
			state := memory.NewState()
			versions := memory.NewContentRepository(state)
			if test.versionExists {
				snapshot, err := snapshotdata.CanonicalJSON([]byte(`{"releaseId":"rel_private"}`))
				if err != nil {
					t.Fatal(err)
				}
				err = versions.CreateVersion(context.Background(), domain.ContentVersion{ID: "ver_private", Status: domain.StatusInReview, Revision: 3, ReviewApproved: true, Snapshot: snapshot, Checksum: snapshotdata.Checksum(snapshot)})
				if err != nil {
					t.Fatal(err)
				}
			}
			repository := memory.NewPublishRepository(state)
			snapshots := &diagnosticSnapshotStore{}
			if test.snapshotError {
				snapshots.err = errors.New("private database detail")
			}
			service := publish.NewService(publish.ServiceOptions{Repository: repository, BlobStore: blobs, SnapshotStore: snapshots, BuildTrigger: diagnosticBuildTrigger{}, Validator: diagnosticValidator{}})
			var logs bytes.Buffer
			router := LoggingMiddleware(testRouter(t, nil, service), slog.New(slog.NewJSONHandler(&logs, nil)))
			request := authenticatedRequest(http.MethodPost, "/api/v1/publishes", `{"versionId":"ver_private"}`, "publisher")
			request.Header.Set("Idempotency-Key", "private-idempotency")
			request.Header.Set("Cookie", "private-cookie")
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)
			if recorder.Code != test.status {
				t.Fatalf("status=%d, expected=%d, body=%s", recorder.Code, test.status, recorder.Body.String())
			}
			if test.stage != "" {
				found := false
				for _, line := range strings.Split(strings.TrimSpace(logs.String()), "\n") {
					var entry map[string]any
					if err := json.Unmarshal([]byte(line), &entry); err != nil {
						t.Fatal(err)
					}
					if entry["msg"] == "publish request failed" {
						found = true
						if entry["request_id"] != "req-test" || entry["stage"] != test.stage || entry["error_kind"] != test.kind || entry["actor_subject"] != "editor-1" {
							t.Fatalf("incorrect failure diagnostics: %#v", entry)
						}
					}
				}
				if !found {
					t.Fatalf("missing publish failure log: %s", logs.String())
				}
			}
			for _, secret := range []string{"private database detail", "private-cookie", "private-idempotency", "ver_private", "rel_private", provider.URL} {
				if strings.Contains(logs.String()+recorder.Body.String(), secret) && test.status != http.StatusAccepted {
					t.Fatalf("private detail leaked: %s", secret)
				}
			}
			if test.versionExists {
				if objectRequests != 0 {
					t.Fatalf("publish accessed the media object store %d times", objectRequests)
				}
				version, err := versions.GetVersion(context.Background(), "ver_private")
				if err != nil {
					t.Fatal(err)
				}
				if test.status != http.StatusAccepted && (version.Status != domain.StatusInReview || !version.ReviewApproved || version.Revision != 3) {
					t.Fatalf("failed storage operation changed reviewed version: %#v", version)
				}
			} else if snapshots.puts != 0 || objectRequests != 0 {
				t.Fatalf("missing version performed snapshot or media writes: snapshots=%d media=%d", snapshots.puts, objectRequests)
			}
			if test.status != http.StatusAccepted {
				if _, err := repository.GetActivePublishJob(context.Background()); !errors.Is(err, domain.ErrNotFound) {
					t.Fatalf("failed admission created an active publish: %v", err)
				}
			}
		})
	}
}

type diagnosticSnapshotStore struct {
	err      error
	puts     int
	contents map[string][]byte
}

// PutSnapshot captures publish input or injects a storage failure for diagnostics tests.
func (store *diagnosticSnapshotStore) PutSnapshot(_ context.Context, key string, content []byte) error {
	store.puts++
	if store.err != nil {
		return store.err
	}
	if store.contents == nil {
		store.contents = make(map[string][]byte)
	}
	store.contents[key] = append([]byte(nil), content...)
	return nil
}

// ReadSnapshot returns the captured canonical bytes to the publisher test double.
func (store *diagnosticSnapshotStore) ReadSnapshot(_ context.Context, key string) (io.ReadCloser, error) {
	content, ok := store.contents[key]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return io.NopCloser(bytes.NewReader(content)), nil
}

type diagnosticValidator struct{}

// Validate isolates transport diagnostics from the independent schema suite.
func (diagnosticValidator) Validate([]byte) error { return nil }

type diagnosticBuildTrigger struct{}

// Trigger represents a durable build accepted after the immutable snapshot write.
func (diagnosticBuildTrigger) Trigger(context.Context, ports.BuildRequest) (ports.BuildRun, error) {
	return ports.BuildRun{ID: "build_1", Status: domain.PublishBuilding}, nil
}

// Status leaves the accepted build running for the HTTP admission test.
func (diagnosticBuildTrigger) Status(context.Context, string) (ports.BuildRun, error) {
	return ports.BuildRun{ID: "build_1", Status: domain.PublishBuilding}, nil
}
