package progression_test

import (
	"testing"

	"gitea.homelab/gitadmin/iron-temple/api/internal/progression"
)

func perf(weight float64, reps ...int32) *progression.AssistancePerformance {
	return &progression.AssistancePerformance{WeightLb: weight, Reps: reps}
}

func ok(w float64) progression.SessionResult {
	return progression.SessionResult{WeightLb: w, Success: true}
}

func miss(w float64) progression.SessionResult {
	return progression.SessionResult{WeightLb: w, Success: false}
}

// Without a range an accessory runs the prescribed lifts' own engine. This is
// the reversal of the behaviour this file used to defend — carry the weight
// forward and let nothing move it — which read as a considered default but was
// the only reachable one, and left every accessory frozen at its first weight.
func TestNextAssistanceWithoutARange(t *testing.T) {
	tests := []struct {
		name       string
		fallback   float64
		history    []progression.SessionResult
		wantWeight float64
		wantStatus progression.Status
	}{
		{
			name:       "never performed uses the stored fallback",
			fallback:   25,
			history:    nil,
			wantWeight: 25,
			wantStatus: progression.StatusStart,
		},
		{
			// The whole point: hit your reps and it goes up next time, by the
			// least the equipment admits.
			name:       "a session where every set landed advances the weight",
			fallback:   25,
			history:    []progression.SessionResult{ok(40)},
			wantWeight: 45,
			wantStatus: progression.StatusAdvance,
		},
		{
			name:       "a miss repeats the weight",
			fallback:   25,
			history:    []progression.SessionResult{ok(35), miss(40)},
			wantWeight: 40,
			wantStatus: progression.StatusHold,
		},
		{
			// Three misses at one weight deload, exactly as they do on a squat.
			// The old rule refused this on the grounds that a stalled curl is not
			// a signal; the cost of that was a curl with nowhere to go.
			name:     "three misses deload",
			fallback: 25,
			history: []progression.SessionResult{
				miss(100), miss(100), miss(100),
			},
			wantWeight: 90,
			wantStatus: progression.StatusDeload,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := progression.NextAssistance(
				tc.fallback, 0, 0, nil, tc.history, progression.BarLadder,
			)
			if got.WeightLb != tc.wantWeight {
				t.Errorf("weight = %v, want %v", got.WeightLb, tc.wantWeight)
			}
			if got.Status != tc.wantStatus {
				t.Errorf("status = %q, want %q", got.Status, tc.wantStatus)
			}
		})
	}
}

// The weight last actually worked is the starting point, not the stored
// fallback. History counts only finished sessions — so an abandoned workout
// cannot drive progression — but a weight logged ten minutes ago in a session
// still open must not be forgotten, which for an accessory added at bodyweight
// would mean dropping from 65 lb back to 0.
func TestNextAssistanceCarriesAnUnfinishedSessionsWeight(t *testing.T) {
	got := progression.NextAssistance(
		0, 0, 0, perf(65, 10, 10), nil, progression.BarLadder,
	)
	if got.WeightLb != 65 {
		t.Errorf("weight = %v, want 65 carried from the open session", got.WeightLb)
	}
	// Not "start": the lift has been done, nothing has computed a next weight.
	if got.Status != progression.StatusFixed {
		t.Errorf("status = %q, want %q", got.Status, progression.StatusFixed)
	}

	// Once that session is in the history it advances off it rather than
	// carrying it again.
	if adv := progression.NextAssistance(
		0, 0, 0, perf(65, 10, 10),
		[]progression.SessionResult{ok(65)}, progression.BarLadder,
	); adv.WeightLb != 70 || adv.Status != progression.StatusAdvance {
		t.Errorf("got %v/%q, want 70/advance", adv.WeightLb, adv.Status)
	}
}

// The same rule on the RANGED path, which had no guard at all until the picker
// started defaulting a range on.
//
// A lifter tops out 12 reps on three sets of dips. There is nothing to add to —
// the lift is loaded at 0 — so the honest answer is that the reps went up and
// the weight did not. Without the guard this returned 0 + the ladder's step,
// which is how "3×12 dips" became "3×8 dips holding a 10 lb dumbbell" without
// anyone asking for it. Previously this needed a deliberate tick to reach; now
// every bodyweight accessory takes this branch the first time it tops out.
func TestNextAssistanceDoesNotLoadRangedBodyweightWork(t *testing.T) {
	db := progression.LadderFor("Dip", "dumbbell", progression.GymSteps{})
	got := progression.NextAssistance(0, 8, 12, perf(0, 12, 12, 12), nil, db)

	if got.WeightLb != 0 {
		t.Errorf("weight = %v, want 0 — topping out adds reps, not a plate", got.WeightLb)
	}
	if got.Status != progression.StatusFixed {
		t.Errorf("status = %q, want %q — no engine moved this weight",
			got.Status, progression.StatusFixed)
	}
	// The bottom of the range stays the target, as on every other ranged path:
	// being told to do fewer reps than you just did, for a weight that did not
	// move, is not a prescription.
	if got.TargetReps != 8 {
		t.Errorf("target reps = %d, want 8", got.TargetReps)
	}

	// A loaded lift on the same range still progresses — the guard is about
	// having nothing to add to, not about ranges.
	loaded := progression.NextAssistance(0, 8, 12, perf(40, 12, 12, 12), nil, db)
	if loaded.WeightLb != 50 || loaded.Status != progression.StatusProgressing {
		t.Errorf("loaded = %v/%q, want 50/progressing",
			loaded.WeightLb, loaded.Status)
	}
}

// Bodyweight work stays bodyweight. A set of push-ups that went well must not
// come back prescribed at 5 lb — that is a different exercise, and choosing it
// is the lifter's.
func TestNextAssistanceDoesNotLoadBodyweightWork(t *testing.T) {
	got := progression.NextAssistance(
		0, 0, 0, nil,
		[]progression.SessionResult{ok(0), ok(0)},
		progression.BarLadder,
	)
	if got.WeightLb != 0 {
		t.Errorf("weight = %v, want 0 — bodyweight work does not self-load", got.WeightLb)
	}
	if got.Status != progression.StatusFixed {
		t.Errorf("status = %q, want %q", got.Status, progression.StatusFixed)
	}

	// Once a load is entered it progresses like anything else.
	loaded := progression.NextAssistance(
		0, 0, 0, nil,
		[]progression.SessionResult{ok(0), ok(25)},
		progression.BarLadder,
	)
	if loaded.WeightLb != 30 {
		t.Errorf("weight = %v, want 30 once the lift carries a load", loaded.WeightLb)
	}
}

// An accessory advances by the least its equipment admits, not by a programme's
// pace. Those agree on a bar and on a pair of dumbbells; they come apart when a
// lifter owns finer plates, and there the curl takes the smaller jump.
func TestNextAssistanceAdvancesByTheEquipmentNotThePace(t *testing.T) {
	db := progression.LadderFor("Dumbbell Curl", "dumbbell", progression.GymSteps{})
	got := progression.NextAssistance(
		0, 0, 0, nil, []progression.SessionResult{ok(30)}, db,
	)
	if got.WeightLb != 40 {
		t.Errorf("dumbbell curl = %v, want 40 (5 lb a bell, 10 on the pair)", got.WeightLb)
	}

	fine := progression.LadderFor("Barbell Curl", "barbell",
		progression.GymSteps{BarLb: 2.5})
	if got := progression.NextAssistance(
		0, 0, 0, nil, []progression.SessionResult{ok(45)}, fine,
	); got.WeightLb != 47.5 {
		t.Errorf("curl on 1.25s = %v, want 47.5, not the squat's 50", got.WeightLb)
	}
}

func TestNextAssistanceDoubleProgression(t *testing.T) {
	tests := []struct {
		name       string
		last       *progression.AssistancePerformance
		wantWeight float64
		wantReps   int32
		wantStatus progression.Status
	}{
		{
			// Every set at the top of 8-12: the weight goes up and the reps go
			// back to the bottom.
			name:       "topping out every set advances the weight",
			last:       perf(40, 12, 12, 12),
			wantWeight: 45,
			wantReps:   8,
			wantStatus: progression.StatusProgressing,
		},
		{
			// The point of the range is to carry the top rep count across the
			// whole prescription. One set of 12 and two of 9 is not a session
			// that earned an increase.
			name:       "one short set holds the weight",
			last:       perf(40, 12, 12, 9),
			wantWeight: 40,
			wantReps:   8,
			wantStatus: progression.StatusFixed,
		},
		{
			name:       "over the top still counts as topped out",
			last:       perf(40, 14, 13, 12),
			wantWeight: 45,
			wantReps:   8,
			wantStatus: progression.StatusProgressing,
		},
		{
			name:       "the bottom of the range holds",
			last:       perf(40, 8, 8, 8),
			wantWeight: 40,
			wantReps:   8,
			wantStatus: progression.StatusFixed,
		},
		{
			// A never-performed lift starts at its stored weight, chasing the
			// bottom of the range.
			name:       "no history starts at the fallback and the bottom rep",
			last:       nil,
			wantWeight: 25,
			wantReps:   8,
			wantStatus: progression.StatusFixed,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := progression.NextAssistance(25, 8, 12, tc.last, nil, progression.BarLadder)
			if got.WeightLb != tc.wantWeight {
				t.Errorf("weight = %v, want %v", got.WeightLb, tc.wantWeight)
			}
			if got.TargetReps != tc.wantReps {
				t.Errorf("target reps = %d, want %d", got.TargetReps, tc.wantReps)
			}
			if got.Status != tc.wantStatus {
				t.Errorf("status = %q, want %q", got.Status, tc.wantStatus)
			}
		})
	}
}

// The one rule this engine will not learn. A stalled curl is not a signal, and
// cutting the weight on one solves a problem nobody has — so no history, however
// bad, may produce a weight below what was last worked.
func TestNextAssistanceNeverDeloads(t *testing.T) {
	histories := []*progression.AssistancePerformance{
		perf(40, 1, 1, 1),
		perf(40, 8, 2),
		perf(40, 1),
		perf(40, 0),
	}
	for _, last := range histories {
		got := progression.NextAssistance(25, 8, 12, last, nil, progression.BarLadder)
		if got.WeightLb < last.WeightLb {
			t.Errorf("reps %v cut the weight %v → %v", last.Reps, last.WeightLb, got.WeightLb)
		}
		if got.Status == progression.StatusDeload {
			t.Errorf("reps %v produced a deload", last.Reps)
		}
	}
}

// A range only half given, or given backwards, is not a range. The database
// rejects both, but the engine is pure and does not get to assume its caller
// checked — it collapses to the linear path rather than inventing a rule.
func TestNextAssistanceIgnoresAnUnusableRange(t *testing.T) {
	last := perf(40, 12, 12, 12)
	history := []progression.SessionResult{ok(40)}
	for _, r := range [][2]int32{{0, 12}, {8, 0}, {12, 8}, {0, 0}} {
		got := progression.NextAssistance(
			25, r[0], r[1], last, history, progression.BarLadder,
		)
		if got.WeightLb != 45 || got.Status != progression.StatusAdvance {
			t.Errorf("range %v: got %v/%q, want 45/advance", r, got.WeightLb, got.Status)
		}
	}
}

// A set nobody touched is not a set that fell short. Unlogged sets are filtered
// out before the engine sees them, so an empty list reads as "no history" rather
// than as a failure to top out.
func TestNextAssistanceTreatsNoLoggedSetsAsNoHistory(t *testing.T) {
	got := progression.NextAssistance(25, 8, 12, &progression.AssistancePerformance{
		WeightLb: 40,
		Reps:     nil,
	}, nil, progression.BarLadder)
	if got.WeightLb != 25 || got.Status != progression.StatusFixed {
		t.Errorf("got %v/%q, want the fallback 25/fixed", got.WeightLb, got.Status)
	}
}

// Topping the range moves the weight by what the equipment can build, which is
// the bug this pairs with the prescribed lifts' +5. A dumbbell accessory used to
// be sent up 5 lb on the pair — half a bell, a weight no rack makes — because
// the increment was a barbell constant with a comment claiming dumbbells were
// "no finer". They are coarser: 5 lb a bell is 10 on the pair.
func TestNextAssistanceStepsByEquipment(t *testing.T) {
	toppedOut := perf(40, 12, 12, 12)

	bar := progression.NextAssistance(25, 8, 12, toppedOut, nil, progression.BarLadder)
	if bar.WeightLb != 45 {
		t.Errorf("barbell accessory = %v, want 45", bar.WeightLb)
	}

	db := progression.LadderFor("Hammer Curl", "dumbbell", progression.GymSteps{})
	got := progression.NextAssistance(25, 8, 12, toppedOut, nil, db)
	if got.WeightLb != 50 {
		t.Errorf("dumbbell accessory = %v, want 50 (a pair that exists)", got.WeightLb)
	}
	if got.Status != progression.StatusProgressing {
		t.Errorf("status = %q, want %q", got.Status, progression.StatusProgressing)
	}
	// The range still resets to the bottom — only the size of the jump moved.
	if got.TargetReps != 8 {
		t.Errorf("target reps = %d, want 8", got.TargetReps)
	}
}

// A caller that builds a Ladder itself rather than taking one from LadderFor
// gets the barbell increment this replaced, not a weight that never moves.
func TestNextAssistanceZeroLadderKeepsTheOldBehaviour(t *testing.T) {
	got := progression.NextAssistance(25, 8, 12, perf(40, 12, 12, 12), nil, progression.Ladder{})
	if got.WeightLb != 40+progression.AssistanceIncrement {
		t.Errorf("zero ladder = %v, want %v", got.WeightLb, 40+progression.AssistanceIncrement)
	}
}
