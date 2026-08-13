// Package progression computes the target weight for the next session of a
// lift from its performance history. All programs are linear (see
// docs/implementation-plan.md); they differ only in set count, not in how
// weight advances, so a single engine serves all of them.
//
// Rules (pounds):
//   - Advance by a per-lift increment after a successful session:
//     +10 lb on the deadlift, +5 lb on every other lift.
//   - Repeat the same weight after a failed session.
//   - After 3 consecutive failed sessions at the same weight, deload to 90%
//     of that weight (rounded to the nearest 5 lb), giving a fresh run-up.
//
// The engine is pure: it takes a history and returns a number, with no I/O.
// The data layer supplies the history; the API layer maps exercises to
// increments and rounds for display.
package progression

import "math"

// Defaults for the linear model.
const (
	// IncrementDeadlift is the per-session jump for the deadlift, in lb.
	IncrementDeadlift = 10.0
	// IncrementDefault is the per-session jump for every other lift, in lb.
	IncrementDefault = 5.0

	// DeloadFactor is the fraction of the working weight kept on a deload.
	DeloadFactor = 0.90
	// FailuresBeforeDeload is how many consecutive failed sessions at a
	// weight trigger a deload.
	FailuresBeforeDeload = 3
	// BarIncrementLb is the smallest change a standard barbell admits
	// (2.5 lb per side); computed weights snap to this.
	BarIncrementLb = 5.0
)

// deadliftName is the seeded exercise name that progresses at the faster rate.
const deadliftName = "Deadlift"

// IncrementFor returns the linear per-session increment for a lift, in lb,
// keyed by its (seeded) exercise name. Unknown names take the default +5.
func IncrementFor(exerciseName string) float64 {
	if exerciseName == deadliftName {
		return IncrementDeadlift
	}
	return IncrementDefault
}

// SessionResult is the outcome of one past performance of a single lift,
// oldest-to-newest when collected into a history.
type SessionResult struct {
	// WeightLb is the weight worked at that session.
	WeightLb float64
	// Success is true only if every prescribed set met its target reps.
	Success bool
}

// Status labels why the engine chose the next weight, so callers can explain a
// deload or an impending stall instead of showing a bare number.
type Status string

const (
	// StatusStart means there is no history yet; the starting weight is used.
	StatusStart Status = "start"
	// StatusAdvance means the last session succeeded and the weight went up.
	StatusAdvance Status = "advance"
	// StatusHold means a recent failure is repeating the same weight (short of
	// the deload threshold).
	StatusHold Status = "hold"
	// StatusDeload means the failure streak hit the threshold and the weight
	// dropped to a fraction of the stalled working weight.
	StatusDeload Status = "deload"
)

// Plan is the engine's decision for the next session: the target weight plus the
// reasoning behind it.
type Plan struct {
	// WeightLb is the target weight for the upcoming session.
	WeightLb float64
	// Status is why this weight was chosen.
	Status Status
	// FailureCount is the number of consecutive trailing failures at the working
	// weight (0 on StatusStart and StatusAdvance). Held as int32 to match the
	// width the API serialises it at, so the value never needs a narrowing
	// conversion on the way out (which gosec flags as G115).
	FailureCount int32
	// PreviousLb is the weight just worked — the weight advanced past, repeated,
	// or deloaded from (0 on StatusStart).
	PreviousLb float64
}

// Next returns the target weight for the upcoming session of a lift. It is a
// thin wrapper over NextPlan for callers that only need the number.
func Next(startingWeight, increment float64, history []SessionResult) float64 {
	return NextPlan(startingWeight, increment, history).WeightLb
}

// NextPlan computes the next session's weight and the reasoning behind it.
//
// startingWeight is the program's prescribed starting weight, returned when
// there is no history. increment is the per-session jump for this lift (see
// IncrementFor). history is chronological, oldest first.
//
// A failure streak is only counted at the most recent weight: a deload lowers
// the weight, so failures before the drop belong to the old weight and do not
// re-trigger a deload on the next attempt.
func NextPlan(startingWeight, increment float64, history []SessionResult) Plan {
	if len(history) == 0 {
		return Plan{WeightLb: startingWeight, Status: StatusStart}
	}

	last := history[len(history)-1]
	if last.Success {
		return Plan{
			WeightLb:   last.WeightLb + increment,
			Status:     StatusAdvance,
			PreviousLb: last.WeightLb,
		}
	}

	// Count consecutive trailing failures at the current working weight.
	var fails int32
	for i := len(history) - 1; i >= 0; i-- {
		h := history[i]
		if h.Success || h.WeightLb != last.WeightLb {
			break
		}
		fails++
	}

	if fails >= FailuresBeforeDeload {
		return Plan{
			WeightLb:     roundToBar(last.WeightLb * DeloadFactor),
			Status:       StatusDeload,
			FailureCount: fails,
			PreviousLb:   last.WeightLb,
		}
	}
	return Plan{
		WeightLb:     last.WeightLb,
		Status:       StatusHold,
		FailureCount: fails,
		PreviousLb:   last.WeightLb,
	}
}

// roundToBar snaps a weight to the nearest loadable barbell increment.
func roundToBar(w float64) float64 {
	return math.Round(w/BarIncrementLb) * BarIncrementLb
}
