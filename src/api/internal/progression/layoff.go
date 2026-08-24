package progression

// Time away from the bar.
//
// The engine in progression.go knows exactly one reason to lower a weight: a
// stall. Three failures at a weight and it drops to 90%. What it cannot see is
// the calendar — stop training for a month and it still prescribes the weight
// you were advancing to the day you stopped, because from its point of view the
// last session is the last session however long ago it happened.
//
// Strength does not wait. So a layoff is the second reason to lower a weight,
// and it deepens with the time off: 10% for each full week away, capped at 50%.
// The cap is where the reduction stops being useful — half your working weight
// is already a warm-up, and no amount of further time off makes the empty bar
// the right place to restart a squat.
//
// This is offered, never imposed. The lifter is asked, and the answer decides
// whether prescribe() applies any of it — nothing here runs on its own. Which is
// also why nothing here is stored: performing the deloaded session puts those
// weights in the lift's history, the linear engine runs back up from them, and
// the layoff is over. There is no state to keep once the answer is acted on.

// The layoff curve. Whole percent rather than fractions, so that LayoffPct
// below can do its arithmetic in integers and divide exactly once. Scaling a
// 0.10 constant by a week count instead answers 0.30000000000000004 at three
// weeks — a number that would reach the wire and then a UI that multiplies it
// by 100 to write a label.
const (
	// LayoffWeeksBeforeDeload is how many full weeks away before there is
	// anything to offer. One: six days off is a rest, seven is a layoff.
	LayoffWeeksBeforeDeload = 1
	// LayoffPctPerWeek is how much comes off per full week away, in percent.
	LayoffPctPerWeek = 10
	// MaxLayoffPct is the deepest cut a layoff can make, in percent, however
	// long it ran. Half your working weight is already a warm-up; no further
	// time off makes an empty bar the right place to restart a squat.
	MaxLayoffPct = 50
	// MaxLayoffWeeks is where the curve flattens — the last week off that
	// deepens the cut.
	MaxLayoffWeeks = MaxLayoffPct / LayoffPctPerWeek
)

// LayoffPct is how much to take off after weeks away from training, as a
// fraction (0.30 is 30%). Below the threshold it is 0, which callers read as
// "no layoff" — there is nothing to offer and nothing to prompt about.
//
// The week count is clamped before it is multiplied rather than after, so no
// value of weeks can overflow the product into a negative percentage and out
// the far side of the cap.
func LayoffPct(weeks int) float64 {
	if weeks < LayoffWeeksBeforeDeload {
		return 0
	}
	if weeks > MaxLayoffWeeks {
		weeks = MaxLayoffWeeks
	}
	return float64(weeks*LayoffPctPerWeek) / 100
}

// LayoffWeight applies the layoff cut to a weight, snapped to the bar. Returns
// previousLb unchanged when the time away is too short to count, so a caller
// need not check the threshold itself.
//
// This is the whole calculation for a lift the engine does not drive — see
// ApplyLayoff for one it does.
func LayoffWeight(previousLb float64, weeks int) float64 {
	pct := LayoffPct(weeks)
	if pct == 0 {
		return previousLb
	}
	return roundToBar(previousLb * (1 - pct))
}

// ApplyLayoff adjusts a Plan for weeks spent away from the gym, returning it
// unchanged when the layoff does not reach the lift.
//
// The cut is taken from PreviousLb — the weight last actually worked — and not
// from the plan's forward-looking target. The distinction matters after a
// success: coming back from three weeks off, 30% comes off the 225 that was
// lifted, not off the 230 the engine was about to ask for.
//
// Two cases leave the plan alone:
//
//   - StatusStart, where there is no history at all. PreviousLb is 0 there, so
//     without this guard a lift never performed would be prescribed an empty bar
//     — and a lift you have never done is not one you have detrained on.
//   - A layoff weight that is not below the plan's own. This is what keeps a
//     stall deload and a layoff deload from compounding into 90% × 80%: they are
//     two answers to "how light should this be", so the deeper one wins outright
//     and the other is a no-op. A week off after a stall changes nothing; a month
//     off after one takes over.
func ApplyLayoff(p Plan, weeks int) Plan {
	if LayoffPct(weeks) == 0 || p.Status == StatusStart {
		return p
	}
	weight := LayoffWeight(p.PreviousLb, weeks)
	if weight >= p.WeightLb {
		return p
	}
	p.WeightLb = weight
	p.Status = StatusLayoff
	p.LayoffPct = LayoffPct(weeks)
	return p
}
