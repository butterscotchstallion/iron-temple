package racked

import (
	"testing"
	"time"
)

func TestParsePeriodKind(t *testing.T) {
	if k, ok := ParsePeriodKind("month"); !ok || k != PeriodMonth {
		t.Fatalf("month parsed as %q ok=%v", k, ok)
	}
	if k, ok := ParsePeriodKind("year"); !ok || k != PeriodYear {
		t.Fatalf("year parsed as %q ok=%v", k, ok)
	}
	if k, ok := ParsePeriodKind("week"); !ok || k != PeriodWeek {
		t.Fatalf("week parsed as %q ok=%v", k, ok)
	}
	for _, bad := range []string{"", "day", "MONTH", "decade"} {
		if _, ok := ParsePeriodKind(bad); ok {
			t.Fatalf("ParsePeriodKind(%q) accepted", bad)
		}
	}
}

func TestBounds(t *testing.T) {
	cases := []struct {
		name       string
		kind       PeriodKind
		on         time.Time
		start, end time.Time
	}{
		{"month", PeriodMonth, day(2026, time.March, 15), day(2026, time.March, 1), day(2026, time.March, 31)},
		{"february", PeriodMonth, day(2026, time.February, 3), day(2026, time.February, 1), day(2026, time.February, 28)},
		{"leap february", PeriodMonth, day(2028, time.February, 3), day(2028, time.February, 1), day(2028, time.February, 29)},
		{"december", PeriodMonth, day(2026, time.December, 31), day(2026, time.December, 1), day(2026, time.December, 31)},
		{"year", PeriodYear, day(2026, time.June, 9), day(2026, time.January, 1), day(2026, time.December, 31)},
		// Weeks run Monday to Sunday, the same convention the streak counter
		// uses. March 18th 2026 is a Wednesday.
		{"week from midweek", PeriodWeek, day(2026, time.March, 18), day(2026, time.March, 16), day(2026, time.March, 22)},
		// The two ends of a week are the cases an off-by-one lands on: a Monday
		// opens its own week rather than closing the one before, and a Sunday
		// closes its own rather than opening the next.
		{"week from its monday", PeriodWeek, day(2026, time.March, 16), day(2026, time.March, 16), day(2026, time.March, 22)},
		{"week from its sunday", PeriodWeek, day(2026, time.March, 22), day(2026, time.March, 16), day(2026, time.March, 22)},
		// A week does not respect month or year boundaries, and must not be
		// clipped to them — the days on the other side are part of it.
		{"week across a month", PeriodWeek, day(2026, time.March, 31), day(2026, time.March, 30), day(2026, time.April, 5)},
		{"week across a year", PeriodWeek, day(2026, time.January, 1), day(2025, time.December, 29), day(2026, time.January, 4)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			start, end := Bounds(tc.kind, tc.on)
			if !start.Equal(tc.start) || !end.Equal(tc.end) {
				t.Fatalf("Bounds = %v..%v, want %v..%v", start, end, tc.start, tc.end)
			}
		})
	}
}

func TestPreviousBounds(t *testing.T) {
	cases := []struct {
		name       string
		kind       PeriodKind
		on         time.Time
		start, end time.Time
	}{
		{"month", PeriodMonth, day(2026, time.March, 15), day(2026, time.February, 1), day(2026, time.February, 28)},
		// The month before January is in the previous year, which is the case a
		// naive month-1 would get wrong.
		{"january", PeriodMonth, day(2026, time.January, 9), day(2025, time.December, 1), day(2025, time.December, 31)},
		{"year", PeriodYear, day(2026, time.June, 9), day(2025, time.January, 1), day(2025, time.December, 31)},
		{"week", PeriodWeek, day(2026, time.March, 18), day(2026, time.March, 9), day(2026, time.March, 15)},
		// The week before the one containing New Year's Day is in the previous
		// year, and is not the last week of it by any calendar arithmetic that
		// starts from January.
		{"week across a year", PeriodWeek, day(2026, time.January, 1), day(2025, time.December, 22), day(2025, time.December, 28)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			start, end := PreviousBounds(tc.kind, tc.on)
			if !start.Equal(tc.start) || !end.Equal(tc.end) {
				t.Fatalf("PreviousBounds = %v..%v, want %v..%v", start, end, tc.start, tc.end)
			}
		})
	}
}

func TestDuePeriods(t *testing.T) {
	const window = CatchUpWindow

	cases := []struct {
		name string
		now  time.Time
		want []Due
	}{
		{
			// The day both a month and a year close. Two recaps, as specified.
			name: "new year's day owes both",
			now:  day(2027, time.January, 1),
			want: []Due{
				{PeriodMonth, day(2026, time.December, 1), day(2026, time.December, 31)},
				{PeriodYear, day(2026, time.January, 1), day(2026, time.December, 31)},
			},
		},
		{
			name: "the first of a month owes that month",
			now:  day(2026, time.April, 1),
			want: []Due{{PeriodMonth, day(2026, time.March, 1), day(2026, time.March, 31)}},
		},
		{
			// Still inside the catch-up window: an API that was down over the
			// turn of the month sends late rather than not at all. report_runs,
			// not this function, stops a second copy going out.
			name: "mid-month still owes the month just gone",
			now:  day(2026, time.April, 20),
			want: []Due{{PeriodMonth, day(2026, time.March, 1), day(2026, time.March, 31)}},
		},
		{
			// Only ever the month just gone, never a history — which is what
			// keeps a first deploy from mailing back-issues, with or without
			// the window.
			name: "only the most recent month is ever owed",
			now:  day(2026, time.June, 15),
			want: []Due{{PeriodMonth, day(2026, time.May, 1), day(2026, time.May, 31)}},
		},
		{
			// The annual recap belongs to early January. By March the year is
			// past the window and only the month is owed.
			name: "the year drops out once January is well past",
			now:  day(2027, time.March, 20),
			want: []Due{{PeriodMonth, day(2027, time.February, 1), day(2027, time.February, 28)}},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := DuePeriods(tc.now, window)
			if len(got) != len(tc.want) {
				t.Fatalf("got %d periods %+v, want %d", len(got), got, len(tc.want))
			}
			for i := range got {
				if got[i].Kind != tc.want[i].Kind ||
					!got[i].Start.Equal(tc.want[i].Start) ||
					!got[i].End.Equal(tc.want[i].End) {
					t.Fatalf("period %d = %+v, want %+v", i, got[i], tc.want[i])
				}
			}
		})
	}
}

// hasKind reports whether a recap of that granularity is owed.
func hasKind(due []Due, kind PeriodKind) bool {
	for _, d := range due {
		if d.Kind == kind {
			return true
		}
	}
	return false
}

// The window binds the annual recap and effectively nothing else, so the
// boundary worth pinning is the year's. 2026 closed on December 31; 40 days
// later is February 9.
func TestDuePeriodsWindowBoundsTheYear(t *testing.T) {
	if !hasKind(DuePeriods(day(2027, time.February, 9), CatchUpWindow), PeriodYear) {
		t.Error("the annual recap fell out of the window a day early")
	}
	if hasKind(DuePeriods(day(2027, time.February, 10), CatchUpWindow), PeriodYear) {
		t.Error("the annual recap is still owed past the window")
	}
}

// The monthly recap is never dropped by the window: a month's end is at most 31
// days behind any date in the month after it. Being down over the 1st delays
// the recap; it does not cancel it.
func TestDuePeriodsAlwaysOwesTheMonthJustGone(t *testing.T) {
	for _, d := range []time.Time{
		day(2026, time.April, 1),
		day(2026, time.April, 15),
		day(2026, time.April, 30),
		day(2026, time.January, 31),
	} {
		if !hasKind(DuePeriods(d, CatchUpWindow), PeriodMonth) {
			t.Errorf("no monthly recap owed on %s", d.Format(dateOnly))
		}
	}
}

const dateOnly = "2006-01-02"

// The week label names the days a week covers rather than numbering it, and has
// three forms because a week is the one period that routinely straddles a
// boundary.
func TestWeekLabel(t *testing.T) {
	cases := []struct {
		name string
		on   time.Time
		want string
	}{
		{"within one month", day(2026, time.March, 18), "March 16–22 2026"},
		{"across two months", day(2026, time.March, 31), "March 30 – April 5 2026"},
		{"across two years", day(2026, time.January, 1), "December 29 2025 – January 4 2026"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			start, _ := Bounds(PeriodWeek, tc.on)
			if got := periodLabel(PeriodWeek, start); got != tc.want {
				t.Errorf("periodLabel = %q, want %q", got, tc.want)
			}
		})
	}
}

// A weekly recap is a page, not an email. The reporter mails the periods a
// lifter reflects on; 52 messages a year is a decision about somebody's inbox
// and wants a way to opt out before it wants an implementation.
func TestDuePeriodsNeverOwesAWeek(t *testing.T) {
	// A Monday in March: the previous week ended yesterday, so a week would be
	// due here if weeks were ever due.
	for _, d := range []time.Time{
		day(2026, time.March, 16),
		day(2026, time.January, 1),
		day(2026, time.June, 10),
	} {
		for _, due := range DuePeriods(d, CatchUpWindow) {
			if due.Kind == PeriodWeek {
				t.Errorf("DuePeriods(%v) owes a weekly recap", d.Format("2006-01-02"))
			}
		}
	}
}

// The recap's own week and the streak counter's week are the same week. They
// were always going to be, since Bounds calls weekStart — which is exactly why
// it does, and why this pins it rather than trusting the call site to stay.
func TestWeekBoundsAgreeWithTheStreakCounter(t *testing.T) {
	for i := 0; i < 14; i++ {
		on := day(2026, time.March, 16).AddDate(0, 0, i)
		start, end := Bounds(PeriodWeek, on)
		if got := weekStart(on); !got.Equal(start) {
			t.Fatalf("on %v: weekStart = %v, Bounds start = %v", on, got, start)
		}
		if d := end.Sub(start).Hours() / 24; d != 6 {
			t.Fatalf("on %v: week spans %v days after its start, want 6", on, d)
		}
	}
}
