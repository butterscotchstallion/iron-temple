package racked

import (
	"testing"
	"time"
)

// priorBest is a baseline saying exercise 1 has been lifted before.
//
// Every test below that is about RECORDS needs one, because a lift absent from
// the baseline has no history and its first set is a first time rather than a
// record — which is what the tests at the bottom of this group cover. An empty
// Baseline is a brand-new lifter, not a convenient zero.
func priorBest(weight, e1rm float64) Baseline {
	return Baseline{
		BestWeight: map[int32]float64{1: weight},
		BestE1RM:   map[int32]float64{1: e1rm},
	}
}

// The baseline is what separates a record from a maximum: without it every lift
// in a lifter's second year would open with a false PR.
func TestPersonalRecordsRespectBaseline(t *testing.T) {
	sets := mkSets(1, day(2026, time.March, 2), 1, "Squat", 5, 5, 200)
	if got, _ := personalRecords(groupSessions(sets), priorBest(225, 263)); len(got) != 0 {
		t.Fatalf("got %d PRs, want none below the standing best", len(got))
	}
}

// A 5x5 at a new weight is one achievement. Reporting it five times would bury
// every other lift in the period.
func TestPersonalRecordsOnePerLiftPerSession(t *testing.T) {
	sets := mkSets(1, day(2026, time.March, 2), 1, "Squat", 5, 5, 200)
	got, _ := personalRecords(groupSessions(sets), priorBest(195, 220))
	if len(got) != 1 {
		t.Fatalf("got %d PRs, want 1", len(got))
	}
	if got[0].Kind != PRWeight || got[0].WeightLb != 200 {
		t.Fatalf("PR = %+v, want a 200 lb weight record", got[0])
	}
}

// Same bar, more reps, is progress a 5x5 makes constantly and the plate count
// never shows.
func TestPersonalRecordsEstimatedMax(t *testing.T) {
	base := priorBest(200, 220)                                        // 200 x 3
	sets := mkSets(1, day(2026, time.March, 2), 1, "Squat", 1, 5, 200) // e1RM 233
	got, _ := personalRecords(groupSessions(sets), base)

	if len(got) != 1 {
		t.Fatalf("got %d PRs, want 1", len(got))
	}
	if got[0].Kind != PREstimated {
		t.Fatalf("kind = %q, want %q", got[0].Kind, PREstimated)
	}
	if got[0].Reps != 5 {
		t.Fatalf("reps = %d, want the 5-rep set", got[0].Reps)
	}
}

// A heavier bar implies a higher estimated max; reporting both would be one
// achievement told twice.
func TestPersonalRecordsWeightSuppressesEstimated(t *testing.T) {
	sets := mkSets(1, day(2026, time.March, 2), 1, "Squat", 1, 5, 300)
	got, _ := personalRecords(groupSessions(sets), priorBest(250, 275))
	if len(got) != 1 || got[0].Kind != PRWeight {
		t.Fatalf("got %+v, want a single weight record", got)
	}
}

// The running best must advance through the period, or every session after the
// first would keep clearing the same stale baseline.
func TestPersonalRecordsAdvanceWithinPeriod(t *testing.T) {
	var sets []Set
	sets = append(sets, mkSets(1, day(2026, time.March, 2), 1, "Squat", 1, 5, 200)...)
	sets = append(sets, mkSets(2, day(2026, time.March, 9), 1, "Squat", 1, 5, 195)...)
	sets = append(sets, mkSets(3, day(2026, time.March, 16), 1, "Squat", 1, 5, 205)...)

	got, _ := personalRecords(groupSessions(sets), priorBest(190, 210))
	if len(got) != 2 {
		t.Fatalf("got %d PRs, want 2 (the 195 lb session is not a record)", len(got))
	}
	if got[0].WeightLb != 200 || got[1].WeightLb != 205 {
		t.Fatalf("PRs = %v, want 200 then 205", []float64{got[0].WeightLb, got[1].WeightLb})
	}
}

// The whole point. A lifter's first workout beat nothing, and calling six lifts
// six personal records is what made the seventh mean nothing.
func TestFirstEverSessionSetsNoRecords(t *testing.T) {
	var sets []Set
	sets = append(sets, mkSets(1, day(2026, time.March, 2), 1, "Squat", 5, 5, 95)...)
	sets = append(sets, mkSets(1, day(2026, time.March, 2), 2, "Bench Press", 5, 5, 65)...)

	prs, firsts := personalRecords(groupSessions(sets), Baseline{})
	if len(prs) != 0 {
		t.Fatalf("got %d PRs, want none — there was nothing to beat", len(prs))
	}
	if len(firsts) != 2 {
		t.Fatalf("got %d first times, want one per lift", len(firsts))
	}
	// Alphabetical, because sessionTops sorts that way and the report is stable.
	if firsts[0].ExerciseName != "Bench Press" || firsts[0].WeightLb != 65 {
		t.Fatalf("first = %+v, want the 65 lb Bench Press", firsts[0])
	}
	if firsts[0].Reps != 5 {
		t.Fatalf("reps = %d, want the 5 logged", firsts[0].Reps)
	}
}

// A first time happens once. The session after it has a history to beat, so the
// same lift going up is a record — otherwise a linear program would report a
// first time every week.
func TestAFirstTimeBecomesARecordNextSession(t *testing.T) {
	var sets []Set
	sets = append(sets, mkSets(1, day(2026, time.March, 2), 1, "Squat", 5, 5, 95)...)
	sets = append(sets, mkSets(2, day(2026, time.March, 9), 1, "Squat", 5, 5, 100)...)

	prs, firsts := personalRecords(groupSessions(sets), Baseline{})
	if len(firsts) != 1 || firsts[0].WeightLb != 95 {
		t.Fatalf("first times = %+v, want only the opening 95", firsts)
	}
	if len(prs) != 1 || prs[0].ValueLb != 100 {
		t.Fatalf("PRs = %+v, want only the 100 lb record", prs)
	}
	// The mark it beat is the first time's weight, not zero: a first time is not
	// a record, but it is still history.
	if prs[0].PreviousLb != 95 {
		t.Fatalf("previous = %v, want 95", prs[0].PreviousLb)
	}
}

// A lift loaded with no plates has a legitimate best of zero, so "best is 0"
// cannot be read as "never done it" — presence in the baseline decides. Without
// that distinction a chin-up is a first time every single session, forever.
func TestABodyweightLiftIsAFirstTimeOnlyOnce(t *testing.T) {
	var sets []Set
	sets = append(sets, mkSets(1, day(2026, time.March, 2), 3, "Chin-up", 3, 8, 0)...)
	sets = append(sets, mkSets(2, day(2026, time.March, 9), 3, "Chin-up", 3, 8, 0)...)

	prs, firsts := personalRecords(groupSessions(sets), Baseline{})
	if len(firsts) != 1 {
		t.Fatalf("got %d first times for one lift, want 1", len(firsts))
	}
	if len(prs) != 0 {
		t.Fatalf("got %+v, want no records — the bar never got heavier", prs)
	}
}

// The baseline carrying a lift at zero is a lifter who HAS done it, which is
// exactly the row RackedExerciseBaseline returns for bodyweight work.
func TestABaselineOfZeroIsStillAHistory(t *testing.T) {
	sets := mkSets(1, day(2026, time.March, 2), 1, "Chin-up", 3, 8, 0)
	base := Baseline{
		BestWeight: map[int32]float64{1: 0},
		BestE1RM:   map[int32]float64{1: 0},
	}
	_, firsts := personalRecords(groupSessions(sets), base)
	if len(firsts) != 0 {
		t.Fatalf("got %+v, want no first time for a lift already in the baseline", firsts)
	}
}

func TestHeaviestSetTieBreaks(t *testing.T) {
	var sets []Set
	sets = append(sets, mkSets(1, day(2026, time.March, 2), 1, "Squat", 1, 3, 300)...)
	sets = append(sets, mkSets(2, day(2026, time.March, 9), 1, "Squat", 1, 5, 300)...)

	got := heaviestSet(sets)
	if got == nil || got.Reps != 5 {
		t.Fatalf("heaviest = %+v, want the 5-rep set at equal weight", got)
	}
}

func TestHeaviestSetEmpty(t *testing.T) {
	if got := heaviestSet(nil); got != nil {
		t.Fatalf("heaviest = %+v, want nil", got)
	}
}

func TestFastestSession(t *testing.T) {
	t.Run("picks the shortest finished session", func(t *testing.T) {
		var sets []Set
		sets = append(sets, finish(mkSets(1, day(2026, time.March, 2), 1, "Squat", 1, 5, 100), time.Hour)...)
		sets = append(sets, finish(mkSets(2, day(2026, time.March, 9), 1, "Squat", 1, 5, 100), 40*time.Minute)...)

		got := fastestSession(groupSessions(sets))
		if got == nil || got.Duration != 40*time.Minute {
			t.Fatalf("fastest = %+v, want 40m", got)
		}
	})

	t.Run("ignores sessions never finished", func(t *testing.T) {
		sets := mkSets(1, day(2026, time.March, 2), 1, "Squat", 1, 5, 100)
		if got := fastestSession(groupSessions(sets)); got != nil {
			t.Fatalf("fastest = %+v, want nil without a finish stamp", got)
		}
	})

	t.Run("ignores sessions past the trust window", func(t *testing.T) {
		// A tab left open overnight would otherwise become the longest session
		// on record and drag the archetype's average with it.
		sets := finish(mkSets(1, day(2026, time.March, 2), 1, "Squat", 1, 5, 100), 13*time.Hour)
		if got := fastestSession(groupSessions(sets)); got != nil {
			t.Fatalf("fastest = %+v, want nil beyond %v", got, maxSessionDuration)
		}
	})
}

func TestDeloads(t *testing.T) {
	squat := func(id int32, on time.Time, w float64) []Set {
		return mkSets(id, on, 1, "Squat", 1, 5, w)
	}

	t.Run("detects a drop and its recovery", func(t *testing.T) {
		var sets []Set
		sets = append(sets, squat(1, day(2026, time.March, 2), 225)...)
		sets = append(sets, squat(2, day(2026, time.March, 9), 200)...)
		sets = append(sets, squat(3, day(2026, time.March, 16), 225)...)

		got := deloads(groupSessions(sets))
		if len(got) != 1 {
			t.Fatalf("got %d deloads, want 1", len(got))
		}
		if got[0].FromLb != 225 || got[0].ToLb != 200 {
			t.Fatalf("deload = %+v, want 225 -> 200", got[0])
		}
		if !got[0].Recovered || !got[0].RecoveredOn.Equal(day(2026, time.March, 16)) {
			t.Fatalf("deload = %+v, want recovery on the 16th", got[0])
		}
	})

	t.Run("reports a drop not yet answered", func(t *testing.T) {
		var sets []Set
		sets = append(sets, squat(1, day(2026, time.March, 2), 225)...)
		sets = append(sets, squat(2, day(2026, time.March, 9), 200)...)

		got := deloads(groupSessions(sets))
		if len(got) != 1 || got[0].Recovered {
			t.Fatalf("deloads = %+v, want one unrecovered", got)
		}
	})

	t.Run("ignores a drop below a bar increment", func(t *testing.T) {
		var sets []Set
		sets = append(sets, squat(1, day(2026, time.March, 2), 225)...)
		sets = append(sets, squat(2, day(2026, time.March, 9), 222.5)...)

		if got := deloads(groupSessions(sets)); len(got) != 0 {
			t.Fatalf("deloads = %+v, want none under a full increment", got)
		}
	})
}

// A deload is a claim about a progression, and assistance has no progression
// behind it — program_day_assistance carries a plain weight column with no
// engine on it. Picking up the 15s because the 20s were taken is not stalling
// out, and calling it one tells the lifter something untrue about a fine month.
func TestDeloadsIgnoreAssistanceWork(t *testing.T) {
	var sets []Set
	sets = append(sets, assist(mkSets(1, day(2026, time.March, 2), 9, "Lateral Raise", 1, 10, 20))...)
	sets = append(sets, assist(mkSets(2, day(2026, time.March, 9), 9, "Lateral Raise", 1, 10, 10))...)

	if got := deloads(groupSessions(sets)); len(got) != 0 {
		t.Fatalf("deloads = %+v, want none from assistance work", got)
	}
}

// The exclusion is on the work, not on the session: a main lift that stalls in
// the same period is still reported.
func TestDeloadsStillCatchMainLiftsAlongsideAssistance(t *testing.T) {
	var sets []Set
	sets = append(sets, mkSets(1, day(2026, time.March, 2), 1, "Squat", 1, 5, 225)...)
	sets = append(sets, assist(mkSets(1, day(2026, time.March, 2), 9, "Lateral Raise", 1, 10, 20))...)
	sets = append(sets, mkSets(2, day(2026, time.March, 9), 1, "Squat", 1, 5, 200)...)
	sets = append(sets, assist(mkSets(2, day(2026, time.March, 9), 9, "Lateral Raise", 1, 10, 10))...)

	got := deloads(groupSessions(sets))
	if len(got) != 1 || got[0].ExerciseName != "Squat" {
		t.Fatalf("deloads = %+v, want only the squat's", got)
	}
}

// A lift prescribed on one day and bolted onto another keeps its prescription,
// so a drop in it is still a stall worth reporting.
func TestDeloadsCatchAMixedLift(t *testing.T) {
	var sets []Set
	sets = append(sets, mkSets(1, day(2026, time.March, 2), 1, "Squat", 1, 5, 225)...)
	sets = append(sets, assist(mkSets(2, day(2026, time.March, 9), 1, "Squat", 1, 5, 200))...)

	if got := deloads(groupSessions(sets)); len(got) != 1 {
		t.Fatalf("deloads = %+v, want one from a lift the program prescribes", got)
	}
}

// sessionTops is the rail behind three filters, not the gate — every live path
// already drops unlogged rows before this package sees them (see the note on
// sessionTops). Pinned anyway, because a bar loaded and walked away from must not
// become a record, a first time, a plate milestone, a rung to close in on, or a
// row in the recap's lift table: this one function decides all five.
func TestARackedBarEarnsNothing(t *testing.T) {
	sets := mkSets(1, day(2026, time.March, 2), 1, "Squat", 5, 0, 225)
	for i := range sets {
		sets[i].Completed = false
	}

	prs, firsts := personalRecords(groupSessions(sets), priorBest(200, 220))
	if len(prs) != 0 || len(firsts) != 0 {
		t.Fatalf("PRs %+v, first times %+v — want neither for a weight never lifted", prs, firsts)
	}
	if got := plateMilestones(groupSessions(sets), map[int32]float64{1: 200}); len(got) != 0 {
		t.Errorf("got %+v, want no milestone for a weight never lifted", got)
	}
}

// Putting a belt on. A chin-up is recorded at 0 lb, so the lift is in the
// baseline with a legitimate best of zero — and the first LOADED set of it is a
// genuine weight record against that zero, carrying PreviousLb == 0.
//
// So previousLb greater than zero is NOT an invariant of a record, and a client
// dividing by it to show a percentage gain would print "+Infinity%". This is the
// one live case behind formatPRGain's guard, which is why that guard is not
// merely defending against a stale cached recap.
func TestALoadedSetOverABodyweightZeroIsARecord(t *testing.T) {
	sets := mkSets(1, day(2026, time.March, 2), 1, "Chin-up", 3, 5, 25)
	base := Baseline{
		BestWeight: map[int32]float64{1: 0},
		BestE1RM:   map[int32]float64{1: 0},
	}

	prs, firsts := personalRecords(groupSessions(sets), base)
	if len(firsts) != 0 {
		t.Fatalf("got %+v, want no first time — the lift is in the baseline", firsts)
	}
	if len(prs) != 1 || prs[0].Kind != PRWeight {
		t.Fatalf("got %+v, want one weight record", prs)
	}
	if prs[0].PreviousLb != 0 {
		t.Errorf("previous = %v, want 0 — the documented zero a client must not divide by",
			prs[0].PreviousLb)
	}
}
