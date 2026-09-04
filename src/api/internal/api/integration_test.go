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
	_ "github.com/jackc/pgx/v5/stdlib"
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
	sqlDB, err := sql.Open("pgx", dsn)
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
	// The ten lifts the programs prescribe, plus the accessory catalogue 0009
	// seeded for the exercise library.
	e.GET("/exercises").Expect().
		Status(http.StatusOK).
		JSON().Array().Length().IsEqual(53)
	e.GET("/programs").Expect().
		Status(http.StatusOK).
		JSON().Array().Length().IsEqual(6)
}

// TestListExercisesCarriesTopSet pins the top set the list row now carries
// against the history endpoint the Progress page used to compute it from.
//
// That equivalence is the entire basis for dropping the per-card history
// request, so it is asserted rather than assumed: the test takes the maximum
// the browser used to take, and requires the server's column to agree. A
// divergence here — a different definition of "a set", a different tie-break —
// would otherwise surface as silently wrong weights on the Progress page.
func TestListExercisesCarriesTopSet(t *testing.T) {
	e := expect(t)
	_, dayID := firstProgramAndDay(e)

	created := startSession(t, e, dayID)
	sessionID := int(created.Value("id").Number().Raw())
	firstSet := created.Value("sets").Array().Value(0).Object()
	exerciseID := int(firstSet.Value("exerciseId").Number().Raw())
	setID := int(firstSet.Value("id").Number().Raw())

	// Before anything is logged the lift has no history, so it reports null —
	// not a set at zero pounds, which is a real weight. Scoped to this test's
	// own exercise rather than sweeping the library: every test in the package
	// shares one database, so "nothing anywhere has a top set" would be an
	// assertion about the suite's ordering rather than about this endpoint.
	exerciseTopSet(t, e, exerciseID).IsNull()

	// actual_reps > 0 is what promotes a prescribed set to a performed one, in
	// this column exactly as in the history endpoint.
	e.PATCH(fmt.Sprintf("/sessions/%d/sets/%d", sessionID, setID)).
		WithJSON(map[string]any{"actualReps": 5, "completed": true}).
		Expect().Status(http.StatusOK)

	// The maximum the browser used to take: scan history oldest-first, keep the
	// strictly-greater weight, so ties fall to the session that set it first.
	var wantWeight float64
	var wantDate string
	for _, p := range e.GET(fmt.Sprintf("/exercises/%d/history", exerciseID)).
		Expect().Status(http.StatusOK).JSON().Object().Value("points").Array().Iter() {
		point := p.Object()
		if w := point.Value("weightLb").Number().Raw(); wantDate == "" || w > wantWeight {
			wantWeight = w
			wantDate = point.Value("performedOn").String().Raw()
		}
	}
	if wantDate == "" {
		t.Fatal("logged a set but the lift reports no history to compare against")
	}

	top := exerciseTopSet(t, e, exerciseID).Object()
	top.Value("weightLb").Number().IsEqual(wantWeight)
	top.Value("performedOn").String().IsEqual(wantDate)
}

// exerciseTopSet picks one exercise's topSet out of the library listing. A
// missing id is fatal rather than a nil return, so a caller asserting IsNull()
// on the result cannot be reading a lift that simply wasn't in the response.
func exerciseTopSet(t *testing.T, e *httpexpect.Expect, exerciseID int) *httpexpect.Value {
	t.Helper()
	for _, ex := range e.GET("/exercises").Expect().Status(http.StatusOK).
		JSON().Array().Iter() {
		obj := ex.Object()
		if int(obj.Value("id").Number().Raw()) == exerciseID {
			return obj.Value("topSet")
		}
	}
	t.Fatalf("exercise %d missing from the library listing", exerciseID)
	return nil
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

// TestPreviewNextSessionsMatchesPerDay pins the batched preview against the
// per-day one it replaced on the program screen. Both endpoints run the same
// progression engine, so a day prescribed either way must come out identical —
// if they can disagree, the screen shows different weights depending on which
// call happened to fill it.
func TestPreviewNextSessionsMatchesPerDay(t *testing.T) {
	e := expect(t)
	programID := firstProgramID(e)

	all := e.GET(fmt.Sprintf("/programs/%d/next-sessions", programID)).Expect().
		Status(http.StatusOK).
		JSON().Object()
	all.Value("programId").Number().IsEqual(programID)

	days := all.Value("days").Array()
	days.NotEmpty()
	// Every day of the program, not just the one the old screen asked for first.
	days.Length().IsEqual(
		e.GET(fmt.Sprintf("/programs/%d", programID)).Expect().Status(http.StatusOK).
			JSON().Object().Value("days").Array().Length().Raw())

	for _, d := range days.Iter() {
		day := d.Object()
		dayID := int(day.Value("programDayId").Number().Raw())

		single := e.GET(fmt.Sprintf("/programs/%d/days/%d/next-session", programID, dayID)).
			Expect().Status(http.StatusOK).JSON().Object()
		day.Value("programDayName").IsEqual(single.Value("programDayName").Raw())
		day.Value("exercises").IsEqual(single.Value("exercises").Raw())
		// The layoff is hoisted onto the wrapper, so the per-day copy the batch
		// response drops must be the one it reports once.
		all.Value("layoff").IsEqual(single.Value("layoff").Raw())
	}
}

func TestPreviewNextSessionsUnknownProgram(t *testing.T) {
	e := expect(t)
	// A program that does not exist is a 404, not a 200 with an empty day list —
	// which is what listing days without checking the program would produce.
	e.GET("/programs/999999/next-sessions").Expect().Status(http.StatusNotFound)
}

// The history endpoint names its own lift, which is what lets the progress
// chart stop fetching the entire library to label itself.
func TestExerciseHistoryNamesTheLift(t *testing.T) {
	e := expect(t)
	_, dayID := firstProgramAndDay(e)

	created := startSession(t, e, dayID)
	firstSet := created.Value("sets").Array().Value(0).Object()
	exerciseID := int(firstSet.Value("exerciseId").Number().Raw())
	wantName := firstSet.Value("exerciseName").String().Raw()

	history := e.GET(fmt.Sprintf("/exercises/%d/history", exerciseID)).
		Expect().Status(http.StatusOK).JSON().Object()
	history.Value("exerciseId").Number().IsEqual(exerciseID)
	// The same name the session calls it, rather than a second spelling of it.
	history.Value("exerciseName").String().IsEqual(wantName)
	// A lift that exists but has not been performed has an empty list, not a
	// missing one — the chart draws "no sessions yet" from that.
	history.Value("points").Array().IsEmpty()
}

// A lift nobody can see is a 404, not an empty history. Before the endpoint had
// a name to return it never looked the exercise up, so an unknown id was
// indistinguishable from a movement that had simply never been trained.
func TestExerciseHistoryUnknownLift(t *testing.T) {
	e := expect(t)
	e.GET("/exercises/999999/history").Expect().Status(http.StatusNotFound)
}

// TestSessionPreviousBestsExcludeThisSession pins the record-to-beat the active
// session screen reads instead of fetching a history per lift.
//
// The exclusion is the part worth asserting. The browser used to take the
// maximum over everything it could see, so a set already logged in THIS session
// counted towards the record it was about to be compared against — which made
// "is this a PR" depend on when the page was opened.
func TestSessionPreviousBestsExcludeThisSession(t *testing.T) {
	e := expect(t)
	_, dayID := firstProgramAndDay(e)

	// A first session, logged, to become the record.
	first := startSession(t, e, dayID)
	firstID := int(first.Value("id").Number().Raw())
	firstSet := first.Value("sets").Array().Value(0).Object()
	exerciseID := int(firstSet.Value("exerciseId").Number().Raw())
	recordWeight := firstSet.Value("weightLb").Number().Raw()
	e.PATCH(fmt.Sprintf("/sessions/%d/sets/%d", firstID, int(firstSet.Value("id").Number().Raw()))).
		WithJSON(map[string]any{"actualReps": 5, "completed": true}).
		Expect().Status(http.StatusOK)

	// Read back through the session that set it: its own logged set must not
	// count, so it still sees nothing to beat for that lift.
	if got, ok := previousBest(e, firstID, exerciseID); ok {
		t.Errorf("session %d counted its own set towards the record to beat: got %v", firstID, got)
	}

	// A later session does see it.
	second := startSession(t, e, dayID)
	secondID := int(second.Value("id").Number().Raw())
	got, ok := previousBest(e, secondID, exerciseID)
	if !ok {
		t.Fatalf("session %d sees no record for exercise %d after one was set", secondID, exerciseID)
	}
	if got != recordWeight {
		t.Errorf("record to beat = %v, want %v", got, recordWeight)
	}
}

// previousBest reads one lift's record-to-beat off a session, reporting whether
// the session listed it at all — absent means "nothing to beat", which is a
// different answer from a record of zero.
func previousBest(e *httpexpect.Expect, sessionID, exerciseID int) (float64, bool) {
	for _, b := range e.GET(fmt.Sprintf("/sessions/%d", sessionID)).
		Expect().Status(http.StatusOK).
		JSON().Object().Value("previousBests").Array().Iter() {
		best := b.Object()
		if int(best.Value("exerciseId").Number().Raw()) == exerciseID {
			return best.Value("weightLb").Number().Raw(), true
		}
	}
	return 0, false
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

// A weigh-in is recorded per session and carried forward to the next one as a
// suggestion, never copied into it. That distinction is the feature: a session
// nobody stood on a scale for stays null, so the series a weight chart reads is
// the days that were actually measured rather than a flat line of the last
// number anyone typed.
func TestSessionBodyweightCarriesForwardWithoutBeingCopied(t *testing.T) {
	e := expect(t)
	_, dayID := firstProgramAndDay(e)

	first := startSession(t, e, dayID)
	firstID := int(first.Value("id").Number().Raw())

	// Nothing weighed anywhere yet, so there is neither a record nor a hint.
	first.Value("bodyweightLb").IsNull()
	first.Value("lastWeighIn").IsNull()

	recorded := e.PATCH(fmt.Sprintf("/sessions/%d", firstID)).
		WithJSON(map[string]any{"bodyweightLb": 184.5}).
		Expect().Status(http.StatusOK).JSON().Object()
	recorded.HasValue("bodyweightLb", 184.5)
	// A session never carries itself: its own weigh-in is already bodyweightLb,
	// and offering it back as a hint would ask for what it already has.
	recorded.Value("lastWeighIn").IsNull()

	// Omitting the field leaves the weigh-in alone — absent is not null.
	e.PATCH(fmt.Sprintf("/sessions/%d", firstID)).
		WithJSON(map[string]any{"notes": "felt light"}).
		Expect().Status(http.StatusOK).JSON().Object().
		HasValue("bodyweightLb", 184.5)

	// The next session offers the number without recording it.
	second := startSession(t, e, dayID)
	secondID := int(second.Value("id").Number().Raw())
	second.Value("bodyweightLb").IsNull()
	carried := second.Value("lastWeighIn").Object()
	carried.HasValue("weightLb", 184.5)
	carried.Value("performedOn").String().NotEmpty()

	// Explicit null erases the entry, and the hint goes with it.
	e.PATCH(fmt.Sprintf("/sessions/%d", firstID)).
		WithJSON(map[string]any{"bodyweightLb": nil}).
		Expect().Status(http.StatusOK).JSON().Object().
		Value("bodyweightLb").IsNull()
	e.GET(fmt.Sprintf("/sessions/%d", secondID)).
		Expect().Status(http.StatusOK).JSON().Object().
		Value("lastWeighIn").IsNull()
}

// Zero is rejected along with the negatives: unlike an assistance weight, where
// 0 legitimately means bodyweight work, a lifter who weighs nothing has mistyped.
func TestSessionBodyweightRejectsNonsense(t *testing.T) {
	e := expect(t)
	_, dayID := firstProgramAndDay(e)
	sessionID := int(startSession(t, e, dayID).Value("id").Number().Raw())

	for _, bad := range []any{0, -1, 5000, "heavy"} {
		e.PATCH(fmt.Sprintf("/sessions/%d", sessionID)).
			WithJSON(map[string]any{"bodyweightLb": bad}).
			Expect().Status(http.StatusBadRequest)
	}

	// None of them landed.
	e.GET(fmt.Sprintf("/sessions/%d", sessionID)).
		Expect().Status(http.StatusOK).JSON().Object().
		Value("bodyweightLb").IsNull()
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

// A lift's history is the lift's, not the program's. This is the regression for
// ListLiftHistory having been scoped to one program: the squat is prescribed by
// both StrongLifts 5x5 and Advanced 3x5, and taking the second used to restart
// it at the seeded 45 lb no matter what had been logged under the first — which
// hit hardest on the one program switch the app actually recommends, since
// Advanced 3x5 is where a stalled 5x5 is supposed to go.
func TestProgressionFollowsTheLiftAcrossPrograms(t *testing.T) {
	e := expect(t)

	fromProgram, fromDay := programAndFirstDay(e, "StrongLifts 5x5")
	toProgram, toDay := programAndFirstDay(e, "Advanced 3x5")

	// Both days open on the squat, which is what makes them comparable.
	fromLift := firstExerciseName(e, fromProgram, fromDay)
	toLift := firstExerciseName(e, toProgram, toDay)
	if fromLift != toLift {
		t.Fatalf("expected both days to open on the same lift, got %q and %q", fromLift, toLift)
	}

	_, seeded := firstExercisePreview(e, toProgram, toDay)

	// A clean session under the first program.
	logCleanFirstExercise(t, e, fromDay)

	_, advanced := firstExercisePreview(e, fromProgram, fromDay)
	if advanced <= seeded {
		t.Fatalf("the session did not advance the lift under its own program: %v → %v",
			seeded, advanced)
	}

	// The other program must pick the bar up where that left it.
	status, carried := firstExercisePreview(e, toProgram, toDay)
	if carried != advanced {
		t.Fatalf("switching programs did not carry the lift: %v under %q but %v under %q",
			advanced, "StrongLifts 5x5", carried, "Advanced 3x5")
	}
	if status == "start" {
		t.Fatalf("the lift read as having no history under the second program (weight %v)", carried)
	}
}

// programAndFirstDay resolves a program by name and returns it with its first
// day. By name rather than by position because the point of the caller is to
// compare two named programs.
func programAndFirstDay(e *httpexpect.Expect, name string) (int, int) {
	programs := e.GET("/programs").Expect().Status(http.StatusOK).JSON().Array()
	for i := 0; i < int(programs.Length().Raw()); i++ {
		p := programs.Value(i).Object()
		if p.Value("name").String().Raw() != name {
			continue
		}
		id := int(p.Value("id").Number().Raw())
		dayID := int(e.GET(fmt.Sprintf("/programs/%d", id)).Expect().Status(http.StatusOK).
			JSON().Object().Value("days").Array().Value(0).Object().Value("id").Number().Raw())
		return id, dayID
	}
	panic("no seeded program named " + name)
}

func firstExerciseName(e *httpexpect.Expect, programID, dayID int) string {
	return e.GET(fmt.Sprintf("/programs/%d/days/%d/next-session", programID, dayID)).
		Expect().Status(http.StatusOK).
		JSON().Object().Value("exercises").Array().Value(0).Object().
		Value("exerciseName").String().Raw()
}

// logCleanFirstExercise runs a session on a day, hitting every prescribed rep of
// its first exercise, and finishes it — one successful session for that lift.
func logCleanFirstExercise(t *testing.T, e *httpexpect.Expect, dayID int) {
	t.Helper()
	created := startSession(t, e, dayID)
	sessionID := int(created.Value("id").Number().Raw())

	sets := created.Value("sets").Array()
	firstExerciseID := sets.Value(0).Object().Value("exerciseId").Number().Raw()
	for i := 0; i < int(sets.Length().Raw()); i++ {
		set := sets.Value(i).Object()
		if set.Value("exerciseId").Number().Raw() != firstExerciseID {
			continue
		}
		logSet(e, sessionID, int(set.Value("id").Number().Raw()),
			int(set.Value("targetReps").Number().Raw()), true)
	}
	e.POST(fmt.Sprintf("/sessions/%d/finish", sessionID)).Expect().Status(http.StatusOK)
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
	rep.Value("bodyweight").IsNull()

	// The split is always present — a period that moved nothing divides nothing
	// — and must not divide by zero into a NaN the page would render verbatim.
	split := rep.Value("split").Object()
	split.Value("main").Object().HasValue("volumeLb", 0).HasValue("share", 0).HasValue("lifts", 0)
	split.Value("assistance").Object().HasValue("volumeLb", 0).HasValue("share", 0)

	rep.HasValue("bestWeekday", -1)
	rep.HasValue("hourLabel", "")
	rep.Value("weekdays").Array().Length().IsEqual(7)
	rep.Value("hours").Array().Length().IsEqual(24)
	rep.Value("archetype").Object().HasValue("name", "")
	rep.Value("comparison").Object().HasValue("count", 0)
}

// The audit fixes, checked where they cross the wire rather than only in
// internal/racked: these are the fields the page reads, and a report that is
// right in Go and absent from the JSON is still wrong on screen.

// 1970 has ended, so its recap is measured over the whole of itself. The
// default view is the month in progress and must say so, because every rate in
// it is then measured over the days elapsed.
func TestRackedFlagsThePeriodInProgress(t *testing.T) {
	e := expect(t)

	past := e.GET("/racked").WithQuery("on", "1970-01-15").
		Expect().Status(http.StatusOK).JSON().Object()
	past.Value("period").Object().HasValue("inProgress", false)

	// The default view is today's month, and a month is no longer in progress on
	// its final day. Asserting `true` outright would fail on the 28th, 30th or
	// 31st — roughly twelve days a year, against a clock this test cannot set.
	// The report zone is UTC here, matching the server's default.
	now := time.Now().UTC()
	lastDay := time.Date(now.Year(), now.Month()+1, 0, 0, 0, 0, 0, time.UTC).Day()
	running := now.Day() != lastDay

	current := e.GET("/racked").Expect().Status(http.StatusOK).JSON().Object()
	current.Value("period").Object().HasValue("inProgress", running)
}

// peakHour is published so the page accents the bar hourLabel names instead of
// breaking a tie its own way.
func TestRackedPublishesThePeakHour(t *testing.T) {
	e := expect(t)

	empty := e.GET("/racked").WithQuery("on", "1970-01-15").
		Expect().Status(http.StatusOK).JSON().Object()
	empty.HasValue("peakHour", -1).HasValue("hourLabel", "")

	_, dayID := firstProgramAndDay(e)
	created := startSession(t, e, dayID)
	sessionID := int(created.Value("id").Number().Raw())
	set := created.Value("sets").Array().Value(0).Object()
	logSet(e, sessionID, int(set.Value("id").Number().Raw()), 1, true)

	rep := e.GET("/racked").Expect().Status(http.StatusOK).JSON().Object()
	peak := int(rep.Value("peakHour").Number().Raw())
	if peak < 0 || peak > 23 {
		t.Fatalf("peakHour = %d, want an hour of the day", peak)
	}
	// The label describes that hour and nothing else, so one implies the other.
	rep.Value("hourLabel").String().NotEmpty()
	rep.Value("hours").Array().Value(peak).Number().Gt(0)
}

// A single is worth what was on the bar. The estimate travels through the
// baseline query as well as through Go, so this pins the pair of them.
func TestRackedEstimatedMaxOfASingleIsTheWeight(t *testing.T) {
	e := expect(t)
	_, dayID := firstProgramAndDay(e)

	created := startSession(t, e, dayID)
	sessionID := int(created.Value("id").Number().Raw())
	set := created.Value("sets").Array().Value(0).Object()
	exerciseID := int(set.Value("exerciseId").Number().Raw())

	// Heavier than anything else the suite logs, so this set owns the series.
	const single = 1235.0
	logSetAt(e, sessionID, int(set.Value("id").Number().Raw()), 1, single, true)

	series := e.GET("/racked").Expect().Status(http.StatusOK).
		JSON().Object().Value("series").Array()

	var checked bool
	for i := 0; i < int(series.Length().Raw()); i++ {
		lift := series.Value(i).Object()
		if int(lift.Value("exerciseId").Number().Raw()) != exerciseID {
			continue
		}
		points := lift.Value("points").Array()
		for j := 0; j < int(points.Length().Raw()); j++ {
			p := points.Value(j).Object()
			if p.Value("topWeightLb").Number().Raw() != single {
				continue
			}
			// Epley would have made this 1275.6 — an estimate above the number
			// it is estimating.
			p.HasValue("e1rmLb", single)
			checked = true
		}
	}
	if !checked {
		t.Fatalf("no series point at %v lb to check", single)
	}
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

// A week runs Monday to Sunday and is not clipped to the month it starts in.
// 1970-06-17 is a Wednesday, so its week opens on the 15th; 1970-04-01 is also a
// Wednesday, and its week opens back in March.
func TestRackedWeekPeriod(t *testing.T) {
	e := expect(t)
	e.GET("/racked").WithQuery("period", "week").WithQuery("on", "1970-06-17").
		Expect().Status(http.StatusOK).JSON().Object().
		Value("period").Object().
		HasValue("kind", "week").
		HasValue("label", "June 15–21 1970").
		HasValue("start", "1970-06-15").
		HasValue("end", "1970-06-21")

	// Across a month boundary, where the label widens and the bounds must not
	// be trimmed back to the 1st.
	e.GET("/racked").WithQuery("period", "week").WithQuery("on", "1970-04-01").
		Expect().Status(http.StatusOK).JSON().Object().
		Value("period").Object().
		HasValue("label", "March 30 – April 5 1970").
		HasValue("start", "1970-03-30").
		HasValue("end", "1970-04-05")
}

func TestRackedRejectsBadParameters(t *testing.T) {
	e := expect(t)
	e.GET("/racked").WithQuery("period", "day").Expect().Status(http.StatusBadRequest)
	e.GET("/racked").WithQuery("period", "WEEK").Expect().Status(http.StatusBadRequest)
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

// Regression: the estimated-max baseline must round exactly as Go does.
//
// Set.E1RM rounds to the pound; the baseline query has to as well, or the two
// disagree inside a sub-pound band — and that band is where a record is decided.
// 185 lb x 3 is 203.5, which Go calls 204, so an unrounded baseline made
// repeating the identical set in a later period look like a new record.
func TestRackedEstimatedMaxBaselineRoundsLikeGo(t *testing.T) {
	e := expect(t)
	_, dayID := firstProgramAndDay(e)

	// The same work twice: once in a period that has closed, once in this one.
	// Nothing improved, so nothing here is a record.
	const weight, reps = 185.0, 3
	lastMonth := time.Now().UTC().AddDate(0, 0, -1-time.Now().UTC().Day())

	past := startSession(t, e, dayID)
	pastID := int(past.Value("id").Number().Raw())
	pastSet := past.Value("sets").Array().Value(0).Object()
	exerciseID := int(pastSet.Value("exerciseId").Number().Raw())
	logSetAt(e, pastID, int(pastSet.Value("id").Number().Raw()), reps, weight, true)
	backdatePerformedOn(t, pastID, lastMonth)

	now := startSession(t, e, dayID)
	nowID := int(now.Value("id").Number().Raw())
	nowSet := now.Value("sets").Array().Value(0).Object()
	logSetAt(e, nowID, int(nowSet.Value("id").Number().Raw()), reps, weight, true)

	prs := e.GET("/racked").Expect().Status(http.StatusOK).
		JSON().Object().Value("prs").Array()
	for i := 0; i < int(prs.Length().Raw()); i++ {
		pr := prs.Value(i).Object()
		if int(pr.Value("exerciseId").Number().Raw()) != exerciseID {
			continue
		}
		if pr.Value("weightLb").Number().Raw() == weight {
			t.Fatalf("repeating %v lb x %d was reported as a %s record",
				weight, reps, pr.Value("kind").String().Raw())
		}
	}
}

// The same regression for a single, which is the branch the baseline query grew
// a CASE for. Go stopped applying Epley at one rep; had the query kept applying
// it, the stored baseline would sit a thirtieth above every in-period estimate
// and repeating an identical single would never clear it — the record would
// simply stop being reported. Pulling the same weight twice is not a record and
// not a silence: it is one weight record the first time and nothing after.
func TestRackedSingleBaselineAgreesWithGo(t *testing.T) {
	e := expect(t)
	_, dayID := firstProgramAndDay(e)

	// Heavier than anything else the suite logs, so the history is this test's.
	const weight = 1111.0
	lastMonth := time.Now().UTC().AddDate(0, 0, -1-time.Now().UTC().Day())

	past := startSession(t, e, dayID)
	pastID := int(past.Value("id").Number().Raw())
	pastSet := past.Value("sets").Array().Value(0).Object()
	exerciseID := int(pastSet.Value("exerciseId").Number().Raw())
	logSetAt(e, pastID, int(pastSet.Value("id").Number().Raw()), 1, weight, true)
	backdatePerformedOn(t, pastID, lastMonth)

	now := startSession(t, e, dayID)
	nowID := int(now.Value("id").Number().Raw())
	nowSet := now.Value("sets").Array().Value(0).Object()
	logSetAt(e, nowID, int(nowSet.Value("id").Number().Raw()), 1, weight, true)

	prs := e.GET("/racked").Expect().Status(http.StatusOK).
		JSON().Object().Value("prs").Array()
	for i := 0; i < int(prs.Length().Raw()); i++ {
		pr := prs.Value(i).Object()
		if int(pr.Value("exerciseId").Number().Raw()) != exerciseID {
			continue
		}
		if pr.Value("weightLb").Number().Raw() == weight {
			t.Fatalf("repeating a %v lb single was reported as a %s record",
				weight, pr.Value("kind").String().Raw())
		}
	}
}

// backdatePerformedOn moves a session into an earlier period, which is the only
// way to give the baseline queries something to find.
func backdatePerformedOn(t *testing.T, sessionID int, on time.Time) {
	t.Helper()
	_, err := testPool.Exec(context.Background(),
		"UPDATE sessions SET performed_on = $1 WHERE id = $2", on, sessionID)
	if err != nil {
		t.Fatalf("backdate performed_on for %d: %v", sessionID, err)
	}
}

// Assistance is counted, and named.
//
// The headline is the claim worth pinning: the recap counted assistance in its
// tonnage from the day assistance shipped, and the split names it without
// moving it. A lifter who read last month's number must find the same number
// there, with a breakdown beside it rather than instead of it.
func TestRackedSplitsAssistanceOutWithoutRemovingIt(t *testing.T) {
	e := expect(t)
	programID, dayID := firstProgramAndDay(e)

	curlID := exerciseIDByName(t, e, "Barbell Curl")
	addAssistance(t, e, programID, dayID, curlID, 3, 10, 40)

	before := rackedTotals(e)
	beforeSplit := rackedSplit(e)

	created := startSession(t, e, dayID)
	sessionID := int(created.Value("id").Number().Raw())

	curlSets := sessionSetsFor(created, curlID)
	if len(curlSets) == 0 {
		t.Fatal("the session materialized no assistance sets")
	}
	const reps = 10
	var want float64
	for _, set := range curlSets {
		logSet(e, sessionID, int(set.Value("id").Number().Raw()), reps, true)
		want += reps * set.Value("weightLb").Number().Raw()
	}
	if want <= 0 {
		t.Fatalf("expected the assistance entry to carry weight, got %v", want)
	}

	after := rackedTotals(e)
	afterSplit := rackedSplit(e)

	// In the headline, where it has always been.
	if got := after.volume - before.volume; math.Abs(got-want) > 0.001 {
		t.Fatalf("headline volume moved by %v, want %v — assistance is training", got, want)
	}
	// And on the assistance side of the split, not the main one.
	if got := afterSplit.assistance - beforeSplit.assistance; math.Abs(got-want) > 0.001 {
		t.Fatalf("assistance volume moved by %v, want %v", got, want)
	}
	if got := afterSplit.main - beforeSplit.main; math.Abs(got) > 0.001 {
		t.Fatalf("main volume moved by %v on assistance work, want 0", got)
	}
	// The two halves are the whole.
	if got := afterSplit.main + afterSplit.assistance; math.Abs(got-after.volume) > 0.001 {
		t.Fatalf("split sums to %v, want the headline %v", got, after.volume)
	}

	// And the lift itself is labelled, so the page can tag its row.
	lifts := e.GET("/racked").Expect().Status(http.StatusOK).JSON().Object().Value("lifts").Array()
	var found bool
	for i := 0; i < int(lifts.Length().Raw()); i++ {
		lift := lifts.Value(i).Object()
		if int(lift.Value("exerciseId").Number().Raw()) == curlID {
			lift.Value("isAssistance").Boolean().IsTrue()
			found = true
		}
	}
	if !found {
		t.Fatal("the assistance lift is missing from the volume breakdown")
	}
}

// A lift the program prescribes is never labelled assistance, whatever else the
// lifter added that day.
func TestRackedLabelsPrescribedWorkAsMain(t *testing.T) {
	e := expect(t)
	_, dayID := firstProgramAndDay(e)

	created := startSession(t, e, dayID)
	sessionID := int(created.Value("id").Number().Raw())
	set := created.Value("sets").Array().Value(0).Object()
	exerciseID := int(set.Value("exerciseId").Number().Raw())
	logSet(e, sessionID, int(set.Value("id").Number().Raw()), 5, true)

	lifts := e.GET("/racked").Expect().Status(http.StatusOK).JSON().Object().Value("lifts").Array()
	for i := 0; i < int(lifts.Length().Raw()); i++ {
		lift := lifts.Value(i).Object()
		if int(lift.Value("exerciseId").Number().Raw()) == exerciseID {
			lift.Value("isAssistance").Boolean().IsFalse()
			return
		}
	}
	t.Fatal("the prescribed lift is missing from the volume breakdown")
}

// The bodyweight column reaches the recap as a dated series. Recording one is
// optional, so a period without any carries no section at all rather than a
// lifter who weighs nothing.
func TestRackedReportsWeighIns(t *testing.T) {
	e := expect(t)
	_, dayID := firstProgramAndDay(e)

	e.GET("/racked").WithQuery("on", "1970-01-15").Expect().Status(http.StatusOK).
		JSON().Object().Value("bodyweight").IsNull()

	created := startSession(t, e, dayID)
	sessionID := int(created.Value("id").Number().Raw())
	const weight = 181.4
	e.PATCH(fmt.Sprintf("/sessions/%d", sessionID)).
		WithJSON(map[string]any{"bodyweightLb": weight}).
		Expect().Status(http.StatusOK)

	body := e.GET("/racked").Expect().Status(http.StatusOK).
		JSON().Object().Value("bodyweight").Object()

	// The suite shares one account and this month may already hold weigh-ins
	// from another test, so this asserts the reading is present rather than that
	// it is the only one.
	points := body.Value("points").Array()
	var found bool
	for i := 0; i < int(points.Length().Raw()); i++ {
		if points.Value(i).Object().Value("weightLb").Number().Raw() == weight {
			found = true
		}
	}
	if !found {
		t.Fatalf("the %v lb weigh-in is missing from the bodyweight series", weight)
	}
	body.Value("highLb").Number().Ge(body.Value("lowLb").Number().Raw())
}

// A weigh-in is a fact about the day, not about the session it was typed into:
// it counts whether or not any set was ever logged. RackedPeriodSets filters on
// logged work and this query deliberately does not, and that difference is easy
// to erase by copying the wrong WHERE clause.
func TestRackedCountsAWeighInFromAnUnloggedSession(t *testing.T) {
	e := expect(t)
	_, dayID := firstProgramAndDay(e)

	created := startSession(t, e, dayID)
	sessionID := int(created.Value("id").Number().Raw())
	const weight = 923.5
	e.PATCH(fmt.Sprintf("/sessions/%d", sessionID)).
		WithJSON(map[string]any{"bodyweightLb": weight}).
		Expect().Status(http.StatusOK)

	points := e.GET("/racked").Expect().Status(http.StatusOK).
		JSON().Object().Value("bodyweight").Object().Value("points").Array()
	for i := 0; i < int(points.Length().Raw()); i++ {
		if points.Value(i).Object().Value("weightLb").Number().Raw() == weight {
			return
		}
	}
	t.Fatalf("a weigh-in on a session with no logged sets was dropped")
}

// rackedSplit reads the two sides of the work split, which the recap tests
// compare as deltas for the same reason rackedTotals does.
type rackedSplitFigures struct {
	main       float64
	assistance float64
}

func rackedSplit(e *httpexpect.Expect) rackedSplitFigures {
	split := e.GET("/racked").Expect().Status(http.StatusOK).
		JSON().Object().Value("split").Object()
	return rackedSplitFigures{
		main:       split.Value("main").Object().Value("volumeLb").Number().Raw(),
		assistance: split.Value("assistance").Object().Value("volumeLb").Number().Raw(),
	}
}
