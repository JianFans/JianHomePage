// Package pages implements the Pages direct-upload HTTP protocol without a Node runtime.
package pages

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

const maxResponseBytes = 1 << 20
const requestTimeout = 30 * time.Second

var identifier = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,255}$`)

// Deployment is the verified production identity and normalized cloud state.
type Deployment struct{ ID, ProjectID, Environment, Status string }

// Config selects the Pages token, fixed API region, and optional HTTP transport.
type Config struct {
	Token, Region string
	HTTPClient    *http.Client
}

// Client communicates with Pages and its temporary COS upload bucket.
type Client struct {
	token    string
	endpoint string
	http     *http.Client
}

// NewClient validates configuration before any cloud request is possible.
func NewClient(config Config) (*Client, error) {
	if !safeHeaderValue(config.Token) {
		return nil, errors.New("invalid Pages token")
	}
	var endpoint string
	switch config.Region {
	case "china":
		endpoint = "https://pages-api.cloud.tencent.com/v1"
	case "global":
		endpoint = "https://pages-api.edgeone.ai/v1"
	default:
		return nil, errors.New("invalid Pages region")
	}
	client := http.Client{Timeout: requestTimeout}
	if config.HTTPClient != nil {
		client = *config.HTTPClient
	}
	if client.Timeout <= 0 || client.Timeout > requestTimeout {
		client.Timeout = requestTimeout
	}
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &Client{token: config.Token, endpoint: endpoint, http: &client}, nil
}

// Deploy submits one production deployment without replaying cloud writes.
func (c *Client) Deploy(ctx context.Context, projectID string, files map[string][]byte) (Deployment, error) {
	if !identifier.MatchString(projectID) {
		return Deployment{}, errors.New("invalid Pages project identity")
	}
	archive, err := prepareArchive(ctx, files)
	if err != nil {
		return Deployment{}, err
	}
	var projects struct {
		Projects []struct {
			ProjectID string `json:"ProjectId"`
			Provider  string
		}
	}
	if err := c.request(ctx, map[string]any{"Action": "DescribePagesProjects", "Filters": []any{map[string]any{"Name": "ProjectId", "Values": []string{projectID}}}, "Offset": 0, "Limit": 1}, &projects); err != nil {
		return Deployment{}, err
	}
	if len(projects.Projects) != 1 || projects.Projects[0].ProjectID != projectID || projects.Projects[0].Provider != "Upload" {
		return Deployment{}, errors.New("Pages project is not a verified direct-upload project")
	}
	var credentials uploadCredentials
	if err := c.request(ctx, map[string]any{"Action": "DescribePagesCosTempToken", "ProjectId": projectID}, &credentials); err != nil {
		return Deployment{}, err
	}
	objectKey, err := credentials.objectKey(projectID)
	if err != nil {
		return Deployment{}, err
	}
	if err := c.upload(ctx, credentials, objectKey, archive); err != nil {
		return Deployment{}, err
	}
	var created struct {
		ID          string  `json:"DeploymentId"`
		ProjectID   *string `json:"ProjectId"`
		Environment *string `json:"Env"`
	}
	if err := c.request(ctx, map[string]any{"Action": "CreatePagesDeployment", "ProjectId": projectID, "Env": "Production", "ViaMeta": "Upload", "Provider": "Upload", "DistType": "Zip", "TempBucketPath": objectKey}, &created); err != nil {
		return Deployment{}, err
	}
	if !identifier.MatchString(created.ID) || (created.ProjectID != nil && *created.ProjectID != projectID) || (created.Environment != nil && *created.Environment != "Production") {
		return Deployment{}, errors.New("invalid Pages deployment identity")
	}
	return Deployment{ID: created.ID, ProjectID: projectID, Environment: "Production", Status: "building"}, nil
}

// Get reads and validates one known production deployment.
func (c *Client) Get(ctx context.Context, projectID, deploymentID string) (Deployment, error) {
	if !identifier.MatchString(projectID) || !identifier.MatchString(deploymentID) {
		return Deployment{}, errors.New("invalid Pages deployment identity")
	}
	var response struct {
		Deployments []cloudDeployment
		Deployment  *cloudDeployment
	}
	if err := c.request(ctx, map[string]any{"Action": "DescribePagesDeployments", "ProjectId": projectID, "DeploymentIds": []string{deploymentID}, "Offset": 0, "Limit": 1}, &response); err != nil {
		return Deployment{}, err
	}
	if len(response.Deployments) == 0 && response.Deployment != nil {
		response.Deployments = []cloudDeployment{*response.Deployment}
	}
	if len(response.Deployments) != 1 {
		return Deployment{}, errors.New("Pages deployment was not uniquely identified")
	}
	result := response.Deployments[0]
	if result.ID != deploymentID || result.ProjectID != projectID || result.Environment != "Production" {
		return Deployment{}, errors.New("invalid Pages deployment identity")
	}
	var status string
	switch result.Status {
	case "Pending", "Process":
		status = "building"
	case "Success":
		status = "succeeded"
	case "Failed", "Timeout", "Cancelled", "Invalid":
		status = "failed"
	default:
		return Deployment{}, errors.New("unknown Pages deployment status")
	}
	return Deployment{ID: deploymentID, ProjectID: projectID, Environment: "Production", Status: status}, nil
}

// cloudDeployment retains the explicit target fields that must be checked before reconciliation.
type cloudDeployment struct {
	ID          string `json:"DeploymentId"`
	ProjectID   string `json:"ProjectId"`
	Environment string `json:"Env"`
	Status      string
}

// request performs exactly one authenticated action and accepts only the documented success envelope.
func (c *Client) request(ctx context.Context, body map[string]any, out any) error {
	encoded, err := json.Marshal(body)
	if err != nil {
		return errors.New("invalid Pages request")
	}
	action, ok := body["Action"].(string)
	if !ok || (&cloudError{action: action}).ProviderAction() == "unknown" {
		return errors.New("invalid Pages request")
	}
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	// Wrapping the reader prevents net/http from synthesizing GetBody and replaying a write.
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, io.NopCloser(bytes.NewReader(encoded)))
	if err != nil {
		return errors.New("invalid Pages request")
	}
	req.ContentLength = int64(len(encoded))
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "yujian-pages/go")
	data, status, err := c.send(req, action)
	if err != nil {
		return err
	}
	var envelope struct {
		Code json.RawMessage
		Data struct{ Response json.RawMessage }
	}
	if json.Unmarshal(data, &envelope) != nil || !successCode(envelope.Code) || len(envelope.Data.Response) == 0 || bytes.Equal(bytes.TrimSpace(envelope.Data.Response), []byte("null")) {
		return newCloudError(action, status, data, nil)
	}
	var payload struct{ Error json.RawMessage }
	if json.Unmarshal(envelope.Data.Response, &payload) != nil || (len(payload.Error) > 0 && !bytes.Equal(bytes.TrimSpace(payload.Error), []byte("null"))) {
		return newCloudError(action, status, data, nil)
	}
	if json.Unmarshal(envelope.Data.Response, out) != nil {
		return newCloudError(action, status, data, nil)
	}
	return nil
}

func successCode(code json.RawMessage) bool {
	code = bytes.TrimSpace(code)
	return bytes.Equal(code, []byte("0")) || bytes.Equal(code, []byte(`"0"`))
}

// send bounds provider responses and preserves only safe status and request classifications.
func (c *Client) send(req *http.Request, action string) ([]byte, int, error) {
	if err := req.Context().Err(); err != nil {
		return nil, 0, newCloudError(action, 0, nil, err)
	}
	response, err := c.http.Do(req)
	if err != nil {
		if req.Context().Err() != nil {
			err = req.Context().Err()
		}
		return nil, 0, newCloudError(action, 0, nil, err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		data, readErr := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
		if len(data) > maxResponseBytes {
			data = nil
		}
		return nil, response.StatusCode, newCloudError(action, response.StatusCode, data, readErr)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		if req.Context().Err() != nil {
			err = req.Context().Err()
		}
		return nil, response.StatusCode, newCloudError(action, response.StatusCode, nil, err)
	}
	if len(data) > maxResponseBytes {
		return nil, response.StatusCode, newCloudError(action, response.StatusCode, nil, nil)
	}
	return data, response.StatusCode, nil
}

// safeHeaderValue allows only bounded non-whitespace ASCII credential values.
func safeHeaderValue(value string) bool {
	if len(value) == 0 || len(value) > 16384 {
		return false
	}
	return strings.IndexFunc(value, func(r rune) bool { return r <= 32 || r >= 127 }) < 0
}
