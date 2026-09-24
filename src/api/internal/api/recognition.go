package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"gitea.homelab/gitadmin/iron-temple/api/internal/store"
)

// Applause and conversation on a session.
//
// These hang off /sessions rather than /lifters, and the distinction is the one
// lifters.go's header draws: the id in the path names a SESSION, and the actor is
// always the authenticated caller. That is what makes it safe for these to write
// where nothing under /lifters may — there is no path parameter here that says
// who is doing the writing, so there is no ownership check to forget.
//
// WHAT THESE DO NOT DO
//
// They are not in the offline write queue, and must not be added to it. That queue
// exists because a tap at a rack in a basement has to survive having no network
// (see src/ui/src/lib/writeQueue.svelte.ts). Reacting to somebody's workout is a
// couch activity: the request happens where there is signal, nothing is lost if it
// fails, and a reaction replayed twenty minutes later is a reaction, not a rescued
// rep. Putting it in the queue would add a durable-state problem to a feature that
// does not have one.
//
// They are also not part of the session recap response. That one is built to be
// answerable from memory with a dead network; applause is live and arrives after
// the fact, so folding it in would cost the recap the one property it was designed
// around.

// allowedReactions is what this install offers.
//
// A fixed set, validated here rather than constrained by the database. Which emoji
// a lifting app should offer is a product question that will change, and a
// migration per emoji is a poor trade — worse, a CHECK would have to be widened
// before the deploy that offers a new one and narrowed after a rollback, which is
// a two-step dance around a one-line constant.
//
// Free-form would be the other option and is worse than it looks: it makes the
// column a place to put arbitrary strings, and "reaction" stops meaning anything a
// surface can lay out.
//
// MUST AGREE WITH the ReactionEmoji enum in openapi.yaml, which is what the UI
// generates its buttons from. The spec is the contract and this is the
// enforcement: a generated client is a convenience, not a guarantee about what
// arrives on the wire, so both exist on purpose. Adding one means editing both.
var allowedReactions = map[string]bool{
	"💪": true, // the obvious one
	"🔥": true,
	"👏": true,
	"🎉": true,
}

// maxCommentBody bounds a comment, counted in RUNES rather than bytes so that a
// comment of 256 emoji is accepted instead of refused for being 1 KB. The column
// is plain TEXT; this is the only place the limit lives.
const maxCommentBody = 256

// How many comments one account may post per window.
//
// Posting is the only write a signed-in account can repeat without limit, and
// every one of them puts a row in somebody else's panel — so the thing being
// bounded is not load, it is how much of another lifter's attention one account
// can spend. Set where a conversation never reaches it: twenty in five minutes
// is faster than anybody talks about a workout, and slow enough that a script
// cannot fill a panel.
//
// Not a moderation tool. On a box shared by a household there is nobody to
// appeal to, and the answer to somebody being unpleasant is the account, not the
// rate.
const (
	maxCommentsPerWindow = 20
	commentWindow        = 5 * time.Minute
)

// sessionForRecognition resolves the {sessionId} path parameter to a session and
// its owner, writing the 404 itself. ok=false means the caller should stop.
//
// SessionExists, NOT GetSession. That is the whole subtlety of this file:
// GetSession is scoped by (id, user_id) and would answer 404 for every session
// except the caller's own — which is to say, for every session this feature
// exists to act on. The ownership check is not being skipped, it is being replaced
// by a different rule, and the rule is that anyone on the install may applaud
// anything on it. That is the same premise /lifters rests on, recorded there.
func (s *Server) sessionForRecognition(
	w http.ResponseWriter, r *http.Request,
) (store.SessionExistsRow, bool) {
	id, ok := idParam(r, "sessionId")
	if !ok {
		notFound(w, "session not found")
		return store.SessionExistsRow{}, false
	}
	row, err := s.q.SessionExists(r.Context(), id)
	if errors.Is(err, pgx.ErrNoRows) {
		notFound(w, "session not found")
		return store.SessionExistsRow{}, false
	}
	if err != nil {
		internalError(w)
		return store.SessionExistsRow{}, false
	}
	return row, true
}

// ---- reactions ----

type sessionReactionRequest struct {
	Emoji string `json:"emoji"`
}

// listSessionReactions serves a session's applause, grouped by emoji.
func (s *Server) listSessionReactions(w http.ResponseWriter, r *http.Request) {
	session, ok := s.sessionForRecognition(w, r)
	if !ok {
		return
	}

	ctx := r.Context()
	rows, err := s.q.ListSessionReactions(ctx, store.ListSessionReactionsParams{
		SessionID: session.ID,
		ViewerID:  userFrom(ctx).ID,
	})
	if err != nil {
		internalError(w)
		return
	}

	// Never nil: most sessions have no applause, which is the common case and must
	// serialize as [] so a surface branches on length rather than on null.
	out := make([]sessionReactionDTO, 0, len(rows))
	for _, row := range rows {
		out = append(out, sessionReactionDTO{
			Emoji: row.Emoji,
			Count: row.Total,
			Mine:  row.Mine,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

// addSessionReaction records applause. Idempotent — see AddSessionReaction.
func (s *Server) addSessionReaction(w http.ResponseWriter, r *http.Request) {
	session, ok := s.sessionForRecognition(w, r)
	if !ok {
		return
	}

	var req sessionReactionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "invalid JSON body")
		return
	}
	if !allowedReactions[req.Emoji] {
		badRequest(w, "emoji must be one this install offers")
		return
	}

	ctx := r.Context()
	caller := userFrom(ctx).ID
	// Applauding yourself is not a thing to record. Comments are deliberately
	// different — a comment is a contribution, and you may need to answer somebody
	// on your own workout — which is why this check is here and not in a helper
	// both endpoints share.
	if session.UserID != nil && *session.UserID == caller {
		forbidden(w, "own_session", "you cannot react to your own session")
		return
	}

	// The applause and the telling of it go in one transaction. They are two
	// statements and they must not be separable: applause nobody was told about
	// is exactly the state 0026 exists to abolish, and it would be invisible —
	// the lifter's tap worked, the count went up, and the only sign of the
	// failure would be a notification that never came.
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		internalError(w)
		return
	}
	defer func() { _ = tx.Rollback(ctx) }()
	qtx := s.q.WithTx(tx)

	added, err := qtx.AddSessionReaction(ctx, store.AddSessionReactionParams{
		SessionID: session.ID,
		UserID:    caller,
		Emoji:     req.Emoji,
	})
	if err != nil {
		internalError(w)
		return
	}

	// Only a tap that actually recorded something tells anybody. A repeat tap is
	// a no-op on the applause — the primary key settles that, which is why the
	// query is :execrows — and it has to be a no-op on the notification too, or
	// a lifter leaning on a button would fill somebody's panel with one row over
	// and over.
	if added > 0 {
		if err := qtx.CreateReactionNotification(ctx, store.CreateReactionNotificationParams{
			ActorID:   caller,
			SessionID: session.ID,
			Emoji:     req.Emoji,
		}); err != nil {
			internalError(w)
			return
		}
	}

	if err := tx.Commit(ctx); err != nil {
		internalError(w)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// removeSessionReaction withdraws it.
//
// 204 whether or not a row went. The caller asked for a state — "I am not
// applauding this" — and that state holds either way; reporting 404 for a
// reaction already gone would turn a double-tap into an error.
func (s *Server) removeSessionReaction(w http.ResponseWriter, r *http.Request) {
	session, ok := s.sessionForRecognition(w, r)
	if !ok {
		return
	}

	// A query parameter rather than a body: a DELETE carrying one is awkward for
	// clients and for anything in between.
	emoji := r.URL.Query().Get("emoji")
	if !allowedReactions[emoji] {
		badRequest(w, "emoji must be one this install offers")
		return
	}

	ctx := r.Context()
	caller := userFrom(ctx).ID

	// Withdrawing takes the notification with it, in one transaction for the
	// same reason giving it does.
	//
	// This is the asymmetry 0026 records: a deleted comment's notification goes
	// by cascade, because a notification references a comment by id. A reaction
	// has no id to reference — 0024 gave session_reactions a composite primary
	// key and no surrogate — so there is nothing to cascade from and the row has
	// to be deleted by hand, matched on the three columns that identify it.
	//
	// Unconditional about what it finds, like the withdrawal itself: taking back
	// applause that was never given removes nothing and reports nothing.
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		internalError(w)
		return
	}
	defer func() { _ = tx.Rollback(ctx) }()
	qtx := s.q.WithTx(tx)

	if _, err := qtx.RemoveSessionReaction(ctx, store.RemoveSessionReactionParams{
		SessionID: session.ID,
		UserID:    caller,
		Emoji:     emoji,
	}); err != nil {
		internalError(w)
		return
	}
	if err := qtx.DeleteReactionNotification(ctx, store.DeleteReactionNotificationParams{
		ActorID:   caller,
		SessionID: session.ID,
		Emoji:     emoji,
	}); err != nil {
		internalError(w)
		return
	}

	if err := tx.Commit(ctx); err != nil {
		internalError(w)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- comments ----

type sessionCommentRequest struct {
	Body string `json:"body"`
}

// commentRateKey namespaces the limiter's map by what the key MEANS.
//
// The login limiter keys on "address|username" and this one keys on an account
// id. They are separate limiters today, so nothing can collide — the prefix is
// here so that stays true if anybody ever merges them, since "17" is a
// perfectly plausible username.
func commentRateKey(userID int32) string {
	return "comment|" + strconv.FormatInt(int64(userID), 10)
}

// listSessionComments serves a page of a session's conversation, oldest first.
//
// PAGED FROM THE NEWEST END AND RENDERED FROM THE OLDEST, which is the one
// surprising thing about this endpoint: offset counts back from the most recent
// comment, so offset 0 is the tail of the thread. That is the part somebody
// opening a session wants, and it is the part a notification points at. Paging
// the other way would open a forty-comment session at a conversation whose last
// line — the one they were told about — is twenty rows below the fold.
//
// Total covers the whole thread so a surface can say how much is above the page
// it is drawing. That is why this list has one where the feed does not: "17
// earlier comments" is a sentence worth writing, and a short page cannot count
// what it did not return.
func (s *Server) listSessionComments(w http.ResponseWriter, r *http.Request) {
	session, ok := s.sessionForRecognition(w, r)
	if !ok {
		return
	}
	limit, offset, ok := pageParams(w, r)
	if !ok {
		return
	}

	ctx := r.Context()
	rows, err := s.q.ListSessionComments(ctx, store.ListSessionCommentsParams{
		SessionID: session.ID,
		Lim:       limit,
		Off:       offset,
	})
	if err != nil {
		internalError(w)
		return
	}
	total, err := s.q.CountSessionComments(ctx, session.ID)
	if err != nil {
		internalError(w)
		return
	}

	out := make([]sessionCommentDTO, 0, len(rows))
	for _, row := range rows {
		out = append(out, sessionCommentDTO{
			ID:        row.ID,
			SessionID: row.SessionID,
			Author: lifterDTO{
				ID:          row.UserID,
				Username:    row.Username,
				DisplayName: row.DisplayName,
				AvatarColor: row.AvatarColor,
				HasAvatar:   row.AvatarEtag != "",
				AvatarEtag:  row.AvatarEtag,
			},
			Body:      row.Body,
			CreatedAt: timestamptzToString(row.CreatedAt),
		})
	}
	writeJSON(w, http.StatusOK, sessionCommentListDTO{
		Items: out, Total: total, Limit: limit, Offset: offset,
	})
}

// addSessionComment posts one.
//
// Allowed on the caller's own session, unlike a reaction: answering somebody who
// commented on your workout is the obvious thing to want, and refusing it would
// make the conversation one-sided on exactly the sessions most likely to have one.
func (s *Server) addSessionComment(w http.ResponseWriter, r *http.Request) {
	session, ok := s.sessionForRecognition(w, r)
	if !ok {
		return
	}

	var req sessionCommentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "invalid JSON body")
		return
	}

	// Trimmed before it is measured, so a comment of spaces is blank rather than
	// a valid one-character body.
	body := strings.TrimSpace(req.Body)
	if body == "" {
		badRequest(w, "comment cannot be blank")
		return
	}
	if utf8.RuneCountInString(body) > maxCommentBody {
		badRequest(w, "comment must be at most 256 characters")
		return
	}

	ctx := r.Context()
	caller := userFrom(ctx)

	// Checked after the body is validated, so a blank comment is told it is
	// blank rather than silently costing a post. Keyed on the account and not on
	// the address: this bounds what one lifter can do, and unlike login there is
	// no anonymous caller here to rotate anything.
	//
	// Consume rather than Allow — see the note on the method. An accepted
	// comment is exactly what this counts.
	if !s.comments.Consume(commentRateKey(caller.ID)) {
		writeError(w, http.StatusTooManyRequests, "too_many_comments",
			"you're posting faster than this install allows; try again in a few minutes")
		return
	}

	// One transaction, as the reaction path does it. A comment that reached
	// nobody is worse here than there: applause is ambient, but a comment is
	// addressed to somebody and may be a question waiting on an answer.
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		internalError(w)
		return
	}
	defer func() { _ = tx.Rollback(ctx) }()
	qtx := s.q.WithTx(tx)

	row, err := qtx.AddSessionComment(ctx, store.AddSessionCommentParams{
		SessionID: session.ID,
		UserID:    caller.ID,
		Body:      body,
	})
	if err != nil {
		internalError(w)
		return
	}

	// Who hears about this is decided entirely in SQL — the session's owner, and
	// everybody else already talking on it, minus the author. See
	// CreateCommentNotifications for why that rule lives there and not here: the
	// generated-activity scheduler has to apply the same one, and it is not
	// going through this handler to do it.
	//
	// Runs after the insert on purpose. The query reads the session's other
	// comments to find the thread, and the comment just posted is among them by
	// then — excluded by id, and its author excluded again by actor.
	if err := qtx.CreateCommentNotifications(ctx, store.CreateCommentNotificationsParams{
		ActorID:   caller.ID,
		SessionID: session.ID,
		CommentID: row.ID,
	}); err != nil {
		internalError(w)
		return
	}

	if err := tx.Commit(ctx); err != nil {
		internalError(w)
		return
	}

	// The author is the caller, so it is assembled from the session rather than
	// re-read. The avatar tag is the one thing that is not on currentUser, and a
	// query for it here would be a round trip to decorate a row the client is
	// about to refetch anyway if it cares.
	author := lifterDTO{
		ID:          caller.ID,
		Username:    caller.Username,
		DisplayName: caller.DisplayName,
		AvatarColor: caller.AvatarColor,
	}
	if etag, err := s.q.GetUserAvatarEtag(ctx, caller.ID); err == nil {
		author.HasAvatar = true
		author.AvatarEtag = etag
	}

	writeJSON(w, http.StatusCreated, sessionCommentDTO{
		ID:        row.ID,
		SessionID: row.SessionID,
		Author:    author,
		Body:      row.Body,
		CreatedAt: timestamptzToString(row.CreatedAt),
	})
}

// deleteSessionComment removes one.
//
// The author may remove their own; the install's owner may remove any. That is the
// only moderation here, and on a box shared by a household it is the only kind
// that means anything — there is nobody to appeal to and nobody to report to.
func (s *Server) deleteSessionComment(w http.ResponseWriter, r *http.Request) {
	session, ok := s.sessionForRecognition(w, r)
	if !ok {
		return
	}
	commentID, ok := idParam(r, "commentId")
	if !ok {
		notFound(w, "comment not found")
		return
	}

	ctx := r.Context()
	row, err := s.q.GetSessionComment(ctx, commentID)
	if errors.Is(err, pgx.ErrNoRows) {
		notFound(w, "comment not found")
		return
	}
	if err != nil {
		internalError(w)
		return
	}
	// The comment must belong to the session the URL names. Without this a comment
	// could be deleted through any session's URL and the path would be decorative
	// — and a 404 rather than a 400, because as far as this URL is concerned the
	// resource it names does not exist.
	if row.SessionID != session.ID {
		notFound(w, "comment not found")
		return
	}

	caller := userFrom(ctx)
	if row.UserID != caller.ID && !caller.IsAdmin {
		forbidden(w, "not_your_comment", "only the author or the install's owner can remove this")
		return
	}

	if _, err := s.q.DeleteSessionComment(ctx, commentID); err != nil {
		internalError(w)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
