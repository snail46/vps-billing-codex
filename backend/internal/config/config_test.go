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
	t.Setenv("USER_SESSION_SECRET", "user-session-secret-at-least-32-chars")
	t.Setenv("ADMIN_SESSION_SECRET", "admin-session-secret-at-least-32-chars")
	t.Setenv("USER_CSRF_SECRET", "user-csrf-secret-at-least-32-chars")
	t.Setenv("ADMIN_CSRF_SECRET", "admin-csrf-secret-at-least-32-chars")
	t.Setenv("ADMIN_TOTP_ENCRYPTION_KEY", "admin-totp-encryption-at-least-32-char")
	t.Setenv("FAKE_PAYMENT_WEBHOOK_SECRET", "fake-payment-webhook-secret-32-chars")

	config, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if config.Environment != "test" || config.ServerAddress != "127.0.0.1:8081" {
		t.Fatalf("Load() config = %#v", config)
	}
	if config.SubscriptionGracePeriod.Hours() != 72 {
		t.Fatalf("SubscriptionGracePeriod = %v", config.SubscriptionGracePeriod)
	}
}

func TestLoadRejectsInvalidSubscriptionGracePeriod(t *testing.T) {
	setRequiredEnvironment(t)
	t.Setenv("SUBSCRIPTION_GRACE_PERIOD", "0s")
	if _, err := Load(); err == nil {
		t.Fatal("Load() accepted a non-positive subscription grace period")
	}
}

func TestLoadRejectsReusedSecrets(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://database")
	t.Setenv("REDIS_URL", "redis://cache")
	for _, key := range []string{"USER_SESSION_SECRET", "ADMIN_SESSION_SECRET", "USER_CSRF_SECRET", "ADMIN_CSRF_SECRET", "ADMIN_TOTP_ENCRYPTION_KEY", "FAKE_PAYMENT_WEBHOOK_SECRET"} {
		t.Setenv(key, "same-secret-value-that-is-at-least-32-characters")
	}
	_, err := Load()
	if !errors.Is(err, ErrSecretsNotDistinct) {
		t.Fatalf("Load() error = %v", err)
	}
}

func TestLoadRequiresDependencies(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("REDIS_URL", "")
	t.Setenv("USER_SESSION_SECRET", "")
	t.Setenv("ADMIN_SESSION_SECRET", "")
	t.Setenv("USER_CSRF_SECRET", "")
	t.Setenv("ADMIN_CSRF_SECRET", "")
	t.Setenv("ADMIN_TOTP_ENCRYPTION_KEY", "")
	t.Setenv("FAKE_PAYMENT_WEBHOOK_SECRET", "")

	_, err := Load()
	if !errors.Is(err, ErrMissingEnvironment) {
		t.Fatalf("Load() error = %v, want ErrMissingEnvironment", err)
	}
}

func setRequiredEnvironment(t *testing.T) {
	t.Helper()
	t.Setenv("DATABASE_URL", "postgres://database")
	t.Setenv("REDIS_URL", "redis://cache")
	t.Setenv("USER_SESSION_SECRET", "user-session-secret-at-least-32-chars")
	t.Setenv("ADMIN_SESSION_SECRET", "admin-session-secret-at-least-32-chars")
	t.Setenv("USER_CSRF_SECRET", "user-csrf-secret-at-least-32-chars")
	t.Setenv("ADMIN_CSRF_SECRET", "admin-csrf-secret-at-least-32-chars")
	t.Setenv("ADMIN_TOTP_ENCRYPTION_KEY", "admin-totp-encryption-at-least-32-char")
	t.Setenv("FAKE_PAYMENT_WEBHOOK_SECRET", "fake-payment-webhook-secret-32-chars")
}
