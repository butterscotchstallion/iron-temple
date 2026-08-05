package progression

import "testing"

func TestIncrementFor(t *testing.T) {
	if got := IncrementFor("Deadlift"); got != IncrementDeadlift {
		t.Errorf("Deadlift increment = %v, want %v", got, IncrementDeadlift)
	}
	for _, name := range []string{"Squat", "Bench Press", "Barbell Row", "Overhead Press", "Unknown"} {
		if got := IncrementFor(name); got != IncrementDefault {
			t.Errorf("%s increment = %v, want %v", name, got, IncrementDefault)
		}
	}
}

func ok(w float64) SessionResult   { return SessionResult{WeightLb: w, Success: true} }
func fail(w float64) SessionResult { return SessionResult{WeightLb: w, Success: false} }

func TestNext(t *testing.T) {
	tests := []struct {
		name      string
		start     float64
		increment float64
		history   []SessionResult
		want      float64
	}{
		{
			name:      "no history returns starting weight",
			start:     45,
			increment: IncrementDefault,
			history:   nil,
			want:      45,
		},
		{
			name:      "success advances by increment",
			increment: IncrementDefault,
			history:   []SessionResult{ok(100)},
			want:      105,
		},
		{
			name:      "deadlift success advances by ten",
			increment: IncrementDeadlift,
			history:   []SessionResult{ok(135)},
			want:      145,
		},
		{
			name:      "single failure repeats the weight",
			increment: IncrementDefault,
			history:   []SessionResult{ok(95), fail(100)},
			want:      100,
		},
		{
			name:      "two failures still repeat",
			increment: IncrementDefault,
			history:   []SessionResult{fail(100), fail(100)},
			want:      100,
		},
		{
			name:      "three failures deload to ninety percent",
			increment: IncrementDefault,
			history:   []SessionResult{fail(100), fail(100), fail(100)},
			want:      90,
		},
		{
			name:      "deload rounds to nearest five",
			increment: IncrementDefault,
			// 105 * 0.90 = 94.5 -> 95
			history: []SessionResult{fail(105), fail(105), fail(105)},
			want:    95,
		},
		{
			name:      "failure streak counts only at current weight",
			increment: IncrementDefault,
			// Failed thrice at 100, deloaded to 90, failed once there:
			// the old streak must not carry over and re-trigger a deload.
			history: []SessionResult{fail(100), fail(100), fail(100), fail(90)},
			want:    90,
		},
		{
			name:      "success after failures resets and advances",
			increment: IncrementDefault,
			history:   []SessionResult{fail(100), fail(100), ok(100)},
			want:      105,
		},
		{
			name:      "only trailing failures count",
			increment: IncrementDefault,
			// Two early failures, a success, then two failures: streak is 2.
			history: []SessionResult{fail(100), fail(100), ok(100), fail(105), fail(105)},
			want:    105,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Next(tt.start, tt.increment, tt.history); got != tt.want {
				t.Errorf("Next(%v, %v, %v) = %v, want %v",
					tt.start, tt.increment, tt.history, got, tt.want)
			}
		})
	}
}
