package racked

import (
	"math"
	"testing"
	"time"
)

// weighIns builds a series a week apart from the given weights, the shape a
// lifter who steps on the scale each Monday produces.
func weighIns(from time.Time, lb ...float64) []WeighIn {
	out := make([]WeighIn, 0, len(lb))
	for i, w := range lb {
		out = append(out, WeighIn{PerformedOn: from.AddDate(0, 0, 7*i), WeightLb: w})
	}
	return out
}

// A period nobody weighed in for has no bodyweight section at all. A zeroed
// struct would render as a lifter who weighs nothing, which is worse than
// silence and is the reason this returns a pointer.
func TestBodyweightAbsentWithoutWeighIns(t *testing.T) {
	if got := bodyweight(nil); got != nil {
		t.Fatalf("bodyweight(nil) = %+v, want nil", got)
	}
	if got := bodyweight([]WeighIn{}); got != nil {
		t.Fatalf("bodyweight(empty) = %+v, want nil", got)
	}
}

func TestBodyweightMeasuresEndAgainstStart(t *testing.T) {
	b := bodyweight(weighIns(day(2026, time.March, 2), 184, 186.2, 182.5, 181.4))
	if b == nil {
		t.Fatal("bodyweight = nil, want a summary")
	}
	if b.StartLb != 184 || b.EndLb != 181.4 {
		t.Fatalf("start/end = %v/%v, want 184/181.4", b.StartLb, b.EndLb)
	}
	// The range is the series' extremes, not its ends: the lifter was heavier in
	// week two than in week one.
	if b.LowLb != 181.4 || b.HighLb != 186.2 {
		t.Fatalf("low/high = %v/%v, want 181.4/186.2", b.LowLb, b.HighLb)
	}
	if b.ChangeLb == nil || math.Abs(*b.ChangeLb-(-2.6)) > 1e-9 {
		t.Fatalf("changeLb = %v, want -2.6", b.ChangeLb)
	}
	if b.ChangePct == nil || math.Abs(*b.ChangePct-(-2.6/184)) > 1e-9 {
		t.Fatalf("changePct = %v, want -2.6/184", b.ChangePct)
	}
	if len(b.Points) != 4 {
		t.Fatalf("got %d points, want 4", len(b.Points))
	}
}

// One reading is a fact, not a trend. Reporting it as no change would claim a
// stability nobody measured — the same reasoning that leaves Change.VolumePct
// nil rather than zero when there is no ratio to quote.
func TestBodyweightSingleWeighInHasNoChange(t *testing.T) {
	b := bodyweight(weighIns(day(2026, time.March, 2), 181.4))
	if b == nil {
		t.Fatal("bodyweight = nil, want a summary from one weigh-in")
	}
	if b.ChangeLb != nil || b.ChangePct != nil {
		t.Fatalf("change = %v/%v, want nil from a single reading", b.ChangeLb, b.ChangePct)
	}
	// There is still a number to report, and both ends are it.
	if b.StartLb != 181.4 || b.EndLb != 181.4 || b.LowLb != 181.4 || b.HighLb != 181.4 {
		t.Fatalf("summary = %+v, want 181.4 throughout", b)
	}
}

// A lifter who held their weight has a change, and it is zero. That is a
// different answer from "we don't know", which is what a single weigh-in gives,
// and the two must not collapse into each other.
func TestBodyweightHeldSteadyReportsZeroNotNil(t *testing.T) {
	b := bodyweight(weighIns(day(2026, time.March, 2), 181.4, 181.4))
	if b.ChangeLb == nil {
		t.Fatal("changeLb = nil, want an explicit zero from two equal readings")
	}
	if *b.ChangeLb != 0 || *b.ChangePct != 0 {
		t.Fatalf("change = %v/%v, want 0/0", *b.ChangeLb, *b.ChangePct)
	}
}

// Two sessions in one day are two readings. Neither is dropped, and the later
// one is the period's end.
func TestBodyweightKeepsBothReadingsFromOneDay(t *testing.T) {
	on := day(2026, time.March, 2)
	b := bodyweight([]WeighIn{
		{PerformedOn: on, WeightLb: 184},
		{PerformedOn: on, WeightLb: 183},
	})
	if len(b.Points) != 2 {
		t.Fatalf("got %d points, want both readings", len(b.Points))
	}
	if b.EndLb != 183 {
		t.Fatalf("endLb = %v, want the later reading, 183", b.EndLb)
	}
}

// The report carries the summary through, so the page and the email read the
// same series.
func TestBuildCarriesBodyweight(t *testing.T) {
	in := Input{
		Kind:     PeriodMonth,
		Start:    day(2026, time.March, 1),
		End:      day(2026, time.March, 31),
		Sets:     mkSets(1, day(2026, time.March, 2), 1, "Squat", 5, 5, 200),
		WeighIns: weighIns(day(2026, time.March, 2), 184, 181.4),
	}
	if rep := Build(in); rep.Bodyweight == nil || rep.Bodyweight.EndLb != 181.4 {
		t.Fatalf("report bodyweight = %+v, want the series", rep.Bodyweight)
	}

	in.WeighIns = nil
	if rep := Build(in); rep.Bodyweight != nil {
		t.Fatalf("report bodyweight = %+v, want nil without weigh-ins", rep.Bodyweight)
	}
}
