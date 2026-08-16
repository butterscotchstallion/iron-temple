package api_test

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
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

// testPool is the same pool the server runs on, kept package-level so tests can
// reach past the API for setup the API deliberately does not expose — namely
// backdating a session's created_at to exercise the 12-hour cutoff, and minting
// a second account once registration has closed itself.
var testPool *pgxpool.Pool

// primaryToken is the session cookie for the account registered in TestMain.
// Every endpoint except /health and /auth/* now requires one, so expect()
// attaches it by default and the tests below read as they did before.
var primaryToken string

const (
	primaryUsername = "primary"
	primaryPassword = "integration-test-pw"
)

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

	testPool = pool
	srv := httptest.NewServer(api.NewServer(pool, "", "").Router(""))
	baseURL = srv.URL + "/api/v1"

	// Claim the install. Registration closes behind this call, which is itself
	// asserted in TestRegistrationClosesAfterTheFirstAccount.
	primaryToken = registerPrimary()

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

// registerPrimary creates the first account and returns its session token.
// Runs before the suite, so it uses net/http directly rather than httpexpect,
// which needs a *testing.T.
func registerPrimary() string {
	body := fmt.Sprintf(`{"username":%q,"password":%q,"displayName":"Primary Lifter"}`,
		primaryUsername, primaryPassword)
	resp, err := http.Post(baseURL+"/auth/register", "application/json", strings.NewReader(body))
	if err != nil {
		log.Fatalf("register primary user: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusCreated {
		log.Fatalf("register primary user: status %d", resp.StatusCode)
	}
	for _, c := range resp.Cookies() {
		if c.Name == sessionCookie {
			return c.Value
		}
	}
	log.Fatalf("register primary user: no %s cookie in the response", sessionCookie)
	return ""
}

// sessionCookie mirrors auth.CookieName. Named here as a literal on purpose:
// the cookie name is part of the wire contract with the browser, so a test that
// merely echoes the constant would not notice it changing.
const sessionCookie = "it_session"

// expect returns a client signed in as the primary user — the default, since
// nearly every endpoint requires a session.
func expect(t *testing.T) *httpexpect.Expect {
	t.Helper()
	return expectAs(t, primaryToken)
}

// expectAnon returns a client with no session, for testing the public
// endpoints and the 401s.
func expectAnon(t *testing.T) *httpexpect.Expect {
	t.Helper()
	if testing.Short() {
		t.Skip("integration test requires a Docker daemon")
	}
	return httpexpect.Default(t, baseURL)
}

// expectAs returns a client carrying the given session token.
func expectAs(t *testing.T, token string) *httpexpect.Expect {
	t.Helper()
	if testing.Short() {
		t.Skip("integration test requires a Docker daemon")
	}
	return httpexpect.WithConfig(httpexpect.Config{
		BaseURL:  baseURL,
		Reporter: httpexpect.NewAssertReporter(t),
		Client:   http.DefaultClient,
	}).Builder(func(req *httpexpect.Request) {
		req.WithCookie(sessionCookie, token)
	})
}

func TestHealth(t *testing.T) {
	// Explicitly anonymous: /health is a Kubernetes probe target, and a probe
	// has no cookie to present.
	e := expectAnon(t)
	e.GET("/health").Expect().
		Status(http.StatusOK).
		JSON().Object().HasValue("status", "ok")
}

func TestSeedData(t *testing.T) {
	e := expect(t)
	e.GET("/exercises").Expect().
		Status(http.StatusOK).
		JSON().Array().Length().IsEqual(10)
	e.GET("/programs").Expect().
		Status(http.StatusOK).
		JSON().Array().Length().IsEqual(5)
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

// startSession creates a session for the seeded day and removes it when the
// test ends, so progression-sensitive tests don't leak history into each other.
func startSession(t *testing.T, e *httpexpect.Expect, dayID int) *httpexpect.Object {
	t.Helper()
	created := e.POST("/sessions").
		WithJSON(map[string]any{"programDayId": dayID}).
		Expect().Status(http.StatusCreated).
		JSON().Object()
	id := int(created.Value("id").Number().Raw())
	t.Cleanup(func() {
		e.DELETE(fmt.Sprintf("/sessions/%d", id)).Expect().Status(http.StatusNoContent)
	})
	return created
}

// backdateSession moves a session's created_at into the past, which is the only
// way to age it past the 12-hour cutoff within a test run.
func backdateSession(t *testing.T, sessionID int, d time.Duration) {
	t.Helper()
	_, err := testPool.Exec(context.Background(),
		"UPDATE sessions SET created_at = now() - $1::interval WHERE id = $2",
		fmt.Sprintf("%d seconds", int(d.Seconds())), sessionID)
	if err != nil {
		t.Fatalf("backdate session %d: %v", sessionID, err)
	}
}

// firstExercisePreview returns the progression block and weight the engine
// currently prescribes for a day's first exercise.
func firstExercisePreview(e *httpexpect.Expect, programID, dayID int) (status string, weight float64) {
	ex := e.GET(fmt.Sprintf("/programs/%d/days/%d/next-session", programID, dayID)).
		Expect().Status(http.StatusOK).
		JSON().Object().Value("exercises").Array().Value(0).Object()
	return ex.Value("progression").Object().Value("status").String().Raw(),
		ex.Value("weightLb").Number().Raw()
}

func TestFinishSessionIsExplicitAndIdempotent(t *testing.T) {
	e := expect(t)
	_, dayID := firstProgramAndDay(e)

	created := startSession(t, e, dayID)
	sessionID := int(created.Value("id").Number().Raw())

	// A brand-new session is neither finished nor over.
	created.Value("finishedAt").IsNull()
	created.HasValue("isOver", false)

	// Finishing stamps finishedAt and flips isOver, even with nothing logged.
	finished := e.POST(fmt.Sprintf("/sessions/%d/finish", sessionID)).
		Expect().Status(http.StatusOK).
		JSON().Object()
	finished.HasValue("isOver", true)
	stamp := finished.Value("finishedAt").String().NotEmpty().Raw()

	// Finishing again is a no-op rather than a fresh timestamp.
	e.POST(fmt.Sprintf("/sessions/%d/finish", sessionID)).
		Expect().Status(http.StatusOK).
		JSON().Object().HasValue("finishedAt", stamp)

	// The state survives a re-fetch.
	fetched := e.GET(fmt.Sprintf("/sessions/%d", sessionID)).
		Expect().Status(http.StatusOK).JSON().Object()
	fetched.HasValue("isOver", true)
	fetched.HasValue("finishedAt", stamp)
}

func TestFinishUnknownSession(t *testing.T) {
	e := expect(t)
	e.POST("/sessions/999999/finish").Expect().Status(http.StatusNotFound)
}

func TestSessionBecomesOverTwelveHoursAfterItStarted(t *testing.T) {
	e := expect(t)
	_, dayID := firstProgramAndDay(e)

	sessionID := int(startSession(t, e, dayID).Value("id").Number().Raw())

	// Just under the cutoff it is still in progress.
	backdateSession(t, sessionID, 11*time.Hour)
	e.GET(fmt.Sprintf("/sessions/%d", sessionID)).Expect().Status(http.StatusOK).
		JSON().Object().HasValue("isOver", false)

	// Past it, the session is over on its own — with no finishedAt, because
	// nobody pressed Finish.
	backdateSession(t, sessionID, 13*time.Hour)
	aged := e.GET(fmt.Sprintf("/sessions/%d", sessionID)).
		Expect().Status(http.StatusOK).JSON().Object()
	aged.HasValue("isOver", true)
	aged.Value("finishedAt").IsNull()
}

// Volume is weight actually moved, so it counts logged reps rather than
// completed sets — a set that stopped short of its target still lifted what it
// lifted. This test pins that reading: it logs one set clean and one short, and
// expects both in the total even though only one is completed.
func TestSessionVolumeCountsLoggedRepsNotCompletedSets(t *testing.T) {
	e := expect(t)
	_, dayID := firstProgramAndDay(e)

	// Measured as a delta, not an absolute: the whole suite shares one account,
	// so the lifetime total carries whatever earlier tests left behind.
	volumeBefore := historyVolume(e)

	created := startSession(t, e, dayID)
	sessionID := int(created.Value("id").Number().Raw())
	sets := created.Value("sets").Array()

	clean := sets.Value(0).Object()
	short := sets.Value(1).Object()
	cleanReps := int(clean.Value("targetReps").Number().Raw())
	shortReps := cleanReps - 2
	if shortReps < 1 {
		shortReps = 1
	}

	logSet(e, sessionID, int(clean.Value("id").Number().Raw()), cleanReps, true)
	logSet(e, sessionID, int(short.Value("id").Number().Raw()), shortReps, false)

	want := float64(cleanReps)*clean.Value("weightLb").Number().Raw() +
		float64(shortReps)*short.Value("weightLb").Number().Raw()
	// Guard against a vacuous pass: if the seed ever prescribed a zero weight,
	// every assertion below would hold at 0 while proving nothing.
	if want <= 0 {
		t.Fatalf("expected the seeded day to prescribe real weight, got want=%v", want)
	}

	summary := historySummary(t, e, sessionID)
	summary.Value("volumeLb").Number().InDelta(want, 0.001)
	// The two figures disagreeing is the point: one set met its prescription,
	// both moved weight.
	summary.HasValue("completedSetCount", 1)

	if got := historyVolume(e) - volumeBefore; math.Abs(got-want) > 0.001 {
		t.Fatalf("lifetime totalVolumeLb moved by %v, want %v", got, want)
	}
}

// logSet records reps against one materialized set.
func logSet(e *httpexpect.Expect, sessionID, setID, reps int, completed bool) {
	e.PATCH(fmt.Sprintf("/sessions/%d/sets/%d", sessionID, setID)).
		WithJSON(map[string]any{"actualReps": reps, "completed": completed}).
		Expect().Status(http.StatusOK)
}

// historyVolume reads the lifetime volume off the session list. It is deliberately
// taken from a page of the history rather than computed from the items, because
// spanning every session and not just the page is the field's whole contract.
func historyVolume(e *httpexpect.Expect) float64 {
	return e.GET("/sessions").Expect().Status(http.StatusOK).
		JSON().Object().Value("totalVolumeLb").Number().Raw()
}

// historySummary returns one session's summary from the history list.
func historySummary(t *testing.T, e *httpexpect.Expect, sessionID int) *httpexpect.Object {
	t.Helper()
	items := e.GET("/sessions").Expect().Status(http.StatusOK).
		JSON().Object().Value("items").Array()
	for i := 0; i < int(items.Length().Raw()); i++ {
		item := items.Value(i).Object()
		if int(item.Value("id").Number().Raw()) == sessionID {
			return item
		}
	}
	t.Fatalf("session %d is missing from the history list", sessionID)
	return nil
}

// Regression: sets are materialized up front with completed = false, so a
// session that was merely started used to score as BOOL_AND(completed) = false
// and be counted by the progression engine as a failed session. Three of those
// forced an unearned deload.
func TestProgressionIgnoresSessionsWithNoLoggedWork(t *testing.T) {
	e := expect(t)
	programID, dayID := firstProgramAndDay(e)

	statusBefore, weightBefore := firstExercisePreview(e, programID, dayID)

	// An in-progress session with nothing logged must not move the engine.
	sessionID := int(startSession(t, e, dayID).Value("id").Number().Raw())
	statusAfter, weightAfter := firstExercisePreview(e, programID, dayID)
	if statusAfter != statusBefore || weightAfter != weightBefore {
		t.Fatalf("an unlogged in-progress session changed the prescription: %s/%v → %s/%v",
			statusBefore, weightBefore, statusAfter, weightAfter)
	}

	// Nor once it ages out — being over is not enough without logged reps.
	backdateSession(t, sessionID, 13*time.Hour)
	statusAged, weightAged := firstExercisePreview(e, programID, dayID)
	if statusAged != statusBefore || weightAged != weightBefore {
		t.Fatalf("an unlogged aged-out session changed the prescription: %s/%v → %s/%v",
			statusBefore, weightBefore, statusAged, weightAged)
	}
}

// The counterpart to the regression above: a finished session that carries real
// work must still drive progression, or the new guards would be too broad.
func TestProgressionCountsFinishedSessionsWithLoggedWork(t *testing.T) {
	e := expect(t)
	programID, dayID := firstProgramAndDay(e)

	_, weightBefore := firstExercisePreview(e, programID, dayID)

	created := startSession(t, e, dayID)
	sessionID := int(created.Value("id").Number().Raw())

	// Log every set of the first exercise at its target reps — a clean session
	// for that lift.
	sets := created.Value("sets").Array()
	firstExerciseID := sets.Value(0).Object().Value("exerciseId").Number().Raw()
	for i := 0; i < int(sets.Length().Raw()); i++ {
		set := sets.Value(i).Object()
		if set.Value("exerciseId").Number().Raw() != firstExerciseID {
			continue
		}
		setID := int(set.Value("id").Number().Raw())
		e.PATCH(fmt.Sprintf("/sessions/%d/sets/%d", sessionID, setID)).
			WithJSON(map[string]any{
				"actualReps": int(set.Value("targetReps").Number().Raw()),
				"completed":  true,
			}).
			Expect().Status(http.StatusOK)
	}

	// Still in progress, so the engine hasn't counted it yet.
	_, weightMidSession := firstExercisePreview(e, programID, dayID)
	if weightMidSession != weightBefore {
		t.Fatalf("an in-progress session moved the prescription: %v → %v",
			weightBefore, weightMidSession)
	}

	// Finishing publishes it: a successful session advances the weight.
	e.POST(fmt.Sprintf("/sessions/%d/finish", sessionID)).Expect().Status(http.StatusOK)
	statusAfter, weightAfter := firstExercisePreview(e, programID, dayID)
	if weightAfter <= weightBefore {
		t.Fatalf("a finished successful session did not advance the weight: %v → %v",
			weightBefore, weightAfter)
	}
	if statusAfter != "advance" {
		t.Fatalf("expected status advance after a clean session, got %q", statusAfter)
	}
}

// ---- Racked ----

// A period the lifter did not train in is a valid recap, not a 404 and not a
// pile of nulls the UI has to guess at. 1970 is chosen because no other test
// can reach back and put a session in it.
func TestRackedEmptyPeriodIsAValidReport(t *testing.T) {
	e := expect(t)
	rep := e.GET("/racked").WithQuery("on", "1970-01-15").
		Expect().Status(http.StatusOK).JSON().Object()

	rep.Value("period").Object().HasValue("kind", "month").HasValue("label", "January 1970")
	totals := rep.Value("totals").Object()
	totals.HasValue("volumeLb", 0).HasValue("sessions", 0).HasValue("sets", 0).HasValue("reps", 0)

	// The nullable fields are the contract worth pinning: the UI branches on
	// each one rather than rendering a zero as if it were a measurement.
	rep.Value("change").IsNull()
	rep.Value("mostImproved").IsNull()
	rep.Value("heaviestSet").IsNull()
	rep.Value("fastestSession").IsNull()

	rep.HasValue("bestWeekday", -1)
	rep.HasValue("hourLabel", "")
	rep.Value("weekdays").Array().Length().IsEqual(7)
	rep.Value("hours").Array().Length().IsEqual(24)
	rep.Value("archetype").Object().HasValue("name", "")
	rep.Value("comparison").Object().HasValue("count", 0)
}

func TestRackedYearPeriod(t *testing.T) {
	e := expect(t)
	e.GET("/racked").WithQuery("period", "year").WithQuery("on", "1970-06-15").
		Expect().Status(http.StatusOK).JSON().Object().
		Value("period").Object().
		HasValue("kind", "year").
		HasValue("label", "1970").
		HasValue("start", "1970-01-01").
		HasValue("end", "1970-12-31")
}

func TestRackedRejectsBadParameters(t *testing.T) {
	e := expect(t)
	e.GET("/racked").WithQuery("period", "week").Expect().Status(http.StatusBadRequest)
	e.GET("/racked").WithQuery("on", "last-tuesday").Expect().Status(http.StatusBadRequest)
}

func TestRackedRequiresASession(t *testing.T) {
	expectAnon(t).GET("/racked").Expect().Status(http.StatusUnauthorized)
}

// The recap must count the same pounds the history does. Measured as a delta,
// because the suite shares one account and this month already holds whatever
// the other tests logged.
func TestRackedVolumeMatchesLoggedWork(t *testing.T) {
	e := expect(t)
	_, dayID := firstProgramAndDay(e)
	before := rackedTotals(e)

	created := startSession(t, e, dayID)
	sessionID := int(created.Value("id").Number().Raw())
	sets := created.Value("sets").Array()

	first := sets.Value(0).Object()
	second := sets.Value(1).Object()
	fullReps := int(first.Value("targetReps").Number().Raw())
	shortReps := fullReps - 2
	if shortReps < 1 {
		shortReps = 1
	}
	logSet(e, sessionID, int(first.Value("id").Number().Raw()), fullReps, true)
	logSet(e, sessionID, int(second.Value("id").Number().Raw()), shortReps, false)

	want := float64(fullReps)*first.Value("weightLb").Number().Raw() +
		float64(shortReps)*second.Value("weightLb").Number().Raw()
	if want <= 0 {
		t.Fatalf("expected the seeded day to prescribe real weight, got want=%v", want)
	}

	after := rackedTotals(e)
	if got := after.volume - before.volume; math.Abs(got-want) > 0.001 {
		t.Fatalf("racked volume moved by %v, want %v", got, want)
	}
	if got := after.sets - before.sets; got != 2 {
		t.Fatalf("racked set count moved by %d, want 2 — only logged sets count", got)
	}
	if got := after.reps - before.reps; got != fullReps+shortReps {
		t.Fatalf("racked reps moved by %d, want %d", got, fullReps+shortReps)
	}
}

// A set heavier than anything on record is a record, and a set at the same
// weight later in the period is not a second one.
func TestRackedDetectsPersonalRecords(t *testing.T) {
	e := expect(t)
	_, dayID := firstProgramAndDay(e)

	created := startSession(t, e, dayID)
	sessionID := int(created.Value("id").Number().Raw())
	set := created.Value("sets").Array().Value(0).Object()
	exerciseID := int(set.Value("exerciseId").Number().Raw())

	// Well past anything the seeded programs prescribe, so this cannot depend
	// on what earlier tests left in the history.
	const prWeight = 987.5
	logSetAt(e, sessionID, int(set.Value("id").Number().Raw()), 3, prWeight, true)

	prs := e.GET("/racked").Expect().Status(http.StatusOK).
		JSON().Object().Value("prs").Array()

	var found int
	for i := 0; i < int(prs.Length().Raw()); i++ {
		pr := prs.Value(i).Object()
		if int(pr.Value("exerciseId").Number().Raw()) != exerciseID {
			continue
		}
		if pr.Value("weightLb").Number().Raw() == prWeight {
			pr.HasValue("kind", "weight")
			found++
		}
	}
	if found != 1 {
		t.Fatalf("got %d records at %v lb, want exactly 1", found, prWeight)
	}
}

// Fastest session reads created_at against finished_at, so it only has an
// answer once the lifter has actually finished something.
func TestRackedFastestSessionNeedsAFinish(t *testing.T) {
	e := expect(t)
	_, dayID := firstProgramAndDay(e)

	created := startSession(t, e, dayID)
	sessionID := int(created.Value("id").Number().Raw())
	set := created.Value("sets").Array().Value(0).Object()
	logSet(e, sessionID, int(set.Value("id").Number().Raw()), 1, true)

	// Backdate the start so the finished session has a duration worth reading
	// rather than the few milliseconds this test takes.
	backdateSession(t, sessionID, 40*time.Minute)
	e.POST(fmt.Sprintf("/sessions/%d/finish", sessionID)).Expect().Status(http.StatusOK)

	fastest := e.GET("/racked").Expect().Status(http.StatusOK).
		JSON().Object().Value("fastestSession").Object()
	fastest.Value("durationSeconds").Number().Gt(0)
	fastest.Value("sessionId").Number().Gt(0)
}

// rackedFigures are the counters the recap tests compare as deltas.
type rackedFigures struct {
	volume float64
	sets   int
	reps   int
}

func rackedTotals(e *httpexpect.Expect) rackedFigures {
	totals := e.GET("/racked").Expect().Status(http.StatusOK).
		JSON().Object().Value("totals").Object()
	return rackedFigures{
		volume: totals.Value("volumeLb").Number().Raw(),
		sets:   int(totals.Value("sets").Number().Raw()),
		reps:   int(totals.Value("reps").Number().Raw()),
	}
}

// logSetAt logs a set at an explicit weight, which the prescription-driven
// helpers cannot express.
func logSetAt(e *httpexpect.Expect, sessionID, setID, reps int, weightLb float64, completed bool) {
	e.PATCH(fmt.Sprintf("/sessions/%d/sets/%d", sessionID, setID)).
		WithJSON(map[string]any{"actualReps": reps, "weightLb": weightLb, "completed": completed}).
		Expect().Status(http.StatusOK)
}
