package api_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/gavv/httpexpect/v2"
)

// The level rungs, end to end.
//
// What is worth most here is what a SECOND pass does not do, which is also what the
// crown suite says about itself: a rung already held must not be announced again, and
// a lifter who has dropped back below one must not have it taken away. Neither is
// distinguishable from "has not run yet" by sleeping, which is why the reconciler has
// a synchronous seam.
//
// Every test makes its own lifter. The suite shares one database and the primary
// account has trained all over it, so its rungs are not a figure anybody can assert
// on — and the negative cases here are all "this lifter has nothing", which only
// holds for somebody new.

// lifterRungs reads one lifter's level achievements from their profile, slug → held.
func lifterRungs(t *testing.T, e *httpexpect.Expect, lifterID int32) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	items := e.GET(fmt.Sprintf("/lifters/%d/achievements", lifterID)).
		Expect().Status(http.StatusOK).JSON().Object().Value("items").Array()
	for _, item := range items.Iter() {
		a := item.Object().Value("achievement").Object()
		if a.Value("kind").String().Raw() != "level" {
			continue
		}
		out[a.Value("slug").String().Raw()] = item.Object().Value("heldNow").Boolean().Raw()
	}
	return out
}

// trainSessions puts n finished sessions with real work behind this lifter, which is
// n x 100 XP. Backdated so they do not all collide on today's date.
func trainSessions(t *testing.T, e *httpexpect.Expect, n int) {
	t.Helper()
	_, dayID := firstProgramAndDay(e)
	for i := range n {
		created := startSession(t, e, dayID)
		id := int(created.Value("id").Number().Raw())
		setID := int(created.Value("sets").Array().Value(0).Object().Value("id").Number().Raw())
		logSet(e, id, setID, 5, true)
		e.POST(fmt.Sprintf("/sessions/%d/finish", id)).Expect().Status(http.StatusOK)
		// Spread across days so nothing depends on one date carrying ten sessions.
		backdateSession(t, id, time.Duration(i+1)*24*time.Hour)
	}
}

// clearRungs removes one lifter's level reigns and the notifications they raised, so
// a test's own awards do not leak into the next one's.
func clearRungs(t *testing.T, lifterID int32) {
	t.Helper()
	t.Cleanup(func() {
		ctx := context.Background()
		if _, err := testPool.Exec(ctx, `DELETE FROM notifications
			WHERE achievement_slug IN (SELECT slug FROM achievements WHERE kind = 'level')
			  AND (user_id = $1 OR actor_id = $1)`, lifterID); err != nil {
			t.Errorf("clearing level notifications: %v", err)
		}
		if _, err := testPool.Exec(ctx, `DELETE FROM lifter_achievements
			WHERE user_id = $1
			  AND achievement_slug IN (SELECT slug FROM achievements WHERE kind = 'level')`,
			lifterID); err != nil {
			t.Errorf("clearing level reigns: %v", err)
		}
	})
}

// Level 5 is ten sessions. The session that reaches it is what awards the rung, in
// the same request — not an hour later on the sweeper.
func TestFinishingASessionAwardsTheRungItCrosses(t *testing.T) {
	id, token := secondLifter(t, "rung-crossed")
	e := expectAs(t, token)
	clearRungs(t, id)

	// Nine sessions is Level 4: nothing yet.
	trainSessions(t, e, 9)
	if held := lifterRungs(t, e, id); len(held) != 0 {
		t.Fatalf("held %v at level 4, want nothing", held)
	}

	// The tenth reaches Level 5.
	trainSessions(t, e, 1)

	held := lifterRungs(t, e, id)
	if !held["level-5"] {
		t.Errorf("level-5 not held after ten sessions, got %v", held)
	}
	if _, ok := held["level-10"]; ok {
		t.Error("level-10 awarded at Level 5")
	}
}

// A rung is PERMANENT, which is the whole reason it is a different kind from a
// crown. heldNow stays true and timesHeld stays 1 — there is no second reign to have.
func TestARungIsHeldOnceAndForever(t *testing.T) {
	id, token := secondLifter(t, "rung-permanent")
	e := expectAs(t, token)
	clearRungs(t, id)

	trainSessions(t, e, 10)

	items := e.GET(fmt.Sprintf("/lifters/%d/achievements", id)).
		Expect().Status(http.StatusOK).JSON().Object().Value("items").Array()
	found := false
	for _, item := range items.Iter() {
		obj := item.Object()
		if obj.Value("achievement").Object().Value("slug").String().Raw() != "level-5" {
			continue
		}
		found = true
		obj.HasValue("heldNow", true)
		obj.HasValue("timesHeld", 1)
	}
	if !found {
		t.Fatal("level-5 is not on the profile")
	}
}

// THE CASE THAT MAKES IT A CROSSING AND NOT A STANDING. Deleting the sessions drops
// the lifter back below Level 5 — /levels says so — and the rung stays held. The two
// disagree on purpose; see migration 0035.
func TestARungSurvivesDroppingBackBelowIt(t *testing.T) {
	id, token := secondLifter(t, "rung-survives-a-fall")
	e := expectAs(t, token)
	clearRungs(t, id)

	_, dayID := firstProgramAndDay(e)
	ids := make([]int, 0, 10)
	for range 10 {
		created := e.POST("/sessions").
			WithJSON(map[string]any{"programDayId": dayID}).
			Expect().Status(http.StatusCreated).JSON().Object()
		sessionID := int(created.Value("id").Number().Raw())
		setID := int(created.Value("sets").Array().Value(0).Object().Value("id").Number().Raw())
		logSet(e, sessionID, setID, 5, true)
		e.POST(fmt.Sprintf("/sessions/%d/finish", sessionID)).Expect().Status(http.StatusOK)
		ids = append(ids, sessionID)
	}
	if held := lifterRungs(t, e, id); !held["level-5"] {
		t.Fatalf("level-5 not held after ten sessions, got %v", held)
	}

	// Built without startSession because this test deletes them itself, and that
	// helper's cleanup would take a 404 for the second attempt.
	for _, sessionID := range ids {
		e.DELETE(fmt.Sprintf("/sessions/%d", sessionID)).
			Expect().Status(http.StatusNoContent)
	}

	// The standing fell.
	if got := levelXP(t, e, id); got != 0 {
		t.Errorf("experience is %d after deleting every session, want 0", got)
	}
	// The history did not.
	if held := lifterRungs(t, e, id); !held["level-5"] {
		t.Errorf("level-5 was taken away when the level fell, got %v", held)
	}

	// And a reconcile does not take it either, which is the thing a reign-closing
	// pass would get wrong. Run twice: the second is what proves the first was not
	// merely slow.
	testAPI.RefreshLevelAwardsNow(context.Background())
	testAPI.RefreshLevelAwardsNow(context.Background())
	if held := lifterRungs(t, e, id); !held["level-5"] {
		t.Errorf("a reconcile closed a rung the lifter had dropped below, got %v", held)
	}
}

// The lifter is told, and so is somebody following them. Crowns' delivery rule
// unchanged — the actor and their followers, nobody else — and the actor being a
// recipient of their own news is the exception both kinds now make.
func TestReachingARungNotifiesTheLifterAndAFollower(t *testing.T) {
	id, token := secondLifter(t, "rung-notifies")
	e := expectAs(t, token)
	clearRungs(t, id)

	_, followerToken := secondLifter(t, "rung-follower")
	follower := expectAs(t, followerToken)
	follower.POST(fmt.Sprintf("/me/following/%d", id)).Expect().Status(http.StatusNoContent)
	t.Cleanup(func() {
		follower.DELETE(fmt.Sprintf("/me/following/%d", id)).
			Expect().Status(http.StatusNoContent)
	})

	trainSessions(t, e, 10)

	for name, reader := range map[string]*httpexpect.Expect{
		"the lifter": e,
		"a follower": follower,
	} {
		if !hasLevelNotification(t, reader) {
			t.Errorf("%s was not told about the rung", name)
		}
	}
}

// A SECOND PASS SAYS NOTHING. The reconciler runs hourly and every lifter past a
// rung still holds it, so announcing on every pass would tell somebody about Level 5
// once an hour forever. Asserted by counting rather than by waiting.
func TestReconcilingAgainAnnouncesNothing(t *testing.T) {
	id, token := secondLifter(t, "rung-quiet-second-pass")
	e := expectAs(t, token)
	clearRungs(t, id)

	trainSessions(t, e, 10)
	before := levelNotificationCount(t, id)
	if before == 0 {
		t.Fatal("the crossing was never announced")
	}

	testAPI.RefreshLevelAwardsNow(context.Background())
	testAPI.RefreshLevelAwardsNow(context.Background())

	if after := levelNotificationCount(t, id); after != before {
		t.Errorf("two more passes raised %d notifications, want 0", after-before)
	}
}

// Several rungs can land at once — an account backfilled with a year of history, or
// a reconcile catching up a lifter whose sessions all aged out — and each is its own
// reign and its own row.
func TestSeveralRungsCanBeReachedAtOnce(t *testing.T) {
	id, token := secondLifter(t, "rung-several-at-once")
	e := expectAs(t, token)
	clearRungs(t, id)

	// 45 sessions is Level 10, which passes both the Level 5 and Level 10 rungs.
	trainSessions(t, e, 45)

	held := lifterRungs(t, e, id)
	for _, slug := range []string{"level-5", "level-10"} {
		if !held[slug] {
			t.Errorf("%s not held after 45 sessions, got %v", slug, held)
		}
	}
	if _, ok := held["level-20"]; ok {
		t.Error("level-20 awarded at Level 10")
	}
}

// A lifter who has trained nothing holds no rung, and the catalogue still lists all
// four so a profile can draw what is still to come.
func TestAnUntrainedLifterHoldsNoRung(t *testing.T) {
	id, token := secondLifter(t, "rung-untrained")
	e := expectAs(t, token)

	if held := lifterRungs(t, e, id); len(held) != 0 {
		t.Errorf("an untrained lifter holds %v, want nothing", held)
	}

	rungs := 0
	for _, item := range e.GET("/achievements").Expect().Status(http.StatusOK).
		JSON().Object().Value("items").Array().Iter() {
		a := item.Object().Value("achievement").Object()
		if a.Value("kind").String().Raw() != "level" {
			continue
		}
		rungs++
		// The threshold is on the wire so a surface need not read it out of the slug.
		a.Value("levelThreshold").Number().Gt(0)
		// And no level rung claims a board.
		a.NotContainsKey("metric")
	}
	if rungs != 4 {
		t.Errorf("%d level rungs in the catalogue, want 4", rungs)
	}
}

// hasLevelNotification reports whether this reader's panel carries a level row.
func hasLevelNotification(t *testing.T, e *httpexpect.Expect) bool {
	t.Helper()
	items := e.GET("/notifications").Expect().Status(http.StatusOK).
		JSON().Object().Value("items").Array()
	for _, item := range items.Iter() {
		if item.Object().Value("kind").String().Raw() == "level" {
			return true
		}
	}
	return false
}

// levelNotificationCount counts the level rows addressed to one lifter, read past the
// API because the panel folds them into one row per kind and this needs the rows.
func levelNotificationCount(t *testing.T, userID int32) int {
	t.Helper()
	var n int
	err := testPool.QueryRow(context.Background(), `SELECT count(*) FROM notifications
		WHERE user_id = $1 AND kind = 'level'`, userID).Scan(&n)
	if err != nil {
		t.Fatalf("counting level notifications: %v", err)
	}
	return n
}
