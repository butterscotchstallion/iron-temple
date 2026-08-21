package api_test

import (
	"fmt"
	"net/http"
	"testing"
)

// Prescribed rest.
//
// restSeconds was in the contract long before there was a column behind it —
// dto.go answered a flat 180 everywhere, so a set of deadlifts and a set of
// curls both asked for three minutes. Migration 0011 put the number on the
// exercise; these tests pin that it reaches all three surfaces that advertise
// it, because each one assembles its response from a different query and
// nothing but a test stops one of them regressing to a constant.

// The day the seeded programs open with is squat-led, which is the lift the
// five-minute tier exists for.
func TestProgramDayCarriesEachLiftsRest(t *testing.T) {
	e := expect(t)
	programID, _ := firstProgramAndDay(e)

	day := e.GET(fmt.Sprintf("/programs/%d", programID)).Expect().
		Status(http.StatusOK).JSON().Object().
		Value("days").Array().Value(0).Object()

	var seen []float64
	exercises := day.Value("exercises").Array()
	for i := 0; i < int(exercises.Length().Raw()); i++ {
		ex := exercises.Value(i).Object()
		rest := ex.Value("restSeconds").Number().Raw()
		seen = append(seen, rest)
		if ex.Value("exerciseName").String().Raw() == "Squat" {
			ex.Value("restSeconds").Number().IsEqual(300)
		}
	}
	// Not merely "the squat is 300": a day whose lifts all report the same
	// number is the bug this replaced, and it would survive the check above.
	if len(seen) < 2 {
		t.Fatalf("day has %d exercises; expected a multi-lift day", len(seen))
	}
	if allEqual(seen) {
		t.Errorf("every lift on the day rests %v — the fixed default is back", seen[0])
	}
}

// The preview is assembled by a different query than the program detail, and
// covers assistance too — which is the case that has no prescription row of its
// own to hang a rest off.
func TestNextSessionRestCoversMainAndAssistance(t *testing.T) {
	e := expect(t)
	programID, dayID := firstProgramAndDay(e)
	curlID := exerciseIDByName(t, e, "Barbell Curl")
	addAssistance(t, e, programID, dayID, curlID, 3, 10, 30)

	exercises := e.GET(fmt.Sprintf("/programs/%d/days/%d/next-session", programID, dayID)).
		Expect().Status(http.StatusOK).JSON().Object().
		Value("exercises").Array()

	var sawSquat, sawCurl bool
	for i := 0; i < int(exercises.Length().Raw()); i++ {
		ex := exercises.Value(i).Object()
		switch ex.Value("exerciseName").String().Raw() {
		case "Squat":
			sawSquat = true
			ex.Value("kind").String().IsEqual("main")
			ex.Value("restSeconds").Number().IsEqual(300)
		case "Barbell Curl":
			sawCurl = true
			// Isolation work the lifter bolted on: ninety seconds, and
			// specifically not the main lifts' three minutes.
			ex.Value("kind").String().IsEqual("assistance")
			ex.Value("restSeconds").Number().IsEqual(90)
		}
	}
	if !sawSquat || !sawCurl {
		t.Fatalf("preview missed a lift: squat=%v curl=%v", sawSquat, sawCurl)
	}
}

// The session is the surface the countdown actually reads, and its sets come
// back from ListSessionSets — a third query, and the one a PATCH echoes from.
func TestSessionSetsCarryTheirLiftsRest(t *testing.T) {
	e := expect(t)
	programID, dayID := firstProgramAndDay(e)
	curlID := exerciseIDByName(t, e, "Barbell Curl")
	addAssistance(t, e, programID, dayID, curlID, 3, 10, 30)

	session := startSession(t, e, dayID)
	sessionID := int(session.Value("id").Number().Raw())

	curlSets := sessionSetsFor(session, curlID)
	if len(curlSets) == 0 {
		t.Fatal("assistance did not materialize into the session")
	}
	curlSets[0].Value("restSeconds").Number().IsEqual(90)

	// A logged set echoes the same number back, so the timer restarting off the
	// PATCH response cannot disagree with the one it started from.
	setID := int(curlSets[0].Value("id").Number().Raw())
	e.PATCH(fmt.Sprintf("/sessions/%d/sets/%d", sessionID, setID)).
		WithJSON(map[string]any{"actualReps": 10, "completed": true}).
		Expect().Status(http.StatusOK).JSON().Object().
		Value("restSeconds").Number().IsEqual(90)
}

func allEqual(xs []float64) bool {
	for _, x := range xs {
		if x != xs[0] {
			return false
		}
	}
	return true
}
