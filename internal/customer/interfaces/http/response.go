// Package http is the customer context's HTTP delivery adapter. It translates
// requests into application commands/queries and results/errors into responses.
// It contains no business logic.
package http

import (
	"encoding/json"
	"errors"
	"net/http"

	shareddomain "github.com/example/myapp/internal/shared/domain"
	"github.com/example/myapp/internal/shared/infrastructure/logging"
)

type errorBody struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if v != nil {
		_ = json.NewEncoder(w).Encode(v)
	}
}

// writeError maps a domain/application error to an HTTP status + stable code.
func writeError(w http.ResponseWriter, r *http.Request, err error) {
	status, code := http.StatusInternalServerError, "internal"
	switch {
	case errors.Is(err, shareddomain.ErrNotFound):
		status, code = http.StatusNotFound, "not_found"
	case errors.Is(err, shareddomain.ErrConflict):
		status, code = http.StatusConflict, "conflict"
	case errors.Is(err, shareddomain.ErrValidation):
		status, code = http.StatusBadRequest, "validation_failed"
	case errors.Is(err, shareddomain.ErrPermission):
		status, code = http.StatusForbidden, "forbidden"
	}

	if status >= 500 {
		logging.From(r.Context()).Error("request failed", "error", err, "path", r.URL.Path)
	}

	var body errorBody
	body.Error.Code = code
	if status < 500 {
		body.Error.Message = err.Error()
	} else {
		body.Error.Message = "internal server error"
	}
	writeJSON(w, status, body)
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		writeError(w, r, shareddomain.Invalid("malformed request body: "+err.Error()))
		return false
	}
	return true
}
