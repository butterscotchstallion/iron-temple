package db_test

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
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

	sqlDB, err := sql.Open("pgx", dsn)
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

	// 0002 seeds three programs over five lifts; 0007 adds two more programs and
	// the five variation lifts the Intermediate program needs; 0009 adds the
	// accessory catalogue the exercise library browses.
	assertCount(t, sqlDB, "SELECT count(*) FROM exercises", 53)
	assertCount(t, sqlDB, "SELECT count(*) FROM programs", 5)
	assertCount(t, sqlDB, "SELECT count(*) FROM program_days", 11)
	assertCount(t, sqlDB, "SELECT count(*) FROM program_day_exercises", 31)

	// The split 0009 draws: the ten lifts the programs prescribe are not
	// accessories, and everything it seeded is. Asserted as a split rather than
	// two magic numbers because it is what the library's default view leans on.
	assertCount(t, sqlDB, "SELECT count(*) FROM exercises WHERE NOT is_accessory", 10)
	assertCount(t, sqlDB, "SELECT count(*) FROM exercises WHERE is_accessory", 43)
	// Nothing seeded belongs to a user; every seeded row is shared.
	assertCount(t, sqlDB, "SELECT count(*) FROM exercises WHERE created_by_user_id IS NOT NULL", 0)

	// 0011's three rest tiers. Asserted as a partition — the three counts sum to
	// the 53 above — because the tiers are applied as successive UPDATEs that
	// narrow one another, and the failure mode worth catching is a lift left
	// behind in the tier before, which a spot check of one row would miss.
	assertCount(t, sqlDB, "SELECT count(*) FROM exercises WHERE rest_seconds = 300", 6)
	assertCount(t, sqlDB, "SELECT count(*) FROM exercises WHERE rest_seconds = 180", 23)
	assertCount(t, sqlDB, "SELECT count(*) FROM exercises WHERE rest_seconds = 90", 24)
	// The two ends of the range, named: the lift a five-minute rest exists for,
	// and an isolation movement that must not have inherited one.
	assertRest(t, sqlDB, "Deadlift", 300)
	assertRest(t, sqlDB, "Lateral Raise", 90)
	// An accessory promoted back up to a prescribed lift's rest, which is the
	// tier that only exists because is_accessory alone gets it wrong.
	assertRest(t, sqlDB, "Leg Press", 180)
}

func assertRest(t *testing.T, db *sql.DB, name string, want int) {
	t.Helper()
	var got int
	err := db.QueryRow("SELECT rest_seconds FROM exercises WHERE name = $1", name).Scan(&got)
	if err != nil {
		t.Fatalf("rest_seconds for %q: %v", name, err)
	}
	if got != want {
		t.Errorf("rest_seconds for %q = %d, want %d", name, got, want)
	}
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
