package api_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/gavv/httpexpect/v2"
)

// forgetAssistance drops the overlay row a test created once it is done.
//
// Needed because the endpoint under test deliberately edits the program day, and
// the suite shares one lifter and one database: without this, the first test to
// add curls leaves every later one a day that already has them, and the second
// add is a perfectly correct 409. Reaches past the API for the same reason
// backdateSession does — the delete exists, but it needs an assistance id this
// endpoint does not return.
func forgetAssistance(t *testing.T, exerciseID int) {
	t.Helper()
	t.Cleanup(func() {
		if _, err := testPool.Exec(context.Background(),
			"DELETE FROM program_day_assistance WHERE exercise_id = $1", exerciseID); err != nil {
			t.Fatalf("clear assistance for exercise %d: %v", exerciseID, err)
		}
	})
}

// accessoryID returns a library movement that no seeded program prescribes, so
// it can be added as assistance without tripping already_prescribed.
func accessoryID(t *testing.T, e *httpexpect.Expect) int {
	t.Helper()
	for _, ex := range e.GET("/exercises").Expect().Status(http.StatusOK).
		JSON().Array().Iter() {
		obj := ex.Object()
		if obj.Value("isAccessory").Boolean().Raw() {
			return int(obj.Value("id").Number().Raw())
		}
	}
	t.Fatal("no accessory movement in the seeded library")
	return 0
}

func addSessionAssistance(
	e *httpexpect.Expect, sessionID int, body map[string]any,
) *httpexpect.Response {
	return e.POST(fmt.Sprintf("/sessions/%d/assistance", sessionID)).
		WithJSON(body).Expect()
}

// The point of the feature: the lift lands in the workout the lifter is standing
// in, at the numbers they typed, marked as assistance and sorted after the
// program's own work.
func TestAddSessionAssistanceMaterialisesIntoTheLiveSession(t *testing.T) {
	e := expect(t)
	_, dayID := firstProgramAndDay(e)
	session := startSession(t, e, dayID)
	sessionID := int(session.Value("id").Number().Raw())
	mainSets := len(session.Value("sets").Array().Iter())

	exerciseID := accessoryID(t, e)
	forgetAssistance(t, exerciseID)
	created := addSessionAssistance(e, sessionID, map[string]any{
		"exerciseId": exerciseID, "sets": 3, "reps": 10, "weightLb": 30,
	}).Status(http.StatusCreated).JSON().Array()

	created.Length().IsEqual(3)
	for i, s := range created.Iter() {
		set := s.Object()
		set.Value("exerciseId").Number().IsEqual(exerciseID)
		// In set order, which the response promises — a client that queued this
		// offline matches these against the rows it already drew.
		set.Value("setNumber").Number().IsEqual(i + 1)
		set.Value("targetReps").Number().IsEqual(10)
		set.Value("weightLb").Number().IsEqual(30)
		set.Value("actualReps").IsNull()
		set.Value("completed").Boolean().IsFalse()
		// Derived from the absence of a program_day_exercises row, so it needs
		// nothing from the insert.
		set.Value("kind").String().IsEqual("assistance")
		set.Value("restSeconds").Number().InRange(30, 900)
	}

	// And the session now reads back with them last — assistance is what you do
	// after the barbell work, not instead of it.
	sets := e.GET(fmt.Sprintf("/sessions/%d", sessionID)).
		Expect().Status(http.StatusOK).JSON().Object().Value("sets").Array()
	sets.Length().IsEqual(mainSets + 3)
	for i := 0; i < 3; i++ {
		sets.Value(mainSets + i).Object().
			Value("exerciseId").Number().IsEqual(exerciseID)
	}
}

// The other half, and the reason this writes to the program day at all: the lift
// is prescribed the next time that day comes round, with a progression behind it
// rather than being a one-off with no future.
func TestAddSessionAssistancePrescribesItNextTime(t *testing.T) {
	e := expect(t)
	programID, dayID := firstProgramAndDay(e)
	session := startSession(t, e, dayID)
	sessionID := int(session.Value("id").Number().Raw())

	exerciseID := accessoryID(t, e)
	forgetAssistance(t, exerciseID)
	addSessionAssistance(e, sessionID, map[string]any{
		"exerciseId": exerciseID, "sets": 3, "reps": 12, "weightLb": 25,
	}).Status(http.StatusCreated)

	var found bool
	for _, ex := range e.GET(fmt.Sprintf("/programs/%d/days/%d/next-session", programID, dayID)).
		Expect().Status(http.StatusOK).
		JSON().Object().Value("exercises").Array().Iter() {
		obj := ex.Object()
		if int(obj.Value("exerciseId").Number().Raw()) == exerciseID {
			found = true
			obj.Value("kind").String().IsEqual("assistance")
		}
	}
	if !found {
		t.Error("the lift was added to the session but not to the day it came from")
	}
}

// Everything that can refuse it. Each is a 409 the client should have prevented
// by filtering its picker — these are backstops against a race, not a path.
func TestAddSessionAssistanceConflicts(t *testing.T) {
	e := expect(t)
	_, dayID := firstProgramAndDay(e)
	session := startSession(t, e, dayID)
	sessionID := int(session.Value("id").Number().Raw())
	exerciseID := accessoryID(t, e)
	forgetAssistance(t, exerciseID)

	t.Run("a lift the program already prescribes", func(t *testing.T) {
		prescribed := int(session.Value("sets").Array().Value(0).Object().
			Value("exerciseId").Number().Raw())
		addSessionAssistance(e, sessionID, map[string]any{
			"exerciseId": prescribed, "sets": 3, "reps": 10,
		}).Status(http.StatusConflict).
			JSON().Object().Value("code").String().IsEqual("already_prescribed")
	})

	t.Run("a lift already in this session", func(t *testing.T) {
		addSessionAssistance(e, sessionID, map[string]any{
			"exerciseId": exerciseID, "sets": 3, "reps": 10,
		}).Status(http.StatusCreated)

		// Second time: it is now both on the day and in the session. Either
		// guard is a correct refusal; what must not happen is a second
		// set-number-1 colliding on session_sets' UNIQUE.
		addSessionAssistance(e, sessionID, map[string]any{
			"exerciseId": exerciseID, "sets": 3, "reps": 10,
		}).Status(http.StatusConflict)
	})
}

// Same rule as adding a set, and it bites harder: what is refused is not one
// appended set but a whole exercise.
func TestAddSessionAssistanceRefusedOnAClosedSession(t *testing.T) {
	e := expect(t)
	_, dayID := firstProgramAndDay(e)
	session := startSession(t, e, dayID)
	sessionID := int(session.Value("id").Number().Raw())
	logEverySet(t, e, session)
	e.POST(fmt.Sprintf("/sessions/%d/finish", sessionID)).Expect().Status(http.StatusOK)

	addSessionAssistance(e, sessionID, map[string]any{
		"exerciseId": accessoryID(t, e), "sets": 3, "reps": 10,
	}).Status(http.StatusConflict).
		JSON().Object().Value("code").String().IsEqual("session_over")
}

func TestAddSessionAssistanceValidation(t *testing.T) {
	e := expect(t)
	_, dayID := firstProgramAndDay(e)
	session := startSession(t, e, dayID)
	sessionID := int(session.Value("id").Number().Raw())
	exerciseID := accessoryID(t, e)

	for _, tc := range []struct {
		name string
		body map[string]any
	}{
		{"no exercise", map[string]any{"sets": 3, "reps": 10}},
		{"zero sets", map[string]any{"exerciseId": exerciseID, "sets": 0, "reps": 10}},
		{"too many sets", map[string]any{"exerciseId": exerciseID, "sets": 21, "reps": 10}},
		{"zero reps", map[string]any{"exerciseId": exerciseID, "sets": 3, "reps": 0}},
		{"too many reps", map[string]any{"exerciseId": exerciseID, "sets": 3, "reps": 101}},
		{"negative weight", map[string]any{
			"exerciseId": exerciseID, "sets": 3, "reps": 10, "weightLb": -5,
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			addSessionAssistance(e, sessionID, tc.body).Status(http.StatusBadRequest)
		})
	}

	t.Run("unknown exercise", func(t *testing.T) {
		addSessionAssistance(e, sessionID, map[string]any{
			"exerciseId": 999999, "sets": 3, "reps": 10,
		}).Status(http.StatusNotFound)
	})
}

// Omitted weight is bodyweight — dips, chin-ups, planks — rather than a missing
// field.
func TestAddSessionAssistanceDefaultsToBodyweight(t *testing.T) {
	e := expect(t)
	_, dayID := firstProgramAndDay(e)
	session := startSession(t, e, dayID)
	sessionID := int(session.Value("id").Number().Raw())

	exerciseID := accessoryID(t, e)
	forgetAssistance(t, exerciseID)
	created := addSessionAssistance(e, sessionID, map[string]any{
		"exerciseId": exerciseID, "sets": 2, "reps": 15,
	}).Status(http.StatusCreated).JSON().Array()

	created.Length().IsEqual(2)
	created.Value(0).Object().Value("weightLb").Number().IsEqual(0)
}

// Another lifter's session does not resolve — the same isolation model the rest
// of the session endpoints use, a 404 rather than a 403.
func TestAddSessionAssistanceIsScopedToTheOwner(t *testing.T) {
	e := expect(t)
	_, dayID := firstProgramAndDay(e)
	session := startSession(t, e, dayID)
	sessionID := int(session.Value("id").Number().Raw())

	other := expectAs(t, secondUserToken(t))
	other.POST(fmt.Sprintf("/sessions/%d/assistance", sessionID)).
		WithJSON(map[string]any{"exerciseId": accessoryID(t, e), "sets": 3, "reps": 10}).
		Expect().Status(http.StatusNotFound)

	e.POST("/sessions/999999/assistance").
		WithJSON(map[string]any{"exerciseId": accessoryID(t, e), "sets": 3, "reps": 10}).
		Expect().Status(http.StatusNotFound)
}
