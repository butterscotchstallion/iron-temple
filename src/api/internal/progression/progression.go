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

// Next returns the target weight for the upcoming session of a lift.
//
// startingWeight is the program's prescribed starting weight, returned when
// there is no history. increment is the per-session jump for this lift (see
// IncrementFor). history is chronological, oldest first.
//
// A failure streak is only counted at the most recent weight: a deload lowers
// the weight, so failures before the drop belong to the old weight and do not
// re-trigger a deload on the next attempt.
func Next(startingWeight, increment float64, history []SessionResult) float64 {
	if len(history) == 0 {
		return startingWeight
	}

	last := history[len(history)-1]
	if last.Success {
		return last.WeightLb + increment
	}

	// Count consecutive trailing failures at the current working weight.
	fails := 0
	for i := len(history) - 1; i >= 0; i-- {
		h := history[i]
		if h.Success || h.WeightLb != last.WeightLb {
			break
		}
		fails++
	}

	if fails >= FailuresBeforeDeload {
		return roundToBar(last.WeightLb * DeloadFactor)
	}
	return last.WeightLb
}

// roundToBar snaps a weight to the nearest loadable barbell increment.
func roundToBar(w float64) float64 {
	return math.Round(w/BarIncrementLb) * BarIncrementLb
}
