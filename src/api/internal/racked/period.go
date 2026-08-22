package racked

import "time"

// ParsePeriodKind validates a period granularity from the wire.
func ParsePeriodKind(s string) (PeriodKind, bool) {
	switch PeriodKind(s) {
	case PeriodWeek:
		return PeriodWeek, true
	case PeriodMonth:
		return PeriodMonth, true
	case PeriodYear:
		return PeriodYear, true
	default:
		return "", false
	}
}

// Bounds returns the inclusive first and last date of the period containing on.
//
// Dates are held at UTC midnight throughout, matching pgtype.Date and the
// performed_on column: a session is recorded on a day, not at an instant, so
// shifting these into a zone would only introduce a way for a session logged
// late on the 31st to fall out of its own month.
//
// A week runs Monday to Sunday, via the same weekStart the streak counter has
// always used. Reusing it is the point rather than a convenience: a recap whose
// week disagreed with the week its own streak counts would report a two-week
// streak inside a one-week period.
func Bounds(kind PeriodKind, on time.Time) (time.Time, time.Time) {
	y, m, _ := on.Date()
	switch kind {
	case PeriodYear:
		start := time.Date(y, time.January, 1, 0, 0, 0, 0, time.UTC)
		return start, start.AddDate(1, 0, -1)
	case PeriodWeek:
		start := weekStart(on)
		return start, start.AddDate(0, 0, 6)
	default:
		start := time.Date(y, m, 1, 0, 0, 0, 0, time.UTC)
		return start, start.AddDate(0, 1, -1)
	}
}

// PreviousBounds returns the period immediately before the one containing on,
// which is what the recap compares against.
func PreviousBounds(kind PeriodKind, on time.Time) (time.Time, time.Time) {
	start, _ := Bounds(kind, on)
	if kind == PeriodYear {
		return Bounds(kind, start.AddDate(-1, 0, 0))
	}
	// One day back from the start lands in the preceding period for a month and
	// for a week alike — the Sunday before a Monday is the previous week.
	return Bounds(kind, start.AddDate(0, 0, -1))
}

// CatchUpWindow is how late a recap may still be sent.
//
// What stops a first deploy mailing years of back-issues is not this window but
// DuePeriods only ever considering the *most recently* completed month and
// year: an empty report_runs table means at most two recaps, never a history.
//
// In practice the window therefore binds only the annual recap. A month's end
// is at most 31 days behind any date inside the following month, so the
// previous month always falls inside 40 days and is always sent if it has not
// been — which is exactly the resilience wanted, since the API being down over
// the 1st should delay the recap rather than cancel it. The year is the case
// that needs a bound: without one, a deployment first started in September
// would mail a recap of a year that ended nine months earlier.
const CatchUpWindow = 40 * 24 * time.Hour

// Due is a completed period that a recap is owed for.
type Due struct {
	Kind  PeriodKind
	Start time.Time
	End   time.Time
}

// DuePeriods returns the periods a recap is owed for as of now.
//
// The reporter asks this question on a ticker rather than waking at midnight on
// the 1st, which is what makes it survive downtime: a job that misses its
// instant has missed it, but a question asked every hour gets the right answer
// the moment the process comes back. Whether a recap has already been sent is
// not decided here — that is report_runs' job — so this can be a pure function
// of the clock and stay easy to test at a year boundary.
//
// Only the most recently completed month and year are ever considered. On
// January 1st both are due and two emails go out, which is the specified
// behaviour.
func DuePeriods(now time.Time, window time.Duration) []Due {
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)

	// Month and year only, deliberately — PeriodWeek is a page a lifter opens,
	// not something to put in their inbox 52 times a year. Adding it here is a
	// one-line change, but it is a decision about how often to mail somebody
	// rather than a gap in the engine, and it wants a way to opt out before it
	// wants an implementation.
	var due []Due
	for _, kind := range []PeriodKind{PeriodMonth, PeriodYear} {
		start, end := PreviousBounds(kind, today)
		// end is the period's last day; it becomes complete the day after.
		if today.Sub(end) <= window {
			due = append(due, Due{Kind: kind, Start: start, End: end})
		}
	}
	return due
}
