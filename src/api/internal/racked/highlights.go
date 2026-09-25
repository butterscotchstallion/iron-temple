package racked

import (
	"sort"
	"time"

	"gitea.homelab/gitadmin/iron-temple/api/internal/progression"
)

// personalRecords finds the records set during the period, and separately the
// lifts performed for the first time.
//
// The running best starts from the lifter's history, not from zero, so the
// first session of a second year does not award a record on every lift. It then
// advances as the period is walked, which is why the caller must pass sessions
// in chronological order: a record is a record against what came before it, not
// against the period's final state.
//
// A lift with no history at all is a FIRST TIME and not a record, because there
// was nothing to beat. That is the one thing the baseline cannot express: it
// carries what each lift's best was, so a lift the lifter has never touched is
// simply missing from it, and the zero a missing key reads as is cleared by any
// weight. See FirstTime for why the two are separate types.
//
// Presence in the map is what decides, NOT a best of zero. weight_lb permits 0,
// so a bodyweight lift has a legitimate best of zero and RackedExerciseBaseline
// returns a row for it; reading "best is 0" as "never done it" would make every
// chin-up session a first time forever. `seen` is therefore tracked separately
// from bestWeight and set unconditionally — bestWeight only moves when a weight
// improves, which for a 0 lb lift is never.
//
// At most one record of each kind per lift per session. A 5x5 at a new weight is
// one achievement, and reporting it five times would bury the other four lifts.
// A weight record suppresses the estimated-max record it implies — the heavier
// bar is the better story — but still advances the estimated best, so the next
// session has a real bar to clear.
func personalRecords(sessions []session, base Baseline) ([]PR, []FirstTime) {
	bestWeight := copyBests(base.BestWeight)
	bestE1RM := copyBests(base.BestE1RM)
	seen := make(map[int32]bool, len(base.BestWeight))
	for id := range base.BestWeight {
		seen[id] = true
	}

	var out []PR
	var firsts []FirstTime
	for _, sess := range sessions {
		tops := sessionTops(sess)
		for _, top := range tops {
			prevWeight := bestWeight[top.ExerciseID]
			prevE1RM := bestE1RM[top.ExerciseID]

			switch {
			case !seen[top.ExerciseID]:
				firsts = append(firsts, FirstTime{
					PerformedOn:  sess.PerformedOn,
					ExerciseID:   top.ExerciseID,
					ExerciseName: top.ExerciseName,
					WeightLb:     top.WeightLb,
					Reps:         top.WeightReps,
				})
			case top.WeightLb > prevWeight:
				out = append(out, PR{
					Kind:         PRWeight,
					PerformedOn:  sess.PerformedOn,
					ExerciseID:   top.ExerciseID,
					ExerciseName: top.ExerciseName,
					WeightLb:     top.WeightLb,
					Reps:         top.WeightReps,
					ValueLb:      top.WeightLb,
					PreviousLb:   prevWeight,
				})
			case top.E1RMLb > prevE1RM:
				out = append(out, PR{
					Kind:         PREstimated,
					PerformedOn:  sess.PerformedOn,
					ExerciseID:   top.ExerciseID,
					ExerciseName: top.ExerciseName,
					WeightLb:     top.E1RMWeightLb,
					Reps:         top.E1RMReps,
					ValueLb:      top.E1RMLb,
					PreviousLb:   prevE1RM,
				})
			}

			// Unconditional, and before the bests: from here on this lift has a
			// history, so a heavier session later in the period is a record rather
			// than a second first time. The bests below are conditional because a
			// lighter session must not lower them.
			seen[top.ExerciseID] = true

			if top.WeightLb > prevWeight {
				bestWeight[top.ExerciseID] = top.WeightLb
			}
			if top.E1RMLb > prevE1RM {
				bestE1RM[top.ExerciseID] = top.E1RMLb
			}
		}
	}
	return out, firsts
}

func copyBests(in map[int32]float64) map[int32]float64 {
	out := make(map[int32]float64, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// sessionTop is one lift's best showing within a single session.
type sessionTop struct {
	ExerciseID   int32
	ExerciseName string
	// Equipment picks the milestone ladder — see ladderFor. Carried on the top
	// rather than looked up again, because the sets it was reduced from all agree
	// about it and the callers that need it have only this.
	Equipment    string
	WeightLb     float64
	WeightReps   int
	E1RMLb       float64
	E1RMWeightLb float64
	E1RMReps     int
}

// sessionTops reduces a session to one entry per lift, returned in a stable
// order so that two lifts setting records in the same session always appear the
// same way round.
//
// A set with no logged reps is skipped, and this is the rail rather than the
// gate. Every path that reaches here already filters: RackedPeriodSets and
// RackedExerciseBaseline both carry `AND ss.actual_reps > 0`, and the recap
// handler splits its rows at recap.go's `if reps <= 0 { continue }` — which is
// the split RecapSessionSets promises when it explains why it is the one query
// that returns unlogged rows at all. So no live caller can pass one in.
//
// It is here anyway because this function decides five different things — records,
// first times, plate milestones, the rung a lifter is closing in on, and the
// recap's per-lift rows — and a bar that was loaded and walked away from must not
// become any of them. Set.E1RM guards its own `Reps <= 0` for the same reason,
// which is why an unlogged row could only ever have reached the WEIGHT half.
// sessionLifts also documents "a lift with no logged set is absent: it was not
// performed, and a row of zeroes claims otherwise" — a promise it can only keep
// if this function keeps it first.
func sessionTops(sess session) []sessionTop {
	byID := map[int32]*sessionTop{}
	for _, set := range sess.Sets {
		if set.Reps <= 0 {
			continue
		}
		t, ok := byID[set.ExerciseID]
		if !ok {
			t = &sessionTop{
				ExerciseID:   set.ExerciseID,
				ExerciseName: set.ExerciseName,
				Equipment:    set.Equipment,
			}
			byID[set.ExerciseID] = t
		}
		if set.WeightLb > t.WeightLb {
			t.WeightLb, t.WeightReps = set.WeightLb, set.Reps
		}
		if e := set.E1RM(); e > t.E1RMLb {
			t.E1RMLb, t.E1RMWeightLb, t.E1RMReps = e, set.WeightLb, set.Reps
		}
	}
	out := make([]sessionTop, 0, len(byID))
	for _, t := range byID {
		out = append(out, *t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ExerciseName < out[j].ExerciseName })
	return out
}

// heaviestSet is the single heaviest set of the period. Ties break toward more
// reps, then toward the earlier date — the first time a lifter got there.
func heaviestSet(sets []Set) *SetHighlight {
	var best *Set
	for i := range sets {
		s := sets[i]
		if best == nil || better(s, *best) {
			cur := s
			best = &cur
		}
	}
	if best == nil {
		return nil
	}
	return &SetHighlight{
		PerformedOn:  best.PerformedOn,
		ExerciseID:   best.ExerciseID,
		ExerciseName: best.ExerciseName,
		WeightLb:     best.WeightLb,
		Reps:         best.Reps,
	}
}

func better(a, b Set) bool {
	if a.WeightLb != b.WeightLb {
		return a.WeightLb > b.WeightLb
	}
	if a.Reps != b.Reps {
		return a.Reps > b.Reps
	}
	return a.PerformedOn.Before(b.PerformedOn)
}

// fastestSession is the quickest complete session of the period. Sessions the
// lifter never finished have no duration and are skipped — see session.Duration.
func fastestSession(sessions []session) *SessionHighlight {
	var best *session
	var bestDur time.Duration
	for i := range sessions {
		d := sessions[i].Duration()
		if d <= 0 {
			continue
		}
		if best == nil || d < bestDur {
			cur := sessions[i]
			best, bestDur = &cur, d
		}
	}
	if best == nil {
		return nil
	}
	return &SessionHighlight{
		SessionID:      best.ID,
		PerformedOn:    best.PerformedOn,
		ProgramDayName: best.DayName,
		Duration:       bestDur,
		VolumeLb:       best.VolumeLb(),
		Sets:           len(best.Sets),
	}
}

// deloads finds the drops in a lift's working weight and reports whether the
// lifter climbed back.
//
// A drop counts only at a full bar increment or more, so that a lighter back-off
// set or a rounding difference is not mistaken for stalling out. Recovery is
// judged within the period: a deload the lifter has not yet answered is still
// part of the story, and saying so is the point of including this at all.
//
// Assistance work is skipped, alone among the statistics in this package. A
// deload is a claim about a progression, and assistance has no progression
// behind it: 0009 gave program_day_assistance a plain weight_lb column with no
// engine on it, so the weight is whatever the lifter last logged. Picking up the
// 15s instead of the 20s because the 20s were taken is not stalling out, and
// reporting it as "still climbing" tells the lifter something untrue about a
// month that went fine. Records and most-improved read assistance happily —
// those are claims about what was lifted, and what was lifted was lifted.
func deloads(sessions []session) []Deload {
	series := liftSeries(sessions)
	var out []Deload
	for _, s := range series {
		if s.IsAssistance {
			continue
		}
		for i := 1; i < len(s.Points); i++ {
			from, to := s.Points[i-1].TopWeightLb, s.Points[i].TopWeightLb
			if from-to < progression.BarIncrementLb {
				continue
			}
			d := Deload{
				ExerciseID:   s.ExerciseID,
				ExerciseName: s.ExerciseName,
				PerformedOn:  s.Points[i].PerformedOn,
				FromLb:       from,
				ToLb:         to,
			}
			for j := i + 1; j < len(s.Points); j++ {
				if s.Points[j].TopWeightLb >= from {
					d.Recovered = true
					d.RecoveredOn = s.Points[j].PerformedOn
					break
				}
			}
			out = append(out, d)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].PerformedOn.Equal(out[j].PerformedOn) {
			return out[i].PerformedOn.Before(out[j].PerformedOn)
		}
		return out[i].ExerciseName < out[j].ExerciseName
	})
	return out
}
