package middleware

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCorrelationPropagatesValidIDs(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := Correlation(logger, http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if RequestID(request.Context()) != "request-123" {
			t.Errorf("request ID = %q", RequestID(request.Context()))
		}
		if TraceID(request.Context()) != "trace-123" {
			t.Errorf("trace ID = %q", TraceID(request.Context()))
		}
		response.WriteHeader(http.StatusNoContent)
	}))
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set("X-Request-ID", "request-123")
	request.Header.Set("X-Trace-ID", "trace-123")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Header().Get("X-Request-ID") != "request-123" {
		t.Fatalf("X-Request-ID = %q", response.Header().Get("X-Request-ID"))
	}
}

func TestCorrelationReplacesInvalidIDs(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := Correlation(logger, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set("X-Request-ID", "invalid value")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, request)

	if response.Header().Get("X-Request-ID") == "invalid value" || response.Header().Get("X-Request-ID") == "" {
		t.Fatalf("X-Request-ID = %q", response.Header().Get("X-Request-ID"))
	}
}
