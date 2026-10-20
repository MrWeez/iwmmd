package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/mrweez/iwmmd/internal/auth"
	"github.com/mrweez/iwmmd/internal/domain"
	"github.com/mrweez/iwmmd/internal/store"
)

type problem struct {
	Type   string              `json:"type"`
	Title  string              `json:"title"`
	Status int                 `json:"status"`
	Detail string              `json:"detail,omitempty"`
	Errors map[string][]string `json:"errors,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if value != nil {
		_ = json.NewEncoder(w).Encode(value)
	}
}

func writeProblem(w http.ResponseWriter, status int, title, detail string) {
	writeProblemErrors(w, status, title, detail, nil)
}

func writeProblemErrors(w http.ResponseWriter, status int, title, detail string, fields map[string][]string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(problem{
		Type:   problemType(status),
		Title:  title,
		Status: status,
		Detail: detail,
		Errors: fields,
	})
}

func problemType(status int) string {
	switch status {
	case http.StatusBadRequest:
		return "https://iwmmd.local/problems/validation"
	case http.StatusUnauthorized:
		return "https://iwmmd.local/problems/unauthorized"
	case http.StatusForbidden:
		return "https://iwmmd.local/problems/forbidden"
	case http.StatusNotFound:
		return "https://iwmmd.local/problems/not-found"
	case http.StatusConflict:
		return "https://iwmmd.local/problems/conflict"
	case http.StatusTooManyRequests:
		return "https://iwmmd.local/problems/rate-limited"
	case http.StatusServiceUnavailable:
		return "https://iwmmd.local/problems/unavailable"
	default:
		return "about:blank"
	}
}

func decodeJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		writeProblem(w, http.StatusBadRequest, "Invalid request", err.Error())
		return false
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		writeProblem(w, http.StatusBadRequest, "Invalid request", "request body must contain one JSON object")
		return false
	}
	return true
}

func writeServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeProblem(w, http.StatusNotFound, "Not found", "The requested record does not exist.")
	case errors.Is(err, store.ErrConflict):
		writeProblem(w, http.StatusConflict, "Conflict", "The record changed or already exists.")
	case errors.Is(err, domain.ErrRateLimited):
		writeProblem(w, http.StatusTooManyRequests, "Rate limited", "This domain was checked recently. Try again later.")
	case errors.Is(err, domain.ErrInvalidState):
		writeProblem(w, http.StatusConflict, "Invalid state", "The domain is not in a state that allows this action.")
	case errors.Is(err, auth.ErrDenied):
		writeProblem(w, http.StatusForbidden, "Access denied", "Your account is not authorized.")
	default:
		writeProblem(w, http.StatusInternalServerError, "Internal error", "The request could not be completed.")
	}
}

func parseLimit(r *http.Request, fallback, max int) int {
	value := r.URL.Query().Get("limit")
	if value == "" {
		return fallback
	}
	n, err := strconv.Atoi(value)
	if err != nil || n <= 0 {
		return fallback
	}
	if n > max {
		return max
	}
	return n
}

func parseOffset(r *http.Request) int {
	n, err := strconv.Atoi(r.URL.Query().Get("offset"))
	if err != nil || n < 0 {
		return 0
	}
	return n
}

func parseVersion(r *http.Request) int {
	n, _ := strconv.Atoi(r.URL.Query().Get("version"))
	return n
}

func now() time.Time { return time.Now().UTC() }
