package httpapi

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// TestOpenAPIContainsManagementOperationsAndSecurity checks route identifiers,
// bearer requirements and the documented asset query, rights and URL contracts.
func TestOpenAPIContainsManagementOperationsAndSecurity(t *testing.T) {
	raw, err := os.ReadFile("../../../../packages/schema/openapi/admin.yaml")
	if err != nil {
		t.Fatalf("read OpenAPI: %v", err)
	}
	var document struct {
		OpenAPI string `json:"openapi"`
		Servers []struct {
			URL string `json:"url"`
		} `json:"servers"`
		Paths map[string]map[string]struct {
			OperationID string                `json:"operationId"`
			Security    []map[string][]string `json:"security"`
			RequestBody struct {
				Content map[string]struct {
					Schema struct {
						Ref string `json:"$ref"`
					} `json:"schema"`
				} `json:"content"`
			} `json:"requestBody"`
			Parameters []struct {
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
					Format      string   `json:"format"`
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
	if len(document.Servers) != 1 || document.Servers[0].URL != "/" {
		t.Fatal("management OpenAPI must use the same origin as the embedded application")
	}
	for path, methods := range document.Paths {
		for method, operation := range methods {
			for _, requirement := range operation.Security {
				for scheme := range requirement {
					if _, ok := document.Components.SecuritySchemes[scheme]; !ok {
						t.Fatalf("%s %s references undefined security scheme %q", method, path, scheme)
					}
				}
			}
		}
	}

	expected := map[string]string{
		"GET /healthz":                                     "getHealth",
		"POST /api/v1/versions":                            "createVersion",
		"GET /api/v1/versions":                             "listVersions",
		"POST /api/v1/versions/{versionId}/clone":          "cloneVersion",
		"GET /api/v1/versions/{versionId}":                 "getVersion",
		"PUT /api/v1/versions/{versionId}":                 "updateVersion",
		"POST /api/v1/versions/{versionId}/review":         "submitReview",
		"POST /api/v1/versions/{versionId}/approve":        "approveReview",
		"POST /api/v1/versions/{versionId}/reject":         "rejectReview",
		"GET /api/v1/assets":                               "listAssets",
		"POST /api/v1/assets/uploads":                      "createAssetUpload",
		"POST /api/v1/assets/{assetId}/complete":           "completeAssetUpload",
		"DELETE /api/v1/assets/{assetId}":                  "deleteAsset",
		"POST /api/v1/publishes":                           "createPublish",
		"GET /api/v1/publishes/{publishId}":                "getPublish",
		"POST /api/v1/publishes/{publishId}/refresh":       "refreshPublish",
		"POST /api/v1/rollbacks":                           "createRollback",
		"GET /api/v1/publisher/builds/{buildId}":           "inspectPublisherBuild",
		"POST /api/v1/publisher/builds/{buildId}/recovery": "recoverPublisherBuild",
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
	assertOptionalParameters(t, document.Paths["/api/v1/assets"]["get"].Parameters, "status", "limit", "cursor")
	assertOptionalParameters(t, document.Paths["/api/v1/versions"]["get"].Parameters, "limit", "cursor")
	for path, operationID := range map[string]string{"/api/v1/auth/session": "getSession", "/api/v1/auth/login": "startLogin", "/api/v1/auth/callback": "completeLogin"} {
		operation := document.Paths[path]["get"]
		if operation.OperationID != operationID || len(operation.Security) != 0 {
			t.Fatalf("public auth contract %s %#v", path, operation)
		}
	}
	if document.Paths["/api/v1/auth/logout"]["post"].OperationID != "logout" {
		t.Fatal("logout contract missing")
	}
	if _, exists := document.Components.Schemas["VersionSummary"].Properties["snapshot"]; exists {
		t.Fatal("summary schema must omit snapshots")
	}
	for _, schema := range []string{"VersionSummary", "ContentVersion"} {
		for _, field := range []string{"createdAt", "updatedAt"} {
			if document.Components.Schemas[schema].Properties[field].Format != "date-time" {
				t.Fatalf("%s.%s must define a date-time property", schema, field)
			}
		}
	}
	if !containsString(document.Components.Schemas["AdminSession"].Required, "loginAvailable") {
		t.Fatal("session availability contract missing")
	}
	buildSchema := document.Components.Schemas["PublisherBuild"]
	if containsString(buildSchema.Required, "idempotencyKey") {
		t.Fatal("publisher responses do not expose the internal idempotency key")
	}
	if _, exists := buildSchema.Properties["idempotencyKey"]; exists {
		t.Fatal("publisher response properties must omit the internal idempotency key")
	}
	if _, exists := document.Components.Schemas["VersionWrite"].Properties["name"]; exists {
		t.Fatal("update schema must reject display names")
	}
	if _, exists := document.Components.Schemas["VersionCreate"].Properties["name"]; !exists {
		t.Fatal("create schema must accept display names")
	}
	if document.Paths["/api/v1/versions"]["post"].RequestBody.Content["application/json"].Schema.Ref != "#/components/schemas/VersionCreate" || document.Paths["/api/v1/versions/{versionId}"]["put"].RequestBody.Content["application/json"].Schema.Ref != "#/components/schemas/VersionWrite" {
		t.Fatal("create and update must use separate request schemas")
	}
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
	if assetSchema.Properties["createdAt"].Format != "date-time" || assetSchema.Properties["deletedAt"].Format != "date-time" {
		t.Fatalf("asset timestamps must be documented: %#v", assetSchema.Properties)
	}
}

// TestOpenAPISourceIsSyncedIntoServerModule compares the canonical contract with
// the embedded server copy so go generate cannot leave runtime docs stale.
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

// containsString checks required-property membership without assuming order,
// since OpenAPI's required array expresses a set rather than response layout.
func containsString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

// assertOptionalParameters verifies presence without accidentally making asset
// filters mandatory for clients that rely on server-side defaults.
func assertOptionalParameters(t *testing.T, parameters []struct {
	Name     string `json:"name"`
	Required bool   `json:"required"`
}, names ...string) {
	t.Helper()
	for _, name := range names {
		found := false
		for _, parameter := range parameters {
			if parameter.Name == name && !parameter.Required {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("missing optional parameter %s", name)
		}
	}
}
