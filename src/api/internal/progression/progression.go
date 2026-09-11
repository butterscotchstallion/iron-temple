// Package progression computes the target weight for the next session of a
// lift from its performance history. All programs are linear (see
// docs/implementation-plan.md); they differ only in set count, not in how
// weight advances, so a single engine serves all of them.
//
// Rules (pounds):
//   - Advance by a per-lift increment after a successful session:
//     +10 lb on the deadlift, +10 lb on a dumbbell lift, +5 lb on every other
//     lift. See Ladder for why the dumbbell number is what it is.
//   - Repeat the same weight after a failed session.
//   - After 3 consecutive failed sessions at the same weight, deload to 90%
//     of that weight, snapped to what the equipment can build, giving a fresh
//     run-up.
//   - After time away from training, optionally deload by 10% per week off,
//     capped at 50%. This one is the lifter's to ask for; see layoff.go.
//
// Both the advance and the snap come off the lift's Ladder, because neither is
// a property of the program: a pair of dumbbells has nothing between 10 lb
// however the sets and reps are arranged.
//
// The engine is pure: it takes a history and returns a number, with no I/O.
// The data layer supplies the history; the API layer maps exercises to
// ladders and rounds for display.
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
	// DumbbellIncrementLb is the smallest change a pair of dumbbells admits.
	//
	// Ten, not five, because every weight in this app is the WHOLE load and a
	// dumbbell lift is two bells. A rack steps 5 lb a bell, so the smallest
	// move a lifter can actually make is 10 lb on the pair. Asking for 5 is
	// asking for a bell that is not on the rack.
	DumbbellIncrementLb = 10.0
)

// deadliftName is the seeded exercise name that progresses at the faster rate.
const deadliftName = "Deadlift"

// equipmentDumbbell is the exercises.equipment value for a dumbbell lift, as
// 0009 constrained it.
const equipmentDumbbell = "dumbbell"

// Ladder is the set of jumps a lift's equipment can make: how much it goes up
// after a good session, and the smallest change it admits at all.
//
// The two are not the same number and never were. A deadlift advances 10 lb a
// session but is still loaded on a bar that moves in 5s, so its deload snaps to
// 5. Keeping them apart is what lets equipment decide reachability while the
// programme decides pace.
//
// It exists because "+5 lb" was baked in as a universal truth about barbells,
// and 0018 put a dumbbell press in a program for the first time. On the pair,
// +5 lb is half a bell — a weight no rack can build, prescribed every session.
type Ladder struct {
	// Increment is the per-session advance after a successful session.
	Increment float64
	// Step is the smallest change the equipment admits. Deloads and layoff
	// cuts snap to it; it is never zero.
	Step float64
}

// BarLadder is the ladder for anything loaded on a barbell, which is every
// lift the app knew about before dumbbells were prescribed.
var BarLadder = Ladder{Increment: IncrementDefault, Step: BarIncrementLb}

// LadderFor returns the jumps a lift can make, from its (seeded) exercise name
// and its equipment. Unknown names and unknown equipment take the bar's.
//
// Equipment is checked first and wins outright, because it answers a stricter
// question. The deadlift's +10 is a claim about how fast the movement should be
// pushed; a pair of dumbbells stepping 10 is a claim about which weights exist
// at all. A preference can be overruled by a constraint, never the other way
// round — so a dumbbell deadlift, if a program ever prescribes one, takes the
// pair's grid rather than the bar's.
func LadderFor(exerciseName, equipment string) Ladder {
	if equipment == equipmentDumbbell {
		return Ladder{Increment: DumbbellIncrementLb, Step: DumbbellIncrementLb}
	}
	if exerciseName == deadliftName {
		return Ladder{Increment: IncrementDeadlift, Step: BarIncrementLb}
	}
	return BarLadder
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
	// StatusLayoff means the weight was cut for time away from training rather
	// than for anything that happened in a session. Only ApplyLayoff produces
	// it, and only when the lifter asked for it — see layoff.go.
	StatusLayoff Status = "layoff"
)

// Plan is the engine's decision for the next session: the target weight plus the
// reasoning behind it.
type Plan struct {
	// WeightLb is the target weight for the upcoming session.
	WeightLb float64
	// Status is why this weight was chosen.
	Status Status
	// FailureCount is the number of consecutive trailing failures at the working
	// weight (0 on StatusStart and StatusAdvance). int, matching
	// progressionInfoDTO.FailureCount — see the note there for why these
	// engine-domain counts are not int32.
	FailureCount int
	// PreviousLb is the weight just worked — the weight advanced past, repeated,
	// or deloaded from (0 on StatusStart).
	PreviousLb float64
	// LayoffPct is the fraction taken off for time away from training, as a
	// fraction (0.30 is 30%). 0 unless Status is StatusLayoff; NextPlan never
	// sets it, because the calendar is not something it is shown.
	LayoffPct float64
}

// Next returns the target weight for the upcoming session of a lift. It is a
// thin wrapper over NextPlan for callers that only need the number.
func Next(startingWeight float64, l Ladder, history []SessionResult) float64 {
	return NextPlan(startingWeight, l, history).WeightLb
}

// NextPlan computes the next session's weight and the reasoning behind it.
//
// startingWeight is the program's prescribed starting weight, returned when
// there is no history. l is the ladder this lift's equipment admits (see
// LadderFor). history is chronological, oldest first.
//
// A failure streak is only counted at the most recent weight: a deload lowers
// the weight, so failures before the drop belong to the old weight and do not
// re-trigger a deload on the next attempt.
func NextPlan(startingWeight float64, l Ladder, history []SessionResult) Plan {
	if len(history) == 0 {
		return Plan{WeightLb: startingWeight, Status: StatusStart}
	}

	last := history[len(history)-1]
	if last.Success {
		return Plan{
			WeightLb:   last.WeightLb + l.Increment,
			Status:     StatusAdvance,
			PreviousLb: last.WeightLb,
		}
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
		return Plan{
			WeightLb:     roundToStep(last.WeightLb*DeloadFactor, l.Step),
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

// roundToStep snaps a weight to the nearest change the equipment admits.
//
// A zero or negative step is treated as the bar's. Callers get their step from
// a Ladder, which never carries one — but this function divides by it, and a
// silent Inf reaching a prescription is worse than quietly using the number
// that was the only option before ladders existed.
func roundToStep(w, step float64) float64 {
	if step <= 0 {
		step = BarIncrementLb
	}
	return math.Round(w/step) * step
}
