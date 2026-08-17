package racked

import (
	"fmt"
	"sort"
	"time"
)

// MilestoneKind distinguishes the two kinds of threshold worth celebrating.
type MilestoneKind string

const (
	// MilestoneVolume is a lifetime tonnage threshold.
	MilestoneVolume MilestoneKind = "volume"
	// MilestonePlate is a round working weight on one lift.
	MilestonePlate MilestoneKind = "plate"
)

// Milestone is a threshold crossed during the period.
type Milestone struct {
	Kind         MilestoneKind
	PerformedOn  time.Time
	Label        string
	ValueLb      float64
	ExerciseID   int32
	ExerciseName string
}

// volumeThresholds are lifetime tonnage marks. They step by roughly half an
// order of magnitude so that a lifter meets one every so often rather than
// every month early on and never again after.
var volumeThresholds = []float64{
	100_000, 250_000, 500_000, 1_000_000, 2_500_000, 5_000_000, 10_000_000, 25_000_000,
}

// plateThresholds are the working weights lifters actually name out loud.
//
// The ladder has to cover every lift, not just the two heaviest. Starting at 135
// — a 45 lb plate a side — meant an overhead press could progress for a year
// without passing a single milestone, while a deadlift collected four, and a
// lighter lifter got none at all. The recap then said nothing about most of the
// lifts in it.
//
// So the rungs are every landmark the plate set actually makes: 95 is a pair of
// 25s on the bar, 135 a pair of 45s, and from there each step adds a pair of 25s
// or 45s. That is the sequence someone announces without thinking about it,
// which is the only real test for a list like this.
//
// Each is awarded once per lift, ever — plateMilestones compares against the
// lifter's all-time best before the period — so a dense low end costs a beginner
// a handful of one-off milestones in their first months and nothing after.
var plateThresholds = []float64{95, 135, 185, 225, 275, 315, 365, 405, 455, 495}

// milestones reports the thresholds crossed inside the period.
//
// Both kinds need the lifter's history to be news: a millionth pound is only a
// milestone in the month it is passed, and a first 225 is only a first if the
// lifter had not already done it in March. That is what Baseline carries, and
// why a recap cannot be computed from the period alone.
func milestones(sessions []session, base Baseline) []Milestone {
	out := volumeMilestones(sessions, base.VolumeLb)
	out = append(out, plateMilestones(sessions, base.BestWeight)...)
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].PerformedOn.Equal(out[j].PerformedOn) {
			return out[i].PerformedOn.Before(out[j].PerformedOn)
		}
		return out[i].ValueLb > out[j].ValueLb
	})
	return out
}

// volumeMilestones walks the period accumulating on top of the lifetime total,
// so each threshold is dated to the session that carried it over rather than to
// the period as a whole.
func volumeMilestones(sessions []session, lifetimeBefore float64) []Milestone {
	var out []Milestone
	running := lifetimeBefore
	for _, s := range sessions {
		before := running
		running += s.VolumeLb()
		for _, t := range volumeThresholds {
			if before < t && running >= t {
				out = append(out, Milestone{
					Kind:        MilestoneVolume,
					PerformedOn: s.PerformedOn,
					Label:       fmt.Sprintf("%s lb lifted, all time", formatLb(t)),
					ValueLb:     t,
				})
			}
		}
	}
	return out
}

// plateMilestones reports the first time each lift reached a named weight.
func plateMilestones(sessions []session, bestBefore map[int32]float64) []Milestone {
	best := copyBests(bestBefore)
	var out []Milestone
	for _, s := range sessions {
		for _, top := range sessionTops(s) {
			prev := best[top.ExerciseID]
			if top.WeightLb <= prev {
				continue
			}
			for _, t := range plateThresholds {
				if prev < t && top.WeightLb >= t {
					out = append(out, Milestone{
						Kind:         MilestonePlate,
						PerformedOn:  s.PerformedOn,
						Label:        fmt.Sprintf("First %s lb %s", formatLb(t), top.ExerciseName),
						ValueLb:      t,
						ExerciseID:   top.ExerciseID,
						ExerciseName: top.ExerciseName,
					})
				}
			}
			best[top.ExerciseID] = top.WeightLb
		}
	}
	return out
}
