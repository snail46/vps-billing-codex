package migrations

import (
	"errors"
	"io"
	"testing"

	"github.com/golang-migrate/migrate/v4/source/iofs"
)

func TestEmbeddedMigrationPair(t *testing.T) {
	driver, err := iofs.New(migrationFiles, "sql")
	if err != nil {
		t.Fatalf("iofs.New() error = %v", err)
	}
	defer func() {
		if err := driver.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	}()

	up, _, err := driver.ReadUp(1)
	if err != nil {
		t.Fatalf("ReadUp(1) error = %v", err)
	}
	if err := requireContents(up); err != nil {
		t.Fatalf("up migration: %v", err)
	}
	down, _, err := driver.ReadDown(1)
	if err != nil {
		t.Fatalf("ReadDown(1) error = %v", err)
	}
	if err := requireContents(down); err != nil {
		t.Fatalf("down migration: %v", err)
	}
}

func TestPGXMigrationURL(t *testing.T) {
	got, err := pgxMigrationURL("postgres://user:pass@localhost/database?sslmode=disable")
	if err != nil {
		t.Fatalf("pgxMigrationURL() error = %v", err)
	}
	if got != "pgx5://user:pass@localhost/database?sslmode=disable" {
		t.Fatalf("pgxMigrationURL() = %q", got)
	}
	if _, err := pgxMigrationURL("mysql://localhost/database"); err == nil {
		t.Fatal("pgxMigrationURL() accepted unsupported scheme")
	}
}

func requireContents(reader io.ReadCloser) (resultErr error) {
	defer func() {
		resultErr = errors.Join(resultErr, reader.Close())
	}()
	contents, err := io.ReadAll(reader)
	if err != nil {
		return err
	}
	if len(contents) == 0 {
		return errors.New("migration is empty")
	}
	return nil
}
