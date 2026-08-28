package api_test

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/gavv/httpexpect/v2"
)

// A session used to materialize a fixed number of sets and that was the whole
// of it: no way to log the extra set, the AMRAP or the drop set, and no way to
// drop one that was skipped. The closest a lifter could get was a ghost row at
// zero reps, which is not the same claim.

func TestAddSetAppendsToTheLift(t *testing.T) {
	e := expect(t)
	_, dayID := firstProgramAndDay(e)

	created := startSession(t, e, dayID)
	sessionID := int(created.Value("id").Number().Raw())
	first := created.Value("sets").Array().Value(0).Object()
	exerciseID := int(first.Value("exerciseId").Number().Raw())

	before := countSetsFor(e, sessionID, exerciseID)
	lastNumber := lastSetNumberFor(e, sessionID, exerciseID)

	added := e.POST(fmt.Sprintf("/sessions/%d/sets", sessionID)).
		WithJSON(map[string]any{"exerciseId": exerciseID}).
		Expect().Status(http.StatusCreated).JSON().Object()

	// Numbered past the last one, and carrying that lift's rep target and
	// weight — an extra set is another set of the same thing.
	added.Value("setNumber").Number().IsEqual(lastNumber + 1)
	added.Value("targetReps").Number().IsEqual(first.Value("targetReps").Number().Raw())
	added.Value("weightLb").Number().IsEqual(first.Value("weightLb").Number().Raw())
	added.Value("actualReps").IsNull()
	added.Value("completed").Boolean().IsFalse()
	added.Value("exerciseName").String().NotEmpty()

	if after := countSetsFor(e, sessionID, exerciseID); after != before+1 {
		t.Fatalf("expected %d sets for the lift, got %d", before+1, after)
	}
}

func TestRemoveSetLeavesAGapRatherThanRenumbering(t *testing.T) {
	e := expect(t)
	_, dayID := firstProgramAndDay(e)

	created := startSession(t, e, dayID)
	sessionID := int(created.Value("id").Number().Raw())
	sets := created.Value("sets").Array()
	first := sets.Value(0).Object()
	exerciseID := int(first.Value("exerciseId").Number().Raw())

	// Drop the lift's second set, so the surviving numbers are 1, 3, 4...
	second := setNumbered(e, sessionID, exerciseID, 2)
	e.DELETE(fmt.Sprintf("/sessions/%d/sets/%d", sessionID, second)).
		Expect().Status(http.StatusNoContent)

	numbers := setNumbersFor(e, sessionID, exerciseID)
	for _, n := range numbers {
		if n == 2 {
			t.Fatal("the deleted set is still in the session")
		}
	}
	// Set numbers order the sets, they do not name them: nothing shuffled down
	// to close the gap.
	if len(numbers) > 1 && numbers[1] != 3 {
		t.Fatalf("sets were renumbered after a delete: %v", numbers)
	}

	// And an append lands past the gap rather than filling it.
	added := e.POST(fmt.Sprintf("/sessions/%d/sets", sessionID)).
		WithJSON(map[string]any{"exerciseId": exerciseID}).
		Expect().Status(http.StatusCreated).JSON().Object()
	added.Value("setNumber").Number().Gt(float64(numbers[len(numbers)-1]))
}

// Removing a lift's last set takes the lift out of the session. That is what
// skipping it looks like, and it has to read correctly downstream rather than
// leaving something half-present.
func TestRemovingEverySetTakesTheLiftOutOfTheSession(t *testing.T) {
	e := expect(t)
	_, dayID := firstProgramAndDay(e)

	created := startSession(t, e, dayID)
	sessionID := int(created.Value("id").Number().Raw())
	exerciseID := int(created.Value("sets").Array().Value(0).Object().
		Value("exerciseId").Number().Raw())

	for _, id := range setIDsFor(e, sessionID, exerciseID) {
		e.DELETE(fmt.Sprintf("/sessions/%d/sets/%d", sessionID, id)).
			Expect().Status(http.StatusNoContent)
	}

	if n := countSetsFor(e, sessionID, exerciseID); n != 0 {
		t.Fatalf("expected the lift to be gone, still has %d sets", n)
	}
	// The session is still readable and still has its other lifts.
	e.GET(fmt.Sprintf("/sessions/%d", sessionID)).
		Expect().Status(http.StatusOK).
		JSON().Object().Value("sets").Array().NotEmpty()
}

// A finished session is a record. Changing its shape is refused, because a lift
// succeeds only when every one of its sets was completed — so appending an
// unlogged set to a closed session would retroactively turn a success into a
// failure and move the next session's weight.
func TestSetsAreLockedOnceTheSessionIsOver(t *testing.T) {
	e := expect(t)
	_, dayID := firstProgramAndDay(e)

	created := startSession(t, e, dayID)
	sessionID := int(created.Value("id").Number().Raw())
	sets := created.Value("sets").Array()
	exerciseID := int(sets.Value(0).Object().Value("exerciseId").Number().Raw())
	setID := int(sets.Value(0).Object().Value("id").Number().Raw())

	e.POST(fmt.Sprintf("/sessions/%d/finish", sessionID)).Expect().Status(http.StatusOK)

	e.POST(fmt.Sprintf("/sessions/%d/sets", sessionID)).
		WithJSON(map[string]any{"exerciseId": exerciseID}).
		Expect().Status(http.StatusConflict)

	e.DELETE(fmt.Sprintf("/sessions/%d/sets/%d", sessionID, setID)).
		Expect().Status(http.StatusConflict)
}

// Ageing out is the other way a session becomes over, and it locks the same way.
func TestSetsAreLockedOnceTheSessionAgesOut(t *testing.T) {
	e := expect(t)
	_, dayID := firstProgramAndDay(e)

	created := startSession(t, e, dayID)
	sessionID := int(created.Value("id").Number().Raw())
	exerciseID := int(created.Value("sets").Array().Value(0).Object().
		Value("exerciseId").Number().Raw())

	backdateSession(t, sessionID, 13*time.Hour)

	e.POST(fmt.Sprintf("/sessions/%d/sets", sessionID)).
		WithJSON(map[string]any{"exerciseId": exerciseID}).
		Expect().Status(http.StatusConflict)
}

func TestAddSetValidationAndUnknownTargets(t *testing.T) {
	e := expect(t)
	_, dayID := firstProgramAndDay(e)

	created := startSession(t, e, dayID)
	sessionID := int(created.Value("id").Number().Raw())

	// This adds a set, not an exercise: a lift the session does not have is a
	// 404 rather than a silent insert.
	e.POST(fmt.Sprintf("/sessions/%d/sets", sessionID)).
		WithJSON(map[string]any{"exerciseId": 999999}).
		Expect().Status(http.StatusNotFound)

	e.POST(fmt.Sprintf("/sessions/%d/sets", sessionID)).
		WithJSON(map[string]any{}).
		Expect().Status(http.StatusBadRequest)

	e.POST("/sessions/999999/sets").
		WithJSON(map[string]any{"exerciseId": 1}).
		Expect().Status(http.StatusNotFound)

	e.DELETE(fmt.Sprintf("/sessions/%d/sets/999999", sessionID)).
		Expect().Status(http.StatusNotFound)
}

// A set id is not a capability. Deleting one through a session that does not
// own it must fail, or the path segment is decoration.
func TestASetCannotBeRemovedThroughAnotherSession(t *testing.T) {
	e := expect(t)
	_, dayID := firstProgramAndDay(e)

	one := startSession(t, e, dayID)
	setID := int(one.Value("sets").Array().Value(0).Object().Value("id").Number().Raw())

	two := startSession(t, e, dayID)
	otherSessionID := int(two.Value("id").Number().Raw())

	e.DELETE(fmt.Sprintf("/sessions/%d/sets/%d", otherSessionID, setID)).
		Expect().Status(http.StatusNotFound)
}

// ---- helpers ----

func liftSets(e *httpexpect.Expect, sessionID, exerciseID int) []*httpexpect.Object {
	all := e.GET(fmt.Sprintf("/sessions/%d", sessionID)).
		Expect().Status(http.StatusOK).
		JSON().Object().Value("sets").Array()

	var out []*httpexpect.Object
	for i := 0; i < int(all.Length().Raw()); i++ {
		set := all.Value(i).Object()
		if int(set.Value("exerciseId").Number().Raw()) == exerciseID {
			out = append(out, set)
		}
	}
	return out
}

func countSetsFor(e *httpexpect.Expect, sessionID, exerciseID int) int {
	return len(liftSets(e, sessionID, exerciseID))
}

func setNumbersFor(e *httpexpect.Expect, sessionID, exerciseID int) []int {
	var out []int
	for _, s := range liftSets(e, sessionID, exerciseID) {
		out = append(out, int(s.Value("setNumber").Number().Raw()))
	}
	return out
}

func setIDsFor(e *httpexpect.Expect, sessionID, exerciseID int) []int {
	var out []int
	for _, s := range liftSets(e, sessionID, exerciseID) {
		out = append(out, int(s.Value("id").Number().Raw()))
	}
	return out
}

func lastSetNumberFor(e *httpexpect.Expect, sessionID, exerciseID int) float64 {
	numbers := setNumbersFor(e, sessionID, exerciseID)
	return float64(numbers[len(numbers)-1])
}

// setNumbered returns the id of a lift's nth set within a session.
func setNumbered(e *httpexpect.Expect, sessionID, exerciseID, setNumber int) int {
	for _, s := range liftSets(e, sessionID, exerciseID) {
		if int(s.Value("setNumber").Number().Raw()) == setNumber {
			return int(s.Value("id").Number().Raw())
		}
	}
	return 0
}
