package main

import (
	"context"
	"os"

	platformruntime "vps-billing/backend/internal/platform/runtime"
	workerapp "vps-billing/backend/internal/worker"
)

func main() {
	logger := platformruntime.Logger("worker")
	worker := workerapp.New(logger)
	ctx, stop := platformruntime.SignalContext(context.Background())
	defer stop()

	if err := worker.Run(ctx); err != nil {
		logger.Error("worker failed", "error", err)
		os.Exit(1)
	}
}
