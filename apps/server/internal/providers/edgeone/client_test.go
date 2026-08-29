package edgeone

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"yujian.me/server/internal/domain"
	"yujian.me/server/internal/ports"
)

func TestNewClientRejectsMissingTriggerURL(t *testing.T) {
	if _, err := NewClient(Config{StatusURL: "https://status.example.test"}); err == nil {
		t.Fatal("expected missing trigger URL error")
	}
}

func TestClientRejectsHTTPSDowngradeRedirectBeforeSendingToken(t *testing.T) {
	var insecureCalls atomic.Int32
	insecure := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		insecureCalls.Add(1)
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"id":"build-1","status":"building"}`))
	}))
	defer insecure.Close()
	secure := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		http.Redirect(writer, request, insecure.URL, http.StatusTemporaryRedirect)
	}))
	defer secure.Close()
	client, err := NewClient(Config{
		TriggerURL: secure.URL + "/trigger", StatusURL: secure.URL + "/status",
		Token: "secret-token", HTTPClient: secure.Client(), RequireHTTPS: true,
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	if _, err := client.Trigger(context.Background(), ports.BuildRequest{ReleaseID: "rel_1"}); err == nil {
		t.Fatal("expected HTTPS downgrade rejection")
	}
	if insecureCalls.Load() != 0 {
		t.Fatalf("insecure redirect target received %d requests", insecureCalls.Load())
	}
}

func TestTriggerSendsReleaseAndBearerToken(t *testing.T) {
	var received struct {
		ReleaseID        string `json:"releaseId"`
		SnapshotKey      string `json:"snapshotKey"`
		SnapshotChecksum string `json:"snapshotChecksum"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			t.Fatalf("unexpected method %s", request.Method)
		}
		if got := request.Header.Get("Authorization"); got != "Bearer secret-token" {
			t.Fatalf("unexpected authorization header %q", got)
		}
		if got := request.Header.Get("Idempotency-Key"); got != "publish-key-1" {
			t.Fatalf("unexpected idempotency header %q", got)
		}
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatalf("read request: %v", err)
		}
		var raw map[string]json.RawMessage
		if err := json.Unmarshal(body, &raw); err != nil {
			t.Fatalf("decode raw request: %v", err)
		}
		if _, exists := raw["releaseId"]; !exists || raw["ReleaseID"] != nil || raw["idempotencyKey"] != nil {
			t.Fatalf("provider contract must use lowerCamelCase keys: %s", body)
		}
		if err := json.Unmarshal(body, &received); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		writer.Header().Set("Content-Type", "application/json")
		writer.WriteHeader(http.StatusAccepted)
		_, _ = writer.Write([]byte(`{"id":"build-1","status":"building","previewUrl":"https://preview.example.test"}`))
	}))
	defer server.Close()

	client, err := NewClient(Config{TriggerURL: server.URL + "/trigger", StatusURL: server.URL + "/status", Token: "secret-token"})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	run, err := client.Trigger(context.Background(), ports.BuildRequest{
		IdempotencyKey:   "publish-key-1",
		ReleaseID:        "rel_1",
		SnapshotKey:      "snapshots/rel_1/sha256:test.json",
		SnapshotChecksum: "sha256:test",
	})
	if err != nil {
		t.Fatalf("trigger: %v", err)
	}
	if run.ID != "build-1" || run.Status != domain.PublishBuilding || run.PreviewURL == "" {
		t.Fatalf("unexpected build run %#v", run)
	}
	if received.ReleaseID != "rel_1" || received.SnapshotKey == "" || received.SnapshotChecksum != "sha256:test" {
		t.Fatalf("unexpected request %#v", received)
	}
}

func TestStatusEscapesBuildIDAndMapsFailure(t *testing.T) {
	var path string
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		path = request.URL.Path
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"id":"build/a","status":"failed","error":"lint failed"}`))
	}))
	defer server.Close()

	client, err := NewClient(Config{TriggerURL: server.URL, StatusURL: server.URL + "/status"})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	run, err := client.Status(context.Background(), "build/a")
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if !strings.Contains(path, "build%2Fa") || run.Status != domain.PublishFailed || run.Error != "lint failed" {
		t.Fatalf("unexpected path/run path=%q run=%#v", path, run)
	}
}

func TestNonSuccessResponseDoesNotExposeProviderBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusBadGateway)
		_, _ = writer.Write([]byte("provider secret token should not escape"))
	}))
	defer server.Close()

	client, err := NewClient(Config{TriggerURL: server.URL, StatusURL: server.URL + "/status"})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	_, err = client.Trigger(context.Background(), ports.BuildRequest{ReleaseID: "rel_1"})
	if err == nil || strings.Contains(err.Error(), "provider secret token") {
		t.Fatalf("unexpected provider error %v", err)
	}
}
