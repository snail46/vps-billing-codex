package health

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"vps-billing/backend/internal/http/middleware"
)

const checkTimeout = 2 * time.Second

type Checker interface {
	Ping(context.Context) error
}

type Handler struct {
	checks map[string]Checker
	logger *slog.Logger
}

func New(postgres, redis Checker, logger *slog.Logger) *Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{
		checks: map[string]Checker{"postgres": postgres, "redis": redis},
		logger: logger,
	}
}

func (h *Handler) Register(router chi.Router) {
	router.Get("/health/live", h.live)
	router.Get("/health/ready", h.ready)
	router.Get("/api/v1/health/live", h.live)
	router.Get("/api/v1/health/ready", h.ready)
}

func (h *Handler) live(response http.ResponseWriter, request *http.Request) {
	h.writeJSON(response, request, http.StatusOK, successResponse{
		Success:   true,
		Data:      healthData{Status: "alive"},
		RequestID: middleware.RequestID(request.Context()),
	})
}

func (h *Handler) ready(response http.ResponseWriter, request *http.Request) {
	checks, ready := h.runChecks(request.Context())
	if !ready {
		h.writeJSON(response, request, http.StatusServiceUnavailable, failureResponse{
			Success: false,
			Error: apiError{
				Code:       "DEPENDENCY_UNAVAILABLE",
				MessageKey: "errors.service_unavailable",
				Details:    map[string]any{"checks": checks},
			},
			RequestID: middleware.RequestID(request.Context()),
		})
		return
	}

	h.writeJSON(response, request, http.StatusOK, successResponse{
		Success:   true,
		Data:      healthData{Status: "ready", Checks: checks},
		RequestID: middleware.RequestID(request.Context()),
	})
}

func (h *Handler) runChecks(parent context.Context) (map[string]string, bool) {
	type result struct {
		name string
		up   bool
	}
	results := make(chan result, len(h.checks))
	var waitGroup sync.WaitGroup
	for name, checker := range h.checks {
		waitGroup.Add(1)
		go func() {
			defer waitGroup.Done()
			ctx, cancel := context.WithTimeout(parent, checkTimeout)
			defer cancel()
			results <- result{name: name, up: checker.Ping(ctx) == nil}
		}()
	}
	waitGroup.Wait()
	close(results)

	checks := make(map[string]string, len(h.checks))
	ready := true
	for result := range results {
		if result.up {
			checks[result.name] = "up"
		} else {
			checks[result.name] = "down"
			ready = false
		}
	}
	return checks, ready
}

type healthData struct {
	Status string            `json:"status"`
	Checks map[string]string `json:"checks,omitempty"`
}

type successResponse struct {
	Success   bool       `json:"success"`
	Data      healthData `json:"data"`
	RequestID string     `json:"request_id"`
}

type apiError struct {
	Code       string         `json:"code"`
	MessageKey string         `json:"message_key"`
	Details    map[string]any `json:"details,omitempty"`
}

type failureResponse struct {
	Success   bool     `json:"success"`
	Error     apiError `json:"error"`
	RequestID string   `json:"request_id"`
}

func (h *Handler) writeJSON(response http.ResponseWriter, request *http.Request, status int, payload any) {
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(status)
	if err := json.NewEncoder(response).Encode(payload); err != nil {
		h.logger.ErrorContext(
			request.Context(),
			"write health response",
			"error", err,
			"request_id", middleware.RequestID(request.Context()),
			"trace_id", middleware.TraceID(request.Context()),
		)
	}
}
