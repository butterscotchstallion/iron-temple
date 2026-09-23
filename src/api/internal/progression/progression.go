// Package progression computes the target weight for the next session of a
// lift from its performance history. All programs are linear (see
// docs/implementation-plan.md); they differ only in set count, not in how
// weight advances, so a single engine serves all of them.
//
// Rules (pounds):
//   - Advance by a per-lift increment after a successful session: +10 lb on the
//     deadlift and +5 lb on every other lift, each rounded up to something the
//     lifter's own equipment can build. See Ladder and GymSteps.
//   - Repeat the same weight after a failed session.
//   - After 3 consecutive failed sessions at the same weight, deload to 90%
//     of that weight, snapped to what the equipment can build, giving a fresh
//     run-up.
//   - After time away from training, optionally deload by 10% per week off,
//     capped at 50%. This one is the lifter's to ask for; see layoff.go.
//
// Both the advance and the snap come off the lift's Ladder, because neither is
// a property of the program: a pair of dumbbells has nothing between 10 lb
// however the sets and reps are arranged — and nothing between 5 lb if the rack
// steps 2.5 a bell, which is why the grid is read from the lifter's gym rather
// than assumed.
//
// The engine is pure: it takes a history and returns a number, with no I/O.
// The data layer supplies the history and the gym; the API layer maps exercises
// to ladders and rounds for display.
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
	//
	// The standard bar, not every bar. A lifter who owns 1.25s moves in 2.5s,
	// which GymSteps carries and this constant is the fallback for.
	BarIncrementLb = 5.0
	// DumbbellIncrementLb is the smallest change a pair of dumbbells admits on
	// the rack this app assumes when told about no other.
	//
	// Ten, not five, because every weight in this app is the WHOLE load and a
	// dumbbell lift is two bells. A rack steps 5 lb a bell, so the smallest
	// move a lifter can actually make is 10 lb on the pair. Asking for 5 is
	// asking for a bell that is not on the rack — unless the rack goes in 2.5s,
	// in which case 5 is exactly right and GymSteps is what says so.
	DumbbellIncrementLb = 10.0
	// MachineIncrementLb, CableIncrementLb and BandIncrementLb are the
	// smallest change those admit when the lifter has not said otherwise.
	//
	// Written out separately rather than aliased to BarIncrementLb or to each
	// other, even though all four are 5. They are not the same fact: the bar's
	// is twice the lightest plate on a standard rack, a machine's is the gap
	// between two holes somebody drilled, a cable's is the next plate in the
	// stack, and a band's is however the manufacturer graded the set. Landing
	// on 5 is a coincidence of American gyms, not a reason to make one of them
	// the other — and each of these mirrors a column default in 0028, which is
	// where a lifter overrides it.
	MachineIncrementLb = 5.0
	CableIncrementLb   = 5.0
	BandIncrementLb    = 5.0
)

// deadliftName is the seeded exercise name that progresses at the faster rate.
const deadliftName = "Deadlift"

// The exercises.equipment values that get a grid of their own, out of the seven
// 0009 constrained and 0023 extended ('barbell', 'dumbbell', 'machine',
// 'cable', 'bodyweight', 'band', 'other').
//
// Only the four that stepFor branches on are named. 'barbell', 'bodyweight' and
// 'other' have no constant because nothing compares against them — they are
// what the default arm is for, and a constant no branch reads is a claim that
// some code depends on the spelling when none does.
const (
	equipmentDumbbell = "dumbbell"
	equipmentMachine  = "machine"
	equipmentCable    = "cable"
	equipmentBand     = "band"
)

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

// BarLadder is the ladder for anything loaded on a barbell in a gym that has
// not been described — the standard rack, whose lightest plate is a 2.5 and
// which therefore moves in 5s. It is what LadderFor returns for a barbell lift
// under a zero GymSteps.
var BarLadder = Ladder{Increment: IncrementDefault, Step: BarIncrementLb}

// GymSteps is the smallest weight change each kind of equipment admits in ONE
// lifter's gym, as WHOLE load — the pair, not the bell; the bar and both sides
// of it, not one plate.
//
// It exists because Step was written as a constant and is not one. "A barbell
// moves in 5s" is really "a barbell moves in twice the lightest plate you own",
// and "a pair of dumbbells moves in 10s" is really "twice whatever your rack
// steps by". Both facts are recorded per lifter (user_plates since 0013,
// user_gym.dumbbell_step_lb since 0020) and neither was reaching the engine, so
// a lifter with finer equipment was prescribed jumps coarser than their gym
// actually makes — most visibly on assistance work, which advances by exactly
// this number.
//
// A zero field means "not configured" and falls back to the constant that was
// hardcoded before this type existed. That is deliberate and load-bearing: the
// engine is pure and does not get to assume its caller looked the gym up, and a
// missed call site should prescribe what it always did rather than divide by
// zero. Same defensive shape as roundToStep and assistanceStep.
type GymSteps struct {
	// BarLb is the smallest change a loaded barbell admits: twice the lightest
	// plate owned. Zero when unknown.
	BarLb float64
	// DumbbellLb is the smallest change a PAIR of dumbbells admits: twice the
	// rack's per-bell step. Zero when unknown.
	DumbbellLb float64
	// MachineLb is the gap between two holes on a selectorized stack — 10 or
	// 15 on most, sometimes with a 2.5 add-on. Zero when unknown.
	MachineLb float64
	// CableLb is the next plate up on a cable stack, commonly 5. Zero when
	// unknown.
	CableLb float64
	// BandLb is the jump between two bands in a graded set, whatever the
	// manufacturer decided it is. Zero when unknown.
	BandLb float64
	// Bodyweight has no field. The load on a weighted dip is a plate on a belt
	// or a bell between the feet, so it is already described by BarLb and
	// DumbbellLb; there is no third inventory to record. It takes the bar's,
	// as it always has.
}

// orDefault is the "zero means not configured" rule of GymSteps, written once.
//
// Guarding on > 0 rather than != 0 on purpose: a negative step would be as
// unusable as a zero one, and the database's CHECK (> 0) is the same sentence
// said in the same direction. The engine is pure and does not get to assume its
// caller validated anything.
func orDefault(configured, fallback float64) float64 {
	if configured > 0 {
		return configured
	}
	return fallback
}

// stepFor picks the grid a lift is loaded on, falling back to the constants.
//
// A switch over the catalogue rather than the two-way if this used to be. The
// if was not choosing between dumbbells and barbells — it was choosing between
// dumbbells and EVERYTHING, and so a machine, a cable and a band were all being
// told they moved in twice the lightest plate the lifter owns. That is a
// sentence about a barbell, and it is false about a pin in a stack: see 0028.
//
// The default arm is deliberately wide. 'bodyweight' and 'other' take the bar's
// because their load, when they have one, is plates or bells — and any kind the
// catalogue grows later lands here too, prescribing what it would have
// prescribed before the kind existed rather than zero.
func (g GymSteps) stepFor(equipment string) float64 {
	switch equipment {
	case equipmentDumbbell:
		return orDefault(g.DumbbellLb, DumbbellIncrementLb)
	case equipmentMachine:
		return orDefault(g.MachineLb, MachineIncrementLb)
	case equipmentCable:
		return orDefault(g.CableLb, CableIncrementLb)
	case equipmentBand:
		return orDefault(g.BandLb, BandIncrementLb)
	default:
		return orDefault(g.BarLb, BarIncrementLb)
	}
}

// LadderFor returns the jumps a lift can make, from its (seeded) exercise name,
// its equipment, and the gym it is being lifted in. Unknown names and unknown
// equipment take the bar's.
//
// Equipment decides the grid and wins outright, because it answers a stricter
// question. The deadlift's +10 is a claim about how fast the movement should be
// pushed; a pair of dumbbells stepping 10 is a claim about which weights exist
// at all. A preference can be overruled by a constraint, never the other way
// round — so a dumbbell deadlift, if a program ever prescribes one, takes the
// pair's grid rather than the bar's.
//
// What used to be three branches is now one, because the branches were all
// saying the same thing in different arithmetic: take the programme's pace and
// round it up to something the equipment can build. That reproduces every
// number this function used to return — a bar stepping 5 gives +5 and the
// deadlift's +10; a pair stepping 10 gives +10, which is the whole reason
// DumbbellIncrementLb was introduced — and it keeps producing the right one
// when the grid is finer. A rack that steps 2.5 a bell moves the pair in 5s, so
// the seeded dumbbell press advances 5 lb a session rather than 10.
func LadderFor(exerciseName, equipment string, g GymSteps) Ladder {
	step := g.stepFor(equipment)

	// Pace is a preference, so it is chosen by the movement...
	inc := IncrementDefault
	if exerciseName == deadliftName {
		inc = IncrementDeadlift
	}
	// ...and then made reachable, because a preference no rack can build is not
	// a prescription.
	return Ladder{Increment: advanceAtLeast(inc, step), Step: step}
}

// advanceAtLeast rounds a desired advance UP to a multiple of the smallest
// change the equipment admits.
//
// Up rather than to-nearest: rounding down can reach zero, and an advance of
// zero is a lift that never progresses dressed up as one that does. A gym so
// coarse that +5 is unbuildable gets the next weight that exists, which is the
// honest answer — it is what the dumbbell pair has always done.
//
// The epsilon absorbs float noise in the division so a quotient that is exact
// in decimal does not ceil to one step too many. Both inputs arrive from
// NUMERIC(5,2) by way of float64, so 5/2.5 can land a hair either side of 2,
// and on the high side that would advance 7.5 instead of 5. Same tolerance the
// plate loader applies for the same reason.
func advanceAtLeast(want, step float64) float64 {
	if step <= 0 {
		return want
	}
	return math.Ceil(want/step-stepEpsilon) * step
}

// stepEpsilon is the float-noise tolerance in advanceAtLeast. Far below any
// real weight — the finest plate anyone racks is a quarter pound — and far
// above the error in dividing two two-decimal numbers.
const stepEpsilon = 1e-9

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
//
// A lift worked at 0 lb never advances. See the guard below.
func NextPlan(startingWeight float64, l Ladder, history []SessionResult) Plan {
	if len(history) == 0 {
		return Plan{WeightLb: startingWeight, Status: StatusStart}
	}

	last := history[len(history)-1]
	if last.Success {
		// A lift logged at 0 has nothing to add to. Bodyweight work stays
		// bodyweight and band work stays band work — "push-ups plus five
		// pounds" is a decision, not a consequence, and a banded lateral walk
		// that went well must not come back prescribed at 5 lb.
		//
		// This rule is older than this guard: assistance.go has enforced it on
		// both of its paths since accessories started advancing at all. It
		// belongs HERE, in the one engine both paths run through, and it was
		// missing only because it was unreachable — every seeded program
		// started every lift above zero, so the prescribed path had never been
		// handed a zero to advance. 0023 seeds the first program that does, in
		// band work and a bodyweight back extension.
		//
		// StatusFixed rather than StatusAdvance, because nothing advanced. It
		// is the status assistance work already carries for exactly this, and
		// it is already in the API's status enum, so no contract moves.
		//
		// The guard is on what was WORKED, not on what was prescribed: log a
		// plate against one of these and it starts progressing from there like
		// any other lift. Entering a load is what opts a lift in.
		if last.WeightLb <= 0 {
			return Plan{WeightLb: 0, Status: StatusFixed}
		}
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
			WeightLb:     deloadFrom(last.WeightLb, l.Step),
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

// deloadFrom is the weight a stall drops to: DeloadFactor of what was worked,
// snapped to the grid, and guaranteed to be LOWER than what was worked.
//
// The guarantee is the point, and snapping alone does not give it. Ten percent
// off a light weight can be less than half a step, so it rounds back to the
// weight that just failed three times — 40 lb on a pair of dumbbells goes to 36
// and snaps to 40, prescribing the identical weight as a fresh run-up, forever.
// The coarser the grid relative to the weight the likelier that is, which is why
// it shows up on accessories and on the seeded dumbbell press at 30 lb rather
// than on a squat at 300.
//
// So when the snap fails to descend, take one step off instead. That is a deeper
// cut than 10% — a third, at 30 lb on a 10 lb grid — but it is the only lower
// weight the equipment can build, and the alternative is not a gentler deload,
// it is no deload at all.
//
// Clamped at zero, which is where a bodyweight lift already sits.
func deloadFrom(workedLb, step float64) float64 {
	w := roundToStep(workedLb*DeloadFactor, step)
	if w >= workedLb {
		w = workedLb - step
	}
	return math.Max(w, 0)
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
