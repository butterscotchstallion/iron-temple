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
//
// BARBELL ONLY. These are facts about a plate set, and they were applied to every
// lift in the catalogue until ladderFor existed.
var plateThresholds = []float64{95, 135, 185, 225, 275, 315, 365, 405, 455, 495}

// dumbbellThresholds are the bells a lifter names, doubled.
//
// Every weight in this app is the WHOLE LOAD — a dumbbell set at 100 lb is a pair
// of 50s, stated at length in migrations 0018 and 0023 — so a ladder of bells has
// to be written here as pairs. 40 is a pair of 20s, 100 a pair of 50s.
//
// The plate ladder was nonsense here, and expensively so: its rungs are odd
// multiples of 5 that halve to numbers no rack has. "First 135 lb Dumbbell Bench
// Press" is 67.5 lb per hand — not a bell that exists on a rack stepping in 5s,
// which is the rack this app assumes (0020 stores dumbbell_step_lb PER BELL for
// exactly that reason). So a lifter pressing the 65s sailed past 135 without being
// told, and was instead told about a weight they could not have lifted.
//
// The top end is sparser than the bottom on purpose, matching how racks are built:
// bells go up in 5s to about 50 and in 10s or 20s after that, so 120/140/160/200
// are pairs of 60/70/80/100.
var dumbbellThresholds = []float64{40, 50, 60, 70, 80, 90, 100, 120, 140, 160, 200}

// stackThresholds are for machines and cables: round numbers, because that is what
// a selectorised stack is labelled in and "I'm on the hundred" is a thing lifters
// say.
//
// The weakest of the four ladders, and the one to tune first if any of them reads
// wrong. A plate set and a dumbbell rack have landmarks that lifters genuinely
// name; a cable stack's numbering is whoever built it, and it differs between two
// machines in the same room. Round hundreds are the least arbitrary thing
// available, not a claim that they are the right ones.
var stackThresholds = []float64{50, 100, 150, 200, 250, 300}

// bodyweightThresholds are what somebody hangs off a belt, not what they load on a
// bar.
//
// A weighted dip or chin-up carries ONE plate on a strap, so the rungs are plates
// rather than pairs: 25, 45, then a couple of each. The plate ladder's 95 would
// have meant a pair of 25s, which is not how the load is attached.
//
// Only ever reached once a lifter enters a weight at all. Unloaded bodyweight work
// logs at 0 and is skipped before any ladder is consulted — see the guards in
// plateMilestones and upcoming — and entering a weight is what opts a lift in
// (0023). A plank will never see these, which is correct: its milestones would be
// seconds, and this app does not record them.
var bodyweightThresholds = []float64{25, 45, 70, 90, 135, 180}

// ladderFor is the ladder of named weights a lift's milestones are measured
// against, chosen by what the movement is loaded with.
//
// One ladder per kind of loading, because "the weights lifters name out loud" is
// not one list. It is a fact about the equipment: plates make 95 and 135, racks
// make pairs of 20s and 50s, stacks make round hundreds, and a dip belt makes
// single plates. Sharing one list across all of them is what produced milestones
// at weights the lifter's gym cannot assemble.
//
// 'band' returns nil — band work logs 0 lb permanently and by design (0023), so
// there is no weight to have a rung. It never gets this far in practice, since the
// callers skip a best of zero first, but nil says so rather than leaving it to
// them.
//
// 'other' and anything unrecognised take the barbell ladder, matching the default
// `exercises.equipment` itself carries and the same wide default arm
// progression.stepFor uses, for its stated reason: their load, when they have one,
// is plates or bells.
//
// Deliberately NOT progression.LadderFor, which answers a different question: that
// is the grid a weight MOVES on — the step and the increment — where this is the
// landmarks worth announcing. A 5 lb bar step would make every rung 95, 100, 105.
func ladderFor(equipment string) []float64 {
	switch equipment {
	case "dumbbell":
		return dumbbellThresholds
	case "machine", "cable":
		return stackThresholds
	case "bodyweight":
		return bodyweightThresholds
	case "band":
		return nil
	default:
		return plateThresholds
	}
}

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

// UpcomingMilestone is a threshold the lifter has NOT reached, and the nearest
// one of its kind.
//
// The mirror of Milestone, and a different thing rather than a variant of it: a
// crossed threshold has a date and a story, and an uncrossed one has a distance.
// Sharing a type would mean a nullable date on one and a nullable target on the
// other, and a client having to work out which half it was handed.
//
// This is the only figure in the report that is about the future. Everything else
// is a reading of what happened; this is the one thing to aim at, which is the
// whole reason it exists — a lifter 20 lb from a first 225 lb squat has something
// to do on Tuesday, and until now nothing told them.
type UpcomingMilestone struct {
	Kind MilestoneKind
	// Label reads exactly as the Milestone's will when it is finally earned, from
	// the same format string — so the thing a lifter was chasing and the thing
	// they are congratulated for are recognisably one.
	Label string
	// TargetLb is the rung; CurrentLb is where they stand now, counting the
	// period. Both are sent rather than only the difference, so a client can draw
	// a bar without having to reconstruct the denominator.
	//
	// Both are the WHOLE LOAD, as every weight in this app is — so on a dumbbell
	// lift they are the pair while Label reads per hand. That is not a discrepancy
	// to fix by halving them here: the ratio a bar is drawn from is the same either
	// way, and a field that changed units by equipment would be worse than one that
	// never does. Equipment is carried so a client can halve at the point it prints
	// a figure, which is the rule the set card follows too.
	TargetLb  float64
	CurrentLb float64
	// Equipment is what the lift is loaded with, so a surface printing "20 lb to
	// go" beside a per-hand label can agree with it. Empty for a volume threshold,
	// which belongs to no lift.
	Equipment string
	// Zero for a volume threshold, which belongs to no single lift — the same
	// convention Milestone uses.
	ExerciseID   int32
	ExerciseName string
}

// UpcomingLimit is how many unreached thresholds the report carries.
//
// Four, which is a reading decision rather than a technical one. A lifter training
// five lifts has a next rung on each of them plus a volume mark, and a list of six
// things to chase is a list nobody chases — the point is the one or two that are
// nearly in hand. Bounded here rather than in the client so that every surface
// agrees about what "closing in" means.
const UpcomingLimit = 4

// upcoming reports the nearest thresholds the lifter has not reached, closest
// first, at most limit of them.
//
// # WHAT "CLOSEST" MEANS
//
// Fraction of the way there, not pounds remaining. Those give opposite answers
// and only one of them is useful: 20 lb short of a 225 lb squat is 91% of the way,
// while 180,000 lb short of a million is 82% — but ranked by pounds the squat sits
// behind every volume rung forever, and the volume ladder's steps are so large
// that a plate milestone would never be listed at all.
//
// # WHAT IS DELIBERATELY ABSENT
//
// A lift the lifter has never loaded. A best of zero would claim a first 95 lb on
// every banded and bodyweight movement in the catalogue — migration 0023 seeds
// five of those at 0 lb on purpose — and "95 lb to go" on a banded hip abduction
// is not a goal, it is a category error.
//
// A rung above the top of a ladder. Somebody squatting 500 lb has passed every
// plate threshold there is, and the honest answer is a shorter list rather than an
// invented 545. Same for a lifter with no history: nothing to report, reported as
// nothing.
//
// And a lift not trained during the period — see the check below for why that is a
// feature rather than a shortfall.
func upcoming(sessions []session, base Baseline, limit int) []UpcomingMilestone {
	var out []UpcomingMilestone

	// Lifetime to NOW, which is the baseline plus everything in the period — the
	// same running total volumeMilestones builds and discards. A target measured
	// against the period alone would tell a lifter they are 100,000 lb from their
	// first 100,000 lb in the month after they passed it.
	lifetime := base.VolumeLb
	for _, s := range sessions {
		lifetime += s.VolumeLb()
	}
	// ZERO PROGRESS IS NOT PROGRESS, which is the same rule the plate ladder
	// applies below and worth stating once for both. A lifter who has moved nothing
	// is not closing in on their first 100,000 lb; telling them they are 0% of the
	// way there on the day they sign up is a bar drawn at nothing, and it reads as
	// a very long way to go rather than as an invitation.
	if t, ok := nextRung(volumeThresholds, lifetime); ok && lifetime > 0 {
		out = append(out, UpcomingMilestone{
			Kind:      MilestoneVolume,
			Label:     fmt.Sprintf("%s lb lifted, all time", formatLb(t)),
			TargetLb:  t,
			CurrentLb: lifetime,
		})
	}

	// Per lift, the same walk plateMilestones does — the baseline advanced by the
	// period — so the two cannot disagree about what a lift's best is.
	best := copyBests(base.BestWeight)
	names := map[int32]string{}
	// Equipment comes from the period's sets for the same reason the name does —
	// the baseline knows ids and nothing else — and it is only ever read for a lift
	// that has one, since a lift absent from here is skipped below.
	kit := map[int32]string{}
	for _, s := range sessions {
		for _, top := range sessionTops(s) {
			if top.WeightLb > best[top.ExerciseID] {
				best[top.ExerciseID] = top.WeightLb
			}
			names[top.ExerciseID] = top.ExerciseName
			kit[top.ExerciseID] = top.Equipment
		}
	}

	// Sorted by id so the output is stable for lifts that tie on fraction. The
	// map iteration above is not ordered, and a report that reshuffles between
	// identical requests is a report nobody trusts.
	ids := make([]int32, 0, len(best))
	for id := range best {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })

	for _, id := range ids {
		current := best[id]
		if current <= 0 {
			continue
		}
		// ONLY LIFTS TRAINED IN THE PERIOD, which falls out of the baseline knowing
		// ids and not names — and is the behaviour to want anyway. A first 315 lb
		// deadlift is not something a lifter is closing in on if they have not
		// deadlifted since spring; listing it would fill the card with goals nobody
		// is working towards and bury the two they are.
		//
		// Moved above the rung lookup, which needs the equipment this same check
		// establishes we have.
		name, trainedInPeriod := names[id]
		if !trainedInPeriod {
			continue
		}
		t, ok := nextRung(ladderFor(kit[id]), current)
		if !ok {
			continue
		}
		out = append(out, UpcomingMilestone{
			Kind:         MilestonePlate,
			Label:        rungLabel(t, kit[id], name),
			Equipment:    kit[id],
			TargetLb:     t,
			CurrentLb:    current,
			ExerciseID:   id,
			ExerciseName: name,
		})
	}

	sort.SliceStable(out, func(i, j int) bool {
		return out[i].CurrentLb/out[i].TargetLb > out[j].CurrentLb/out[j].TargetLb
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

// nextRung is the lowest threshold strictly above value, and whether there is
// one. Ladders are ascending, so the first match wins.
func nextRung(ladder []float64, value float64) (float64, bool) {
	for _, t := range ladder {
		if value < t {
			return t, true
		}
	}
	return 0, false
}

// NextRung is the lowest named weight above a lift's current best, on the ladder
// its equipment uses, and whether there is one.
//
// Exported for the session screen, which is the one surface that needs this BEFORE
// the lifter has done anything. Every other reader of a rung is looking back at a
// period and gets it through upcoming(); this answers "what is the next thing to
// aim at on this lift" for a workout that has not happened yet, so it takes a bare
// best rather than a Baseline and a slice of sessions.
//
// A best of zero has no rung, for the reason upcoming() spells out at length: a
// lift never loaded would otherwise be told it is 95 lb from a first 95 lb, and on
// a banded movement that is a category error rather than a goal.
//
// The weight in and the weight out are both the WHOLE LOAD, as everywhere else. A
// caller printing them beside a per-hand figure halves them itself.
func NextRung(equipment string, bestLb float64) (float64, bool) {
	if bestLb <= 0 {
		return 0, false
	}
	return nextRung(ladderFor(equipment), bestLb)
}

// rungLabel names a rung the way the lifter holding it would.
//
// Dumbbells read PER HAND. The stored number is the pair, which is what every
// weight in this app is (0018), but a rack is labelled in bells and a lifter says
// "the fifties" — and the set card beside this already prints "50 lb per hand"
// (ExerciseCard.svelte). Announcing "First 100 lb Dumbbell Bench Press" against a
// card reading 50 would be two units ten pixels apart, which is the confusion that
// card's own comment exists to prevent. So the milestone says what the card says.
//
// Everything else is the whole load and reads plainly. Halving is safe for the
// dumbbell ladder specifically because every rung on it is even — it is written as
// doubled bells.
func rungLabel(rung float64, equipment, exerciseName string) string {
	if equipment == "dumbbell" {
		return fmt.Sprintf("First %s lb per hand %s", formatLb(rung/2), exerciseName)
	}
	return fmt.Sprintf("First %s lb %s", formatLb(rung), exerciseName)
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
			for _, t := range ladderFor(top.Equipment) {
				if prev < t && top.WeightLb >= t {
					out = append(out, Milestone{
						Kind:         MilestonePlate,
						PerformedOn:  s.PerformedOn,
						Label:        rungLabel(t, top.Equipment, top.ExerciseName),
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
