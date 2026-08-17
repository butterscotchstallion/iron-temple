package racked

import (
	"testing"
	"time"
)

// Accuracy fixes from the recap audit.
//
// Each test here names a figure the recap used to get wrong and pins the
// corrected reading. They are grouped in one file because they were found
// together and share a cause more often than the file layout suggests: four of
// them are the same mistake — measuring a period that has not finished as
// though it had.

// Epley is defined for a set carried past one rep. At exactly one it returns
// weight * 31/30, so a lifter who actually pulled a 225 single was told their
// estimated max was 233 — an estimate three percent above a number that needs
// no estimating.
func TestE1RMOfASingleIsTheWeightLifted(t *testing.T) {
	if got := (Set{Reps: 1, WeightLb: 225}).E1RM(); got != 225 {
		t.Fatalf("E1RM of 225x1 = %v, want 225", got)
	}
	// Unchanged above one rep: 185 * (1 + 3/30) = 203.5, rounded to 204.
	if got := (Set{Reps: 3, WeightLb: 185}).E1RM(); got != 204 {
		t.Fatalf("E1RM of 185x3 = %v, want 204", got)
	}
}

// A single must not be able to beat a genuinely harder set on the same bar.
// Under the old formula 225x1 estimated 233 and 225x2 estimated 240; the first
// is now 225, which is the right order.
func TestE1RMSingleDoesNotOutrankAHarderSet(t *testing.T) {
	single := Set{Reps: 1, WeightLb: 225}.E1RM()
	double := Set{Reps: 2, WeightLb: 225}.E1RM()
	if single >= double {
		t.Fatalf("225x1 estimated %v, 225x2 estimated %v — a single should not rank higher", single, double)
	}
}

// --- the period in progress ---

// The live page opens on the month in progress, so Expected used to count every
// scheduled day of a month that had barely started: a lifter two days in, having
// trained both of them, was told they had made 15% of their sessions.
func TestAttendanceCountsOnlyTheDaysThatHaveHappened(t *testing.T) {
	start, end := Bounds(PeriodMonth, day(2026, time.March, 3))
	mon, wed, fri := 1, 3, 5

	rep := Build(Input{
		Kind:  PeriodMonth,
		Start: start,
		End:   end,
		AsOf:  day(2026, time.March, 3),
		Sets: append(
			mkSets(1, day(2026, time.March, 2), 1, "Squat", 5, 5, 200),
			mkSets(2, day(2026, time.March, 3), 1, "Squat", 5, 5, 205)...,
		),
		ProgramDays: []ProgramDay{
			{Name: "A", Weekday: &mon}, {Name: "B", Weekday: &wed}, {Name: "C", Weekday: &fri},
		},
	})

	// March 2 is a Monday and March 3 a Tuesday, so only Monday the 2nd has come
	// round of the three scheduled weekdays.
	if rep.Attendance.Expected != 1 {
		t.Fatalf("Expected = %d, want 1 — only the elapsed days count", rep.Attendance.Expected)
	}
	if rep.Attendance.Rate != 2 {
		t.Fatalf("Rate = %v, want 2 (two sessions against one scheduled day)", rep.Attendance.Rate)
	}
}

// Same cause: dividing by the whole month's four and a half weeks reported a
// lifter training daily as managing half a session a week.
func TestSessionsPerWeekUsesTheElapsedWindow(t *testing.T) {
	start, end := Bounds(PeriodMonth, day(2026, time.March, 3))

	rep := Build(Input{
		Kind: PeriodMonth, Start: start, End: end, AsOf: day(2026, time.March, 3),
		Sets: append(
			mkSets(1, day(2026, time.March, 2), 1, "Squat", 5, 5, 200),
			mkSets(2, day(2026, time.March, 3), 1, "Squat", 5, 5, 205)...,
		),
	})

	// Three days elapsed, floored to one week: two sessions in that week.
	if rep.Attendance.SessionsPerWeek != 2 {
		t.Fatalf("SessionsPerWeek = %v, want 2", rep.Attendance.SessionsPerWeek)
	}
}

// The archetype reads frequency off the same window, so a lifter training every
// day of a young month used to be classified from a denominator of weeks they
// had not reached.
func TestArchetypeJudgesFrequencyOnElapsedWeeks(t *testing.T) {
	start, end := Bounds(PeriodMonth, day(2026, time.March, 5))

	var sets []Set
	for i := 1; i <= 5; i++ {
		sets = append(sets, mkSets(int32(i), day(2026, time.March, i), 1, "Squat", 5, 5, 200)...)
	}

	rep := Build(Input{
		Kind: PeriodMonth, Start: start, End: end, AsOf: day(2026, time.March, 5), Sets: sets,
	})
	if rep.Archetype.Name != "The Machine" {
		t.Fatalf("Archetype = %q, want The Machine — five sessions in five days", rep.Archetype.Name)
	}
}

// The headline compared a month three days old against a month that ran its full
// length, which made every in-progress recap open on a large fall.
func TestChangeComparesTheSameElapsedWindow(t *testing.T) {
	start, end := Bounds(PeriodMonth, day(2026, time.March, 3))
	prevStart, _ := PreviousBounds(PeriodMonth, day(2026, time.March, 3))

	// Two sessions in the first three days of March, against a February that
	// opened with the same two and then ran on for another eight.
	current := append(
		mkSets(1, day(2026, time.March, 1), 1, "Squat", 5, 5, 200),
		mkSets(2, day(2026, time.March, 3), 1, "Squat", 5, 5, 200)...,
	)
	previous := append(
		mkSets(10, day(2026, time.February, 1), 1, "Squat", 5, 5, 200),
		mkSets(11, day(2026, time.February, 3), 1, "Squat", 5, 5, 200)...,
	)
	for i := 4; i <= 11; i++ {
		previous = append(previous, mkSets(int32(20+i), day(2026, time.February, i), 1, "Squat", 5, 5, 200)...)
	}

	rep := Build(Input{
		Kind: PeriodMonth, Start: start, End: end, AsOf: day(2026, time.March, 3),
		Sets: current, PreviousSets: previous, PreviousStart: prevStart,
	})

	if rep.Change == nil || rep.Change.VolumePct == nil {
		t.Fatalf("Change = %+v, want a volume percentage", rep.Change)
	}
	// Like for like: the same two sessions on both sides, so no movement at all.
	if *rep.Change.VolumePct != 0 {
		t.Fatalf("VolumePct = %v, want 0 — the first three days of each month match",
			*rep.Change.VolumePct)
	}
}

// A completed period is measured over the whole of itself, which is what the
// recap email sends and what an `on` date inside a past month asks for.
func TestCompletedPeriodMeasuresItsWholeLength(t *testing.T) {
	start, end := Bounds(PeriodMonth, day(2026, time.March, 1))
	mon := 1

	rep := Build(Input{
		Kind: PeriodMonth, Start: start, End: end, AsOf: day(2026, time.April, 2),
		Sets:        mkSets(1, day(2026, time.March, 2), 1, "Squat", 5, 5, 200),
		ProgramDays: []ProgramDay{{Name: "A", Weekday: &mon}},
	})

	if rep.Period.InProgress {
		t.Fatal("InProgress = true for a month that has ended")
	}
	// Five Mondays in March 2026.
	if rep.Attendance.Expected != 5 {
		t.Fatalf("Expected = %d, want 5", rep.Attendance.Expected)
	}
}

func TestPeriodInProgressIsFlagged(t *testing.T) {
	start, end := Bounds(PeriodMonth, day(2026, time.March, 3))
	rep := Build(Input{
		Kind: PeriodMonth, Start: start, End: end, AsOf: day(2026, time.March, 3),
		Sets: mkSets(1, day(2026, time.March, 2), 1, "Squat", 5, 5, 200),
	})
	if !rep.Period.InProgress {
		t.Fatal("InProgress = false for a month still running")
	}
}

// An absent AsOf must not silently truncate anything — Build is called directly
// in tests and by the reporter, and a zero date means "the period as a whole".
func TestZeroAsOfMeasuresTheWholePeriod(t *testing.T) {
	start, end := Bounds(PeriodMonth, day(2026, time.March, 1))
	mon := 1

	rep := Build(Input{
		Kind: PeriodMonth, Start: start, End: end,
		Sets:        mkSets(1, day(2026, time.March, 2), 1, "Squat", 5, 5, 200),
		ProgramDays: []ProgramDay{{Name: "A", Weekday: &mon}},
	})
	if rep.Period.InProgress {
		t.Fatal("InProgress = true with no AsOf")
	}
	if rep.Attendance.Expected != 5 {
		t.Fatalf("Expected = %d, want 5", rep.Attendance.Expected)
	}
}

// --- the peak hour ---

// The report named the hour that first reached the highest count as the sessions
// were walked, while the page highlighted the earliest hour holding that count.
// Two evenings followed by two mornings put the label on one bar and the accent
// on another.
func TestPeakHourBreaksTiesTowardTheEarlierHour(t *testing.T) {
	at := func(id int32, d int, hour int) []Set {
		sets := mkSets(id, day(2026, time.March, d), 1, "Squat", 5, 5, 100)
		for i := range sets {
			sets[i].StartedAt = time.Date(2026, time.March, d, hour, 0, 0, 0, time.UTC)
		}
		return sets
	}
	var sets []Set
	sets = append(sets, at(1, 2, 18)...)
	sets = append(sets, at(2, 3, 18)...)
	sets = append(sets, at(3, 4, 6)...)
	sets = append(sets, at(4, 5, 6)...)

	start, end := Bounds(PeriodMonth, day(2026, time.March, 1))
	rep := Build(Input{Kind: PeriodMonth, Start: start, End: end, AsOf: end, Sets: sets})

	if rep.PeakHour != 6 {
		t.Fatalf("PeakHour = %d, want 6 — the earlier of two hours tied on two sessions", rep.PeakHour)
	}
	if rep.HourLabel != "Early bird" {
		t.Fatalf("HourLabel = %q, want Early bird", rep.HourLabel)
	}
}

func TestPeakHourIsAbsentWithoutSessions(t *testing.T) {
	start, end := Bounds(PeriodMonth, day(2026, time.March, 1))
	rep := Build(Input{Kind: PeriodMonth, Start: start, End: end, AsOf: end})
	if rep.PeakHour != -1 {
		t.Fatalf("PeakHour = %d, want -1", rep.PeakHour)
	}
	if rep.HourLabel != "" {
		t.Fatalf("HourLabel = %q, want empty", rep.HourLabel)
	}
}

// --- attendance against the program ---

// Two program days on the same weekday are two sessions asked for, not one. The
// weekdays used to collapse into a set, so a four-day program running Monday
// twice expected three sessions a week.
func TestAttendanceCountsEveryProgramDayOnAWeekday(t *testing.T) {
	start, end := Bounds(PeriodMonth, day(2026, time.March, 1))
	mon, wed := 1, 3

	rep := Build(Input{
		Kind: PeriodMonth, Start: start, End: end, AsOf: end,
		Sets: mkSets(1, day(2026, time.March, 2), 1, "Squat", 5, 5, 200),
		ProgramDays: []ProgramDay{
			{Name: "A", Weekday: &mon},
			{Name: "B", Weekday: &mon},
			{Name: "C", Weekday: &wed},
		},
	})

	// March 2026 holds five Mondays and four Wednesdays: 5*2 + 4.
	if rep.Attendance.Expected != 14 {
		t.Fatalf("Expected = %d, want 14 — both Monday days count", rep.Attendance.Expected)
	}
}

// A program created after the period ended cannot have governed it, so grading
// that period against its schedule invents a target. There is no record of which
// program was current back then, so the honest answer is to report frequency.
func TestAttendanceWillNotGradeAPeriodAgainstALaterProgram(t *testing.T) {
	start, end := Bounds(PeriodMonth, day(2026, time.March, 1))
	mon := 1

	rep := Build(Input{
		Kind: PeriodMonth, Start: start, End: end, AsOf: day(2026, time.June, 1),
		Sets:           mkSets(1, day(2026, time.March, 2), 1, "Squat", 5, 5, 200),
		ProgramDays:    []ProgramDay{{Name: "A", Weekday: &mon}},
		ProgramStarted: day(2026, time.May, 1),
	})

	if rep.Attendance.Basis != AttendanceNone {
		t.Fatalf("Basis = %q, want none for a program newer than the period", rep.Attendance.Basis)
	}
	if rep.Attendance.Expected != 0 || rep.Attendance.Rate != 0 {
		t.Fatalf("Expected/Rate = %d/%v, want 0/0", rep.Attendance.Expected, rep.Attendance.Rate)
	}
	if rep.Attendance.SessionsPerWeek <= 0 {
		t.Fatal("SessionsPerWeek must still be reported when there is no target")
	}
}

func TestAttendanceGradesAPeriodTheProgramCouldHaveGoverned(t *testing.T) {
	start, end := Bounds(PeriodMonth, day(2026, time.March, 1))
	mon := 1

	rep := Build(Input{
		Kind: PeriodMonth, Start: start, End: end, AsOf: day(2026, time.April, 1),
		Sets:           mkSets(1, day(2026, time.March, 2), 1, "Squat", 5, 5, 200),
		ProgramDays:    []ProgramDay{{Name: "A", Weekday: &mon}},
		ProgramStarted: day(2026, time.January, 15),
	})

	if rep.Attendance.Basis != AttendanceWeekday {
		t.Fatalf("Basis = %q, want weekday", rep.Attendance.Basis)
	}
}

// A month whose first scheduled day has not come round yet has no denominator,
// and a rate out of zero is not a rate. This is reachable on the 1st and 2nd of
// most months, which is exactly when somebody opens the recap to see the new
// month start.
func TestAttendanceHasNoRateBeforeTheFirstScheduledDay(t *testing.T) {
	start, end := Bounds(PeriodMonth, day(2026, time.March, 1))
	thu := 4 // March 2026 opens on a Sunday; the first Thursday is the 5th.

	rep := Build(Input{
		Kind: PeriodMonth, Start: start, End: end, AsOf: day(2026, time.March, 2),
		Sets:        mkSets(1, day(2026, time.March, 2), 1, "Squat", 5, 5, 200),
		ProgramDays: []ProgramDay{{Name: "A", Weekday: &thu}},
	})

	if rep.Attendance.Basis != AttendanceNone {
		t.Fatalf("Basis = %q, want none before any scheduled day has passed", rep.Attendance.Basis)
	}
	if rep.Attendance.Rate != 0 || rep.Attendance.Expected != 0 {
		t.Fatalf("attendance = %+v, want no rate", rep.Attendance)
	}
	if rep.Attendance.SessionsPerWeek <= 0 {
		t.Fatal("SessionsPerWeek must still be reported")
	}
}
