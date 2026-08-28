package api_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/gavv/httpexpect/v2"
)

// Madcow, end to end.
//
// 0012 seeded it as a two-day A/B of straight sets on the linear engine, which
// shares a name with the program a lifter picked and nothing else. These assert
// the two properties that make it the program it says it is: sets RAMP to a top
// set, and the top set advances WEEKLY rather than every session.

func TestMadcowPrescribesARamp(t *testing.T) {
	e := expect(t)
	programID := programIDByName(e, "Madcow 5x5")

	e.GET(fmt.Sprintf("/programs/%d", programID)).
		Expect().Status(http.StatusOK).
		JSON().Object().Value("progressionKind").String().IsEqual("madcow")

	days := madcowDays(e, programID)
	if len(days) != 3 {
		t.Fatalf("Madcow has %d days, want 3 (volume, light, intensity)", len(days))
	}

	// Volume day: five ascending sets of five, topping at the lift's top set.
	squat := liftPreview(e, programID, days["Volume"], "Squat")
	plan := setPlanOf(squat)
	if len(plan) != 5 {
		t.Fatalf("volume squat has %d sets, want 5", len(plan))
	}
	assertAscending(t, "volume squat", plan)
	for _, s := range plan {
		if s.reps != 5 {
			t.Errorf("volume squat set %d is %d reps, want 5", s.setNumber, s.reps)
		}
	}
	// weightLb on the lift is the TOP set — the number that moves week to week.
	top := squat.Value("weightLb").Number().Raw()
	if plan[len(plan)-1].weightLb != top {
		t.Errorf("last ramp set is %v but the lift reports a top set of %v",
			plan[len(plan)-1].weightLb, top)
	}
	if plan[0].weightLb >= top {
		t.Errorf("the ramp opens at %v, which is not below the %v top set",
			plan[0].weightLb, top)
	}
}

func TestMadcowIntensityDayGoesAboveTheTopSetThenBacksOff(t *testing.T) {
	e := expect(t)
	programID := programIDByName(e, "Madcow 5x5")
	days := madcowDays(e, programID)

	// At the seeded 45 lb top set the day's percentages are indistinguishable —
	// 102.5% of 45 is 46.1, which rounds back onto the bar it started from. That
	// is not a fault in the ramp, it is what a 45 lb top set means; Madcow has
	// nothing to say at an empty bar. Give the squat a realistic starting point
	// and the structure of the day is visible.
	setSquatBaseline(t, e, 200)

	volumeTop := liftPreview(e, programID, days["Volume"], "Squat").
		Value("weightLb").Number().Raw()

	plan := setPlanOf(liftPreview(e, programID, days["Intensity"], "Squat"))
	if len(plan) != 6 {
		t.Fatalf("intensity squat has %d sets, want 6", len(plan))
	}

	// A triple heavier than the volume day's top: the point of the day.
	triple := plan[4]
	if triple.reps != 3 {
		t.Errorf("set 5 is %d reps, want a triple", triple.reps)
	}
	if triple.weightLb <= volumeTop {
		t.Errorf("the triple is %v, not above the %v volume top", triple.weightLb, volumeTop)
	}

	// Then a backoff set of eight, well below it.
	backoff := plan[5]
	if backoff.reps != 8 {
		t.Errorf("set 6 is %d reps, want an eight", backoff.reps)
	}
	if backoff.weightLb >= triple.weightLb {
		t.Errorf("the backoff (%v) is not below the triple (%v)", backoff.weightLb, triple.weightLb)
	}
}

// The light day is a recovery day: the same ramp, stopped one rung short.
func TestMadcowLightDayStaysBelowTheVolumeDay(t *testing.T) {
	e := expect(t)
	programID := programIDByName(e, "Madcow 5x5")
	days := madcowDays(e, programID)

	volumeTop := liftPreview(e, programID, days["Volume"], "Squat").
		Value("weightLb").Number().Raw()

	for _, s := range setPlanOf(liftPreview(e, programID, days["Light"], "Squat")) {
		if s.weightLb >= volumeTop {
			t.Errorf("light-day set %d is %v, not below the %v volume top",
				s.setNumber, s.weightLb, volumeTop)
		}
	}
}

// The session materializes the ramp, not five copies of one weight. This is what
// could not be expressed before: session_sets always stored a weight and a rep
// target per row, but the prescription only ever had one of each to give it.
func TestMadcowSessionMaterializesTheRamp(t *testing.T) {
	e := expect(t)
	programID := programIDByName(e, "Madcow 5x5")
	days := madcowDays(e, programID)

	created := startSession(t, e, days["Intensity"])
	sets := created.Value("sets").Array()

	// Three lifts: four fives, a triple and an eight each.
	if n := int(sets.Length().Raw()); n != 18 {
		t.Fatalf("the session has %d sets, want 18", n)
	}

	squatID := int(sets.Value(0).Object().Value("exerciseId").Number().Raw())
	var weights []float64
	var reps []int
	for i := 0; i < int(sets.Length().Raw()); i++ {
		set := sets.Value(i).Object()
		if int(set.Value("exerciseId").Number().Raw()) != squatID {
			continue
		}
		weights = append(weights, set.Value("weightLb").Number().Raw())
		reps = append(reps, int(set.Value("targetReps").Number().Raw()))
	}

	if len(weights) != 6 {
		t.Fatalf("the squat materialized %d sets, want 6", len(weights))
	}
	// Not one weight repeated — that was the whole bug.
	distinct := map[float64]bool{}
	for _, w := range weights {
		distinct[w] = true
	}
	if len(distinct) < 4 {
		t.Errorf("the squat's sets are %v — a ramp should not be one weight repeated", weights)
	}
	if reps[4] != 3 || reps[5] != 8 {
		t.Errorf("rep targets are %v, want a triple then an eight at the end", reps)
	}
}

// Weekly, not per session. The squat is trained on all three days, but only the
// volume day decides its top set — so a light or intensity session must not move
// next week's numbers, and a volume session must.
func TestMadcowTopSetAdvancesOnlyOnTheReferenceDay(t *testing.T) {
	e := expect(t)
	programID := programIDByName(e, "Madcow 5x5")
	days := madcowDays(e, programID)

	setSquatBaseline(t, e, 200)

	before := liftPreview(e, programID, days["Volume"], "Squat").
		Value("weightLb").Number().Raw()

	// A clean intensity session: real work on the squat, on a day that is not
	// its reference.
	logEveryRepOf(t, e, days["Intensity"], "Squat")
	afterIntensity := liftPreview(e, programID, days["Volume"], "Squat").
		Value("weightLb").Number().Raw()
	if afterIntensity != before {
		t.Fatalf("an intensity session moved the top set: %v → %v", before, afterIntensity)
	}

	// A clean volume session does move it — once.
	logEveryRepOf(t, e, days["Volume"], "Squat")
	afterVolume := liftPreview(e, programID, days["Volume"], "Squat").
		Value("weightLb").Number().Raw()
	if afterVolume <= before {
		t.Fatalf("a volume session did not advance the top set: %v → %v", before, afterVolume)
	}

	// And the whole ramp moves with it, because every rung is a percentage of it.
	for _, s := range setPlanOf(liftPreview(e, programID, days["Light"], "Squat")) {
		if s.weightLb <= 0 {
			t.Errorf("light-day set %d came back at %v", s.setNumber, s.weightLb)
		}
	}
}

// Every other program keeps prescribing a uniform block — a flat set plan rather
// than no set plan, so a client has one shape to read.
func TestLinearProgramsStillEmitAFlatSetPlan(t *testing.T) {
	e := expect(t)
	programID, dayID := programAndFirstDay(e, "StrongLifts 5x5")

	squat := liftPreview(e, programID, dayID, "Squat")
	plan := setPlanOf(squat)
	if len(plan) != 5 {
		t.Fatalf("StrongLifts squat has %d sets in its plan, want 5", len(plan))
	}
	weight := squat.Value("weightLb").Number().Raw()
	for _, s := range plan {
		if s.weightLb != weight || s.reps != 5 {
			t.Errorf("set %d is %v lb x %d, want %v x 5", s.setNumber, s.weightLb, s.reps, weight)
		}
	}
}

// ---- helpers ----

type planSet struct {
	setNumber int
	reps      int
	weightLb  float64
}

func setPlanOf(lift *httpexpect.Object) []planSet {
	arr := lift.Value("setPlan").Array()
	out := make([]planSet, 0, int(arr.Length().Raw()))
	for i := 0; i < int(arr.Length().Raw()); i++ {
		s := arr.Value(i).Object()
		out = append(out, planSet{
			setNumber: int(s.Value("setNumber").Number().Raw()),
			reps:      int(s.Value("reps").Number().Raw()),
			weightLb:  s.Value("weightLb").Number().Raw(),
		})
	}
	return out
}

func assertAscending(t *testing.T, what string, plan []planSet) {
	t.Helper()
	for i := 1; i < len(plan); i++ {
		if plan[i].weightLb <= plan[i-1].weightLb {
			t.Errorf("%s does not ramp: set %d (%v) is not above set %d (%v)",
				what, plan[i].setNumber, plan[i].weightLb,
				plan[i-1].setNumber, plan[i-1].weightLb)
		}
	}
}

// setSquatBaseline gives the squat a realistic top set for the duration of a
// test. It lands because a baseline is consulted when the lift has no history —
// and Madcow reads history from the lift's reference day alone, which no other
// test touches.
func setSquatBaseline(t *testing.T, e *httpexpect.Expect, weightLb float64) {
	t.Helper()
	squatID := exerciseIDByName(t, e, "Squat")
	e.PUT(fmt.Sprintf("/me/baselines/%d", squatID)).
		WithJSON(map[string]any{"weightLb": weightLb}).
		Expect().Status(http.StatusNoContent)
	t.Cleanup(func() {
		e.DELETE(fmt.Sprintf("/me/baselines/%d", squatID)).Expect()
	})
}

func programIDByName(e *httpexpect.Expect, name string) int {
	programs := e.GET("/programs").Expect().Status(http.StatusOK).JSON().Array()
	for i := 0; i < int(programs.Length().Raw()); i++ {
		p := programs.Value(i).Object()
		if p.Value("name").String().Raw() == name {
			return int(p.Value("id").Number().Raw())
		}
	}
	panic("no seeded program named " + name)
}

// madcowDays maps day name to id.
func madcowDays(e *httpexpect.Expect, programID int) map[string]int {
	days := e.GET(fmt.Sprintf("/programs/%d", programID)).
		Expect().Status(http.StatusOK).
		JSON().Object().Value("days").Array()
	out := map[string]int{}
	for i := 0; i < int(days.Length().Raw()); i++ {
		d := days.Value(i).Object()
		out[d.Value("name").String().Raw()] = int(d.Value("id").Number().Raw())
	}
	return out
}

func liftPreview(e *httpexpect.Expect, programID, dayID int, name string) *httpexpect.Object {
	exercises := e.GET(fmt.Sprintf("/programs/%d/days/%d/next-session", programID, dayID)).
		Expect().Status(http.StatusOK).
		JSON().Object().Value("exercises").Array()
	for i := 0; i < int(exercises.Length().Raw()); i++ {
		ex := exercises.Value(i).Object()
		if ex.Value("exerciseName").String().Raw() == name {
			return ex
		}
	}
	panic(name + " is not prescribed on that day")
}

// logEveryRepOf runs a session on a day and hits every prescribed rep of one
// lift, then finishes it — one clean session for that lift on that day.
func logEveryRepOf(t *testing.T, e *httpexpect.Expect, dayID int, name string) {
	t.Helper()
	created := startSession(t, e, dayID)
	sessionID := int(created.Value("id").Number().Raw())

	sets := created.Value("sets").Array()
	for i := 0; i < int(sets.Length().Raw()); i++ {
		set := sets.Value(i).Object()
		if set.Value("exerciseName").String().Raw() != name {
			continue
		}
		logSet(e, sessionID, int(set.Value("id").Number().Raw()),
			int(set.Value("targetReps").Number().Raw()), true)
	}
	e.POST(fmt.Sprintf("/sessions/%d/finish", sessionID)).Expect().Status(http.StatusOK)
}
