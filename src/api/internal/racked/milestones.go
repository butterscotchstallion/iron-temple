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

// plateThresholds are the working weights lifters actually name, each a round
// number of 45 lb plates a side on a 45 lb bar.
var plateThresholds = []float64{135, 225, 315, 405, 495}

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
