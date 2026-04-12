package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// These helpers exist only to support legacy tests in this package.
// They were moved here from deleted production files (helpers.go, response.go).

type Pagination struct {
	Page    int
	PerPage int
	Offset  int
}

type APIResponse struct {
	Success bool      `json:"success"`
	Data    any       `json:"data,omitempty"`
	Error   *APIError `json:"error,omitempty"`
	Meta    *APIMeta  `json:"meta,omitempty"`
}

type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}

type APIMeta struct {
	RequestID  string `json:"request_id,omitempty"`
	Page       int    `json:"page,omitempty"`
	PerPage    int    `json:"per_page,omitempty"`
	Total      int    `json:"total,omitempty"`
	TotalPages int    `json:"total_pages,omitempty"`
}

type PaginatedResponse struct {
	Data       any `json:"data"`
	Total      int `json:"total"`
	Page       int `json:"page"`
	PerPage    int `json:"per_page"`
	TotalPages int `json:"total_pages"`
}

func writeError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}

func parseJSON(r *http.Request, dest any) error {
	if r.Body == nil {
		return fmt.Errorf("empty request body")
	}
	defer r.Body.Close()

	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dest); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	return nil
}

func parsePagination(r *http.Request) Pagination {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	perPage, _ := strconv.Atoi(r.URL.Query().Get("per_page"))

	if page < 1 {
		page = 1
	}
	if perPage < 1 || perPage > 100 {
		perPage = 20
	}

	return Pagination{
		Page:    page,
		PerPage: perPage,
		Offset:  (page - 1) * perPage,
	}
}

func realIP(r *http.Request) string {
	if ip := r.Header.Get("X-Real-IP"); ip != "" {
		return ip
	}
	if ip := r.Header.Get("X-Forwarded-For"); ip != "" {
		return ip
	}
	return r.RemoteAddr
}

func RespondOK(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(APIResponse{Success: true, Data: data})
}

func RespondCreated(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(APIResponse{Success: true, Data: data})
}

func RespondError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(APIResponse{
		Success: false,
		Error:   &APIError{Code: code, Message: message},
	})
}

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
