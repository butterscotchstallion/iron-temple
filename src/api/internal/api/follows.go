package api

import (
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5"

	"gitea.homelab/gitadmin/iron-temple/api/internal/store"
)

// Following, which decides whose achievements reach the caller's panel.
//
// # WHY THESE LIVE UNDER /me AND NOT UNDER /lifters
//
// The obvious shape is POST /lifters/{lifterId}/follow, and lifters.go forbids it
// in as many words: "nothing under /lifters may ever be made to write — a
// path-supplied user id reaching an INSERT is how one lifter edits another's
// history". A follow is not that bug — the row it writes is keyed on the caller,
// not on the id in the path — but the rule is worth more as an absolute than as one
// with an exception a future handler can reason its way into. lifters.go names the
// alternative too: a social write "belongs on the resource being written... where
// the caller is the authenticated user".
//
// So the resource is the caller's own following list. /me/following/{lifterId} is
// the caller's list with a lifter in it, which is what the write actually is, and
// the /lifters subtree keeps having no write handler to forget an ownership check
// in. It is the same justification /notifications gives for being top-level: the
// subject is a person, and it is always the one holding the session cookie.
//
// # WHAT A FOLLOW DOES AND DOES NOT DO
//
// It decides delivery, not access. Following somebody adds their achievements to
// your panel; it grants nothing, because there was nothing to grant — their
// profile, sessions, boards and crowns were always readable. See 0033 and the
// amended note in lifters.go.

// noViewer is the viewer id to pass when a query's follow check has no reader to
// answer for.
//
// Only computeBoards uses it: it runs both from a request and from the hourly
// reconciler, and the second has no authenticated user at all. Zero matches no
// account, so is_following comes back uniformly false — which is the right answer
// for a ranking, since a leaderboard draws no Follow button. Named rather than
// written as a bare 0 so the call site reads as a decision instead of a parameter
// somebody forgot to fill in.
const noViewer int32 = 0

// followTarget resolves the {lifterId} path parameter to an account the caller may
// follow, writing the refusal itself. ok=false means the caller should stop.
//
// Three refusals, and the order matters. A malformed or unknown id is a 404, which
// is what /lifters already answers for one. Following yourself is a 403 rather than
// a silent no-op, mirroring self-applause — "you cannot react to your own session"
// — because it is a button the client should not have offered and saying so is more
// use than pretending it worked. 0033's CHECK is the rail behind this, not the
// thing reporting it.
func (s *Server) followTarget(w http.ResponseWriter, r *http.Request) (int32, bool) {
	id, ok := idParam(r, "lifterId")
	if !ok {
		notFound(w, "lifter not found")
		return 0, false
	}

	caller := userFrom(r.Context()).ID
	if id == caller {
		forbidden(w, "own_account", "you cannot follow yourself")
		return 0, false
	}

	// Existence checked through GetLifter for lifterFromPath's reason: without it,
	// following a nonexistent id would be a well-formed insert against a foreign
	// key that happens to fail, reported as a 500.
	if _, err := s.q.GetLifter(r.Context(), store.GetLifterParams{
		ID:       id,
		ViewerID: caller,
	}); errors.Is(err, pgx.ErrNoRows) {
		notFound(w, "lifter not found")
		return 0, false
	} else if err != nil {
		internalError(w)
		return 0, false
	}

	return id, true
}

// followLifter starts hearing about somebody's achievements.
//
// 204 and idempotent. The primary key makes a second press a no-op — see
// FollowLifter — and a lifter pressing a button twice is not a mistake to report.
//
// No live event and no notification. Nobody's panel changes: the follower's next
// poll will simply start including the followee's crowns, and telling the followee
// they have a new follower is a feature nobody asked for on a box where everybody
// already knows each other.
func (s *Server) followLifter(w http.ResponseWriter, r *http.Request) {
	id, ok := s.followTarget(w, r)
	if !ok {
		return
	}

	if _, err := s.q.FollowLifter(r.Context(), store.FollowLifterParams{
		FolloweeID: id,
		FollowerID: userFrom(r.Context()).ID,
	}); err != nil {
		internalError(w)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// unfollowLifter stops.
//
// 204 whether or not a row went, for removeSessionReaction's reason: the caller
// asked for a state — "I do not follow this lifter" — and that state holds either
// way. Reporting 404 for a follow already gone would turn a double-tap into an
// error.
//
// Note what this does NOT do: it leaves behind every crown notification the follow
// already delivered. Those are things that happened, and unfollowing is not a claim
// that they did not — the same reasoning 0026 gives for "clear all" being the only
// thing that removes a row.
func (s *Server) unfollowLifter(w http.ResponseWriter, r *http.Request) {
	id, ok := s.followTarget(w, r)
	if !ok {
		return
	}

	if _, err := s.q.UnfollowLifter(r.Context(), store.UnfollowLifterParams{
		FolloweeID: id,
		FollowerID: userFrom(r.Context()).ID,
	}); err != nil {
		internalError(w)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
