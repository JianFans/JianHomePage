package observability

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/smithy-go"
	"github.com/jackc/pgx/v5/pgconn"
)

// TestLoggerDefaultsLevelsAndContext covers readable defaults and request-local propagation.
func TestLoggerDefaultsLevelsAndContext(t *testing.T) {
	var output bytes.Buffer
	logger, closeLog, err := New(Options{Console: &output})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = closeLog() })
	logger.Debug("hidden")
	Logger(WithLogger(t.Context(), logger)).Info("readable", "count", 3)
	if strings.Contains(output.String(), "hidden") || !strings.Contains(output.String(), "level=INFO msg=readable count=3") {
		t.Fatal(output.String())
	}
	if Logger(context.Background()) == nil {
		t.Fatal("missing fallback logger")
	}
	for _, options := range []Options{{Level: "trace"}, {Format: "xml"}, {MaxSizeMB: -1}, {MaxBackups: -1}} {
		if _, _, err := New(options); err == nil {
			t.Fatalf("accepted invalid options: %#v", options)
		}
	}
}

// TestRedactionCoversMessagesBoundAttributesGroupsAndErrors protects every slog entry path.
func TestRedactionCoversMessagesBoundAttributesGroupsAndErrors(t *testing.T) {
	var output bytes.Buffer
	logger, closeLog, err := New(Options{Format: "json", Console: &output, Secrets: []string{"configured-secret"}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = closeLog() })
	logger.With("bound", "configured-secret").WithGroup("request").Error(
		"Bearer opaque-bearer configured-secret https://alice:urlpass@example.test/path?signature=signed-secret#private-fragment",
		"password", "plain-password", "nested", map[string]any{"client_secret": "nested-secret", "safe": "healthy"},
		"jwt", "eyJhbGciOiJSUzI1NiJ9.eyJzdWIiOiJhIn0.signature",
		Error(fmt.Errorf("connect: %w", errors.New("password=dsn-secret token=inline-secret"))),
	)
	text := output.String()
	for _, secret := range []string{"configured-secret", "opaque-bearer", "urlpass", "signed-secret", "private-fragment", "plain-password", "nested-secret", "eyJhbGci", "dsn-secret", "inline-secret"} {
		if strings.Contains(text, secret) {
			t.Fatalf("secret %q leaked: %s", secret, text)
		}
	}
	if !strings.Contains(text, "https://example.test/path") || !strings.Contains(text, "healthy") || !json.Valid(bytes.TrimSpace(output.Bytes())) {
		t.Fatal(text)
	}
}

// TestErrorRedactionPrecedesTruncation prevents a boundary from preserving partial secrets.
func TestErrorRedactionPrecedesTruncation(t *testing.T) {
	var output bytes.Buffer
	secret := "configured-sensitive-value"
	logger, closeLog, err := New(Options{Format: "json", Console: &output, Secrets: []string{secret}})
	if err != nil {
		t.Fatal(err)
	}
	defer closeLog()
	logger.Error("operation failed", Error(errors.New(strings.Repeat("x", maxString-6)+secret)))
	if strings.Contains(output.String(), "config") || !strings.Contains(output.String(), "[REDAC") {
		t.Fatal("secret must be redacted before truncation")
	}
}

// TestRedactionCoversQuotedJSONCredentials protects JSON included in messages and error text.
func TestRedactionCoversQuotedJSONCredentials(t *testing.T) {
	var output bytes.Buffer
	logger, closeLog, err := New(Options{Format: "json", Console: &output})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = closeLog() })

	logger.Error(`upstream response: {"access_token":"message-secret"}`)
	logger.Error("upstream request failed", Error(errors.New(`upstream response: {"access_token":"error-secret"}`)))

	for _, secret := range []string{"message-secret", "error-secret"} {
		if strings.Contains(output.String(), secret) {
			t.Fatalf("secret %q leaked: %s", secret, output.String())
		}
	}
	if !strings.Contains(output.String(), "[REDACTED]") {
		t.Fatalf("redaction marker missing: %s", output.String())
	}
}

// TestErrorDiagnosticsKeepClassificationWithoutProviderOrDatabasePayloads avoids logging business data.
func TestErrorDiagnosticsKeepClassificationWithoutProviderOrDatabasePayloads(t *testing.T) {
	for _, tc := range []struct {
		err  error
		keep []string
		hide []string
	}{
		{fmt.Errorf("wrapped: %w", &smithy.GenericAPIError{Code: "AccessDenied", Message: "secret-provider-body"}), []string{"AccessDenied", "smithy.GenericAPIError"}, []string{"secret-provider-body"}},
		{&smithy.GenericAPIError{Code: "provider-private-code", Message: "private provider response"}, []string{"Unknown"}, []string{"provider-private-code", "private provider response"}},
		{fmt.Errorf("query: %w", &pgconn.PgError{Code: "23505", Message: "secret-table-value", Detail: "private-email", InternalQuery: "private-sql"}), []string{"23505", "pgconn.PgError"}, []string{"secret-table-value", "private-email", "private-sql"}},
		{&url.Error{Op: "Get", URL: "https://user:secret@example.test/path?token=private", Err: &net.DNSError{Err: "lookup unavailable", Name: "db.example.test", IsTimeout: true}}, []string{"timeout", "true", "lookup unavailable"}, []string{"user:secret", "token=private"}},
	} {
		var output bytes.Buffer
		logger, closeLog, err := New(Options{Format: "json", Console: &output})
		if err != nil {
			t.Fatal(err)
		}
		logger.Error("operation failed", Error(tc.err))
		_ = closeLog()
		for _, keep := range tc.keep {
			if !strings.Contains(output.String(), keep) {
				t.Fatalf("missing %q: %s", keep, output.String())
			}
		}
		for _, hide := range tc.hide {
			if strings.Contains(output.String(), hide) {
				t.Fatalf("leaked %q: %s", hide, output.String())
			}
		}
	}
}

type safeProviderDiagnostic struct{}

func (safeProviderDiagnostic) Error() string            { return "client_secret=private-value" }
func (safeProviderDiagnostic) SafeProviderCode() string { return "UnauthorizedOperation" }
func (safeProviderDiagnostic) HTTPStatusCode() int      { return 403 }
func (safeProviderDiagnostic) RequestID() string        { return "pages-req-123" }
func (safeProviderDiagnostic) ProviderAction() string   { return "CreatePagesDeployment" }

// TestSafeProviderDiagnosticsRetainActionAndRequestIdentity keeps cloud failures useful without logging their bodies.
func TestSafeProviderDiagnosticsRetainActionAndRequestIdentity(t *testing.T) {
	var output bytes.Buffer
	logger, closeLog, err := New(Options{Format: "json", Console: &output})
	if err != nil {
		t.Fatal(err)
	}
	defer closeLog()
	logger.Error("deployment failed", Error(safeProviderDiagnostic{}))

	var record map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(output.Bytes()), &record); err != nil {
		t.Fatal(err)
	}
	diagnostic, ok := record["error"].(map[string]any)
	if !ok || diagnostic["provider_code"] != "UnauthorizedOperation" || diagnostic["http_status"] != float64(403) || diagnostic["provider_request_id"] != "pages-req-123" || diagnostic["provider_action"] != "CreatePagesDeployment" {
		t.Fatalf("incomplete provider diagnostics: %#v", record)
	}
	if strings.Contains(output.String(), "private-value") {
		t.Fatalf("provider error message leaked: %s", output.String())
	}
}

type failingWriter struct{}

// Write simulates a disconnected console without exposing arbitrary writer errors.
func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("private-writer-error") }

// TestOutputsFailIndependentlyAndFileIsJSON verifies console failure never suppresses the durable copy.
func TestOutputsFailIndependentlyAndFileIsJSON(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "server.jsonl")
	var diagnostic bytes.Buffer
	logger, closeLog, err := New(Options{Console: failingWriter{}, Diagnostic: &diagnostic, File: filename})
	if err != nil {
		t.Fatal(err)
	}
	logger.Error("persist despite console failure")
	if err := closeLog(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filename)
	if err != nil || !json.Valid(bytes.TrimSpace(data)) || !bytes.Contains(data, []byte("persist despite")) {
		t.Fatalf("file=%s error=%v", data, err)
	}
	if !strings.Contains(diagnostic.String(), "console") || strings.Contains(diagnostic.String(), "private-writer-error") {
		t.Fatalf("diagnostic %q", diagnostic.String())
	}
	if _, _, err := New(Options{File: t.TempDir(), Console: io.Discard}); err == nil {
		t.Fatal("accepted a directory as log file")
	}
}

// TestExistingLogFilePermissionsAreRestricted closes the permissions gap left by OpenFile's creation mode.
func TestExistingLogFilePermissionsAreRestricted(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not expose Unix permission bits")
	}
	filename := filepath.Join(t.TempDir(), "server.jsonl")
	if err := os.WriteFile(filename, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filename, 0o644); err != nil {
		t.Fatal(err)
	}

	_, closeLog, err := New(Options{Console: io.Discard, File: filename})
	if err != nil {
		t.Fatal(err)
	}
	defer closeLog()

	info, err := os.Stat(filename)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("log file permissions = %#o, want 0600", got)
	}
}

// TestNewLogFilePermissionsAreRestricted protects files created by the rotation writer.
func TestNewLogFilePermissionsAreRestricted(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not expose Unix permission bits")
	}
	filename := filepath.Join(t.TempDir(), "server.jsonl")
	logger, closeLog, err := New(Options{Console: io.Discard, File: filename})
	if err != nil {
		t.Fatal(err)
	}
	logger.Info("permission check")
	if err := closeLog(); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(filename)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("log file permissions = %#o, want 0600", got)
	}
}

// TestConcurrentFileRotationAndBoundedRecords checks valid concurrent JSONL and bounded disk retention.
func TestConcurrentFileRotationAndBoundedRecords(t *testing.T) {
	directory := t.TempDir()
	filename := filepath.Join(directory, "server.jsonl")
	logger, closeLog, err := New(Options{Console: io.Discard, File: filename, MaxSizeMB: 1, MaxBackups: 2})
	if err != nil {
		t.Fatal(err)
	}
	var workers sync.WaitGroup
	for worker := range 8 {
		workers.Go(func() {
			for range 200 {
				logger.Info("concurrent", "worker", worker, "detail", strings.Repeat("x", 16_000))
			}
		})
	}
	workers.Wait()
	if err := closeLog(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		files, err := filepath.Glob(filepath.Join(directory, "server-*.jsonl"))
		if err != nil {
			t.Fatal(err)
		}
		if len(files) > 0 && len(files) <= 2 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("expected one or two backup files after rotation: %v", files)
		}
		time.Sleep(10 * time.Millisecond)
	}
	files, err := os.ReadDir(directory)
	if err != nil || len(files) > 3 {
		t.Fatalf("rotation files=%v error=%v", files, err)
	}
	for _, file := range files {
		filename := filepath.Join(directory, file.Name())
		info, err := file.Info()
		if err != nil {
			t.Fatal(err)
		}
		if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
			t.Fatalf("rotated log %q permissions = %#o, want 0600", filename, info.Mode().Perm())
		}
		data, err := os.Open(filename)
		if err != nil {
			t.Fatal(err)
		}
		scanner := bufio.NewScanner(data)
		for scanner.Scan() {
			if !json.Valid(scanner.Bytes()) || len(scanner.Bytes()) > 12_000 {
				t.Fatalf("invalid or unbounded log record: %d bytes", len(scanner.Bytes()))
			}
		}
		if err := scanner.Err(); err != nil {
			t.Fatal(err)
		}
		_ = data.Close()
	}
}

// TestBoundedNestedAndBoundAttributes caps a whole record, not just individual strings.
func TestBoundedNestedAndBoundAttributes(t *testing.T) {
	var output bytes.Buffer
	logger, closeLog, err := New(Options{Format: "json", Console: &output})
	if err != nil {
		t.Fatal(err)
	}
	defer closeLog()
	logger = logger.With("bound_a", strings.Repeat("a", 6000)).With("bound_b", strings.Repeat("b", 6000)).With("bound_c", strings.Repeat("c", 6000))
	nested := make(map[string]string, 200)
	for index := range 200 {
		nested[fmt.Sprintf("field_%03d", index)] = strings.Repeat("x", 1000)
	}
	logger.Info("bounded", "payload", nested)
	if output.Len() > maxRecordBytes+2048 {
		t.Fatalf("record exceeded byte budget: %d", output.Len())
	}
	if !strings.Contains(output.String(), "truncated") {
		t.Fatal("bounded output lacks truncation marker")
	}
}

// TestSensitiveWithGroupDoesNotLeakUngroupedAttributeKeys protects dynamically grouped slog fields.
func TestSensitiveWithGroupDoesNotLeakUngroupedAttributeKeys(t *testing.T) {
	var output bytes.Buffer
	logger, closeLog, err := New(Options{Format: "json", Console: &output})
	if err != nil {
		t.Fatal(err)
	}
	defer closeLog()
	logger.WithGroup("token").With("value", "opaque-token-value").Info("request")
	if strings.Contains(output.String(), "opaque-token-value") {
		t.Fatalf("secret group leaked value: %s", output.String())
	}
	if !strings.Contains(output.String(), "[REDACTED]") {
		t.Fatalf("redaction marker missing: %s", output.String())
	}
}

// TestFileFailureDoesNotSuppressConsoleOutput reports a broken file sink without recursive errors.
func TestFileFailureDoesNotSuppressConsoleOutput(t *testing.T) {
	var console, diagnostic bytes.Buffer
	logger := slog.New(&redactingHandler{next: &outputHandler{
		outputs:    []namedHandler{{name: "console", handler: slog.NewTextHandler(&console, nil)}, {name: "file", handler: slog.NewJSONHandler(failingWriter{}, nil)}},
		diagnostic: &diagnosticWriter{output: &diagnostic},
	}, redactor: newRedactor(nil)})
	logger.Error("console survives")
	if !strings.Contains(console.String(), "console survives") || !strings.Contains(diagnostic.String(), "file output failed") {
		t.Fatalf("console=%q diagnostic=%q", console.String(), diagnostic.String())
	}
}
