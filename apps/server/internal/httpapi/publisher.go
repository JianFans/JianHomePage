package httpapi

import (
	"context"
	"net/http"

	"yujian.me/server/internal/publisher"
)

// PublisherService exposes durable cloud diagnostics only to administrators.
type PublisherService interface {
	Inspect(context.Context, string) (publisher.Task, error)
	Recover(context.Context, string, publisher.RecoveryInput) (publisher.Task, error)
}

// inspectBuild returns the frozen target and cloud submission state without changing it.
func (handler *Handler) inspectBuild(writer http.ResponseWriter, request *http.Request) {
	if handler.publisher == nil {
		writeError(writer, request, http.StatusServiceUnavailable, "unavailable", "Publisher unavailable.")
		return
	}
	task, err := handler.publisher.Inspect(request.Context(), request.PathValue("buildId"))
	if err != nil {
		writeDomainError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, task)
}

// recoverBuild binds an operator-verified deployment or records confirmed absence.
// Subject always comes from the authenticated session, never the request body.
func (handler *Handler) recoverBuild(writer http.ResponseWriter, request *http.Request) {
	actor, ok := principal(writer, request)
	if !ok {
		return
	}
	if handler.publisher == nil {
		writeError(writer, request, http.StatusServiceUnavailable, "unavailable", "Publisher unavailable.")
		return
	}
	var input publisher.RecoveryInput
	if !decodeJSON(writer, request, &input) {
		return
	}
	input.ActorSubject = actor.Subject
	task, err := handler.publisher.Recover(request.Context(), request.PathValue("buildId"), input)
	if err != nil {
		writeDomainError(writer, request, err)
		return
	}
	writeJSON(writer, http.StatusOK, task)
}
