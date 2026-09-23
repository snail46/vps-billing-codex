package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"
)

const defaultServerAddress = ":8080"

type Config struct {
	Environment            string
	ServerAddress          string
	DatabaseURL            string
	RedisURL               string
	UserWebOrigin          string
	AdminWebOrigin         string
	UserSessionSecret      string
	AdminSessionSecret     string
	UserCSRFSecret         string
	AdminCSRFSecret        string
	AdminTOTPEncryptionKey string
	SessionTTL             time.Duration
	CookieSecure           bool
}

func Load() (Config, error) {
	config := Config{
		Environment:            valueOrDefault("APP_ENV", "development"),
		ServerAddress:          valueOrDefault("SERVER_ADDRESS", defaultServerAddress),
		DatabaseURL:            os.Getenv("DATABASE_URL"),
		RedisURL:               os.Getenv("REDIS_URL"),
		UserWebOrigin:          valueOrDefault("USER_WEB_ORIGIN", "http://localhost:3000"),
		AdminWebOrigin:         valueOrDefault("ADMIN_WEB_ORIGIN", "http://localhost:3001"),
		UserSessionSecret:      os.Getenv("USER_SESSION_SECRET"),
		AdminSessionSecret:     os.Getenv("ADMIN_SESSION_SECRET"),
		UserCSRFSecret:         os.Getenv("USER_CSRF_SECRET"),
		AdminCSRFSecret:        os.Getenv("ADMIN_CSRF_SECRET"),
		AdminTOTPEncryptionKey: os.Getenv("ADMIN_TOTP_ENCRYPTION_KEY"),
		SessionTTL:             24 * time.Hour,
	}
	secure, err := strconv.ParseBool(valueOrDefault("COOKIE_SECURE", "false"))
	if err != nil {
		return Config{}, fmt.Errorf("configuration: COOKIE_SECURE: %w", err)
	}
	config.CookieSecure = secure
	if config.Environment == "production" && !config.CookieSecure {
		return Config{}, ErrInsecureProductionCookie
	}

	var missing []string
	if config.DatabaseURL == "" {
		missing = append(missing, "DATABASE_URL")
	}
	if config.RedisURL == "" {
		missing = append(missing, "REDIS_URL")
	}
	for key, value := range map[string]string{
		"USER_SESSION_SECRET":       config.UserSessionSecret,
		"ADMIN_SESSION_SECRET":      config.AdminSessionSecret,
		"USER_CSRF_SECRET":          config.UserCSRFSecret,
		"ADMIN_CSRF_SECRET":         config.AdminCSRFSecret,
		"ADMIN_TOTP_ENCRYPTION_KEY": config.AdminTOTPEncryptionKey,
	} {
		if len(value) < 32 {
			missing = append(missing, key+" (minimum 32 characters)")
		}
	}
	if len(missing) > 0 {
		return Config{}, fmt.Errorf("configuration: %w: %v", ErrMissingEnvironment, missing)
	}
	secrets := []string{config.UserSessionSecret, config.AdminSessionSecret, config.UserCSRFSecret, config.AdminCSRFSecret, config.AdminTOTPEncryptionKey}
	seen := make(map[string]struct{}, len(secrets))
	for _, secret := range secrets {
		if _, exists := seen[secret]; exists {
			return Config{}, ErrSecretsNotDistinct
		}
		seen[secret] = struct{}{}
	}
	return config, nil
}

var ErrMissingEnvironment = errors.New("required environment variable is missing")
var ErrSecretsNotDistinct = errors.New("identity secrets must be distinct")
var ErrInsecureProductionCookie = errors.New("COOKIE_SECURE must be true in production")

func valueOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
