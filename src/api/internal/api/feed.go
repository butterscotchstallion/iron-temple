package api

import (
	"net/http"

	"gitea.homelab/gitadmin/iron-temple/api/internal/store"
)

// What the other lifters on this install have been doing.
//
// A read across accounts, like everything under /lifters, and the same rules
// apply — see the header comment there for why there is no visibility filter and
// why none of this writes. This sits at /feed rather than under /lifters because
// it is not about a lifter: the resource is the install's recent activity, and no
// id in the path names a person.
//
// Every judgement about which sessions belong here and what they are worth lives
// in ListFeedSessions, including the one that gives this endpoint its shape — the
// caller's own sessions are excluded in SQL. The handler below is a DTO mapping
// and nothing else, which is deliberate: a feed that decided anything in Go would
// be deciding it differently from the history page, which decides it in the
// query.

// getFeed serves a page of other lifters' recent sessions, newest first.
func (s *Server) getFeed(w http.ResponseWriter, r *http.Request) {
	limit, offset, ok := pageParams(w, r)
	if !ok {
		return
	}

	ctx := r.Context()
	rows, err := s.q.ListFeedSessions(ctx, store.ListFeedSessionsParams{
		ViewerID: userFrom(ctx).ID,
		Lim:      limit,
		Off:      offset,
	})
	if err != nil {
		internalError(w)
		return
	}

	// Never nil: a single-lifter install has an empty feed, which is the common
	// case rather than an edge one, and it must serialize as [] so the surfaces
	// drawing it can branch on length instead of on null.
	items := make([]feedEntryDTO, 0, len(rows))
	for _, row := range rows {
		items = append(items, feedEntryDTO{
			ID: row.ID,
			Lifter: lifterDTO{
				// row.UserID is nullable because the column is — a session older
				// than accounts has none. ListFeedSessions inner-joins users, so
				// no such row reaches here; deref through a local rather than
				// assuming, because a nil deref in a list handler is a 500 for
				// the whole page.
				ID:          lifterID(row.UserID),
				Username:    row.Username,
				DisplayName: row.DisplayName,
				AvatarColor: row.AvatarColor,
				HasAvatar:   row.AvatarEtag != "",
				AvatarEtag:  row.AvatarEtag,
				// LastTrainedOn is left empty on purpose: this is a fact about one
				// session, not about the lifter's whole record, and the roster is
				// where that question is answered.
			},
			ProgramID:         row.ProgramID,
			ProgramName:       row.ProgramName,
			ProgramDayID:      row.ProgramDayID,
			ProgramDayName:    row.ProgramDayName,
			PerformedOn:       dateToString(row.PerformedOn),
			SetCount:          row.SetCount,
			CompletedSetCount: row.CompletedSetCount,
			VolumeLb:          numericToFloat(row.VolumeLb),
			IsOver:            row.IsOver,
			ReactionCount:     row.ReactionCount,
			CommentCount:      row.CommentCount,
		})
	}

	writeJSON(w, http.StatusOK, feedDTO{Items: items, Limit: limit, Offset: offset})
}

// lifterID unwraps the nullable user_id a session row carries. Zero for an
// unowned session, which no feed row is — see the note at the call site.
func lifterID(id *int32) int32 {
	if id == nil {
		return 0
	}
	return *id
}
