package api_test

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/gavv/httpexpect/v2"

	"gitea.homelab/gitadmin/iron-temple/api/internal/progression"
)

// The layoff deload.
//
// The progression engine only ever saw sessions, never the calendar, so a
// month away prescribed the weight you were advancing to the day you stopped.
// These pin the two halves of the fix that the pure engine tests cannot reach:
// that the API measures the layoff off real history, and that the answer
// travels from the preview into the session that gets materialized.
//
// Two definitions of "trained" meet here and are deliberately not the same.
// The layoff is measured from any session with a logged rep (ListSessions),
// because you trained that day whether or not you remembered to tap Finish.
// The progression engine additionally requires the session to be over
// (ListLiftHistory), so these tests finish the session as well as log against
// it — otherwise there would be a layoff but no history to cut.

// daysAway is how far back to date a session to read as `weeks` away.
//
// Deliberately not 7×weeks, which lands exactly on a week boundary where one
// day either way changes the answer. A date column truncated in a different
// zone than the test's clock is a day either way, so the backdate is placed
// mid-window: ±1 day still floors to the same week, and the test measures the
// feature rather than the server's timezone.
func daysAway(weeks int) int { return 7*weeks + 3 }

// trainedWeeksAgo puts one finished, logged session in the account's history at
// a date `weeks` in the past, and returns its id plus the top weight worked on
// the first lift. Cleaned up with the test.
func trainedWeeksAgo(t *testing.T, e *httpexpect.Expect, dayID, weeks int) (sessionID int, weightLb float64) {
	t.Helper()

	created := startSession(t, e, dayID)
	sessionID = int(created.Value("id").Number().Raw())
	firstSet := created.Value("sets").Array().Value(0).Object()
	weightLb = firstSet.Value("weightLb").Number().Raw()

	// One logged rep is what separates a session that happened from one that
	// was merely opened, for both definitions above.
	logSet(e, sessionID, int(firstSet.Value("id").Number().Raw()), 5, true)
	e.POST(fmt.Sprintf("/sessions/%d/finish", sessionID)).Expect().Status(http.StatusOK)

	backdatePerformedOn(t, sessionID, time.Now().AddDate(0, 0, -daysAway(weeks)))
	return sessionID, weightLb
}

// preview reads a day's prescription, optionally asking for the layoff deload.
func preview(e *httpexpect.Expect, programID, dayID int, deload bool) *httpexpect.Object {
	return e.GET(fmt.Sprintf("/programs/%d/days/%d/next-session", programID, dayID)).
		WithQuery("deload", deload).
		Expect().Status(http.StatusOK).JSON().Object()
}

// A layoff is reported but never imposed: the weights come back untouched until
// the lifter says otherwise. This is the whole reason the field is on every
// preview rather than only on a deloading one — it is what tells the UI there
// is a question to ask.
func TestLayoffIsReportedWithoutBeingApplied(t *testing.T) {
	e := expect(t)
	programID, dayID := firstProgramAndDay(e)
	_, worked := trainedWeeksAgo(t, e, dayID, 3)

	body := preview(e, programID, dayID, false)

	layoff := body.Value("layoff").Object()
	layoff.Value("weeks").Number().IsEqual(3)
	layoff.Value("deloadPct").Number().IsEqual(0.3)
	layoff.Value("applied").Boolean().IsFalse()
	layoff.Value("lastTrainedOn").String().NotEmpty()

	// One set of five logged, so the lift held rather than advanced — and it
	// held at the weight that was worked, not at a cut one.
	first := body.Value("exercises").Array().Value(0).Object()
	first.Value("weightLb").Number().IsEqual(worked)
	first.Value("progression").Object().Value("status").String().IsEqual("hold")
	first.Value("progression").Object().Value("layoffPct").Number().IsEqual(0)
}

// Saying yes re-prescribes the day, which is what lets the UI show the lifter
// the numbers before they commit to them.
func TestLayoffDeloadCutsThePreviewedWeights(t *testing.T) {
	e := expect(t)
	programID, dayID := firstProgramAndDay(e)
	_, worked := trainedWeeksAgo(t, e, dayID, 3)

	body := preview(e, programID, dayID, true)
	body.Value("layoff").Object().Value("applied").Boolean().IsTrue()

	first := body.Value("exercises").Array().Value(0).Object()
	first.Value("weightLb").Number().IsEqual(progression.LayoffWeight(worked, 3))
	first.Value("weightLb").Number().Lt(worked)

	prog := first.Value("progression").Object()
	prog.Value("status").String().IsEqual("layoff")
	prog.Value("layoffPct").Number().IsEqual(0.3)
	// The cut explains itself against the weight it came off.
	prog.Value("previousWeightLb").Number().IsEqual(worked)
}

// A lift with no history is not one you have detrained on, so the cut does not
// reach it. Only the first exercise was logged above; the rest of the day has
// never been performed and must still open at its starting weight.
func TestLayoffLeavesALiftWithNoHistoryAlone(t *testing.T) {
	e := expect(t)
	programID, dayID := firstProgramAndDay(e)
	trainedWeeksAgo(t, e, dayID, 3)

	plain := preview(e, programID, dayID, false).Value("exercises").Array()
	cut := preview(e, programID, dayID, true).Value("exercises").Array()

	var checked int
	for i := 1; i < int(plain.Length().Raw()); i++ {
		ex := plain.Value(i).Object()
		if ex.Value("progression").Object().Value("status").String().Raw() != "start" {
			continue
		}
		checked++
		cutEx := cut.Value(i).Object()
		cutEx.Value("weightLb").Number().IsEqual(ex.Value("weightLb").Number().Raw())
		cutEx.Value("progression").Object().Value("status").String().IsEqual("start")
	}
	if checked == 0 {
		t.Fatal("no never-performed lift on the day; this test asserts nothing")
	}
}

// The answer has to survive into the session, or the prompt is decoration: the
// materialized sets are what actually goes on the bar.
func TestLayoffDeloadReachesTheCreatedSession(t *testing.T) {
	e := expect(t)
	_, dayID := firstProgramAndDay(e)
	_, worked := trainedWeeksAgo(t, e, dayID, 3)

	created := e.POST("/sessions").
		WithJSON(map[string]any{"programDayId": dayID, "deload": true}).
		Expect().Status(http.StatusCreated).JSON().Object()
	id := int(created.Value("id").Number().Raw())
	t.Cleanup(func() {
		e.DELETE(fmt.Sprintf("/sessions/%d", id)).Expect().Status(http.StatusNoContent)
	})
	created.Value("sets").Array().Value(0).Object().
		Value("weightLb").Number().IsEqual(progression.LayoffWeight(worked, 3))

	// Omitting the flag is the old behaviour exactly, which is what lets a
	// client that has never heard of this ship unchanged. Sound because neither
	// session above logged a rep, so neither counts as training and the layoff
	// is still three weeks.
	startSession(t, e, dayID).Value("sets").Array().Value(0).Object().
		Value("weightLb").Number().IsEqual(worked)
}

// Six days off is a rest, not a layoff. Nothing is reported, and asking for the
// deload anyway changes nothing — how long they have been away is the server's
// to measure, so the flag cannot be used to prescribe an arbitrary weight.
func TestNoLayoffWithinTheWeek(t *testing.T) {
	e := expect(t)
	programID, dayID := firstProgramAndDay(e)
	_, worked := trainedWeeksAgo(t, e, dayID, 0)

	body := preview(e, programID, dayID, true)
	body.Value("layoff").IsNull()
	body.Value("exercises").Array().Value(0).Object().
		Value("weightLb").Number().IsEqual(worked)
}

// The cut deepens with the time away and then stops. Driven through the API
// rather than the engine so that the week arithmetic — which is the API's, not
// the engine's — is what is under test.
func TestLayoffDeepensWithTimeAwayAndThenCaps(t *testing.T) {
	e := expect(t)
	programID, dayID := firstProgramAndDay(e)
	sessionID, worked := trainedWeeksAgo(t, e, dayID, 1)

	for _, tc := range []struct {
		weeks   int
		wantPct float64
	}{
		{weeks: 1, wantPct: 0.1},
		{weeks: 2, wantPct: 0.2},
		{weeks: 5, wantPct: 0.5},
		{weeks: 30, wantPct: 0.5},
	} {
		t.Run(fmt.Sprintf("%d weeks", tc.weeks), func(t *testing.T) {
			backdatePerformedOn(t, sessionID, time.Now().AddDate(0, 0, -daysAway(tc.weeks)))

			body := preview(e, programID, dayID, true)
			body.Value("layoff").Object().Value("weeks").Number().IsEqual(tc.weeks)
			body.Value("layoff").Object().Value("deloadPct").Number().IsEqual(tc.wantPct)
			body.Value("exercises").Array().Value(0).Object().
				Value("weightLb").Number().
				IsEqual(progression.LayoffWeight(worked, tc.weeks))
		})
	}
}
