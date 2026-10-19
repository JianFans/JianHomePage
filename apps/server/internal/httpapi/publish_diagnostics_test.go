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

// TestPublishStorageFailuresHaveSafeDiagnostics follows the real publish service
// through signed S3 HTTP operations, distinguishing provider failures from a
// missing version and the normal first-write path for an absent snapshot.
func TestPublishStorageFailuresHaveSafeDiagnostics(t *testing.T) {
	for _, test := range []struct {
		name, providerCode, stage, kind string
		versionExists                   bool
		status, headStatus, writes      int
		loggedCode                      string
	}{
		{"missing bucket", "NoSuchBucket", "snapshot.put", "external_dependency", true, http.StatusInternalServerError, http.StatusNotFound, 1, "NoSuchBucket"},
		{"storage access denied", "AccessDenied", "snapshot.put", "external_dependency", true, http.StatusInternalServerError, http.StatusNotFound, 1, "AccessDenied"},
		{"snapshot stat denied", "AccessDenied", "snapshot.stat", "external_dependency", true, http.StatusInternalServerError, http.StatusForbidden, 0, "Forbidden"},
		{"untrusted provider code", "provider-private-code", "snapshot.put", "external_dependency", true, http.StatusInternalServerError, http.StatusNotFound, 1, "Unknown"},
		{"missing version", "", "version.lookup", "not_found", false, http.StatusNotFound, http.StatusNotFound, 0, ""},
		{"new snapshot", "", "", "", true, http.StatusAccepted, http.StatusNotFound, 1, ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			puts := 0
			provider := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.Method == http.MethodHead {
					writer.WriteHeader(test.headStatus)
					return
				}
				if request.Method != http.MethodPut {
					t.Errorf("unexpected method %s", request.Method)
				}
				puts++
				if test.providerCode == "" {
					writer.WriteHeader(http.StatusOK)
					return
				}
				writer.Header().Set("Content-Type", "application/xml")
				status := http.StatusNotFound
				if test.providerCode == "AccessDenied" {
					status = http.StatusForbidden
				}
				writer.WriteHeader(status)
				_, _ = io.WriteString(writer, "<Error><Code>"+test.providerCode+"</Code><Message>provider-secret https://secret.example/private?token=private</Message></Error>")
			}))
			defer provider.Close()
			blobs, err := storage.NewBlobStore(storage.Config{Endpoint: provider.URL, PublicBaseURL: "https://media.example.test", Region: "test", Bucket: "private-bucket", AccessKeyID: "private-access", SecretAccessKey: "private-secret", UsePathStyle: true, HTTPClient: provider.Client()})
			if err != nil {
				t.Fatal(err)
			}
			state := memory.NewState()
			versions := memory.NewContentRepository(state)
			if test.versionExists {
				snapshot, err := snapshotdata.CanonicalJSON([]byte(`{"releaseId":"rel_e625589b-8110-4074-8013-11b529bd1e40"}`))
				if err != nil {
					t.Fatal(err)
				}
				err = versions.CreateVersion(context.Background(), domain.ContentVersion{ID: "ver_513db4a6-c494-4b3e-a481-1ac18ea5f3b6", Status: domain.StatusInReview, Revision: 3, ReviewApproved: true, Snapshot: snapshot, Checksum: snapshotdata.Checksum(snapshot)})
				if err != nil {
					t.Fatal(err)
				}
			}
			repository := memory.NewPublishRepository(state)
			service := publish.NewService(publish.ServiceOptions{Repository: repository, BlobStore: blobs, BuildTrigger: diagnosticBuildTrigger{}, Validator: diagnosticValidator{}})
			var logs bytes.Buffer
			router := LoggingMiddleware(testRouter(t, nil, service), slog.New(slog.NewJSONHandler(&logs, nil)))
			request := authenticatedRequest(http.MethodPost, "/api/v1/publishes", `{"versionId":"ver_513db4a6-c494-4b3e-a481-1ac18ea5f3b6"}`, "publisher")
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
						if entry["request_id"] != "req_550e8400-e29b-41d4-a716-446655440000" || entry["stage"] != test.stage || entry["error_kind"] != test.kind || entry["actor_subject"] != "editor-1" {
							t.Fatalf("incorrect failure diagnostics: %#v", entry)
						}
						if test.kind == "external_dependency" {
							status := http.StatusNotFound
							if test.providerCode == "AccessDenied" {
								status = http.StatusForbidden
							}
							if entry["provider_error_code"] != test.loggedCode || entry["provider_http_status"] != float64(status) {
								t.Fatalf("incorrect provider diagnostics: %#v", entry)
							}
						}
					}
				}
				if !found {
					t.Fatalf("missing publish failure log: %s", logs.String())
				}
			}
			for _, secret := range []string{"provider-secret", "provider-private-code", "secret.example", "private-bucket", "private-access", "private-secret", "private-cookie", "private-idempotency", "ver_513db4a6-c494-4b3e-a481-1ac18ea5f3b6", "rel_e625589b-8110-4074-8013-11b529bd1e40", provider.URL} {
				if strings.Contains(logs.String()+recorder.Body.String(), secret) && test.status != http.StatusAccepted {
					t.Fatalf("private detail leaked: %s", secret)
				}
			}
			if test.versionExists {
				if puts != test.writes {
					t.Fatalf("snapshot writes = %d, expected %d", puts, test.writes)
				}
				version, err := versions.GetVersion(context.Background(), "ver_513db4a6-c494-4b3e-a481-1ac18ea5f3b6")
				if err != nil {
					t.Fatal(err)
				}
				if test.status != http.StatusAccepted && (version.Status != domain.StatusInReview || !version.ReviewApproved || version.Revision != 3) {
					t.Fatalf("failed storage operation changed reviewed version: %#v", version)
				}
			} else if puts != 0 {
				t.Fatalf("missing version made %d storage writes", puts)
			}
			if test.status != http.StatusAccepted {
				if _, err := repository.GetActivePublishJob(context.Background()); !errors.Is(err, domain.ErrNotFound) {
					t.Fatalf("failed admission created an active publish: %v", err)
				}
			}
		})
	}
}

type diagnosticValidator struct{}

// Validate isolates transport diagnostics from the independent schema suite.
func (diagnosticValidator) Validate([]byte) error { return nil }

type diagnosticBuildTrigger struct{}

// Trigger represents a durable build accepted after the immutable snapshot write.
func (diagnosticBuildTrigger) Trigger(context.Context, ports.BuildRequest) (ports.BuildRun, error) {
	return ports.BuildRun{ID: "build_12b823dc-69a5-4093-93ed-5a85034ddbea", Status: domain.PublishBuilding}, nil
}

// Status leaves the accepted build running for the HTTP admission test.
func (diagnosticBuildTrigger) Status(context.Context, string) (ports.BuildRun, error) {
	return ports.BuildRun{ID: "build_12b823dc-69a5-4093-93ed-5a85034ddbea", Status: domain.PublishBuilding}, nil
}
