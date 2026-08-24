package progression

import "testing"

func TestLayoffPct(t *testing.T) {
	tests := []struct {
		name  string
		weeks int
		want  float64
	}{
		{name: "trained this week is not a layoff", weeks: 0, want: 0},
		{name: "one week off takes ten percent", weeks: 1, want: 0.10},
		{name: "two weeks off takes twenty", weeks: 2, want: 0.20},
		{name: "three weeks off takes thirty", weeks: 3, want: 0.30},
		{name: "five weeks off hits the cap exactly", weeks: 5, want: 0.50},
		{name: "a year off is still the cap", weeks: 52, want: 0.50},
		// Nothing should produce this, but a clock skew or a back-dated session
		// could; it must read as "no layoff" rather than adding weight.
		{name: "negative weeks are not a layoff", weeks: -3, want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := LayoffPct(tt.weeks); got != tt.want {
				t.Errorf("LayoffPct(%d) = %v, want %v", tt.weeks, got, tt.want)
			}
		})
	}
}

func TestLayoffWeight(t *testing.T) {
	tests := []struct {
		name       string
		previousLb float64
		weeks      int
		want       float64
	}{
		{name: "no layoff leaves the weight alone", previousLb: 225, weeks: 0, want: 225},
		{name: "one week off", previousLb: 225, weeks: 1, want: 205}, // 202.5 -> 205
		{name: "two weeks off", previousLb: 225, weeks: 2, want: 180},
		// roundToBar rounds a half increment up, as it does for a stall deload
		// (202.5 -> 205), so these land above the exact fraction, not below.
		{name: "three weeks off", previousLb: 225, weeks: 3, want: 160}, // 157.5 -> 160
		{name: "capped at half", previousLb: 225, weeks: 40, want: 115}, // 112.5 -> 115
		{name: "bodyweight assistance stays at zero", previousLb: 0, weeks: 4, want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := LayoffWeight(tt.previousLb, tt.weeks); got != tt.want {
				t.Errorf("LayoffWeight(%v, %d) = %v, want %v", tt.previousLb, tt.weeks, got, tt.want)
			}
		})
	}
}

// Every result is loadable on a standard bar, which is the property the
// table above only samples.
func TestLayoffWeightSnapsToTheBar(t *testing.T) {
	for previous := 45.0; previous <= 500; previous += 5 {
		for weeks := 1; weeks <= 8; weeks++ {
			got := LayoffWeight(previous, weeks)
			if r := got / BarIncrementLb; r != float64(int(r)) {
				t.Errorf("LayoffWeight(%v, %d) = %v, not a multiple of %v",
					previous, weeks, got, BarIncrementLb)
			}
		}
	}
}

func TestApplyLayoff(t *testing.T) {
	tests := []struct {
		name       string
		plan       Plan
		weeks      int
		wantWeight float64
		wantStatus Status
		wantPct    float64
	}{
		{
			name:       "no time away leaves the plan alone",
			plan:       Plan{WeightLb: 230, Status: StatusAdvance, PreviousLb: 225},
			weeks:      0,
			wantWeight: 230,
			wantStatus: StatusAdvance,
		},
		{
			// The cut comes off the 225 that was lifted, not the 230 the engine
			// was about to ask for.
			name:       "an advance is cut from what was actually lifted",
			plan:       Plan{WeightLb: 230, Status: StatusAdvance, PreviousLb: 225},
			weeks:      2,
			wantWeight: 180,
			wantStatus: StatusLayoff,
			wantPct:    0.20,
		},
		{
			name:       "a hold is cut too",
			plan:       Plan{WeightLb: 225, Status: StatusHold, FailureCount: 1, PreviousLb: 225},
			weeks:      3,
			wantWeight: 160,
			wantStatus: StatusLayoff,
			wantPct:    0.30,
		},
		{
			// There is no history to detrain from, and PreviousLb is 0 — the
			// guard here is what stops a first-ever lift being prescribed an
			// empty bar.
			name:       "a first-ever lift is never cut",
			plan:       Plan{WeightLb: 45, Status: StatusStart},
			weeks:      12,
			wantWeight: 45,
			wantStatus: StatusStart,
		},
		{
			// 90% of 225 is 205; a week off would also be 205. The two must not
			// compound into 180.
			name:       "a stall deload as deep as the layoff wins",
			plan:       Plan{WeightLb: 205, Status: StatusDeload, FailureCount: 3, PreviousLb: 225},
			weeks:      1,
			wantWeight: 205,
			wantStatus: StatusDeload,
		},
		{
			name:       "a deeper layoff takes over from a stall deload",
			plan:       Plan{WeightLb: 205, Status: StatusDeload, FailureCount: 3, PreviousLb: 225},
			weeks:      4,
			wantWeight: 135,
			wantStatus: StatusLayoff,
			wantPct:    0.40,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ApplyLayoff(tt.plan, tt.weeks)
			if got.WeightLb != tt.wantWeight {
				t.Errorf("weight = %v, want %v", got.WeightLb, tt.wantWeight)
			}
			if got.Status != tt.wantStatus {
				t.Errorf("status = %v, want %v", got.Status, tt.wantStatus)
			}
			if got.LayoffPct != tt.wantPct {
				t.Errorf("layoffPct = %v, want %v", got.LayoffPct, tt.wantPct)
			}
			// The cut explains itself against the weight it came off, so the
			// reasoning behind the original plan has to survive it.
			if got.PreviousLb != tt.plan.PreviousLb {
				t.Errorf("previousLb = %v, want %v", got.PreviousLb, tt.plan.PreviousLb)
			}
			if got.FailureCount != tt.plan.FailureCount {
				t.Errorf("failureCount = %v, want %v", got.FailureCount, tt.plan.FailureCount)
			}
		})
	}
}

// A layoff never prescribes more than the lifter was already going to do — the
// property the "deeper one wins" case in the table above only samples.
func TestApplyLayoffNeverAddsWeight(t *testing.T) {
	plans := []Plan{
		{WeightLb: 230, Status: StatusAdvance, PreviousLb: 225},
		{WeightLb: 225, Status: StatusHold, FailureCount: 2, PreviousLb: 225},
		{WeightLb: 205, Status: StatusDeload, FailureCount: 3, PreviousLb: 225},
	}
	for _, p := range plans {
		for weeks := 0; weeks <= 20; weeks++ {
			if got := ApplyLayoff(p, weeks).WeightLb; got > p.WeightLb {
				t.Errorf("ApplyLayoff(%+v, %d) raised the weight to %v", p, weeks, got)
			}
		}
	}
}
