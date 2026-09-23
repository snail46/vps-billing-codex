package main

import (
	"context"
	"os"
	"time"

	"vps-billing/backend/internal/config"
	"vps-billing/backend/internal/infrastructure/postgres"
	"vps-billing/backend/internal/infrastructure/rediscache"
	"vps-billing/backend/internal/outbox"
	platformruntime "vps-billing/backend/internal/platform/runtime"
	workerapp "vps-billing/backend/internal/worker"
)

func main() {
	logger := platformruntime.Logger("worker")
	settings, err := config.Load()
	if err != nil {
		logger.Error("worker configuration failed", "error", err)
		os.Exit(1)
	}
	startupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	postgresClient, err := postgres.Open(startupCtx, settings.DatabaseURL)
	if err != nil {
		cancel()
		logger.Error("worker PostgreSQL failed", "error", err)
		os.Exit(1)
	}
	defer postgresClient.Close()
	redisClient, err := rediscache.Open(startupCtx, settings.RedisURL)
	cancel()
	if err != nil {
		logger.Error("worker Redis failed", "error", err)
		os.Exit(1)
	}
	defer func() { _ = redisClient.Close() }()
	worker := workerapp.New(logger, outbox.NewDispatcher(postgresClient.Pool(), redisClient.Raw()))
	ctx, stop := platformruntime.SignalContext(context.Background())
	defer stop()

	if err := worker.Run(ctx); err != nil {
		logger.Error("worker failed", "error", err)
		os.Exit(1)
	}
}
