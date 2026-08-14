package db_test

import (
	"database/sql"
	"os"
	"testing"

	_ "github.com/lib/pq"
)

// The database the integration suite runs against is built two different ways: CI creates a
// throwaway `postgres:17-alpine` pod (.gitea/ci/postgres.yaml), and the sandbox devcontainer
// starts a loopback server that was initdb'd at image build time. Two mechanisms, two repos,
// no shared declaration — so nothing stops them drifting apart, and a drift is precisely the
// thing that makes a green local gate lie about CI.
//
// These constants are that shared declaration, asserted at run time on whichever database
// the caller supplied. If the sandbox image bumps to PG 18, the local gate fails; if CI's pod
// image moves, the CI job fails. Either way it is a named failure rather than a silent
// behaviour change.
//
// Asserted here, in Go, rather than in dev/integration-test.sh: the CI runner image ships no
// postgresql-client, so a psql-based check would need a new runner dependency — and a check
// living inside the suite also cannot be skipped by someone running `go test` directly.
const (
	wantPGMajor  = 17
	wantEncoding = "UTF8"

	// "C" is a deliberate choice, not a default. The only collation-sensitive ORDER BY in the
	// codebase is db/queries/exercises.sql (`ORDER BY name`), and it sorts a closed set of five
	// seed rows — exercises are written once by 0002_seed.up.sql and have no API write path —
	// which order identically under C and under a linguistic locale. So C is safe here, and it
	// lets the sandbox image skip locales/locale-gen entirely.
	//
	// If THIS is what failed and the actual value came from CI, CI's image is the source of
	// truth: change this constant to match and re-run. See homelab-gitops
	// docs/sandbox/iron-temple-postgres-plan.md §8.1.
	wantCollate = "C"
)

// TestDatabaseParity pins the properties of the test database that could silently change
// query results between the local gate and CI.
func TestDatabaseParity(t *testing.T) {
	if testing.Short() {
		t.Skip("no database in -short mode")
	}
	// Only the two pinned paths are asserted. With TEST_DATABASE_URL unset the suite falls back
	// to Testcontainers, which is a developer-laptop path neither gate uses — holding it to
	// these constants would fail runs that are not claiming CI parity in the first place.
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL unset (Testcontainers path) — parity constants do not apply")
	}

	sqlDB, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer func() { _ = sqlDB.Close() }()

	var versionNum int
	if err := sqlDB.QueryRow("SHOW server_version_num").Scan(&versionNum); err != nil {
		t.Fatalf("read server_version_num: %v", err)
	}

	var encoding, collate, ctype string
	const q = `SELECT pg_encoding_to_char(encoding), datcollate, datctype
	           FROM pg_database WHERE datname = current_database()`
	if err := sqlDB.QueryRow(q).Scan(&encoding, &collate, &ctype); err != nil {
		t.Fatalf("read database properties: %v", err)
	}

	// Logged unconditionally: a passing run still records what this side actually is, which is
	// how the other side's values get confirmed without a separate probe.
	major := versionNum / 10000
	t.Logf("test database: pg_major=%d server_version_num=%d encoding=%s collate=%s ctype=%s",
		major, versionNum, encoding, collate, ctype)

	if major != wantPGMajor {
		t.Errorf("Postgres major version %d, want %d — the sandbox image (homelab-gitops "+
			"infrastructure/sandbox/devcontainer/Dockerfile PG_MAJOR) and CI's pod image "+
			"(.gitea/ci/postgres.yaml) must match", major, wantPGMajor)
	}
	if encoding != wantEncoding {
		t.Errorf("database encoding %q, want %q", encoding, wantEncoding)
	}
	if collate != wantCollate {
		t.Errorf("database collation %q, want %q — see the constant's comment before changing it",
			collate, wantCollate)
	}
	if ctype != wantCollate {
		t.Errorf("database ctype %q, want %q", ctype, wantCollate)
	}
}
