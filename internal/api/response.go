package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// APIResponse is the standard response envelope for all API responses.
type APIResponse struct {
	Success bool   `json:"success"`
	Data    any    `json:"data,omitempty"`
	Error   *APIError `json:"error,omitempty"`
	Meta    *APIMeta  `json:"meta,omitempty"`
}

// APIError is the standardized error format.
type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}

// APIMeta holds pagination and request metadata.
type APIMeta struct {
	RequestID  string `json:"request_id,omitempty"`
	Page       int    `json:"page,omitempty"`
	PerPage    int    `json:"per_page,omitempty"`
	Total      int    `json:"total,omitempty"`
	TotalPages int    `json:"total_pages,omitempty"`
}

// RespondOK sends a success response.
func RespondOK(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(APIResponse{Success: true, Data: data})
}

// RespondCreated sends a 201 response.
func RespondCreated(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(APIResponse{Success: true, Data: data})
}

// RespondError sends an error response with appropriate status code.
func RespondError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(APIResponse{
		Success: false,
		Error:   &APIError{Code: code, Message: message},
	})
}

// RespondFromError maps core errors to HTTP status codes.
func RespondFromError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, core.ErrNotFound):
		RespondError(w, http.StatusNotFound, "not_found", err.Error())
	case errors.Is(err, core.ErrAlreadyExists):
		RespondError(w, http.StatusConflict, "already_exists", err.Error())
	case errors.Is(err, core.ErrUnauthorized):
		RespondError(w, http.StatusUnauthorized, "unauthorized", err.Error())
	case errors.Is(err, core.ErrForbidden):
		RespondError(w, http.StatusForbidden, "forbidden", err.Error())
	case errors.Is(err, core.ErrQuotaExceeded):
		RespondError(w, http.StatusForbidden, "quota_exceeded", err.Error())
	case errors.Is(err, core.ErrInvalidInput):
		RespondError(w, http.StatusBadRequest, "invalid_input", err.Error())
	case errors.Is(err, core.ErrInvalidToken):
		RespondError(w, http.StatusUnauthorized, "invalid_token", err.Error())
	default:
		RespondError(w, http.StatusInternalServerError, "internal_error", "an unexpected error occurred")
	}
}

// RespondPaginated sends a paginated response.
func RespondPaginated(w http.ResponseWriter, data any, page, perPage, total int) {
	totalPages := (total + perPage - 1) / perPage
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(APIResponse{
		Success: true,
		Data:    data,
		Meta: &APIMeta{
			Page:       page,
			PerPage:    perPage,
			Total:      total,
			TotalPages: totalPages,
		},
	})
}
