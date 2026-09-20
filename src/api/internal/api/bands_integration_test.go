package api_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/gavv/httpexpect/v2"
)

// The program 0023 seeds, and the day the band work opens.
const (
	bandProgram = "Glutes & Legs (Bands and Free Weights)"
	bandLift    = "Banded Lateral Walk"
	bandDay     = "Workout A"
)

// Band work does not creep.
//
// A band carries no poundage, so 0023 prescribes it at 0 — and until that
// migration, every seeded program started every lift above zero, so NextPlan
// had never been handed a zero to advance. It would have added the ladder's
// increment to it like anything else: a banded lateral walk that went well
// coming back at 5 lb, then 10, then 15, none of which is a thing a band can be.
//
// Asserted end to end rather than only in the engine, for the reason
// TestDumbbellLiftAdvancesByAPairThatExists gives about equipment: a guard that
// is right in progression.NextPlan and a preview that computes its weight some
// other way produce exactly the old behaviour, and a unit test cannot see the
// difference.
func TestBandedLiftDoesNotAdvanceFromZero(t *testing.T) {
	e := expect(t)

	programID, dayID := programDayByName(e, bandProgram, bandDay)

	if _, seeded := exercisePreview(e, programID, dayID, bandLift); seeded != 0 {
		t.Fatalf("%s is seeded at %v lb, not 0 — this test asserts nothing", bandLift, seeded)
	}

	logCleanExercise(t, e, dayID, bandLift)

	status, after := exercisePreview(e, programID, dayID, bandLift)
	if after != 0 {
		t.Errorf("a clean session sent %s to %v lb; band work must stay at 0", bandLift, after)
	}
	// "fixed" is the engine saying nothing moved this weight, which is the
	// honest answer and the one the UI already knows not to badge.
	if status != "fixed" {
		t.Errorf("%s read as %q after a clean session, want %q", bandLift, status, "fixed")
	}
}

// Three clean sessions, not one. A guard that only holds on the first is a bug
// that surfaces a month later, and the arithmetic it would have to survive
// (0 + 5 + 5 + 5) is exactly what makes the single-session check look fine.
func TestBandedLiftStaysAtZeroAcrossSessions(t *testing.T) {
	e := expect(t)

	programID, dayID := programDayByName(e, bandProgram, bandDay)

	for i := 1; i <= 3; i++ {
		logCleanExercise(t, e, dayID, bandLift)
		if _, w := exercisePreview(e, programID, dayID, bandLift); w != 0 {
			t.Fatalf("%s reached %v lb after %d clean sessions", bandLift, w, i)
		}
	}
}

// The guard reads what was WORKED, not what was prescribed — so it is a floor,
// not a cage. Hang a plate off the movement and it is a loaded lift from then
// on, progressing like anything else. Entering a load is what opts a lift in,
// which is the rule assistance work has always followed for bodyweight.
func TestLoggingALoadOptsABandedLiftIntoProgressing(t *testing.T) {
	e := expect(t)

	programID, dayID := programDayByName(e, bandProgram, bandDay)
	logCleanExerciseAt(t, e, dayID, bandLift, 10)

	status, after := exercisePreview(e, programID, dayID, bandLift)
	if status != "advance" {
		t.Fatalf("%s logged at 10 lb read as %q, not an advance", bandLift, status)
	}
	// The bar's step, since a band is not equipment LadderFor has a grid for.
	if after != 15 {
		t.Errorf("%s went 10 → %v lb, want 15", bandLift, after)
	}
}

// The loaded lifts on the very same day are untouched by the guard. Front Squat
// shares Workout A with two banded movements, so this is the narrowest
// available check that the guard keys on the weight worked rather than on the
// program, the day, or the equipment.
func TestLoadedLiftOnABandDayStillAdvances(t *testing.T) {
	e := expect(t)

	const squat = "Front Squat"
	programID, dayID := programDayByName(e, bandProgram, bandDay)

	_, seeded := exercisePreview(e, programID, dayID, squat)
	logCleanExercise(t, e, dayID, squat)
	status, advanced := exercisePreview(e, programID, dayID, squat)

	if status != "advance" {
		t.Fatalf("a clean session of %s read as %q, not an advance", squat, status)
	}
	if got := advanced - seeded; got != 5 {
		t.Errorf("%s went %v → %v (+%v), want +5", squat, seeded, advanced, got)
	}
}

// A banded lift reaches the client labelled as one, so the session screen can
// decline to draw a bar it is not loaded on. Same fact
// TestSessionSetsCarryTheirLiftsEquipment pins for dumbbells, on the kind 0023
// had to widen a CHECK to admit.
func TestSessionSetsCarryBandEquipment(t *testing.T) {
	e := expect(t)

	_, dayID := programDayByName(e, bandProgram, bandDay)
	session := startSession(t, e, dayID)

	want := map[string]string{
		"Banded Lateral Walk":   "band",
		"Front Squat":           "barbell",
		"Barbell Hip Thrust":    "barbell",
		"Bulgarian Split Squat": "dumbbell",
		"Banded Hip Abduction":  "band",
	}
	seen := map[string]bool{}

	sets := session.Value("sets").Array()
	for i := 0; i < int(sets.Length().Raw()); i++ {
		set := sets.Value(i).Object()
		name := set.Value("exerciseName").String().Raw()
		equipment, ok := want[name]
		if !ok {
			t.Fatalf("unexpected lift %q on %s %s", name, bandProgram, bandDay)
		}
		set.Value("equipment").String().IsEqual(equipment)
		seen[name] = true
	}
	for name := range want {
		if !seen[name] {
			t.Errorf("%s never materialized into the session", name)
		}
	}
}

// logCleanExerciseAt is logCleanExercise with a weight on the bar: every
// prescribed rep of one named lift, logged at weightLb, then the session
// finished. logCleanExercise takes whatever weight the prescription carried,
// which for band work is the 0 these tests are about.
func logCleanExerciseAt(
	t *testing.T, e *httpexpect.Expect, dayID int, name string, weightLb float64,
) {
	t.Helper()
	created := startSession(t, e, dayID)
	sessionID := int(created.Value("id").Number().Raw())

	sets := created.Value("sets").Array()
	logged := 0
	for i := 0; i < int(sets.Length().Raw()); i++ {
		set := sets.Value(i).Object()
		if set.Value("exerciseName").String().Raw() != name {
			continue
		}
		logSetAt(e, sessionID, int(set.Value("id").Number().Raw()),
			int(set.Value("targetReps").Number().Raw()), weightLb, true)
		logged++
	}
	if logged == 0 {
		t.Fatalf("session for day %d logged no sets of %q", dayID, name)
	}
	e.POST(fmt.Sprintf("/sessions/%d/finish", sessionID)).Expect().Status(http.StatusOK)
}
