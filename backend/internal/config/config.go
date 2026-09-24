package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"time"
)

const defaultServerAddress = ":8080"

type Config struct {
	Environment              string
	ServerAddress            string
	DatabaseURL              string
	RedisURL                 string
	UserWebOrigin            string
	AdminWebOrigin           string
	UserSessionSecret        string
	AdminSessionSecret       string
	UserCSRFSecret           string
	AdminCSRFSecret          string
	AdminTOTPEncryptionKey   string
	FakePaymentWebhookSecret string
	MetricsToken             string
	FakePaymentEnabled       bool
	SessionTTL               time.Duration
	SubscriptionGracePeriod  time.Duration
	CookieSecure             bool
	AllowInsecureHTTP        bool
}

func Load() (Config, error) {
	config := Config{
		Environment:              valueOrDefault("APP_ENV", "development"),
		ServerAddress:            valueOrDefault("SERVER_ADDRESS", defaultServerAddress),
		DatabaseURL:              os.Getenv("DATABASE_URL"),
		RedisURL:                 os.Getenv("REDIS_URL"),
		UserWebOrigin:            valueOrDefault("USER_WEB_ORIGIN", "http://localhost:3000"),
		AdminWebOrigin:           valueOrDefault("ADMIN_WEB_ORIGIN", "http://localhost:3001"),
		UserSessionSecret:        os.Getenv("USER_SESSION_SECRET"),
		AdminSessionSecret:       os.Getenv("ADMIN_SESSION_SECRET"),
		UserCSRFSecret:           os.Getenv("USER_CSRF_SECRET"),
		AdminCSRFSecret:          os.Getenv("ADMIN_CSRF_SECRET"),
		AdminTOTPEncryptionKey:   os.Getenv("ADMIN_TOTP_ENCRYPTION_KEY"),
		FakePaymentWebhookSecret: os.Getenv("FAKE_PAYMENT_WEBHOOK_SECRET"),
		MetricsToken:             os.Getenv("METRICS_TOKEN"),
		SessionTTL:               24 * time.Hour,
		SubscriptionGracePeriod:  72 * time.Hour,
	}
	gracePeriod, err := time.ParseDuration(valueOrDefault("SUBSCRIPTION_GRACE_PERIOD", "72h"))
	if err != nil || gracePeriod <= 0 {
		return Config{}, fmt.Errorf("configuration: SUBSCRIPTION_GRACE_PERIOD must be a positive duration")
	}
	config.SubscriptionGracePeriod = gracePeriod
	secure, err := strconv.ParseBool(valueOrDefault("COOKIE_SECURE", "false"))
	if err != nil {
		return Config{}, fmt.Errorf("configuration: COOKIE_SECURE: %w", err)
	}
	config.CookieSecure = secure
	allowInsecureHTTP, err := strconv.ParseBool(valueOrDefault("ALLOW_INSECURE_HTTP", "false"))
	if err != nil {
		return Config{}, fmt.Errorf("configuration: ALLOW_INSECURE_HTTP: %w", err)
	}
	config.AllowInsecureHTTP = allowInsecureHTTP
	if config.Environment == "production" && !config.AllowInsecureHTTP && !config.CookieSecure {
		return Config{}, ErrInsecureProductionCookie
	}
	fakeDefault := "true"
	if config.Environment == "production" {
		fakeDefault = "false"
	}
	fakeEnabled, err := strconv.ParseBool(valueOrDefault("FAKE_PAYMENT_ENABLED", fakeDefault))
	if err != nil {
		return Config{}, fmt.Errorf("configuration: FAKE_PAYMENT_ENABLED: %w", err)
	}
	config.FakePaymentEnabled = fakeEnabled
	if config.Environment == "production" {
		if config.FakePaymentEnabled {
			return Config{}, ErrFakePaymentInProduction
		}
		if config.AllowInsecureHTTP {
			if !exactWebOrigin(config.UserWebOrigin) || !exactWebOrigin(config.AdminWebOrigin) {
				return Config{}, ErrInvalidProductionOrigin
			}
		} else if !secureHTTPSOrigin(config.UserWebOrigin) || !secureHTTPSOrigin(config.AdminWebOrigin) {
			return Config{}, ErrInsecureProductionOrigin
		}
	}

	var missing []string
	if config.DatabaseURL == "" {
		missing = append(missing, "DATABASE_URL")
	}
	if config.RedisURL == "" {
		missing = append(missing, "REDIS_URL")
	}
	for key, value := range map[string]string{
		"USER_SESSION_SECRET":         config.UserSessionSecret,
		"ADMIN_SESSION_SECRET":        config.AdminSessionSecret,
		"USER_CSRF_SECRET":            config.UserCSRFSecret,
		"ADMIN_CSRF_SECRET":           config.AdminCSRFSecret,
		"ADMIN_TOTP_ENCRYPTION_KEY":   config.AdminTOTPEncryptionKey,
		"FAKE_PAYMENT_WEBHOOK_SECRET": config.FakePaymentWebhookSecret,
		"METRICS_TOKEN":               config.MetricsToken,
	} {
		if len(value) < 32 {
			missing = append(missing, key+" (minimum 32 characters)")
		}
	}
	if len(missing) > 0 {
		return Config{}, fmt.Errorf("configuration: %w: %v", ErrMissingEnvironment, missing)
	}
	secrets := []string{config.UserSessionSecret, config.AdminSessionSecret, config.UserCSRFSecret, config.AdminCSRFSecret, config.AdminTOTPEncryptionKey, config.FakePaymentWebhookSecret, config.MetricsToken}
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
var ErrInsecureProductionCookie = errors.New("COOKIE_SECURE must be true in production unless ALLOW_INSECURE_HTTP is enabled")
var ErrFakePaymentInProduction = errors.New("FAKE_PAYMENT_ENABLED must be false in production")
var ErrInsecureProductionOrigin = errors.New("production web origins must be exact HTTPS origins")
var ErrInvalidProductionOrigin = errors.New("production web origins must be exact HTTP or HTTPS origins")

func secureHTTPSOrigin(value string) bool {
	parsed, err := url.Parse(value)
	return err == nil && parsed.Scheme == "https" && exactParsedOrigin(parsed)
}

func exactWebOrigin(value string) bool {
	parsed, err := url.Parse(value)
	return err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") && exactParsedOrigin(parsed)
}

func exactParsedOrigin(parsed *url.URL) bool {
	return parsed.Host != "" && parsed.Path == "" && parsed.RawQuery == "" && parsed.Fragment == "" && parsed.User == nil
}

func valueOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
