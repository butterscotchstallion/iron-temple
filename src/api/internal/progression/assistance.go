package progression

// Progression for assistance work: the main lifts' rule by default, double
// progression where the lifter asked for it.
//
// This file has now argued both sides, so it is worth recording why it changed
// its mind rather than quietly reversing.
//
// It used to hold that "a curl is not a squat: it does not advance five pounds a
// session, and stalling on one is not a signal worth deloading over", and so an
// accessory without a rep range carried its last weight forward and nothing ever
// moved it. That reasoning was sound about lifting and wrong about this app. The
// rep range was opt-in, defaulted off, and had no editor — so "carry forward"
// was not a considered default, it was the only behaviour reachable, and in
// practice no accessory here ever gained weight. An app that silently never
// progresses half a lifter's work is worse than one that pushes an accessory a
// little too hard: the second is visible and adjustable, the first looks like
// the lift is simply not working.
//
// So an accessory now runs the SAME engine as the prescribed lifts — advance on
// a session where every set hit its reps, hold after a miss, deload after three
// — which is what a lifter means by "make it increase like everything else".
// Two things temper it:
//
//   - The advance is the smallest jump the equipment admits, never a programme
//     pace. See assistanceStep.
//   - Bodyweight work stays bodyweight. A lift logged at 0 has nothing to add
//     to, and "push-ups plus five pounds" is a decision, not a consequence.
//
// The rep range remains, and is now the gentler option rather than the only
// working one: on a coarse grid — a pair of dumbbells stepping 10 lb — climbing
// reps inside 8-12 before the weight moves is a far smaller weekly increase than
// the grid's own step. A lifter who finds the default too aggressive turns it on
// per lift, and that path never deloads, because cutting the weight on a lateral
// raise is still a solution to a problem nobody has.
//
// Like the linear engine this is pure — history in, a number out, no I/O.

// AssistanceIncrement is how much a lift goes up when every set tops the range
// in a gym nobody has described: the same 5 lb the main lifts use, and the
// smallest change a standard barbell admits.
//
// It is a fallback and not the rule, which is what it used to claim twice over.
// The first note here read "the dumbbell rack it usually means is no finer",
// and that is only true of ONE bell — every weight in this app is the whole
// load, so a dumbbell accessory is two bells. The second read "the smallest
// change a standard barbell admits", which assumed the lightest plate anyone
// owns is a 2.5. Both are now questions the lifter's gym answers; see GymSteps
// and use LadderFor.
const AssistanceIncrement = 5.0

// AssistancePerformance is what a lift did the last time it was performed: the
// weight worked, and the reps actually completed on each set.
type AssistancePerformance struct {
	WeightLb float64
	// Reps holds one entry per logged set, in set order. Unlogged sets are not
	// included — a set nobody touched is not a set that fell short.
	Reps []int32
}

// AssistancePlan is the prescription for one assistance lift: what to load, and
// what to chase.
type AssistancePlan struct {
	WeightLb float64
	// TargetReps is the bottom of the range, not the top. A set is complete when
	// it reaches the bottom — that is what makes the range a range rather than a
	// rep target with extra words — and the weight moves when every set reaches
	// the top. Keeping "complete" and "progressed" as separate thresholds is
	// what lets a lifter finish a session at 8s without the app calling it a
	// failure, and still get the increase when they reach 12s.
	TargetReps int32
	// Status is StatusProgressing when a rep range moved the weight up,
	// StatusFixed when nothing moved it (no history, bodyweight work, or a range
	// still being climbed), and otherwise one of the linear engine's own —
	// start, advance, hold, deload — because the unranged path IS that engine.
	Status Status
	// FailureCount is the consecutive trailing failures at the working weight,
	// for the "one more miss and this deloads" copy. Only the linear path sets
	// it; a rep range never deloads, so it has nothing to count toward.
	FailureCount int
	// PreviousLb is the weight last worked, 0 when the lift has no history.
	PreviousLb float64
}

// StatusProgressing means a rep range advanced the weight: every set reached the
// top of the range last time, so this session is heavier and back at the bottom.
const StatusProgressing Status = "progressing"

// StatusFixed means no engine moved this weight — it is what was last logged,
// or the stored fallback for a lift never performed. The status assistance work
// carries whenever it has no rep range.
const StatusFixed Status = "fixed"

// NextAssistance computes the next session for one assistance lift.
//
// fallbackLb is the weight stored on the assistance row, used only when the lift
// has never been performed. repMin and repMax bound the range. last is the most
// recent performance, or nil when there is none. history is the lift's sessions
// oldest-first, read only on the unranged path, where it drives the same linear
// engine the prescribed lifts use. l is the lift's ladder.
//
// A zero or inverted range is treated as no range at all, which collapses to the
// linear path. The database constrains both columns to be set together and the
// right way round, so this is a guard rather than a rule — but the engine is
// pure and does not get to assume its caller checked.
func NextAssistance(
	fallbackLb float64,
	repMin, repMax int32,
	last *AssistancePerformance,
	history []SessionResult,
	l Ladder,
) AssistancePlan {
	ranged := repMin > 0 && repMax >= repMin

	// No range: the main lifts' rule, which is what an accessory gets unless the
	// lifter has asked for something gentler.
	if !ranged {
		return linearAssistance(fallbackLb, last, history, l)
	}

	// Never performed: the stored weight is the starting point, and the bottom
	// of the range is what to chase first.
	if last == nil || len(last.Reps) == 0 {
		return AssistancePlan{
			WeightLb:   fallbackLb,
			TargetReps: repMin,
			Status:     StatusFixed,
		}
	}

	if toppedOut(last.Reps, repMax) {
		return AssistancePlan{
			WeightLb:   last.WeightLb + assistanceStep(l),
			TargetReps: repMin,
			Status:     StatusProgressing,
			PreviousLb: last.WeightLb,
		}
	}

	// Short of the top somewhere: same weight, keep adding reps.
	return AssistancePlan{
		WeightLb:   last.WeightLb,
		TargetReps: repMin,
		Status:     StatusFixed,
		PreviousLb: last.WeightLb,
	}
}

// linearAssistance runs the prescribed lifts' engine on an accessory, with the
// two adjustments that make it honest for accessory work.
//
// The advance is the ladder's Step rather than its Increment. Increment is the
// programme's pace — +5 on a squat, +10 on a deadlift — and an accessory is not
// on a programme; what it can do is the least its equipment allows. On a barbell
// those are the same 5 lb, so nothing changes for a barbell curl; on a pair of
// dumbbells the Step IS 10, so nothing changes there either. They come apart
// only where a lifter owns finer plates, and there the accessory should take the
// 2.5 rather than the squat's 5.
//
// Bodyweight work does not advance. A lift logged at 0 lb has nothing to add to,
// and a successful set of push-ups must not come back prescribed at 5 lb — that
// is a different exercise, and choosing it is the lifter's. Reported as
// StatusFixed, which says exactly what happened: no engine moved this weight.
// Entering a load is what opts the lift into progressing.
//
// The weight last actually worked displaces the stored fallback as the starting
// point, which is the carry-forward rule surviving inside the engine that
// replaced it. It matters because the two read different pasts: history counts
// only sessions that FINISHED (or aged out), deliberately, so an abandoned
// workout cannot drive progression — while last is whatever was logged most
// recently, finished or not. Without this, logging a curl at 65 in a session
// still in progress and then looking at the next prescription would show the
// stored fallback, which for an accessory added at bodyweight is 0: the app
// would appear to forget the weight you were holding ten minutes ago.
func linearAssistance(
	fallbackLb float64,
	last *AssistancePerformance,
	history []SessionResult,
	l Ladder,
) AssistancePlan {
	step := Ladder{Increment: assistanceStep(l), Step: assistanceStep(l)}

	start := fallbackLb
	carried := false
	if last != nil && last.WeightLb > 0 {
		start = last.WeightLb
		carried = true
	}

	p := NextPlan(start, step, history)

	if p.Status == StatusAdvance && p.PreviousLb == 0 {
		return AssistancePlan{WeightLb: 0, Status: StatusFixed}
	}

	// No finished history, but a weight was carried in: nothing computed this
	// number, it is simply what was last lifted. "fixed" says that; "start"
	// would claim the lift has never been done.
	if p.Status == StatusStart && carried {
		return AssistancePlan{WeightLb: p.WeightLb, Status: StatusFixed, PreviousLb: start}
	}

	return AssistancePlan{
		WeightLb:     p.WeightLb,
		Status:       p.Status,
		FailureCount: p.FailureCount,
		PreviousLb:   p.PreviousLb,
	}
}

// toppedOut reports whether every logged set reached the top of the range.
//
// Every set, not the average and not the last one: the point of the range is to
// carry the top rep count across the whole prescription before the weight moves.
// One set of 12 and two of 9 is not a session that earned an increase.
func toppedOut(reps []int32, repMax int32) bool {
	if len(reps) == 0 {
		return false
	}
	for _, r := range reps {
		if r < repMax {
			return false
		}
	}
	return true
}

// assistanceStep is how much a ranged lift goes up when every set tops out: the
// smallest change its equipment admits in this lifter's gym, which is the whole
// reason Ladder carries a Step distinct from its Increment. A curl does not
// advance at the pace of a squat; it advances by the least the rack allows.
//
// A zero ladder — one a caller built itself rather than taking from LadderFor —
// falls back to the 5 this function replaced, so a missed call site keeps the
// old behaviour rather than prescribing a weight of zero.
func assistanceStep(l Ladder) float64 {
	if l.Step <= 0 {
		return AssistanceIncrement
	}
	return l.Step
}
