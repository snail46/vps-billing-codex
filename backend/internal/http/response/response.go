package response

import (
	"encoding/json"
	"net/http"

	"vps-billing/backend/internal/http/middleware"
)

type envelope struct {
	Success   bool      `json:"success"`
	Data      any       `json:"data,omitempty"`
	Error     *apiError `json:"error,omitempty"`
	RequestID string    `json:"request_id"`
}

type apiError struct {
	Code       string         `json:"code"`
	MessageKey string         `json:"message_key"`
	Details    map[string]any `json:"details,omitempty"`
}

func JSON(w http.ResponseWriter, r *http.Request, status int, data any) {
	write(w, status, envelope{Success: true, Data: data, RequestID: middleware.RequestID(r.Context())})
}

func Error(w http.ResponseWriter, r *http.Request, status int, code, messageKey string) {
	write(w, status, envelope{Success: false, Error: &apiError{Code: code, MessageKey: messageKey}, RequestID: middleware.RequestID(r.Context())})
}

func write(w http.ResponseWriter, status int, value envelope) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
