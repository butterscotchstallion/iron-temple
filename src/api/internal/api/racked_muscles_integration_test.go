package api_test

import (
	"net/http"
	"testing"

	"github.com/gavv/httpexpect/v2"
)

// Racked's muscle-group slice, checked where it crosses the wire.
//
// internal/racked has the arithmetic under test already; what these cover is the
// half that only exists end to end — that muscle_group survives the join in
// RackedPeriodSets and lands on the movement the lifter actually performed. The
// engine cannot catch a query that selects the wrong column, because the engine
// is handed the answer.

// muscleSlice pulls one group out of a report's muscles array.
func muscleSlice(t *testing.T, rep *httpexpect.Object, group string) *httpexpect.Object {
	t.Helper()
	list := rep.Value("muscles").Array()
	for i := 0; i < int(list.Length().Raw()); i++ {
		m := list.Value(i).Object()
		if m.Value("group").String().Raw() == group {
			return m
		}
	}
	t.Fatalf("report has no %q slice", group)
	return nil
}

// An empty period has no muscles to divide, and specifically does not report
// seven groups the lifter failed to train in a month they never trained at all.
//
// Null rather than [], which is the same convention lifts and series already
// follow — a nil slice marshals to null, and every reader of this report guards
// with `?? []`. Pinned here because it is the shape the page branches on to drop
// the section entirely.
func TestRackedMusclesAbsentForAnEmptyPeriod(t *testing.T) {
	expect(t).GET("/racked").WithQuery("on", "1970-01-15").
		Expect().Status(http.StatusOK).JSON().Object().
		Value("muscles").IsNull()
}

// The seeded day is squat-led, so a session logged against it must land on legs
// — and must leave the groups it never touched present and empty.
func TestRackedMusclesCreditTheLiftsOwnGroup(t *testing.T) {
	e := expect(t)
	_, dayID := firstProgramAndDay(e)

	session := startSession(t, e, dayID)
	sessionID := int(session.Value("id").Number().Raw())
	// Log every set of the day, so the report covers whatever the seed
	// prescribes rather than depending on which lift comes first.
	sets := session.Value("sets").Array()
	for i := 0; i < int(sets.Length().Raw()); i++ {
		set := sets.Value(i).Object()
		logSet(e, sessionID, int(set.Value("id").Number().Raw()), 5, true)
	}

	rep := e.GET("/racked").Expect().Status(http.StatusOK).JSON().Object()

	// Workout A is squat/bench/row: legs, chest and back, and nothing below the
	// waist of the taxonomy.
	legs := muscleSlice(t, rep, "legs")
	legs.Value("trained").Boolean().IsTrue()
	legs.Value("volumeLb").Number().Gt(0)
	legs.Value("lifts").Number().Ge(1)

	core := muscleSlice(t, rep, "core")
	core.Value("trained").Boolean().IsFalse()
	core.Value("volumeLb").Number().IsEqual(0)

	// Every group is present. The whole point of the section is the rows with
	// nothing in them, so a report that only carried what was trained would be
	// the bug rather than an optimisation.
	rep.Value("muscles").Array().Length().Ge(7)

	// And the slices divide the headline rather than sampling it.
	var volume float64
	list := rep.Value("muscles").Array()
	for i := 0; i < int(list.Length().Raw()); i++ {
		volume += list.Value(i).Object().Value("volumeLb").Number().Raw()
	}
	total := rep.Value("totals").Object().Value("volumeLb").Number().Raw()
	if diff := volume - total; diff > 0.001 || diff < -0.001 {
		t.Errorf("muscle volume sums to %v, want the headline %v", volume, total)
	}
}

// Assistance is how most lifters train their arms at all, so it has to count
// here — a slice that read only the program's prescription would report that
// nobody has any.
func TestRackedMusclesCountAssistanceWork(t *testing.T) {
	e := expect(t)
	programID, dayID := firstProgramAndDay(e)
	curlID := exerciseIDByName(t, e, "Barbell Curl")
	addAssistance(t, e, programID, dayID, curlID, 3, 10, 40)

	session := startSession(t, e, dayID)
	sessionID := int(session.Value("id").Number().Raw())
	for _, set := range sessionSetsFor(session, curlID) {
		logSet(e, sessionID, int(set.Value("id").Number().Raw()), 10, true)
	}

	rep := e.GET("/racked").Expect().Status(http.StatusOK).JSON().Object()

	arms := muscleSlice(t, rep, "arms")
	arms.Value("trained").Boolean().IsTrue()
	arms.Value("volumeLb").Number().Gt(0)
}
