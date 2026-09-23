package config

import (
	"errors"
	"fmt"
	"os"
)

const defaultServerAddress = ":8080"

type Config struct {
	Environment   string
	ServerAddress string
	DatabaseURL   string
	RedisURL      string
}

func Load() (Config, error) {
	config := Config{
		Environment:   valueOrDefault("APP_ENV", "development"),
		ServerAddress: valueOrDefault("SERVER_ADDRESS", defaultServerAddress),
		DatabaseURL:   os.Getenv("DATABASE_URL"),
		RedisURL:      os.Getenv("REDIS_URL"),
	}

	var missing []string
	if config.DatabaseURL == "" {
		missing = append(missing, "DATABASE_URL")
	}
	if config.RedisURL == "" {
		missing = append(missing, "REDIS_URL")
	}
	if len(missing) > 0 {
		return Config{}, fmt.Errorf("configuration: %w: %v", ErrMissingEnvironment, missing)
	}
	return config, nil
}

var ErrMissingEnvironment = errors.New("required environment variable is missing")

func valueOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
