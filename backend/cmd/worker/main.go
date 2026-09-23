package main

import (
	"context"
	"os"
	"time"

	"vps-billing/backend/internal/config"
	"vps-billing/backend/internal/infrastructure"
	"vps-billing/backend/internal/infrastructure/postgres"
	"vps-billing/backend/internal/infrastructure/rediscache"
	"vps-billing/backend/internal/operation"
	"vps-billing/backend/internal/outbox"
	platformruntime "vps-billing/backend/internal/platform/runtime"
	"vps-billing/backend/internal/provider"
	"vps-billing/backend/internal/provider/lxdapi"
	providermock "vps-billing/backend/internal/provider/mock"
	"vps-billing/backend/internal/provision"
	"vps-billing/backend/internal/subscription"
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
	operationRepository := operation.NewPostgresRepository(postgresClient.Pool())
	workflowRegistry := operation.NewWorkflowRegistry()
	infrastructureRepository := infrastructure.NewPostgresRepository(postgresClient.Pool())
	providerRegistry := provider.NewDynamicRegistry(postgresClient.Pool())
	if err := providerRegistry.RegisterFactory("mock", func(provider.FactoryConfig) (provider.Provider, error) { return providermock.New(), nil }); err != nil {
		logger.Error("mock provider registration failed", "error", err)
		os.Exit(1)
	}
	if err := providerRegistry.RegisterFactory("lxdapi", lxdapi.NewFromFactory); err != nil {
		logger.Error("LXD provider registration failed", "error", err)
		os.Exit(1)
	}
	provisionRepository := provision.NewRepository(postgresClient.Pool())
	if err := workflowRegistry.Register("provision", provision.NewWorkflow(provisionRepository, infrastructure.NewScheduler(infrastructureRepository), providerRegistry)); err != nil {
		logger.Error("provision workflow registration failed", "error", err)
		os.Exit(1)
	}
	consumerName, hostnameErr := os.Hostname()
	if hostnameErr != nil || consumerName == "" {
		consumerName = "worker"
	}
	worker := workerapp.New(logger,
		outbox.NewDispatcher(postgresClient.Pool(), redisClient.Raw()),
		provision.NewTriggerConsumer(provisionRepository, redisClient.Raw(), consumerName),
		operation.NewRetryScheduler(operationRepository),
		operation.NewQueueConsumer(operationRepository, redisClient.Raw(), workflowRegistry, consumerName),
		subscription.NewLifecycleProcessor(postgresClient.Pool(), settings.SubscriptionGracePeriod),
	)
	ctx, stop := platformruntime.SignalContext(context.Background())
	defer stop()

	if err := worker.Run(ctx); err != nil {
		logger.Error("worker failed", "error", err)
		os.Exit(1)
	}
}
