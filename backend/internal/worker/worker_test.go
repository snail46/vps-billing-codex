package worker

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"
)

func TestWorkerStopsWhenContextIsCancelled(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	worker := New(logger)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)

	go func() {
		done <- worker.Run(ctx)
	}()
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
