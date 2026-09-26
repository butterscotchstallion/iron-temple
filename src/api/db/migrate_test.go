package db_test

import (
	"context"
	"database/sql"
	"os"
	"strings"
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
	// accessory catalogue the exercise library browses; 0012 adds Madcow 5x5 and
	// 0015 reshapes it from two days to three — nine prescriptions where 0012 had
	// six; 0018 clones Lite with a dumbbell press, for two more days and six more
	// prescriptions.
	//
	// Every one of those prescribes lifts the seed already held, which is why the
	// exercise total stood at 53 through all of them. 0023 is the first that does
	// not: the glutes program needs a barbell hip thrust and four banded
	// movements the catalogue had never heard of, so 53 becomes 58 alongside the
	// eighth program, its two days and its ten prescriptions.
	assertCount(t, sqlDB, "SELECT count(*) FROM exercises", 58)
	assertCount(t, sqlDB, "SELECT count(*) FROM programs", 8)
	assertCount(t, sqlDB, "SELECT count(*) FROM program_days", 18)
	assertCount(t, sqlDB, "SELECT count(*) FROM program_day_exercises", 56)

	// 0023 is the first program to prescribe a lift at 0 lb — four band
	// movements and a bodyweight back extension, which carry no poundage to
	// prescribe. Asserted because it is the precondition for the zero guard in
	// progression.NextPlan being reachable at all: if a later migration quietly
	// gave these a starting weight, the guard would go untested and band work
	// would start creeping 5 lb a session again.
	assertCount(t, sqlDB,
		"SELECT count(*) FROM program_day_exercises WHERE starting_weight_lb = 0", 5)
	// And the kind that needed a new CHECK. Four movements, one program.
	assertCount(t, sqlDB, "SELECT count(*) FROM exercises WHERE equipment = 'band'", 4)

	// Madcow is the only program with per-set prescriptions, and the only one
	// with a progression kind of its own. Every other program is a uniform block
	// of sets x reps at one weight, which is what an absent set plan means.
	assertCount(t, sqlDB,
		"SELECT count(*) FROM programs WHERE progression_kind = 'madcow'", 1)
	// 15 on the volume day (3 lifts x 5), 12 on the light day (a 4-set squat plus
	// a 4-set press and deadlift), 18 on the intensity day (3 lifts x 6: four
	// ramp sets, a triple and a backoff).
	assertCount(t, sqlDB, "SELECT count(*) FROM program_day_exercise_sets", 45)
	// Exactly one day per lift tops out at 100%: that day is the lift's
	// reference, and every other day's weights are a percentage of it. Five
	// lifts, five reference days.
	assertCount(t, sqlDB,
		"SELECT count(*) FROM program_day_exercise_sets WHERE pct_of_top = 100", 5)

	// The split 0009 draws: the lifts the programs prescribe are not accessories,
	// and everything else it seeded is. Asserted as a split rather than two magic
	// numbers because it is what the library's default view leans on — and
	// because the split is what moves when a program picks up a catalogue
	// movement, as 0018 does with the dumbbell press and 0023 does with five
	// more. Twenty-one and thirty-seven where 0018 left eleven and forty-two:
	// 0023 promotes five accessories and seeds five lifts that are prescribed
	// from birth, so the non-accessory side gains ten while the total gains five.
	assertCount(t, sqlDB, "SELECT count(*) FROM exercises WHERE NOT is_accessory", 21)
	assertCount(t, sqlDB, "SELECT count(*) FROM exercises WHERE is_accessory", 37)
	// The promotion itself, named. It must not have dragged the rest tier with
	// it: 0011 had already given this lift a compound's 180s, so 0018 changing
	// what it is classed as must leave what it rests unchanged.
	assertAccessory(t, sqlDB, "Dumbbell Shoulder Press", false)
	assertRest(t, sqlDB, "Dumbbell Shoulder Press", 180)
	// 0023's promotions make the same claim, and one of them is the case the
	// rule is easiest to get wrong. 0011 left the Bulgarian split squat at an
	// accessory's 90s deliberately — "in the family by name but not by load" —
	// and being prescribed does not change what it loads, so the promotion must
	// move is_accessory and nothing else.
	assertAccessory(t, sqlDB, "Bulgarian Split Squat", false)
	assertRest(t, sqlDB, "Bulgarian Split Squat", 90)
	// A lift 0023 seeded rather than promoted, so it must never have been an
	// accessory at all.
	assertAccessory(t, sqlDB, "Barbell Hip Thrust", false)
	assertRest(t, sqlDB, "Barbell Hip Thrust", 180)
	// Nothing seeded belongs to a user; every seeded row is shared.
	assertCount(t, sqlDB, "SELECT count(*) FROM exercises WHERE created_by_user_id IS NOT NULL", 0)

	// 0029 says the same of programs, and it is the assertion that pins the whole
	// custom-program design: a seeded program has NO OWNER, which is what makes
	// it uneditable — every write scopes on created_by_user_id = caller, and NULL
	// matches nobody. If a migration ever gave a seeded program an owner, the
	// install's catalogue would silently become one lifter's to rewrite.
	//
	// Asserted as all three columns at once rather than as a count of NULLs,
	// because the visible-and-live half matters too: is_shared false would hide
	// StrongLifts from the picker, and an archived_at would retire it.
	assertCount(t, sqlDB,
		"SELECT count(*) FROM programs "+
			"WHERE created_by_user_id IS NULL AND is_shared AND archived_at IS NULL", 8)
	// And no day arrives archived — the column exists so a day somebody has
	// trained can leave a program without taking the session with it, not as a
	// state anything ships in.
	assertCount(t, sqlDB, "SELECT count(*) FROM program_days WHERE archived_at IS NULL", 18)

	// 0011's rest tiers, as 0017's three-minute cap leaves them: two, not three.
	// Asserted as a partition — the counts sum to the 58 above — because the
	// tiers are applied as successive UPDATEs that narrow one another, and the
	// failure mode worth catching is a lift left behind in the tier before,
	// which a spot check of one row would miss. The 30 is 0011's 23, plus the six
	// the cap brought down from 300, plus 0023's hip thrust; the 28 is 0011's 24
	// plus 0023's four banded movements.
	assertCount(t, sqlDB, "SELECT count(*) FROM exercises WHERE rest_seconds > 180", 0)
	assertCount(t, sqlDB, "SELECT count(*) FROM exercises WHERE rest_seconds = 180", 30)
	assertCount(t, sqlDB, "SELECT count(*) FROM exercises WHERE rest_seconds = 90", 28)
	// The two ends of the range, named: the lift the five-minute tier used to
	// exist for, now capped, and an isolation movement that must not have
	// inherited a compound's rest.
	assertRest(t, sqlDB, "Deadlift", 180)
	assertRest(t, sqlDB, "Lateral Raise", 90)
	// An accessory promoted back up to a prescribed lift's rest, which is the
	// tier that only exists because is_accessory alone gets it wrong.
	assertRest(t, sqlDB, "Leg Press", 180)
	// 0017 put the cap in the schema, not just in the data. Asserted by trying
	// to break it: the rail is what holds for rows no migration wrote.
	assertRestRejected(t, sqlDB, 181)

	// 0032's catalogue: one crown per leaderboard board, and five boards. Pinned
	// because the reconciler maps a board to a crown THROUGH this table — a board
	// with no row here silently awards nothing, and the only sign would be a
	// leader who never gets a crown.
	assertCount(t, sqlDB, "SELECT count(*) FROM achievements WHERE kind = 'crown'", 5)
	// Every crown names the board it comes from. A NULL metric would make the
	// reconciler skip it, which is the quiet version of the failure above.
	assertCount(t, sqlDB,
		"SELECT count(*) FROM achievements WHERE kind = 'crown' AND metric IS NULL", 0)

	// 0035's rungs, pinned for the crowns' reason turned around: the level
	// reconciler reads the LADDER out of this table, so a missing row is a rung
	// nobody can ever reach and a missing threshold is a row it skips.
	//
	// Both counts are now scoped by kind, which the crown assertions above were not
	// until this migration existed. An unscoped "how many achievements are there"
	// is a number that changes every time the catalogue grows, and it was asserting
	// the size of the table where what it meant was the coverage of the boards.
	assertCount(t, sqlDB, "SELECT count(*) FROM achievements WHERE kind = 'level'", 4)
	assertCount(t, sqlDB,
		"SELECT count(*) FROM achievements WHERE kind = 'level' AND level_threshold IS NULL", 0)
	// The ladder itself, and not merely its size. These four numbers are what the
	// badge means — changing one is a decision about the feature, not a refactor.
	assertCount(t, sqlDB,
		"SELECT count(*) FROM achievements WHERE kind = 'level' AND level_threshold IN (5, 10, 20, 30)", 4)
	// And no other kind claims a threshold, which is the mirror of the metric rule
	// above: level_threshold is a level's column the way metric is a board's.
	assertCount(t, sqlDB,
		"SELECT count(*) FROM achievements WHERE kind <> 'level' AND level_threshold IS NOT NULL", 0)
	// And the ledger ships empty. The crown reigns are deliberately NOT backfilled
	// — see 0032 on why inventing a held_from would be fiction — so a fresh install
	// has no crowns until the first sweeper pass.
	//
	// 0035 DOES backfill, which is what keeps the first pass from announcing rungs
	// everybody passed months ago, and this still holds anyway: it backfills from
	// the sessions, and a fresh install has neither accounts nor sessions to match.
	// That is the case worth pinning here rather than the backfill itself.
	assertCount(t, sqlDB, "SELECT count(*) FROM lifter_achievements", 0)

	// 0033 ships empty too, and for a different reason worth pinning: nothing is
	// lost without a backfill, because a lifter with no follows still hears about
	// their own achievements. A seeded mutual-follow graph would be a migration
	// deciding who trains with whom.
	assertCount(t, sqlDB, "SELECT count(*) FROM follows", 0)

	// 0034 seeds nothing — every House is founded by a lifter — so what is worth
	// pinning here is the rails, which hold for rows no handler wrote. Each is
	// asserted by trying to break it.
	assertCount(t, sqlDB, "SELECT count(*) FROM houses", 0)
	assertHouseRejected(t, sqlDB, "sigil too short", "X", "", "")
	assertHouseRejected(t, sqlDB, "sigil too long", "TOOLONG", "", "")
	assertHouseRejected(t, sqlDB, "sigil not alphanumeric", "A-B", "", "")
	assertHouseRejected(t, sqlDB, "tagline too long", "OK", strings.Repeat("x", 81), "")
	assertHouseRejected(t, sqlDB, "description too long", "OK", "", strings.Repeat("x", 2001))

	// Both uniqueness rules are case-insensitive, which is the whole reason they
	// are expression indexes rather than column constraints. Asserted with a real
	// pair of inserts, then cleaned up so the count above stays true for any test
	// that runs after this one.
	if _, err := sqlDB.Exec(
		`INSERT INTO houses (name, sigil) VALUES ('Case Test', 'CASE')`,
	); err != nil {
		t.Fatalf("inserting the first House: %v", err)
	}
	if _, err := sqlDB.Exec(
		`INSERT INTO houses (name, sigil) VALUES ('case test', 'OTHR')`,
	); err == nil {
		t.Error("a House name differing only in case was accepted")
	}
	if _, err := sqlDB.Exec(
		`INSERT INTO houses (name, sigil) VALUES ('Other Name', 'case')`,
	); err == nil {
		t.Error("a House sigil differing only in case was accepted")
	}
	if _, err := sqlDB.Exec(`DELETE FROM houses WHERE lower(sigil) = 'case'`); err != nil {
		t.Fatalf("cleaning up the case-test House: %v", err)
	}
}

// assertHouseRejected proves a houses CHECK refuses a value, by writing one.
//
// The insert fails atomically, so a rejected write leaves the table as it was and
// an accepted one is both the failure being reported and a row that would break
// the count above — which is why the message names the constraint rather than the
// value.
func assertHouseRejected(t *testing.T, db *sql.DB, what, sigil, tagline, description string) {
	t.Helper()
	_, err := db.Exec(
		`INSERT INTO houses (name, sigil, tagline, description) VALUES ($1, $2, $3, $4)`,
		"Rejected "+what, sigil, tagline, description,
	)
	if err == nil {
		t.Errorf("%s was accepted; the matching houses CHECK is missing", what)
		_, _ = db.Exec(`DELETE FROM houses WHERE name = $1`, "Rejected "+what)
	}
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

// assertRestRejected proves the CHECK refuses a rest length, by writing one. The
// statement fails atomically, so a rejected write leaves the row as it was and
// an accepted one is the failure being reported.
func assertRestRejected(t *testing.T, db *sql.DB, seconds int) {
	t.Helper()
	_, err := db.Exec("UPDATE exercises SET rest_seconds = $1 WHERE name = 'Deadlift'", seconds)
	if err == nil {
		t.Errorf("rest_seconds = %d was accepted; the 30-180 CHECK is missing", seconds)
	}
}

func assertAccessory(t *testing.T, db *sql.DB, name string, want bool) {
	t.Helper()
	var got bool
	err := db.QueryRow(
		"SELECT is_accessory FROM exercises WHERE name = $1 AND created_by_user_id IS NULL",
		name).Scan(&got)
	if err != nil {
		t.Fatalf("is_accessory for %q: %v", name, err)
	}
	if got != want {
		t.Errorf("is_accessory for %q = %t, want %t", name, got, want)
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
