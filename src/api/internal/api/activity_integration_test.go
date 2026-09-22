package api_test

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/gavv/httpexpect/v2"

	"gitea.homelab/gitadmin/iron-temple/api/internal/activity"
)

// Generated training activity.
//
// Two things here matter more than the rest. The authorisation, because these are
// the most powerful handlers in the app — one fabricates history and one deletes
// accounts. And that the history is REAL: generated through the actual progression
// engine, so an install populated this way exercises the same code a lifter's own
// history would, rather than looking plausible and being wrong.

// tearDownActivity removes the generated accounts. Registered by every test that
// creates them, because they are ordinary accounts on a database the whole suite
// shares — left behind, they would appear in every roster, feed and leaderboard
// assertion in the package.
func tearDownActivity(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		expect(t).DELETE("/admin/activity").Expect().Status(http.StatusOK)
	})
}

// ---- who can reach it ----

func TestActivityRoutesRejectAnonymousCallers(t *testing.T) {
	e := expectAnon(t)
	e.GET("/admin/activity").Expect().Status(http.StatusUnauthorized)
	e.POST("/admin/activity/backfill").
		WithJSON(map[string]any{"lifters": 2, "weeks": 2}).
		Expect().Status(http.StatusUnauthorized)
	e.POST("/admin/activity/start").
		WithJSON(map[string]any{"lifters": 2, "tickSeconds": 10}).
		Expect().Status(http.StatusUnauthorized)
	e.POST("/admin/activity/stop").Expect().Status(http.StatusUnauthorized)
	e.DELETE("/admin/activity").Expect().Status(http.StatusUnauthorized)
}

// An ordinary lifter must not be able to fabricate history or delete accounts.
// Asserted on every verb rather than the cheapest one: this is the whole
// authorisation model for the feature.
func TestActivityRoutesRejectNonAdmins(t *testing.T) {
	_, token := secondLifter(t, "activity-outsider")
	e := expectAs(t, token)

	for _, call := range []struct {
		method, path string
		body         map[string]any
	}{
		{http.MethodGet, "/admin/activity", nil},
		{http.MethodPost, "/admin/activity/backfill", map[string]any{"lifters": 2, "weeks": 2}},
		{http.MethodPost, "/admin/activity/start", map[string]any{"lifters": 2, "tickSeconds": 10}},
		{http.MethodPost, "/admin/activity/stop", nil},
		{http.MethodDelete, "/admin/activity", nil},
	} {
		req := e.Request(call.method, call.path)
		if call.body != nil {
			req = req.WithJSON(call.body)
		}
		req.Expect().
			Status(http.StatusForbidden).
			JSON().Object().HasValue("code", "admin_required")
	}
}

// ---- bounds ----

func TestActivityBackfillValidatesItsRequest(t *testing.T) {
	e := expect(t)
	for _, body := range []map[string]any{
		{"lifters": 0, "weeks": 4},
		{"lifters": 99, "weeks": 4},
		{"lifters": -1, "weeks": 4},
		{"lifters": 2, "weeks": 0},
		{"lifters": 2, "weeks": 27},
		{"lifters": 2, "weeks": -4},
	} {
		e.POST("/admin/activity/backfill").WithJSON(body).
			Expect().Status(http.StatusBadRequest)
	}
}

func TestActivityStartValidatesItsRequest(t *testing.T) {
	e := expect(t)
	for _, body := range []map[string]any{
		{"lifters": 2, "tickSeconds": 1},
		{"lifters": 2, "tickSeconds": 0},
		{"lifters": 2, "tickSeconds": 4000},
		{"lifters": 0, "tickSeconds": 10},
		{"lifters": 99, "tickSeconds": 10},
	} {
		e.POST("/admin/activity/start").WithJSON(body).
			Expect().Status(http.StatusBadRequest)
	}
}

// ---- status ----

func TestActivityStatusReportsItsBoundsAndIsIdleByDefault(t *testing.T) {
	status := expect(t).GET("/admin/activity").Expect().
		Status(http.StatusOK).JSON().Object()

	// Sent so a client cannot offer a number the endpoints would refuse.
	status.Value("maxLifters").Number().Gt(0)
	status.HasValue("maxWeeks", 26)
	status.Value("running").Boolean()
}

// The roster is published so a client can name the accounts a teardown considers.
// They are candidates rather than casualties — the hash decides which actually go,
// see TestActivityTeardownSparesARealLifterNamedAfterAPersona — but naming them is
// still what lets an operator see the scope before agreeing to it.
func TestActivityStatusPublishesTheRosterATeardownWouldMatch(t *testing.T) {
	status := expect(t).GET("/admin/activity").Expect().
		Status(http.StatusOK).JSON().Object()

	roster := status.Value("roster").Array()
	// Every name, whether or not the account exists yet: teardown sweeps all of
	// them, so all of them are in scope.
	roster.Length().IsEqual(status.Value("maxLifters").Number().Raw())
	for _, name := range roster.Iter() {
		name.String().NotEmpty()
	}
}

// A loop that fails to start must not leave the status claiming one is running.
//
// The flag is raised by the handler before the goroutine is scheduled, so every
// exit from the loop has to release it — including the early ones. Provoked here
// with a teardown, which removes the accounts out from under a running loop.
func TestActivityStatusDoesNotClaimAStoppedLoopIsRunning(t *testing.T) {
	e := expect(t)
	// A short tick, so the loop actually reaches its body during the test.
	e.POST("/admin/activity/start").
		WithJSON(map[string]any{"lifters": 2, "tickSeconds": 5}).
		Expect().Status(http.StatusNoContent)

	e.POST("/admin/activity/stop").Expect().Status(http.StatusNoContent)
	e.GET("/admin/activity").Expect().Status(http.StatusOK).
		JSON().Object().HasValue("running", false)
}

// Restarting must not be broken by the previous loop winding down afterwards.
//
// Starting replaces rather than refuses, so the old goroutine is cancelled and the
// new one installed at once — then the old one wakes LATER to exit. If its cleanup
// cleared the flag unconditionally it would switch off the loop the admin had just
// started, which is why the runner tracks a generation.
func TestActivityRestartSurvivesTheOldLoopWindingDown(t *testing.T) {
	tearDownActivity(t)
	e := expect(t)

	e.POST("/admin/activity/start").
		WithJSON(map[string]any{"lifters": 2, "tickSeconds": 5}).
		Expect().Status(http.StatusNoContent)
	e.POST("/admin/activity/start").
		WithJSON(map[string]any{"lifters": 3, "tickSeconds": 3600}).
		Expect().Status(http.StatusNoContent)

	// Long enough that the first loop has certainly been scheduled and exited.
	time.Sleep(500 * time.Millisecond)

	status := e.GET("/admin/activity").Expect().Status(http.StatusOK).JSON().Object()
	status.HasValue("running", true)
	status.HasValue("lifters", 3)
	status.HasValue("tickSeconds", 3600)

	e.POST("/admin/activity/stop").Expect().Status(http.StatusNoContent)
}

// Concurrent starts must leave exactly one loop running.
//
// Taking over the slot and cancelling the loop it replaces used to be two separate
// lock acquisitions, so two starts could interleave as stopA, stopB, beginA, beginB
// — and B would overwrite A's CancelFunc without anybody ever calling it, leaving
// goroutine A running forever alongside B, doubling the activity and leaking until
// the process exited. The generation counter does not catch that; only doing both
// under one lock does.
//
// What this can assert over HTTP is that the reported state is coherent and that a
// stop afterwards genuinely stops everything. The goroutine leak itself is what the
// -race build and the runner's own accounting would show.
func TestActivityConcurrentStartsLeaveOneLoop(t *testing.T) {
	tearDownActivity(t)
	e := expect(t)

	// Fired together rather than in sequence: sequential starts were already
	// handled by the generation counter, and it is the overlap that was broken.
	var wg sync.WaitGroup
	for i := range 6 {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			// A fresh client per goroutine — httpexpect instances are not built to
			// be shared across them.
			expectAs(t, primaryToken).POST("/admin/activity/start").
				WithJSON(map[string]any{"lifters": 2, "tickSeconds": 3600 - n}).
				Expect().Status(http.StatusNoContent)
		}(i)
	}
	wg.Wait()

	// One loop, and the status describes a real one rather than a mixture.
	status := e.GET("/admin/activity").Expect().Status(http.StatusOK).JSON().Object()
	status.HasValue("running", true)
	status.HasValue("lifters", 2)
	tick := int(status.Value("tickSeconds").Number().Raw())
	if tick < 3595 || tick > 3600 {
		t.Errorf("tickSeconds is %d, which is not one of the ticks that were started", tick)
	}

	// And one stop is enough. If a start had leaked a loop, the flag would clear
	// while an uncancelled goroutine kept going — which is the state this exists to
	// prevent and the reason the cancel must never be dropped.
	e.POST("/admin/activity/stop").Expect().Status(http.StatusNoContent)
	e.GET("/admin/activity").Expect().Status(http.StatusOK).
		JSON().Object().HasValue("running", false)
}

// Stopping is idempotent: the caller asked for a state and it holds either way.
func TestActivityStopIsIdempotent(t *testing.T) {
	e := expect(t)
	e.POST("/admin/activity/stop").Expect().Status(http.StatusNoContent)
	e.POST("/admin/activity/stop").Expect().Status(http.StatusNoContent)
	e.GET("/admin/activity").Expect().Status(http.StatusOK).
		JSON().Object().HasValue("running", false)
}

func TestActivityStartThenStopFlipsTheFlag(t *testing.T) {
	tearDownActivity(t)
	e := expect(t)

	// A long tick, so nothing actually fires during the test — what is under
	// assertion is the flag and the reported shape, not the loop's output.
	e.POST("/admin/activity/start").
		WithJSON(map[string]any{"lifters": 2, "tickSeconds": 3600}).
		Expect().Status(http.StatusNoContent)

	running := e.GET("/admin/activity").Expect().Status(http.StatusOK).JSON().Object()
	running.HasValue("running", true)
	running.HasValue("lifters", 2)
	running.HasValue("tickSeconds", 3600)
	running.Value("startedAt").String().NotEmpty()

	e.POST("/admin/activity/stop").Expect().Status(http.StatusNoContent)
	e.GET("/admin/activity").Expect().Status(http.StatusOK).
		JSON().Object().HasValue("running", false)
}

// Starting again replaces the loop rather than refusing, because that is what an
// admin does when they want a different tick.
func TestActivityStartTwiceReplacesTheLoop(t *testing.T) {
	tearDownActivity(t)
	e := expect(t)

	e.POST("/admin/activity/start").
		WithJSON(map[string]any{"lifters": 2, "tickSeconds": 3600}).
		Expect().Status(http.StatusNoContent)
	e.POST("/admin/activity/start").
		WithJSON(map[string]any{"lifters": 3, "tickSeconds": 1800}).
		Expect().Status(http.StatusNoContent)

	status := e.GET("/admin/activity").Expect().Status(http.StatusOK).JSON().Object()
	status.HasValue("running", true)
	status.HasValue("lifters", 3)
	status.HasValue("tickSeconds", 1800)

	e.POST("/admin/activity/stop").Expect().Status(http.StatusNoContent)
}

// ---- what a backfill produces ----

func TestActivityBackfillCreatesLiftersWithHistory(t *testing.T) {
	tearDownActivity(t)
	e := expect(t)

	summary := e.POST("/admin/activity/backfill").
		WithJSON(map[string]any{"lifters": 3, "weeks": 4}).
		Expect().Status(http.StatusOK).JSON().Object()

	summary.HasValue("accounts", 3)
	summary.Value("sessions").Number().Gt(0)

	// They show up as ordinary lifters, because that is all they are.
	roster := e.GET("/lifters").Expect().Status(http.StatusOK).JSON().Array()
	roster.Length().Ge(4) // the owner plus three

	// And with real training behind them, dated in the past.
	listed := 0
	for _, entry := range roster.Iter() {
		obj := entry.Object()
		if obj.Value("username").String().Raw() == primaryUsername {
			continue
		}
		if _, ok := obj.Raw()["lastTrainedOn"]; ok {
			listed++
		}
	}
	if listed == 0 {
		t.Error("no generated lifter has a last-trained date")
	}
}

// A second backfill adds history to the lifters the first one made rather than
// failing on a taken username — and says it created nobody new.
func TestActivityBackfillIsRerunnable(t *testing.T) {
	tearDownActivity(t)
	e := expect(t)

	first := e.POST("/admin/activity/backfill").
		WithJSON(map[string]any{"lifters": 2, "weeks": 2}).
		Expect().Status(http.StatusOK).JSON().Object()
	first.HasValue("accounts", 2)

	second := e.POST("/admin/activity/backfill").
		WithJSON(map[string]any{"lifters": 2, "weeks": 2}).
		Expect().Status(http.StatusOK).JSON().Object()
	second.HasValue("accounts", 0)
}

// The weights come from the real progression engine, which is the claim that makes
// generated history worth having. A session's sets must carry a load the app would
// actually have prescribed — never zero, and matching the target reps it was
// asked for.
func TestActivityBackfillUsesTheRealPrescription(t *testing.T) {
	tearDownActivity(t)
	e := expect(t)

	e.POST("/admin/activity/backfill").
		WithJSON(map[string]any{"lifters": 2, "weeks": 3}).
		Expect().Status(http.StatusOK)

	// Read one generated lifter's session back through the ordinary endpoints.
	lifterID := 0
	for _, entry := range e.GET("/lifters").Expect().Status(http.StatusOK).JSON().Array().Iter() {
		obj := entry.Object()
		if obj.Value("username").String().Raw() != primaryUsername {
			lifterID = int(obj.Value("id").Number().Raw())
			break
		}
	}
	if lifterID == 0 {
		t.Fatal("no generated lifter found")
	}

	// Their Racked report is the same report they would read themselves, so a
	// non-zero volume here means real sets at real weights.
	racked := e.GET(fmt.Sprintf("/lifters/%d/racked", lifterID)).
		WithQuery("period", "year").
		Expect().Status(http.StatusOK).JSON().Object()
	racked.Value("totals").Object().Value("volumeLb").Number().Gt(0)
	racked.Value("totals").Object().Value("sets").Number().Gt(0)
}

// Sessions get a plausible clock, not the instant they were generated. Duration
// drives session pace, the fastest-session highlight and the ranking of a workout
// against its own history — an install full of zero-second sessions shows those
// features working on nonsense.
func TestActivityBackfillGivesSessionsARealDuration(t *testing.T) {
	tearDownActivity(t)
	e := expect(t)

	e.POST("/admin/activity/backfill").
		WithJSON(map[string]any{"lifters": 2, "weeks": 3}).
		Expect().Status(http.StatusOK)

	var shortest int
	err := testPool.QueryRow(context.Background(), `
		SELECT COALESCE(MIN(EXTRACT(EPOCH FROM (s.finished_at - s.created_at))::int), -1)
		FROM sessions s
		JOIN users u ON u.id = s.user_id
		WHERE NOT u.is_admin AND s.finished_at IS NOT NULL`).Scan(&shortest)
	if err != nil {
		t.Fatalf("read durations: %v", err)
	}
	if shortest < 0 {
		t.Fatal("no finished generated session to measure")
	}
	// Half an hour is the floor the generator picks from; anything near zero means
	// the clock was never set.
	if shortest < 20*60 {
		t.Errorf("shortest generated session lasted %ds, which is not a workout", shortest)
	}
}

// No lifter applauds their own session. The generator reads through the feed query,
// which excludes the viewer's own — so this holds without an explicit check, and
// this test is what says so.
func TestActivityBackfillNeverReactsToItsOwnSessions(t *testing.T) {
	tearDownActivity(t)
	expect(t).POST("/admin/activity/backfill").
		WithJSON(map[string]any{"lifters": 3, "weeks": 4}).
		Expect().Status(http.StatusOK)

	var selfReactions int
	err := testPool.QueryRow(context.Background(), `
		SELECT COUNT(*)
		FROM session_reactions r
		JOIN sessions s ON s.id = r.session_id
		WHERE s.user_id = r.user_id`).Scan(&selfReactions)
	if err != nil {
		t.Fatalf("count self-reactions: %v", err)
	}
	if selfReactions != 0 {
		t.Errorf("%d reactions are on the reactor's own session, which the API forbids", selfReactions)
	}
}

// Every generated comment must satisfy the rules addSessionComment enforces, since
// this path does not go through that handler.
func TestActivityBackfillWritesAcceptableComments(t *testing.T) {
	tearDownActivity(t)
	expect(t).POST("/admin/activity/backfill").
		WithJSON(map[string]any{"lifters": 3, "weeks": 6}).
		Expect().Status(http.StatusOK)

	var bad int
	err := testPool.QueryRow(context.Background(), `
		SELECT COUNT(*) FROM session_comments
		WHERE btrim(body) = '' OR char_length(body) > 256`).Scan(&bad)
	if err != nil {
		t.Fatalf("count comments: %v", err)
	}
	if bad != 0 {
		t.Errorf("%d generated comments are blank or over the cap", bad)
	}
}

// Generated lifters must not echo each other. Asserted against the rows a real
// backfill actually wrote, not just against the phrase sets — the unit tests prove
// the sets do not overlap, and this proves the runner hands each persona its own.
//
// A shared pool used to mean two lifters could both say "nice one" on the same
// session, which reads as one generator wearing several names.
func TestActivityBackfillGivesEachLifterItsOwnVoice(t *testing.T) {
	tearDownActivity(t)
	// Six lifters over twelve weeks, so there are enough comments for an overlap to
	// show up if one existed.
	expect(t).POST("/admin/activity/backfill").
		WithJSON(map[string]any{"lifters": 6, "weeks": 12}).
		Expect().Status(http.StatusOK)

	rows, err := testPool.Query(context.Background(), `
		SELECT c.body, COUNT(DISTINCT c.user_id) AS authors
		FROM session_comments c
		JOIN users u ON u.id = c.user_id
		WHERE NOT u.is_admin
		GROUP BY c.body
		HAVING COUNT(DISTINCT c.user_id) > 1`)
	if err != nil {
		t.Fatalf("query comments: %v", err)
	}
	defer rows.Close()

	shared := 0
	for rows.Next() {
		var body string
		var authors int
		if err := rows.Scan(&body, &authors); err != nil {
			t.Fatalf("scan: %v", err)
		}
		t.Errorf("%q was said by %d different lifters", body, authors)
		shared++
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}

	// And the run has to have actually produced comments, or the check above is
	// vacuously true.
	var total int
	if err := testPool.QueryRow(context.Background(), `
		SELECT COUNT(*) FROM session_comments c
		JOIN users u ON u.id = c.user_id WHERE NOT u.is_admin`).Scan(&total); err != nil {
		t.Fatalf("count comments: %v", err)
	}
	if total == 0 {
		t.Fatal("the backfill wrote no comments, so nothing was tested")
	}
	t.Logf("%d comments across 6 lifters, %d shared phrases", total, shared)
}

// A GENERATED ACCOUNT MUST NOT BE ABLE TO SIGN IN.
//
// Those accounts hold a real hash of a password that is a constant in this
// repository, and they do not owe a forced change — so without a block at the door
// anyone who has read the source could authenticate as one of the roster's lifters
// and post comments and reactions as them. That the generator is admin-only says
// nothing about the login route.
//
// The credential cannot just be made unusable, because teardown proves which
// accounts it created by verifying that same hash. Refusing at login keeps both:
// the hash stays verifiable server-side and is worthless as a way in.
func TestActivityGeneratedAccountsCannotSignIn(t *testing.T) {
	tearDownActivity(t)
	expect(t).POST("/admin/activity/backfill").
		WithJSON(map[string]any{"lifters": 3, "weeks": 1}).
		Expect().Status(http.StatusOK)

	// The exact constant the generator uses. Spelled out rather than referenced so
	// this test fails loudly if somebody changes it without thinking about this
	// route — which is the whole point of having the test.
	const generated = "generated-activity-not-for-sign-in"

	for _, username := range activity.Usernames(3) {
		// Refused, and refused with the same answer a wrong password gets, so the
		// response does not disclose which accounts are generated.
		body := expectAnon(t).POST("/auth/login").
			WithJSON(map[string]any{"username": username, "password": generated}).
			Expect().Status(http.StatusUnauthorized).
			JSON().Object()
		body.Value("message").String().IsEqual("invalid username or password")

		// And no cookie came back, which is the thing that would actually matter.
		expectAnon(t).POST("/auth/login").
			WithJSON(map[string]any{"username": username, "password": generated}).
			Expect().Status(http.StatusUnauthorized).
			Cookies().NotContainsAll(sessionCookie)
	}
}

// A real account is unaffected — the block is on the credential, and refusing every
// login would be a cure worse than the disease.
func TestRealAccountsCanStillSignIn(t *testing.T) {
	createAccount(t, "still-can-log-in", "still-can-log-in-pw")

	expectAnon(t).POST("/auth/login").
		WithJSON(map[string]any{
			"username": "still-can-log-in",
			"password": "still-can-log-in-pw",
		}).
		Expect().Status(http.StatusOK).
		Cookie(sessionCookie).Value().NotEmpty()
}

// ---- teardown ----

func TestActivityTeardownRemovesWhatItMadeAndNothingElse(t *testing.T) {
	e := expect(t)

	// A real account that must survive, alongside the generated ones.
	_, bystanderToken := secondLifter(t, "activity-bystander")

	e.POST("/admin/activity/backfill").
		WithJSON(map[string]any{"lifters": 3, "weeks": 2}).
		Expect().Status(http.StatusOK)

	removed := e.DELETE("/admin/activity").Expect().Status(http.StatusOK).
		JSON().Object().Value("removed").Number().Raw()
	if removed != 3 {
		t.Errorf("teardown removed %v accounts, want 3", removed)
	}

	// The owner and the bystander are untouched — the owner because the query
	// refuses an admin outright, the bystander because its name is not on the
	// roster.
	e.GET("/me").Expect().Status(http.StatusOK)
	expectAs(t, bystanderToken).GET("/me").Expect().Status(http.StatusOK)

	roster := e.GET("/lifters").Expect().Status(http.StatusOK).JSON().Array()
	for _, entry := range roster.Iter() {
		name := entry.Object().Value("username").String().Raw()
		if name != primaryUsername && name != "activity-bystander" {
			t.Errorf("account %q survived teardown", name)
		}
	}
}

// THE FINDING THIS FEATURE'S TEARDOWN EXISTS TO NOT HAVE.
//
// A real lifter the owner created by hand, who happens to be named after a
// persona, must survive a teardown with everything they have logged. The roster
// names are ordinary household ones on purpose, so a collision is plausible rather
// than contrived — and the accounts carry no marker, so the only thing that can
// tell them apart is the password. A generated account has the fixed one the
// generator uses; a real lifter chose their own.
//
// If teardown ever goes back to matching on username alone, this is the test that
// fails, and what it is protecting is somebody's training history.
func TestActivityTeardownSparesARealLifterNamedAfterAPersona(t *testing.T) {
	// The first roster name, created the ordinary way with a password of its own.
	persona := activity.Usernames(1)[0]
	_, token := secondLifter(t, persona)
	impostor := expectAs(t, token)

	// Give them real history, so a wrongful delete would cost something the test
	// can actually observe.
	_, dayID := firstProgramAndDay(impostor)
	session := startSession(t, impostor, dayID)
	sessionID := int(session.Value("id").Number().Raw())
	setID := int(session.Value("sets").Array().Value(0).Object().Value("id").Number().Raw())
	logSet(impostor, sessionID, setID, 5, true)

	// Generate alongside them, then tear down.
	e := expect(t)
	e.POST("/admin/activity/backfill").
		WithJSON(map[string]any{"lifters": 3, "weeks": 2}).
		Expect().Status(http.StatusOK)
	e.DELETE("/admin/activity").Expect().Status(http.StatusOK)

	// They still exist, still signed in, and still own their session.
	impostor.GET("/me").Expect().Status(http.StatusOK).
		JSON().Object().HasValue("username", persona)
	impostor.GET(fmt.Sprintf("/sessions/%d", sessionID)).Expect().Status(http.StatusOK)

	// And the generated ones are gone — the teardown was not simply a no-op that
	// spared everybody.
	for _, name := range activity.Usernames(3)[1:] {
		if entryPresent(e, name) {
			t.Errorf("generated account %q survived teardown", name)
		}
	}
}

// entryPresent reports whether a username appears on the roster endpoint.
func entryPresent(e *httpexpect.Expect, username string) bool {
	for _, entry := range e.GET("/lifters").Expect().Status(http.StatusOK).JSON().Array().Iter() {
		if entry.Object().Value("username").String().Raw() == username {
			return true
		}
	}
	return false
}

// Nothing to remove is not an error: the caller asked for an install with no
// generated accounts, and it has one.
func TestActivityTeardownOnAnUntouchedInstall(t *testing.T) {
	expect(t).DELETE("/admin/activity").Expect().Status(http.StatusOK).
		JSON().Object().HasValue("removed", 0)
}

// Teardown stops the loop first. A loop still writing while its accounts are
// deleted would spend the next tick failing into the log.
func TestActivityTeardownStopsTheLoop(t *testing.T) {
	e := expect(t)
	e.POST("/admin/activity/start").
		WithJSON(map[string]any{"lifters": 2, "tickSeconds": 3600}).
		Expect().Status(http.StatusNoContent)

	e.DELETE("/admin/activity").Expect().Status(http.StatusOK)

	e.GET("/admin/activity").Expect().Status(http.StatusOK).
		JSON().Object().HasValue("running", false)
}

// Generated accounts must never administer the install: the single-admin index
// permits exactly one and the owner holds it, and teardown's NOT is_admin guard
// depends on these never being one.
func TestActivityAccountsAreNeverAdmins(t *testing.T) {
	tearDownActivity(t)
	expect(t).POST("/admin/activity/backfill").
		WithJSON(map[string]any{"lifters": 3, "weeks": 1}).
		Expect().Status(http.StatusOK)

	users := expect(t).GET("/admin/users").Expect().Status(http.StatusOK).JSON().Array()
	admins := 0
	for _, user := range users.Iter() {
		obj := user.Object()
		if obj.Value("isAdmin").Boolean().Raw() {
			admins++
			obj.HasValue("username", primaryUsername)
		}
		// Nor may they owe a password change: nothing signs in as them to clear
		// it, and the flag would lock them out of the app they exist to populate.
		if obj.Value("username").String().Raw() != primaryUsername {
			obj.HasValue("mustChangePassword", false)
		}
	}
	if admins != 1 {
		t.Errorf("%d accounts administer the install, want exactly 1", admins)
	}
}

// The generated lifters must be indistinguishable from real ones on the wire —
// there is no marker in the schema and none in any response, which is the whole
// reason teardown works by re-deriving the roster.
func TestActivityAccountsCarryNoMarkerOnTheWire(t *testing.T) {
	tearDownActivity(t)
	e := expect(t)
	e.POST("/admin/activity/backfill").
		WithJSON(map[string]any{"lifters": 2, "weeks": 1}).
		Expect().Status(http.StatusOK)

	for _, entry := range e.GET("/lifters").Expect().Status(http.StatusOK).JSON().Array().Iter() {
		obj := entry.Object()
		for _, key := range []string{"generated", "isBot", "bot", "simulated", "synthetic"} {
			obj.NotContainsKey(key)
		}
	}
}

// A generated lifter reads exactly like any other through the social endpoints —
// which is the real test of whether the history is usable for looking at these
// screens.
func TestActivityBackfillPopulatesTheSocialSurfaces(t *testing.T) {
	tearDownActivity(t)
	e := expect(t)

	e.POST("/admin/activity/backfill").
		WithJSON(map[string]any{"lifters": 4, "weeks": 8}).
		Expect().Status(http.StatusOK)

	// The feed has other people's sessions in it.
	e.GET("/feed").Expect().Status(http.StatusOK).
		JSON().Object().Value("items").Array().NotEmpty()

	// And the leaderboard ranks more than one lifter.
	boards := e.GET("/leaderboard").WithQuery("period", "year").
		Expect().Status(http.StatusOK).JSON().Object().Value("boards").Array()
	boards.NotEmpty()
	boards.Value(0).Object().Value("entries").Array().Length().Ge(2)
}

// ---- the unattended daily run ----

// resetActivitySchedule turns the daily run off and clears the record of which days
// have been generated, so one test's schedule cannot leak into the next.
func resetActivitySchedule(t *testing.T) {
	t.Helper()
	off := func() {
		expect(t).PUT("/admin/activity/schedule").
			WithJSON(map[string]any{"enabled": false, "lifters": 4}).
			Expect().Status(http.StatusOK)
		_, _ = testPool.Exec(context.Background(), `DELETE FROM generated_activity_runs`)
	}
	off()
	t.Cleanup(off)
}

func TestActivityScheduleRoutesRejectNonAdmins(t *testing.T) {
	_, token := secondLifter(t, "schedule-outsider")
	e := expectAs(t, token)

	e.GET("/admin/activity/schedule").Expect().
		Status(http.StatusForbidden).JSON().Object().HasValue("code", "admin_required")
	e.PUT("/admin/activity/schedule").
		WithJSON(map[string]any{"enabled": true, "lifters": 2}).
		Expect().Status(http.StatusForbidden).
		JSON().Object().HasValue("code", "admin_required")
}

func TestActivityScheduleRejectsAnonymousCallers(t *testing.T) {
	e := expectAnon(t)
	e.GET("/admin/activity/schedule").Expect().Status(http.StatusUnauthorized)
	e.PUT("/admin/activity/schedule").
		WithJSON(map[string]any{"enabled": true, "lifters": 2}).
		Expect().Status(http.StatusUnauthorized)
}

// Off by default, so an install that migrates to this version generates nothing
// until its owner asks.
func TestActivityScheduleIsOffUntilAskedFor(t *testing.T) {
	resetActivitySchedule(t)
	expect(t).GET("/admin/activity/schedule").Expect().Status(http.StatusOK).
		JSON().Object().HasValue("enabled", false)
}

// Persisted rather than held in memory, which is the whole reason this exists
// alongside the live loop: a flag on the process cannot run daily.
func TestActivityScheduleSurvivesBeingReadBack(t *testing.T) {
	resetActivitySchedule(t)
	e := expect(t)

	updated := e.PUT("/admin/activity/schedule").
		WithJSON(map[string]any{"enabled": true, "lifters": 3}).
		Expect().Status(http.StatusOK).JSON().Object()
	updated.HasValue("enabled", true)
	updated.HasValue("lifters", 3)

	read := e.GET("/admin/activity/schedule").Expect().Status(http.StatusOK).JSON().Object()
	read.HasValue("enabled", true)
	read.HasValue("lifters", 3)
}

// Bounded exactly as the manual endpoints are, so a schedule cannot ask for a
// roster the generator would refuse.
func TestActivityScheduleValidatesItsRequest(t *testing.T) {
	resetActivitySchedule(t)
	e := expect(t)
	for _, body := range []map[string]any{
		{"enabled": true, "lifters": 0},
		{"enabled": true, "lifters": 99},
		{"enabled": true, "lifters": -1},
	} {
		e.PUT("/admin/activity/schedule").WithJSON(body).
			Expect().Status(http.StatusBadRequest)
	}
}

// THE PROPERTY THE WHOLE DESIGN RESTS ON: a day is generated at most once.
//
// Each day is claimed through a primary key before any work happens, so repeated
// passes — a restart, an hourly tick, a second replica — find the day taken and do
// nothing. Without that, every tick would add another day's training to the same
// date and an install would inflate by the hour.
func TestActivitySchedulerGeneratesEachDayOnce(t *testing.T) {
	resetActivitySchedule(t)
	tearDownActivity(t)
	e := expect(t)

	e.PUT("/admin/activity/schedule").
		WithJSON(map[string]any{"enabled": true, "lifters": 3}).
		Expect().Status(http.StatusOK)

	// Driven directly rather than waited for: the scheduler's ticker is an hour, and
	// what is under test is the claim rather than the clock.
	testAPI.GenerateDueActivityForTest(context.Background())

	var afterFirst int
	countRuns := func() int {
		var n int
		if err := testPool.QueryRow(context.Background(),
			`SELECT COUNT(*) FROM generated_activity_runs`).Scan(&n); err != nil {
			t.Fatalf("count runs: %v", err)
		}
		return n
	}
	afterFirst = countRuns()
	if afterFirst == 0 {
		t.Fatal("the first pass recorded no days")
	}

	sessionsAfterFirst := generatedSessionCount(t)

	// Three more passes. Every day is already claimed, so nothing should change.
	for range 3 {
		testAPI.GenerateDueActivityForTest(context.Background())
	}

	if got := countRuns(); got != afterFirst {
		t.Errorf("runs went from %d to %d across repeated passes", afterFirst, got)
	}
	if got := generatedSessionCount(t); got != sessionsAfterFirst {
		t.Errorf("generated sessions went from %d to %d across repeated passes",
			sessionsAfterFirst, got)
	}
}

// Nothing happens while the schedule is off, however many times the scheduler runs.
func TestActivitySchedulerDoesNothingWhileDisabled(t *testing.T) {
	resetActivitySchedule(t)
	before := generatedSessionCount(t)

	for range 3 {
		testAPI.GenerateDueActivityForTest(context.Background())
	}

	var runs int
	if err := testPool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM generated_activity_runs`).Scan(&runs); err != nil {
		t.Fatalf("count runs: %v", err)
	}
	if runs != 0 {
		t.Errorf("a disabled schedule recorded %d days", runs)
	}
	if got := generatedSessionCount(t); got != before {
		t.Errorf("a disabled schedule generated %d sessions", got-before)
	}
}

// The last run is reported so the admin screen can say the thing is actually
// working rather than merely switched on.
func TestActivityScheduleReportsItsLastRun(t *testing.T) {
	resetActivitySchedule(t)
	tearDownActivity(t)
	e := expect(t)

	e.PUT("/admin/activity/schedule").
		WithJSON(map[string]any{"enabled": true, "lifters": 3}).
		Expect().Status(http.StatusOK)
	testAPI.GenerateDueActivityForTest(context.Background())

	schedule := e.GET("/admin/activity/schedule").Expect().Status(http.StatusOK).JSON().Object()
	// Ordered by DAY, so this is how far the activity reaches — which after a
	// catch-up is today rather than whichever row happened to be written last.
	schedule.Value("lastRunOn").String().IsEqual(time.Now().UTC().Format("2006-01-02"))
}

// generatedSessionCount is how many sessions belong to non-admin accounts.
func generatedSessionCount(t *testing.T) int {
	t.Helper()
	var n int
	if err := testPool.QueryRow(context.Background(), `
		SELECT COUNT(*) FROM sessions s
		JOIN users u ON u.id = s.user_id
		WHERE NOT u.is_admin`).Scan(&n); err != nil {
		t.Fatalf("count sessions: %v", err)
	}
	return n
}

// A CATCH-UP MUST NOT PILE COMMENTS ONTO THE OLDEST SESSIONS.
//
// Recognition runs once per day generated, and a catch-up covers up to eight days in
// a single pass. Unbounded, each of those walked the same sixty-session feed, so the
// earliest sessions collected eight rounds of comments at once — reactions are
// idempotent on their primary key, but comments are not and must not be, because a
// lifter commenting twice on a session is legitimate in the real feature.
//
// Two bounds together. A window means only a couple of passes see a given session at
// all, and a per-lifter guard means any one of them adds at most one comment — so the
// assertion here is the strong one: no lifter says more than one thing about the same
// workout, however long the catch-up runs.
func TestActivityCatchUpDoesNotPileCommentsOnOneSession(t *testing.T) {
	resetActivitySchedule(t)
	tearDownActivity(t)
	e := expect(t)

	// History first, so the catch-up has older sessions available to pile onto — the
	// failure mode needs something already there to be the victim.
	e.POST("/admin/activity/backfill").
		WithJSON(map[string]any{"lifters": 4, "weeks": 4}).
		Expect().Status(http.StatusOK)

	e.PUT("/admin/activity/schedule").
		WithJSON(map[string]any{"enabled": true, "lifters": 4}).
		Expect().Status(http.StatusOK)

	// One pass covers the whole catch-up window at once, which is precisely the case
	// that used to multiply.
	testAPI.GenerateDueActivityForTest(context.Background())

	rows, err := testPool.Query(context.Background(), `
		SELECT c.session_id, c.user_id, COUNT(*) AS n
		FROM session_comments c
		JOIN users u ON u.id = c.user_id
		WHERE NOT u.is_admin
		GROUP BY c.session_id, c.user_id
		HAVING COUNT(*) > 1`)
	if err != nil {
		t.Fatalf("query comments: %v", err)
	}
	defer rows.Close()

	for rows.Next() {
		var sessionID, userID, n int
		if err := rows.Scan(&sessionID, &userID, &n); err != nil {
			t.Fatalf("scan: %v", err)
		}
		t.Errorf("lifter %d commented %d times on session %d", userID, n, sessionID)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows: %v", err)
	}
}

// The catch-up still records every day in its window, so bounding recognition did not
// quietly stop the thing from running.
func TestActivityCatchUpRecordsEveryDayInTheWindow(t *testing.T) {
	resetActivitySchedule(t)
	tearDownActivity(t)
	e := expect(t)

	e.PUT("/admin/activity/schedule").
		WithJSON(map[string]any{"enabled": true, "lifters": 3}).
		Expect().Status(http.StatusOK)
	testAPI.GenerateDueActivityForTest(context.Background())

	var days int
	if err := testPool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM generated_activity_runs`).Scan(&days); err != nil {
		t.Fatalf("count runs: %v", err)
	}
	// A week plus today, which is what DueDays yields for the default window.
	if days != 8 {
		t.Errorf("the catch-up recorded %d days, want 8", days)
	}
}
