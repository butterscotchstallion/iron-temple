package racked

import (
	"strings"
	"testing"
	"time"
)

// A millionth pound is only news in the month it is passed.
func TestVolumeMilestoneDatedToTheSessionThatCrossedIt(t *testing.T) {
	// Two sessions of 5 x 5 x 200 lb, 5,000 lb each.
	var sets []Set
	sets = append(sets, mkSets(1, day(2026, time.March, 2), 1, "Squat", 5, 5, 200)...)
	sets = append(sets, mkSets(2, day(2026, time.March, 9), 1, "Squat", 5, 5, 200)...)

	// Standing at 96,000 lb, the first session lands on 101,000 and crosses.
	got := volumeMilestones(groupSessions(sets), 96_000)
	if len(got) != 1 {
		t.Fatalf("got %d milestones, want 1", len(got))
	}
	if !got[0].PerformedOn.Equal(day(2026, time.March, 2)) {
		t.Fatalf("dated %v, want the 2nd", got[0].PerformedOn)
	}
	if got[0].ValueLb != 100_000 {
		t.Fatalf("value = %v, want 100000", got[0].ValueLb)
	}
	if !strings.Contains(got[0].Label, "100,000") {
		t.Fatalf("label = %q, want a grouped number", got[0].Label)
	}
}

func TestVolumeMilestoneNotRepeated(t *testing.T) {
	sets := mkSets(1, day(2026, time.March, 2), 1, "Squat", 5, 5, 200)
	// Already past the mark before the period opened.
	if got := volumeMilestones(groupSessions(sets), 150_000); len(got) != 0 {
		t.Fatalf("got %+v, want no milestone already behind the lifter", got)
	}
}

func TestPlateMilestones(t *testing.T) {
	t.Run("first time at a named weight", func(t *testing.T) {
		sets := mkSets(1, day(2026, time.March, 2), 2, "Bench Press", 1, 5, 225)
		got := plateMilestones(groupSessions(sets), map[int32]float64{2: 220})

		if len(got) != 1 {
			t.Fatalf("got %d milestones, want 1", len(got))
		}
		if got[0].Label != "First 225 lb Bench Press" {
			t.Fatalf("label = %q", got[0].Label)
		}
	})

	t.Run("not a first if the lifter was already there", func(t *testing.T) {
		sets := mkSets(1, day(2026, time.March, 2), 2, "Bench Press", 1, 5, 225)
		if got := plateMilestones(groupSessions(sets), map[int32]float64{2: 225}); len(got) != 0 {
			t.Fatalf("got %+v, want none", got)
		}
	})

	t.Run("a big jump crosses every mark it passes", func(t *testing.T) {
		sets := mkSets(1, day(2026, time.March, 2), 1, "Deadlift", 1, 3, 320)
		got := plateMilestones(groupSessions(sets), nil)
		if len(got) != 3 {
			t.Fatalf("got %d milestones, want 135/225/315", len(got))
		}
	})

	t.Run("only the first session that reaches it", func(t *testing.T) {
		var sets []Set
		sets = append(sets, mkSets(1, day(2026, time.March, 2), 1, "Squat", 1, 5, 225)...)
		sets = append(sets, mkSets(2, day(2026, time.March, 9), 1, "Squat", 1, 5, 230)...)

		got := plateMilestones(groupSessions(sets), nil)
		for _, m := range got {
			if m.ValueLb == 225 && !m.PerformedOn.Equal(day(2026, time.March, 2)) {
				t.Fatalf("225 dated %v, want the 2nd", m.PerformedOn)
			}
		}
		if len(got) != 2 {
			t.Fatalf("got %d milestones, want 135 and 225 once each", len(got))
		}
	})
}

func TestMilestonesSortedChronologically(t *testing.T) {
	var sets []Set
	sets = append(sets, mkSets(1, day(2026, time.March, 2), 1, "Squat", 1, 5, 225)...)
	sets = append(sets, mkSets(2, day(2026, time.March, 9), 1, "Squat", 1, 5, 315)...)

	got := milestones(groupSessions(sets), Baseline{})
	for i := 1; i < len(got); i++ {
		if got[i].PerformedOn.Before(got[i-1].PerformedOn) {
			t.Fatalf("milestone %d predates %d", i, i-1)
		}
	}
}
