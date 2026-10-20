package httpapi

import (
	"errors"
	"net/http"

	"yujian.me/server/internal/domain"
)

// publishBuildResponse exposes operational status without the frozen target, snapshot request or audit notes.
type publishBuildResponse struct {
	Phase                 string `json:"phase"`
	DeploymentID          string `json:"deploymentId,omitempty"`
	Error                 string `json:"error,omitempty"`
	ConfigurationMismatch bool   `json:"configurationMismatch"`
}

// publishResponse enriches a durable job without issuing cloud requests or hiding an accepted write on diagnostic failure.
func (handler *Handler) publishResponse(request *http.Request, job domain.PublishJob) publishResponse {
	response := toPublishResponse(job)
	if job.BuildID != "" && handler.publisher != nil {
		task, err := handler.publisher.Inspect(request.Context(), job.BuildID)
		if err == nil {
			response.Build = &publishBuildResponse{Phase: task.Phase, DeploymentID: task.DeploymentID, Error: task.Error, ConfigurationMismatch: task.ConfigurationMismatch}
		}
	}
	return response
}

// latestPublish returns no content for an empty history, keeping absence distinct from read failures.
func (handler *Handler) latestPublish(writer http.ResponseWriter, request *http.Request) {
	actor, ok := principal(writer, request)
	if !ok {
		return
	}
	if handler.publish == nil {
		writeError(writer, request, http.StatusServiceUnavailable, "unavailable", "Publish service unavailable.")
		return
	}
	job, err := handler.publish.LatestPublishJob(request.Context(), actor)
	if errors.Is(err, domain.ErrNotFound) {
		writer.WriteHeader(http.StatusNoContent)
		return
	}
	if err != nil {
		writePublishError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, handler.publishResponse(request, job))
}
