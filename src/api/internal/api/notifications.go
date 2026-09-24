package api

import (
	"context"
	"log"
	"net/http"

	"gitea.homelab/gitadmin/iron-temple/api/internal/store"
)

// What happened to you, addressed to you.
//
// 0024 gave the install applause and conversation; this is the half that tells
// anybody about it. Before it, the only way to learn somebody had reacted to
// your session was to open that session again and look.
//
// THIS RESOURCE HAS NO POST, AND THAT IS THE DESIGN
//
// A notification is never sent. It is raised as a consequence of something else
// — applauding a session, commenting on one, making an account — and every one
// of those already has an endpoint that owns the rule for who may do it. So the
// only writes here are the two the OWNER of a notification may make about their
// own attention: mark them read, and clear them. There is deliberately no way
// for one account to put a row in another account's panel except by doing
// something the rest of the API already lets them do.
//
// The fan-out itself is not in this file. It lives in notifications.sql as
// INSERT ... SELECT, because the same rule has to run from the recognition
// handlers and from the generated-activity scheduler, and a rule expressed
// twice in Go is a rule that will eventually be expressed two different ways.
// The call sites here are one line each.
//
// WHAT THIS IS BUILT FOR
//
// Polling. The header asks for this once a minute for as long as somebody is
// signed in, which makes it the most-requested authenticated endpoint in the
// app. It is a GET under the authenticated group, so jsonETag answers 304 for
// every poll where nothing has changed — which on a household install is nearly
// all of them. Nothing here is expensive enough to need more than that, and the
// two indexes 0026 adds are what keep it that way.

// notificationRetentionDays is how long a notification stays in the panel.
//
// A month, which is the window this app already thinks in — Racked opens on the
// month, and "did anybody say anything about my training recently" has the same
// horizon. Long enough that nothing a lifter might still want to find is taken
// away, short enough that the live table stays small on an install where the
// generated-activity scheduler adds to it daily.
//
// A constant rather than an environment variable. Nothing else in this app is
// tuned that way, and a knob nobody turns is a knob that goes untested — if this
// ever needs to differ per install, it belongs in the settings table 0025
// already established rather than in the process's environment.
const notificationRetentionDays = 30

// archiveOldNotifications is the retention pass, run from the session sweeper.
//
// It hangs off that loop rather than owning one because it is the same kind of
// work for the same reason: nothing depends on it for correctness, and it exists
// so a table does not grow without bound. Two cheap statements on one hourly
// ticker beats a second ticker.
//
// No claim table either, unlike the daily generator. That one claims because a
// day must be generated AT MOST ONCE and a duplicate would invent training that
// never happened; this is idempotent — a second pass finds nothing left to
// archive — so two replicas racing costs one wasted UPDATE and nothing else.
//
// Errors are logged and swallowed. A failed sweep is retried an hour later by
// construction, and there is nobody to tell.
func (s *Server) archiveOldNotifications(ctx context.Context) {
	archived, err := s.q.ArchiveOldNotifications(ctx, notificationRetentionDays)
	if err != nil {
		log.Printf("notification archive: %v", err)
		return
	}
	// Only when something moved. An hourly no-op is the steady state and does
	// not need a line in the log every hour to say so.
	if archived > 0 {
		log.Printf("notification archive: %d older than %d days",
			archived, notificationRetentionDays)
	}
}

// listNotifications serves a page of the caller's notifications, newest first,
// with the unread count over all of them.
//
// One request rather than two. The badge and the panel are read on the same
// schedule by the same component, and splitting the count onto its own endpoint
// would double the poll to save assembling a page nothing is forced to draw.
func (s *Server) listNotifications(w http.ResponseWriter, r *http.Request) {
	limit, offset, ok := pageParams(w, r)
	if !ok {
		return
	}

	ctx := r.Context()
	caller := userFrom(ctx).ID

	rows, err := s.q.ListNotifications(ctx, store.ListNotificationsParams{
		UserID: caller,
		Lim:    limit,
		Off:    offset,
	})
	if err != nil {
		internalError(w)
		return
	}

	unread, err := s.q.CountUnreadNotifications(ctx, caller)
	if err != nil {
		internalError(w)
		return
	}

	// Never nil: an install where nothing has happened yet is the common case,
	// and it must serialize as [] so the panel branches on length and not on
	// null.
	items := make([]notificationDTO, 0, len(rows))
	for _, row := range rows {
		item := notificationDTO{
			ID:   row.ID,
			Kind: row.Kind,
			Actor: lifterDTO{
				ID:          row.ActorID,
				Username:    row.Username,
				DisplayName: row.DisplayName,
				AvatarColor: row.AvatarColor,
				HasAvatar:   row.AvatarEtag != "",
				AvatarEtag:  row.AvatarEtag,
				// LastTrainedOn stays empty, as it does on a comment's author:
				// this is a fact about one thing that happened, not a report on
				// the actor's training.
			},
			// The nullable columns are passed through as they came. Which of
			// them is populated is decided by Kind, and the DTO's omitempty
			// tags mean a row on the wire carries only the ones its kind gives
			// meaning to.
			SessionID:      row.SessionID,
			SessionOwnerID: row.SessionOwnerID,
			ProgramDayName: row.ProgramDayName,
			Emoji:          row.Emoji,
			CommentID:      row.CommentID,
			CommentBody:    row.CommentBody,
			CreatedAt:      timestamptzToString(row.CreatedAt),
			// Empty while unread, and omitempty drops it — which is the same
			// wire shape a null would give and is what the client tests.
			ReadAt: timestamptzToString(row.ReadAt),
		}
		items = append(items, item)
	}

	writeJSON(w, http.StatusOK, notificationListDTO{
		Items:       items,
		Limit:       limit,
		Offset:      offset,
		UnreadCount: unread,
	})
}

// markNotificationsRead drops the caller's unread count to zero.
//
// 204 and idempotent. The query only stamps rows that are still unread, so a
// second call writes nothing rather than moving every timestamp forward — what
// readAt records is when a notification was FIRST read.
func (s *Server) markNotificationsRead(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := s.q.MarkNotificationsRead(ctx, userFrom(ctx).ID); err != nil {
		internalError(w)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// markNotificationRead stamps one, for a lifter who followed a notification
// through to what it was about.
//
// The other half of "mark all read", and not a decomposition of it. Opening the
// panel is a glance and must not read anything — that is what makes coming back
// to what was new possible — but following a row to the comment it quotes is
// exactly the act of having read that one. Without this the only way to clear
// the badge was to mark everything read, which buries the rows that have not
// been looked at.
//
// No ownership check, and that is deliberate rather than missing:
// MarkNotificationRead scopes on user_id inside the UPDATE, so another
// account's id matches no row. 204 either way — a 404 would confirm the id
// exists, and there is nothing useful for a client to do differently.
func (s *Server) markNotificationRead(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(r, "notificationId")
	if !ok {
		badRequest(w, "notificationId must be a positive integer")
		return
	}

	ctx := r.Context()
	// The row count is deliberately discarded. "Already read", "not yours" and
	// "never existed" are all the same answer to the only question the caller
	// asked, which is for a state rather than for a change.
	if _, err := s.q.MarkNotificationRead(ctx, store.MarkNotificationReadParams{
		ID:     id,
		UserID: userFrom(ctx).ID,
	}); err != nil {
		internalError(w)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// clearNotifications empties the caller's panel, deleting the rows.
//
// 204 whether or not anything went, like removeSessionReaction: the caller
// asked for a state, and that state holds either way.
//
// Nothing that mattered is lost. The applause and the conversation live on
// their sessions and are untouched by this; what goes is the record that the
// caller had been told about them.
func (s *Server) clearNotifications(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := s.q.ClearNotifications(ctx, userFrom(ctx).ID); err != nil {
		internalError(w)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
