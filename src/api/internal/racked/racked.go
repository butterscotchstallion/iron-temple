// Package racked turns a period of logged sets into a recap: a headline volume,
// a handful of hero moments, and the series the charts and the monthly email
// both draw from.
//
// Everything here is a pure function of its input. The package holds no
// database handle and imports no pgtype: the caller flattens store rows into
// []Set and hands them over. That is what makes a deload, a comeback or an
// archetype cheap to test — they are judgements about a series, and judgements
// need many more cases than an integration test will ever carry.
//
// One Report serves both surfaces. The HTTP endpoint marshals it and the email
// template renders it, so the page and the recap cannot drift into disagreeing
// about how much a lifter moved in March.
package racked

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

// PeriodKind is the granularity of a recap.
type PeriodKind string

const (
	// PeriodMonth recaps one calendar month.
	PeriodMonth PeriodKind = "month"
	// PeriodYear recaps one calendar year.
	PeriodYear PeriodKind = "year"
)

// maxSessionDuration bounds what counts as a session's elapsed time.
//
// It is the same 12 hours sessions.sql uses to decide a session can no longer
// be in progress, and for the same reason: a lifter who never tapped Finish and
// closed the tab leaves a row whose created_at is hours from anything real.
// Such a session has no duration worth reporting, and without this guard it
// would win "longest" and poison the average that picks an archetype.
const maxSessionDuration = 12 * time.Hour

// Set is one logged set, flattened from store.RackedPeriodSetsRow. Only sets
// with real logged reps ever reach here — the query filters actual_reps > 0.
type Set struct {
	SessionID      int32
	PerformedOn    time.Time
	StartedAt      time.Time
	FinishedAt     time.Time // zero when the lifter never tapped Finish
	ProgramDayName string
	ExerciseID     int32
	ExerciseName   string
	Reps           int
	WeightLb       float64
	Completed      bool
	// MuscleGroup is the movement's primary mover, from the exercise library's
	// taxonomy (see 0009). One group per lift, not a split across several — see
	// muscles.go for why that is enough for the question it answers.
	MuscleGroup string
	// IsAssistance is true for work the lifter bolted onto the program day
	// rather than work the program prescribed — the same distinction the session
	// screen draws, derived in SQL from the absence of a prescription.
	//
	// It never removes a set from a total. Assistance is training, and the
	// headline tonnage counted it before this field existed and still does; what
	// the flag buys is a report that can say how the month divided.
	IsAssistance bool
}

// VolumeLb is the weight this set moved: reps actually performed times the
// weight on the bar, matching SessionTotals rather than the prescription.
func (s Set) VolumeLb() float64 { return float64(s.Reps) * s.WeightLb }

// E1RM estimates a one-rep max by the Epley formula, rounded to the pound —
// the same calculation as estimateOneRepMax in the UI.
//
// A single is worth exactly what was on the bar. Epley is an extrapolation from
// a set carried past one rep, and applying it at one rep estimates a number that
// needs no estimating: it returned weight * 31/30, so a lifter who pulled a
// genuine 225 single was credited with 233. Worse, it ordered the two wrongly —
// 225x1 estimated above 225x2, when the second is plainly the harder set.
func (s Set) E1RM() float64 {
	if s.WeightLb <= 0 || s.Reps <= 0 {
		return 0
	}
	if s.Reps == 1 {
		return math.Round(s.WeightLb)
	}
	return math.Round(s.WeightLb * (1 + float64(s.Reps)/30))
}

// Baseline is the lifter's history before the period opened. It is what lets a
// maximum inside the period be recognised as a record rather than just a max.
type Baseline struct {
	VolumeLb   float64
	BestWeight map[int32]float64
	BestE1RM   map[int32]float64
}

// ProgramDay is one scheduled day of the lifter's current program. Weekday is
// nil when the day carries no scheduled weekday, which is the common case: the
// column is nullable and no seed populates it.
type ProgramDay struct {
	Name    string
	Weekday *int
}

// Input is everything Build needs. Start and End are inclusive date bounds.
type Input struct {
	Kind  PeriodKind
	Start time.Time
	End   time.Time
	// AsOf is the date the recap is drawn up on. It matters only when it falls
	// inside the period: the page opens on the month in progress, and every rate
	// in the report divides by a window, so a window that runs to the end of a
	// month barely started measures a lifter against days they have not reached.
	// The zero value means "the period as a whole", which is what a completed
	// month is and what the recap email always sends.
	AsOf time.Time
	Loc  *time.Location
	Sets []Set
	// WeighIns is every bodyweight recorded in the period, oldest first. Most
	// lifters record none, and an empty slice is that answer rather than a gap
	// to fill in — see the Bodyweight type.
	WeighIns []WeighIn
	// PreviousSets is the preceding period, for the headline comparison.
	PreviousSets []Set
	// PreviousStart anchors that period so it can be cut to the same number of
	// elapsed days as this one. Zero disables the cut and compares whole periods.
	PreviousStart time.Time
	Baseline      Baseline
	ProgramDays   []ProgramDay
	// ProgramStarted is when the program in ProgramDays came into existence.
	// Attendance grades only the part of the period it existed for, and not at
	// all when it postdates the period entirely. Zero means "always existed".
	ProgramStarted time.Time
}

// Report is the whole recap.
type Report struct {
	Period     Period
	Totals     Totals
	Change     *Change
	Comparison Comparison
	// Split is Totals.VolumeLb divided between prescribed work and assistance.
	Split Split
	// Muscles is Totals.VolumeLb divided between the muscle groups it trained,
	// every group in the taxonomy included — a group with nothing against it is
	// the section's most useful row. See muscles.go.
	Muscles      []MuscleSlice
	Lifts        []LiftSlice
	Series       []LiftSeries
	MostImproved *Improvement
	// Bodyweight is the period's weigh-ins, or nil when it holds none — which is
	// the common case, and a section the surfaces then omit entirely.
	Bodyweight  *Bodyweight
	Days        []DayVolume
	Weekdays    []float64
	BestWeekday int
	Hours       []int
	HourLabel   string
	// PeakHour is the hour of day the lifter started most sessions in, or -1
	// when none of them carry a start time. Published rather than left for each
	// surface to work out: two readers of the same Hours array picked different
	// hours out of a tie, so the page highlighted one bar while the label named
	// another.
	PeakHour       int
	Streak         Streak
	Attendance     Attendance
	PRs            []PR
	Milestones     []Milestone
	HeaviestSet    *SetHighlight
	FastestSession *SessionHighlight
	Deloads        []Deload
	Archetype      Archetype
}

// Period names the window a recap covers.
type Period struct {
	Kind  PeriodKind
	Start time.Time
	End   time.Time
	Label string
	// InProgress is true when the period has not finished yet, which is the
	// default view. Every rate in the report is then measured over the days so
	// far, and the surfaces say so rather than presenting a part-month as a
	// whole one.
	InProgress bool
}

// Totals are the headline counters.
type Totals struct {
	VolumeLb float64
	Sessions int
	Sets     int
	Reps     int
}

// Change compares the headline counters against the preceding period. Percents
// are fractions (0.12 is +12%) and are omitted when the prior figure is zero,
// because "up from nothing" is a ratio with no meaning.
type Change struct {
	VolumeLb    float64
	VolumePct   *float64
	Sessions    int
	SessionsPct *float64
}

// LiftSlice is one lift's share of the period's volume.
type LiftSlice struct {
	ExerciseID   int32
	ExerciseName string
	VolumeLb     float64
	Sets         int
	Reps         int
	Share        float64
	// IsAssistance is true when every set of this lift in the period was
	// assistance work.
	//
	// Every, not most. A lift can sit on two program days — prescribed on one,
	// bolted onto the other — and one prescribed set is enough to make it the
	// program's work. Labelling by majority instead would move a lift between
	// classes as the ratio drifted week to week, and a squat does not become
	// accessory work because of how a month went.
	//
	// Nothing that has to add up depends on this: Split counts set by set, so a
	// mixed lift lands on both sides in the proportion it was actually trained.
	// This field only decides what a lift is called in a list.
	IsAssistance bool
}

// Work is one class of training: what the program asked for, or what the lifter
// added to it. The counters are the same ones Totals carries, so a surface can
// put a class beside the headline and have the two agree.
type Work struct {
	VolumeLb float64
	Sets     int
	Reps     int
	// Lifts is how many distinct exercises the class covered — the figure that
	// makes "18% of your volume" mean something, since eighteen percent across
	// one movement and across eight are different months.
	Lifts int
	// Share is of the period's whole volume, so Main.Share + Assistance.Share is
	// 1 for any period that moved weight and 0 for one that did not.
	Share float64
}

// Split divides the period between the program's own work and the lifter's
// assistance.
//
// It divides, it does not subtract: Main.VolumeLb + Assistance.VolumeLb is
// Totals.VolumeLb exactly. The recap counted assistance in its headline from the
// day assistance shipped — it simply could not name it — and moving that number
// now would rewrite every month a lifter has already read.
type Split struct {
	Main       Work
	Assistance Work
}

// SeriesPoint is one session's showing for one lift.
type SeriesPoint struct {
	PerformedOn time.Time
	TopWeightLb float64
	E1RMLb      float64
}

// LiftSeries is a lift's progression across the period, oldest first.
type LiftSeries struct {
	ExerciseID   int32
	ExerciseName string
	Points       []SeriesPoint
	// IsAssistance is true when every set of this lift in the period was
	// assistance work. deloads reads it; nothing else does.
	IsAssistance bool
}

// Improvement is the lift that moved the most, measured on estimated one-rep
// max so that an extra rep counts as progress and not just an extra plate.
type Improvement struct {
	ExerciseID   int32
	ExerciseName string
	FromLb       float64
	ToLb         float64
	GainLb       float64
	GainPct      float64
}

// DayVolume is one calendar day's tonnage, for the heatmap.
type DayVolume struct {
	Date     time.Time
	VolumeLb float64
	Sessions int
}

// Streak counts consecutive calendar weeks containing at least one session.
// Weeks rather than days because no sensible program trains daily, so a
// day-streak would punish the rest day the program prescribes.
type Streak struct {
	LongestWeeks int
	CurrentWeeks int
}

// AttendanceBasis records whether there is a real target to measure against.
type AttendanceBasis string

const (
	// AttendanceWeekday means the program's days carry scheduled weekdays, so
	// expected counts real calendar occurrences of them and Rate means something.
	AttendanceWeekday AttendanceBasis = "weekday"
	// AttendanceNone means there is no schedule to compare with — either no
	// current program, or a program whose days carry no weekday. Expected and
	// Rate are zero and must not be shown; SessionsPerWeek is what to report.
	AttendanceNone AttendanceBasis = "none"
)

// Attendance compares sessions performed against sessions the program implies.
//
// There used to be a third basis that spread the program's day count over the
// period's weeks when no weekday was set. It was dropped: program_days.weekday is
// nullable and no seed populates it, so that branch was what almost every lifter
// actually saw — a percentage against a denominator nobody had entered, labelled
// an estimate and still read as a grade. A number that precise about a target
// that invented is worse than no number at all.
//
// SessionsPerWeek needs no configuration and is always populated, so there is
// something real to report either way.
type Attendance struct {
	Basis    AttendanceBasis
	Expected int
	Actual   int
	Rate     float64
	// SessionsPerWeek is how often the lifter actually trained, measured over
	// the period's length rather than over the weeks they happened to show up.
	SessionsPerWeek float64
}

// PRKind distinguishes the two ways a set can be a record.
type PRKind string

const (
	// PRWeight is a heavier top set than the lift has ever seen.
	PRWeight PRKind = "weight"
	// PREstimated is the same weight carried for more reps — no new plate, but
	// a higher estimated max, which on a 5x5 is exactly what progress looks
	// like on the session before the jump.
	PREstimated PRKind = "e1rm"
)

// PR is a personal record set during the period.
type PR struct {
	Kind         PRKind
	PerformedOn  time.Time
	ExerciseID   int32
	ExerciseName string
	WeightLb     float64
	Reps         int
	ValueLb      float64
	PreviousLb   float64
}

// SetHighlight is a single set worth calling out.
type SetHighlight struct {
	PerformedOn  time.Time
	ExerciseID   int32
	ExerciseName string
	WeightLb     float64
	Reps         int
}

// SessionHighlight is a single session worth calling out.
type SessionHighlight struct {
	SessionID      int32
	PerformedOn    time.Time
	ProgramDayName string
	Duration       time.Duration
	VolumeLb       float64
	Sets           int
}

// Deload is a drop in a lift's working weight, and whether it was won back.
type Deload struct {
	ExerciseID   int32
	ExerciseName string
	PerformedOn  time.Time
	FromLb       float64
	ToLb         float64
	Recovered    bool
	RecoveredOn  time.Time
}

// Build computes the recap. It never returns an error: an empty period is a
// valid recap with zero totals, and the surfaces above decide how to say so.
func Build(in Input) Report {
	if in.Loc == nil {
		in.Loc = time.UTC
	}
	sessions := groupSessions(in.Sets)

	// Everything measured as a rate uses this rather than in.End: the days of
	// the period that have actually happened. For a finished period the two are
	// the same, which is why a zero AsOf changes nothing.
	measuredTo := measuredEnd(in.Start, in.End, in.AsOf)
	inProgress := measuredTo.Before(in.End)

	rep := Report{
		Period: Period{
			Kind:       in.Kind,
			Start:      in.Start,
			End:        in.End,
			Label:      periodLabel(in.Kind, in.Start),
			InProgress: inProgress,
		},
		Totals:      totals(in.Sets, sessions),
		Split:       split(in.Sets),
		Muscles:     muscleSlices(in.Sets),
		Lifts:       liftSlices(in.Sets),
		Series:      liftSeries(sessions),
		Bodyweight:  bodyweight(in.WeighIns),
		Days:        dayVolumes(sessions),
		Weekdays:    make([]float64, 7),
		BestWeekday: -1,
		Hours:       make([]int, 24),
		Streak:      streak(sessions),
		PRs:         personalRecords(sessions, in.Baseline),
		Milestones:  milestones(sessions, in.Baseline),
		Deloads:     deloads(sessions),
	}

	rep.Comparison = compare(rep.Totals.VolumeLb)
	rep.MostImproved = mostImproved(rep.Series)
	rep.HeaviestSet = heaviestSet(in.Sets)
	rep.FastestSession = fastestSession(sessions)
	rep.Attendance = attendance(
		in.ProgramDays, in.ProgramStarted, sessionDates(sessions), in.Start, measuredTo,
	)
	rep.Archetype = archetype(sessions, in.Start, measuredTo)

	fillWeekdays(&rep, sessions)
	fillHours(&rep, sessions, in.Loc)

	// While the period runs, the comparison is cut to the same number of days as
	// has elapsed here, so three days of March are weighed against the first
	// three days of February rather than the whole of it. Comparing a part-month
	// against a whole one reported a collapse in volume every month, correcting
	// itself only on the last day.
	//
	// Only while it runs. Two finished periods are compared whole against whole,
	// however unequal their lengths — cutting unconditionally trimmed the
	// preceding period to this one's day count, so a completed February dropped
	// the 29th to the 31st of January, April dropped the 31st of March, and a
	// common year dropped the last day of a leap year. That path is the one the
	// recap email takes.
	previous := in.PreviousSets
	if inProgress {
		previous = elapsedSets(previous, in.PreviousStart, daysBetween(in.Start, measuredTo))
	}
	if prev := totals(previous, groupSessions(previous)); prev.Sessions > 0 {
		rep.Change = change(rep.Totals, prev)
	}
	return rep
}

// sessionDates is the day each session was performed on, which is all
// attendance needs of them.
func sessionDates(sessions []session) []time.Time {
	out := make([]time.Time, 0, len(sessions))
	for _, s := range sessions {
		out = append(out, s.PerformedOn)
	}
	return out
}

// measuredEnd is the last day of the period that has actually happened, held
// inside the period whatever AsOf says.
//
// The clamp is not defensive padding. AsOf and the period bounds are two
// readings of "today", and a caller taking them from different clocks — a
// report zone behind UTC, in the small hours of the 1st — can put AsOf before
// the period opens. Every window downstream is derived from this one, so an
// out-of-range answer here produces a report that disagrees with itself:
// attendance with no days to count, a comparison cut to nothing, and totals
// still counting the whole period.
func measuredEnd(start, end, asOf time.Time) time.Time {
	if asOf.IsZero() || !asOf.Before(end) {
		return end
	}
	if asOf.Before(start) {
		return start
	}
	return asOf
}

// daysBetween counts the inclusive span between two dates, never less than one.
func daysBetween(start, end time.Time) int {
	days := int(end.Sub(start).Hours()/24) + 1
	if days < 1 {
		return 1
	}
	return days
}

// elapsedSets keeps only the first `days` days of a period, so the preceding
// period can be compared against the same stretch of calendar as this one.
// A zero start means the caller has nothing to anchor against and wants the
// period whole.
func elapsedSets(sets []Set, start time.Time, days int) []Set {
	if start.IsZero() || len(sets) == 0 {
		return sets
	}
	cutoff := start.AddDate(0, 0, days-1)
	out := make([]Set, 0, len(sets))
	for _, s := range sets {
		if !s.PerformedOn.After(cutoff) {
			out = append(out, s)
		}
	}
	return out
}

// session is a period's sets regrouped by the session that produced them.
type session struct {
	ID          int32
	PerformedOn time.Time
	StartedAt   time.Time
	FinishedAt  time.Time
	DayName     string
	Sets        []Set
}

// VolumeLb totals the weight the session moved.
func (s session) VolumeLb() float64 {
	var v float64
	for _, set := range s.Sets {
		v += set.VolumeLb()
	}
	return v
}

// Duration is the session's elapsed time, or 0 when it was never finished or
// ran past maxSessionDuration and so cannot be trusted.
func (s session) Duration() time.Duration {
	if s.FinishedAt.IsZero() || s.StartedAt.IsZero() || !s.FinishedAt.After(s.StartedAt) {
		return 0
	}
	d := s.FinishedAt.Sub(s.StartedAt)
	if d > maxSessionDuration {
		return 0
	}
	return d
}

// groupSessions collapses the flat set list into sessions, preserving the
// query's chronological order. Sets arrive grouped by session already; this
// tolerates them not being, so a caller reordering rows cannot silently split
// one session into two.
func groupSessions(sets []Set) []session {
	if len(sets) == 0 {
		return nil
	}
	order := make([]int32, 0, 8)
	byID := make(map[int32]*session, 8)
	for _, s := range sets {
		cur, ok := byID[s.SessionID]
		if !ok {
			cur = &session{
				ID:          s.SessionID,
				PerformedOn: s.PerformedOn,
				StartedAt:   s.StartedAt,
				FinishedAt:  s.FinishedAt,
				DayName:     s.ProgramDayName,
			}
			byID[s.SessionID] = cur
			order = append(order, s.SessionID)
		}
		cur.Sets = append(cur.Sets, s)
	}
	out := make([]session, 0, len(order))
	for _, id := range order {
		out = append(out, *byID[id])
	}
	sort.SliceStable(out, func(i, j int) bool {
		if !out[i].PerformedOn.Equal(out[j].PerformedOn) {
			return out[i].PerformedOn.Before(out[j].PerformedOn)
		}
		return out[i].ID < out[j].ID
	})
	return out
}

func totals(sets []Set, sessions []session) Totals {
	t := Totals{Sessions: len(sessions), Sets: len(sets)}
	for _, s := range sets {
		t.VolumeLb += s.VolumeLb()
		t.Reps += s.Reps
	}
	return t
}

func change(cur, prev Totals) *Change {
	c := &Change{
		VolumeLb: cur.VolumeLb - prev.VolumeLb,
		Sessions: cur.Sessions - prev.Sessions,
	}
	if prev.VolumeLb > 0 {
		pct := (cur.VolumeLb - prev.VolumeLb) / prev.VolumeLb
		c.VolumePct = &pct
	}
	if prev.Sessions > 0 {
		pct := float64(cur.Sessions-prev.Sessions) / float64(prev.Sessions)
		c.SessionsPct = &pct
	}
	return c
}

// split divides the period's volume between prescribed work and assistance.
//
// Sets are counted where they were performed, which is the only way the two
// halves can add back up to the headline. A lift is not assigned wholesale to
// one class: squats prescribed on Workout A and added as assistance to Workout B
// contribute to both, and each set knows which it was.
func split(sets []Set) Split {
	var sp Split
	mainLifts, assistLifts := map[int32]bool{}, map[int32]bool{}
	var total float64
	for _, s := range sets {
		side, seen := &sp.Main, mainLifts
		if s.IsAssistance {
			side, seen = &sp.Assistance, assistLifts
		}
		side.VolumeLb += s.VolumeLb()
		side.Sets++
		side.Reps += s.Reps
		seen[s.ExerciseID] = true
		total += s.VolumeLb()
	}
	sp.Main.Lifts, sp.Assistance.Lifts = len(mainLifts), len(assistLifts)
	if total > 0 {
		sp.Main.Share = sp.Main.VolumeLb / total
		sp.Assistance.Share = sp.Assistance.VolumeLb / total
	}
	return sp
}

func liftSlices(sets []Set) []LiftSlice {
	if len(sets) == 0 {
		return nil
	}
	byID := map[int32]*LiftSlice{}
	var total float64
	for _, s := range sets {
		cur, ok := byID[s.ExerciseID]
		if !ok {
			cur = &LiftSlice{
				ExerciseID:   s.ExerciseID,
				ExerciseName: s.ExerciseName,
				// Starts true and is cleared by the first prescribed set, which
				// is liftIsAssistance's rule expressed as one pass.
				IsAssistance: true,
			}
			byID[s.ExerciseID] = cur
		}
		cur.VolumeLb += s.VolumeLb()
		cur.Sets++
		cur.Reps += s.Reps
		cur.IsAssistance = cur.IsAssistance && s.IsAssistance
		total += s.VolumeLb()
	}
	out := make([]LiftSlice, 0, len(byID))
	for _, v := range byID {
		if total > 0 {
			v.Share = v.VolumeLb / total
		}
		out = append(out, *v)
	}
	sortLifts(out)
	return out
}

// sortLifts orders by volume descending, then by name so that two lifts with
// equal volume do not swap places between two renders of the same report.
func sortLifts(out []LiftSlice) {
	sort.Slice(out, func(i, j int) bool {
		if out[i].VolumeLb != out[j].VolumeLb {
			return out[i].VolumeLb > out[j].VolumeLb
		}
		return out[i].ExerciseName < out[j].ExerciseName
	})
}

// liftSeries reduces each session to one point per lift: the top weight worked
// and the best estimated max. A 5x5 produces five sets of the same weight, and
// a chart of every set is a chart of a flat line with steps in it.
func liftSeries(sessions []session) []LiftSeries {
	if len(sessions) == 0 {
		return nil
	}
	order := make([]int32, 0, 8)
	byID := map[int32]*LiftSeries{}
	for _, sess := range sessions {
		best := map[int32]*SeriesPoint{}
		names := map[int32]string{}
		// Assistance until a prescribed set says otherwise — the same "every set"
		// rule LiftSlice.IsAssistance documents.
		assistance := map[int32]bool{}
		for _, set := range sess.Sets {
			p, ok := best[set.ExerciseID]
			if !ok {
				p = &SeriesPoint{PerformedOn: sess.PerformedOn}
				best[set.ExerciseID] = p
				names[set.ExerciseID] = set.ExerciseName
				assistance[set.ExerciseID] = true
			}
			p.TopWeightLb = math.Max(p.TopWeightLb, set.WeightLb)
			p.E1RMLb = math.Max(p.E1RMLb, set.E1RM())
			assistance[set.ExerciseID] = assistance[set.ExerciseID] && set.IsAssistance
		}
		for id, p := range best {
			series, ok := byID[id]
			if !ok {
				series = &LiftSeries{
					ExerciseID:   id,
					ExerciseName: names[id],
					IsAssistance: true,
				}
				byID[id] = series
				order = append(order, id)
			}
			series.IsAssistance = series.IsAssistance && assistance[id]
			series.Points = append(series.Points, *p)
		}
	}
	out := make([]LiftSeries, 0, len(order))
	for _, id := range order {
		out = append(out, *byID[id])
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ExerciseName < out[j].ExerciseName })
	return out
}

// mostImproved picks the largest percentage gain in estimated max, comparing how
// a lift ended the period against how it started it.
//
// Percentage rather than pounds, so a press competes fairly with a deadlift
// carrying three times the load.
//
// "Started" and "ended" are the best session in the period's first third and its
// last third, not the first and last sessions. Single sessions at the edges are
// the wrong thing to build a month's verdict on, and the error is not random: a
// linear program deloads *systematically*, so a lift that stalled in the final
// week reads as a decline no matter how much ground it gained earlier. Taking
// the best of a window absorbs that. It still reports a decline when the whole
// window regressed, which is the honest outcome and the reason this is a window
// and not a maximum over the period — a max can only ever flatter.
//
// Two sessions degenerate to first-versus-last, which is all the data there is.
func mostImproved(series []LiftSeries) *Improvement {
	var best *Improvement
	for _, s := range series {
		if len(s.Points) < 2 {
			continue
		}
		from, to := edgeWindowBests(s.Points)
		if from <= 0 || to <= from {
			continue
		}
		cand := Improvement{
			ExerciseID:   s.ExerciseID,
			ExerciseName: s.ExerciseName,
			FromLb:       from,
			ToLb:         to,
			GainLb:       to - from,
			GainPct:      (to - from) / from,
		}
		if best == nil || cand.GainPct > best.GainPct ||
			(cand.GainPct == best.GainPct && cand.GainLb > best.GainLb) {
			c := cand
			best = &c
		}
	}
	return best
}

// edgeWindowBests returns the best estimated max in the first third of a series
// and in the last third — the two figures mostImproved compares.
//
// The windows are sized by session count rather than by date, so a month with
// sessions bunched at one end still splits into thirds of equal weight. They
// never overlap and are never empty: with two or three points they are one
// session each, which is the most the data supports.
func edgeWindowBests(points []SeriesPoint) (float64, float64) {
	if len(points) == 0 {
		return 0, 0
	}
	window := len(points) / 3
	if window < 1 {
		window = 1
	}

	best := func(ps []SeriesPoint) float64 {
		var m float64
		for _, p := range ps {
			m = math.Max(m, p.E1RMLb)
		}
		return m
	}
	return best(points[:window]), best(points[len(points)-window:])
}

func dayVolumes(sessions []session) []DayVolume {
	if len(sessions) == 0 {
		return nil
	}
	order := make([]time.Time, 0, len(sessions))
	byDay := map[time.Time]*DayVolume{}
	for _, s := range sessions {
		d, ok := byDay[s.PerformedOn]
		if !ok {
			d = &DayVolume{Date: s.PerformedOn}
			byDay[s.PerformedOn] = d
			order = append(order, s.PerformedOn)
		}
		d.VolumeLb += s.VolumeLb()
		d.Sessions++
	}
	out := make([]DayVolume, 0, len(order))
	for _, k := range order {
		out = append(out, *byDay[k])
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Date.Before(out[j].Date) })
	return out
}

// fillWeekdays ranks weekdays by tonnage — "productive" meaning weight moved,
// not sessions attended, so a long Saturday outranks two short Tuesdays.
func fillWeekdays(rep *Report, sessions []session) {
	var best float64
	for _, s := range sessions {
		wd := int(s.PerformedOn.Weekday())
		rep.Weekdays[wd] += s.VolumeLb()
	}
	for i, v := range rep.Weekdays {
		if v > best {
			best, rep.BestWeekday = v, i
		}
	}
}

// fillHours buckets session start times in the lifter's zone. created_at is the
// moment the session row was created, which is the moment they started it in
// the app — a proxy, but the only one the schema carries.
func fillHours(rep *Report, sessions []session, loc *time.Location) {
	for _, s := range sessions {
		if s.StartedAt.IsZero() {
			continue
		}
		rep.Hours[s.StartedAt.In(loc).Hour()]++
	}

	// Counted first, then read — so a tie goes to the earlier hour rather than
	// to whichever hour happened to reach the count first as the sessions were
	// walked. That order was invisible in the data and disagreed with how the
	// page picked the bar to accent, putting "Evening lifter" over a highlighted
	// six in the morning.
	rep.PeakHour = -1
	best := 0
	for h, count := range rep.Hours {
		if count > best {
			best, rep.PeakHour = count, h
		}
	}
	rep.HourLabel = hourLabel(rep.PeakHour)
}

func hourLabel(hour int) string {
	switch {
	case hour < 0:
		return ""
	case hour < 5:
		return "Night owl"
	case hour < 9:
		return "Early bird"
	case hour < 12:
		return "Morning lifter"
	case hour < 17:
		return "Afternoon lifter"
	case hour < 21:
		return "Evening lifter"
	default:
		return "Night owl"
	}
}

// weekStart normalises a date to the Monday of its week, which makes "the next
// week along" a plain seven-day step and sidesteps ISO year boundaries, where
// week 1 can begin in December.
func weekStart(d time.Time) time.Time {
	d = time.Date(d.Year(), d.Month(), d.Day(), 0, 0, 0, 0, time.UTC)
	offset := (int(d.Weekday()) + 6) % 7
	return d.AddDate(0, 0, -offset)
}

// streak measures runs of consecutive weeks containing at least one session.
// CurrentWeeks is the run ending at the last week the lifter trained — within
// this period, so a recap of a past month reports how that month ended.
func streak(sessions []session) Streak {
	if len(sessions) == 0 {
		return Streak{}
	}
	seen := map[time.Time]bool{}
	weeks := make([]time.Time, 0, len(sessions))
	for _, s := range sessions {
		w := weekStart(s.PerformedOn)
		if !seen[w] {
			seen[w] = true
			weeks = append(weeks, w)
		}
	}
	sort.Slice(weeks, func(i, j int) bool { return weeks[i].Before(weeks[j]) })

	longest, run := 1, 1
	for i := 1; i < len(weeks); i++ {
		if weeks[i].Equal(weeks[i-1].AddDate(0, 0, 7)) {
			run++
		} else {
			run = 1
		}
		if run > longest {
			longest = run
		}
	}
	return Streak{LongestWeeks: longest, CurrentWeeks: run}
}

// attendance measures sessions performed against what the program prescribes,
// and how often the lifter trained regardless.
//
// A rate is only produced when the program says which days it wants, because
// that is the only case where a denominator exists rather than being guessed at.
// Everything else reports SessionsPerWeek, which is a measurement rather than a
// score, and is the same figure the archetype already reasons about.
// end is the last day measured, which for a period still running is today
// rather than the period's final date.
//
// programStarted guards against grading a period the program did not exist for.
// It is a partial guard by necessity: nothing records which program was current
// in a given month, only when each program was created, so a lifter who moved
// between two long-standing programs is still measured against the one they
// happen to be running now. Catching the newer-program case is what is available
// without a history table, and it is the case that actually arises — a lifter
// who switches usually switches to something new.
func attendance(
	days []ProgramDay, programStarted time.Time, performed []time.Time, start, end time.Time,
) Attendance {
	a := Attendance{
		Basis:  AttendanceNone,
		Actual: len(performed),
		// Measured over the whole elapsed period, whatever the program was doing
		// in it — this is a fact about the lifter, not about a schedule.
		SessionsPerWeek: float64(len(performed)) / weeksBetween(start, end),
	}

	// Counted, not collected: a program running two different days on a Monday
	// asks for two Monday sessions. Deduping the weekdays into a set quietly
	// lowered the target for exactly the lifters doing the most work.
	scheduled := map[int]int{}
	for _, d := range days {
		if d.Weekday != nil {
			scheduled[*d.Weekday]++
		}
	}
	if len(scheduled) == 0 {
		return a
	}

	// A program speaks only for the days it existed for. Starting one on the
	// 10th does not make the first nine days of the month a missed target, and
	// declining to grade the month at all — which this used to do — throws away
	// the three weeks it does have something to say about.
	from := start
	if !programStarted.IsZero() && programStarted.After(from) {
		from = programStarted
	}
	if from.After(end) {
		return a
	}

	for d := from; !d.After(end); d = d.AddDate(0, 0, 1) {
		a.Expected += scheduled[int(d.Weekday())]
	}
	// No scheduled day has come round yet — the first days of a month that opens
	// mid-week, or a period asked for before it begins. A rate out of nothing is
	// not a rate, so this stays on the measured frequency until there is a real
	// denominator.
	if a.Expected == 0 {
		return a
	}

	// Counted over the same window as Expected, so the two agree: "4 of 5
	// scheduled sessions" has to be four sessions out of five chances at them,
	// not every session of the month over the chances the program had.
	attended := 0
	for _, on := range performed {
		if !on.Before(from) && !on.After(end) {
			attended++
		}
	}

	a.Basis = AttendanceWeekday
	a.Actual = attended
	a.Rate = float64(attended) / float64(a.Expected)
	return a
}

func periodLabel(kind PeriodKind, start time.Time) string {
	if kind == PeriodYear {
		return fmt.Sprintf("%d", start.Year())
	}
	return fmt.Sprintf("%s %d", start.Month().String(), start.Year())
}

// Title is the recap's headline, used as the page heading and email subject.
func (r Report) Title() string {
	return strings.TrimSpace("Racked — " + r.Period.Label)
}
