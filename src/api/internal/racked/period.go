package racked

import "time"

// ParsePeriodKind validates a period granularity from the wire.
func ParsePeriodKind(s string) (PeriodKind, bool) {
	switch PeriodKind(s) {
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
func Bounds(kind PeriodKind, on time.Time) (time.Time, time.Time) {
	y, m, _ := on.Date()
	if kind == PeriodYear {
		start := time.Date(y, time.January, 1, 0, 0, 0, 0, time.UTC)
		return start, start.AddDate(1, 0, -1)
	}
	start := time.Date(y, m, 1, 0, 0, 0, 0, time.UTC)
	return start, start.AddDate(0, 1, -1)
}

// PreviousBounds returns the period immediately before the one containing on,
// which is what the recap compares against.
func PreviousBounds(kind PeriodKind, on time.Time) (time.Time, time.Time) {
	start, _ := Bounds(kind, on)
	if kind == PeriodYear {
		return Bounds(kind, start.AddDate(-1, 0, 0))
	}
	return Bounds(kind, start.AddDate(0, 0, -1))
}
