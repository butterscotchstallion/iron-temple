package api_test

import (
	"context"
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
//
// 0017 capped rest at three minutes, which collapsed the compound tiers into one
// and made 180 both the commonest answer and the old constant. Where that leaves
// a test unable to distinguish the two, it moves a lift's rest itself rather
// than leaning on a number the seed happens to provide.

// The day the seeded programs open with is squat-led.
//
// Every lift a program prescribes rests the same three minutes since 0017 capped
// the squat and deadlift families, so the day's own numbers can no longer tell a
// column read apart from the constant it replaced — 180 IS the old constant. The
// test makes the difference by moving one lift's rest in the database and
// reading the day back: a response that still says 180 for the squat is not
// reading the column.
func TestProgramDayCarriesEachLiftsRest(t *testing.T) {
	e := expect(t)
	programID, _ := firstProgramAndDay(e)
	setExerciseRest(t, "Squat", 150)

	day := e.GET(fmt.Sprintf("/programs/%d", programID)).Expect().
		Status(http.StatusOK).JSON().Object().
		Value("days").Array().Value(0).Object()

	var seen []float64
	var sawSquat bool
	exercises := day.Value("exercises").Array()
	for i := 0; i < int(exercises.Length().Raw()); i++ {
		ex := exercises.Value(i).Object()
		rest := ex.Value("restSeconds").Number().Raw()
		seen = append(seen, rest)
		if ex.Value("exerciseName").String().Raw() == "Squat" {
			sawSquat = true
			ex.Value("restSeconds").Number().IsEqual(150)
		}
	}
	if !sawSquat {
		t.Fatal("the opening day is not squat-led; this test picked the wrong day")
	}
	// Not merely "the squat is 150": a day whose lifts all moved with it is a
	// response assembled from one number, which is the bug this replaced.
	if len(seen) < 2 {
		t.Fatalf("day has %d exercises; expected a multi-lift day", len(seen))
	}
	if allEqual(seen) {
		t.Errorf("every lift on the day rests %v — the fixed default is back", seen[0])
	}
}

// setExerciseRest moves one lift's prescribed rest for the duration of a test.
// The column has no write path through the API — it is seeded and read — so the
// database is the only way to give a lift a rest no other lift has.
func setExerciseRest(t *testing.T, name string, seconds int) {
	t.Helper()
	ctx := context.Background()
	var was int32
	err := testPool.QueryRow(ctx,
		"SELECT rest_seconds FROM exercises WHERE name = $1", name).Scan(&was)
	if err != nil {
		t.Fatalf("read rest for %q: %v", name, err)
	}
	if _, err := testPool.Exec(ctx,
		"UPDATE exercises SET rest_seconds = $1 WHERE name = $2", seconds, name); err != nil {
		t.Fatalf("set rest for %q: %v", name, err)
	}
	t.Cleanup(func() {
		if _, err := testPool.Exec(ctx,
			"UPDATE exercises SET rest_seconds = $1 WHERE name = $2", was, name); err != nil {
			t.Errorf("restore rest for %q: %v", name, err)
		}
	})
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
			ex.Value("restSeconds").Number().IsEqual(180)
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
