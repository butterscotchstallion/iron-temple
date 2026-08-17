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
}

// VolumeLb is the weight this set moved: reps actually performed times the
// weight on the bar, matching SessionTotals rather than the prescription.
func (s Set) VolumeLb() float64 { return float64(s.Reps) * s.WeightLb }

// E1RM estimates a one-rep max by the Epley formula, rounded to the pound —
// the same calculation as estimateOneRepMax in the UI.
func (s Set) E1RM() float64 {
	if s.WeightLb <= 0 || s.Reps <= 0 {
		return 0
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
	Kind         PeriodKind
	Start        time.Time
	End          time.Time
	Loc          *time.Location
	Sets         []Set
	PreviousSets []Set
	Baseline     Baseline
	ProgramDays  []ProgramDay
}

// Report is the whole recap.
type Report struct {
	Period         Period
	Totals         Totals
	Change         *Change
	Comparison     Comparison
	Lifts          []LiftSlice
	Series         []LiftSeries
	MostImproved   *Improvement
	Days           []DayVolume
	Weekdays       []float64
	BestWeekday    int
	Hours          []int
	HourLabel      string
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

	rep := Report{
		Period: Period{
			Kind:  in.Kind,
			Start: in.Start,
			End:   in.End,
			Label: periodLabel(in.Kind, in.Start),
		},
		Totals:      totals(in.Sets, sessions),
		Lifts:       liftSlices(in.Sets),
		Series:      liftSeries(sessions),
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
	rep.Attendance = attendance(in.ProgramDays, len(sessions), in.Start, in.End)
	rep.Archetype = archetype(sessions, in.Start, in.End)

	fillWeekdays(&rep, sessions)
	fillHours(&rep, sessions, in.Loc)

	if prev := totals(in.PreviousSets, groupSessions(in.PreviousSets)); prev.Sessions > 0 {
		rep.Change = change(rep.Totals, prev)
	}
	return rep
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

func liftSlices(sets []Set) []LiftSlice {
	if len(sets) == 0 {
		return nil
	}
	byID := map[int32]*LiftSlice{}
	var total float64
	for _, s := range sets {
		cur, ok := byID[s.ExerciseID]
		if !ok {
			cur = &LiftSlice{ExerciseID: s.ExerciseID, ExerciseName: s.ExerciseName}
			byID[s.ExerciseID] = cur
		}
		cur.VolumeLb += s.VolumeLb()
		cur.Sets++
		cur.Reps += s.Reps
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
		for _, set := range sess.Sets {
			p, ok := best[set.ExerciseID]
			if !ok {
				p = &SeriesPoint{PerformedOn: sess.PerformedOn}
				best[set.ExerciseID] = p
				names[set.ExerciseID] = set.ExerciseName
			}
			p.TopWeightLb = math.Max(p.TopWeightLb, set.WeightLb)
			p.E1RMLb = math.Max(p.E1RMLb, set.E1RM())
		}
		for id, p := range best {
			series, ok := byID[id]
			if !ok {
				series = &LiftSeries{ExerciseID: id, ExerciseName: names[id]}
				byID[id] = series
				order = append(order, id)
			}
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
	modal, modalCount := -1, 0
	for _, s := range sessions {
		if s.StartedAt.IsZero() {
			continue
		}
		h := s.StartedAt.In(loc).Hour()
		rep.Hours[h]++
		if rep.Hours[h] > modalCount {
			modal, modalCount = h, rep.Hours[h]
		}
	}
	rep.HourLabel = hourLabel(modal)
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
func attendance(days []ProgramDay, actual int, start, end time.Time) Attendance {
	a := Attendance{
		Basis:           AttendanceNone,
		Actual:          actual,
		SessionsPerWeek: float64(actual) / weeksBetween(start, end),
	}

	scheduled := map[int]bool{}
	for _, d := range days {
		if d.Weekday != nil {
			scheduled[*d.Weekday] = true
		}
	}
	if len(scheduled) == 0 {
		return a
	}

	a.Basis = AttendanceWeekday
	for d := start; !d.After(end); d = d.AddDate(0, 0, 1) {
		if scheduled[int(d.Weekday())] {
			a.Expected++
		}
	}
	if a.Expected > 0 {
		a.Rate = float64(a.Actual) / float64(a.Expected)
	}
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
