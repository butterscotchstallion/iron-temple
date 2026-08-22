package racked

import "sort"

// Where the period's work landed on the body.
//
// The exercise library has carried a muscle_group on every movement since 0009,
// and until now nothing but the library's own browse screen read it. The recap
// sliced a period by LIFT, which answers "what did you spend the month doing"
// but not "what did you spend it training" — and those differ in the way that
// matters. Four of the five lifts a StrongLifts day prescribes are pushes and
// pulls; a lifter can spend a diligent month never once training their core and
// see nothing about it in a list of lifts sorted by tonnage, because the thing
// worth noticing is an absence and absences do not appear in a ranking.
//
// So this list is deliberately NOT a ranking of what was trained. Every group in
// the taxonomy appears, including the ones with nothing against them, because a
// zero is the whole point of the section.
//
// One honest limitation, stated here rather than implied: a movement has exactly
// one muscle group, its primary mover. A bench press is chest, and the triceps
// and front delts it also trains are credited nowhere. Splitting a set across
// several muscles would need per-exercise contribution weights that the library
// does not hold and that nobody is going to maintain by hand — and the coarse
// version already answers the question being asked, which is whether a whole
// region of the body went untouched.

// muscleGroupOrder is the taxonomy in the order a lifter reads it: the
// conventional push/pull/legs grouping rather than alphabetical, matching
// MUSCLE_GROUPS in the UI's library.ts. It is also the complete set of values
// the exercises.muscle_group CHECK constraint permits.
//
// "other" is last because it is the fallback bucket that custom movements land
// in by default.
var muscleGroupOrder = []string{
	"chest",
	"back",
	"legs",
	"shoulders",
	"arms",
	"core",
	"other",
}

// MuscleSlice is one muscle group's share of the period's volume.
//
// The counters are the same ones Totals and LiftSlice carry, so a surface can
// put a group beside the headline and have the two agree: summed over every
// slice, VolumeLb is Totals.VolumeLb and Share is 1.
type MuscleSlice struct {
	// Group is a value from muscleGroupOrder — a taxonomy key, not a label. The
	// surfaces capitalise it; the report does not, so the page and the email
	// cannot disagree about the wording by each inventing their own.
	Group    string
	VolumeLb float64
	Sets     int
	Reps     int
	// Lifts is how many distinct movements the group covered. It is what
	// separates "a third of your volume, all of it squats" from "a third of your
	// volume across five leg movements", which are different months.
	Lifts int
	// Share is of the period's whole volume, so the slices sum to 1 for any
	// period that moved weight.
	Share float64
	// Trained is false for a group with nothing logged against it. Equivalent to
	// Sets == 0, and named because that is what the surfaces actually branch on:
	// an untrained group is a different kind of row, not a short bar.
	Trained bool
}

// muscleSlices divides the period between the muscle groups its work trained.
//
// Assistance counts, and is not distinguished. That is the point of slicing this
// way rather than by lift: a lifter who trains their arms does it entirely
// through work they bolted on themselves, so a version that reported only the
// program's own prescription would report that nobody has arms.
func muscleSlices(sets []Set) []MuscleSlice {
	if len(sets) == 0 {
		return nil
	}

	byGroup := map[string]*MuscleSlice{}
	lifts := map[string]map[int32]bool{}
	// Seeded with the whole taxonomy, so a group nobody trained still gets a row.
	// Building it from the sets alone would leave exactly the gap this section
	// exists to show.
	order := append([]string(nil), muscleGroupOrder...)
	for _, g := range order {
		byGroup[g] = &MuscleSlice{Group: g}
		lifts[g] = map[int32]bool{}
	}

	var total float64
	for _, s := range sets {
		cur, ok := byGroup[s.MuscleGroup]
		if !ok {
			// A value the CHECK constraint permits but this file has not been
			// told about — a taxonomy grown by a later migration. Report it
			// rather than drop the tonnage on the floor: a slice missing from
			// the list is a report whose shares no longer sum to one.
			cur = &MuscleSlice{Group: s.MuscleGroup}
			byGroup[s.MuscleGroup] = cur
			lifts[s.MuscleGroup] = map[int32]bool{}
			order = append(order, s.MuscleGroup)
		}
		cur.VolumeLb += s.VolumeLb()
		cur.Sets++
		cur.Reps += s.Reps
		cur.Trained = true
		lifts[s.MuscleGroup][s.ExerciseID] = true
		total += s.VolumeLb()
	}

	// Rank within the canonical order, so ties — and the untrained groups, which
	// are all tied at zero — come out in the order a lifter reads them rather
	// than in whichever order the map happened to yield.
	rank := make(map[string]int, len(order))
	for i, g := range order {
		rank[g] = i
	}

	out := make([]MuscleSlice, 0, len(order))
	for _, g := range order {
		v := byGroup[g]
		v.Lifts = len(lifts[g])
		if total > 0 {
			v.Share = v.VolumeLb / total
		}
		out = append(out, *v)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].VolumeLb != out[j].VolumeLb {
			return out[i].VolumeLb > out[j].VolumeLb
		}
		return rank[out[i].Group] < rank[out[j].Group]
	})
	return out
}

// UntrainedMuscles is the groups the period holds no work for, in reading order.
//
// Derived rather than stored: it is Muscles filtered, and two fields that can
// disagree about the same fact is one more than a report needs. The email leans
// on it for a sentence, and it reads better as a method than as the same filter
// written out in a template.
func (r Report) UntrainedMuscles() []string {
	var out []string
	for _, m := range r.Muscles {
		if !m.Trained {
			out = append(out, m.Group)
		}
	}
	return out
}
