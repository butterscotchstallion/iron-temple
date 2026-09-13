package api_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/gavv/httpexpect/v2"
)

// recap reads a session's recap.
func recap(t *testing.T, e *httpexpect.Expect, sessionID int) *httpexpect.Object {
	t.Helper()
	return e.GET(fmt.Sprintf("/sessions/%d/recap", sessionID)).
		Expect().Status(http.StatusOK).
		JSON().Object()
}

// logEverySet logs every set of a session at its target reps and returns the
// weight each lift was worked at, by exercise id.
func logEverySet(t *testing.T, e *httpexpect.Expect, session *httpexpect.Object) map[int]float64 {
	t.Helper()
	id := int(session.Value("id").Number().Raw())
	weights := map[int]float64{}
	for _, s := range session.Value("sets").Array().Iter() {
		set := s.Object()
		weights[int(set.Value("exerciseId").Number().Raw())] = set.Value("weightLb").Number().Raw()
		logSet(e, id, int(set.Value("id").Number().Raw()),
			int(set.Value("targetReps").Number().Raw()), true)
	}
	return weights
}

// repeatEverySet logs a session at the weights given, overriding what the
// progression engine prescribed. Hitting every set advances the next
// prescription, so a test that needs two IDENTICAL sessions has to say so.
func repeatEverySet(
	t *testing.T, e *httpexpect.Expect, session *httpexpect.Object, weights map[int]float64,
) {
	t.Helper()
	id := int(session.Value("id").Number().Raw())
	for _, s := range session.Value("sets").Array().Iter() {
		set := s.Object()
		w, ok := weights[int(set.Value("exerciseId").Number().Raw())]
		if !ok {
			t.Fatalf("session %d carries a lift the earlier one did not", id)
		}
		e.PATCH(fmt.Sprintf("/sessions/%d/sets/%d", id, int(set.Value("id").Number().Raw()))).
			WithJSON(map[string]any{
				"actualReps": int(set.Value("targetReps").Number().Raw()),
				"weightLb":   w,
				"completed":  true,
			}).
			Expect().Status(http.StatusOK)
	}
}

// giveDuration backdates a finished session's start so that it has a length.
//
// Must be called after the finish, since it measures back from finished_at.
// Necessary because a test creates and finishes a session in the same instant,
// which the recap correctly reports as having no duration at all — anything
// under a second rounds to zero seconds on the wire, and a pace ranked against
// a median of zero is a division by noise. Without this, a test asserting on
// pace is asserting on nothing.
func giveDuration(t *testing.T, sessionID int, minutes int) {
	t.Helper()
	_, err := testPool.Exec(context.Background(),
		"UPDATE sessions SET created_at = finished_at - $1::interval WHERE id = $2",
		fmt.Sprintf("%d minutes", minutes), sessionID)
	if err != nil {
		t.Fatalf("give session %d a duration: %v", sessionID, err)
	}
}

// setPerformedOn moves a session's training date, which the API deliberately
// does not expose — the same reaching past it that backdateSession does. It is
// how a test gets two sessions onto one calendar day, or onto different ones,
// inside a single run.
func setPerformedOn(t *testing.T, sessionID int, date string) {
	t.Helper()
	_, err := testPool.Exec(context.Background(),
		"UPDATE sessions SET performed_on = $1::date WHERE id = $2", date, sessionID)
	if err != nil {
		t.Fatalf("set performed_on for session %d: %v", sessionID, err)
	}
}

// Two sessions on one calendar day is the case a plain `performed_on <` cut
// gets wrong: the morning's work vanishes from the evening's baseline, and the
// evening is handed the same personal record all over again.
//
// This is the test the recap queries' row-comparison cut exists for.
func TestRecapDoesNotRecreditRecordsWithinADay(t *testing.T) {
	e := expect(t)
	_, dayID := firstProgramAndDay(e)
	const on = "2026-04-06"

	morning := startSession(t, e, dayID)
	morningID := int(morning.Value("id").Number().Raw())
	setPerformedOn(t, morningID, on)
	weights := logEverySet(t, e, morning)
	e.POST(fmt.Sprintf("/sessions/%d/finish", morningID)).Expect().Status(http.StatusOK)

	// The morning legitimately sets records: it is this lifter's first work.
	if got := len(recap(t, e, morningID).Value("prs").Array().Iter()); got == 0 {
		t.Fatal("the first session of a lift set no records, so the rest of this proves nothing")
	}

	// The evening repeats the same day at the SAME weights, on the same date.
	// Same weights matters: hitting every set advanced the prescription, so a
	// second session left alone would be genuinely heavier and its records
	// genuinely new — which would prove nothing about the date cut.
	evening := startSession(t, e, dayID)
	eveningID := int(evening.Value("id").Number().Raw())
	setPerformedOn(t, eveningID, on)
	repeatEverySet(t, e, evening, weights)
	e.POST(fmt.Sprintf("/sessions/%d/finish", eveningID)).Expect().Status(http.StatusOK)

	r := recap(t, e, eveningID)
	if got := len(r.Value("prs").Array().Iter()); got != 0 {
		t.Errorf("evening session claimed %d records for work already done that morning", got)
	}
	if got := len(r.Value("milestones").Array().Iter()); got != 0 {
		t.Errorf("evening session claimed %d milestones already crossed that morning", got)
	}
	// It should also recognise the morning as the last time this day was done,
	// rather than looking past it to some earlier date.
	prev := r.Value("progress").Object().Value("previousSessionId")
	prev.NotNull()
	if got := int(prev.Number().Raw()); got != morningID {
		t.Errorf("previousSessionId = %d, want the morning session %d", got, morningID)
	}
}

// A lifter's first-ever session has nothing behind it. Every comparison has to
// come back null rather than a zero to divide by, and the recap still has to
// render.
func TestRecapFirstSessionOfADay(t *testing.T) {
	e := expect(t)
	_, dayID := firstProgramAndDay(e)

	// A day nobody has performed: use a later day of the program so the seeded
	// history from other tests cannot reach it.
	session := startSession(t, e, dayID)
	id := int(session.Value("id").Number().Raw())
	setPerformedOn(t, id, "2026-05-04")
	logEverySet(t, e, session)
	e.POST(fmt.Sprintf("/sessions/%d/finish", id)).Expect().Status(http.StatusOK)
	giveDuration(t, id, 48)

	r := recap(t, e, id)
	r.Value("session").Object().Value("sessionId").Number().IsEqual(id)
	r.Value("durationSeconds").Number().IsEqual(48 * 60)

	// Lifts, PRs and milestones are always arrays — never null — so a client
	// can range over them without branching.
	r.Value("lifts").Array().NotEmpty()
	r.Value("prs").Array()
	r.Value("milestones").Array()

	vol := r.Value("volume").Object()
	vol.Value("totalLb").Number().Gt(0)
	if vol.Value("setsLogged").Number().Raw() != vol.Value("setsPrescribed").Number().Raw() {
		t.Error("every set was logged, so the two counts should agree")
	}
}

// An unfinished session has no length worth quoting — but it is still a session
// somebody trained, and refusing it would refuse the lifter whose queued Finish
// has not landed yet.
func TestRecapServesAnUnfinishedSession(t *testing.T) {
	e := expect(t)
	_, dayID := firstProgramAndDay(e)

	session := startSession(t, e, dayID)
	id := int(session.Value("id").Number().Raw())
	logEverySet(t, e, session) // logged, but never finished

	r := recap(t, e, id)
	r.Value("durationSeconds").IsNull()
	r.Value("pace").IsNull()
	// The work still counts.
	r.Value("volume").Object().Value("totalLb").Number().Gt(0)
	r.Value("lifts").Array().NotEmpty()
}

// A session aged past the twelve-hour cutoff is over, but its elapsed time
// measures a tab left open rather than a workout.
func TestRecapDropsDurationPastTheCutoff(t *testing.T) {
	e := expect(t)
	_, dayID := firstProgramAndDay(e)

	session := startSession(t, e, dayID)
	id := int(session.Value("id").Number().Raw())
	logEverySet(t, e, session)
	e.POST(fmt.Sprintf("/sessions/%d/finish", id)).Expect().Status(http.StatusOK)
	backdateSession(t, id, 13*60*60*1e9) // 13 hours

	r := recap(t, e, id)
	r.Value("durationSeconds").IsNull()
	r.Value("pace").IsNull()
	r.Value("volume").Object().Value("totalLb").Number().Gt(0)
}

// Unlogged sets are the denominator of "10 of 25 reps": they count toward the
// prescription and toward nothing else.
func TestRecapCountsPrescribedSetsSeparatelyFromLogged(t *testing.T) {
	e := expect(t)
	_, dayID := firstProgramAndDay(e)

	session := startSession(t, e, dayID)
	id := int(session.Value("id").Number().Raw())
	sets := session.Value("sets").Array()
	total := len(sets.Iter())

	// Log exactly one set and walk away.
	first := sets.Value(0).Object()
	logSet(e, id, int(first.Value("id").Number().Raw()),
		int(first.Value("targetReps").Number().Raw()), true)
	e.POST(fmt.Sprintf("/sessions/%d/finish", id)).Expect().Status(http.StatusOK)

	vol := recap(t, e, id).Value("volume").Object()
	vol.Value("setsLogged").Number().IsEqual(1)
	vol.Value("setsPrescribed").Number().IsEqual(total)
	if vol.Value("repsLogged").Number().Raw() >= vol.Value("repsTargeted").Number().Raw() {
		t.Error("one set of many logged, so repsLogged must fall short of repsTargeted")
	}
}

// The whole point of the feature: how the weights sit against the last time
// this program day came round.
func TestRecapComparesAgainstThePreviousSessionOfTheDay(t *testing.T) {
	e := expect(t)
	_, dayID := firstProgramAndDay(e)

	first := startSession(t, e, dayID)
	firstID := int(first.Value("id").Number().Raw())
	setPerformedOn(t, firstID, "2026-06-01")
	logEverySet(t, e, first)
	e.POST(fmt.Sprintf("/sessions/%d/finish", firstID)).Expect().Status(http.StatusOK)
	giveDuration(t, firstID, 60)

	// A second session of the same day. Having hit every set, the engine
	// prescribes more weight, so this one is genuinely heavier.
	second := startSession(t, e, dayID)
	secondID := int(second.Value("id").Number().Raw())
	setPerformedOn(t, secondID, "2026-06-04")
	logEverySet(t, e, second)
	e.POST(fmt.Sprintf("/sessions/%d/finish", secondID)).Expect().Status(http.StatusOK)
	giveDuration(t, secondID, 50)

	r := recap(t, e, secondID)
	prog := r.Value("progress").Object()
	prog.Value("previousSessionId").Number().IsEqual(firstID)
	prog.Value("previousPerformedOn").String().IsEqual("2026-06-01")
	prog.Value("liftsCompared").Number().Gt(0)
	// Linear progression added weight to every lift, so the paired rollup is up.
	prog.Value("weightDeltaPct").Number().Gt(0)

	// Volume is measured against the same session.
	r.Value("volume").Object().Value("previousLb").Number().Gt(0)
	r.Value("volume").Object().Value("deltaPct").Number().Gt(0)

	// Every lift carries its own before-and-after.
	lift := r.Value("lifts").Array().Value(0).Object()
	lift.Value("previous").Object().Value("topWeightLb").Number().Gt(0)
	lift.Value("weightDeltaLb").Number().Gt(0)

	// And the day now has a length to rank the second session against: 50
	// minutes measured against a history of one 60-minute session.
	r.Value("durationSeconds").Number().IsEqual(50 * 60)
	pace := r.Value("pace").Object()
	pace.Value("sampleSize").Number().IsEqual(1)
	pace.Value("of").Number().IsEqual(2)
	pace.Value("rank").Number().IsEqual(1)
	pace.Value("medianSeconds").Number().IsEqual(60 * 60)
	// Negative is faster. The sign is the trap in this field.
	pace.Value("deltaPct").Number().InRange(-0.17, -0.16)
}

// A session belonging to somebody else does not resolve, and neither does one
// that never existed. Both are 404 — the isolation model, not a 403 that would
// confirm the row is there.
func TestRecapIsScopedToTheOwner(t *testing.T) {
	e := expect(t)
	_, dayID := firstProgramAndDay(e)
	session := startSession(t, e, dayID)
	id := int(session.Value("id").Number().Raw())

	other := expectAs(t, secondUserToken(t))
	other.GET(fmt.Sprintf("/sessions/%d/recap", id)).
		Expect().Status(http.StatusNotFound)

	e.GET("/sessions/999999/recap").Expect().Status(http.StatusNotFound)
}

// Every field the schema documents, spelled the way it documents it.
//
// The DTO's json tags are hand-written while the UI's types are generated from
// openapi.yaml, so the two can disagree without anything failing to compile:
// the field simply arrives undefined and the page renders a blank where a
// number should be. The other tests here read the fields they assert on, which
// covers most of this — but not the ones nothing happens to assert, and those
// are exactly the ones a typo survives in.
func TestRecapShapeMatchesTheSchema(t *testing.T) {
	e := expect(t)
	_, dayID := firstProgramAndDay(e)

	first := startSession(t, e, dayID)
	firstID := int(first.Value("id").Number().Raw())
	setPerformedOn(t, firstID, "2026-07-06")
	logEverySet(t, e, first)
	e.POST(fmt.Sprintf("/sessions/%d/finish", firstID)).Expect().Status(http.StatusOK)
	giveDuration(t, firstID, 55)

	second := startSession(t, e, dayID)
	secondID := int(second.Value("id").Number().Raw())
	setPerformedOn(t, secondID, "2026-07-09")
	logEverySet(t, e, second)
	e.POST(fmt.Sprintf("/sessions/%d/finish", secondID)).Expect().Status(http.StatusOK)
	giveDuration(t, secondID, 52)

	r := recap(t, e, secondID)
	r.Keys().ContainsOnly(
		"session", "durationSeconds", "pace", "volume", "progress",
		"lifts", "prs", "milestones", "streak",
	)
	r.Value("session").Object().Keys().ContainsOnly(
		"sessionId", "programId", "programName", "programDayId", "programDayName",
		"performedOn", "startedAt", "finishedAt", "isOver",
	)
	r.Value("pace").Object().Keys().ContainsOnly(
		"medianSeconds", "deltaPct", "rank", "of", "sampleSize",
	)
	r.Value("volume").Object().Keys().ContainsOnly(
		"totalLb", "previousLb", "deltaPct", "comparison",
		"setsLogged", "setsPrescribed", "repsLogged", "repsTargeted",
	)
	r.Value("volume").Object().Value("comparison").Object().
		Keys().ContainsOnly("count", "label", "unitLb")
	r.Value("progress").Object().Keys().ContainsOnly(
		"previousSessionId", "previousPerformedOn", "weightDeltaPct",
		"liftsCompared", "liftsNew",
	)
	r.Value("streak").Object().Keys().ContainsOnly("sessions", "weeks")

	lift := r.Value("lifts").Array().Value(0).Object()
	lift.Keys().ContainsOnly(
		"exerciseId", "exerciseName", "kind", "topWeightLb", "topReps", "topE1rmLb",
		"setsLogged", "setsPrescribed", "repsLogged", "repsTargeted", "volumeLb",
		"hitEveryTarget", "previous", "weightDeltaLb", "weightDeltaPct", "e1rmDeltaPct",
	)
	lift.Value("previous").Object().
		Keys().ContainsOnly("performedOn", "topWeightLb", "topReps", "topE1rmLb")
	// The enum, not a boolean — this is a session surface, and SessionSet
	// already spells the same distinction this way.
	lift.Value("kind").String().IsEqual("main")
	lift.Value("topE1rmLb").Number().Gt(0)
	lift.Value("hitEveryTarget").Boolean().IsTrue()

	// startedAt is the session's creation, RFC3339 — there is no started_at
	// column, and this is what stands in for one.
	r.Value("session").Object().Value("startedAt").String().NotEmpty()
	r.Value("streak").Object().Value("sessions").Number().Gt(0)
}

// A session opened and abandoned still produces a whole recap rather than an
// error — the arrays are empty, not missing.
func TestRecapOfASessionWithNothingLogged(t *testing.T) {
	e := expect(t)
	_, dayID := firstProgramAndDay(e)
	session := startSession(t, e, dayID)
	id := int(session.Value("id").Number().Raw())
	total := len(session.Value("sets").Array().Iter())

	r := recap(t, e, id)
	r.Value("lifts").Array().IsEmpty()
	r.Value("prs").Array().IsEmpty()
	r.Value("milestones").Array().IsEmpty()

	vol := r.Value("volume").Object()
	vol.Value("totalLb").Number().IsEqual(0)
	vol.Value("setsLogged").Number().IsEqual(0)
	vol.Value("setsPrescribed").Number().IsEqual(total)
	// Nothing moved is nothing to compare to an object, which reads as no
	// comparison rather than "0 plates".
	vol.Value("comparison").Object().Value("count").Number().IsEqual(0)
}
