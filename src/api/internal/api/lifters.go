package api

import (
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"

	"gitea.homelab/gitadmin/iron-temple/api/internal/store"
)

// One lifter reading another, for an install shared by a household or a gym
// crew.
//
// Until this file existed there was no way for one account to read another's
// training at all: every handler scoped its queries to userFrom(ctx).ID, and the
// only cross-account route in the app was the avatar image. That was not an
// oversight to be corrected so much as a property to be given up deliberately,
// and giving it up is what this file does.
//
// # EVERYTHING HERE IS READ-ONLY, BY CONSTRUCTION
//
// Every handler below takes a user id from the URL and uses it as a query
// parameter. Nothing under /lifters writes, and nothing under /lifters may ever
// be made to write — a path-supplied user id reaching an INSERT or an UPDATE is
// how one lifter edits another's history, and the shape of that bug is a
// forgotten ownership check rather than anything that would look wrong at the
// call site. The subtree has no write handler to forget it in, which is a
// stronger guarantee than a check every future handler has to remember. If a
// social feature ever does need a write (a reaction, a comment), it belongs on
// the resource being written — /sessions/{id}/… — where the caller is the
// authenticated user and the id in the path is not a person.
//
// # WHY THERE IS NO VISIBILITY FILTER
//
// There is no per-user privacy setting and no predicate to apply. Registration
// is first-user-only and every subsequent account is created by hand by the
// install's owner (see admin.go), so admission to the install is the consent.
// That is a decision about a homelab box with a handful of lifters on it and not
// a general claim: it is the reason a filter is absent, so anyone adding one is
// changing this premise rather than fixing an omission.
//
// THE PREMISE IS INTACT, AND FOLLOWS DO NOT CHANGE IT. 0033 added a follow table
// and the roster now reports whether the caller follows each row — but nothing has
// become unreadable. Every profile, session, board and crown is exactly as visible
// as it was, and the roster still lists everybody including the owner. A follow
// decides what the install PUSHES at a lifter, not what they may go and look at;
// the paragraph above is about the second, and this is the first. If a read here
// ever does grow a predicate, that will be the premise change this warns about.
//
// # WHY THE STATISTICS ARE NOT RECOMPUTED
//
// buildRacked and buildSessionRecap already take the user as a parameter rather
// than reading it from the request, so the reports below are the existing ones
// pointed at somebody else. That is deliberately all they are. A leaderboard or
// a profile that computed its own notion of volume would give the install two
// answers for one history, and the first time they disagreed there would be no
// way to say which was right.

// lifterFromPath resolves the {lifterId} path parameter to an account, writing
// the 404 itself when the id is malformed or names nobody. ok=false means the
// caller should stop.
//
// The existence check is not ceremony. buildRacked over an unknown user id is a
// set of queries that match no rows and reduce to a perfectly well-formed empty
// report, which would be served as a 200 — telling the caller that a lifter who
// does not exist trained nothing, rather than that they asked about nobody.
func (s *Server) lifterFromPath(
	w http.ResponseWriter, r *http.Request,
) (store.GetLifterRow, bool) {
	id, ok := idParam(r, "lifterId")
	if !ok {
		notFound(w, "lifter not found")
		return store.GetLifterRow{}, false
	}
	// The caller rides along so the row can say whether they follow this lifter.
	// Most callers of this helper want only the existence check and discard it;
	// getLifter is the one that draws a Follow button from it.
	row, err := s.q.GetLifter(r.Context(), store.GetLifterParams{
		ID:       id,
		ViewerID: userFrom(r.Context()).ID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		notFound(w, "lifter not found")
		return store.GetLifterRow{}, false
	}
	if err != nil {
		internalError(w)
		return store.GetLifterRow{}, false
	}
	return row, true
}

// listLifters serves the roster: everyone on this install, oldest account first.
func (s *Server) listLifters(w http.ResponseWriter, r *http.Request) {
	rows, err := s.q.ListLifters(r.Context(), userFrom(r.Context()).ID)
	if err != nil {
		internalError(w)
		return
	}

	// Never nil, for the reason listUsers gives: an empty array is unreachable
	// while somebody is authenticated to read it, but a nil slice serializes as
	// null and a client should not have to treat null and [] as the same answer.
	lifters := make([]lifterDTO, 0, len(rows))
	for _, row := range rows {
		// Copied to a local before its address is taken. Correct as `&row.Is…`
		// under per-iteration loop variables too, but the local says so without
		// the reader having to know which Go version this is.
		following := row.IsFollowing
		lifters = append(lifters, lifterDTO{
			ID:            row.ID,
			Username:      row.Username,
			DisplayName:   row.DisplayName,
			AvatarColor:   row.AvatarColor,
			HasAvatar:     row.AvatarEtag != "",
			AvatarEtag:    row.AvatarEtag,
			LastTrainedOn: dateToString(row.LastTrainedOn),
			Following:     &following,
		})
	}
	writeJSON(w, http.StatusOK, lifters)
}

// getLifter serves one lifter's profile: who they are, what they are running,
// and what they have lifted in total.
func (s *Server) getLifter(w http.ResponseWriter, r *http.Request) {
	row, ok := s.lifterFromPath(w, r)
	if !ok {
		return
	}

	// No program filter, so this is the lifter's whole history — the same query,
	// and therefore the same definition of a session that logged something, that
	// the history page's own totals use.
	totals, err := s.q.SessionTotals(r.Context(), store.SessionTotalsParams{UserID: row.ID})
	if err != nil {
		internalError(w)
		return
	}

	// The profile is the other surface that draws a Follow button, so it reports the
	// same field the roster does — users.sql keeps the two queries' column lists in
	// step precisely so this cannot be filled on one path and left nil on the other.
	following := row.IsFollowing

	writeJSON(w, http.StatusOK, lifterProfileDTO{
		lifterDTO: lifterDTO{
			ID:            row.ID,
			Username:      row.Username,
			DisplayName:   row.DisplayName,
			AvatarColor:   row.AvatarColor,
			HasAvatar:     row.AvatarEtag != "",
			AvatarEtag:    row.AvatarEtag,
			LastTrainedOn: dateToString(row.LastTrainedOn),
			Following:     &following,
		},
		CurrentProgramID: row.CurrentProgramID,
		SessionCount:     totals.Total,
		LifetimeVolumeLb: numericToFloat(totals.VolumeLb),
	})
}

// getLifterSessions serves another lifter's history — listSessions, pointed
// elsewhere.
//
// This is what makes a profile somewhere to go. Before it, the only route to
// getLifterSessionRecap — the screen where one lifter applauds another — was the
// feed, which is everybody's sessions interleaved and paged.
//
// TWO USER IDS, and keeping them apart is the point. The lifter from the path
// scopes the rows; the caller decides only whether a private program's name is
// legible. The query takes both and uses the second in a CASE and nowhere else,
// so a viewer cannot widen what they are shown by being the viewer — see
// ListLifterSessions.
//
// The per-exercise weights are keyed on the LIFTER, not on the caller.
// ListSessionExerciseWeights is scoped by user id like everything else in
// sessions.go, and passing the caller's here would silently return nothing: the
// page would draw, every session would claim to have no lifts in it, and nothing
// would look like an error.
func (s *Server) getLifterSessions(w http.ResponseWriter, r *http.Request) {
	row, ok := s.lifterFromPath(w, r)
	if !ok {
		return
	}
	limit, offset, ok := pageParams(w, r)
	if !ok {
		return
	}

	ctx := r.Context()
	rows, err := s.q.ListLifterSessions(ctx, store.ListLifterSessionsParams{
		LifterID: row.ID,
		ViewerID: userFrom(ctx).ID,
		Lim:      limit,
		Off:      offset,
	})
	if err != nil {
		internalError(w)
		return
	}

	// The lifter's whole history rather than this page, which is the same
	// promise GET /sessions makes — and the same two figures getLifter above
	// already reports, from the same query, so a profile and its session list
	// cannot disagree about how much somebody has lifted.
	totals, err := s.q.SessionTotals(ctx, store.SessionTotalsParams{UserID: row.ID})
	if err != nil {
		internalError(w)
		return
	}

	ids := make([]int32, 0, len(rows))
	for _, session := range rows {
		ids = append(ids, session.ID)
	}
	weightsBySession := make(map[int32][]sessionExerciseWeightDTO, len(rows))
	if len(ids) > 0 {
		weights, err := s.q.ListSessionExerciseWeights(ctx, store.ListSessionExerciseWeightsParams{
			SessionIds: ids, UserID: row.ID,
		})
		if err != nil {
			internalError(w)
			return
		}
		for _, wt := range weights {
			weightsBySession[wt.SessionID] = append(weightsBySession[wt.SessionID], sessionExerciseWeightDTO{
				ExerciseName: wt.ExerciseName,
				Sets:         wt.SetCount,
				Reps:         wt.Reps,
				WeightLb:     numericToFloat(wt.WeightLb),
			})
		}
	}

	items := make([]sessionSummaryDTO, 0, len(rows))
	for _, session := range rows {
		exercises := weightsBySession[session.ID]
		if exercises == nil {
			exercises = []sessionExerciseWeightDTO{}
		}
		items = append(items, sessionSummaryDTO{
			ID:                session.ID,
			ProgramID:         session.ProgramID,
			ProgramName:       session.ProgramName,
			ProgramDayID:      session.ProgramDayID,
			ProgramDayName:    session.ProgramDayName,
			PerformedOn:       dateToString(session.PerformedOn),
			SetCount:          session.SetCount,
			CompletedSetCount: session.CompletedSetCount,
			VolumeLb:          numericToFloat(session.VolumeLb),
			IsOver:            session.IsOver,
			Exercises:         exercises,
		})
	}

	writeJSON(w, http.StatusOK, sessionListDTO{
		Items: items, Total: totals.Total, TotalVolumeLb: numericToFloat(totals.VolumeLb),
		Limit: limit, Offset: offset,
	})
}

// getLifterRacked serves another lifter's Racked report — getRacked, pointed
// elsewhere.
func (s *Server) getLifterRacked(w http.ResponseWriter, r *http.Request) {
	row, ok := s.lifterFromPath(w, r)
	if !ok {
		return
	}
	kind, on, today, ok := s.rackedWindow(w, r)
	if !ok {
		return
	}

	report, err := s.buildRacked(r.Context(), row.ID, kind, on, today)
	if err != nil {
		internalError(w)
		return
	}
	writeJSON(w, http.StatusOK, rackedReportToDTO(report))
}

// getLifterSessionRecap serves another lifter's session recap.
//
// No lifterFromPath here, and that is not an inconsistency. buildSessionRecap
// reads the session through GetSession, which is scoped by (id, user_id): the
// pair in the URL either names a session that lifter performed or matches no row
// at all, and the second case is already a 404. Checking the account existed
// first would cost a query to distinguish "no such lifter" from "not their
// session" — a distinction this endpoint deliberately does not draw, since
// answering it would let a caller enumerate which ids are accounts.
func (s *Server) getLifterSessionRecap(w http.ResponseWriter, r *http.Request) {
	lifterID, ok := idParam(r, "lifterId")
	if !ok {
		notFound(w, "session not found")
		return
	}
	sessionID, ok := idParam(r, "sessionId")
	if !ok {
		notFound(w, "session not found")
		return
	}

	ctx := r.Context()
	rec, earned, err := s.buildSessionRecap(ctx, sessionID, lifterID)
	if errors.Is(err, pgx.ErrNoRows) {
		notFound(w, "session not found")
		return
	}
	if err != nil {
		internalError(w)
		return
	}

	// The one field in this response that is a fact about a PROGRAM rather than
	// about a performance.
	//
	// buildSessionRecap is scoped to the lifter whose session it is — that is
	// what makes the id pair in the URL the authorization — so unlike every
	// self-scoped caller of GetSession it can return the name of a program the
	// VIEWER was never shown. Everything else here is what somebody lifted,
	// which the feed exists to show; a private program's name is not.
	//
	// Masked here rather than in the query, because GetSession's nine other
	// callers are all reading their own sessions and would have to pass the same
	// id twice to satisfy a viewer parameter they do not need. ListFeed masks in
	// SQL for the opposite reason: it returns a page of rows and would otherwise
	// need a query each.
	readable, err := s.q.CanReadProgram(ctx, store.CanReadProgramParams{
		ProgramID: rec.Session.ProgramID, UserID: userFrom(ctx).ID,
	})
	if err != nil {
		internalError(w)
		return
	}
	if !readable {
		rec.Session.ProgramName = maskedProgramName
	}

	writeJSON(w, http.StatusOK, sessionRecapToDTO(rec, earned))
}
