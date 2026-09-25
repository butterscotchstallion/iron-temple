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
		// 95, 135, 185, 225, 275, 315 — everything the bar passed on the way.
		if len(got) != 6 {
			t.Fatalf("got %d milestones, want 6", len(got))
		}
	})

	// The reason the ladder starts at 95 rather than 135. An overhead press can
	// progress for a year without reaching 135, and used to collect nothing while
	// a deadlift collected four.
	t.Run("a press earns milestones too", func(t *testing.T) {
		sets := mkSets(1, day(2026, time.March, 2), 3, "Overhead Press", 1, 5, 100)
		got := plateMilestones(groupSessions(sets), nil)
		if len(got) != 1 || got[0].Label != "First 95 lb Overhead Press" {
			t.Fatalf("got %+v, want a single 95 lb press milestone", got)
		}
	})

	// And the reason it stops there: a lighter lifter still working up to the
	// first rung gets nothing, rather than a milestone for existing.
	t.Run("below the first rung there is nothing to celebrate", func(t *testing.T) {
		sets := mkSets(1, day(2026, time.March, 2), 3, "Overhead Press", 1, 5, 65)
		if got := plateMilestones(groupSessions(sets), nil); len(got) != 0 {
			t.Fatalf("got %+v, want none below 95 lb", got)
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
		// 95, 135, 185, 225 on the first session; the second adds nothing, because
		// each rung is awarded once per lift ever.
		if len(got) != 4 {
			t.Fatalf("got %d milestones, want 4", len(got))
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

// ---- what is still ahead ----
//
// The forward-looking half. Everything here is about what upcoming() refuses to
// say as much as what it does: a target it cannot justify is worse than silence,
// because a lifter who is told they are 95 lb from a banded hip abduction stops
// reading the section.

func TestUpcomingNamesTheNextVolumeRung(t *testing.T) {
	// 5 x 5 x 200 = 5,000 lb on top of 96,000 leaves the lifter at 101,000 —
	// past the 100,000 rung, so the next one up is 250,000.
	sets := mkSets(1, day(2026, time.March, 2), 1, "Squat", 5, 5, 200)
	got := upcoming(groupSessions(sets), Baseline{VolumeLb: 96_000}, UpcomingLimit)

	found := false
	for _, u := range got {
		if u.Kind != MilestoneVolume {
			continue
		}
		found = true
		if u.TargetLb != 250_000 {
			t.Errorf("target = %v, want 250000", u.TargetLb)
		}
		// Lifetime to NOW, counting the period — not the baseline it opened on.
		if u.CurrentLb != 101_000 {
			t.Errorf("current = %v, want 101000", u.CurrentLb)
		}
		if !strings.Contains(u.Label, "250,000") {
			t.Errorf("label = %q, want a grouped number", u.Label)
		}
	}
	if !found {
		t.Fatalf("no volume rung in %+v", got)
	}
}

func TestUpcomingNamesTheNextPlateRungPerLift(t *testing.T) {
	sets := mkSets(1, day(2026, time.March, 2), 2, "Bench Press", 1, 5, 205)
	got := upcoming(groupSessions(sets), Baseline{BestWeight: map[int32]float64{2: 190}}, UpcomingLimit)

	var plate *UpcomingMilestone
	for i := range got {
		if got[i].Kind == MilestonePlate {
			plate = &got[i]
		}
	}
	if plate == nil {
		t.Fatalf("no plate rung in %+v", got)
	}
	if plate.TargetLb != 225 {
		t.Errorf("target = %v, want 225", plate.TargetLb)
	}
	// The period's top set, which beat the baseline's 190.
	if plate.CurrentLb != 205 {
		t.Errorf("current = %v, want 205", plate.CurrentLb)
	}
	if plate.Label != "First 225 lb Bench Press" {
		t.Errorf("label = %q", plate.Label)
	}
	if plate.ExerciseName != "Bench Press" || plate.ExerciseID != 2 {
		t.Errorf("lift = %d/%q", plate.ExerciseID, plate.ExerciseName)
	}
}

// CLOSEST MEANS FRACTION, NOT POUNDS, and this is the test that pins it. Ranked
// by pounds remaining the squat's 20 lb would win trivially; the point is that it
// still wins when measured the way the code actually measures, and that the
// volume rung — 180,000 lb away but 82% of the way there — is not simply last by
// construction.
func TestUpcomingRanksByFractionCompleteNotPoundsRemaining(t *testing.T) {
	// Squat at 205 of 225 → 91%. Lifetime 820,000 of 1,000,000 → 82%.
	sets := mkSets(1, day(2026, time.March, 2), 1, "Squat", 1, 1, 205)
	got := upcoming(groupSessions(sets), Baseline{
		VolumeLb:   819_795, // + 205 for the single above
		BestWeight: map[int32]float64{1: 200},
	}, UpcomingLimit)

	if len(got) < 2 {
		t.Fatalf("got %+v, want both a plate and a volume rung", got)
	}
	if got[0].Kind != MilestonePlate {
		t.Fatalf("first = %v (%v of %v), want the plate rung",
			got[0].Kind, got[0].CurrentLb, got[0].TargetLb)
	}
	if got[1].Kind != MilestoneVolume {
		t.Fatalf("second = %v, want the volume rung", got[1].Kind)
	}
}

// Migration 0023 seeds five banded and bodyweight movements at 0 lb on purpose.
// "95 lb to go" on a banded hip abduction is a category error, not a goal.
func TestUpcomingIgnoresALiftCarryingNoWeight(t *testing.T) {
	sets := mkSets(1, day(2026, time.March, 2), 9, "Banded Hip Abduction", 3, 20, 0)
	got := upcoming(groupSessions(sets), Baseline{}, UpcomingLimit)

	for _, u := range got {
		if u.ExerciseID == 9 {
			t.Fatalf("a 0 lb lift claimed a rung: %+v", u)
		}
	}
}

// A lift trained heavily in February but not this month is not something the
// lifter is closing in on, and the baseline knows it only as an id anyway.
func TestUpcomingOnlyCoversLiftsTrainedInThePeriod(t *testing.T) {
	sets := mkSets(1, day(2026, time.March, 2), 1, "Squat", 1, 5, 200)
	got := upcoming(groupSessions(sets), Baseline{
		BestWeight: map[int32]float64{1: 200, 7: 300}, // 7 never appears in the period
	}, UpcomingLimit)

	for _, u := range got {
		if u.ExerciseID == 7 {
			t.Fatalf("an untrained lift was listed: %+v", u)
		}
	}
}

// Past the top of a ladder there is no next rung, and inventing one would be the
// only dishonest thing this function could do.
func TestUpcomingInventsNothingAboveTheTopRung(t *testing.T) {
	sets := mkSets(1, day(2026, time.March, 2), 1, "Squat", 1, 1, 500)
	got := upcoming(groupSessions(sets), Baseline{
		VolumeLb:   30_000_000,
		BestWeight: map[int32]float64{1: 500},
	}, UpcomingLimit)

	if len(got) != 0 {
		t.Fatalf("got %+v, want nothing for a lifter past every rung", got)
	}
}

func TestUpcomingIsEmptyForALifterWithNoHistory(t *testing.T) {
	if got := upcoming(nil, Baseline{}, UpcomingLimit); len(got) != 0 {
		t.Fatalf("got %+v, want nothing", got)
	}
}

// The limit is what keeps "closing in" to a couple of things worth chasing rather
// than a rung for every lift in the program plus a tonnage mark.
func TestUpcomingHonoursTheLimit(t *testing.T) {
	var sets []Set
	for ex := int32(1); ex <= 5; ex++ {
		sets = append(sets, mkSets(1, day(2026, time.March, 2), ex,
			"Lift "+string(rune('A'+ex-1)), 1, 5, 200)...)
	}
	got := upcoming(groupSessions(sets), Baseline{VolumeLb: 90_000}, 2)
	if len(got) != 2 {
		t.Fatalf("got %d, want 2 — five lifts and a tonnage mark were available", len(got))
	}
}

// Two lifts level on fraction must come back in the same order every time. The
// bests are walked out of a map, and a report that reshuffles between identical
// requests is one nobody trusts.
func TestUpcomingIsStableAcrossRunsWhenLiftsTie(t *testing.T) {
	var sets []Set
	sets = append(sets, mkSets(1, day(2026, time.March, 2), 3, "Lift C", 1, 5, 200)...)
	sets = append(sets, mkSets(1, day(2026, time.March, 2), 1, "Lift A", 1, 5, 200)...)
	sets = append(sets, mkSets(1, day(2026, time.March, 2), 2, "Lift B", 1, 5, 200)...)

	first := upcoming(groupSessions(sets), Baseline{}, UpcomingLimit)
	for i := 0; i < 5; i++ {
		again := upcoming(groupSessions(sets), Baseline{}, UpcomingLimit)
		for j := range first {
			if again[j].ExerciseID != first[j].ExerciseID {
				t.Fatalf("run %d ordered %d at %d, first run had %d",
					i, again[j].ExerciseID, j, first[j].ExerciseID)
			}
		}
	}
}

// A pair of 50s is a milestone; 135 lb of dumbbell is not a thing.
//
// The plate ladder's rungs halve to bells no rack has — 135 is 67.5 a side — so a
// lifter working up the rack was told about weights they could not assemble and
// not told about the ones they could.
func TestDumbbellMilestonesReadInBells(t *testing.T) {
	// A pair of 50s. Past the plate ladder's 95 too, so if the wrong ladder were
	// consulted this would report that instead.
	sets := mkSets(1, day(2026, time.March, 2), 1, "Dumbbell Bench Press", 3, 8, 100)
	for i := range sets {
		sets[i].Equipment = "dumbbell"
	}

	// From a pair of 45s — one 5 lb step per bell, which is how a rack actually
	// moves, and exactly one rung.
	got := plateMilestones(groupSessions(sets), map[int32]float64{1: 90})
	if len(got) != 1 {
		t.Fatalf("got %+v, want just the pair of 50s", got)
	}
	if got[0].ValueLb != 100 {
		t.Errorf("value = %v, want the rung stored as the whole load", got[0].ValueLb)
	}
	// Per hand, matching what the set card prints beside it.
	if got[0].Label != "First 50 lb per hand Dumbbell Bench Press" {
		t.Errorf("label = %q, want it worded in bells", got[0].Label)
	}
}

// The barbell ladder is unchanged, which the rest of the suite assumes.
func TestBarbellMilestonesAreUnchanged(t *testing.T) {
	sets := mkSets(1, day(2026, time.March, 2), 1, "Squat", 3, 5, 225)
	for i := range sets {
		sets[i].Equipment = "barbell"
	}
	got := plateMilestones(groupSessions(sets), map[int32]float64{1: 220})
	if len(got) != 1 || got[0].ValueLb != 225 {
		t.Fatalf("got %+v, want the 225 rung", got)
	}
	if got[0].Label != "First 225 lb Squat" {
		t.Errorf("label = %q, want the whole load", got[0].Label)
	}
}

// An empty equipment string is what a set assembled without one carries, and it
// has to land somewhere deliberate rather than on whichever arm is first.
func TestAnUnknownEquipmentTakesTheBarbellLadder(t *testing.T) {
	for _, kit := range []string{"", "other", "something-new"} {
		if got := ladderFor(kit); len(got) == 0 || got[0] != 95 {
			t.Errorf("ladderFor(%q) = %v, want the plate ladder", kit, got)
		}
	}
}

// Band work logs 0 lb permanently and by design (0023), so it has no weight to
// have a rung. The callers skip a zero best before asking, but the ladder says so
// itself rather than relying on them.
func TestBandsHaveNoLadder(t *testing.T) {
	if got := ladderFor("band"); got != nil {
		t.Errorf("ladderFor(band) = %v, want nil", got)
	}
}

// A weighted dip hangs ONE plate off a belt, so its rungs are plates and not
// pairs — 95 would have meant a pair of 25s, which is not how it is attached.
func TestBodyweightMilestonesCountPlatesNotPairs(t *testing.T) {
	sets := mkSets(1, day(2026, time.March, 2), 1, "Weighted Dip", 3, 8, 45)
	for i := range sets {
		sets[i].Equipment = "bodyweight"
	}
	got := plateMilestones(groupSessions(sets), map[int32]float64{1: 20})
	if len(got) != 2 {
		t.Fatalf("got %+v, want the 25 and the 45", got)
	}
	if got[0].ValueLb != 25 || got[1].ValueLb != 45 {
		t.Errorf("rungs = %v/%v, want 25 then 45", got[0].ValueLb, got[1].ValueLb)
	}
}

// What a lifter is closing in on has to be the next rung of THEIR equipment, and
// the figure beside it has to agree with the label's units.
func TestUpcomingTargetsTheEquipmentsOwnLadder(t *testing.T) {
	sets := mkSets(1, day(2026, time.March, 2), 1, "Dumbbell Bench Press", 3, 8, 90)
	for i := range sets {
		sets[i].Equipment = "dumbbell"
	}
	base := Baseline{
		BestWeight: map[int32]float64{1: 90},
		BestE1RM:   map[int32]float64{1: 110},
	}

	got := upcoming(groupSessions(sets), base, UpcomingLimit)
	var plate *UpcomingMilestone
	for i := range got {
		if got[i].Kind == MilestonePlate {
			plate = &got[i]
		}
	}
	if plate == nil {
		t.Fatalf("got %+v, want a plate rung to chase", got)
	}
	// A pair of 50s next, not the plate ladder's 95.
	if plate.TargetLb != 100 {
		t.Errorf("target = %v, want the pair of 50s", plate.TargetLb)
	}
	if plate.Equipment != "dumbbell" {
		t.Errorf("equipment = %q, want it carried so a client can halve the figure",
			plate.Equipment)
	}
	if plate.Label != "First 50 lb per hand Dumbbell Bench Press" {
		t.Errorf("label = %q", plate.Label)
	}
}
