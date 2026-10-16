package pages

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// roundTripFunc keeps production URLs visible while routing tests to a real HTTP server.
type roundTripFunc func(*http.Request) (*http.Response, error)

// RoundTrip executes the injected transport operation.
func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// testClient exercises HTTP serialization without making live cloud calls.
func testClient(t *testing.T, region string, handler http.HandlerFunc) *Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	target, _ := url.Parse(server.URL)
	client, err := NewClient(Config{Token: "test-pages-token", Region: region, HTTPClient: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Scheme != "https" {
			t.Errorf("insecure production URL: %s", r.URL)
		}
		if r.GetBody != nil || r.Header.Get("Idempotency-Key") != "" {
			t.Error("request permits transparent write replay")
		}
		clone := r.Clone(r.Context())
		clone.Header.Set("X-Test-Original-Host", r.URL.Host)
		clone.URL.Scheme, clone.URL.Host = target.Scheme, target.Host
		return http.DefaultTransport.RoundTrip(clone)
	})}})
	if err != nil {
		t.Fatal(err)
	}
	return client
}

// reply preserves the official API envelope, including its explicit success code.
func reply(w http.ResponseWriter, payload any) {
	_ = json.NewEncoder(w).Encode(map[string]any{"Code": 0, "Data": map[string]any{"Response": payload}})
}

// readAction verifies authenticated Pages JSON rather than a mocked SDK method.
func readAction(t *testing.T, r *http.Request) map[string]any {
	t.Helper()
	if r.Method != http.MethodPost || r.URL.Path != "/v1" || r.Header.Get("Authorization") != "Bearer test-pages-token" || r.Header.Get("Content-Type") != "application/json" {
		t.Errorf("invalid API request: %s %s", r.Method, r.URL.Path)
	}
	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		t.Error(err)
	}
	return body
}

// credentials models the complete fields consumed from DescribePagesCosTempToken.
func credentials() map[string]any {
	return map[string]any{"Bucket": "pages-upload-1234567890", "Region": "ap-singapore", "TargetPath": "/project/release/", "Credentials": map[string]any{"TmpSecretId": "temp-id", "TmpSecretKey": "temp-secret", "Token": "temp-token"}}
}

// TestDeploySendsOfficialProtocol checks the actual JSON actions, COS body, and ZIP entries.
func TestDeploySendsOfficialProtocol(t *testing.T) {
	files := map[string][]byte{"index.html": []byte("<html>real content</html>"), "assets/app.js": []byte("console.log('ok')")}
	var actions []string
	client := testClient(t, "china", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			if r.Header.Get("X-Test-Original-Host") != "pages-upload-1234567890.cos.accelerate.myqcloud.com" || r.URL.Path != "/project/release/artifact.zip" || r.Header.Get("Content-Type") != "application/zip" || r.Header.Get("X-Cos-Security-Token") != "temp-token" || !strings.HasPrefix(r.Header.Get("Authorization"), "q-sign-algorithm=sha1&q-ak=temp-id&") || r.Header.Get("Content-MD5") == "" {
				t.Error("invalid COS upload")
			}
			data, _ := io.ReadAll(r.Body)
			archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
			if err != nil {
				t.Error(err)
				return
			}
			got := map[string][]byte{}
			for _, entry := range archive.File {
				file, err := entry.Open()
				if err != nil {
					t.Error(err)
					return
				}
				content, _ := io.ReadAll(file)
				_ = file.Close()
				got[entry.Name] = content
			}
			if !reflect.DeepEqual(files, got) {
				t.Errorf("ZIP differs: %v", got)
			}
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.Header.Get("X-Test-Original-Host") != "pages-api.cloud.tencent.com" {
			t.Error("incorrect China endpoint")
		}
		body := readAction(t, r)
		action, _ := body["Action"].(string)
		actions = append(actions, action)
		switch action {
		case "DescribePagesProjects":
			want := map[string]any{"Action": action, "Filters": []any{map[string]any{"Name": "ProjectId", "Values": []any{"project-1"}}}, "Offset": float64(0), "Limit": float64(1)}
			if !reflect.DeepEqual(body, want) {
				t.Errorf("incorrect project request: %v", body)
			}
			reply(w, map[string]any{"Projects": []any{map[string]any{"ProjectId": "project-1", "Provider": "Upload"}}})
		case "DescribePagesCosTempToken":
			if !reflect.DeepEqual(body, map[string]any{"Action": action, "ProjectId": "project-1"}) {
				t.Error("incorrect credential request")
			}
			reply(w, credentials())
		case "CreatePagesDeployment":
			want := map[string]any{"Action": action, "ProjectId": "project-1", "Env": "Production", "ViaMeta": "Upload", "Provider": "Upload", "DistType": "Zip", "TempBucketPath": "project/release/artifact.zip"}
			if !reflect.DeepEqual(body, want) {
				t.Errorf("incorrect creation body: %v", body)
			}
			reply(w, map[string]any{"DeploymentId": "deploy-1"})
		default:
			t.Errorf("unexpected action %s", action)
		}
	})
	got, err := client.Deploy(context.Background(), "project-1", files)
	if err != nil {
		t.Fatal(err)
	}
	if got != (Deployment{ID: "deploy-1", ProjectID: "project-1", Environment: "Production", Status: "building"}) {
		t.Fatalf("unexpected deployment: %+v", got)
	}
	if !reflect.DeepEqual(actions, []string{"DescribePagesProjects", "DescribePagesCosTempToken", "CreatePagesDeployment"}) {
		t.Fatalf("wrong actions: %v", actions)
	}
}

// TestGetValidatesCloudIdentityAndStatus rejects ambiguous provider results.
func TestGetValidatesCloudIdentityAndStatus(t *testing.T) {
	for _, tc := range []struct{ name, status, project, id, env, want string }{
		{"pending", "Pending", "project-1", "deploy-1", "Production", "building"}, {"processing", "Process", "project-1", "deploy-1", "Production", "building"}, {"success", "Success", "project-1", "deploy-1", "Production", "succeeded"}, {"failed", "Failed", "project-1", "deploy-1", "Production", "failed"}, {"timeout", "Timeout", "project-1", "deploy-1", "Production", "failed"}, {"cancelled", "Cancelled", "project-1", "deploy-1", "Production", "failed"}, {"invalid", "Invalid", "project-1", "deploy-1", "Production", "failed"}, {"unknown", "FutureState", "project-1", "deploy-1", "Production", ""}, {"wrong project", "Success", "other", "deploy-1", "Production", ""}, {"wrong ID", "Success", "project-1", "other", "Production", ""}, {"preview", "Success", "project-1", "deploy-1", "Preview", ""}, {"missing env", "Success", "project-1", "deploy-1", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := testClient(t, "global", func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("X-Test-Original-Host") != "pages-api.edgeone.ai" {
					t.Error("incorrect global endpoint")
				}
				body := readAction(t, r)
				want := map[string]any{"Action": "DescribePagesDeployments", "ProjectId": "project-1", "DeploymentIds": []any{"deploy-1"}, "Offset": float64(0), "Limit": float64(1)}
				if !reflect.DeepEqual(body, want) {
					t.Errorf("incorrect query: %v", body)
				}
				reply(w, map[string]any{"Deployments": []any{map[string]any{"DeploymentId": tc.id, "ProjectId": tc.project, "Env": tc.env, "Status": tc.status}}})
			})
			got, err := client.Get(context.Background(), "project-1", "deploy-1")
			if tc.want == "" {
				if err == nil {
					t.Fatalf("accepted invalid deployment: %+v", got)
				}
				return
			}
			if err != nil || got.Status != tc.want {
				t.Fatalf("got %+v, %v", got, err)
			}
		})
	}
}

// TestNewClientRejectsInvalidConfig prevents invalid headers or inferred cloud regions.
func TestNewClientRejectsInvalidConfig(t *testing.T) {
	for _, config := range []Config{{Region: "china"}, {Token: " token", Region: "china"}, {Token: "token\r\nInjected: x", Region: "china"}, {Token: "token", Region: "auto"}, {Token: "token"}, {Token: "token", Region: "https://evil.test"}} {
		if _, err := NewClient(config); err == nil {
			t.Errorf("accepted invalid config")
		}
	}
}

// TestCloudErrorsAreBoundedAndRedacted rejects untrusted envelopes without echoing secrets.
func TestCloudErrorsAreBoundedAndRedacted(t *testing.T) {
	for _, body := range []string{`{"Code":105,"Message":"test-pages-token"}`, `{"Code":0,"Data":{"Response":{"Error":{"Code":"Failure","Message":"temp-secret"}}}}`, `{"Data":{"Response":{}}}`, `{"Code":false,"Data":{"Response":{}}}`, `{"Code":0}`, `not JSON`, strings.Repeat("x", (1<<20)+1)} {
		t.Run(body[:min(len(body), 20)], func(t *testing.T) {
			client := testClient(t, "china", func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, body) })
			_, err := client.Get(context.Background(), "project-1", "deploy-1")
			if err == nil || strings.Contains(err.Error(), "test-pages-token") || strings.Contains(err.Error(), "temp-secret") {
				t.Fatalf("unsafe error: %v", err)
			}
		})
	}
}

// TestWritesNeverRetryOrFollowRedirects preserves uncertain deployment outcomes.
func TestWritesNeverRetryOrFollowRedirects(t *testing.T) {
	for _, status := range []int{http.StatusServiceUnavailable, http.StatusTemporaryRedirect, http.StatusUnauthorized} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			var creates atomic.Int32
			client := testClient(t, "china", func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPut {
					w.WriteHeader(200)
					return
				}
				body := readAction(t, r)
				switch body["Action"] {
				case "DescribePagesProjects":
					reply(w, map[string]any{"Projects": []any{map[string]any{"ProjectId": "project-1", "Provider": "Upload"}}})
				case "DescribePagesCosTempToken":
					reply(w, credentials())
				case "CreatePagesDeployment":
					creates.Add(1)
					w.Header().Set("Location", "https://attacker.test/")
					w.WriteHeader(status)
				default:
					t.Error("unexpected redirected action")
				}
			})
			_, err := client.Deploy(context.Background(), "project-1", map[string][]byte{"index.html": []byte("ok")})
			if err == nil || creates.Load() != 1 {
				t.Fatalf("error=%v creates=%d", err, creates.Load())
			}
		})
	}
}

// TestCancellationStopsCloudRequests preserves cancellation for durable caller decisions.
func TestCancellationStopsCloudRequests(t *testing.T) {
	var calls atomic.Int32
	client := testClient(t, "china", func(w http.ResponseWriter, r *http.Request) { calls.Add(1) })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := client.Get(ctx, "project-1", "deploy-1")
	if !errors.Is(err, context.Canceled) || calls.Load() != 0 {
		t.Fatalf("error=%v calls=%d", err, calls.Load())
	}
}

// TestUnsafeFilesNeverReachCloud checks traversal, ambiguous paths and size limits.
func TestUnsafeFilesNeverReachCloud(t *testing.T) {
	for _, name := range []string{"", "../index.html", "/index.html", "a/../b", "a//b", "a\\b", "C:/file", "a\x00b", "./index.html", "a/"} {
		t.Run(name, func(t *testing.T) {
			var calls atomic.Int32
			client := testClient(t, "china", func(w http.ResponseWriter, r *http.Request) { calls.Add(1) })
			_, err := client.Deploy(context.Background(), "project-1", map[string][]byte{name: []byte("bad")})
			if err == nil || calls.Load() != 0 {
				t.Fatalf("error=%v calls=%d", err, calls.Load())
			}
		})
	}
}

// TestInvalidUploadTargetsNeverReceiveCredentials blocks malformed cloud-supplied destinations.
func TestInvalidUploadTargetsNeverReceiveCredentials(t *testing.T) {
	for _, tc := range []struct {
		name, key string
		value     any
	}{
		{"missing secret", "Credentials", map[string]any{"TmpSecretId": "temp-id", "Token": "temp-token"}},
		{"invalid secret ID", "Credentials", map[string]any{"TmpSecretId": "id&injected=x", "TmpSecretKey": "secret", "Token": "token"}},
		{"invalid token", "Credentials", map[string]any{"TmpSecretId": "id", "TmpSecretKey": "secret", "Token": "token\n"}},
		{"host injection", "Bucket", "bucket.evil.test"}, {"invalid region", "Region", ""}, {"traversal", "TargetPath", "/../escape"}, {"empty path", "TargetPath", "/"}, {"query injection", "TargetPath", "/path?token=secret"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var writes atomic.Int32
			client := testClient(t, "china", func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPut {
					writes.Add(1)
					return
				}
				body := readAction(t, r)
				if body["Action"] == "DescribePagesProjects" {
					reply(w, map[string]any{"Projects": []any{map[string]any{"ProjectId": "project-1", "Provider": "Upload"}}})
					return
				}
				if body["Action"] != "DescribePagesCosTempToken" {
					writes.Add(1)
				}
				payload := credentials()
				payload[tc.key] = tc.value
				reply(w, payload)
			})
			_, err := client.Deploy(context.Background(), "project-1", map[string][]byte{"index.html": []byte("ok")})
			if err == nil || writes.Load() != 0 {
				t.Fatalf("error=%v writes=%d", err, writes.Load())
			}
		})
	}
}

// TestWrongProjectOrProviderStopsBeforeUpload prevents deploying to an unverified project.
func TestWrongProjectOrProviderStopsBeforeUpload(t *testing.T) {
	for _, record := range []map[string]any{{"ProjectId": "other", "Provider": "Upload"}, {"ProjectId": "project-1", "Provider": "GitHub"}, {"ProjectId": "project-1"}} {
		var calls atomic.Int32
		client := testClient(t, "china", func(w http.ResponseWriter, r *http.Request) {
			calls.Add(1)
			reply(w, map[string]any{"Projects": []any{record}})
		})
		_, err := client.Deploy(context.Background(), "project-1", map[string][]byte{"index.html": []byte("ok")})
		if err == nil || calls.Load() != 1 {
			t.Fatalf("error=%v calls=%d", err, calls.Load())
		}
	}
}

// TestLimitsAndIdentifiersRejectBeforeRequests bounds allocation and prevents malformed identities.
func TestLimitsAndIdentifiersRejectBeforeRequests(t *testing.T) {
	var calls atomic.Int32
	client := testClient(t, "china", func(w http.ResponseWriter, r *http.Request) { calls.Add(1) })
	for _, files := range []map[string][]byte{nil, {}, {"huge": make([]byte, (64<<20)+1)}} {
		if _, err := client.Deploy(context.Background(), "project-1", files); err == nil {
			t.Error("accepted invalid artifact")
		}
	}
	for _, id := range []string{"", " ", "project\n1", strings.Repeat("p", 257), "../../project"} {
		if _, err := client.Get(context.Background(), id, "deploy-1"); err == nil {
			t.Error("accepted invalid project")
		}
		if _, err := client.Get(context.Background(), "project-1", id); err == nil {
			t.Error("accepted invalid deployment")
		}
	}
	if calls.Load() != 0 {
		t.Error("invalid request reached cloud")
	}
}

// TestCOSSignatureMatchesOfficialSDK locks the signature to a separately generated SDK vector.
func TestCOSSignatureMatchesOfficialSDK(t *testing.T) {
	req, err := http.NewRequest(http.MethodPut, "https://pages-upload-1234567890.cos.accelerate.myqcloud.com/project/release/artifact.zip", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/zip")
	req.Header.Set("Content-MD5", "abc+/=")
	req.Header.Set("X-Cos-Security-Token", "temp-token")
	want := "q-sign-algorithm=sha1&q-ak=temp-id&q-sign-time=1699999999;1700000299&q-key-time=1699999999;1700000299&q-header-list=content-md5;content-type;host;x-cos-security-token&q-url-param-list=&q-signature=ec068dfe86cc97fd0ccaf16c98c502745d103f6a"
	if got := cosAuthorization(req, "temp-id", "temp-secret", time.Unix(1700000000, 0)); got != want {
		t.Fatalf("signature differs from official SDK: %s", got)
	}
}

// TestUploadAndCreateFailuresStopWithoutReplay tests rejection at each cloud write boundary.
func TestUploadAndCreateFailuresStopWithoutReplay(t *testing.T) {
	for _, stage := range []string{"credentials", "upload", "create ID", "create project", "create env", "create disconnected"} {
		t.Run(stage, func(t *testing.T) {
			var uploads, creates atomic.Int32
			client := testClient(t, "china", func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPut {
					uploads.Add(1)
					if stage == "upload" {
						w.WriteHeader(403)
						_, _ = io.WriteString(w, "temp-secret")
						return
					}
					w.WriteHeader(200)
					return
				}
				body := readAction(t, r)
				switch body["Action"] {
				case "DescribePagesProjects":
					reply(w, map[string]any{"Projects": []any{map[string]any{"ProjectId": "project-1", "Provider": "Upload"}}})
				case "DescribePagesCosTempToken":
					if stage == "credentials" {
						reply(w, map[string]any{"Error": map[string]any{"Code": "Denied", "Message": "temp-secret"}})
						return
					}
					reply(w, credentials())
				case "CreatePagesDeployment":
					creates.Add(1)
					switch stage {
					case "create ID":
						reply(w, map[string]any{"DeploymentId": ""})
					case "create project":
						reply(w, map[string]any{"DeploymentId": "deploy-1", "ProjectId": "other"})
					case "create env":
						reply(w, map[string]any{"DeploymentId": "deploy-1", "Env": "Preview"})
					case "create disconnected":
						connection, _, err := w.(http.Hijacker).Hijack()
						if err != nil {
							t.Error(err)
							return
						}
						_ = connection.Close()
					default:
						t.Error("unexpected create")
					}
				}
			})
			_, err := client.Deploy(context.Background(), "project-1", map[string][]byte{"index.html": []byte("ok")})
			if err == nil || strings.Contains(err.Error(), "temp-secret") {
				t.Fatalf("unsafe error: %v", err)
			}
			wantCreates, wantUploads := int32(1), int32(1)
			if stage == "credentials" {
				wantCreates, wantUploads = 0, 0
			}
			if stage == "upload" {
				wantCreates = 0
			}
			if creates.Load() != wantCreates || uploads.Load() != wantUploads {
				t.Fatalf("creates=%d uploads=%d", creates.Load(), uploads.Load())
			}
		})
	}
}

// TestGetSingleResponseAndMissingDeployment supports the SDK's singular response variant safely.
func TestGetSingleResponseAndMissingDeployment(t *testing.T) {
	for _, payload := range []map[string]any{{"Deployment": map[string]any{"DeploymentId": "deploy-1", "ProjectId": "project-1", "Env": "Production", "Status": "Success"}}, {"Deployments": []any{}}, {"Deployments": []any{map[string]any{}, map[string]any{}}}} {
		client := testClient(t, "global", func(w http.ResponseWriter, r *http.Request) { reply(w, payload) })
		got, err := client.Get(context.Background(), "project-1", "deploy-1")
		if payload["Deployment"] != nil {
			if err != nil || got.Status != "succeeded" {
				t.Fatalf("got %+v, %v", got, err)
			}
		} else if err == nil {
			t.Error("accepted missing or ambiguous deployment")
		}
	}
}

// TestNetworkErrorsRedactTransportDetails protects secrets included by arbitrary transports.
func TestNetworkErrorsRedactTransportDetails(t *testing.T) {
	client, err := NewClient(Config{Token: "test-pages-token", Region: "china", HTTPClient: &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return nil, errors.New("test-pages-token temp-secret https://private.test")
	})}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Get(context.Background(), "project-1", "deploy-1")
	if err == nil || strings.Contains(err.Error(), "test-pages-token") || strings.Contains(err.Error(), "private.test") {
		t.Fatalf("unsafe error: %v", err)
	}
}

// TestInFlightCancellationAndTimeout abort active HTTP work instead of holding publisher locks.
func TestInFlightCancellationAndTimeout(t *testing.T) {
	for _, timeout := range []bool{false, true} {
		t.Run(map[bool]string{false: "cancel", true: "deadline"}[timeout], func(t *testing.T) {
			entered := make(chan struct{})
			client := testClient(t, "china", func(w http.ResponseWriter, r *http.Request) {
				close(entered)
				_, _ = io.Copy(io.Discard, r.Body)
				<-r.Context().Done()
			})
			ctx, cancel := context.WithCancel(context.Background())
			if timeout {
				ctx, cancel = context.WithTimeout(context.Background(), 100*time.Millisecond)
			}
			defer cancel()
			result := make(chan error, 1)
			go func() { _, err := client.Get(ctx, "project-1", "deploy-1"); result <- err }()
			select {
			case <-entered:
			case <-time.After(time.Second):
				t.Fatal("request did not start")
			}
			if !timeout {
				cancel()
			}
			select {
			case err := <-result:
				want := context.Canceled
				if timeout {
					want = context.DeadlineExceeded
				}
				if !errors.Is(err, want) {
					t.Fatalf("got %v want %v", err, want)
				}
			case <-time.After(time.Second):
				t.Fatal("request did not abort")
			}
		})
	}
}

// TestClientBoundsInjectedTimeoutAndRejectsRedirects keeps the caller's client unmodified.
func TestClientBoundsInjectedTimeoutAndRejectsRedirects(t *testing.T) {
	for _, timeout := range []time.Duration{0, time.Hour, time.Second} {
		original := &http.Client{Timeout: timeout}
		client, err := NewClient(Config{Token: "token", Region: "china", HTTPClient: original})
		if err != nil {
			t.Fatal(err)
		}
		if client.http.Timeout <= 0 || client.http.Timeout > 30*time.Second {
			t.Fatal("unbounded request timeout")
		}
		if original.Timeout != timeout || original.CheckRedirect != nil {
			t.Fatal("mutated caller client")
		}
		if !errors.Is(client.http.CheckRedirect(nil, nil), http.ErrUseLastResponse) {
			t.Fatal("redirect permitted")
		}
	}
	if _, err := NewClient(Config{Token: "token", Region: "china"}); err != nil {
		t.Fatal(err)
	}
}

// TestArtifactRejectsFileDirectoryConflicts prevents ZIP extraction from replacing a parent file.
func TestArtifactRejectsFileDirectoryConflicts(t *testing.T) {
	_, err := prepareArchive(context.Background(), map[string][]byte{"assets": []byte("file"), "assets/app.js": []byte("script")})
	if err == nil {
		t.Fatal("accepted file/directory conflict")
	}
}

// TestArchiveIsDeterministicAndBoundsFileCount checks reproducible ZIPs and bounded metadata.
func TestArchiveIsDeterministicAndBoundsFileCount(t *testing.T) {
	files := map[string][]byte{"z.txt": []byte("z"), "a.txt": []byte("a")}
	first, err := prepareArchive(context.Background(), files)
	if err != nil {
		t.Fatal(err)
	}
	second, err := prepareArchive(context.Background(), files)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatal("archive is not deterministic")
	}
	tooMany := map[string][]byte{}
	for i := 0; i <= maxArtifactFiles; i++ {
		tooMany[strings.Repeat("a", i/100)+string(rune(0x100+i))] = nil
	}
	if _, err := prepareArchive(context.Background(), tooMany); err == nil {
		t.Fatal("accepted excessive file count")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := prepareArchive(ctx, files); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation lost: %v", err)
	}
}
