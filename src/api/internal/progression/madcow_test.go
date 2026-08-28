package progression_test

import (
	"testing"

	"gitea.homelab/gitadmin/iron-temple/api/internal/progression"
)

// The seeded ramps, as 0015 writes them.
var (
	volumeRamp = []progression.RampStep{
		{SetNumber: 1, Reps: 5, PctOfTop: 50},
		{SetNumber: 2, Reps: 5, PctOfTop: 62.5},
		{SetNumber: 3, Reps: 5, PctOfTop: 75},
		{SetNumber: 4, Reps: 5, PctOfTop: 87.5},
		{SetNumber: 5, Reps: 5, PctOfTop: 100},
	}
	lightRamp = []progression.RampStep{
		{SetNumber: 1, Reps: 5, PctOfTop: 50},
		{SetNumber: 2, Reps: 5, PctOfTop: 62.5},
		{SetNumber: 3, Reps: 5, PctOfTop: 75},
		{SetNumber: 4, Reps: 5, PctOfTop: 87.5},
	}
	intensityRamp = []progression.RampStep{
		{SetNumber: 1, Reps: 5, PctOfTop: 50},
		{SetNumber: 2, Reps: 5, PctOfTop: 62.5},
		{SetNumber: 3, Reps: 5, PctOfTop: 75},
		{SetNumber: 4, Reps: 5, PctOfTop: 87.5},
		{SetNumber: 5, Reps: 3, PctOfTop: 102.5},
		{SetNumber: 6, Reps: 8, PctOfTop: 75},
	}
)

func TestResolveRampClimbsToTheTopSet(t *testing.T) {
	got := progression.ResolveRamp(200, volumeRamp)
	want := []float64{100, 125, 150, 175, 200}
	if len(got) != len(want) {
		t.Fatalf("got %d sets, want %d", len(got), len(want))
	}
	for i, w := range want {
		if got[i].WeightLb != w {
			t.Errorf("set %d = %v lb, want %v", got[i].SetNumber, got[i].WeightLb, w)
		}
		if got[i].Reps != 5 {
			t.Errorf("set %d = %d reps, want 5", got[i].SetNumber, got[i].Reps)
		}
	}
}

// The intensity day is the reason pct_of_top may exceed 100: a triple heavier
// than the volume day's top, then a backoff set of eight well below it.
func TestResolveRampHandlesTheIntensityDay(t *testing.T) {
	got := progression.ResolveRamp(200, intensityRamp)
	if len(got) != 6 {
		t.Fatalf("got %d sets, want 6", len(got))
	}

	top := got[4]
	if top.WeightLb != 205 || top.Reps != 3 {
		t.Errorf("top set = %v lb x %d, want 205 x 3", top.WeightLb, top.Reps)
	}
	if top.WeightLb <= got[3].WeightLb {
		t.Error("the triple should be heavier than the ramp that precedes it")
	}

	backoff := got[5]
	if backoff.WeightLb != 150 || backoff.Reps != 8 {
		t.Errorf("backoff = %v lb x %d, want 150 x 8", backoff.WeightLb, backoff.Reps)
	}
}

// Every rung has to be loadable. 62.5% of 185 is 115.625, which is not a weight.
func TestResolveRampSnapsToTheBar(t *testing.T) {
	for _, top := range []float64{185, 137.5, 95, 245} {
		for _, set := range progression.ResolveRamp(top, intensityRamp) {
			if rem := set.WeightLb / progression.BarIncrementLb; rem != float64(int(rem)) {
				t.Errorf("top %v: set %d is %v lb, not a multiple of %v",
					top, set.SetNumber, set.WeightLb, progression.BarIncrementLb)
			}
		}
	}
}

// The whole point of the light day: it tops out below the volume day rather than
// matching it, which is what makes it a recovery day.
func TestLightDayStaysBelowTheTopSet(t *testing.T) {
	top := 200.0
	for _, set := range progression.ResolveRamp(top, lightRamp) {
		if set.WeightLb >= top {
			t.Errorf("light-day set %d is %v lb, not below the %v top set",
				set.SetNumber, set.WeightLb, top)
		}
	}
}

// Exactly 100, not "the highest percentage here". The intensity day tops at
// 102.5% and is emphatically not the reference — it is a percentage OF a number
// decided elsewhere, and treating it as the reference would compound weekly.
func TestReferenceDayIsTheOneThatReachesAHundred(t *testing.T) {
	if !progression.IsReferenceDay(volumeRamp) {
		t.Error("the volume day reaches 100% and should be the reference")
	}
	if progression.IsReferenceDay(lightRamp) {
		t.Error("the light day tops at 87.5% and is not a reference")
	}
	if progression.IsReferenceDay(intensityRamp) {
		t.Error("the intensity day tops at 102.5% and is not a reference")
	}
}

// Weekly progression is not a special case in the engine: it is the linear
// engine fed only the reference day's history, and a reference day comes round
// once a week.
func TestTopSetAdvancesOncePerReferenceDaySession(t *testing.T) {
	history := []progression.SessionResult{
		{WeightLb: 200, Success: true},
	}
	got := progression.TopSet(45, progression.IncrementDefault, history)
	if got.WeightLb != 205 {
		t.Errorf("top set = %v, want 205", got.WeightLb)
	}
	if got.Status != progression.StatusAdvance {
		t.Errorf("status = %q, want advance", got.Status)
	}

	// A failed week holds, and three hold-then-fail weeks deload — the same
	// answers the linear engine gives, which are as right here as there.
	stalled := []progression.SessionResult{
		{WeightLb: 200, Success: false},
		{WeightLb: 200, Success: false},
		{WeightLb: 200, Success: false},
	}
	if d := progression.TopSet(45, progression.IncrementDefault, stalled); d.Status != progression.StatusDeload {
		t.Errorf("three failed weeks gave %q, want deload", d.Status)
	}
}

// A uniform block is still a plan. Every non-Madcow program emits one, so a
// client has one shape to render rather than two.
func TestUniformRampIsAFlatPlan(t *testing.T) {
	got := progression.UniformRamp(5, 5, 135)
	if len(got) != 5 {
		t.Fatalf("got %d sets, want 5", len(got))
	}
	for i, set := range got {
		if set.SetNumber != int32(i+1) || set.Reps != 5 || set.WeightLb != 135 {
			t.Errorf("set %d = %+v, want %d/5/135", i+1, set, i+1)
		}
	}
}

// A percentage of a very light top set is not a set. Emitting it at 0 lb would
// put an empty row in the session for the lifter to tick off.
func TestResolveRampDropsRungsThatRoundToNothing(t *testing.T) {
	for _, set := range progression.ResolveRamp(4, volumeRamp) {
		if set.WeightLb <= 0 {
			t.Errorf("set %d came back at %v lb", set.SetNumber, set.WeightLb)
		}
	}
}
