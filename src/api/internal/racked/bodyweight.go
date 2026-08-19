package racked

import "time"

// The other half of a training log.
//
// Everything else in this package measures the weight a lifter moved; this
// measures the lifter. The two belong in one recap because they are read
// together — a month where volume fell and bodyweight fell with it is a
// different month from one where volume fell on its own.
//
// Nothing here interpolates, smooths or fills. A weigh-in is a reading taken on
// a day, and the days between two readings hold no information; 0010 left
// sessions.bodyweight_lb NULL rather than carrying the last value forward for
// exactly that reason, and inventing the missing days here would give that back.

// WeighIn is one recorded bodyweight, flattened from store.RackedWeighInsRow.
type WeighIn struct {
	PerformedOn time.Time
	WeightLb    float64
}

// Bodyweight is the period's weigh-ins and what they came to.
//
// A period with no weigh-in has no Bodyweight at all — Build leaves the pointer
// nil and the surfaces drop the section — rather than a zeroed struct that would
// render as a lifter weighing nothing.
type Bodyweight struct {
	// Points are the weigh-ins themselves, oldest first, one per session that
	// recorded one. More than one on a day is possible and kept: two sessions
	// are two readings.
	Points []WeighIn
	// StartLb and EndLb are the first and last readings *in the period*, not the
	// lifter's weight on its first and last dates. A month weighed in on the 3rd
	// and the 24th is measured between those two.
	StartLb float64
	EndLb   float64
	LowLb   float64
	HighLb  float64
	// ChangeLb and ChangePct are EndLb against StartLb, and are nil when the
	// period holds a single weigh-in — one reading is a fact, not a trend, and
	// reporting it as "no change" would claim a stability nobody measured. Same
	// convention as Change.VolumePct, which is nil rather than zero when there is
	// no ratio to quote. ChangePct is a fraction: -0.014 is -1.4%.
	ChangeLb  *float64
	ChangePct *float64
}

// bodyweight reduces a period's weigh-ins to the summary above.
//
// The change is measured strictly inside the period. Anchoring the start to the
// last weigh-in before it would give a single-weigh-in month a delta, which is
// tempting and wrong twice over: the figure would cover a stretch of calendar
// the recap is not reporting on, and a lifter returning to the scale after six
// months off would be shown half a year of drift as though it were March's.
func bodyweight(weighIns []WeighIn) *Bodyweight {
	if len(weighIns) == 0 {
		return nil
	}

	b := &Bodyweight{
		Points:  weighIns,
		StartLb: weighIns[0].WeightLb,
		EndLb:   weighIns[len(weighIns)-1].WeightLb,
		LowLb:   weighIns[0].WeightLb,
		HighLb:  weighIns[0].WeightLb,
	}
	for _, w := range weighIns {
		if w.WeightLb < b.LowLb {
			b.LowLb = w.WeightLb
		}
		if w.WeightLb > b.HighLb {
			b.HighLb = w.WeightLb
		}
	}

	if len(weighIns) < 2 {
		return b
	}
	change := b.EndLb - b.StartLb
	b.ChangeLb = &change
	// StartLb is a bodyweight and so cannot be zero — the column's CHECK rejects
	// it — but the guard costs a line and keeps the division from being a claim
	// about data this function does not own.
	if b.StartLb > 0 {
		pct := change / b.StartLb
		b.ChangePct = &pct
	}
	return b
}
