package racked

import (
	"math"
	"testing"
	"time"
)

// day builds a UTC-midnight date, the form performed_on always takes.
func day(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// mkSets builds count identical sets for one session, the shape a 5x5 produces.
func mkSets(sessionID int32, on time.Time, ex int32, name string, count, reps int, weight float64) []Set {
	out := make([]Set, 0, count)
	for i := 0; i < count; i++ {
		out = append(out, Set{
			SessionID:      sessionID,
			PerformedOn:    on,
			StartedAt:      on.Add(9 * time.Hour),
			ProgramDayName: "Workout A",
			ExerciseID:     ex,
			ExerciseName:   name,
			Reps:           reps,
			WeightLb:       weight,
			Completed:      true,
		})
	}
	return out
}

// finish stamps a duration onto every set of a session.
func finish(sets []Set, d time.Duration) []Set {
	for i := range sets {
		sets[i].FinishedAt = sets[i].StartedAt.Add(d)
	}
	return sets
}

func TestSetVolumeAndE1RM(t *testing.T) {
	s := Set{Reps: 3, WeightLb: 185}
	if got := s.VolumeLb(); got != 555 {
		t.Fatalf("VolumeLb = %v, want 555", got)
	}
	// Epley: 185 * (1 + 3/30) = 203.5, rounded to 204.
	if got := s.E1RM(); got != 204 {
		t.Fatalf("E1RM = %v, want 204", got)
	}
	if got := (Set{Reps: 0, WeightLb: 185}).E1RM(); got != 0 {
		t.Fatalf("E1RM with no reps = %v, want 0", got)
	}
}

// Volume counts logged reps whether or not the set was completed — the same
// rule SessionTotals applies, and the reason a set stopped short still counts.
func TestTotalsCountLoggedRepsNotCompletion(t *testing.T) {
	sets := mkSets(1, day(2026, time.March, 2), 1, "Squat", 5, 5, 100)
	sets[4].Reps = 3
	sets[4].Completed = false

	got := totals(sets, groupSessions(sets))
	want := Totals{VolumeLb: 4*5*100 + 3*100, Sessions: 1, Sets: 5, Reps: 23}
	if got != want {
		t.Fatalf("totals = %+v, want %+v", got, want)
	}
}

func TestGroupSessionsOrdersChronologically(t *testing.T) {
	var sets []Set
	sets = append(sets, mkSets(2, day(2026, time.March, 9), 1, "Squat", 1, 5, 100)...)
	sets = append(sets, mkSets(1, day(2026, time.March, 2), 1, "Squat", 1, 5, 95)...)

	sessions := groupSessions(sets)
	if len(sessions) != 2 {
		t.Fatalf("got %d sessions, want 2", len(sessions))
	}
	if sessions[0].ID != 1 || sessions[1].ID != 2 {
		t.Fatalf("sessions out of order: %d then %d", sessions[0].ID, sessions[1].ID)
	}
}

func TestLiftSlicesShareSumsToOne(t *testing.T) {
	var sets []Set
	sets = append(sets, mkSets(1, day(2026, time.March, 2), 1, "Squat", 5, 5, 200)...)
	sets = append(sets, mkSets(1, day(2026, time.March, 2), 2, "Bench Press", 5, 5, 100)...)

	lifts := liftSlices(sets)
	if len(lifts) != 2 {
		t.Fatalf("got %d lifts, want 2", len(lifts))
	}
	// Heaviest volume leads, so the chart's first slice is the lifter's main lift.
	if lifts[0].ExerciseName != "Squat" {
		t.Fatalf("lifts[0] = %q, want Squat", lifts[0].ExerciseName)
	}
	var share float64
	for _, l := range lifts {
		share += l.Share
	}
	if math.Abs(share-1) > 1e-9 {
		t.Fatalf("shares sum to %v, want 1", share)
	}
}

// One point per lift per session: five sets of the same weight is one step on
// the chart, not five.
func TestLiftSeriesTakesSessionTop(t *testing.T) {
	sets := mkSets(1, day(2026, time.March, 2), 1, "Squat", 5, 5, 100)
	sets[4].WeightLb = 120

	series := liftSeries(groupSessions(sets))
	if len(series) != 1 || len(series[0].Points) != 1 {
		t.Fatalf("got %d series with %d points, want 1 and 1", len(series), len(series[0].Points))
	}
	if got := series[0].Points[0].TopWeightLb; got != 120 {
		t.Fatalf("top weight = %v, want 120", got)
	}
}

func TestMostImprovedPrefersPercentageGain(t *testing.T) {
	var sets []Set
	// Deadlift adds 40 lb on a big base; press adds 20 on a small one. The
	// press improved more as a lifter, and that is what should be reported.
	sets = append(sets, mkSets(1, day(2026, time.March, 2), 1, "Deadlift", 1, 5, 400)...)
	sets = append(sets, mkSets(1, day(2026, time.March, 2), 2, "Overhead Press", 1, 5, 100)...)
	sets = append(sets, mkSets(2, day(2026, time.March, 9), 1, "Deadlift", 1, 5, 440)...)
	sets = append(sets, mkSets(2, day(2026, time.March, 9), 2, "Overhead Press", 1, 5, 120)...)

	got := mostImproved(liftSeries(groupSessions(sets)))
	if got == nil {
		t.Fatal("mostImproved = nil, want the press")
	}
	if got.ExerciseName != "Overhead Press" {
		t.Fatalf("mostImproved = %q, want Overhead Press", got.ExerciseName)
	}
}

func TestMostImprovedNeedsTwoSessions(t *testing.T) {
	sets := mkSets(1, day(2026, time.March, 2), 1, "Squat", 1, 5, 100)
	if got := mostImproved(liftSeries(groupSessions(sets))); got != nil {
		t.Fatalf("mostImproved = %+v, want nil from a single session", got)
	}
}

func TestStreakCountsConsecutiveWeeks(t *testing.T) {
	cases := []struct {
		name             string
		days             []time.Time
		longest, current int
	}{
		{
			name:    "three consecutive weeks",
			days:    []time.Time{day(2026, time.March, 2), day(2026, time.March, 9), day(2026, time.March, 16)},
			longest: 3, current: 3,
		},
		{
			name:    "a gap resets the run",
			days:    []time.Time{day(2026, time.March, 2), day(2026, time.March, 9), day(2026, time.March, 30)},
			longest: 2, current: 1,
		},
		{
			// Two sessions in one week are one week of streak, not two.
			name:    "same week twice",
			days:    []time.Time{day(2026, time.March, 2), day(2026, time.March, 4)},
			longest: 1, current: 1,
		},
		{
			// Dec 28 2026 and Jan 4 2027 are consecutive Mondays across a year
			// boundary, where naive ISO week arithmetic would break the run.
			name:    "across a year boundary",
			days:    []time.Time{day(2026, time.December, 28), day(2027, time.January, 4)},
			longest: 2, current: 2,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var sets []Set
			for i, d := range tc.days {
				sets = append(sets, mkSets(int32(i+1), d, 1, "Squat", 1, 5, 100)...)
			}
			got := streak(groupSessions(sets))
			if got.LongestWeeks != tc.longest || got.CurrentWeeks != tc.current {
				t.Fatalf("streak = %+v, want longest %d current %d", got, tc.longest, tc.current)
			}
		})
	}
}

func TestStreakEmpty(t *testing.T) {
	if got := streak(nil); got != (Streak{}) {
		t.Fatalf("streak(nil) = %+v, want zero", got)
	}
}

func TestAttendanceBasis(t *testing.T) {
	monday, wednesday := 1, 3
	start, end := day(2026, time.March, 1), day(2026, time.March, 31)

	t.Run("weekday when the program is scheduled", func(t *testing.T) {
		days := []ProgramDay{{Name: "A", Weekday: &monday}, {Name: "B", Weekday: &wednesday}}
		got := attendance(days, 8, start, end)
		if got.Basis != AttendanceWeekday {
			t.Fatalf("basis = %q, want weekday", got.Basis)
		}
		// March 2026 holds five Mondays and four Wednesdays.
		if got.Expected != 9 {
			t.Fatalf("expected = %d, want 9", got.Expected)
		}
	})

	// The common case: a program exists but nobody set its weekdays. There is no
	// denominator, so there must be no rate — inventing one was the old behaviour
	// and it is what almost every lifter saw.
	t.Run("no rate when the program carries no schedule", func(t *testing.T) {
		days := []ProgramDay{{Name: "A"}, {Name: "B"}, {Name: "C"}}
		got := attendance(days, 10, start, end)
		if got.Basis != AttendanceNone {
			t.Fatalf("basis = %q, want none", got.Basis)
		}
		if got.Expected != 0 || got.Rate != 0 {
			t.Fatalf("attendance = %+v, want no invented target", got)
		}
		// What it reports instead is a measurement: 10 sessions over ~4.4 weeks.
		if got.SessionsPerWeek < 2 || got.SessionsPerWeek > 3 {
			t.Fatalf("sessionsPerWeek = %v, want about 2.3", got.SessionsPerWeek)
		}
	})

	t.Run("none without a program", func(t *testing.T) {
		got := attendance(nil, 4, start, end)
		if got.Basis != AttendanceNone || got.Expected != 0 || got.Actual != 4 {
			t.Fatalf("attendance = %+v, want none/0/4", got)
		}
	})

	// Always populated, so a surface never has to choose between a fake rate and
	// saying nothing.
	t.Run("sessions per week is reported even with a schedule", func(t *testing.T) {
		days := []ProgramDay{{Name: "A", Weekday: &monday}}
		got := attendance(days, 4, start, end)
		if got.Basis != AttendanceWeekday {
			t.Fatalf("basis = %q, want weekday", got.Basis)
		}
		if got.SessionsPerWeek <= 0 {
			t.Fatalf("sessionsPerWeek = %v, want a measurement", got.SessionsPerWeek)
		}
	})

	t.Run("an empty period does not divide by zero", func(t *testing.T) {
		got := attendance(nil, 0, start, start)
		if got.SessionsPerWeek != 0 {
			t.Fatalf("sessionsPerWeek = %v, want 0", got.SessionsPerWeek)
		}
	})
}

func TestHourLabel(t *testing.T) {
	cases := map[int]string{-1: "", 2: "Night owl", 6: "Early bird", 10: "Morning lifter",
		14: "Afternoon lifter", 18: "Evening lifter", 22: "Night owl"}
	for hour, want := range cases {
		if got := hourLabel(hour); got != want {
			t.Fatalf("hourLabel(%d) = %q, want %q", hour, got, want)
		}
	}
}

func TestFillWeekdaysRanksByVolume(t *testing.T) {
	var sets []Set
	// A heavy Saturday against two light Tuesdays: tonnage decides, not turnout.
	sets = append(sets, mkSets(1, day(2026, time.March, 3), 1, "Squat", 1, 5, 100)...)
	sets = append(sets, mkSets(2, day(2026, time.March, 10), 1, "Squat", 1, 5, 100)...)
	sets = append(sets, mkSets(3, day(2026, time.March, 7), 1, "Squat", 5, 5, 300)...)

	rep := Report{Weekdays: make([]float64, 7), BestWeekday: -1}
	fillWeekdays(&rep, groupSessions(sets))
	if rep.BestWeekday != int(time.Saturday) {
		t.Fatalf("BestWeekday = %d, want Saturday (%d)", rep.BestWeekday, int(time.Saturday))
	}
}

func TestFillHoursUsesLocation(t *testing.T) {
	// 02:00 UTC is the previous evening in New York — the zone decides whether
	// this lifter is an early bird or a night owl.
	sets := mkSets(1, day(2026, time.March, 3), 1, "Squat", 1, 5, 100)
	sets[0].StartedAt = time.Date(2026, time.March, 3, 2, 0, 0, 0, time.UTC)

	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skipf("zone data unavailable: %v", err)
	}
	rep := Report{Hours: make([]int, 24)}
	fillHours(&rep, groupSessions(sets), ny)
	if rep.Hours[21] != 1 {
		t.Fatalf("hours = %v, want a session at 21:00 local", rep.Hours)
	}
	if rep.HourLabel != "Night owl" {
		t.Fatalf("HourLabel = %q, want Night owl", rep.HourLabel)
	}
}

func TestChangeOmitsPercentAgainstZero(t *testing.T) {
	got := change(Totals{VolumeLb: 100, Sessions: 2}, Totals{VolumeLb: 0, Sessions: 1})
	if got.VolumePct != nil {
		t.Fatalf("VolumePct = %v, want nil when the prior period moved nothing", *got.VolumePct)
	}
	if got.SessionsPct == nil || *got.SessionsPct != 1 {
		t.Fatalf("SessionsPct = %v, want +1", got.SessionsPct)
	}
}

// Build is the whole package's entry point; this pins that an empty period is a
// valid report rather than a panic or a nil map.
func TestBuildEmptyPeriod(t *testing.T) {
	start, end := Bounds(PeriodMonth, day(2026, time.March, 15))
	got := Build(Input{Kind: PeriodMonth, Start: start, End: end})

	if got.Totals != (Totals{}) {
		t.Fatalf("totals = %+v, want zero", got.Totals)
	}
	if got.Period.Label != "March 2026" {
		t.Fatalf("label = %q, want March 2026", got.Period.Label)
	}
	if got.BestWeekday != -1 {
		t.Fatalf("BestWeekday = %d, want -1", got.BestWeekday)
	}
	if len(got.Weekdays) != 7 || len(got.Hours) != 24 {
		t.Fatalf("weekdays %d hours %d, want 7 and 24", len(got.Weekdays), len(got.Hours))
	}
	if got.Archetype.Name != "" {
		t.Fatalf("archetype = %q, want none without sessions", got.Archetype.Name)
	}
}

func TestBuildEndToEnd(t *testing.T) {
	start, end := Bounds(PeriodMonth, day(2026, time.March, 15))
	var sets []Set
	sets = append(sets, finish(mkSets(1, day(2026, time.March, 2), 1, "Squat", 5, 5, 200), 50*time.Minute)...)
	sets = append(sets, finish(mkSets(2, day(2026, time.March, 9), 1, "Squat", 5, 5, 205), 45*time.Minute)...)

	got := Build(Input{
		Kind:         PeriodMonth,
		Start:        start,
		End:          end,
		Sets:         sets,
		PreviousSets: mkSets(9, day(2026, time.February, 2), 1, "Squat", 5, 5, 100),
		Baseline:     Baseline{BestWeight: map[int32]float64{1: 195}, BestE1RM: map[int32]float64{1: 228}},
	})

	if got.Totals.Sessions != 2 || got.Totals.Sets != 10 || got.Totals.Reps != 50 {
		t.Fatalf("totals = %+v", got.Totals)
	}
	if got.Totals.VolumeLb != 5*5*200+5*5*205 {
		t.Fatalf("volume = %v", got.Totals.VolumeLb)
	}
	if got.Change == nil || got.Change.VolumeLb <= 0 {
		t.Fatalf("change = %+v, want a gain over February", got.Change)
	}
	if got.FastestSession == nil || got.FastestSession.Duration != 45*time.Minute {
		t.Fatalf("fastest = %+v, want the 45 minute session", got.FastestSession)
	}
	if got.HeaviestSet == nil || got.HeaviestSet.WeightLb != 205 {
		t.Fatalf("heaviest = %+v, want 205", got.HeaviestSet)
	}
	// 195 was the standing best, so both sessions beat it and both are records.
	if len(got.PRs) != 2 {
		t.Fatalf("got %d PRs, want 2", len(got.PRs))
	}
	if got.Comparison.Count == 0 {
		t.Fatal("comparison is empty, want an object count")
	}
}

// A linear program deloads on purpose, so the last session of a month is
// systematically the worst one. Judging improvement by it made a lift that gained
// all month read as a decline.
func TestMostImprovedSurvivesAnEndOfPeriodDeload(t *testing.T) {
	var sets []Set
	// Five sessions climbing 225 → 245, then a deload to 220 — below where the
	// month began, which is what makes first-versus-last call this a decline.
	weights := []float64{225, 230, 235, 240, 245, 220}
	for i, w := range weights {
		on := day(2026, time.March, 2).AddDate(0, 0, i*3)
		sets = append(sets, mkSets(int32(i+1), on, 1, "Squat", 1, 5, w)...)
	}

	got := mostImproved(liftSeries(groupSessions(sets)))
	if got == nil {
		t.Fatal("mostImproved = nil; a month of gains ending in a deload still improved")
	}
	if got.GainPct <= 0 {
		t.Fatalf("gain = %v, want positive despite the final deload", got.GainPct)
	}
}

// It still reports a decline when the lift genuinely went backwards — the window
// absorbs one bad session, it does not launder a bad month.
func TestMostImprovedStillReportsARealDecline(t *testing.T) {
	var sets []Set
	for i, w := range []float64{250, 245, 240, 230, 220, 210} {
		on := day(2026, time.March, 2).AddDate(0, 0, i*3)
		sets = append(sets, mkSets(int32(i+1), on, 1, "Squat", 1, 5, w)...)
	}
	if got := mostImproved(liftSeries(groupSessions(sets))); got != nil {
		t.Fatalf("mostImproved = %+v, want nil for a lift that only regressed", got)
	}
}

func TestEdgeWindowBests(t *testing.T) {
	pts := func(e1rms ...float64) []SeriesPoint {
		out := make([]SeriesPoint, 0, len(e1rms))
		for i, e := range e1rms {
			out = append(out, SeriesPoint{
				PerformedOn: day(2026, time.March, 1).AddDate(0, 0, i),
				E1RMLb:      e,
			})
		}
		return out
	}

	cases := []struct {
		name       string
		points     []SeriesPoint
		from, want float64
	}{
		// Thirds of nine: best of the first three against best of the last three.
		{"nine sessions", pts(100, 110, 105, 120, 130, 125, 140, 135, 150), 110, 150},
		// Too few to split, so it is first against last — all the data there is.
		{"two sessions", pts(100, 120), 100, 120},
		{"three sessions", pts(100, 130, 120), 100, 120},
		// Six sessions: windows of two, and the deload in the last slot loses to
		// the better session beside it.
		{"six with a trailing deload", pts(100, 105, 120, 130, 140, 90), 105, 140},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			from, to := edgeWindowBests(tc.points)
			if from != tc.from || to != tc.want {
				t.Fatalf("edgeWindowBests = %v, %v; want %v, %v", from, to, tc.from, tc.want)
			}
		})
	}

	t.Run("empty", func(t *testing.T) {
		if from, to := edgeWindowBests(nil); from != 0 || to != 0 {
			t.Fatalf("edgeWindowBests(nil) = %v, %v; want 0, 0", from, to)
		}
	})
}
