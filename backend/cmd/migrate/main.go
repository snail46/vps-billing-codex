package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"vps-billing/backend/internal/migrations"
	platformruntime "vps-billing/backend/internal/platform/runtime"
)

const migrationTimeout = 5 * time.Minute

func main() {
	logger := platformruntime.Logger("migrate")
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		logger.Error("migration configuration failed", "error", "DATABASE_URL is required")
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), migrationTimeout)
	defer cancel()
	if err := run(ctx, databaseURL, logger); err != nil {
		logger.Error("migration failed", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, databaseURL string, logger *slog.Logger) error {
	logger.Info("migration started")
	if err := migrations.Run(ctx, databaseURL); err != nil {
		return fmt.Errorf("run migrations: %w", err)
	}
	logger.Info("migration completed")
	return nil
}
