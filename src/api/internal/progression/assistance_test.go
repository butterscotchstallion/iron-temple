package progression_test

import (
	"testing"

	"gitea.homelab/gitadmin/iron-temple/api/internal/progression"
)

func perf(weight float64, reps ...int32) *progression.AssistancePerformance {
	return &progression.AssistancePerformance{WeightLb: weight, Reps: reps}
}

func TestNextAssistanceWithoutARange(t *testing.T) {
	// The behaviour that predates this engine, and still the default: do what
	// you did last time, and change it when you feel like it.
	tests := []struct {
		name       string
		fallback   float64
		last       *progression.AssistancePerformance
		wantWeight float64
		wantStatus progression.Status
	}{
		{
			name:       "never performed uses the stored fallback",
			fallback:   25,
			last:       nil,
			wantWeight: 25,
			wantStatus: progression.StatusFixed,
		},
		{
			name:       "carries the last weight forward",
			fallback:   25,
			last:       perf(40, 8, 8, 8),
			wantWeight: 40,
			wantStatus: progression.StatusFixed,
		},
		{
			// Even a session where every set was enormous. Without a range there
			// is nothing to top out, so nothing advances.
			name:       "does not advance however many reps were done",
			fallback:   25,
			last:       perf(40, 30, 30, 30),
			wantWeight: 40,
			wantStatus: progression.StatusFixed,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := progression.NextAssistance(tc.fallback, 0, 0, tc.last)
			if got.WeightLb != tc.wantWeight {
				t.Errorf("weight = %v, want %v", got.WeightLb, tc.wantWeight)
			}
			if got.Status != tc.wantStatus {
				t.Errorf("status = %q, want %q", got.Status, tc.wantStatus)
			}
		})
	}
}

func TestNextAssistanceDoubleProgression(t *testing.T) {
	tests := []struct {
		name       string
		last       *progression.AssistancePerformance
		wantWeight float64
		wantReps   int32
		wantStatus progression.Status
	}{
		{
			// Every set at the top of 8-12: the weight goes up and the reps go
			// back to the bottom.
			name:       "topping out every set advances the weight",
			last:       perf(40, 12, 12, 12),
			wantWeight: 45,
			wantReps:   8,
			wantStatus: progression.StatusProgressing,
		},
		{
			// The point of the range is to carry the top rep count across the
			// whole prescription. One set of 12 and two of 9 is not a session
			// that earned an increase.
			name:       "one short set holds the weight",
			last:       perf(40, 12, 12, 9),
			wantWeight: 40,
			wantReps:   8,
			wantStatus: progression.StatusFixed,
		},
		{
			name:       "over the top still counts as topped out",
			last:       perf(40, 14, 13, 12),
			wantWeight: 45,
			wantReps:   8,
			wantStatus: progression.StatusProgressing,
		},
		{
			name:       "the bottom of the range holds",
			last:       perf(40, 8, 8, 8),
			wantWeight: 40,
			wantReps:   8,
			wantStatus: progression.StatusFixed,
		},
		{
			// A never-performed lift starts at its stored weight, chasing the
			// bottom of the range.
			name:       "no history starts at the fallback and the bottom rep",
			last:       nil,
			wantWeight: 25,
			wantReps:   8,
			wantStatus: progression.StatusFixed,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := progression.NextAssistance(25, 8, 12, tc.last)
			if got.WeightLb != tc.wantWeight {
				t.Errorf("weight = %v, want %v", got.WeightLb, tc.wantWeight)
			}
			if got.TargetReps != tc.wantReps {
				t.Errorf("target reps = %d, want %d", got.TargetReps, tc.wantReps)
			}
			if got.Status != tc.wantStatus {
				t.Errorf("status = %q, want %q", got.Status, tc.wantStatus)
			}
		})
	}
}

// The one rule this engine will not learn. A stalled curl is not a signal, and
// cutting the weight on one solves a problem nobody has — so no history, however
// bad, may produce a weight below what was last worked.
func TestNextAssistanceNeverDeloads(t *testing.T) {
	histories := []*progression.AssistancePerformance{
		perf(40, 1, 1, 1),
		perf(40, 8, 2),
		perf(40, 1),
		perf(40, 0),
	}
	for _, last := range histories {
		got := progression.NextAssistance(25, 8, 12, last)
		if got.WeightLb < last.WeightLb {
			t.Errorf("reps %v cut the weight %v → %v", last.Reps, last.WeightLb, got.WeightLb)
		}
		if got.Status == progression.StatusDeload {
			t.Errorf("reps %v produced a deload", last.Reps)
		}
	}
}

// A range only half given, or given backwards, is not a range. The database
// rejects both, but the engine is pure and does not get to assume its caller
// checked — it collapses to carry-forward rather than inventing a rule.
func TestNextAssistanceIgnoresAnUnusableRange(t *testing.T) {
	last := perf(40, 12, 12, 12)
	for _, r := range [][2]int32{{0, 12}, {8, 0}, {12, 8}, {0, 0}} {
		got := progression.NextAssistance(25, r[0], r[1], last)
		if got.WeightLb != 40 || got.Status != progression.StatusFixed {
			t.Errorf("range %v: got %v/%q, want 40/fixed", r, got.WeightLb, got.Status)
		}
	}
}

// A set nobody touched is not a set that fell short. Unlogged sets are filtered
// out before the engine sees them, so an empty list reads as "no history" rather
// than as a failure to top out.
func TestNextAssistanceTreatsNoLoggedSetsAsNoHistory(t *testing.T) {
	got := progression.NextAssistance(25, 8, 12, &progression.AssistancePerformance{
		WeightLb: 40,
		Reps:     nil,
	})
	if got.WeightLb != 25 || got.Status != progression.StatusFixed {
		t.Errorf("got %v/%q, want the fallback 25/fixed", got.WeightLb, got.Status)
	}
}
