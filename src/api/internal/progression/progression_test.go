package progression

import "testing"

func TestLadderFor(t *testing.T) {
	tests := []struct {
		name      string
		exercise  string
		equipment string
		want      Ladder
	}{
		// The deadlift advances faster than the bar's own step, which is the
		// whole reason Increment and Step are two fields: it goes up 10 a
		// session but still deloads onto a bar that moves in 5s.
		{"deadlift advances 10 on a 5 lb bar", "Deadlift", "barbell",
			Ladder{Increment: IncrementDeadlift, Step: BarIncrementLb}},
		{"squat takes the bar's own", "Squat", "barbell", BarLadder},
		{"overhead press takes the bar's own", "Overhead Press", "barbell", BarLadder},
		// Every weight in this app is the whole load, so a dumbbell lift is two
		// bells and the rack's 5 lb a bell is 10 on the pair. Advance and step
		// are the same number because there is nothing smaller to reach for.
		{"dumbbell press moves 10 on the pair", "Dumbbell Shoulder Press", "dumbbell",
			Ladder{Increment: DumbbellIncrementLb, Step: DumbbellIncrementLb}},
		{"any dumbbell lift, not a named one", "Hammer Curl", "dumbbell",
			Ladder{Increment: DumbbellIncrementLb, Step: DumbbellIncrementLb}},
		// Equipment outranks the name. A dumbbell deadlift is not loaded on the
		// bar the +10 was written for, and 10 happens to be right for both — so
		// the assertion that matters is the Step, which differs.
		{"equipment wins over the name", "Deadlift", "dumbbell",
			Ladder{Increment: DumbbellIncrementLb, Step: DumbbellIncrementLb}},
		// Anything the catalogue calls machine, cable, bodyweight or other is
		// loaded in whatever units it is loaded in; the bar's is the only guess
		// available and the one every lift used before ladders existed.
		{"machine work falls back to the bar", "Leg Press", "machine", BarLadder},
		{"unknown equipment falls back to the bar", "Something New", "", BarLadder},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := LadderFor(tt.exercise, tt.equipment); got != tt.want {
				t.Errorf("LadderFor(%q, %q) = %+v, want %+v",
					tt.exercise, tt.equipment, got, tt.want)
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
	db := LadderFor("Dumbbell Shoulder Press", "dumbbell")
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
	db := LadderFor("Dumbbell Shoulder Press", "dumbbell")
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
			ladder:  LadderFor(deadliftName, "barbell"),
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
