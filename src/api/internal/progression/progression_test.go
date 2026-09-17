package progression

import (
	"math"
	"testing"
)

// fineGym is a lifter who owns 1.25 lb plates and adjustable dumbbells that
// step 1.25 a bell: a bar moving in 2.5s and a pair moving in 2.5s. Both are
// half of what the constants assume, which is what makes it a useful contrast.
var fineGym = GymSteps{BarLb: 2.5, DumbbellLb: 2.5}

func TestLadderFor(t *testing.T) {
	tests := []struct {
		name      string
		exercise  string
		equipment string
		gym       GymSteps
		want      Ladder
	}{
		// An undescribed gym prescribes exactly what this function returned
		// before it was shown one. Every case below with a zero GymSteps is
		// pinning that, and together they are the regression guard for the
		// whole change.
		//
		// The deadlift advances faster than the bar's own step, which is the
		// whole reason Increment and Step are two fields: it goes up 10 a
		// session but still deloads onto a bar that moves in 5s.
		{"deadlift advances 10 on a 5 lb bar", "Deadlift", "barbell", GymSteps{},
			Ladder{Increment: IncrementDeadlift, Step: BarIncrementLb}},
		{"squat takes the bar's own", "Squat", "barbell", GymSteps{}, BarLadder},
		{"overhead press takes the bar's own", "Overhead Press", "barbell", GymSteps{}, BarLadder},
		// Every weight in this app is the whole load, so a dumbbell lift is two
		// bells and the rack's 5 lb a bell is 10 on the pair. Advance and step
		// are the same number because there is nothing smaller to reach for.
		{"dumbbell press moves 10 on the pair", "Dumbbell Shoulder Press", "dumbbell", GymSteps{},
			Ladder{Increment: DumbbellIncrementLb, Step: DumbbellIncrementLb}},
		{"any dumbbell lift, not a named one", "Hammer Curl", "dumbbell", GymSteps{},
			Ladder{Increment: DumbbellIncrementLb, Step: DumbbellIncrementLb}},
		// Equipment outranks the name. A dumbbell deadlift is not loaded on the
		// bar the +10 was written for, and 10 happens to be right for both — so
		// the assertion that matters is the Step, which differs.
		{"equipment wins over the name", "Deadlift", "dumbbell", GymSteps{},
			Ladder{Increment: DumbbellIncrementLb, Step: DumbbellIncrementLb}},
		// Anything the catalogue calls machine, cable, bodyweight or other is
		// loaded in whatever units it is loaded in; the bar's is the only guess
		// available and the one every lift used before ladders existed.
		{"machine work falls back to the bar", "Leg Press", "machine", GymSteps{}, BarLadder},
		{"unknown equipment falls back to the bar", "Something New", "", GymSteps{}, BarLadder},

		// A described gym changes the grid but not the pace. Owning 1.25s does
		// not mean squatting 2.5 lb heavier every session — it means a deload
		// can land somewhere the standard rack cannot reach.
		{"a finer bar keeps the pace and gains the grid", "Squat", "barbell", fineGym,
			Ladder{Increment: IncrementDefault, Step: 2.5}},
		{"a finer bar does not slow the deadlift", "Deadlift", "barbell", fineGym,
			Ladder{Increment: IncrementDeadlift, Step: 2.5}},
		// The case this whole change exists for. A rack stepping 1.25 a bell
		// moves the pair in 2.5s, so the pace of 5 is reachable — and a curl
		// that tops its range goes up 2.5 rather than 10.
		{"a finer rack lets a pair advance 5", "Dumbbell Shoulder Press", "dumbbell", fineGym,
			Ladder{Increment: IncrementDefault, Step: 2.5}},
		// One field configured and the other not: each falls back on its own.
		{"an unconfigured rack still steps 10", "Hammer Curl", "dumbbell",
			GymSteps{BarLb: 2.5},
			Ladder{Increment: DumbbellIncrementLb, Step: DumbbellIncrementLb}},
		{"an unconfigured bar still steps 5", "Squat", "barbell",
			GymSteps{DumbbellLb: 2.5}, BarLadder},
		// A gym coarser than the pace gets the next weight that exists, not a
		// rounded-down advance of zero. This is what the dumbbell pair has
		// always done; it is only now expressible for a bar too.
		{"a coarse bar advances to what it can build", "Squat", "barbell",
			GymSteps{BarLb: 20}, Ladder{Increment: 20, Step: 20}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := LadderFor(tt.exercise, tt.equipment, tt.gym); got != tt.want {
				t.Errorf("LadderFor(%q, %q, %+v) = %+v, want %+v",
					tt.exercise, tt.equipment, tt.gym, got, tt.want)
			}
		})
	}
}

// advanceAtLeast is the arithmetic the whole rework turns on, so it is pinned
// directly rather than only through LadderFor.
func TestAdvanceAtLeast(t *testing.T) {
	tests := []struct {
		name       string
		want, step float64
		out        float64
	}{
		{"an exact multiple is left alone", 5, 5, 5},
		{"two steps of a finer grid", 5, 2.5, 5},
		{"four steps of a finer grid", 5, 1.25, 5},
		{"the deadlift on a standard bar", 10, 5, 10},
		{"a grid coarser than the pace rounds up", 5, 10, 10},
		{"a grid that divides unevenly rounds up", 5, 3, 6},
		// Not down to 0, which would be a lift that never moves.
		{"a very coarse grid still advances", 5, 45, 45},
		// The guard for a caller that built a Ladder by hand.
		{"a zero step is left as asked", 5, 0, 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := advanceAtLeast(tt.want, tt.step); got != tt.out {
				t.Errorf("advanceAtLeast(%v, %v) = %v, want %v",
					tt.want, tt.step, got, tt.out)
			}
		})
	}
}

// TestDumbbellDeloadIsReachable pins the other half of the bug. Advancing was
// the visible symptom, but a stalled dumbbell lift also deloaded onto the bar's
// 5 lb grid, and half those weights are half a bell.
//
// 130 is the case that separates the two grids: 130 * 0.90 = 117, which rounds
// to 120 on a pair and to 115 on a bar. Most weights do not separate them —
// 100 * 0.90 = 90 is reachable either way — which is exactly why this asserts
// against a number chosen to tell them apart rather than a convenient one.
func TestDumbbellDeloadIsReachable(t *testing.T) {
	db := LadderFor("Dumbbell Shoulder Press", "dumbbell", GymSteps{})
	stalled := []SessionResult{fail(130), fail(130), fail(130)}

	if got := Next(0, db, stalled); got != 120 {
		t.Errorf("dumbbell deload from 130 = %v, want 120 (a pair that exists)", got)
	}
	if got := Next(0, BarLadder, stalled); got != 115 {
		t.Errorf("barbell deload from 130 = %v, want 115", got)
	}
}

// Every weight a dumbbell lift can be sent to is a pair that exists — advancing,
// holding or deloading. The cases above sample it; this is the property.
func TestDumbbellWeightsAreAlwaysPairs(t *testing.T) {
	db := LadderFor("Dumbbell Shoulder Press", "dumbbell", GymSteps{})
	for start := 20.0; start <= 200; start += 10 {
		histories := [][]SessionResult{
			nil,
			{ok(start)},
			{fail(start)},
			{fail(start), fail(start)},
			{fail(start), fail(start), fail(start)},
		}
		for _, h := range histories {
			got := Next(start, db, h)
			if r := got / DumbbellIncrementLb; r != float64(int(r)) {
				t.Errorf("Next(%v, dumbbell, %v) = %v, not a multiple of %v",
					start, h, got, DumbbellIncrementLb)
			}
		}
	}
}

// The same property against a described rack. A lifter whose bells step 1.25
// can be sent to 125 on the pair; one whose bells step 5 cannot, and the point
// of GymSteps is that the engine now knows which lifter it is talking to.
func TestAFinerRackReachesWeightsTheDefaultCannot(t *testing.T) {
	db := LadderFor("Hammer Curl", "dumbbell", fineGym)
	for start := 20.0; start <= 200; start += 2.5 {
		histories := [][]SessionResult{
			nil,
			{ok(start)},
			{fail(start), fail(start), fail(start)},
		}
		for _, h := range histories {
			got := Next(start, db, h)
			if r := got / 2.5; math.Abs(r-math.Round(r)) > 1e-9 {
				t.Errorf("Next(%v, fine rack, %v) = %v, not a multiple of 2.5",
					start, h, got)
			}
		}
	}

	// And it advances by the pace, not by the step: a finer rack does not mean
	// a slower lift.
	if got := Next(0, db, []SessionResult{ok(100)}); got != 105 {
		t.Errorf("advance on a fine rack = %v, want 105", got)
	}
}

// A deload has to actually come down, and snapping alone does not guarantee it.
// Ten percent off a light weight can be less than half a step, so it rounds back
// to the weight that just failed three times and prescribes it again as a fresh
// run-up — forever. The coarser the grid relative to the weight, the likelier,
// which is why this bites a 30 lb pair of dumbbells and not a 300 lb squat.
func TestDeloadAlwaysDescends(t *testing.T) {
	db := LadderFor("Dumbbell Curl", "dumbbell", GymSteps{})

	// 40 * 0.90 = 36, which snaps to 40 on a 10 lb grid. One step off instead.
	stalled := []SessionResult{fail(40), fail(40), fail(40)}
	if got := Next(0, db, stalled); got != 30 {
		t.Errorf("deload from 40 on a 10 lb grid = %v, want 30", got)
	}

	// The seeded dumbbell press starts here, so this was reachable in a shipped
	// program: 30 * 0.90 = 27 snaps back to 30.
	at30 := []SessionResult{fail(30), fail(30), fail(30)}
	if got := Next(0, db, at30); got != 20 {
		t.Errorf("deload from 30 = %v, want 20", got)
	}

	// Never below zero, which is where bodyweight work already sits.
	at10 := []SessionResult{fail(10), fail(10), fail(10)}
	if got := Next(0, db, at10); got != 0 {
		t.Errorf("deload from 10 = %v, want 0", got)
	}

	// And the weights where the snap already descended are untouched: 10% is
	// still 10% wherever the grid can express it.
	if got := Next(0, BarLadder, []SessionResult{fail(135), fail(135), fail(135)}); got != 120 {
		t.Errorf("deload from 135 on a bar = %v, want 120 (unchanged)", got)
	}
	if got := Next(0, db, []SessionResult{fail(130), fail(130), fail(130)}); got != 120 {
		t.Errorf("deload from 130 on the pair = %v, want 120 (unchanged)", got)
	}
}

func ok(w float64) SessionResult   { return SessionResult{WeightLb: w, Success: true} }
func fail(w float64) SessionResult { return SessionResult{WeightLb: w, Success: false} }

func TestNext(t *testing.T) {
	tests := []struct {
		name    string
		start   float64
		ladder  Ladder
		history []SessionResult
		want    float64
	}{
		{
			name:    "no history returns starting weight",
			start:   45,
			ladder:  BarLadder,
			history: nil,
			want:    45,
		},
		{
			name:    "success advances by increment",
			ladder:  BarLadder,
			history: []SessionResult{ok(100)},
			want:    105,
		},
		{
			name:    "deadlift success advances by ten",
			ladder:  LadderFor(deadliftName, "barbell", GymSteps{}),
			history: []SessionResult{ok(135)},
			want:    145,
		},
		{
			name:    "single failure repeats the weight",
			ladder:  BarLadder,
			history: []SessionResult{ok(95), fail(100)},
			want:    100,
		},
		{
			name:    "two failures still repeat",
			ladder:  BarLadder,
			history: []SessionResult{fail(100), fail(100)},
			want:    100,
		},
		{
			name:    "three failures deload to ninety percent",
			ladder:  BarLadder,
			history: []SessionResult{fail(100), fail(100), fail(100)},
			want:    90,
		},
		{
			name:   "deload rounds to nearest five",
			ladder: BarLadder,
			// 105 * 0.90 = 94.5 -> 95
			history: []SessionResult{fail(105), fail(105), fail(105)},
			want:    95,
		},
		{
			name:   "failure streak counts only at current weight",
			ladder: BarLadder,
			// Failed thrice at 100, deloaded to 90, failed once there:
			// the old streak must not carry over and re-trigger a deload.
			history: []SessionResult{fail(100), fail(100), fail(100), fail(90)},
			want:    90,
		},
		{
			name:    "success after failures resets and advances",
			ladder:  BarLadder,
			history: []SessionResult{fail(100), fail(100), ok(100)},
			want:    105,
		},
		{
			name:   "only trailing failures count",
			ladder: BarLadder,
			// Two early failures, a success, then two failures: streak is 2.
			history: []SessionResult{fail(100), fail(100), ok(100), fail(105), fail(105)},
			want:    105,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Next(tt.start, tt.ladder, tt.history); got != tt.want {
				t.Errorf("Next(%v, %v, %v) = %v, want %v",
					tt.start, tt.ladder, tt.history, got, tt.want)
			}
		})
	}
}

// TestNextPlan pins the reasoning NextPlan reports alongside the weight — the
// status, the trailing failure count, and the weight it came from. Next() is a
// wrapper, so its weight math is covered by TestNext above.
func TestNextPlan(t *testing.T) {
	tests := []struct {
		name    string
		start   float64
		ladder  Ladder
		history []SessionResult
		want    Plan
	}{
		{
			name:    "no history is a start with no counts",
			start:   45,
			history: nil,
			want:    Plan{WeightLb: 45, Status: StatusStart},
		},
		{
			name:    "success advances and records the prior weight",
			ladder:  BarLadder,
			history: []SessionResult{ok(100)},
			want:    Plan{WeightLb: 105, Status: StatusAdvance, PreviousLb: 100},
		},
		{
			name:    "one failure holds with a single-fail count",
			ladder:  BarLadder,
			history: []SessionResult{ok(95), fail(100)},
			want:    Plan{WeightLb: 100, Status: StatusHold, FailureCount: 1, PreviousLb: 100},
		},
		{
			name:    "two failures still hold",
			ladder:  BarLadder,
			history: []SessionResult{fail(100), fail(100)},
			want:    Plan{WeightLb: 100, Status: StatusHold, FailureCount: 2, PreviousLb: 100},
		},
		{
			name:   "three failures deload from the stalled weight",
			ladder: BarLadder,
			// 100 * 0.90 = 90.
			history: []SessionResult{fail(100), fail(100), fail(100)},
			want:    Plan{WeightLb: 90, Status: StatusDeload, FailureCount: 3, PreviousLb: 100},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NextPlan(tt.start, tt.ladder, tt.history)
			if got != tt.want {
				t.Errorf("NextPlan(%v, %v, %v) = %+v, want %+v",
					tt.start, tt.ladder, tt.history, got, tt.want)
			}
		})
	}
}
