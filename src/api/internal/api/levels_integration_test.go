package api_test

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/gavv/httpexpect/v2"
)

// The qualifying rule, end to end.
//
// What is pinned here is which sessions PAY, not what the roster adds up to — the
// suite shares one database and every other test leaves history behind, so an
// absolute figure for the install is a test that fails whenever a neighbour
// changes. Each of these makes its own lifter and asks only about that lifter's
// own entry, which is leaderboard_integration_test.go's pattern and for its reason.
//
// A fresh lifter is also the only way to assert the cases that earn NOTHING: those
// are assertions that a lifter is still at zero, and the primary account has
// trained all over this suite.

// levelEntry returns one lifter's entry from the site-wide read.
//
// Every account is listed, so a missing one is a failure rather than a way of
// saying "untrained" — an untrained lifter is present at level 1 with no XP.
func levelEntry(t *testing.T, e *httpexpect.Expect, lifterID int32) *httpexpect.Object {
	t.Helper()
	items := e.GET("/levels").Expect().Status(http.StatusOK).
		JSON().Object().Value("items").Array()
	for i := 0; i < int(items.Length().Raw()); i++ {
		entry := items.Value(i).Object()
		if int32(entry.Value("lifterId").Number().Raw()) == lifterID {
			return entry
		}
	}
	t.Fatalf("lifter %d is missing from /levels, which lists every account", lifterID)
	return nil
}

// levelXP is what most of these tests actually ask: how much experience one lifter
// is being credited with right now.
func levelXP(t *testing.T, e *httpexpect.Expect, lifterID int32) int {
	t.Helper()
	return int(levelEntry(t, e, lifterID).Value("xp").Number().Raw())
}

// trainAndQualify starts a session for the lifter and logs one real set into it,
// leaving it unfinished. What the caller does next is what each test is about.
func trainAndQualify(t *testing.T, e *httpexpect.Expect) int {
	t.Helper()
	_, dayID := firstProgramAndDay(e)
	created := startSession(t, e, dayID)
	sessionID := int(created.Value("id").Number().Raw())
	setID := int(created.Value("sets").Array().Value(0).Object().Value("id").Number().Raw())
	logSet(e, sessionID, setID, 5, true)
	return sessionID
}

// An account that has never trained is LISTED, at level 1. Leaving it out would
// make "untrained" and "no such lifter" the same answer on the wire.
func TestLevelsListsALifterWhoHasTrainedNothing(t *testing.T) {
	id, token := secondLifter(t, "levels-untrained")
	e := expectAs(t, token)

	entry := levelEntry(t, e, id)
	entry.HasValue("level", 1)
	entry.HasValue("xp", 0)
	entry.HasValue("xpIntoLevel", 0)
	// Level 2 is one session away, which is the curve's first step.
	entry.HasValue("xpForNextLevel", 100)
}

// The ordinary path, and the one that pins the curve's first step on the wire: one
// session is 100 XP, which is exactly level 2 with nothing spare.
func TestLevelsCountAFinishedSessionWithWork(t *testing.T) {
	id, token := secondLifter(t, "levels-finished")
	e := expectAs(t, token)

	sessionID := trainAndQualify(t, e)
	e.POST(fmt.Sprintf("/sessions/%d/finish", sessionID)).Expect().Status(http.StatusOK)

	entry := levelEntry(t, e, id)
	entry.HasValue("level", 2)
	entry.HasValue("xp", 100)
	// Standing exactly on the floor of level 2, which costs 200 to leave.
	entry.HasValue("xpIntoLevel", 0)
	entry.HasValue("xpForNextLevel", 200)
}

// THE CASE A FINISH-TIME AWARD WOULD MISS. A session that ages out past the
// twelve-hour cutoff never writes finished_at and never reaches the server again,
// so a lifter who trains and forgets to tap Finish has to be paid by the read or
// they are not paid at all.
func TestLevelsCountASessionThatAgedOutUnfinished(t *testing.T) {
	id, token := secondLifter(t, "levels-aged-out")
	e := expectAs(t, token)

	sessionID := trainAndQualify(t, e)
	backdateSession(t, sessionID, 13*time.Hour)

	// Nothing was posted to /finish, and nothing is going to be.
	e.GET(fmt.Sprintf("/sessions/%d", sessionID)).Expect().Status(http.StatusOK).
		JSON().Object().Value("finishedAt").IsNull()

	if got := levelXP(t, e, id); got != 100 {
		t.Fatalf("an aged-out session with work earned %d XP, want 100", got)
	}
}

// THE GUARD THE AGEING RULE NEEDS. Opening a session and walking away makes it
// "over" twelve hours later all on its own; without the work filter that would pay
// out, and the badge would be counting tabs opened rather than training done.
func TestLevelsIgnoreAnOverSessionWithNothingLogged(t *testing.T) {
	id, token := secondLifter(t, "levels-empty-session")
	e := expectAs(t, token)

	_, dayID := firstProgramAndDay(e)
	sessionID := int(startSession(t, e, dayID).Value("id").Number().Raw())
	backdateSession(t, sessionID, 13*time.Hour)

	// It really is over — this is the state the filter has to reject, not a
	// session that simply has not aged yet.
	e.GET(fmt.Sprintf("/sessions/%d", sessionID)).Expect().Status(http.StatusOK).
		JSON().Object().HasValue("isOver", true)

	if got := levelXP(t, e, id); got != 0 {
		t.Fatalf("an over session with nothing logged earned %d XP, want 0", got)
	}
}

// Experience is for training done, not training under way. A session still in
// progress pays when it ends, and the badge does not climb mid-workout.
func TestLevelsIgnoreASessionStillInProgress(t *testing.T) {
	id, token := secondLifter(t, "levels-in-progress")
	e := expectAs(t, token)

	sessionID := trainAndQualify(t, e)
	e.GET(fmt.Sprintf("/sessions/%d", sessionID)).Expect().Status(http.StatusOK).
		JSON().Object().HasValue("isOver", false)

	if got := levelXP(t, e, id); got != 0 {
		t.Fatalf("a session still in progress earned %d XP, want 0", got)
	}
}

// A set that missed its target is still training. `completed` means the set HIT its
// target reps, so qualifying on it would quietly stop paying anybody having a hard
// day — the opposite of what a badge for showing up should do.
func TestLevelsCountASessionWhereTheTargetWasMissed(t *testing.T) {
	id, token := secondLifter(t, "levels-missed-target")
	e := expectAs(t, token)

	_, dayID := firstProgramAndDay(e)
	created := startSession(t, e, dayID)
	sessionID := int(created.Value("id").Number().Raw())
	setID := int(created.Value("sets").Array().Value(0).Object().Value("id").Number().Raw())

	// Reps logged, target missed.
	logSet(e, sessionID, setID, 1, false)
	e.POST(fmt.Sprintf("/sessions/%d/finish", sessionID)).Expect().Status(http.StatusOK)

	if got := levelXP(t, e, id); got != 100 {
		t.Fatalf("a session of missed targets earned %d XP, want 100", got)
	}
}

// Nothing is stored, so a session that goes takes its experience with it. This is
// the property a counter on the lifter would not have, and the reason the count is
// derived rather than written down.
func TestLevelsFallWhenASessionIsDeleted(t *testing.T) {
	id, token := secondLifter(t, "levels-deleted-session")
	e := expectAs(t, token)

	// Built without startSession, whose t.Cleanup would delete it a second time
	// and take a 404 for it — this test does the deleting itself, on purpose.
	_, dayID := firstProgramAndDay(e)
	created := e.POST("/sessions").
		WithJSON(map[string]any{"programDayId": dayID}).
		Expect().Status(http.StatusCreated).JSON().Object()
	sessionID := int(created.Value("id").Number().Raw())
	setID := int(created.Value("sets").Array().Value(0).Object().Value("id").Number().Raw())
	logSet(e, sessionID, setID, 5, true)

	e.POST(fmt.Sprintf("/sessions/%d/finish", sessionID)).Expect().Status(http.StatusOK)
	if got := levelXP(t, e, id); got != 100 {
		t.Fatalf("the session earned %d XP before it was deleted, want 100", got)
	}

	e.DELETE(fmt.Sprintf("/sessions/%d", sessionID)).Expect().Status(http.StatusNoContent)

	if got := levelXP(t, e, id); got != 0 {
		t.Fatalf("experience outlived the session that explained it: %d XP, want 0", got)
	}
}

// The read is site-wide: it answers for everybody at once so a client can draw a
// level beside any name it renders, rather than asking per name.
func TestLevelsAnswerForOtherLiftersToo(t *testing.T) {
	id, token := secondLifter(t, "levels-seen-by-others")
	theirs := expectAs(t, token)

	sessionID := trainAndQualify(t, theirs)
	theirs.POST(fmt.Sprintf("/sessions/%d/finish", sessionID)).Expect().Status(http.StatusOK)

	// Read by somebody else entirely.
	if got := levelXP(t, expect(t), id); got != 100 {
		t.Fatalf("another lifter sees %d XP, want the 100 they earned", got)
	}
}

// The response is polled every ten minutes by every open tab, so the 304 is what
// keeps that cheap. jsonETag is middleware on the whole authenticated group, and
// this asserts the new route actually sits inside it.
func TestLevelsAreETagged(t *testing.T) {
	e := expect(t)

	etag := e.GET("/levels").Expect().Status(http.StatusOK).
		Header("ETag").NotEmpty().Raw()

	e.GET("/levels").WithHeader("If-None-Match", etag).
		Expect().Status(http.StatusNotModified).NoContent()
}

func TestLevelsNeedAuthentication(t *testing.T) {
	expectAnon(t).GET("/levels").Expect().Status(http.StatusUnauthorized)
}
