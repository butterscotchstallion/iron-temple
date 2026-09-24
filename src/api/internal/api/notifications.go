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

// maxNamedOthers is how many of a group's other actors ride along on the wire.
//
// A WIRE BUDGET, NOT A COPY DECISION. The panel currently says "Bob, Cara and 4
// others", which needs one of these — the second is headroom so the sentence can
// change without the contract changing. What it is really here to stop is an
// install with forty lifters putting forty names on every row of every poll, on
// the endpoint the header asks for once a minute.
const maxNamedOthers = 2

// groupActors folds a group's parallel actor arrays into the number of DISTINCT
// people it represents and the names of the ones the row does not already carry.
//
// Both halves of that are the reason this is in Go rather than in the query.
// Postgres has no DISTINCT for window aggregates — no count(DISTINCT x) OVER w,
// no array_agg(DISTINCT x) OVER w — so ListNotificationGroups hands back every
// member's actor id and name in the panel's own order, duplicates included, and
// the folding happens once, here.
//
// Duplicates are real rather than theoretical: one lifter applauding a session
// with two different emoji is two notifications, and they are one person in the
// sentence. So the dedupe is BY ID, never by name — two accounts that chose the
// same display name are two people, and counting them as one would be a quieter
// bug than it sounds.
//
// The representative actor is seeded into the set rather than skipped by
// position, so the count is 1 even on the arrays a row of one produces and the
// row's own actor can never be named twice. Which also means this does not
// depend on the query's ordering to be correct — only on it to be pleasant.
func groupActors(actorID int32, ids []int32, names []string) (int32, []string) {
	seen := map[int32]struct{}{actorID: {}}
	count := int32(1)
	var others []string

	for i, id := range ids {
		// Parallel arrays out of one window, so they cannot disagree in length.
		// Bounded anyway: the alternative to this line is a panic in the header
		// of every screen.
		if i >= len(names) {
			break
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		count++
		if len(others) < maxNamedOthers {
			others = append(others, names[i])
		}
	}

	return count, others
}

// listNotifications serves a page of the caller's notifications, newest first,
// with the unread count over all of them.
//
// One request rather than two. The badge and the panel are read on the same
// schedule by the same component, and splitting the count onto its own endpoint
// would double the poll to save assembling a page nothing is forced to draw.
//
// A PAGE OF GROUPS, and both numbers in the response are counted that way. The
// query folds every notification about the same subject into one row, so twenty
// items is twenty things that happened rather than twenty rows that might all be
// the same thing, and the unread count counts groups so the badge agrees with
// the list under it. See ListNotificationGroups for why the folding is in SQL.
func (s *Server) listNotifications(w http.ResponseWriter, r *http.Request) {
	limit, offset, ok := pageParams(w, r)
	if !ok {
		return
	}

	ctx := r.Context()
	caller := userFrom(ctx).ID

	rows, err := s.q.ListNotificationGroups(ctx, store.ListNotificationGroupsParams{
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
		actorCount, otherNames := groupActors(row.ActorID, row.ActorIds, row.ActorNames)
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
			// Who else is in this row. 1 and empty on a group of one, which is
			// most rows on most installs.
			ActorCount:      actorCount,
			OtherActorNames: otherNames,
			// The nullable columns are passed through as they came, and on a
			// group they belong to its newest member — so a fold of three
			// comments quotes and links to the most recent one. Which of them is
			// populated is decided by Kind, and the DTO's omitempty tags mean a
			// row on the wire carries only the ones its kind gives meaning to.
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
		// Withheld unless the whole group named ONE achievement. The query
		// decides that — see its note on why a crown group is the one fold whose
		// subject is not simply its representative's — and this honours the
		// answer rather than second-guessing it.
		if row.OneAchievement {
			item.AchievementSlug = row.AchievementSlug
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

// getNotificationGroupMembers unfolds one row of the panel into the notifications
// it stands for.
//
// The panel deliberately says less than it knows: a row carries its newest
// member's subject, and for a crown it withholds even that unless every member
// agrees on one board. "Grace and 2 others took crowns" is the honest one-line
// summary and it leaves the other two unnamed, so a surface that wants to detail
// them asks here.
//
// NO PAGING. Every other list in this API takes limit and offset; this one is the
// expansion of a row the caller is already looking at, bounded by the retention
// window rather than by a page — and a client that has been told a row folds three
// things cannot be handed two of them.
//
// A 404 covers every way this can fail to resolve: an id that names nothing, one
// that belongs to another account, and one that has been archived out of the
// panel. Answering them identically is what keeps the endpoint from confirming
// that somebody else's notification id exists — the same reasoning
// MarkNotificationGroupRead gives for having no 403.
func (s *Server) getNotificationGroupMembers(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(r, "notificationId")
	if !ok {
		notFound(w, "notification not found")
		return
	}

	ctx := r.Context()
	rows, err := s.q.ListNotificationGroupMembers(ctx, store.ListNotificationGroupMembersParams{
		ID:     id,
		UserID: userFrom(ctx).ID,
	})
	if err != nil {
		internalError(w)
		return
	}
	// A live row the caller owns always matches at least itself, so no rows means
	// the group could not be resolved rather than that it is empty.
	if len(rows) == 0 {
		notFound(w, "notification not found")
		return
	}

	// One DTO per member, and it is the SAME type the panel's rows use, because a
	// member is a notification — there is nothing about it that a row of the panel
	// is not also. This mapping and listNotifications' above have to stay in step;
	// a field added to one and not the other is a detail dialog that silently says
	// less than the row it was opened from.
	//
	// ActorCount is 1 and OtherActorNames absent on every one of these: a member
	// folds nobody, which is exactly what those two fields report.
	//
	// AchievementSlug is passed through UNCONDITIONALLY, which is the one place
	// this mapping deliberately differs from the panel's. Up there it is gated on
	// the group agreeing about which board was won; down here a row is a single
	// notification, so its board is not in doubt and withholding it would defeat
	// the only reason a client asked.
	items := make([]notificationDTO, 0, len(rows))
	for _, row := range rows {
		items = append(items, notificationDTO{
			ID:   row.ID,
			Kind: row.Kind,
			Actor: lifterDTO{
				ID:          row.ActorID,
				Username:    row.Username,
				DisplayName: row.DisplayName,
				AvatarColor: row.AvatarColor,
				HasAvatar:   row.AvatarEtag != "",
				AvatarEtag:  row.AvatarEtag,
			},
			ActorCount:      1,
			SessionID:       row.SessionID,
			SessionOwnerID:  row.SessionOwnerID,
			ProgramDayName:  row.ProgramDayName,
			Emoji:           row.Emoji,
			CommentID:       row.CommentID,
			CommentBody:     row.CommentBody,
			AchievementSlug: row.AchievementSlug,
			CreatedAt:       timestamptzToString(row.CreatedAt),
			ReadAt:          timestamptzToString(row.ReadAt),
		})
	}

	writeJSON(w, http.StatusOK, notificationMemberListDTO{Items: items})
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

// markNotificationRead stamps one ROW OF THE PANEL, for a lifter who followed a
// notification through to what it was about.
//
// The other half of "mark all read", and not a decomposition of it. Opening the
// panel is a glance and must not read anything — that is what makes coming back
// to what was new possible — but following a row to the comment it quotes is
// exactly the act of having read that one. Without this the only way to clear
// the badge was to mark everything read, which buries the rows that have not
// been looked at.
//
// A ROW IS A GROUP, so this marks everything that row folded. The id comes back
// from listNotifications and is the group's newest member; the query resolves the
// rest from it, so the grouping rule stays in SQL and the client never has to
// know what it is. Stamping only that one member would hand the row straight back
// with its dot still on — a badge that cannot be cleared except by the blunt
// instrument this endpoint exists to avoid.
//
// No ownership check, and that is deliberate rather than missing:
// MarkNotificationGroupRead scopes on user_id inside the UPDATE, so another
// account's id matches no row. 204 either way — a 404 would confirm the id
// exists, and there is nothing useful for a client to do differently.
func (s *Server) markNotificationRead(w http.ResponseWriter, r *http.Request) {
	id, ok := idParam(r, "notificationId")
	if !ok {
		badRequest(w, "notificationId must be a positive integer")
		return
	}

	ctx := r.Context()
	// The row count is deliberately discarded, and it is a count of the group's
	// members now rather than 0-or-1. "Already read", "not yours" and "never
	// existed" are all the same answer to the only question the caller asked,
	// which is for a state rather than for a change.
	if _, err := s.q.MarkNotificationGroupRead(ctx, store.MarkNotificationGroupReadParams{
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
