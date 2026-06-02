package httperr

import (
	"encoding/json"
	"net/http"
)

type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type errorBody struct {
	Error APIError `json:"error"`
}

func writeJSON(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(errorBody{
		Error: APIError{Code: code, Message: message},
	})
}

func Error(w http.ResponseWriter, status int, message string) {
	code := http.StatusText(status)
	writeJSON(w, status, code, message)
}

func BadRequest(w http.ResponseWriter, message string) {
	writeJSON(w, http.StatusBadRequest, "bad_request", message)
}

func NotFound(w http.ResponseWriter, message string) {
	writeJSON(w, http.StatusNotFound, "not_found", message)
}

func Internal(w http.ResponseWriter, message string) {
	writeJSON(w, http.StatusInternalServerError, "internal_error", message)
}

func Unauthorized(w http.ResponseWriter, message string) {
	writeJSON(w, http.StatusUnauthorized, "unauthorized", message)
}

func Forbidden(w http.ResponseWriter, message string) {
	writeJSON(w, http.StatusForbidden, "forbidden", message)
}

func Conflict(w http.ResponseWriter, message string) {
	writeJSON(w, http.StatusConflict, "conflict", message)
}
