package migrations

import (
	"errors"
	"io"
	"testing"

	"github.com/golang-migrate/migrate/v4/source/iofs"
)

func TestEmbeddedMigrationPairs(t *testing.T) {
	driver, err := iofs.New(migrationFiles, "sql")
	if err != nil {
		t.Fatalf("iofs.New() error = %v", err)
	}
	defer func() {
		if err := driver.Close(); err != nil {
			t.Errorf("Close() error = %v", err)
		}
	}()

	for version := uint(1); version <= 12; version++ {
		up, _, readErr := driver.ReadUp(version)
		if readErr != nil {
			t.Fatalf("ReadUp(%d) error = %v", version, readErr)
		}
		if contentErr := requireContents(up); contentErr != nil {
			t.Fatalf("up migration %d: %v", version, contentErr)
		}
		down, _, readErr := driver.ReadDown(version)
		if readErr != nil {
			t.Fatalf("ReadDown(%d) error = %v", version, readErr)
		}
		if contentErr := requireContents(down); contentErr != nil {
			t.Fatalf("down migration %d: %v", version, contentErr)
		}
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
