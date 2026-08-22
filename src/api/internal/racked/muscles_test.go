package racked

import (
	"math"
	"testing"
	"time"
)

// muscle tags every set with a group, which mkSets leaves empty.
func muscle(sets []Set, group string) []Set {
	for i := range sets {
		sets[i].MuscleGroup = group
	}
	return sets
}

// bySlice finds one group's slice, failing rather than returning a zero value —
// every group is supposed to be present, so an absence is the bug.
func bySlice(t *testing.T, out []MuscleSlice, group string) MuscleSlice {
	t.Helper()
	for _, m := range out {
		if m.Group == group {
			return m
		}
	}
	t.Fatalf("no slice for %q in %d slices", group, len(out))
	return MuscleSlice{}
}

func TestMuscleSlicesEmptyPeriod(t *testing.T) {
	// Nil, not seven empty rows: an empty period has no shares to report, and
	// the page's own empty state says so far better than a chart of nothing.
	if out := muscleSlices(nil); out != nil {
		t.Errorf("muscleSlices(nil) = %v, want nil", out)
	}
}

// The point of the section: a group nobody trained still gets a row.
func TestMuscleSlicesKeepsUntrainedGroups(t *testing.T) {
	sets := muscle(mkSets(1, day(2026, time.March, 2), 1, "Squat", 5, 5, 200), "legs")

	out := muscleSlices(sets)

	if len(out) != len(muscleGroupOrder) {
		t.Fatalf("got %d slices, want one per group (%d)", len(out), len(muscleGroupOrder))
	}
	legs := bySlice(t, out, "legs")
	if !legs.Trained || legs.Sets != 5 {
		t.Errorf("legs = %+v, want trained with 5 sets", legs)
	}
	core := bySlice(t, out, "core")
	if core.Trained || core.Sets != 0 || core.VolumeLb != 0 || core.Share != 0 {
		t.Errorf("core = %+v, want an untrained, empty slice", core)
	}
}

// Trained groups lead, and the untrained ones trail in the taxonomy's own
// reading order rather than in whatever order the map yielded.
func TestMuscleSlicesOrderedByVolumeThenReadingOrder(t *testing.T) {
	sets := muscle(mkSets(1, day(2026, time.March, 2), 1, "Squat", 5, 5, 200), "legs")
	sets = append(sets,
		muscle(mkSets(1, day(2026, time.March, 2), 2, "Bench Press", 5, 5, 100), "chest")...)

	out := muscleSlices(sets)

	if out[0].Group != "legs" || out[1].Group != "chest" {
		t.Fatalf("leading groups = %q, %q; want legs then chest", out[0].Group, out[1].Group)
	}
	var rest []string
	for _, m := range out[2:] {
		rest = append(rest, m.Group)
	}
	want := []string{"back", "shoulders", "arms", "core", "other"}
	for i := range want {
		if rest[i] != want[i] {
			t.Fatalf("untrained groups = %v, want %v", rest, want)
		}
	}
}

// The slices divide the headline rather than sampling it, which is what lets a
// surface put a group next to the total. A movement belongs to exactly one
// group, so there is nothing to double-count.
func TestMuscleSlicesSumToTheReportTotal(t *testing.T) {
	sets := muscle(mkSets(1, day(2026, time.March, 2), 1, "Squat", 5, 5, 200), "legs")
	sets = append(sets,
		muscle(mkSets(1, day(2026, time.March, 2), 2, "Bench Press", 5, 5, 100), "chest")...)
	sets = append(sets,
		muscle(assist(mkSets(1, day(2026, time.March, 2), 3, "Barbell Curl", 3, 10, 40)), "arms")...)

	rep := Build(Input{
		Kind:  PeriodMonth,
		Start: day(2026, time.March, 1),
		End:   day(2026, time.March, 31),
		Sets:  sets,
	})

	var volume, share float64
	var setCount, reps int
	for _, m := range rep.Muscles {
		volume += m.VolumeLb
		share += m.Share
		setCount += m.Sets
		reps += m.Reps
	}
	if math.Abs(volume-rep.Totals.VolumeLb) > 0.001 {
		t.Errorf("muscle volume = %v, want the report total %v", volume, rep.Totals.VolumeLb)
	}
	if math.Abs(share-1) > 0.001 {
		t.Errorf("shares sum to %v, want 1", share)
	}
	if setCount != rep.Totals.Sets || reps != rep.Totals.Reps {
		t.Errorf("sets/reps = %d/%d, want %d/%d",
			setCount, reps, rep.Totals.Sets, rep.Totals.Reps)
	}
}

// Assistance is what most lifters train their arms with, so a version that
// counted only the program's own prescription would report that nobody has any.
func TestMuscleSlicesCountAssistance(t *testing.T) {
	sets := muscle(mkSets(1, day(2026, time.March, 2), 1, "Squat", 5, 5, 200), "legs")
	sets = append(sets,
		muscle(assist(mkSets(1, day(2026, time.March, 2), 3, "Barbell Curl", 3, 10, 40)), "arms")...)

	arms := bySlice(t, muscleSlices(sets), "arms")

	if !arms.Trained || arms.VolumeLb != 3*10*40 {
		t.Errorf("arms = %+v, want the curl's volume counted", arms)
	}
}

// Lifts is what separates a month of nothing but squats from a month of five
// different leg movements.
func TestMuscleSlicesCountDistinctLifts(t *testing.T) {
	sets := muscle(mkSets(1, day(2026, time.March, 2), 1, "Squat", 5, 5, 200), "legs")
	// The same lift again, in a second session — one movement, not two.
	sets = append(sets,
		muscle(mkSets(2, day(2026, time.March, 4), 1, "Squat", 5, 5, 205), "legs")...)
	sets = append(sets,
		muscle(mkSets(2, day(2026, time.March, 4), 4, "Leg Press", 3, 10, 300), "legs")...)

	legs := bySlice(t, muscleSlices(sets), "legs")

	if legs.Lifts != 2 {
		t.Errorf("legs covered %d lifts, want 2", legs.Lifts)
	}
}

// A group added by a later migration must not have its tonnage dropped on the
// floor — a missing slice is a report whose shares no longer sum to one.
func TestMuscleSlicesKeepsAnUnknownGroup(t *testing.T) {
	sets := muscle(mkSets(1, day(2026, time.March, 2), 1, "Squat", 5, 5, 200), "legs")
	sets = append(sets,
		muscle(mkSets(1, day(2026, time.March, 2), 9, "Sled Push", 3, 1, 400), "conditioning")...)

	out := muscleSlices(sets)

	extra := bySlice(t, out, "conditioning")
	if !extra.Trained || extra.Sets != 3 {
		t.Errorf("conditioning = %+v, want trained with 3 sets", extra)
	}
	var share float64
	for _, m := range out {
		share += m.Share
	}
	if math.Abs(share-1) > 0.001 {
		t.Errorf("shares sum to %v, want 1", share)
	}
}

func TestUntrainedMusclesNamesTheGaps(t *testing.T) {
	sets := muscle(mkSets(1, day(2026, time.March, 2), 1, "Squat", 5, 5, 200), "legs")
	rep := Build(Input{
		Kind:  PeriodMonth,
		Start: day(2026, time.March, 1),
		End:   day(2026, time.March, 31),
		Sets:  sets,
	})

	got := rep.UntrainedMuscles()
	want := []string{"chest", "back", "shoulders", "arms", "core", "other"}
	if len(got) != len(want) {
		t.Fatalf("untrained = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("untrained = %v, want %v", got, want)
		}
	}

	// And nothing to say about a period with no work in it at all: Muscles is
	// nil there, so the surfaces omit the section rather than announcing that a
	// lifter trained none of the seven groups.
	quiet := Build(Input{
		Kind:  PeriodMonth,
		Start: day(2026, time.March, 1),
		End:   day(2026, time.March, 31),
	})
	if got := quiet.UntrainedMuscles(); got != nil {
		t.Errorf("untrained on an empty period = %v, want nil", got)
	}
}
