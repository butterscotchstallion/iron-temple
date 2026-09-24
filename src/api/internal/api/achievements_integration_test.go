package api_test

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"testing"
	"time"

	"github.com/gavv/httpexpect/v2"
)

// Crowns: the catalogue, the reconciler's diff, and the reigns it keeps.
//
// WHY THESE TESTS DRIVE THE PASS BY HAND
//
// refreshCrowns runs from the session sweeper, which the suite does not start
// (see TestMain). So nothing here has a crown until a test asks for one, which
// makes the ledger's state a property of the test rather than of how long the run
// has been going. RefreshCrownsNow is the synchronous seam for that — and most of
// what is worth asserting is what a SECOND pass does NOT do, which no amount of
// sleeping can distinguish from a pass that has not happened yet.
//
// WHY THE WINNER IS NEVER NAMED
//
// The board leader depends on every account the rest of the suite has left
// behind, so a test that expected a particular lifter on top would be asserting
// the order the suite happens to run in. What is actually under test is the
// agreement: whoever /leaderboard says is first is exactly who wears the crown.
// That holds on any install, covers ties for free, and is the invariant the
// feature would be broken without.

// refreshCrowns runs one reconcile and undoes it afterwards.
//
// The cleanup is not tidiness. A pass mints a notification for every lifter on
// the install, and the notification suite asserts on unread counts — leaving
// crowns behind would make this file's presence change those tests' answers.
func refreshCrowns(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	t.Cleanup(func() {
		if _, err := testPool.Exec(ctx, "DELETE FROM notifications WHERE kind = 'crown'"); err != nil {
			t.Errorf("clearing crown notifications: %v", err)
		}
		if _, err := testPool.Exec(ctx, "DELETE FROM lifter_achievements"); err != nil {
			t.Errorf("clearing reigns: %v", err)
		}
	})
	testAPI.RefreshCrownsNow(ctx)
}

// holdersByMetric reads /achievements into metric → sorted holder ids.
func holdersByMetric(e *httpexpect.Expect) map[string][]int {
	out := map[string][]int{}
	items := e.GET("/achievements").Expect().Status(http.StatusOK).
		JSON().Object().Value("items").Array()
	for _, item := range items.Iter() {
		obj := item.Object()
		metric := obj.Value("achievement").Object().Value("metric").String().Raw()
		ids := []int{}
		for _, h := range obj.Value("holders").Array().Iter() {
			ids = append(ids, int(h.Object().Value("id").Number().Raw()))
		}
		sort.Ints(ids)
		out[metric] = ids
	}
	return out
}

// leadersByMetric reads the month's boards into metric → sorted ids of the
// lifters who should be crowned.
//
// Which is NOT simply the rank-1 rows: a board whose leader is at zero awards
// nothing, because every idle account ties at zero on three of the five boards and
// a crown for training nothing is not a crown. The rule is duplicated here rather
// than read off /achievements, since /achievements is the thing under test.
func leadersByMetric(e *httpexpect.Expect) map[string][]int {
	out := map[string][]int{}
	for _, b := range boards(e, "period", "month").Iter() {
		obj := b.Object()
		ids := []int{}
		entries := obj.Value("entries").Array()
		if entries.Length().Raw() > 0 &&
			entries.Value(0).Object().Value("value").Number().Raw() > 0 {
			for _, entry := range entries.Iter() {
				row := entry.Object()
				if int(row.Value("rank").Number().Raw()) != 1 {
					break
				}
				ids = append(ids, int(row.Value("lifter").Object().Value("id").Number().Raw()))
			}
		}
		sort.Ints(ids)
		out[obj.Value("metric").String().Raw()] = ids
	}
	return out
}

// trainOnce logs one heavy set as this lifter, so at least one board has a leader
// above zero.
//
// Needed because of the zero rule above: on an install where nobody has trained
// this month every board is led at zero, nothing is crowned, and every assertion
// about a holder would pass by testing nothing.
func trainOnce(t *testing.T, e *httpexpect.Expect) {
	t.Helper()
	_, dayID := firstProgramAndDay(e)
	session := startSession(t, e, dayID)
	sessionID := int(session.Value("id").Number().Raw())
	setID := int(session.Value("sets").Array().Value(0).Object().Value("id").Number().Raw())
	logSet(e, sessionID, setID, 5, true)
}

// achievementFor returns one lifter's entry for a slug, or nil if they have none.
func achievementFor(e *httpexpect.Expect, lifterID int32, slug string) *httpexpect.Object {
	items := e.GET(fmt.Sprintf("/lifters/%d/achievements", lifterID)).
		Expect().Status(http.StatusOK).
		JSON().Object().Value("items").Array()
	for _, item := range items.Iter() {
		obj := item.Object()
		if obj.Value("achievement").Object().Value("slug").String().Raw() == slug {
			return obj
		}
	}
	return nil
}

// openReigns is every open reign as "slug/user@held_from" strings, sorted.
//
// Read from the table rather than through the API because held_from is the field
// that matters here and no endpoint exposes it for a reign in progress — and
// because comparing two whole snapshots is what says a pass changed nothing,
// which no per-lifter assertion can.
func openReigns(t *testing.T) []string {
	t.Helper()
	rows, err := testPool.Query(context.Background(),
		`SELECT achievement_slug, user_id, held_from FROM lifter_achievements
		 WHERE held_until IS NULL ORDER BY achievement_slug, user_id`)
	if err != nil {
		t.Fatalf("reading open reigns: %v", err)
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var slug string
		var user int32
		var from time.Time
		if err := rows.Scan(&slug, &user, &from); err != nil {
			t.Fatalf("scanning a reign: %v", err)
		}
		out = append(out, fmt.Sprintf("%s/%d@%s", slug, user, from.UTC().Format(time.RFC3339Nano)))
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterating reigns: %v", err)
	}
	return out
}

// openReign reports whether this lifter currently holds this achievement,
// according to the table rather than according to the endpoint that reads it.
func openReign(t *testing.T, lifterID int32, slug string) bool {
	t.Helper()
	var n int
	err := testPool.QueryRow(context.Background(),
		`SELECT COUNT(*) FROM lifter_achievements
		 WHERE user_id = $1 AND achievement_slug = $2 AND held_until IS NULL`,
		lifterID, slug).Scan(&n)
	if err != nil {
		t.Fatalf("counting open reigns: %v", err)
	}
	return n > 0
}

// ---- the catalogue ----

// Every board awards a crown, and a board with no crown would be a board whose
// leader is invisible everywhere except the leaderboard page. Asserted against
// the boards themselves rather than against a list of five metrics written here,
// so adding a board fails this test instead of silently shipping without a crown.
func TestEveryLeaderboardBoardHasACrown(t *testing.T) {
	e := expect(t)

	catalogue := map[string]bool{}
	for _, item := range e.GET("/achievements").Expect().Status(http.StatusOK).
		JSON().Object().Value("items").Array().Iter() {
		a := item.Object().Value("achievement").Object()
		a.Value("kind").String().IsEqual("crown")
		catalogue[a.Value("metric").String().Raw()] = true
	}

	for _, b := range boards(e).Iter() {
		metric := b.Object().Value("metric").String().Raw()
		if !catalogue[metric] {
			t.Errorf("board %q has no crown in the catalogue", metric)
		}
	}
}

// The catalogue is listed in the boards' own order, so a surface drawing crowns
// puts them in the same sequence the leaderboard offers its boards — volume last,
// for the reason leaderboard.go gives at length.
func TestCrownsAreListedInBoardOrder(t *testing.T) {
	e := expect(t)

	var crowns []string
	for _, item := range e.GET("/achievements").Expect().Status(http.StatusOK).
		JSON().Object().Value("items").Array().Iter() {
		crowns = append(crowns,
			item.Object().Value("achievement").Object().Value("metric").String().Raw())
	}

	var boardOrder []string
	for _, b := range boards(e).Iter() {
		boardOrder = append(boardOrder, b.Object().Value("metric").String().Raw())
	}

	if len(crowns) != len(boardOrder) {
		t.Fatalf("%d crowns for %d boards", len(crowns), len(boardOrder))
	}
	for i := range crowns {
		if crowns[i] != boardOrder[i] {
			t.Fatalf("crown %d is %q, want %q", i, crowns[i], boardOrder[i])
		}
	}
}

// Before a pass has run nobody holds anything, and that has to serialize as an
// entry with an empty list rather than as a missing one — a client draws the
// "still to win" rows off exactly these.
func TestEveryCrownIsListedEvenWhenNobodyHoldsIt(t *testing.T) {
	items := expect(t).GET("/achievements").Expect().Status(http.StatusOK).
		JSON().Object().Value("items").Array()
	items.Length().Gt(0)
	for _, item := range items.Iter() {
		item.Object().ContainsKey("holders")
	}
}

// ---- the reconciler ----

// THE INVARIANT. Whoever the leaderboard says is first is exactly who wears that
// board's crown — no more, no fewer. Ties are covered without being singled out:
// a board with three lifters level on rank 1 has three holders, and the sets are
// compared rather than the counts.
func TestCrownsGoToExactlyTheLiftersLeadingEachBoard(t *testing.T) {
	// Two accounts, so the install is not a leaderboard of one, and one of them
	// trains — otherwise every board is led at zero, nothing is crowned, and the
	// comparison below holds trivially while testing nothing.
	_, token := secondLifter(t, "crown-invariant-one")
	secondLifter(t, "crown-invariant-two")
	trainOnce(t, expectAs(t, token))

	refreshCrowns(t)

	e := expect(t)
	want := leadersByMetric(e)
	got := holdersByMetric(e)

	crowned := 0
	for metric, leaders := range want {
		holders, ok := got[metric]
		if !ok {
			t.Errorf("board %q has no crown", metric)
			continue
		}
		if fmt.Sprint(holders) != fmt.Sprint(leaders) {
			t.Errorf("board %q: crowned %v, leaderboard says %v", metric, holders, leaders)
		}
		crowned += len(holders)
	}
	// The positive branch was actually reached. Without this the whole test passes
	// on an install where every board is empty.
	if crowned == 0 {
		t.Fatal("nothing was crowned, so the agreement was never tested")
	}
}

// A crown says somebody did more than anyone else, so leading at zero earns
// nothing. Without this rule every idle account is joint-first on three of the
// five boards — sessions-a-week, streak and volume list a lifter at zero rather
// than omitting them — which would crown the whole install for training nothing
// and fan a notification out to every account for each of them.
func TestABoardLedAtZeroCrownsNobody(t *testing.T) {
	secondLifter(t, "crown-zero-one")
	secondLifter(t, "crown-zero-two")
	refreshCrowns(t)

	e := expect(t)
	holders := holdersByMetric(e)

	zeroBoards := 0
	for _, b := range boards(e, "period", "month").Iter() {
		obj := b.Object()
		metric := obj.Value("metric").String().Raw()
		entries := obj.Value("entries").Array()
		if entries.Length().Raw() == 0 {
			continue
		}
		if entries.Value(0).Object().Value("value").Number().Raw() > 0 {
			continue
		}
		zeroBoards++
		// The board lists lifters at rank 1 — that part is correct and stays — and
		// none of them wears anything for it.
		if got := holders[metric]; len(got) != 0 {
			t.Errorf("board %q is led at zero but crowned %v", metric, got)
		}
	}
	if zeroBoards == 0 {
		t.Skip("every board on this install has a leader above zero")
	}
}

// A REIGN IS A CONTINUOUS STRETCH, and this is the test that says so. The hourly
// pass must leave a holder who is still on top exactly as it found them:
// restamping would turn a month on top into hundreds of reigns and make
// timesHeld a measure of server uptime instead of of winning.
func TestAPassLeavesARunningReignAlone(t *testing.T) {
	_, token := secondLifter(t, "crown-reign-stable")
	// Somebody has to be above zero or the ledger stays empty and there is no
	// reign to leave alone.
	trainOnce(t, expectAs(t, token))
	refreshCrowns(t)

	// Every open reign, read from the table rather than through a holder this test
	// has to identify. Which lifter leads depends on what the rest of the suite has
	// left behind; that a running reign is untouched does not.
	before := openReigns(t)
	if len(before) == 0 {
		t.Fatal("no reigns were opened, so there is nothing to test")
	}

	// Again, with nothing about the standings changed.
	testAPI.RefreshCrownsNow(context.Background())

	after := openReigns(t)
	if fmt.Sprint(after) != fmt.Sprint(before) {
		t.Errorf("a second pass rewrote the reigns:\n before %v\n after  %v", before, after)
	}
}

// Losing a crown CLOSES the reign and keeps it. That is the whole reason the
// ledger exists rather than a cache of who is top: nothing else in the database
// records who led a board last month.
//
// The dethroning is staged by opening a reign the standings do not support, which
// is the one thing the API cannot be asked to do and the reconciler must undo. It
// exercises the same close path a real overtaking does — CloseAchievementReignsExcept
// does not know why somebody is no longer a leader.
func TestLosingACrownKeepsItAsHistory(t *testing.T) {
	id, token := secondLifter(t, "crown-dethroned")
	ctx := context.Background()

	// ATTENDANCE, and the choice is load-bearing. A crown this account cannot
	// possibly be leading has to be one it is not even ELIGIBLE for, and volume is
	// not that: a fresh lifter has moved zero pounds, every other idle account has
	// moved zero pounds too, and tied figures share rank 1 — so a lifter who has
	// done nothing is legitimately joint-first on volume whenever nobody has
	// trained this month.
	//
	// Attendance is the one board that LEAVES LIFTERS OFF rather than ranking them
	// at zero: it measures sessions against the weekdays a lifter's own program
	// asks for, and an account with no current program has no schedule to be
	// measured against. So this account is absent from that board, cannot be among
	// its leaders, and its staged reign must be closed.
	const slug = "crown-attendance"
	if _, err := testPool.Exec(ctx,
		`INSERT INTO lifter_achievements (user_id, achievement_slug, held_from)
		 VALUES ($1, $2, now() - interval '2 days')`, id, slug); err != nil {
		t.Fatalf("staging a reign: %v", err)
	}

	mine := expectAs(t, token)
	achievementFor(mine, id, slug).Value("heldNow").Boolean().IsTrue()

	refreshCrowns(t)

	if openReign(t, id, slug) {
		t.Fatal("the reign is still open after the standings disagreed with it")
	}
	// Kept, not deleted: still listed, no longer held, and still counted as one
	// time held.
	lapsed := achievementFor(mine, id, slug)
	if lapsed == nil {
		t.Fatal("a closed reign vanished from the lifter's achievements")
	}
	lapsed.Value("heldNow").Boolean().IsFalse()
	lapsed.Value("timesHeld").Number().IsEqual(1)
}

// Taking a crown again after losing it is a SECOND reign, which is what makes
// timesHeld worth printing.
func TestRetakingACrownCountsAsecondReign(t *testing.T) {
	id, token := secondLifter(t, "crown-retaken")
	ctx := context.Background()
	const slug = "crown-volume"

	// A reign that has already ended, as if they had been overtaken.
	if _, err := testPool.Exec(ctx,
		`INSERT INTO lifter_achievements (user_id, achievement_slug, held_from, held_until)
		 VALUES ($1, $2, now() - interval '9 days', now() - interval '8 days')`,
		id, slug); err != nil {
		t.Fatalf("staging a closed reign: %v", err)
	}
	// And a fresh one on top of it.
	if _, err := testPool.Exec(ctx,
		`INSERT INTO lifter_achievements (user_id, achievement_slug, held_from)
		 VALUES ($1, $2, now() - interval '1 day')`, id, slug); err != nil {
		t.Fatalf("staging an open reign: %v", err)
	}
	t.Cleanup(func() {
		if _, err := testPool.Exec(ctx,
			"DELETE FROM lifter_achievements WHERE user_id = $1", id); err != nil {
			t.Errorf("clearing staged reigns: %v", err)
		}
	})

	entry := achievementFor(expectAs(t, token), id, slug)
	if entry == nil {
		t.Fatal("no entry for a lifter with two reigns")
	}
	entry.Value("timesHeld").Number().IsEqual(2)
	entry.Value("heldNow").Boolean().IsTrue()
}

// ---- what the install is told ----

// A crown is announced to everybody EXCEPT the lifter who took it. That is the
// notifications table's rule — the actor is always filtered out of the recipients
// — and the right answer besides: the holder learns it from the crown on their
// own name, and being told you did the thing you are looking at is noise.
func TestTakingACrownTellsEverybodyButTheHolder(t *testing.T) {
	_, token := secondLifter(t, "crown-announce")
	// Somebody above zero, or the zero rule means no crown is taken and there is
	// no announcement to assert on.
	trainOnce(t, expectAs(t, token))
	refreshCrowns(t)

	ctx := context.Background()

	// NOBODY is told about their own. Asserted across every crown the pass raised
	// rather than for one account, because this is the table's invariant and one
	// escaping row is the whole failure.
	//
	// Note what this is NOT: "the holder received no crown notification at all".
	// A lifter is told when SOMEBODY ELSE takes a crown, which is the point of the
	// notification — so the pair to count is (recipient = actor), not the
	// recipient alone.
	var toThemselves int
	if err := testPool.QueryRow(ctx,
		`SELECT COUNT(*) FROM notifications WHERE kind = 'crown' AND user_id = actor_id`).
		Scan(&toThemselves); err != nil {
		t.Fatalf("counting self-notifications: %v", err)
	}
	if toThemselves != 0 {
		t.Fatalf("%d lifters were told about their own crown", toThemselves)
	}

	// And the fan-out reaches EVERYBODY else. One crown means one row per other
	// account on the install — the same rule CreateJoinNotifications applies —
	// so a per-crown recipient count that is short means somebody was skipped.
	var accounts int
	if err := testPool.QueryRow(ctx, "SELECT COUNT(*) FROM users").Scan(&accounts); err != nil {
		t.Fatalf("counting accounts: %v", err)
	}
	rows, err := testPool.Query(ctx,
		`SELECT actor_id, achievement_slug, COUNT(*)
		 FROM notifications WHERE kind = 'crown'
		 GROUP BY actor_id, achievement_slug`)
	if err != nil {
		t.Fatalf("grouping crown notifications: %v", err)
	}
	defer rows.Close()

	announced := 0
	for rows.Next() {
		var actor int32
		var slug *string
		var told int
		if err := rows.Scan(&actor, &slug, &told); err != nil {
			t.Fatalf("scanning: %v", err)
		}
		announced++
		// Every row names its board, so the panel can say which one was won.
		if slug == nil {
			t.Errorf("a crown taken by %d names no board", actor)
		}
		if told != accounts-1 {
			t.Errorf("crown %v by %d reached %d lifters, want %d",
				slug, actor, told, accounts-1)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterating: %v", err)
	}
	// The pass opened reigns on an install with several accounts, so it must have
	// announced something — an assertion loop that ran zero times would pass while
	// saying nothing.
	if announced == 0 {
		t.Fatal("the pass opened no reigns, so nothing here was tested")
	}
}

// A crown that has not moved is not news. Without this the install would announce
// the same standing every hour forever, which is the failure the reconciler's
// ON CONFLICT exists to prevent.
func TestAnUnchangedCrownIsNotAnnouncedTwice(t *testing.T) {
	secondLifter(t, "crown-quiet")
	refreshCrowns(t)

	ctx := context.Background()
	count := func() int {
		var n int
		if err := testPool.QueryRow(ctx,
			"SELECT COUNT(*) FROM notifications WHERE kind = 'crown'").Scan(&n); err != nil {
			t.Fatalf("counting crown notifications: %v", err)
		}
		return n
	}

	after := count()
	testAPI.RefreshCrownsNow(ctx)
	if again := count(); again != after {
		t.Fatalf("a second pass raised %d more notifications", again-after)
	}
}

// ---- access ----

func TestAchievementsRejectAnonymousCallers(t *testing.T) {
	anon := expectAnon(t)
	anon.GET("/achievements").Expect().Status(http.StatusUnauthorized)
	anon.GET("/lifters/1/achievements").Expect().Status(http.StatusUnauthorized)
}

// The other half of that gate: an account that has not replaced the password
// somebody else chose for it is refused everywhere except the two endpoints it
// needs to get out.
func TestAchievementsAreGatedUntilThePasswordChanges(t *testing.T) {
	const username = "crown-gated"
	const password = "crown-gated-pw"
	createAccount(t, username, password)
	gated := expectAs(t, signIn(t, username, password))

	gated.GET("/achievements").Expect().Status(http.StatusForbidden).
		JSON().Object().Value("code").String().IsEqual("password_change_required")
	gated.GET("/lifters/1/achievements").Expect().Status(http.StatusForbidden).
		JSON().Object().Value("code").String().IsEqual("password_change_required")
}

// An id that names nobody is a 404, not an empty list. A lifter who has earned
// nothing and a lifter who does not exist are different answers, and only one of
// them is a profile section.
func TestAchievementsOfAnUnknownLifterAreNotFound(t *testing.T) {
	expect(t).GET("/lifters/99999999/achievements").
		Expect().Status(http.StatusNotFound)
}

func TestAchievementsAreReadOnly(t *testing.T) {
	e := expect(t)
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		e.Request(method, "/achievements").WithJSON(map[string]any{}).
			Expect().Status(http.StatusMethodNotAllowed)
	}
}
