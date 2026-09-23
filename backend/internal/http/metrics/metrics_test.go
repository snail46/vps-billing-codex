package metrics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestMetricsRequireTokenAndExposeHTTPMeasurements(t *testing.T) {
	collector := NewCollector()
	measured := collector.Instrument(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusAccepted) }))
	measured.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/api/v1/orders", nil))

	router := chi.NewRouter()
	New(collector, nil, "metrics-token-at-least-32-characters").Register(router)
	unauthorized := httptest.NewRecorder()
	router.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d", unauthorized.Code)
	}

	request := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	request.Header.Set("Authorization", "Bearer metrics-token-at-least-32-characters")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `vps_billing_http_requests_total{method="POST",status="202"} 1`) {
		t.Fatalf("metrics response = %d %q", response.Code, response.Body.String())
	}
}
