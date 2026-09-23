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
	row, err := s.q.GetLifter(r.Context(), id)
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
	rows, err := s.q.ListLifters(r.Context())
	if err != nil {
		internalError(w)
		return
	}

	// Never nil, for the reason listUsers gives: an empty array is unreachable
	// while somebody is authenticated to read it, but a nil slice serializes as
	// null and a client should not have to treat null and [] as the same answer.
	lifters := make([]lifterDTO, 0, len(rows))
	for _, row := range rows {
		lifters = append(lifters, lifterDTO{
			ID:            row.ID,
			Username:      row.Username,
			DisplayName:   row.DisplayName,
			AvatarColor:   row.AvatarColor,
			HasAvatar:     row.AvatarEtag != "",
			AvatarEtag:    row.AvatarEtag,
			LastTrainedOn: dateToString(row.LastTrainedOn),
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

	writeJSON(w, http.StatusOK, lifterProfileDTO{
		lifterDTO: lifterDTO{
			ID:            row.ID,
			Username:      row.Username,
			DisplayName:   row.DisplayName,
			AvatarColor:   row.AvatarColor,
			HasAvatar:     row.AvatarEtag != "",
			AvatarEtag:    row.AvatarEtag,
			LastTrainedOn: dateToString(row.LastTrainedOn),
		},
		CurrentProgramID: row.CurrentProgramID,
		SessionCount:     totals.Total,
		LifetimeVolumeLb: numericToFloat(totals.VolumeLb),
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
