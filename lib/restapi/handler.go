// Package restapi exposes asynchronous translation jobs over HTTP.
package restapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	transliter "github.com/snowmerak/transliter/lib"
	"github.com/snowmerak/transliter/lib/jobs"
)

const (
	DefaultRetention = 30 * 24 * time.Hour
	DefaultBodyLimit = 2 << 20
)

type Handler struct {
	Authenticator jobs.Authenticator
	Queue         jobs.Queue
	Store         jobs.Store
	Catalog       Catalog
	Models        Models
	Retention     time.Duration
	MaxBodyBytes  int64
	Now           func() time.Time
	Validate      func(jobs.Request) error
}

func (handler *Handler) Routes() (http.Handler, error) {
	if handler.Authenticator == nil || handler.Queue == nil || handler.Store == nil {
		return nil, fmt.Errorf("REST authenticator, queue, and store are required")
	}
	if handler.Retention <= 0 {
		handler.Retention = DefaultRetention
	}
	if handler.MaxBodyBytes <= 0 {
		handler.MaxBodyBytes = DefaultBodyLimit
	}
	if handler.Now == nil {
		handler.Now = time.Now
	}
	if handler.Validate == nil {
		handler.Validate = validateRequest
	}

	mux := http.NewServeMux()
	if err := registerDocumentation(mux); err != nil {
		return nil, err
	}
	mux.HandleFunc("GET /healthz", handler.health)
	mux.HandleFunc("GET /v1/models", handler.listModels)
	mux.HandleFunc("GET /v1/model-catalogs", handler.listModelCatalogs)
	mux.HandleFunc("GET /v1/model-catalogs/{id}", handler.getModelCatalog)
	mux.HandleFunc("POST /v1/model-catalogs/{id}/preview", handler.previewModelCatalog)
	mux.HandleFunc("POST /v1/jobs", handler.auth(handler.create))
	mux.HandleFunc("GET /v1/jobs", handler.auth(handler.list))
	mux.HandleFunc("GET /v1/jobs/{id}", handler.auth(handler.get))
	return mux, nil
}

type authenticatedHandler func(http.ResponseWriter, *http.Request, jobs.Principal)

func (handler *Handler) auth(next authenticatedHandler) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		key := apiKey(request)
		principal, err := handler.Authenticator.Authenticate(request.Context(), key)
		if err != nil {
			if key == "" {
				writeError(writer, http.StatusUnauthorized, "missing API key")
				return
			}
			writeError(writer, http.StatusUnauthorized, "invalid API key")
			return
		}
		next(writer, request, principal)
	}
}

func (*Handler) health(writer http.ResponseWriter, _ *http.Request) {
	writeJSON(writer, http.StatusOK, map[string]string{"status": "ok"})
}

func (handler *Handler) create(
	writer http.ResponseWriter,
	request *http.Request,
	principal jobs.Principal,
) {
	var input jobs.Request
	if err := decodeJSON(writer, request, handler.MaxBodyBytes, &input); err != nil {
		writeError(writer, http.StatusBadRequest, err.Error())
		return
	}
	if err := handler.Validate(input); err != nil {
		writeError(writer, http.StatusUnprocessableEntity, err.Error())
		return
	}
	job, err := jobs.New(principal.ID, input, handler.Now(), handler.Retention)
	if err != nil {
		writeError(writer, http.StatusInternalServerError, "could not allocate job")
		return
	}
	if err := handler.Store.Create(request.Context(), job); err != nil {
		writeError(writer, http.StatusServiceUnavailable, "could not store job")
		return
	}
	if err := handler.Queue.Enqueue(request.Context(), job.ID); err != nil {
		now := handler.Now().UTC()
		_ = handler.Store.Update(request.Context(), job.ID, jobs.Update{
			Status:      jobs.StatusFailed,
			Error:       "job queue unavailable",
			UpdatedAt:   now,
			CompletedAt: &now,
		})
		writeError(writer, http.StatusServiceUnavailable, "could not enqueue job")
		return
	}
	writer.Header().Set("Location", "/v1/jobs/"+job.ID)
	writeJSON(writer, http.StatusAccepted, job)
}

func (handler *Handler) get(
	writer http.ResponseWriter,
	request *http.Request,
	principal jobs.Principal,
) {
	job, err := handler.Store.Get(request.Context(), request.PathValue("id"))
	if err != nil || job.OwnerID != principal.ID {
		writeError(writer, http.StatusNotFound, "job not found")
		return
	}
	writeJSON(writer, http.StatusOK, job)
}

func (handler *Handler) list(
	writer http.ResponseWriter,
	request *http.Request,
	principal jobs.Principal,
) {
	limit := 20
	if raw := request.URL.Query().Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 100 {
			writeError(writer, http.StatusBadRequest, "limit must be between 1 and 100")
			return
		}
		limit = parsed
	}
	var before time.Time
	if raw := request.URL.Query().Get("before"); raw != "" {
		parsed, err := time.Parse(time.RFC3339Nano, raw)
		if err != nil {
			writeError(writer, http.StatusBadRequest, "before must be RFC3339")
			return
		}
		before = parsed
	}
	history, err := handler.Store.List(request.Context(), principal.ID, jobs.ListOptions{
		Limit:  limit,
		Before: before,
	})
	if err != nil {
		writeError(writer, http.StatusServiceUnavailable, "could not list jobs")
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"jobs": history})
}

func validateRequest(request jobs.Request) error {
	if request.ModelCatalog == "" {
		return fmt.Errorf("model_catalog must not be empty")
	}
	return transliter.ValidateTranslationRequest(request.Translation)
}

func apiKey(request *http.Request) string {
	authorization := request.Header.Get("Authorization")
	if scheme, value, ok := strings.Cut(authorization, " "); ok &&
		strings.EqualFold(scheme, "Bearer") {
		return strings.TrimSpace(value)
	}
	return strings.TrimSpace(request.Header.Get("X-API-Key"))
}

func decodeJSON(
	writer http.ResponseWriter,
	request *http.Request,
	limit int64,
	target any,
) error {
	request.Body = http.MaxBytesReader(writer, request.Body, limit)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("invalid JSON request: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return fmt.Errorf("request must contain one JSON object")
	}
	return nil
}

func writeError(writer http.ResponseWriter, status int, message string) {
	writeJSON(writer, status, map[string]any{
		"error": map[string]any{
			"status":  status,
			"message": message,
		},
	})
}

func writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}
