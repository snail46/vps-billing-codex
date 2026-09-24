package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCORSRejectsUnknownOrigin(t *testing.T) {
	nextCalled := false
	handler := CORS([]string{"https://user.example.com"}, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { nextCalled = true }))
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
	request.Header.Set("Origin", "https://attacker.example")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden || nextCalled {
		t.Fatalf("status = %d, nextCalled = %v", response.Code, nextCalled)
	}
}

func TestCORSAllowsConfiguredOrigin(t *testing.T) {
	handler := CORS([]string{"https://user.example.com"}, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) }))
	request := httptest.NewRequest(http.MethodOptions, "/api/v1/auth/login", nil)
	request.Header.Set("Origin", "https://user.example.com")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent || response.Header().Get("Access-Control-Allow-Origin") != "https://user.example.com" {
		t.Fatalf("response = %#v", response)
	}
}
