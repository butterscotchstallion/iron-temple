package api

import (
	"math"
	"math/big"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// dateLayout is the wire format for date-only fields (OpenAPI `format: date`).
const dateLayout = "2006-01-02"

// numericToFloat flattens a Postgres NUMERIC to float64 for JSON. Weights are
// small (NUMERIC(6,2)), so float64 is exact enough for display and arithmetic.
// An invalid/NULL numeric reads as 0.
func numericToFloat(n pgtype.Numeric) float64 {
	if !n.Valid {
		return 0
	}
	f, err := n.Float64Value()
	if err != nil || !f.Valid {
		return 0
	}
	return f.Float64
}

// floatToNumeric builds a NUMERIC(6,2) from a float64 by scaling to hundredths,
// so the value round-trips without binary-float drift (e.g. 45.05 stays 45.05).
func floatToNumeric(f float64) pgtype.Numeric {
	hundredths := int64(math.Round(f * 100))
	return pgtype.Numeric{Int: big.NewInt(hundredths), Exp: -2, Valid: true}
}

// dateToString formats a date-only value; an invalid/NULL date reads as "".
func dateToString(d pgtype.Date) string {
	if !d.Valid {
		return ""
	}
	return d.Time.Format(dateLayout)
}

// parseDate parses an OpenAPI `format: date` string into a pgtype.Date.
func parseDate(s string) (pgtype.Date, error) {
	t, err := time.Parse(dateLayout, s)
	if err != nil {
		return pgtype.Date{}, err
	}
	return pgtype.Date{Time: t, Valid: true}, nil
}

// dateToday is the server's current date, used when a session omits performedOn.
func dateToday() pgtype.Date {
	return pgtype.Date{Time: time.Now(), Valid: true}
}

// timestamptzToString formats a timestamp as RFC 3339 (OpenAPI `date-time`).
func timestamptzToString(t pgtype.Timestamptz) string {
	if !t.Valid {
		return ""
	}
	return t.Time.UTC().Format(time.RFC3339)
}

// optionalTimestamptz formats a nullable timestamp for a nullable JSON field,
// where SQL NULL must serialize as null rather than the empty string.
func optionalTimestamptz(t pgtype.Timestamptz) *string {
	if !t.Valid {
		return nil
	}
	s := t.Time.UTC().Format(time.RFC3339)
	return &s
}
