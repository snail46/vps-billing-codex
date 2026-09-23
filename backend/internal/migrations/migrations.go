package migrations

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
)

const advisoryLockID int64 = 8_675_309_421

//go:embed sql/*.sql
var migrationFiles embed.FS

func Run(ctx context.Context, databaseURL string) (resultErr error) {
	connection, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		return fmt.Errorf("connect for migrations: %w", err)
	}
	defer func() {
		if err := connection.Close(context.Background()); err != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("close migration connection: %w", err))
		}
	}()

	if _, err := connection.Exec(ctx, "SELECT pg_advisory_lock($1)", advisoryLockID); err != nil {
		return fmt.Errorf("acquire migration lock: %w", err)
	}
	defer func() {
		if _, err := connection.Exec(context.Background(), "SELECT pg_advisory_unlock($1)", advisoryLockID); err != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("release migration lock: %w", err))
		}
	}()

	if _, err := connection.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version bigint PRIMARY KEY,
			name text NOT NULL,
			applied_at timestamptz NOT NULL DEFAULT now()
		)
	`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	migrations, err := load()
	if err != nil {
		return err
	}
	for _, migration := range migrations {
		var applied bool
		if err := connection.QueryRow(
			ctx,
			"SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = $1)",
			migration.version,
		).Scan(&applied); err != nil {
			return fmt.Errorf("check migration %d: %w", migration.version, err)
		}
		if applied {
			continue
		}
		if err := apply(ctx, connection, migration); err != nil {
			return err
		}
	}
	return nil
}

type migration struct {
	version int64
	name    string
	sql     string
}

func load() ([]migration, error) {
	entries, err := fs.ReadDir(migrationFiles, "sql")
	if err != nil {
		return nil, fmt.Errorf("read embedded migrations: %w", err)
	}
	sort.Slice(entries, func(left, right int) bool {
		return entries[left].Name() < entries[right].Name()
	})

	migrations := make([]migration, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".sql" {
			continue
		}
		parts := strings.SplitN(entry.Name(), "_", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid migration filename %q", entry.Name())
		}
		version, err := strconv.ParseInt(parts[0], 10, 64)
		if err != nil {
			return nil, fmt.Errorf("parse migration version %q: %w", entry.Name(), err)
		}
		contents, err := migrationFiles.ReadFile("sql/" + entry.Name())
		if err != nil {
			return nil, fmt.Errorf("read migration %q: %w", entry.Name(), err)
		}
		migrations = append(migrations, migration{version: version, name: entry.Name(), sql: string(contents)})
	}
	if len(migrations) == 0 {
		return nil, fmt.Errorf("no migrations found")
	}
	return migrations, nil
}

type beginner interface {
	Begin(context.Context) (pgx.Tx, error)
}

func apply(ctx context.Context, connection beginner, migration migration) (resultErr error) {
	transaction, err := connection.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin migration %d: %w", migration.version, err)
	}
	committed := false
	defer func() {
		if !committed {
			if err := transaction.Rollback(context.Background()); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
				resultErr = errors.Join(resultErr, fmt.Errorf("roll back migration %d: %w", migration.version, err))
			}
		}
	}()
	if _, err := transaction.Exec(ctx, migration.sql); err != nil {
		return fmt.Errorf("execute migration %d: %w", migration.version, err)
	}
	if _, err := transaction.Exec(
		ctx,
		"INSERT INTO schema_migrations (version, name) VALUES ($1, $2)",
		migration.version,
		migration.name,
	); err != nil {
		return fmt.Errorf("record migration %d: %w", migration.version, err)
	}
	if err := transaction.Commit(ctx); err != nil {
		return fmt.Errorf("commit migration %d: %w", migration.version, err)
	}
	committed = true
	return nil
}
