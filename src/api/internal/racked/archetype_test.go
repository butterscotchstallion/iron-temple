package racked

import (
	"testing"
	"time"
)

// mondaysFrom builds n weekly sessions of the given duration starting at from.
func weeklySessions(from time.Time, n int, d time.Duration, step int) []Set {
	var sets []Set
	for i := 0; i < n; i++ {
		on := from.AddDate(0, 0, i*step)
		s := mkSets(int32(i+1), on, 1, "Squat", 5, 5, 200)
		if d > 0 {
			s = finish(s, d)
		}
		sets = append(sets, s...)
	}
	return sets
}

func TestArchetype(t *testing.T) {
	start, end := Bounds(PeriodMonth, day(2026, time.March, 15))

	cases := []struct {
		name string
		sets []Set
		want string
	}{
		{
			// Every other day beats what any seeded program prescribes.
			name: "frequency outranks everything",
			sets: weeklySessions(day(2026, time.March, 1), 20, time.Hour, 1),
			want: "The Machine",
		},
		{
			name: "long sessions",
			sets: weeklySessions(day(2026, time.March, 2), 8, 90*time.Minute, 3),
			want: "The Grinder",
		},
		{
			name: "short sessions",
			sets: weeklySessions(day(2026, time.March, 2), 8, 30*time.Minute, 3),
			want: "The Sprinter",
		},
		{
			// Saturdays only, unfinished so pace cannot claim it first.
			name: "weekends only",
			sets: weeklySessions(day(2026, time.March, 7), 4, 0, 7),
			want: "The Weekend Warrior",
		},
		{
			name: "steady midweek training",
			sets: weeklySessions(day(2026, time.March, 2), 9, 0, 3),
			want: "The Metronome",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := archetype(groupSessions(tc.sets), start, end)
			if got.Name != tc.want {
				t.Fatalf("archetype = %q, want %q", got.Name, tc.want)
			}
			if got.Description == "" {
				t.Fatal("archetype has no description")
			}
		})
	}
}

func TestArchetypeEmpty(t *testing.T) {
	start, end := Bounds(PeriodMonth, day(2026, time.March, 15))
	if got := archetype(nil, start, end); got.Name != "" {
		t.Fatalf("archetype = %q, want none", got.Name)
	}
}

// Pace must not classify from one or two timed sessions; below the threshold
// the lifter falls through to a frequency- or weekday-based label instead.
func TestArchetypeIgnoresThinPaceEvidence(t *testing.T) {
	start, end := Bounds(PeriodMonth, day(2026, time.March, 15))
	sets := weeklySessions(day(2026, time.March, 2), 2, 90*time.Minute, 3)

	if got := archetype(groupSessions(sets), start, end); got.Name == "The Grinder" {
		t.Fatalf("archetype = %q, want pace ignored below %d timed sessions",
			got.Name, minSessionsForPace)
	}
}

func TestWeeksBetweenNeverBelowOne(t *testing.T) {
	d := day(2026, time.March, 2)
	if got := weeksBetween(d, d); got != 1 {
		t.Fatalf("weeksBetween(same day) = %v, want 1", got)
	}
}

func TestAverageMinutesSkipsUnfinished(t *testing.T) {
	var sets []Set
	sets = append(sets, finish(mkSets(1, day(2026, time.March, 2), 1, "Squat", 1, 5, 100), time.Hour)...)
	sets = append(sets, mkSets(2, day(2026, time.March, 9), 1, "Squat", 1, 5, 100)...)

	avg, n := averageMinutes(groupSessions(sets))
	if n != 1 || avg != 60 {
		t.Fatalf("averageMinutes = %v over %d sessions, want 60 over 1", avg, n)
	}
}
