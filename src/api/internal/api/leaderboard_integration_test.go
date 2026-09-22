package api_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/gavv/httpexpect/v2"
)

// How the lifters compare.
//
// The board contents cannot be asserted absolutely — the suite shares one database
// and every other test leaves history behind — so what is pinned here is the
// SHAPE and the RULES: which boards exist and in what order, that ranks handle
// ties, that a lifter appears when a metric is defined for them rather than when
// it is non-zero, and that the figures match the lifter's own Racked page.

func boards(e *httpexpect.Expect, query ...any) *httpexpect.Array {
	req := e.GET("/leaderboard")
	for i := 0; i+1 < len(query); i += 2 {
		req = req.WithQuery(fmt.Sprint(query[i]), query[i+1])
	}
	return req.Expect().Status(http.StatusOK).
		JSON().Object().Value("boards").Array()
}

func boardFor(e *httpexpect.Expect, metric string) *httpexpect.Object {
	return boards(e).
		Find(func(_ int, value *httpexpect.Value) bool {
			return value.Object().Value("metric").String().Raw() == metric
		}).Object()
}

// entryFor finds one lifter's row on a board, or nil if they are not listed.
func entryFor(board *httpexpect.Object, username string) *httpexpect.Object {
	for _, entry := range board.Value("entries").Array().Iter() {
		obj := entry.Object()
		if obj.Value("lifter").Object().Value("username").String().Raw() == username {
			return obj
		}
	}
	return nil
}

// ---- shape and order ----

// The order encodes an opinion about fairness: raw tonnage ranks lifters by
// bodyweight and training age, so it goes last, and measures relative to the
// lifter lead. This is the test that fails if somebody "tidies" the slice.
func TestLeaderboardOrdersBoardsFairestFirst(t *testing.T) {
	got := boards(expect(t))
	got.Length().IsEqual(5)

	want := []string{"sessionsPerWeek", "attendance", "improvement", "streak", "volume"}
	for i, metric := range want {
		got.Value(i).Object().HasValue("metric", metric)
	}
}

func TestLeaderboardBoardsCarryTheirOwnLabelsAndUnits(t *testing.T) {
	units := map[string]string{
		"sessionsPerWeek": "per_week",
		"attendance":      "percent",
		"improvement":     "percent",
		"streak":          "count",
		"volume":          "pounds",
	}
	for metric, unit := range units {
		board := boardFor(expect(t), metric)
		board.HasValue("unit", unit)
		board.Value("label").String().NotEmpty()
		// The note is on the wire because a board that omits lifters has to say so
		// where it is drawn.
		board.Value("note").String().NotEmpty()
		// Present and an array even when nobody qualifies — never null, so a
		// client branches on length.
		board.Value("entries").Array()
	}
}

func TestLeaderboardLabelsThePeriodItMeasured(t *testing.T) {
	e := expect(t)
	for _, period := range []string{"week", "month", "year"} {
		e.GET("/leaderboard").WithQuery("period", period).
			Expect().Status(http.StatusOK).
			JSON().Object().Value("period").Object().HasValue("kind", period)
	}
}

func TestLeaderboardValidatesItsWindow(t *testing.T) {
	e := expect(t)
	e.GET("/leaderboard").WithQuery("period", "fortnight").
		Expect().Status(http.StatusBadRequest)
	e.GET("/leaderboard").WithQuery("on", "last-tuesday").
		Expect().Status(http.StatusBadRequest)
}

// ---- who is listed ----

// A lifter who trained nothing still appears on the volume board, at zero. Hiding
// them would flatter the board — and would quietly turn a leaderboard of everyone
// into a leaderboard of the people doing well.
func TestLeaderboardListsALifterWhoTrainedNothing(t *testing.T) {
	secondLifter(t, "board-idle")

	entry := entryFor(boardFor(expect(t), "volume"), "board-idle")
	if entry == nil {
		t.Fatal("a lifter who trained nothing should still be on the volume board")
	}
	entry.HasValue("value", 0)
}

// A lifter with no scheduled weekdays has no attendance, which is different from
// an attendance of zero: racked's own comment says Rate must not be shown without
// a basis, because it is a percentage against a denominator nobody entered.
func TestLeaderboardOmitsLiftersWithNoScheduleFromAttendance(t *testing.T) {
	secondLifter(t, "board-unscheduled")
	e := expect(t)

	// They are on the universal board...
	if entryFor(boardFor(e, "sessionsPerWeek"), "board-unscheduled") == nil {
		t.Error("every lifter should be on the sessions-a-week board")
	}
	// ...and off the one that needs a schedule.
	if entryFor(boardFor(e, "attendance"), "board-unscheduled") != nil {
		t.Error("a lifter with no scheduled weekdays should not be graded on attendance")
	}
}

// Nor does a lifter with no repeated lift have an improvement to report.
func TestLeaderboardOmitsLiftersWithNothingToImproveOn(t *testing.T) {
	_, token := secondLifter(t, "board-nothing-twice")
	theirs := expectAs(t, token)

	// One session, so no lift has been performed twice.
	_, dayID := firstProgramAndDay(theirs)
	session := startSession(t, theirs, dayID)
	sessionID := int(session.Value("id").Number().Raw())
	setID := int(session.Value("sets").Array().Value(0).Object().Value("id").Number().Raw())
	logSet(theirs, sessionID, setID, 5, true)

	if entryFor(boardFor(expect(t), "improvement"), "board-nothing-twice") != nil {
		t.Error("a lifter with no lift performed twice has no improvement to report")
	}
}

// ---- the figures agree with the lifter's own page ----

// The whole social layer rests on there being one implementation of each
// statistic. This is the leaderboard's version of that assertion: the volume it
// ranks a lifter by is the volume that lifter reads on their own Racked page.
func TestLeaderboardFiguresMatchTheLiftersOwnRacked(t *testing.T) {
	_, token := secondLifter(t, "board-agrees")
	theirs := expectAs(t, token)

	_, dayID := firstProgramAndDay(theirs)
	session := startSession(t, theirs, dayID)
	sessionID := int(session.Value("id").Number().Raw())
	setID := int(session.Value("sets").Array().Value(0).Object().Value("id").Number().Raw())
	logSet(theirs, sessionID, setID, 5, true)

	own := theirs.GET("/racked").Expect().Status(http.StatusOK).JSON().Object()
	ownVolume := own.Value("totals").Object().Value("volumeLb").Number().Raw()
	ownPerWeek := own.Value("attendance").Object().Value("sessionsPerWeek").Number().Raw()

	entry := entryFor(boardFor(expect(t), "volume"), "board-agrees")
	if entry == nil {
		t.Fatal("the lifter should be on the volume board")
	}
	entry.Value("value").Number().IsEqual(ownVolume)

	perWeek := entryFor(boardFor(expect(t), "sessionsPerWeek"), "board-agrees")
	if perWeek == nil {
		t.Fatal("the lifter should be on the sessions-a-week board")
	}
	perWeek.Value("value").Number().IsEqual(ownPerWeek)
}

// The caller is on the board, unlike the feed. A leaderboard you are not on is
// not a leaderboard.
func TestLeaderboardIncludesTheCaller(t *testing.T) {
	if entryFor(boardFor(expect(t), "volume"), primaryUsername) == nil {
		t.Error("the caller should be on the board they are reading")
	}
}

// ---- ranking ----

// Ranks descend, start at 1, and are computed server-side so every surface agrees.
func TestLeaderboardRanksDescendFromOne(t *testing.T) {
	entries := boardFor(expect(t), "volume").Value("entries").Array()
	entries.NotEmpty()
	entries.Value(0).Object().HasValue("rank", 1)

	var lastValue float64 = -1
	for i, entry := range entries.Iter() {
		obj := entry.Object()
		value := obj.Value("value").Number().Raw()
		if i > 0 && value > lastValue {
			t.Fatalf("entry %d has value %v, above the one before it (%v)", i, value, lastValue)
		}
		lastValue = value
	}
}

// Equal figures share a rank and the next distinct figure skips the ones they used
// up: 1, 2, 2, 4. Two fresh accounts both sit at zero volume, which is the tie.
func TestLeaderboardTiedFiguresShareARank(t *testing.T) {
	secondLifter(t, "board-tie-one")
	secondLifter(t, "board-tie-two")

	entries := boardFor(expect(t), "volume").Value("entries").Array()

	// Walk the board checking the rule holds everywhere, rather than looking for
	// the two accounts just made — other tests leave zero-volume lifters behind and
	// the tie group may be larger than two.
	var values []float64
	var ranks []int
	for _, entry := range entries.Iter() {
		obj := entry.Object()
		values = append(values, obj.Value("value").Number().Raw())
		ranks = append(ranks, int(obj.Value("rank").Number().Raw()))
	}

	sawTie := false
	for i := range values {
		switch {
		case i == 0:
			if ranks[0] != 1 {
				t.Fatalf("first rank is %d, want 1", ranks[0])
			}
		case values[i] == values[i-1]:
			sawTie = true
			if ranks[i] != ranks[i-1] {
				t.Errorf("entries %d and %d are both %v but ranked %d and %d",
					i-1, i, values[i], ranks[i-1], ranks[i])
			}
		default:
			// A new figure takes the position it actually occupies, skipping the
			// places the tie above it used up.
			if ranks[i] != i+1 {
				t.Errorf("entry %d follows a distinct figure and is ranked %d, want %d",
					i, ranks[i], i+1)
			}
		}
	}
	if !sawTie {
		t.Skip("no two lifters are tied in this run, so the sharing rule was not exercised")
	}
}

// ---- the gate ----

func TestLeaderboardRejectsAnonymousCallers(t *testing.T) {
	expectAnon(t).GET("/leaderboard").Expect().Status(http.StatusUnauthorized)
}

func TestLeaderboardIsGatedUntilThePasswordChanges(t *testing.T) {
	createAccount(t, "board-gated", "board-gated-pw")
	token := signIn(t, "board-gated", "board-gated-pw")

	expectAs(t, token).GET("/leaderboard").Expect().
		Status(http.StatusForbidden).
		JSON().Object().HasValue("code", "password_change_required")
}

func TestLeaderboardIsReadOnly(t *testing.T) {
	e := expect(t)
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete} {
		e.Request(method, "/leaderboard").WithJSON(map[string]any{}).
			Expect().Status(http.StatusMethodNotAllowed)
	}
}
