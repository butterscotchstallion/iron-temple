package api

import (
	"context"
	"time"

	"gitea.homelab/gitadmin/iron-temple/api/internal/progression"
	"gitea.homelab/gitadmin/iron-temple/api/internal/store"
)

// How long the lifter has been away, and whether they asked us to do anything
// about it. Threaded into prescribe() so the two surfaces that build a
// prescription — the preview and session creation — cannot disagree about the
// weight on the bar, for the same reason prescribe() itself is shared.
type layoffState struct {
	// weeks is full weeks since the last session that counts as training; 0
	// when they trained this week or have never trained at all.
	weeks int
	// lastTrainedOn is the date of that session (YYYY-MM-DD), "" when there
	// isn't one.
	lastTrainedOn string
	// apply is the lifter's answer. False leaves every weight alone and makes
	// the rest of this advisory — the state is still reported, which is how the
	// UI knows there is anything to ask about.
	apply bool
}

// active reports whether this state should actually change any weight.
func (l layoffState) active() bool {
	return l.apply && progression.LayoffPct(l.weeks) > 0
}

// dto renders the state for the wire, or nil when there is no layoff to speak
// of. Nil rather than a zeroed object on purpose: "you have been away 0 weeks"
// is not a thing to tell someone who trained yesterday, and the UI's whole
// decision about whether to prompt is this field's presence.
func (l layoffState) dto() *layoffDTO {
	pct := progression.LayoffPct(l.weeks)
	if pct == 0 {
		return nil
	}
	return &layoffDTO{
		Weeks:         l.weeks,
		LastTrainedOn: l.lastTrainedOn,
		DeloadPct:     pct,
		Applied:       l.apply,
	}
}

// layoffFor measures how long a lifter has been away from training.
//
// "Trained" is the definition ListSessions already enforces in SQL — a session
// with at least one logged rep — so a session started and abandoned does not
// quietly count as a week of training. That is why this reads through that
// query with a limit of 1 rather than asking for a MAX(performed_on): the
// definition lives in one place, and a second query is a second place for it to
// drift. Deliberately not scoped to a program, either. Time off is time off;
// switching programs is not a layoff.
//
// A lifter with no sessions at all gets a zero state. They are not coming back
// from anything, and offering to deload a program they have never run would be
// asking a question with no meaning behind it.
func (s *Server) layoffFor(ctx context.Context, userID int32, apply bool) (layoffState, error) {
	rows, err := s.q.ListSessions(ctx, store.ListSessionsParams{
		UserID: userID, ProgramID: nil, Off: 0, Lim: 1,
	})
	if err != nil {
		return layoffState{}, err
	}
	if len(rows) == 0 || !rows[0].PerformedOn.Valid {
		return layoffState{}, nil
	}

	last := rows[0].PerformedOn.Time
	return layoffState{
		weeks:         weeksSince(last, dateToday().Time),
		lastTrainedOn: dateToString(rows[0].PerformedOn),
		apply:         apply,
	}, nil
}

// weeksSince is whole weeks between two days, floored — six days off is 0 and
// seven is 1, which is what makes "hasn't trained in a week" mean the week and
// not the calendar.
//
// Both ends are truncated to their date before subtracting. The stored value is
// already a date at midnight but `now` is not, and an untruncated subtraction
// makes the answer depend on the time of day the lifter opens the app: 6 days
// and 20 hours is not 7 days, so the prompt would appear in the evening and
// vanish again by morning.
//
// A future date (a session logged ahead, a clock skew) gives a negative count,
// which progression.LayoffPct reads as no layoff.
func weeksSince(last, now time.Time) int {
	days := int(truncateToDay(now).Sub(truncateToDay(last)).Hours() / 24)
	return days / 7
}

func truncateToDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}
