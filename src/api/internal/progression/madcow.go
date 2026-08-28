package progression

import "math"

// Madcow: ramping sets, and a top set that moves once a week.
//
// The linear engine gives a lift one weight per session and raises it every
// session. Madcow does neither. A day's work climbs to a top set through a ramp
// — 50%, 62.5%, 75%, 87.5%, 100% — and what advances is the top set, once per
// week, with every other set following it as a percentage.
//
// The percentages are the program and live in program_day_exercise_sets. This
// file does two things with them: works out what the top set should be, and
// resolves the ramp against it.
//
// # How weekly progression falls out of a per-session engine
//
// It is not a special case. Each lift has exactly one REFERENCE DAY — the day
// whose ramp reaches 100% — and the top set is computed by the ordinary linear
// engine fed only that day's history. A reference day comes round once a week,
// so "advance once per reference-day session" and "advance once a week" are the
// same sentence. Squat, bench and row reference the volume day; the press and
// the deadlift reference the light day, because that is the only day they are
// trained.
//
// Scoping the history is what makes this work and is the part worth being
// careful about. The squat's top set on the light day is 87.5% and on the
// intensity day 102.5%, so a history that took every day's heaviest set would
// see the reference wander up and down and read it as progress and regress that
// never happened.

// TopSet computes the weight a lift's 100% set should be this week.
//
// history is the lift's performances ON ITS REFERENCE DAY, oldest first —
// nothing else. startingWeight is the top set's starting weight, and increment
// is the per-week jump (Madcow proper moves 2.5% a week; this uses the same
// per-lift pounds the rest of the app does, which is close enough at these
// weights and keeps one rule rather than two).
//
// This is NextPlan, unchanged, and deliberately so: hold after a failure and
// deload after three are as right for a ramping program as for a linear one, and
// a lifter who has stalled three weeks running needs the same answer either way.
func TopSet(startingWeight, increment float64, history []SessionResult) Plan {
	return NextPlan(startingWeight, increment, history)
}

// RampSet is one set of a resolved ramp: what to load and for how many reps.
type RampSet struct {
	SetNumber int32
	Reps      int32
	WeightLb  float64
}

// RampStep is one rung of a prescription as the program states it — a rep count
// and a percentage of the lift's top set.
type RampStep struct {
	SetNumber int32
	Reps      int32
	PctOfTop  float64
}

// ResolveRamp turns a prescription into weights for a given top set.
//
// Every rung snaps to the nearest 5 lb, the smallest change a standard barbell
// admits. It rounds rather than floors: a warm-up rung is not a working set, and
// putting 2.5 lb more on a 62.5% rung matters far less than the arithmetic
// staying legible. Where it does matter — a weight the lifter's rack cannot
// build — the client rounds again against the plates actually owned, and rounds
// DOWN, which is the direction that cannot hurt.
//
// A rung that lands at or below zero is dropped rather than emitted at 0: the
// top set is the only number a lifter chose, and a percentage of a very light
// top set is not a set at all.
func ResolveRamp(topSetLb float64, steps []RampStep) []RampSet {
	out := make([]RampSet, 0, len(steps))
	for _, s := range steps {
		w := roundToBar(topSetLb * s.PctOfTop / 100)
		if w <= 0 {
			continue
		}
		out = append(out, RampSet{SetNumber: s.SetNumber, Reps: s.Reps, WeightLb: w})
	}
	return out
}

// UniformRamp is the prescription for a lift with no set plan: sets of the same
// reps at the same weight, which is what every non-Madcow program prescribes.
//
// It exists so callers have one shape to render rather than two. A program that
// has never heard of ramping still emits a plan; it simply has one weight in it.
func UniformRamp(sets, reps int32, weightLb float64) []RampSet {
	out := make([]RampSet, 0, sets)
	for i := int32(1); i <= sets; i++ {
		out = append(out, RampSet{SetNumber: i, Reps: reps, WeightLb: weightLb})
	}
	return out
}

// TopPct is the percentage that marks a lift's reference day. A day whose ramp
// contains it is the day the engine reads history from.
const TopPct = 100.0

// IsReferenceDay reports whether a ramp reaches the lift's top set — that is,
// whether this is the day the top set is decided on.
//
// Exactly 100, not "the highest percentage here": the intensity day tops at
// 102.5% and is emphatically not the reference. It is a heavier effort for fewer
// reps, taken as a percentage of a number decided elsewhere.
func IsReferenceDay(steps []RampStep) bool {
	for _, s := range steps {
		if math.Abs(s.PctOfTop-TopPct) < 0.001 {
			return true
		}
	}
	return false
}
