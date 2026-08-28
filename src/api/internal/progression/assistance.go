package progression

// Double progression for assistance work.
//
// This lives beside the linear engine rather than inside it, because it is not
// the same rule and should not be able to become one by accident. The linear
// engine advances weight every session and deloads on a stall; that is right for
// a squat and wrong for a curl, which is why prescribe() ran no engine on
// assistance at all and simply carried forward whatever was last logged.
//
// What accessories actually progress on is reps. The lifter picks a range —
// 3×8-12 — and adds reps inside it week to week. When every set reaches the top
// of the range, the weight goes up and the reps go back to the bottom. That is
// the whole rule, and it has two properties worth stating:
//
//   - There is no deload. Ever. Stalling on a lateral raise is not a signal, and
//     cutting the weight on one is a solution to a problem nobody has.
//   - It only runs where the lifter asked for it. Without a range, assistance
//     behaves exactly as it did before: carry forward, change it by hand.
//
// Like the linear engine this is pure — history in, a number out, no I/O.

// AssistanceIncrement is how much a lift goes up when every set tops the range.
// The same 5 lb the main lifts use: it is the smallest change a standard barbell
// admits, and the dumbbell rack it usually means is no finer.
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
	// Status is StatusFixed when the weight is being carried forward and
	// StatusProgressing when the range moved it up.
	Status Status
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
// recent performance, or nil when there is none.
//
// A zero or inverted range is treated as no range at all, which collapses to the
// carry-forward behaviour. The database constrains both columns to be set
// together and the right way round, so this is a guard rather than a rule — but
// the engine is pure and does not get to assume its caller checked.
func NextAssistance(
	fallbackLb float64,
	repMin, repMax int32,
	last *AssistancePerformance,
) AssistancePlan {
	ranged := repMin > 0 && repMax >= repMin

	// Never performed: the stored weight is the starting point, and the bottom
	// of the range is what to chase first.
	if last == nil || len(last.Reps) == 0 {
		target := repMin
		if !ranged {
			target = 0
		}
		return AssistancePlan{WeightLb: fallbackLb, TargetReps: target, Status: StatusFixed}
	}

	if !ranged {
		// No range: do what you did last time, and change it when you feel like
		// it. Unchanged from before this engine existed.
		return AssistancePlan{
			WeightLb:   last.WeightLb,
			Status:     StatusFixed,
			PreviousLb: last.WeightLb,
		}
	}

	if toppedOut(last.Reps, repMax) {
		return AssistancePlan{
			WeightLb:   last.WeightLb + AssistanceIncrement,
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
