package s3

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"yujian.me/server/internal/domain"
	"yujian.me/server/internal/ports"
)

// TestObjectMissingClassificationDistinguishesBucketAndWriteFailures exercises
// the SDK response decoder, including bodyless HEAD failures from S3 providers.
func TestObjectMissingClassificationDistinguishesBucketAndWriteFailures(t *testing.T) {
	for _, test := range []struct {
		name, method, code string
		missingObject      bool
	}{
		{"missing HEAD object", http.MethodHead, "", true},
		{"missing GET object", http.MethodGet, "NoSuchKey", true},
		{"missing GET bucket", http.MethodGet, "NoSuchBucket", false},
		{"missing PUT bucket", http.MethodPut, "NoSuchBucket", false},
		{"unclassified PUT endpoint", http.MethodPut, "", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.Method != test.method {
					t.Errorf("unexpected storage method %s", request.Method)
				}
				writer.Header().Set("Content-Type", "application/xml")
				writer.WriteHeader(http.StatusNotFound)
				if test.code != "" {
					_, _ = io.WriteString(writer, "<Error><Code>"+test.code+"</Code><Message>private provider detail</Message></Error>")
				}
			}))
			defer server.Close()
			store, err := NewBlobStore(Config{Endpoint: server.URL, PublicBaseURL: "https://media.example.test", Region: "test", Bucket: "media", AccessKeyID: "test", SecretAccessKey: "test", UsePathStyle: true, HTTPClient: server.Client()})
			if err != nil {
				t.Fatal(err)
			}
			switch test.method {
			case http.MethodHead:
				_, err = store.Stat(context.Background(), "snapshots/rel_b73feb78-c751-4ccb-be54-8a8039247ca0/content.json")
			case http.MethodGet:
				_, err = store.ReadSnapshot(context.Background(), "snapshots/rel_b73feb78-c751-4ccb-be54-8a8039247ca0/content.json")
			case http.MethodPut:
				err = store.Put(context.Background(), "snapshots/rel_b73feb78-c751-4ccb-be54-8a8039247ca0/content.json", bytes.NewReader([]byte("{}")), ports.BlobMetadata{ContentType: "application/json", Size: 2, Checksum: "sha256:44136fa355b3678a1146ad16f7e8649e94fb4fc21fe77e8310c060f61caaff8a"})
			}
			if err == nil || errors.Is(err, domain.ErrNotFound) != test.missingObject {
				t.Fatalf("missing-object classification = %v, want %v (error=%v)", errors.Is(err, domain.ErrNotFound), test.missingObject, err)
			}
		})
	}
}
