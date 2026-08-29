package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"yujian.me/server/internal/assets"
	"yujian.me/server/internal/auth"
	"yujian.me/server/internal/content"
	"yujian.me/server/internal/domain"
	"yujian.me/server/internal/publish"
)

const defaultMaxBodyBytes int64 = 2 * 1024 * 1024

// ContentService is the application boundary used by the HTTP adapter.
type ContentService interface {
	CreateDraft(context.Context, domain.Principal, json.RawMessage) (domain.ContentVersion, error)
	GetVersion(context.Context, domain.Principal, string) (domain.ContentVersion, error)
	UpdateDraft(context.Context, domain.Principal, string, int64, json.RawMessage) (domain.ContentVersion, error)
	SubmitReview(context.Context, domain.Principal, string, int64) (domain.ContentVersion, error)
	ApproveReview(context.Context, domain.Principal, string, int64) (domain.ContentVersion, error)
	RejectReview(context.Context, domain.Principal, string, int64, string) (domain.ContentVersion, error)
}

// AssetService is the application boundary used by the HTTP adapter.
type AssetService interface {
	CreateUpload(context.Context, domain.Principal, assets.CreateUploadInput) (assets.CreateUploadResult, error)
	CompleteUpload(context.Context, domain.Principal, string) (domain.AssetRecord, error)
	Delete(context.Context, domain.Principal, string) error
}

// PublishService is the application boundary used by the HTTP adapter.
type PublishService interface {
	Publish(context.Context, domain.Principal, string, string) (domain.PublishJob, error)
	GetPublishJob(context.Context, domain.Principal, string) (domain.PublishJob, error)
	RefreshStatus(context.Context, domain.Principal, string) (domain.PublishJob, error)
	Rollback(context.Context, domain.Principal, string, string) (domain.PublishJob, error)
}

type RouterOptions struct {
	Content      ContentService
	Assets       AssetService
	Publish      PublishService
	Middleware   *auth.Middleware
	MaxBodyBytes int64
	RequestID    func(*http.Request) string
}

type Handler struct {
	content      ContentService
	assets       AssetService
	publish      PublishService
	maxBodyBytes int64
}

type versionWrite struct {
	Snapshot json.RawMessage `json:"snapshot"`
}

type revisionRequest struct {
	Revision int64 `json:"revision"`
}

type rejectRequest struct {
	Revision int64  `json:"revision"`
	Reason   string `json:"reason"`
}

type assetUploadRequest struct {
	FileName    string          `json:"fileName"`
	ContentType string          `json:"contentType"`
	Size        int64           `json:"size"`
	Checksum    string          `json:"checksum"`
	Rights      json.RawMessage `json:"rights"`
}

type publishRequest struct {
	VersionID string `json:"versionId"`
}

type errorResponse struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"requestId"`
}

type versionResponse struct {
	ID             string               `json:"id"`
	Status         domain.ContentStatus `json:"status"`
	Revision       int64                `json:"revision"`
	Snapshot       json.RawMessage      `json:"snapshot"`
	Checksum       string               `json:"checksum"`
	ReviewApproved bool                 `json:"reviewApproved,omitempty"`
	CreatedAt      time.Time            `json:"createdAt,omitempty"`
	UpdatedAt      time.Time            `json:"updatedAt,omitempty"`
}

type assetResponse struct {
	ID        string             `json:"id"`
	Status    domain.AssetStatus `json:"status"`
	Metadata  json.RawMessage    `json:"metadata"`
	Rights    json.RawMessage    `json:"rights"`
	CreatedAt time.Time          `json:"createdAt,omitempty"`
	DeletedAt *time.Time         `json:"deletedAt,omitempty"`
}

type assetUploadResponse struct {
	Asset     assetResponse     `json:"asset"`
	UploadURL string            `json:"uploadUrl"`
	Headers   map[string]string `json:"headers,omitempty"`
	ExpiresAt time.Time         `json:"expiresAt"`
}

type publishResponse struct {
	ID               string               `json:"id"`
	VersionID        string               `json:"versionId"`
	Status           domain.PublishStatus `json:"status"`
	SnapshotKey      string               `json:"snapshotKey"`
	SnapshotChecksum string               `json:"snapshotChecksum"`
	BuildID          string               `json:"buildId,omitempty"`
	ErrorMessage     string               `json:"errorMessage,omitempty"`
}

// NewRouter builds the public health endpoint and authenticated management API.
func NewRouter(options RouterOptions) http.Handler {
	maxBodyBytes := options.MaxBodyBytes
	if maxBodyBytes <= 0 {
		maxBodyBytes = defaultMaxBodyBytes
	}
	handler := &Handler{
		content:      options.Content,
		assets:       options.Assets,
		publish:      options.Publish,
		maxBodyBytes: maxBodyBytes,
	}
	middleware := options.Middleware
	if middleware == nil {
		middleware, _ = auth.NewMiddleware(auth.MiddlewareOptions{})
	}

	api := http.NewServeMux()
	register := func(pattern string, permission auth.Permission, endpoint http.HandlerFunc) {
		api.Handle(pattern, middleware.Require(permission, endpoint))
	}
	register("POST /api/v1/versions", auth.PermissionEditDraft, handler.createVersion)
	api.Handle("GET /api/v1/versions/{versionId}", http.HandlerFunc(handler.getVersion))
	register("PUT /api/v1/versions/{versionId}", auth.PermissionEditDraft, handler.updateVersion)
	register("POST /api/v1/versions/{versionId}/review", auth.PermissionSubmitReview, handler.submitReview)
	register("POST /api/v1/versions/{versionId}/approve", auth.PermissionReview, handler.approveReview)
	register("POST /api/v1/versions/{versionId}/reject", auth.PermissionReview, handler.rejectReview)
	register("POST /api/v1/assets/uploads", auth.PermissionCreateAsset, handler.createAssetUpload)
	register("POST /api/v1/assets/{assetId}/complete", auth.PermissionCreateAsset, handler.completeAssetUpload)
	register("DELETE /api/v1/assets/{assetId}", auth.PermissionDeleteAsset, handler.deleteAsset)
	register("POST /api/v1/publishes", auth.PermissionPublish, handler.createPublish)
	register("GET /api/v1/publishes/{publishId}", auth.PermissionPublish, handler.getPublish)
	register("POST /api/v1/publishes/{publishId}/refresh", auth.PermissionPublish, handler.refreshPublish)
	register("POST /api/v1/rollbacks", auth.PermissionRollback, handler.createRollback)

	root := http.NewServeMux()
	root.HandleFunc("GET /healthz", healthHandler)
	root.Handle("/api/", middleware.Authenticate(handler.withBodyLimit(api)))
	requestID := options.RequestID
	if requestID == nil {
		var sequence atomic.Uint64
		requestID = func(*http.Request) string {
			return fmt.Sprintf("req-%d", sequence.Add(1))
		}
	}
	return requestIDMiddleware(root, requestID)
}

func (handler *Handler) withBodyLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		request.Body = http.MaxBytesReader(writer, request.Body, handler.maxBodyBytes)
		next.ServeHTTP(writer, request)
	})
}

func requestIDMiddleware(next http.Handler, generator func(*http.Request) string) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requestID := strings.TrimSpace(request.Header.Get("X-Request-ID"))
		if requestID == "" {
			requestID = strings.TrimSpace(generator(request))
		}
		if requestID == "" {
			requestID = "req-unknown"
		}
		request.Header.Set("X-Request-ID", requestID)
		writer.Header().Set("X-Request-ID", requestID)
		next.ServeHTTP(writer, request)
	})
}

func (handler *Handler) createVersion(writer http.ResponseWriter, request *http.Request) {
	var input versionWrite
	if !decodeJSON(writer, request, &input) {
		return
	}
	actor, ok := principal(writer, request)
	if !ok || handler.content == nil {
		if ok {
			writeError(writer, request, http.StatusInternalServerError, "internal_error", "Service unavailable.")
		}
		return
	}
	version, err := handler.content.CreateDraft(request.Context(), actor, input.Snapshot)
	if err != nil {
		writeDomainError(writer, request, err)
		return
	}
	writeVersion(writer, http.StatusCreated, version)
}

func (handler *Handler) getVersion(writer http.ResponseWriter, request *http.Request) {
	actor, ok := principal(writer, request)
	if !ok || handler.content == nil {
		if ok {
			writeError(writer, request, http.StatusInternalServerError, "internal_error", "Service unavailable.")
		}
		return
	}
	version, err := handler.content.GetVersion(request.Context(), actor, request.PathValue("versionId"))
	if err != nil {
		writeDomainError(writer, request, err)
		return
	}
	writeVersion(writer, http.StatusOK, version)
}

func (handler *Handler) updateVersion(writer http.ResponseWriter, request *http.Request) {
	revision, ok := parseETag(request.Header.Get("If-Match"))
	if !ok {
		writeError(writer, request, http.StatusPreconditionRequired, "precondition_required", "If-Match header is required.")
		return
	}
	var input versionWrite
	if !decodeJSON(writer, request, &input) {
		return
	}
	actor, ok := principal(writer, request)
	if !ok || handler.content == nil {
		if ok {
			writeError(writer, request, http.StatusInternalServerError, "internal_error", "Service unavailable.")
		}
		return
	}
	version, err := handler.content.UpdateDraft(request.Context(), actor, request.PathValue("versionId"), revision, input.Snapshot)
	if err != nil {
		writeDomainError(writer, request, err)
		return
	}
	writeVersion(writer, http.StatusOK, version)
}

func (handler *Handler) submitReview(writer http.ResponseWriter, request *http.Request) {
	var input revisionRequest
	if !decodeJSON(writer, request, &input) {
		return
	}
	actor, ok := principal(writer, request)
	if !ok || handler.content == nil {
		if ok {
			writeError(writer, request, http.StatusInternalServerError, "internal_error", "Service unavailable.")
		}
		return
	}
	version, err := handler.content.SubmitReview(request.Context(), actor, request.PathValue("versionId"), input.Revision)
	if err != nil {
		writeDomainError(writer, request, err)
		return
	}
	writeVersion(writer, http.StatusOK, version)
}

func (handler *Handler) approveReview(writer http.ResponseWriter, request *http.Request) {
	var input revisionRequest
	if !decodeJSON(writer, request, &input) {
		return
	}
	actor, ok := principal(writer, request)
	if !ok || handler.content == nil {
		if ok {
			writeError(writer, request, http.StatusInternalServerError, "internal_error", "Service unavailable.")
		}
		return
	}
	version, err := handler.content.ApproveReview(request.Context(), actor, request.PathValue("versionId"), input.Revision)
	if err != nil {
		writeDomainError(writer, request, err)
		return
	}
	writeVersion(writer, http.StatusOK, version)
}

func (handler *Handler) rejectReview(writer http.ResponseWriter, request *http.Request) {
	var input rejectRequest
	if !decodeJSON(writer, request, &input) {
		return
	}
	actor, ok := principal(writer, request)
	if !ok || handler.content == nil {
		if ok {
			writeError(writer, request, http.StatusInternalServerError, "internal_error", "Service unavailable.")
		}
		return
	}
	version, err := handler.content.RejectReview(request.Context(), actor, request.PathValue("versionId"), input.Revision, input.Reason)
	if err != nil {
		writeDomainError(writer, request, err)
		return
	}
	writeVersion(writer, http.StatusOK, version)
}

func (handler *Handler) createAssetUpload(writer http.ResponseWriter, request *http.Request) {
	var input assetUploadRequest
	if !decodeJSON(writer, request, &input) {
		return
	}
	actor, ok := principal(writer, request)
	if !ok || handler.assets == nil {
		if ok {
			writeError(writer, request, http.StatusInternalServerError, "internal_error", "Service unavailable.")
		}
		return
	}
	result, err := handler.assets.CreateUpload(request.Context(), actor, assets.CreateUploadInput{
		FileName: input.FileName, ContentType: input.ContentType, Size: input.Size, Checksum: input.Checksum, Rights: input.Rights,
	})
	if err != nil {
		writeDomainError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusCreated, assetUploadResponse{
		Asset:     toAssetResponse(result.Asset),
		UploadURL: result.Upload.URL,
		Headers:   result.Upload.Headers,
		ExpiresAt: result.Upload.ExpiresAt,
	})
}

func (handler *Handler) completeAssetUpload(writer http.ResponseWriter, request *http.Request) {
	actor, ok := principal(writer, request)
	if !ok || handler.assets == nil {
		if ok {
			writeError(writer, request, http.StatusInternalServerError, "internal_error", "Service unavailable.")
		}
		return
	}
	asset, err := handler.assets.CompleteUpload(request.Context(), actor, request.PathValue("assetId"))
	if err != nil {
		writeDomainError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, toAssetResponse(asset))
}

func (handler *Handler) deleteAsset(writer http.ResponseWriter, request *http.Request) {
	actor, ok := principal(writer, request)
	if !ok || handler.assets == nil {
		if ok {
			writeError(writer, request, http.StatusInternalServerError, "internal_error", "Service unavailable.")
		}
		return
	}
	if err := handler.assets.Delete(request.Context(), actor, request.PathValue("assetId")); err != nil {
		writeDomainError(writer, request, err)
		return
	}
	writer.WriteHeader(http.StatusNoContent)
}

func (handler *Handler) createPublish(writer http.ResponseWriter, request *http.Request) {
	key := strings.TrimSpace(request.Header.Get("Idempotency-Key"))
	if key == "" {
		writeError(writer, request, http.StatusBadRequest, "invalid_request", "Idempotency-Key header is required.")
		return
	}
	var input publishRequest
	if !decodeJSON(writer, request, &input) {
		return
	}
	actor, ok := principal(writer, request)
	if !ok || handler.publish == nil {
		if ok {
			writeError(writer, request, http.StatusInternalServerError, "internal_error", "Service unavailable.")
		}
		return
	}
	job, err := handler.publish.Publish(request.Context(), actor, input.VersionID, key)
	if err != nil {
		writeDomainError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusAccepted, toPublishResponse(job))
}

func (handler *Handler) getPublish(writer http.ResponseWriter, request *http.Request) {
	actor, ok := principal(writer, request)
	if !ok || handler.publish == nil {
		if ok {
			writeError(writer, request, http.StatusInternalServerError, "internal_error", "Service unavailable.")
		}
		return
	}
	job, err := handler.publish.GetPublishJob(request.Context(), actor, request.PathValue("publishId"))
	if err != nil {
		writeDomainError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, toPublishResponse(job))
}

func (handler *Handler) refreshPublish(writer http.ResponseWriter, request *http.Request) {
	actor, ok := principal(writer, request)
	if !ok || handler.publish == nil {
		if ok {
			writeError(writer, request, http.StatusInternalServerError, "internal_error", "Service unavailable.")
		}
		return
	}
	job, err := handler.publish.RefreshStatus(request.Context(), actor, request.PathValue("publishId"))
	if err != nil {
		writeDomainError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, toPublishResponse(job))
}

func (handler *Handler) createRollback(writer http.ResponseWriter, request *http.Request) {
	key := strings.TrimSpace(request.Header.Get("Idempotency-Key"))
	if key == "" {
		writeError(writer, request, http.StatusBadRequest, "invalid_request", "Idempotency-Key header is required.")
		return
	}
	var input publishRequest
	if !decodeJSON(writer, request, &input) {
		return
	}
	actor, ok := principal(writer, request)
	if !ok || handler.publish == nil {
		if ok {
			writeError(writer, request, http.StatusInternalServerError, "internal_error", "Service unavailable.")
		}
		return
	}
	job, err := handler.publish.Rollback(request.Context(), actor, input.VersionID, key)
	if err != nil {
		writeDomainError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusAccepted, toPublishResponse(job))
}

func principal(writer http.ResponseWriter, request *http.Request) (domain.Principal, bool) {
	value, ok := auth.PrincipalFromContext(request.Context())
	if !ok {
		writeError(writer, request, http.StatusUnauthorized, "unauthorized", "Authentication required.")
	}
	return value, ok
}

func decodeJSON(writer http.ResponseWriter, request *http.Request, destination any) bool {
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		if isBodyTooLarge(err) {
			writeError(writer, request, http.StatusRequestEntityTooLarge, "request_too_large", "Request body is too large.")
		} else {
			writeError(writer, request, http.StatusBadRequest, "invalid_request", "Request body is invalid.")
		}
		return false
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		writeError(writer, request, http.StatusBadRequest, "invalid_request", "Request body must contain one JSON value.")
		return false
	}
	return true
}

func isBodyTooLarge(err error) bool {
	var maxErr *http.MaxBytesError
	return errors.As(err, &maxErr)
}

func parseETag(value string) (int64, bool) {
	value = strings.TrimSpace(value)
	if len(value) < 3 || value[0] != '"' || value[len(value)-1] != '"' {
		return 0, false
	}
	revision, err := strconv.ParseInt(value[1:len(value)-1], 10, 64)
	return revision, err == nil && revision > 0
}

func writeVersion(writer http.ResponseWriter, status int, version domain.ContentVersion) {
	writer.Header().Set("ETag", strconv.Quote(strconv.FormatInt(version.Revision, 10)))
	writeJSON(writer, status, versionResponse{
		ID: version.ID, Status: version.Status, Revision: version.Revision, Snapshot: version.Snapshot,
		Checksum: version.Checksum, ReviewApproved: version.ReviewApproved, CreatedAt: version.CreatedAt, UpdatedAt: version.UpdatedAt,
	})
}

func toAssetResponse(asset domain.AssetRecord) assetResponse {
	return assetResponse{ID: asset.ID, Status: asset.Status, Metadata: asset.Metadata, Rights: asset.Rights, CreatedAt: asset.CreatedAt, DeletedAt: asset.DeletedAt}
}

func toPublishResponse(job domain.PublishJob) publishResponse {
	return publishResponse{ID: job.ID, VersionID: job.VersionID, Status: job.Status, SnapshotKey: job.SnapshotKey, SnapshotChecksum: job.SnapshotChecksum, BuildID: job.BuildID, ErrorMessage: job.ErrorMessage}
}

func writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}

func writeDomainError(writer http.ResponseWriter, request *http.Request, err error) {
	switch {
	case errors.Is(err, domain.ErrForbidden):
		writeError(writer, request, http.StatusForbidden, "forbidden", "Permission denied.")
	case errors.Is(err, domain.ErrNotFound):
		writeError(writer, request, http.StatusNotFound, "not_found", "Resource not found.")
	case errors.Is(err, domain.ErrConflict):
		writeError(writer, request, http.StatusConflict, "conflict", "Resource was modified by another request.")
	case errors.Is(err, domain.ErrInvalidTransition):
		writeError(writer, request, http.StatusConflict, "invalid_state", "The requested state transition is not allowed.")
	case errors.Is(err, domain.ErrInvalidInput):
		writeError(writer, request, http.StatusBadRequest, "invalid_request", "Request data is invalid.")
	default:
		writeError(writer, request, http.StatusInternalServerError, "internal_error", "Service unavailable.")
	}
}

func writeError(writer http.ResponseWriter, request *http.Request, status int, code, message string) {
	writeJSON(writer, status, errorResponse{Code: code, Message: message, RequestID: request.Header.Get("X-Request-ID")})
}

func healthHandler(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writer.Header().Set("Allow", http.MethodGet)
		writeError(writer, request, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.")
		return
	}
	writeJSON(writer, http.StatusOK, map[string]string{"status": "ok"})
}

// Keep application package names part of the compile-time contract until the
// generated OpenAPI adapter is introduced.
var (
	_ ContentService = (*content.Service)(nil)
	_ AssetService   = (*assets.Service)(nil)
	_ PublishService = (*publish.Service)(nil)
)
