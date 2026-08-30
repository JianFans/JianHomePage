package httpapi

import (
	"encoding/json"
	"os"
	"strings"
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
			Schemas map[string]struct {
				Required   []string `json:"required"`
				Properties map[string]struct {
					Ref         string   `json:"$ref"`
					Enum        []string `json:"enum"`
					Description string   `json:"description"`
					OneOf       []struct {
						Format  string `json:"format"`
						Pattern string `json:"pattern"`
					} `json:"oneOf"`
				} `json:"properties"`
			} `json:"schemas"`
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
	contentTypes := document.Components.Schemas["AssetUploadRequest"].Properties["contentType"].Enum
	if len(contentTypes) != 5 {
		t.Fatalf("asset upload MIME enum is not aligned with snapshot schema: %#v", contentTypes)
	}
	rightsReference := document.Components.Schemas["AssetUploadRequest"].Properties["rights"].Ref
	if rightsReference != "#/components/schemas/AssetRights" {
		t.Fatalf("asset upload rights are not bound to the snapshot contract: %q", rightsReference)
	}
	rightsRequired := document.Components.Schemas["AssetRights"].Required
	if len(rightsRequired) != 1 || rightsRequired[0] != "source" {
		t.Fatalf("asset rights must require source: %#v", rightsRequired)
	}
	assetSchema := document.Components.Schemas["Asset"]
	if !containsString(assetSchema.Required, "src") {
		t.Fatalf("asset response must require src: %#v", assetSchema.Required)
	}
	source := assetSchema.Properties["src"]
	if !strings.Contains(source.Description, "稳定") || len(source.OneOf) != 2 ||
		source.OneOf[0].Format != "uri" || source.OneOf[0].Pattern != "^https://" ||
		source.OneOf[1].Pattern != "^/media/" {
		t.Fatalf("asset src must document the stable HTTPS or local media contract: %#v", source)
	}
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

func containsString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}
