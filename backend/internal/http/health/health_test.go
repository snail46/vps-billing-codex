package health

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"vps-billing/backend/internal/http/middleware"
)

type fakeChecker struct {
	err error
}

func (f fakeChecker) Ping(context.Context) error {
	return f.err
}

func TestLivenessDoesNotCheckDependencies(t *testing.T) {
	handler := testHandler(fakeChecker{err: errors.New("down")}, fakeChecker{err: errors.New("down")})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/health/live", nil))

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"status":"alive"`) {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

func TestReadinessReportsDependencyFailure(t *testing.T) {
	handler := testHandler(fakeChecker{}, fakeChecker{err: errors.New("down")})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/health/ready", nil))

	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), `"redis":"down"`) {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

func TestReadinessSucceedsWhenDependenciesAreUp(t *testing.T) {
	handler := testHandler(fakeChecker{}, fakeChecker{})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/health/ready", nil))

	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"status":"ready"`) {
		t.Fatalf("response = %d %s", response.Code, response.Body.String())
	}
}

func testHandler(postgres, redis Checker) http.Handler {
	mux := http.NewServeMux()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	New(postgres, redis, logger).Register(mux)
	return middleware.Correlation(logger, mux)
}
