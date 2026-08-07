package db_test

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	appdb "gitea.homelab/gitadmin/iron-temple/api/db"
)

// TestMigrateAppliesSchemaAndSeed boots a throwaway Postgres via Testcontainers,
// applies the embedded migrations, and asserts the seed landed. Requires a
// Docker-compatible daemon (see deploy/README.md / implementation plan).
func TestMigrateAppliesSchemaAndSeed(t *testing.T) {
	if testing.Short() {
		t.Skip("requires a Docker daemon (Testcontainers)")
	}
	ctx := context.Background()

	// In CI, TEST_DATABASE_URL points at an ephemeral Postgres pod (the host-executor
	// runner has no container runtime for Testcontainers). Locally, with the var unset,
	// boot a throwaway Testcontainers Postgres as before.
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		pg, err := postgres.Run(ctx, "postgres:17-alpine",
			postgres.WithDatabase("iron_temple"),
			postgres.WithUsername("test"),
			postgres.WithPassword("test"),
			testcontainers.WithWaitStrategy(
				wait.ForLog("database system is ready to accept connections").
					WithOccurrence(2).
					WithStartupTimeout(60*time.Second)),
		)
		if err != nil {
			t.Fatalf("start postgres container: %v", err)
		}
		t.Cleanup(func() { _ = pg.Terminate(ctx) })

		dsn, err = pg.ConnectionString(ctx, "sslmode=disable")
		if err != nil {
			t.Fatalf("connection string: %v", err)
		}
	}

	sqlDB, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer func() { _ = sqlDB.Close() }()

	// Migrate is idempotent, so applying twice must also succeed.
	for i := 0; i < 2; i++ {
		if err := appdb.Migrate(sqlDB); err != nil {
			t.Fatalf("migrate (pass %d): %v", i+1, err)
		}
	}

	assertCount(t, sqlDB, "SELECT count(*) FROM exercises", 5)
	assertCount(t, sqlDB, "SELECT count(*) FROM programs", 3)
	assertCount(t, sqlDB, "SELECT count(*) FROM program_days", 6)
	assertCount(t, sqlDB, "SELECT count(*) FROM program_day_exercises", 18)
}

func assertCount(t *testing.T, db *sql.DB, query string, want int) {
	t.Helper()
	var got int
	if err := db.QueryRow(query).Scan(&got); err != nil {
		t.Fatalf("%q: %v", query, err)
	}
	if got != want {
		t.Errorf("%q = %d, want %d", query, got, want)
	}
}
