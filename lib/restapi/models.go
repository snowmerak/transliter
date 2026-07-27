package restapi

import (
	"context"
	"net/http"
)

// Models fetches the upstream OpenAI-compatible model list.
type Models interface {
	ListModels(ctx context.Context) (statusCode int, contentType string, body []byte, err error)
}

func (handler *Handler) listModels(writer http.ResponseWriter, request *http.Request) {
	models, ok := handler.models()
	if !ok {
		writeError(writer, http.StatusServiceUnavailable, "provider models are unavailable")
		return
	}
	statusCode, contentType, body, err := models.ListModels(request.Context())
	if err != nil {
		writeError(writer, http.StatusBadGateway, "could not reach provider models")
		return
	}
	if contentType == "" {
		contentType = "application/json"
	}
	writer.Header().Set("Content-Type", contentType)
	writer.WriteHeader(statusCode)
	_, _ = writer.Write(body)
}

func (handler *Handler) models() (Models, bool) {
	if handler == nil || handler.Models == nil {
		return nil, false
	}
	return handler.Models, true
}
