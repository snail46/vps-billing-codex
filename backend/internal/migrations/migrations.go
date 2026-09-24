package migrations

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"net/url"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

//go:embed sql/*.sql
var migrationFiles embed.FS

func Run(ctx context.Context, databaseURL string) (resultErr error) {
	sourceDriver, err := iofs.New(migrationFiles, "sql")
	if err != nil {
		return fmt.Errorf("open embedded migrations: %w", err)
	}

	databaseURL, err = pgxMigrationURL(databaseURL)
	if err != nil {
		return err
	}
	runner, err := migrate.NewWithSourceInstance("iofs", sourceDriver, databaseURL)
	if err != nil {
		return errors.Join(fmt.Errorf("create migration runner: %w", err), sourceDriver.Close())
	}
	defer func() {
		sourceErr, databaseErr := runner.Close()
		if sourceErr != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("close migration source: %w", sourceErr))
		}
		if databaseErr != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("close migration database: %w", databaseErr))
		}
	}()

	stop := make(chan struct{})
	go func() {
		select {
		case <-ctx.Done():
			select {
			case runner.GracefulStop <- true:
			case <-stop:
			}
		case <-stop:
		}
	}()
	defer close(stop)

	if err := runner.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("apply migrations: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("migration context: %w", err)
	}
	return nil
}

func pgxMigrationURL(databaseURL string) (string, error) {
	parsed, err := url.Parse(databaseURL)
	if err != nil {
		return "", fmt.Errorf("parse migration database URL: %w", err)
	}
	if parsed.Scheme != "postgres" && parsed.Scheme != "postgresql" {
		return "", fmt.Errorf("unsupported migration database scheme %q", parsed.Scheme)
	}
	parsed.Scheme = "pgx5"
	return parsed.String(), nil
}
