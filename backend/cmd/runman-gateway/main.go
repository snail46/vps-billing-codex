package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"os"
	"strconv"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"

	"vps-billing/backend/internal/config"
	"vps-billing/backend/internal/infrastructure/postgres"
	platformruntime "vps-billing/backend/internal/platform/runtime"
	"vps-billing/backend/internal/runman"
	pb "vps-billing/backend/internal/runman/proto"
)

func main() {
	logger := platformruntime.Logger("runman-gateway")
	settings, err := config.Load()
	if err != nil {
		logger.Error("gateway configuration failed", "error", err)
		os.Exit(1)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	database, err := postgres.Open(ctx, settings.DatabaseURL)
	cancel()
	if err != nil {
		logger.Error("gateway PostgreSQL failed", "error", err)
		os.Exit(1)
	}
	defer database.Close()

	options, err := transportOptions(settings.Environment)
	if err != nil {
		logger.Error("gateway transport configuration failed", "error", err)
		os.Exit(1)
	}
	address := envOrDefault("RUNMAN_GATEWAY_ADDRESS", ":9090")
	listener, err := net.Listen("tcp", address)
	if err != nil {
		logger.Error("gateway listen failed", "error", err)
		os.Exit(1)
	}
	server := grpc.NewServer(options...)
	pb.RegisterAgentGatewayServer(server, runman.NewGateway(runman.NewPostgresStore(database.Pool())))
	healthServer := health.NewServer()
	healthServer.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	healthpb.RegisterHealthServer(server, healthServer)

	shutdown, stop := platformruntime.SignalContext(context.Background())
	defer stop()
	go func() {
		<-shutdown.Done()
		healthServer.Shutdown()
		server.GracefulStop()
	}()
	logger.Info("runman gateway listening", "address", address, "tls", len(options) > 0)
	if err = server.Serve(listener); err != nil {
		logger.Error("gateway stopped", "error", err)
		os.Exit(1)
	}
}

func transportOptions(environment string) ([]grpc.ServerOption, error) {
	certFile, keyFile := os.Getenv("RUNMAN_TLS_CERT_FILE"), os.Getenv("RUNMAN_TLS_KEY_FILE")
	if certFile != "" || keyFile != "" {
		if certFile == "" || keyFile == "" {
			return nil, fmt.Errorf("RUNMAN_TLS_CERT_FILE and RUNMAN_TLS_KEY_FILE must be configured together")
		}
		certificate, err := tls.LoadX509KeyPair(certFile, keyFile)
		if err != nil {
			return nil, fmt.Errorf("load gateway certificate: %w", err)
		}
		return []grpc.ServerOption{grpc.Creds(credentials.NewTLS(&tls.Config{Certificates: []tls.Certificate{certificate}, MinVersion: tls.VersionTLS12}))}, nil
	}
	insecure, err := strconv.ParseBool(envOrDefault("RUNMAN_INSECURE", "false"))
	if err != nil {
		return nil, fmt.Errorf("RUNMAN_INSECURE: %w", err)
	}
	if environment == "production" || !insecure {
		return nil, fmt.Errorf("TLS certificate is required; plaintext is only allowed with RUNMAN_INSECURE=true outside production")
	}
	return nil, nil
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
