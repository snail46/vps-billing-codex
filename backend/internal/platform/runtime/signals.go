package runtime

import (
	"context"
	"os"
	"os/signal"
	"syscall"
)

// SignalContext is cancelled when the process receives a termination signal.
func SignalContext(parent context.Context) (context.Context, context.CancelFunc) {
	return signal.NotifyContext(parent, os.Interrupt, syscall.SIGTERM)
}
