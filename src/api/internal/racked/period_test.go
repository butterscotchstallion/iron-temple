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
	for _, bad := range []string{"", "week", "MONTH", "decade"} {
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
