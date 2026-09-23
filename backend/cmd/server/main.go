package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/go-chi/chi/v5"

	"vps-billing/backend/internal/config"
	"vps-billing/backend/internal/http/health"
	"vps-billing/backend/internal/http/middleware"
	"vps-billing/backend/internal/infrastructure/postgres"
	"vps-billing/backend/internal/infrastructure/rediscache"
	platformruntime "vps-billing/backend/internal/platform/runtime"
	serverapp "vps-billing/backend/internal/server"
)

const dependencyStartupTimeout = 10 * time.Second

func main() {
	logger := platformruntime.Logger("server")
	if err := run(logger); err != nil {
		logger.Error("server failed", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	settings, err := config.Load()
	if err != nil {
		return err
	}
	startupCtx, cancelStartup := context.WithTimeout(context.Background(), dependencyStartupTimeout)
	defer cancelStartup()
	postgresClient, err := postgres.Open(startupCtx, settings.DatabaseURL)
	if err != nil {
		return err
	}
	defer postgresClient.Close()
	redisClient, err := rediscache.Open(startupCtx, settings.RedisURL)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := redisClient.Close(); closeErr != nil {
			logger.Error("close Redis client", "error", closeErr)
		}
	}()

	router := chi.NewRouter()
	health.New(postgresClient, redisClient, logger).Register(router)
	handler := middleware.Correlation(logger, middleware.AccessLog(logger, router))

	server, err := serverapp.New(settings.ServerAddress, handler, logger)
	if err != nil {
		return err
	}

	ctx, stop := platformruntime.SignalContext(context.Background())
	defer stop()
	return server.Run(ctx)
}
