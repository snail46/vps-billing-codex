package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/go-chi/chi/v5"

	"vps-billing/backend/internal/admin"
	"vps-billing/backend/internal/audit"
	"vps-billing/backend/internal/commerce"
	"vps-billing/backend/internal/config"
	adminhttp "vps-billing/backend/internal/http/admin"
	commercehttp "vps-billing/backend/internal/http/commerce"
	"vps-billing/backend/internal/http/health"
	identityhttp "vps-billing/backend/internal/http/identity"
	metricshttp "vps-billing/backend/internal/http/metrics"
	"vps-billing/backend/internal/http/middleware"
	operationhttp "vps-billing/backend/internal/http/operation"
	portalhttp "vps-billing/backend/internal/http/portal"
	subscriptionhttp "vps-billing/backend/internal/http/subscription"
	"vps-billing/backend/internal/identity"
	"vps-billing/backend/internal/infrastructure/postgres"
	"vps-billing/backend/internal/infrastructure/rediscache"
	"vps-billing/backend/internal/operation"
	"vps-billing/backend/internal/payment/fake"
	platformruntime "vps-billing/backend/internal/platform/runtime"
	"vps-billing/backend/internal/portal"
	"vps-billing/backend/internal/security/ratelimit"
	serverapp "vps-billing/backend/internal/server"
	"vps-billing/backend/internal/subscription"
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
	metricsCollector := metricshttp.NewCollector()
	metricshttp.New(metricsCollector, postgresClient.Pool(), settings.MetricsToken).Register(router)
	identityService := identity.NewService(identity.NewPostgresRepository(postgresClient.Pool()), settings.UserSessionSecret, settings.AdminSessionSecret, settings.UserCSRFSecret, settings.AdminCSRFSecret, settings.AdminTOTPEncryptionKey, settings.SessionTTL)
	identityHandler := identityhttp.New(identityService, ratelimit.New(redisClient.Raw(), "auth:"), audit.NewPostgresRecorder(postgresClient.Pool()), settings.CookieSecure)
	identityHandler.Register(router)
	commerceService := commerce.NewService(commerce.NewPostgresRepository(postgresClient.Pool()))
	commercehttp.New(commerceService, identityHandler, fake.New(settings.FakePaymentWebhookSecret), settings.FakePaymentEnabled).Register(router)
	subscriptionService := subscription.NewService(subscription.NewPostgresRepository(postgresClient.Pool()))
	subscriptionhttp.New(subscriptionService, commerceService, identityHandler).Register(router)
	operationService := operation.NewService(operation.NewPostgresRepository(postgresClient.Pool()))
	operationhttp.New(operationService, identityHandler, redisClient.Raw()).Register(router)
	portalService := portal.NewService(portal.NewPostgresRepository(postgresClient.Pool()))
	portalhttp.New(portalService, operationService, identityHandler).Register(router)
	adminhttp.New(admin.NewService(admin.NewPostgresRepository(postgresClient.Pool())), identityHandler).Register(router)
	handler := middleware.Correlation(logger, metricsCollector.Instrument(middleware.AccessLog(logger, middleware.SecurityHeaders(settings.CookieSecure, middleware.CORS([]string{settings.UserWebOrigin, settings.AdminWebOrigin}, router)))))

	server, err := serverapp.New(settings.ServerAddress, handler, logger)
	if err != nil {
		return err
	}

	ctx, stop := platformruntime.SignalContext(context.Background())
	defer stop()
	return server.Run(ctx)
}
