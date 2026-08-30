// Package edgeone contains the HTTP adapter used to trigger and observe an
// EdgeOne Pages build. The endpoint shape is intentionally configurable: a
// small webhook or gateway can translate this stable contract to the selected
// EdgeOne account/API without leaking provider-specific types into the domain.
package edgeone

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"yujian.me/server/internal/domain"
	"yujian.me/server/internal/httpurl"
	"yujian.me/server/internal/ports"
)

const defaultMaxResponseBytes int64 = 64 * 1024

type Config struct {
	TriggerURL       string
	StatusURL        string
	Token            string
	HTTPClient       *http.Client
	MaxResponseBytes int64
	RequireHTTPS     bool
}

type Client struct {
	triggerURL       *url.URL
	statusURL        *url.URL
	token            string
	httpClient       *http.Client
	maxResponseBytes int64
}

type buildResponse struct {
	ID         string `json:"id"`
	Status     string `json:"status"`
	PreviewURL string `json:"previewUrl"`
	Error      string `json:"error"`
}

func NewClient(config Config) (*Client, error) {
	triggerURL, err := parseEndpoint(config.TriggerURL, config.RequireHTTPS)
	if err != nil {
		return nil, fmt.Errorf("trigger endpoint: %w", err)
	}
	statusURL, err := parseEndpoint(config.StatusURL, config.RequireHTTPS)
	if err != nil {
		return nil, fmt.Errorf("status endpoint: %w", err)
	}
	maxResponseBytes := config.MaxResponseBytes
	if maxResponseBytes <= 0 {
		maxResponseBytes = defaultMaxResponseBytes
	}
	httpClient := config.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 20 * time.Second}
	}
	httpClient = edgeOneHTTPClient(httpClient, config.RequireHTTPS)
	return &Client{
		triggerURL:       triggerURL,
		statusURL:        statusURL,
		token:            config.Token,
		httpClient:       httpClient,
		maxResponseBytes: maxResponseBytes,
	}, nil
}

func edgeOneHTTPClient(client *http.Client, requireHTTPS bool) *http.Client {
	if !requireHTTPS {
		return client
	}
	secured := *client
	checkRedirect := client.CheckRedirect
	secured.CheckRedirect = func(request *http.Request, via []*http.Request) error {
		if request.URL.Scheme != "https" {
			return errors.New("EdgeOne redirect must use HTTPS")
		}
		if checkRedirect != nil {
			return checkRedirect(request, via)
		}
		return nil
	}
	return &secured
}

func (client *Client) Trigger(ctx context.Context, request ports.BuildRequest) (ports.BuildRun, error) {
	body, err := json.Marshal(request)
	if err != nil {
		return ports.BuildRun{}, err
	}
	response, err := client.do(ctx, http.MethodPost, client.triggerURL, body, request.IdempotencyKey)
	if err != nil {
		return ports.BuildRun{}, err
	}
	return decodeBuildRun(response)
}

func (client *Client) Status(ctx context.Context, buildID string) (ports.BuildRun, error) {
	if strings.TrimSpace(buildID) == "" {
		return ports.BuildRun{}, domain.ErrInvalidInput
	}
	endpoint := *client.statusURL
	basePath := strings.TrimRight(endpoint.Path, "/")
	baseRawPath := strings.TrimRight(endpoint.EscapedPath(), "/")
	endpoint.Path = basePath + "/" + buildID
	endpoint.RawPath = baseRawPath + "/" + url.PathEscape(buildID)
	response, err := client.do(ctx, http.MethodGet, &endpoint, nil, "")
	if err != nil {
		return ports.BuildRun{}, err
	}
	run, err := decodeBuildRun(response)
	if err != nil {
		return ports.BuildRun{}, err
	}
	if run.ID != buildID {
		return ports.BuildRun{}, errors.New("provider response build id mismatch")
	}
	return run, nil
}

func (client *Client) do(ctx context.Context, method string, endpoint *url.URL, body []byte, idempotencyKey string) ([]byte, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	request, err := http.NewRequestWithContext(ctx, method, endpoint.String(), reader)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/json")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if client.token != "" {
		request.Header.Set("Authorization", "Bearer "+client.token)
	}
	if idempotencyKey != "" {
		request.Header.Set("Idempotency-Key", idempotencyKey)
	}
	response, err := client.httpClient.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	limited := io.LimitReader(response.Body, client.maxResponseBytes+1)
	responseBody, readErr := io.ReadAll(limited)
	if readErr != nil {
		return nil, readErr
	}
	if int64(len(responseBody)) > client.maxResponseBytes {
		return nil, errors.New("provider response too large")
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("provider request failed with status %d", response.StatusCode)
	}
	return responseBody, nil
}

func decodeBuildRun(body []byte) (ports.BuildRun, error) {
	var response buildResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return ports.BuildRun{}, errors.New("provider returned invalid JSON")
	}
	if strings.TrimSpace(response.ID) == "" {
		return ports.BuildRun{}, errors.New("provider response missing build id")
	}
	status := domain.PublishStatus(response.Status)
	switch status {
	case domain.PublishPending, domain.PublishBuilding, domain.PublishSucceeded, domain.PublishFailed:
	default:
		return ports.BuildRun{}, errors.New("provider response has invalid build status")
	}
	return ports.BuildRun{ID: response.ID, Status: status, PreviewURL: response.PreviewURL, Error: response.Error}, nil
}

func parseEndpoint(value string, requireHTTPS bool) (*url.URL, error) {
	if strings.TrimSpace(value) == "" {
		return nil, errors.New("URL is required")
	}
	parsed, err := httpurl.ParseAbsolute(value)
	if err != nil || parsed.Path == "" && parsed.RawQuery != "" {
		return nil, errors.New("URL must be absolute")
	}
	if requireHTTPS && parsed.Scheme != "https" {
		return nil, errors.New("HTTPS is required")
	}
	return parsed, nil
}

var _ ports.BuildTrigger = (*Client)(nil)
