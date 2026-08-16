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
