package racked

import (
	"testing"
	"time"
)

func TestCompare(t *testing.T) {
	cases := []struct {
		name   string
		volume float64
		count  int
		label  string
	}{
		// The heaviest unit that yields a whole count wins, so the number stays
		// small and the object stays worth picturing.
		{"picks the heaviest unit that fits", 84_000, 3, "school buses"},
		{"singular at exactly one", 800, 1, "grand piano"},
		{"falls back to plates", 500, 11, "45 lb plates"},
		{"nothing to say below the lightest unit", 20, 0, ""},
		{"zero volume", 0, 0, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := compare(tc.volume)
			if got.Count != tc.count || got.Label != tc.label {
				t.Fatalf("compare(%v) = %d %q, want %d %q",
					tc.volume, got.Count, got.Label, tc.count, tc.label)
			}
		})
	}
}

// comparisonUnits must stay sorted, because compare walks it and stops at the
// first unit the volume cannot fill.
func TestComparisonUnitsAscend(t *testing.T) {
	for i := 1; i < len(comparisonUnits); i++ {
		if comparisonUnits[i].lb <= comparisonUnits[i-1].lb {
			t.Fatalf("unit %q is not heavier than %q",
				comparisonUnits[i].singular, comparisonUnits[i-1].singular)
		}
	}
}

func TestFormatLb(t *testing.T) {
	cases := map[float64]string{
		0:         "0",
		-5:        "0",
		45:        "45",
		999:       "999",
		1_000:     "1,000",
		12_345:    "12,345",
		100_000:   "100,000",
		1_234_567: "1,234,567",
		1_499.6:   "1,500",
	}
	for in, want := range cases {
		if got := formatLb(in); got != want {
			t.Fatalf("formatLb(%v) = %q, want %q", in, got, want)
		}
	}
}

func TestReportTitle(t *testing.T) {
	start, end := Bounds(PeriodYear, day(2026, time.June, 1))
	got := Build(Input{Kind: PeriodYear, Start: start, End: end}).Title()
	if got != "Racked — 2026" {
		t.Fatalf("Title = %q", got)
	}
}
