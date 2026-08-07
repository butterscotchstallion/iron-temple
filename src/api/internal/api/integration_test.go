package api_test

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gavv/httpexpect/v2"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/lib/pq"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	appdb "gitea.homelab/gitadmin/iron-temple/api/db"
	"gitea.homelab/gitadmin/iron-temple/api/internal/api"
)

// baseURL is the /api/v1 root of the test server, set up in TestMain. These
// tests need a Docker-compatible daemon (Testcontainers); they self-skip under
// `go test -short`, which is what the pre-commit hook runs.
var baseURL string

func TestMain(m *testing.M) {
	flag.Parse()
	if testing.Short() {
		os.Exit(m.Run()) // individual tests skip; don't boot a container
	}

	ctx := context.Background()
	dsn, stopDB := testDB(ctx)

	// Apply schema + seed via the shared migrator.
	sqlDB, err := sql.Open("postgres", dsn)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}
	if err := appdb.Migrate(sqlDB); err != nil {
		log.Fatalf("migrate: %v", err)
	}
	_ = sqlDB.Close()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		log.Fatalf("pgxpool: %v", err)
	}

	srv := httptest.NewServer(api.NewServer(pool).Router(""))
	baseURL = srv.URL + "/api/v1"

	code := m.Run()

	srv.Close()
	pool.Close()
	stopDB()
	os.Exit(code)
}

// testDB returns a Postgres DSN for the suite plus a teardown func. In CI the
// host-executor runner has no container runtime for Testcontainers, so when
// TEST_DATABASE_URL is set (an ephemeral Postgres pod provisioned by the CI job) it
// is used directly. Locally, with the var unset, a throwaway Testcontainers Postgres
// is booted as before. stop terminates that container (a no-op for the external DB).
func testDB(ctx context.Context) (dsn string, stop func()) {
	if url := os.Getenv("TEST_DATABASE_URL"); url != "" {
		return url, func() {}
	}
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
		log.Fatalf("start postgres container: %v", err)
	}
	dsn, err = pg.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		log.Fatalf("connection string: %v", err)
	}
	return dsn, func() { _ = pg.Terminate(ctx) }
}

func expect(t *testing.T) *httpexpect.Expect {
	t.Helper()
	if testing.Short() {
		t.Skip("integration test requires a Docker daemon")
	}
	return httpexpect.Default(t, baseURL)
}

func TestHealth(t *testing.T) {
	e := expect(t)
	e.GET("/health").Expect().
		Status(http.StatusOK).
		JSON().Object().HasValue("status", "ok")
}

func TestSeedData(t *testing.T) {
	e := expect(t)
	e.GET("/exercises").Expect().
		Status(http.StatusOK).
		JSON().Array().Length().IsEqual(5)
	e.GET("/programs").Expect().
		Status(http.StatusOK).
		JSON().Array().Length().IsEqual(3)
}

func TestGetProgramAndUnknown(t *testing.T) {
	e := expect(t)

	programID := int(firstProgramID(e))
	obj := e.GET(fmt.Sprintf("/programs/%d", programID)).Expect().
		Status(http.StatusOK).
		JSON().Object()
	obj.Value("id").Number().IsEqual(programID)
	obj.Value("days").Array().NotEmpty()

	e.GET("/programs/999999").Expect().Status(http.StatusNotFound)
}

func TestPreviewNextSessionUsesStartingWeights(t *testing.T) {
	e := expect(t)
	programID, dayID := firstProgramAndDay(e)

	obj := e.GET(fmt.Sprintf("/programs/%d/days/%d/next-session", programID, dayID)).Expect().
		Status(http.StatusOK).
		JSON().Object()
	obj.Value("programDayId").Number().IsEqual(dayID)
	// No history yet, so weights equal the prescribed starting weights (>0).
	obj.Value("exercises").Array().NotEmpty()
	obj.Value("exercises").Array().Value(0).Object().
		Value("weightLb").Number().Gt(0)
}

func TestSessionLifecycle(t *testing.T) {
	e := expect(t)
	_, dayID := firstProgramAndDay(e)

	// Create.
	created := e.POST("/sessions").
		WithJSON(map[string]any{"programDayId": dayID}).
		Expect().Status(http.StatusCreated).
		JSON().Object()
	created.Value("programDayId").Number().IsEqual(dayID)
	created.Value("sets").Array().NotEmpty()
	sessionID := int(created.Value("id").Number().Raw())
	firstSetID := int(created.Value("sets").Array().Value(0).Object().Value("id").Number().Raw())

	// Fetch.
	e.GET(fmt.Sprintf("/sessions/%d", sessionID)).Expect().
		Status(http.StatusOK).
		JSON().Object().Value("id").Number().IsEqual(sessionID)

	// Log the first set.
	e.PATCH(fmt.Sprintf("/sessions/%d/sets/%d", sessionID, firstSetID)).
		WithJSON(map[string]any{"actualReps": 5, "completed": true}).
		Expect().Status(http.StatusOK).
		JSON().Object().HasValue("completed", true)

	// Patch session metadata.
	e.PATCH(fmt.Sprintf("/sessions/%d", sessionID)).
		WithJSON(map[string]any{"notes": "felt strong"}).
		Expect().Status(http.StatusOK).
		JSON().Object().HasValue("notes", "felt strong")

	// It appears in history.
	e.GET("/sessions").Expect().Status(http.StatusOK).
		JSON().Object().Value("total").Number().Gt(0)

	// Pagination params are validated before the query runs. The offset-overflow
	// case is a regression guard: an offset past int32 used to wrap negative
	// instead of being rejected.
	e.GET("/sessions").WithQuery("offset", "3000000000").
		Expect().Status(http.StatusBadRequest)
	e.GET("/sessions").WithQuery("offset", "-1").
		Expect().Status(http.StatusBadRequest)
	e.GET("/sessions").WithQuery("limit", "101").
		Expect().Status(http.StatusBadRequest)
	e.GET("/sessions").WithQuery("limit", "0").
		Expect().Status(http.StatusBadRequest)

	// Delete, then it's gone.
	e.DELETE(fmt.Sprintf("/sessions/%d", sessionID)).Expect().Status(http.StatusNoContent)
	e.GET(fmt.Sprintf("/sessions/%d", sessionID)).Expect().Status(http.StatusNotFound)
}

func TestCreateSessionUnknownDay(t *testing.T) {
	e := expect(t)
	e.POST("/sessions").
		WithJSON(map[string]any{"programDayId": 999999}).
		Expect().Status(http.StatusNotFound)
}

// firstProgramID returns the id of the first seeded program.
func firstProgramID(e *httpexpect.Expect) int {
	return int(e.GET("/programs").Expect().Status(http.StatusOK).
		JSON().Array().Value(0).Object().Value("id").Number().Raw())
}

// firstProgramAndDay returns a program id and one of its day ids.
func firstProgramAndDay(e *httpexpect.Expect) (int, int) {
	programID := firstProgramID(e)
	dayID := int(e.GET(fmt.Sprintf("/programs/%d", programID)).Expect().Status(http.StatusOK).
		JSON().Object().Value("days").Array().Value(0).Object().Value("id").Number().Raw())
	return programID, dayID
}
