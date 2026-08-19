package racked

import (
	"testing"
	"time"
)

// The baseline is what separates a record from a maximum: without it every lift
// in a lifter's second year would open with a false PR.
func TestPersonalRecordsRespectBaseline(t *testing.T) {
	sets := mkSets(1, day(2026, time.March, 2), 1, "Squat", 5, 5, 200)
	base := Baseline{
		BestWeight: map[int32]float64{1: 225},
		BestE1RM:   map[int32]float64{1: 263},
	}
	if got := personalRecords(groupSessions(sets), base); len(got) != 0 {
		t.Fatalf("got %d PRs, want none below the standing best", len(got))
	}
}

// A 5x5 at a new weight is one achievement. Reporting it five times would bury
// every other lift in the period.
func TestPersonalRecordsOnePerLiftPerSession(t *testing.T) {
	sets := mkSets(1, day(2026, time.March, 2), 1, "Squat", 5, 5, 200)
	got := personalRecords(groupSessions(sets), Baseline{})
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
	base := Baseline{
		BestWeight: map[int32]float64{1: 200},
		BestE1RM:   map[int32]float64{1: 220}, // 200 x 3
	}
	sets := mkSets(1, day(2026, time.March, 2), 1, "Squat", 1, 5, 200) // e1RM 233
	got := personalRecords(groupSessions(sets), base)

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
	got := personalRecords(groupSessions(sets), Baseline{})
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

	got := personalRecords(groupSessions(sets), Baseline{})
	if len(got) != 2 {
		t.Fatalf("got %d PRs, want 2 (the 195 lb session is not a record)", len(got))
	}
	if got[0].WeightLb != 200 || got[1].WeightLb != 205 {
		t.Fatalf("PRs = %v, want 200 then 205", []float64{got[0].WeightLb, got[1].WeightLb})
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
