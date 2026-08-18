// Package db owns the schema: the embedded SQL migrations and a runner that
// applies them. Both the app (via cmd/migrate) and the integration tests use
// Migrate so there is a single migration path.
package db

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/golang-migrate/migrate/v4"
	// The pgx/v5 migrate driver, not database/postgres: the latter hard-imports
	// github.com/lib/pq, which is unmaintained (last release v1.10.9, 2023) and
	// carries five unfixable advisories. Same Postgres, driven through pgx —
	// which the app already uses for its runtime pool.
	pgxmigrate "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

// Migrate applies all pending up migrations against sqlDB. It is idempotent:
// running it when the schema is already current is a no-op.
func Migrate(sqlDB *sql.DB) error {
	src, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("load embedded migrations: %w", err)
	}

	driver, err := pgxmigrate.WithInstance(sqlDB, &pgxmigrate.Config{})
	if err != nil {
		return fmt.Errorf("build postgres driver: %w", err)
	}

	m, err := migrate.NewWithInstance("iofs", src, "postgres", driver)
	if err != nil {
		return fmt.Errorf("init migrator: %w", err)
	}

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("apply migrations: %w", err)
	}
	return nil
}
