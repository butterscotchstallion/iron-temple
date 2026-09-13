package racked

import (
	"sort"
	"time"
)

// One session, in review — the screen a lifter lands on the moment they finish.
//
// This is a second entry point into the same machinery Build uses, not a second
// implementation of it. A recap of a month is a reduction over a slice of
// sessions; a recap of a workout is that reduction over a slice of one. So the
// records here come from personalRecords, the milestones from milestones, the
// tonnage comparison from compare, and the duration from session.Duration —
// every one of them the function the monthly recap calls. A record announced at
// the rack on Tuesday and the same record listed in March's recap are one claim
// decided once, which is the only way the two can be trusted to agree.
//
// What is genuinely new here is everything that compares this session to the
// LAST TIME THIS PROGRAM DAY CAME ROUND: pace, and the weight progression. A
// month has no equivalent question, so there was nothing to reuse.

// minReportableDuration is the shortest gap the recap will call a duration.
//
// The recap reports whole seconds, so anything under one rounds to zero on the
// wire — and a zero is indistinguishable from the "never finished" that the
// same field uses null for. Worse is what it does to pace: a median of zero
// makes the percentage a division by noise, and a session created and finished
// inside the same second was ranked "6% faster" against it. Below a second is
// not a short workout; it is no measurement at all.
const minReportableDuration = time.Second

// SessionDuration is how long a session took, or 0 when that cannot be said:
// the lifter never tapped Finish, the gap ran past the cap, or it was too short
// to survive being expressed in seconds.
//
// Exported so a caller holding raw timestamps — the recap handler, ranking a
// program day's past lengths — can apply exactly the rule the recap applies
// internally, rather than restating a 12-hour constant somewhere it can drift
// from this one.
func SessionDuration(startedAt, finishedAt time.Time) time.Duration {
	d := session{StartedAt: startedAt, FinishedAt: finishedAt}.Duration()
	if d < minReportableDuration {
		return 0
	}
	return d
}

// SessionMeta identifies the session a recap is about.
type SessionMeta struct {
	SessionID      int32
	ProgramID      int32
	ProgramName    string
	ProgramDayID   int32
	ProgramDayName string
	PerformedOn    time.Time
	StartedAt      time.Time
	// FinishedAt is zero when the lifter never tapped Finish.
	FinishedAt time.Time
	IsOver     bool
}

// Prescribed is one row of the session's prescription, logged or not.
//
// It is deliberately not a Set: a Set is work that happened, and the whole point
// of this type is to carry the rows where nothing did. Reps is 0 for an unlogged
// row, which is how "10 of 25 reps" gets a denominator.
type Prescribed struct {
	ExerciseID   int32
	ExerciseName string
	TargetReps   int
	Reps         int
	Completed    bool
	IsAssistance bool
}

// SessionOutcome is one past session reduced to what a streak is judged on.
type SessionOutcome struct {
	PerformedOn       time.Time
	SetCount          int
	CompletedSetCount int
}

// SessionInput is everything BuildSession needs. Nothing here is fetched; the
// handler gathers it, exactly as buildRacked does for Build.
type SessionInput struct {
	Meta SessionMeta
	// Sets is this session's logged work, and Prescribed is every row it
	// carried. The two overlap: a logged set appears in both.
	Sets       []Set
	Prescribed []Prescribed
	// PreviousMeta and PreviousSets describe the last time this program day was
	// performed, or are nil when it never was.
	PreviousMeta *SessionMeta
	PreviousSets []Set
	// DayDurations are the lengths of earlier FINISHED sessions of this program
	// day. Already filtered: a zero-length duration never reaches here, because
	// the caller drops it through session.Duration, which is also what applies
	// the 12-hour cap.
	DayDurations []time.Duration
	Baseline     Baseline
	// Outcomes is this session and the ones before it, newest first.
	Outcomes []SessionOutcome
}

// SessionPace places this session's length against the same day's history.
type SessionPace struct {
	Median time.Duration
	// DeltaPct is (this − median) / median. Negative is faster, which is the
	// opposite of every other delta in this package; see the field's note in
	// openapi.yaml for why that has to be said out loud.
	DeltaPct   float64
	Rank       int
	Of         int
	SampleSize int
}

// SessionVolume is what the session moved and how much of it was prescribed.
type SessionVolume struct {
	TotalLb    float64
	PreviousLb *float64
	DeltaPct   *float64
	Comparison Comparison
	// SetsLogged counts sets with reps against them; SetsPrescribed counts every
	// row, so a workout cut short reads as the fraction it was.
	SetsLogged     int
	SetsPrescribed int
	RepsLogged     int
	RepsTargeted   int
}

// SessionProgress is how the weights moved since this day was last performed.
type SessionProgress struct {
	PreviousSessionID   *int32
	PreviousPerformedOn *time.Time
	WeightDeltaPct      *float64
	LiftsCompared       int
	LiftsNew            int
}

// SessionLiftPrevious is one lift, the last time this program day came round.
type SessionLiftPrevious struct {
	PerformedOn time.Time
	TopWeightLb float64
	TopReps     int
	TopE1RMLb   float64
}

// SessionLift is one lift's showing in this session.
type SessionLift struct {
	ExerciseID   int32
	ExerciseName string
	IsAssistance bool
	TopWeightLb  float64
	TopReps      int
	TopE1RMLb    float64

	SetsLogged     int
	SetsPrescribed int
	RepsLogged     int
	RepsTargeted   int
	VolumeLb       float64
	HitEveryTarget bool

	Previous       *SessionLiftPrevious
	WeightDeltaLb  *float64
	WeightDeltaPct *float64
	E1RMDeltaPct   *float64
}

// SessionStreak carries both runs the recap reports. See the schema note for
// why one number was not enough.
type SessionStreak struct {
	Sessions int
	Weeks    int
}

// SessionRecap is one session, in review.
type SessionRecap struct {
	Session    SessionMeta
	Duration   time.Duration
	Pace       *SessionPace
	Volume     SessionVolume
	Progress   SessionProgress
	Lifts      []SessionLift
	PRs        []PR
	Milestones []Milestone
	Streak     SessionStreak
}

// BuildSession reduces one session to its recap.
//
// Like Build, it fetches nothing and decides everything. Every slice it returns
// is non-nil so that a JSON encoder writes [] rather than null — a lifter's
// first workout has no records and no milestones, and a surface should render
// that as an empty list rather than branch on a missing one.
func BuildSession(in SessionInput) SessionRecap {
	sess := session{
		ID:          in.Meta.SessionID,
		PerformedOn: in.Meta.PerformedOn,
		StartedAt:   in.Meta.StartedAt,
		FinishedAt:  in.Meta.FinishedAt,
		DayName:     in.Meta.ProgramDayName,
		Sets:        in.Sets,
	}
	one := []session{sess}

	rec := SessionRecap{
		Session: in.Meta,
		// SessionDuration rather than sess.Duration(), so the one-second floor
		// applies here exactly as it does to the history pace is ranked within.
		Duration:   SessionDuration(in.Meta.StartedAt, in.Meta.FinishedAt),
		Volume:     sessionVolume(sess, in),
		Lifts:      sessionLifts(sess, in),
		PRs:        personalRecords(one, in.Baseline),
		Milestones: milestones(one, in.Baseline),
		Streak:     sessionStreak(in.Outcomes),
	}
	rec.Pace = sessionPace(rec.Duration, in.DayDurations)
	rec.Progress = sessionProgress(rec.Lifts, in)

	if rec.PRs == nil {
		rec.PRs = []PR{}
	}
	if rec.Milestones == nil {
		rec.Milestones = []Milestone{}
	}
	return rec
}

// sessionPace ranks this session's length among the same day's other lengths.
//
// Nil when this session has no duration of its own, and nil when no earlier
// session of the day was ever finished — with nothing to compare against there
// is no claim to make, and "as fast as you have ever been" on a first workout
// is a claim.
func sessionPace(d time.Duration, prior []time.Duration) *SessionPace {
	if d <= 0 || len(prior) == 0 {
		return nil
	}
	med := medianDuration(prior)
	if med <= 0 {
		return nil
	}

	// Rank counts strictly faster sessions, so a session that ties the record
	// shares its rank rather than being pushed below a session it matched.
	rank := 1
	for _, p := range prior {
		if p < d {
			rank++
		}
	}
	return &SessionPace{
		Median:     med,
		DeltaPct:   float64(d-med) / float64(med),
		Rank:       rank,
		Of:         len(prior) + 1,
		SampleSize: len(prior),
	}
}

// medianDuration is the middle of a set of durations, averaging the two middle
// values when there is an even number of them.
//
// Median rather than mean, because the distribution is skewed by construction:
// a session can run long for a reason that has nothing to do with training —
// the lifter forgot to tap Finish until the 12-hour cap caught it — but it
// cannot run short for one. A mean carries that tail into every comparison and
// flatters every subsequent workout.
func medianDuration(in []time.Duration) time.Duration {
	if len(in) == 0 {
		return 0
	}
	s := append([]time.Duration(nil), in...)
	sort.Slice(s, func(i, j int) bool { return s[i] < s[j] })
	mid := len(s) / 2
	if len(s)%2 == 1 {
		return s[mid]
	}
	return (s[mid-1] + s[mid]) / 2
}

// sessionVolume totals what moved and sizes it against the prescription.
func sessionVolume(sess session, in SessionInput) SessionVolume {
	v := SessionVolume{
		TotalLb:        sess.VolumeLb(),
		SetsPrescribed: len(in.Prescribed),
	}
	for _, p := range in.Prescribed {
		v.RepsTargeted += p.TargetReps
		if p.Reps > 0 {
			v.SetsLogged++
			v.RepsLogged += p.Reps
		}
	}
	v.Comparison = compare(v.TotalLb)

	if in.PreviousMeta != nil {
		var prev float64
		for _, s := range in.PreviousSets {
			prev += s.VolumeLb()
		}
		v.PreviousLb = &prev
		v.DeltaPct = ratio(v.TotalLb, prev)
	}
	return v
}

// sessionLifts reduces the session to one row per lift performed.
//
// Prescription order, taken from in.Prescribed — which arrives in the order the
// session screen draws, main lifts before assistance. sessionTops sorts
// alphabetically, which is right for a month's records and wrong for a recap a
// lifter reads against the workout they just did.
//
// A lift with no logged set is absent: it was not performed, and a row of
// zeroes claims otherwise.
func sessionLifts(sess session, in SessionInput) []SessionLift {
	tops := map[int32]sessionTop{}
	for _, t := range sessionTops(sess) {
		tops[t.ExerciseID] = t
	}
	prevTops := map[int32]sessionTop{}
	if in.PreviousMeta != nil {
		for _, t := range sessionTops(session{Sets: in.PreviousSets}) {
			prevTops[t.ExerciseID] = t
		}
	}

	out := make([]SessionLift, 0, len(tops))
	at := map[int32]int{}
	for _, p := range in.Prescribed {
		i, seen := at[p.ExerciseID]
		if !seen {
			top, performed := tops[p.ExerciseID]
			if !performed {
				continue
			}
			at[p.ExerciseID] = len(out)
			i = len(out)
			out = append(out, SessionLift{
				ExerciseID:     p.ExerciseID,
				ExerciseName:   p.ExerciseName,
				IsAssistance:   p.IsAssistance,
				TopWeightLb:    top.WeightLb,
				TopReps:        top.WeightReps,
				TopE1RMLb:      top.E1RMLb,
				HitEveryTarget: true,
			})
		}
		l := &out[i]
		l.SetsPrescribed++
		l.RepsTargeted += p.TargetReps
		if p.Reps > 0 {
			l.SetsLogged++
			l.RepsLogged += p.Reps
		}
		if !p.Completed {
			l.HitEveryTarget = false
		}
	}

	for i := range out {
		l := &out[i]
		for _, s := range in.Sets {
			if s.ExerciseID == l.ExerciseID {
				l.VolumeLb += s.VolumeLb()
			}
		}
		prev, ok := prevTops[l.ExerciseID]
		if !ok {
			continue
		}
		l.Previous = &SessionLiftPrevious{
			PerformedOn: in.PreviousMeta.PerformedOn,
			TopWeightLb: prev.WeightLb,
			TopReps:     prev.WeightReps,
			TopE1RMLb:   prev.E1RMLb,
		}
		delta := l.TopWeightLb - prev.WeightLb
		l.WeightDeltaLb = &delta
		l.WeightDeltaPct = ratio(l.TopWeightLb, prev.WeightLb)
		l.E1RMDeltaPct = ratio(l.TopE1RMLb, prev.E1RMLb)
	}
	return out
}

// sessionProgress is the headline "you are up N%" against the last time this
// program day was performed.
//
// It sums the top-set weights on both sides and takes the ratio, over only the
// lifts that appear in BOTH sessions. The two obvious alternatives are both
// wrong. Tonnage moves when a lifter adds a set or misses a rep, neither of
// which is the bar getting heavier. Averaging each lift's own percentage lets
// ten pounds added to a curl outweigh ten pounds added to a squat, because the
// curl's denominator is a quarter the size.
//
// Pairing is what makes the figure honest: a lift skipped today, or one added
// today, cannot move it at all. LiftsCompared is published alongside so a
// surface can say what the number was measured across — a percentage drawn from
// one lift of five is a different claim from one drawn from all five.
func sessionProgress(lifts []SessionLift, in SessionInput) SessionProgress {
	var p SessionProgress
	if in.PreviousMeta == nil {
		return p
	}
	id, on := in.PreviousMeta.SessionID, in.PreviousMeta.PerformedOn
	p.PreviousSessionID, p.PreviousPerformedOn = &id, &on

	var now, then float64
	for _, l := range lifts {
		if l.Previous == nil {
			p.LiftsNew++
			continue
		}
		p.LiftsCompared++
		now += l.TopWeightLb
		then += l.Previous.TopWeightLb
	}
	if p.LiftsCompared > 0 {
		p.WeightDeltaPct = ratio(now, then)
	}
	return p
}

// sessionStreak walks the lifter's recent sessions for the two runs the recap
// reports. Outcomes arrive newest first and include this session.
//
// The session run is broken by the first workout that was not completed to the
// letter, matching isSessionComplete in the UI. The week run is broken by a
// calendar week with nothing in it, matching streak() above — and both are
// deliberate: one measures precision, the other measures showing up, and a
// lifter who missed a rep last Thursday has not stopped training.
func sessionStreak(outcomes []SessionOutcome) SessionStreak {
	var s SessionStreak
	if len(outcomes) == 0 {
		return s
	}

	for _, o := range outcomes {
		if o.SetCount == 0 || o.CompletedSetCount != o.SetCount {
			break
		}
		s.Sessions++
	}

	// Distinct weeks, newest first, stepping back seven days at a time. Weeks
	// rather than dates for the reason weekStart exists: it makes "the week
	// before" a plain subtraction instead of a calendar question.
	seen := map[time.Time]bool{}
	weeks := make([]time.Time, 0, len(outcomes))
	for _, o := range outcomes {
		w := weekStart(o.PerformedOn)
		if !seen[w] {
			seen[w] = true
			weeks = append(weeks, w)
		}
	}
	sort.Slice(weeks, func(i, j int) bool { return weeks[i].After(weeks[j]) })
	s.Weeks = 1
	for i := 1; i < len(weeks); i++ {
		if !weeks[i].Equal(weeks[i-1].AddDate(0, 0, -7)) {
			break
		}
		s.Weeks++
	}
	return s
}

// ratio is (now − then) / then, or nil when there is nothing to divide by.
//
// A rise from zero is not a percentage. It is reported as an absent figure
// rather than as a very large one, which is the same stance Build takes on a
// period whose predecessor held no sessions.
func ratio(now, then float64) *float64 {
	if then <= 0 {
		return nil
	}
	r := (now - then) / then
	return &r
}
