package api_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/gavv/httpexpect/v2"

	"gitea.homelab/gitadmin/iron-temple/api/internal/progression"
)

// The dumbbell press advances by a jump the rack can actually make.
//
// Every lift but the deadlift moved 5 lb a session, which was a fact about a
// barbell that had never met a counterexample — 0018 prescribed a dumbbell
// press and made one. Every weight in this app is the whole load, so a dumbbell
// lift is two bells and 5 lb on the pair is half a bell: a weight no rack
// builds, prescribed every single week.
//
// Asserted end to end rather than only in the engine because the fix is as much
// about the equipment reaching the engine as about the arithmetic once it is
// there. A LadderFor that is right and a query that does not select equipment
// produce exactly the old behaviour, and a unit test cannot see the difference.
func TestDumbbellLiftAdvancesByAPairThatExists(t *testing.T) {
	e := expect(t)

	const program = "StrongLifts 5x5 Lite (Dumbbell Press)"
	const press = "Dumbbell Shoulder Press"
	programID, dayID := programDayByName(e, program, "Workout B")

	_, seeded := exercisePreview(e, programID, dayID, press)
	logCleanExercise(t, e, dayID, press)
	status, advanced := exercisePreview(e, programID, dayID, press)

	if status != "advance" {
		t.Fatalf("a clean session read as %q, not an advance", status)
	}
	if got := advanced - seeded; got != progression.DumbbellIncrementLb {
		t.Errorf("%s went %v → %v (+%v), want +%v",
			press, seeded, advanced, got, progression.DumbbellIncrementLb)
	}
	// The point of the whole change: the number is a pair of bells.
	if r := advanced / progression.DumbbellIncrementLb; r != float64(int(r)) {
		t.Errorf("%s was sent to %v lb, which is not a pair of bells", press, advanced)
	}
}

// ...and the barbell lifts on the very same day did not come along for the ride.
// The squat shares Workout B with the press, so this is the narrowest available
// check that equipment — not the program, not the day — is what decides.
func TestBarbellLiftOnADumbbellDayStillMovesFive(t *testing.T) {
	e := expect(t)

	const program = "StrongLifts 5x5 Lite (Dumbbell Press)"
	const squat = "Squat"
	programID, dayID := programDayByName(e, program, "Workout B")

	_, seeded := exercisePreview(e, programID, dayID, squat)
	logCleanExercise(t, e, dayID, squat)
	_, advanced := exercisePreview(e, programID, dayID, squat)

	if got := advanced - seeded; got != progression.IncrementDefault {
		t.Errorf("%s went %v → %v (+%v), want +%v",
			squat, seeded, advanced, got, progression.IncrementDefault)
	}
}

// programDayByName resolves a program and one of its days, both by name.
// programAndFirstDay cannot serve here: the dumbbell press is on Workout B, and
// the first day of that program is Workout A.
func programDayByName(e *httpexpect.Expect, program, day string) (int, int) {
	programID := programIDByName(e, program)
	days := e.GET(fmt.Sprintf("/programs/%d", programID)).Expect().Status(http.StatusOK).
		JSON().Object().Value("days").Array()
	for i := 0; i < int(days.Length().Raw()); i++ {
		d := days.Value(i).Object()
		if d.Value("name").String().Raw() == day {
			return programID, int(d.Value("id").Number().Raw())
		}
	}
	panic(fmt.Sprintf("program %q has no day named %q", program, day))
}

// exercisePreview reads one named lift off a day's next-session preview.
// firstExercisePreview only ever looks at position 1.
func exercisePreview(
	e *httpexpect.Expect, programID, dayID int, name string,
) (status string, weight float64) {
	exercises := e.GET(fmt.Sprintf("/programs/%d/days/%d/next-session", programID, dayID)).
		Expect().Status(http.StatusOK).
		JSON().Object().Value("exercises").Array()
	for i := 0; i < int(exercises.Length().Raw()); i++ {
		ex := exercises.Value(i).Object()
		if ex.Value("exerciseName").String().Raw() != name {
			continue
		}
		return ex.Value("progression").Object().Value("status").String().Raw(),
			ex.Value("weightLb").Number().Raw()
	}
	panic("no prescribed lift named " + name + " on that day")
}

// logCleanExercise runs a session on a day, hitting every prescribed rep of one
// NAMED lift, and finishes it. logCleanFirstExercise does the same for whichever
// lift happens to be first.
func logCleanExercise(t *testing.T, e *httpexpect.Expect, dayID int, name string) {
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
		logSet(e, sessionID, int(set.Value("id").Number().Raw()),
			int(set.Value("targetReps").Number().Raw()), true)
		logged++
	}
	if logged == 0 {
		t.Fatalf("session for day %d logged no sets of %q", dayID, name)
	}
	e.POST(fmt.Sprintf("/sessions/%d/finish", sessionID)).Expect().Status(http.StatusOK)
}
