package racked

import (
	"testing"
	"time"
)

// mkPrescribed builds the prescription rows matching a mkSets call. reps is what
// was logged; pass 0 for a set the lifter never got to.
func mkPrescribed(ex int32, name string, count, target, reps int, assistance bool) []Prescribed {
	out := make([]Prescribed, 0, count)
	for i := 0; i < count; i++ {
		out = append(out, Prescribed{
			ExerciseID:   ex,
			ExerciseName: name,
			TargetReps:   target,
			Reps:         reps,
			Completed:    reps >= target,
			IsAssistance: assistance,
		})
	}
	return out
}

// mkMeta is a session finished d after it started.
func mkMeta(id int32, on time.Time, d time.Duration) SessionMeta {
	started := on.Add(9 * time.Hour)
	m := SessionMeta{
		SessionID:      id,
		ProgramID:      1,
		ProgramName:    "StrongLifts 5x5",
		ProgramDayID:   7,
		ProgramDayName: "Workout A",
		PerformedOn:    on,
		StartedAt:      started,
		IsOver:         true,
	}
	if d > 0 {
		m.FinishedAt = started.Add(d)
	}
	return m
}

// A first workout has nothing to be measured against, and every comparison must
// say so rather than inventing a zero to divide by.
func TestBuildSessionFirstEver(t *testing.T) {
	on := day(2026, time.March, 2)
	sets := finish(mkSets(1, on, 1, "Squat", 5, 5, 200), 45*time.Minute)

	rec := BuildSession(SessionInput{
		Meta:       mkMeta(1, on, 45*time.Minute),
		Sets:       sets,
		Prescribed: mkPrescribed(1, "Squat", 5, 5, 5, false),
		Outcomes:   []SessionOutcome{{PerformedOn: on, SetCount: 5, CompletedSetCount: 5}},
	})

	if rec.Pace != nil {
		t.Fatalf("pace = %+v, want nil with no history to rank against", rec.Pace)
	}
	if rec.Progress.PreviousSessionID != nil || rec.Progress.WeightDeltaPct != nil {
		t.Fatalf("progress = %+v, want an empty comparison", rec.Progress)
	}
	if rec.Volume.PreviousLb != nil || rec.Volume.DeltaPct != nil {
		t.Fatalf("volume comparison = %+v, want nil", rec.Volume)
	}
	if len(rec.Lifts) != 1 || rec.Lifts[0].Previous != nil {
		t.Fatalf("lifts = %+v, want one lift with no previous", rec.Lifts)
	}
	if rec.Duration != 45*time.Minute {
		t.Fatalf("duration = %s, want 45m", rec.Duration)
	}
	if rec.Volume.TotalLb != 5000 {
		t.Fatalf("volume = %v, want 5000", rec.Volume.TotalLb)
	}
}

// A session the lifter never closed has no length worth quoting, and neither
// does one left open overnight — so neither may be ranked for pace.
func TestBuildSessionDurationGuards(t *testing.T) {
	on := day(2026, time.March, 2)
	prior := []time.Duration{40 * time.Minute, 50 * time.Minute}

	for _, tc := range []struct {
		name string
		d    time.Duration
	}{
		{"never finished", 0},
		{"past the 12h cap", 13 * time.Hour},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sets := mkSets(1, on, 1, "Squat", 5, 5, 200)
			if tc.d > 0 {
				sets = finish(sets, tc.d)
			}
			rec := BuildSession(SessionInput{
				Meta:         mkMeta(1, on, tc.d),
				Sets:         sets,
				Prescribed:   mkPrescribed(1, "Squat", 5, 5, 5, false),
				DayDurations: prior,
			})
			if rec.Duration != 0 {
				t.Fatalf("duration = %s, want 0", rec.Duration)
			}
			if rec.Pace != nil {
				t.Fatalf("pace = %+v, want nil without a duration of our own", rec.Pace)
			}
			// The rest of the recap still has to be computed: a session nobody
			// closed is still a session somebody trained.
			if rec.Volume.TotalLb != 5000 {
				t.Fatalf("volume = %v, want the work to still count", rec.Volume.TotalLb)
			}
		})
	}
}

// A session finished in the same instant it started has no length, and must not
// be given one.
//
// Found by running the real endpoint rather than by reasoning about it: the
// recap reports whole seconds, so a sub-second gap is truthy in Go and zero on
// the wire — which reads as "never finished" in one field and, far worse, makes
// pace a division by a median of zero. The API cheerfully called such a session
// "6% faster than usual".
func TestBuildSessionIgnoresASubSecondDuration(t *testing.T) {
	on := day(2026, time.March, 2)
	rec := BuildSession(SessionInput{
		Meta:         mkMeta(1, on, 300*time.Millisecond),
		Sets:         finish(mkSets(1, on, 1, "Squat", 5, 5, 200), 300*time.Millisecond),
		Prescribed:   mkPrescribed(1, "Squat", 5, 5, 5, false),
		DayDurations: []time.Duration{45 * time.Minute},
	})

	if rec.Duration != 0 {
		t.Fatalf("duration = %s, want 0 — it rounds to no seconds at all", rec.Duration)
	}
	if rec.Pace != nil {
		t.Fatalf("pace = %+v, want nil without a length of our own", rec.Pace)
	}
	// The work still counts. Only the clock is in doubt.
	if rec.Volume.TotalLb != 5000 {
		t.Fatalf("volume = %v, want 5000", rec.Volume.TotalLb)
	}
}

// The same floor applies to the history pace is ranked within, so a median can
// never be zero.
func TestSessionDurationFloor(t *testing.T) {
	start := day(2026, time.March, 2).Add(9 * time.Hour)
	for _, tc := range []struct {
		name string
		gap  time.Duration
		want time.Duration
	}{
		{"same instant", 0, 0},
		{"sub-second", 999 * time.Millisecond, 0},
		{"exactly a second", time.Second, time.Second},
		{"a real workout", 45 * time.Minute, 45 * time.Minute},
		{"past the 12h cap", 13 * time.Hour, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := SessionDuration(start, start.Add(tc.gap))
			if got != tc.want {
				t.Fatalf("SessionDuration(+%s) = %s, want %s", tc.gap, got, tc.want)
			}
		})
	}
	// A session never finished has no end to measure to.
	if got := SessionDuration(start, time.Time{}); got != 0 {
		t.Fatalf("unfinished = %s, want 0", got)
	}
}

// Pace against a history: median, rank, and the sign of the delta.
func TestBuildSessionPace(t *testing.T) {
	on := day(2026, time.March, 2)
	sets := finish(mkSets(1, on, 1, "Squat", 5, 5, 200), 40*time.Minute)

	rec := BuildSession(SessionInput{
		Meta:       mkMeta(1, on, 40*time.Minute),
		Sets:       sets,
		Prescribed: mkPrescribed(1, "Squat", 5, 5, 5, false),
		DayDurations: []time.Duration{
			50 * time.Minute, 60 * time.Minute, 70 * time.Minute,
		},
	})

	if rec.Pace == nil {
		t.Fatal("pace = nil, want a ranking")
	}
	if rec.Pace.Median != 60*time.Minute {
		t.Fatalf("median = %s, want 60m", rec.Pace.Median)
	}
	if rec.Pace.Rank != 1 || rec.Pace.Of != 4 || rec.Pace.SampleSize != 3 {
		t.Fatalf("rank %d of %d (n=%d), want 1 of 4 (n=3)",
			rec.Pace.Rank, rec.Pace.Of, rec.Pace.SampleSize)
	}
	// Faster than usual reads as a NEGATIVE delta. The sign is the whole
	// hazard of this field; see SessionPace.DeltaPct.
	if rec.Pace.DeltaPct >= 0 {
		t.Fatalf("deltaPct = %v, want negative for a faster session", rec.Pace.DeltaPct)
	}
	if got, want := rec.Pace.DeltaPct, -1.0/3.0; got < want-1e-9 || got > want+1e-9 {
		t.Fatalf("deltaPct = %v, want %v", got, want)
	}
}

// An even-sized history has no middle value, so the two either side of it are
// averaged rather than one of them being picked.
func TestMedianDuration(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   []time.Duration
		want time.Duration
	}{
		{"empty", nil, 0},
		{"one", []time.Duration{40 * time.Minute}, 40 * time.Minute},
		{"odd", []time.Duration{70, 40, 50}, 50},
		{"even", []time.Duration{40, 50, 60, 70}, 55},
		// The skew median exists to resist: one forgotten Finish, capped at
		// 12 hours, must not drag the usual pace upward.
		{"outlier", []time.Duration{40, 42, 44, 12 * time.Hour}, 43},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := medianDuration(tc.in); got != tc.want {
				t.Fatalf("median = %v, want %v", got, tc.want)
			}
		})
	}
}

// A session that ties the fastest shares the rank rather than being ranked
// below the session it matched.
func TestBuildSessionPaceTie(t *testing.T) {
	on := day(2026, time.March, 2)
	rec := BuildSession(SessionInput{
		Meta:         mkMeta(1, on, 40*time.Minute),
		Sets:         finish(mkSets(1, on, 1, "Squat", 5, 5, 200), 40*time.Minute),
		Prescribed:   mkPrescribed(1, "Squat", 5, 5, 5, false),
		DayDurations: []time.Duration{40 * time.Minute, 50 * time.Minute},
	})
	if rec.Pace.Rank != 1 {
		t.Fatalf("rank = %d, want 1 for a session that matched the best", rec.Pace.Rank)
	}
}

// The headline progression pairs top sets. A lift only today, or only last time,
// must not move it — that is the whole reason the figure is trustworthy.
func TestBuildSessionProgressPairsLifts(t *testing.T) {
	prevOn := day(2026, time.February, 23)
	on := day(2026, time.March, 2)

	// Last time: Squat 200, Bench 100. Today: Squat 220, Bench 110, plus a Curl
	// that was not on the day before.
	prevSets := append(
		mkSets(9, prevOn, 1, "Squat", 5, 5, 200),
		mkSets(9, prevOn, 2, "Bench Press", 5, 5, 100)...,
	)
	sets := append(
		mkSets(10, on, 1, "Squat", 5, 5, 220),
		mkSets(10, on, 2, "Bench Press", 5, 5, 110)...,
	)
	sets = append(sets, mkSets(10, on, 3, "Curl", 3, 10, 30)...)

	pres := mkPrescribed(1, "Squat", 5, 5, 5, false)
	pres = append(pres, mkPrescribed(2, "Bench Press", 5, 5, 5, false)...)
	pres = append(pres, mkPrescribed(3, "Curl", 3, 10, 10, true)...)

	prevMeta := mkMeta(9, prevOn, time.Hour)
	rec := BuildSession(SessionInput{
		Meta:         mkMeta(10, on, time.Hour),
		Sets:         sets,
		Prescribed:   pres,
		PreviousMeta: &prevMeta,
		PreviousSets: prevSets,
	})

	if rec.Progress.LiftsCompared != 2 {
		t.Fatalf("liftsCompared = %d, want 2", rec.Progress.LiftsCompared)
	}
	if rec.Progress.LiftsNew != 1 {
		t.Fatalf("liftsNew = %d, want 1 for the Curl", rec.Progress.LiftsNew)
	}
	// (220 + 110) / (200 + 100) - 1 = 0.10. The Curl's 30 lb is absent from
	// both sides; had it been averaged in as its own percentage it would have
	// swung this wildly.
	if got := *rec.Progress.WeightDeltaPct; got < 0.0999 || got > 0.1001 {
		t.Fatalf("weightDeltaPct = %v, want 0.10", got)
	}
}

// Every lift new means there is nothing to compare, which is a different answer
// from "no change" and has to read as one.
func TestBuildSessionProgressNothingInCommon(t *testing.T) {
	prevOn := day(2026, time.February, 23)
	on := day(2026, time.March, 2)
	prevMeta := mkMeta(9, prevOn, time.Hour)

	rec := BuildSession(SessionInput{
		Meta:         mkMeta(10, on, time.Hour),
		Sets:         mkSets(10, on, 1, "Squat", 5, 5, 200),
		Prescribed:   mkPrescribed(1, "Squat", 5, 5, 5, false),
		PreviousMeta: &prevMeta,
		PreviousSets: mkSets(9, prevOn, 2, "Bench Press", 5, 5, 100),
	})

	if rec.Progress.WeightDeltaPct != nil {
		t.Fatalf("weightDeltaPct = %v, want nil with no lift in common", *rec.Progress.WeightDeltaPct)
	}
	if rec.Progress.LiftsCompared != 0 || rec.Progress.LiftsNew != 1 {
		t.Fatalf("progress = %+v, want 0 compared and 1 new", rec.Progress)
	}
	// The reference session is still named: there WAS a last time, and the
	// screen should be able to say so even when nothing lines up.
	if rec.Progress.PreviousSessionID == nil {
		t.Fatal("previousSessionId = nil, want the session that was found")
	}
}

// A rise from nothing is not a percentage.
func TestBuildSessionProgressFromZero(t *testing.T) {
	prevOn := day(2026, time.February, 23)
	on := day(2026, time.March, 2)
	prevMeta := mkMeta(9, prevOn, time.Hour)

	// Bodyweight work last time: the bar was 0 lb.
	rec := BuildSession(SessionInput{
		Meta:         mkMeta(10, on, time.Hour),
		Sets:         mkSets(10, on, 1, "Chin-Up", 3, 8, 25),
		Prescribed:   mkPrescribed(1, "Chin-Up", 3, 8, 8, false),
		PreviousMeta: &prevMeta,
		PreviousSets: mkSets(9, prevOn, 1, "Chin-Up", 3, 8, 0),
	})

	if rec.Progress.WeightDeltaPct != nil {
		t.Fatalf("weightDeltaPct = %v, want nil rather than a division by zero",
			*rec.Progress.WeightDeltaPct)
	}
	if rec.Lifts[0].WeightDeltaPct != nil {
		t.Fatalf("lift delta = %v, want nil", *rec.Lifts[0].WeightDeltaPct)
	}
	// The absolute figure is still meaningful even when the ratio is not.
	if got := *rec.Lifts[0].WeightDeltaLb; got != 25 {
		t.Fatalf("weightDeltaLb = %v, want 25", got)
	}
}

// Unlogged sets are the denominator of "10 of 25 reps". They must count toward
// the prescription and toward nothing else.
func TestBuildSessionUnloggedSets(t *testing.T) {
	on := day(2026, time.March, 2)

	// Squat: 2 of 5 sets logged. Bench: prescribed but never touched.
	sets := mkSets(1, on, 1, "Squat", 2, 5, 200)
	pres := mkPrescribed(1, "Squat", 2, 5, 5, false)
	pres = append(pres, mkPrescribed(1, "Squat", 3, 5, 0, false)...)
	pres = append(pres, mkPrescribed(2, "Bench Press", 5, 5, 0, false)...)

	rec := BuildSession(SessionInput{
		Meta: mkMeta(1, on, 20*time.Minute), Sets: sets, Prescribed: pres,
	})

	if rec.Volume.SetsLogged != 2 || rec.Volume.SetsPrescribed != 10 {
		t.Fatalf("sets = %d/%d, want 2/10", rec.Volume.SetsLogged, rec.Volume.SetsPrescribed)
	}
	if rec.Volume.RepsLogged != 10 || rec.Volume.RepsTargeted != 50 {
		t.Fatalf("reps = %d/%d, want 10/50", rec.Volume.RepsLogged, rec.Volume.RepsTargeted)
	}
	if rec.Volume.TotalLb != 2000 {
		t.Fatalf("volume = %v, want 2000 — unlogged sets moved nothing", rec.Volume.TotalLb)
	}
	// A lift nobody performed is not a row of zeroes; it is absent.
	if len(rec.Lifts) != 1 || rec.Lifts[0].ExerciseName != "Squat" {
		t.Fatalf("lifts = %+v, want only the Squat", rec.Lifts)
	}
	l := rec.Lifts[0]
	if l.SetsLogged != 2 || l.SetsPrescribed != 5 {
		t.Fatalf("squat sets = %d/%d, want 2/5", l.SetsLogged, l.SetsPrescribed)
	}
	if l.HitEveryTarget {
		t.Fatal("hitEveryTarget = true, want false with three sets unlogged")
	}
}

// Lifts read in the order the session prescribed them, not alphabetically —
// the recap is read against the workout that was just performed.
func TestBuildSessionLiftOrder(t *testing.T) {
	on := day(2026, time.March, 2)
	sets := append(
		mkSets(1, on, 1, "Squat", 5, 5, 200),
		mkSets(1, on, 2, "Bench Press", 5, 5, 100)...,
	)
	sets = append(sets, mkSets(1, on, 3, "Curl", 3, 10, 30)...)

	pres := mkPrescribed(1, "Squat", 5, 5, 5, false)
	pres = append(pres, mkPrescribed(2, "Bench Press", 5, 5, 5, false)...)
	pres = append(pres, mkPrescribed(3, "Curl", 3, 10, 10, true)...)

	rec := BuildSession(SessionInput{
		Meta: mkMeta(1, on, time.Hour), Sets: sets, Prescribed: pres,
	})

	want := []string{"Squat", "Bench Press", "Curl"}
	if len(rec.Lifts) != len(want) {
		t.Fatalf("got %d lifts, want %d", len(rec.Lifts), len(want))
	}
	for i, n := range want {
		if rec.Lifts[i].ExerciseName != n {
			t.Fatalf("lift %d = %q, want %q", i, rec.Lifts[i].ExerciseName, n)
		}
	}
	// Assistance is training: it gets a row, flagged, and its volume counts.
	if !rec.Lifts[2].IsAssistance {
		t.Fatal("Curl not flagged as assistance")
	}
	if rec.Lifts[2].VolumeLb != 900 {
		t.Fatalf("curl volume = %v, want 900", rec.Lifts[2].VolumeLb)
	}
}

// The recap's records come from personalRecords, so the suppression rule has to
// survive the new entry point rather than being re-decided here.
func TestBuildSessionPRsDelegate(t *testing.T) {
	on := day(2026, time.March, 2)
	rec := BuildSession(SessionInput{
		Meta:       mkMeta(1, on, time.Hour),
		Sets:       mkSets(1, on, 1, "Squat", 1, 5, 300),
		Prescribed: mkPrescribed(1, "Squat", 1, 5, 5, false),
		Baseline:   Baseline{BestWeight: map[int32]float64{1: 250}, BestE1RM: map[int32]float64{1: 280}},
	})
	if len(rec.PRs) != 1 {
		t.Fatalf("got %d PRs, want 1", len(rec.PRs))
	}
	if rec.PRs[0].Kind != PRWeight {
		t.Fatalf("kind = %q, want the weight record to suppress the estimated one", rec.PRs[0].Kind)
	}
	if rec.PRs[0].PreviousLb != 250 {
		t.Fatalf("previousLb = %v, want the standing best of 250", rec.PRs[0].PreviousLb)
	}
}

// Milestones likewise come from the shared reducer, measured against the
// lifter's history rather than against this session alone.
func TestBuildSessionMilestones(t *testing.T) {
	on := day(2026, time.March, 2)
	rec := BuildSession(SessionInput{
		Meta:       mkMeta(1, on, time.Hour),
		Sets:       mkSets(1, on, 1, "Squat", 5, 5, 225),
		Prescribed: mkPrescribed(1, "Squat", 5, 5, 5, false),
		Baseline:   Baseline{BestWeight: map[int32]float64{1: 220}},
	})
	var found bool
	for _, m := range rec.Milestones {
		if m.Kind == MilestonePlate && m.ValueLb == 225 {
			found = true
		}
	}
	if !found {
		t.Fatalf("milestones = %+v, want a first-225 plate milestone", rec.Milestones)
	}
}

// A lifter who already squatted 225 last year does not cross it again today.
func TestBuildSessionMilestonesRespectHistory(t *testing.T) {
	on := day(2026, time.March, 2)
	rec := BuildSession(SessionInput{
		Meta:       mkMeta(1, on, time.Hour),
		Sets:       mkSets(1, on, 1, "Squat", 5, 5, 230),
		Prescribed: mkPrescribed(1, "Squat", 5, 5, 5, false),
		Baseline:   Baseline{BestWeight: map[int32]float64{1: 225}},
	})
	for _, m := range rec.Milestones {
		if m.ValueLb == 225 {
			t.Fatalf("milestone = %+v, want no repeat of a mark already passed", m)
		}
	}
}

// The two streaks answer different questions and break on different things.
func TestSessionStreak(t *testing.T) {
	mar2 := day(2026, time.March, 2)      // week of Mar 2
	feb25 := day(2026, time.February, 25) // week of Feb 23
	feb23 := day(2026, time.February, 23)
	feb16 := day(2026, time.February, 16) // week of Feb 16

	// Newest first. The Feb 23 session was short of its targets.
	outcomes := []SessionOutcome{
		{PerformedOn: mar2, SetCount: 5, CompletedSetCount: 5},
		{PerformedOn: feb25, SetCount: 5, CompletedSetCount: 5},
		{PerformedOn: feb23, SetCount: 5, CompletedSetCount: 4},
		{PerformedOn: feb16, SetCount: 5, CompletedSetCount: 5},
	}
	got := sessionStreak(outcomes)

	// Two perfect sessions, then a miss ends the run.
	if got.Sessions != 2 {
		t.Fatalf("sessions = %d, want 2", got.Sessions)
	}
	// Three consecutive weeks trained — the missed reps do not break showing up.
	if got.Weeks != 3 {
		t.Fatalf("weeks = %d, want 3", got.Weeks)
	}
}

// A gap in the calendar ends the week run even though the sessions either side
// were both perfect.
func TestSessionStreakWeekGap(t *testing.T) {
	got := sessionStreak([]SessionOutcome{
		{PerformedOn: day(2026, time.March, 2), SetCount: 5, CompletedSetCount: 5},
		{PerformedOn: day(2026, time.February, 9), SetCount: 5, CompletedSetCount: 5},
	})
	if got.Weeks != 1 {
		t.Fatalf("weeks = %d, want 1 across a three-week gap", got.Weeks)
	}
	// The session run is unaffected: both were completed to the letter.
	if got.Sessions != 2 {
		t.Fatalf("sessions = %d, want 2", got.Sessions)
	}
}

// An empty history is not a panic and not a streak of one.
func TestSessionStreakEmpty(t *testing.T) {
	if got := sessionStreak(nil); got.Sessions != 0 || got.Weeks != 0 {
		t.Fatalf("streak = %+v, want zero", got)
	}
}

// A session opened and abandoned still has to produce a whole recap — every
// slice present, every comparison absent, and nothing nil that will be ranged
// over.
func TestBuildSessionNothingLogged(t *testing.T) {
	on := day(2026, time.March, 2)
	rec := BuildSession(SessionInput{
		Meta:       mkMeta(1, on, 0),
		Prescribed: mkPrescribed(1, "Squat", 5, 5, 0, false),
	})

	if rec.Volume.TotalLb != 0 {
		t.Fatalf("volume = %v, want 0", rec.Volume.TotalLb)
	}
	if rec.Volume.SetsPrescribed != 5 || rec.Volume.SetsLogged != 0 {
		t.Fatalf("sets = %d/%d, want 0/5", rec.Volume.SetsLogged, rec.Volume.SetsPrescribed)
	}
	if rec.Lifts == nil || len(rec.Lifts) != 0 {
		t.Fatalf("lifts = %+v, want an empty non-nil slice", rec.Lifts)
	}
	if rec.PRs == nil || rec.Milestones == nil {
		t.Fatal("PRs/Milestones nil, want empty slices so JSON writes [] not null")
	}
	// Too little moved to be worth a comparison, which reads as no comparison
	// rather than "0 plates".
	if rec.Volume.Comparison.Count != 0 {
		t.Fatalf("comparison = %+v, want nothing to say", rec.Volume.Comparison)
	}
}

// The estimated max catches the session where the bar stayed put and the reps
// went up — the one the plate count cannot show.
func TestBuildSessionLiftE1RMDelta(t *testing.T) {
	prevOn := day(2026, time.February, 23)
	on := day(2026, time.March, 2)
	prevMeta := mkMeta(9, prevOn, time.Hour)

	rec := BuildSession(SessionInput{
		Meta:         mkMeta(10, on, time.Hour),
		Sets:         mkSets(10, on, 1, "Squat", 5, 5, 200), // e1RM 233
		Prescribed:   mkPrescribed(1, "Squat", 5, 5, 5, false),
		PreviousMeta: &prevMeta,
		PreviousSets: mkSets(9, prevOn, 1, "Squat", 5, 3, 200), // e1RM 220
	})

	l := rec.Lifts[0]
	if *l.WeightDeltaLb != 0 {
		t.Fatalf("weightDeltaLb = %v, want 0 — same bar", *l.WeightDeltaLb)
	}
	if l.E1RMDeltaPct == nil || *l.E1RMDeltaPct <= 0 {
		t.Fatalf("e1rmDeltaPct = %v, want a gain from the extra reps", l.E1RMDeltaPct)
	}
	if l.TopE1RMLb != 233 || l.Previous.TopE1RMLb != 220 {
		t.Fatalf("e1RM %v vs %v, want 233 vs 220", l.TopE1RMLb, l.Previous.TopE1RMLb)
	}
}

// Volume against the last time this day came round.
func TestBuildSessionVolumeDelta(t *testing.T) {
	prevOn := day(2026, time.February, 23)
	on := day(2026, time.March, 2)
	prevMeta := mkMeta(9, prevOn, time.Hour)

	rec := BuildSession(SessionInput{
		Meta:         mkMeta(10, on, time.Hour),
		Sets:         mkSets(10, on, 1, "Squat", 5, 5, 220), // 5500
		Prescribed:   mkPrescribed(1, "Squat", 5, 5, 5, false),
		PreviousMeta: &prevMeta,
		PreviousSets: mkSets(9, prevOn, 1, "Squat", 5, 5, 200), // 5000
	})

	if *rec.Volume.PreviousLb != 5000 {
		t.Fatalf("previousLb = %v, want 5000", *rec.Volume.PreviousLb)
	}
	if got := *rec.Volume.DeltaPct; got < 0.0999 || got > 0.1001 {
		t.Fatalf("deltaPct = %v, want 0.10", got)
	}
}

// bonus marks prescription rows as sets the lifter added after the rest of the
// session was already done — what AppendSessionSet stamps onto session_sets.
func bonus(rows []Prescribed) []Prescribed {
	for i := range rows {
		rows[i].IsBonus = true
	}
	return rows
}

// A bonus set is counted, and counted INSIDE the ordinary totals rather than
// beside them.
//
// The distinction is the whole reason the figure is safe to add: a lifter who
// did five prescribed sets and one extra reads "6 of 6 sets, 1 bonus", not
// "5 of 5 and also 1", and the tonnage is the tonnage either way. A recap that
// quietly held bonus work out of its own totals would disagree with the same
// session's volume in the history list, which is the bug this shape avoids.
func TestBuildSessionCountsBonusSets(t *testing.T) {
	on := day(2026, time.March, 2)
	sets := finish(mkSets(1, on, 1, "Squat", 6, 5, 200), 45*time.Minute)

	prescribed := mkPrescribed(1, "Squat", 5, 5, 5, false)
	prescribed = append(prescribed, bonus(mkPrescribed(1, "Squat", 1, 5, 5, false))...)

	rec := BuildSession(SessionInput{
		Meta:       mkMeta(1, on, 45*time.Minute),
		Sets:       sets,
		Prescribed: prescribed,
	})

	if rec.Volume.SetsBonus != 1 {
		t.Fatalf("setsBonus = %d, want 1", rec.Volume.SetsBonus)
	}
	// Inside, not beside: six sets were done and one of them was the extra.
	if rec.Volume.SetsLogged != 6 || rec.Volume.SetsPrescribed != 6 {
		t.Fatalf("sets = %d of %d, want 6 of 6 — bonus work stays in both",
			rec.Volume.SetsLogged, rec.Volume.SetsPrescribed)
	}
	if rec.Volume.TotalLb != 6000 {
		t.Fatalf("volume = %v, want 6000 — the bonus set moved weight too", rec.Volume.TotalLb)
	}
	if len(rec.Lifts) != 1 || rec.Lifts[0].SetsBonus != 1 {
		t.Fatalf("lifts = %+v, want the bonus attributed to Squat", rec.Lifts)
	}
}

// An ordinary session reports zero, which is also what every session performed
// before the column existed reports. Nothing infers a bonus set from a set
// count that happens to exceed the usual prescription.
func TestBuildSessionWithoutBonusSetsReportsZero(t *testing.T) {
	on := day(2026, time.March, 2)

	rec := BuildSession(SessionInput{
		Meta:       mkMeta(1, on, 45*time.Minute),
		Sets:       finish(mkSets(1, on, 1, "Squat", 6, 5, 200), 45*time.Minute),
		Prescribed: mkPrescribed(1, "Squat", 6, 5, 5, false),
	})

	if rec.Volume.SetsBonus != 0 {
		t.Fatalf("setsBonus = %d, want 0 — six prescribed sets are not five plus a bonus",
			rec.Volume.SetsBonus)
	}
	if len(rec.Lifts) != 1 || rec.Lifts[0].SetsBonus != 0 {
		t.Fatalf("lifts = %+v, want no bonus on any lift", rec.Lifts)
	}
}

// A bonus set added and then never logged is not bonus work that happened.
//
// The recap reports what the lifter did, and an empty row is the one thing it
// must not dress up as a set. SetsBonus therefore counts only logged sets,
// which is the same rule SetsLogged keeps — and it stays a count OF SetsLogged
// rather than drifting above it.
func TestBuildSessionIgnoresUnloggedBonusSets(t *testing.T) {
	on := day(2026, time.March, 2)

	prescribed := mkPrescribed(1, "Squat", 5, 5, 5, false)
	prescribed = append(prescribed, bonus(mkPrescribed(1, "Squat", 1, 5, 0, false))...)

	rec := BuildSession(SessionInput{
		Meta:       mkMeta(1, on, 45*time.Minute),
		Sets:       finish(mkSets(1, on, 1, "Squat", 5, 5, 200), 45*time.Minute),
		Prescribed: prescribed,
	})

	if rec.Volume.SetsBonus != 0 {
		t.Fatalf("setsBonus = %d, want 0 — the set was added but never performed",
			rec.Volume.SetsBonus)
	}
	if rec.Volume.SetsLogged != 5 || rec.Volume.SetsPrescribed != 6 {
		t.Fatalf("sets = %d of %d, want 5 of 6", rec.Volume.SetsLogged, rec.Volume.SetsPrescribed)
	}
}

// Bonus sets land on the lift they belong to, not on whichever lift came first.
func TestBuildSessionAttributesBonusSetsPerLift(t *testing.T) {
	on := day(2026, time.March, 2)

	sets := finish(mkSets(1, on, 1, "Squat", 5, 5, 200), time.Hour)
	sets = append(sets, finish(mkSets(1, on, 2, "Bench Press", 7, 5, 135), time.Hour)...)

	prescribed := mkPrescribed(1, "Squat", 5, 5, 5, false)
	prescribed = append(prescribed, mkPrescribed(2, "Bench Press", 5, 5, 5, false)...)
	prescribed = append(prescribed, bonus(mkPrescribed(2, "Bench Press", 2, 5, 5, false))...)

	rec := BuildSession(SessionInput{
		Meta:       mkMeta(1, on, time.Hour),
		Sets:       sets,
		Prescribed: prescribed,
	})

	if rec.Volume.SetsBonus != 2 {
		t.Fatalf("setsBonus = %d, want 2", rec.Volume.SetsBonus)
	}
	byName := map[string]SessionLift{}
	for _, l := range rec.Lifts {
		byName[l.ExerciseName] = l
	}
	if got := byName["Squat"].SetsBonus; got != 0 {
		t.Fatalf("Squat setsBonus = %d, want 0", got)
	}
	if got := byName["Bench Press"].SetsBonus; got != 2 {
		t.Fatalf("Bench Press setsBonus = %d, want 2", got)
	}
	// The per-lift counts must add up to the session's, or the two figures on
	// the recap screen contradict each other.
	var total int
	for _, l := range rec.Lifts {
		total += l.SetsBonus
	}
	if total != rec.Volume.SetsBonus {
		t.Fatalf("per-lift bonus sets sum to %d, session says %d", total, rec.Volume.SetsBonus)
	}
}
