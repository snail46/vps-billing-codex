package server

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"testing"
	"time"
)

func TestServerRunsAndShutsDown(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	server, err := New("127.0.0.1:0", handler, logger)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- server.Run(ctx)
	}()

	client := &http.Client{Timeout: time.Second}
	response, err := client.Get("http://" + server.Address())
	if err != nil {
		cancel()
		t.Fatalf("GET server: %v", err)
	}
	if err := response.Body.Close(); err != nil {
		cancel()
		t.Fatalf("close response body: %v", err)
	}
	if response.StatusCode != http.StatusNoContent {
		cancel()
		t.Fatalf("status = %d, want %d", response.StatusCode, http.StatusNoContent)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Run() did not stop after cancellation")
	}
}

func TestNewRejectsInvalidAddress(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	if _, err := New("invalid address", nil, logger); err == nil {
		t.Fatal("New() error = nil, want an address error")
	}
}
