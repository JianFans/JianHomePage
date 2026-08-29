package httpapi

import (
	"encoding/json"
	"os"
	"testing"
)

func TestOpenAPIContainsManagementOperationsAndSecurity(t *testing.T) {
	raw, err := os.ReadFile("../../../../packages/schema/openapi/admin.yaml")
	if err != nil {
		t.Fatalf("read OpenAPI: %v", err)
	}
	var document struct {
		OpenAPI string `json:"openapi"`
		Paths   map[string]map[string]struct {
			OperationID string                `json:"operationId"`
			Security    []map[string][]string `json:"security"`
			Parameters  []struct {
				Name     string `json:"name"`
				Required bool   `json:"required"`
			} `json:"parameters"`
		} `json:"paths"`
		Components struct {
			SecuritySchemes map[string]struct {
				Type   string `json:"type"`
				Scheme string `json:"scheme"`
			} `json:"securitySchemes"`
		} `json:"components"`
	}
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatalf("OpenAPI source must remain JSON-compatible YAML: %v", err)
	}
	if document.OpenAPI != "3.1.0" {
		t.Fatalf("unexpected OpenAPI version %q", document.OpenAPI)
	}

	expected := map[string]string{
		"GET /healthz":                               "getHealth",
		"POST /api/v1/versions":                      "createVersion",
		"GET /api/v1/versions/{versionId}":           "getVersion",
		"PUT /api/v1/versions/{versionId}":           "updateVersion",
		"POST /api/v1/versions/{versionId}/review":   "submitReview",
		"POST /api/v1/versions/{versionId}/approve":  "approveReview",
		"POST /api/v1/versions/{versionId}/reject":   "rejectReview",
		"POST /api/v1/assets/uploads":                "createAssetUpload",
		"POST /api/v1/assets/{assetId}/complete":     "completeAssetUpload",
		"DELETE /api/v1/assets/{assetId}":            "deleteAsset",
		"POST /api/v1/publishes":                     "createPublish",
		"GET /api/v1/publishes/{publishId}":          "getPublish",
		"POST /api/v1/publishes/{publishId}/refresh": "refreshPublish",
		"POST /api/v1/rollbacks":                     "createRollback",
	}
	for key, operationID := range expected {
		method, path := splitOperation(t, key)
		operation, ok := document.Paths[path][method]
		if !ok || operation.OperationID != operationID {
			t.Fatalf("%s: got operation %#v", key, operation)
		}
		if path != "/healthz" && len(operation.Security) == 0 {
			t.Fatalf("%s must require bearer authentication", key)
		}
	}

	bearer := document.Components.SecuritySchemes["bearerAuth"]
	if bearer.Type != "http" || bearer.Scheme != "bearer" {
		t.Fatalf("unexpected bearer scheme %#v", bearer)
	}
	assertRequiredHeader(t, document.Paths["/api/v1/versions/{versionId}"]["put"].Parameters, "If-Match")
	assertRequiredHeader(t, document.Paths["/api/v1/publishes"]["post"].Parameters, "Idempotency-Key")
	assertRequiredHeader(t, document.Paths["/api/v1/rollbacks"]["post"].Parameters, "Idempotency-Key")
}

func TestOpenAPISourceIsSyncedIntoServerModule(t *testing.T) {
	source, err := os.ReadFile("../../../../packages/schema/openapi/admin.yaml")
	if err != nil {
		t.Fatalf("read source OpenAPI: %v", err)
	}
	synced, err := os.ReadFile("openapi.yaml")
	if err != nil {
		t.Fatalf("read synced OpenAPI: %v", err)
	}
	if string(source) != string(synced) {
		t.Fatal("server OpenAPI copy is stale; run go generate ./...")
	}
}

func splitOperation(t *testing.T, value string) (string, string) {
	t.Helper()
	for index, character := range value {
		if character == ' ' {
			method := value[:index]
			path := value[index+1:]
			for _, pair := range []struct{ from, to string }{
				{"GET", "get"}, {"POST", "post"}, {"PUT", "put"}, {"DELETE", "delete"},
			} {
				if method == pair.from {
					return pair.to, path
				}
			}
		}
	}
	t.Fatalf("invalid operation %q", value)
	return "", ""
}

func assertRequiredHeader(t *testing.T, parameters []struct {
	Name     string `json:"name"`
	Required bool   `json:"required"`
}, name string) {
	t.Helper()
	for _, parameter := range parameters {
		if parameter.Name == name && parameter.Required {
			return
		}
	}
	t.Fatalf("missing required header %s", name)
}
