package config

import (
	"errors"
	"testing"
)

func TestLoad(t *testing.T) {
	t.Setenv("APP_ENV", "test")
	t.Setenv("SERVER_ADDRESS", "127.0.0.1:8081")
	t.Setenv("DATABASE_URL", "postgres://database")
	t.Setenv("REDIS_URL", "redis://cache")

	config, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if config.Environment != "test" || config.ServerAddress != "127.0.0.1:8081" {
		t.Fatalf("Load() config = %#v", config)
	}
}

func TestLoadRequiresDependencies(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("REDIS_URL", "")

	_, err := Load()
	if !errors.Is(err, ErrMissingEnvironment) {
		t.Fatalf("Load() error = %v, want ErrMissingEnvironment", err)
	}
}
