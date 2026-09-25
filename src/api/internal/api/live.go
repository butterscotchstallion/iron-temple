package api

import (
	"context"
	"net/http"

	"gitea.homelab/gitadmin/iron-temple/api/internal/live"
)

// The live socket: the API's half of internal/live.
//
// # WHY THIS ROUTE SITS OUTSIDE THE AUTHENTICATED GROUP
//
// Not because it is public — it is not, and requireUser is spelled out on it —
// but because jsonETag cannot be applied to it. That middleware replaces the
// ResponseWriter with a recorder that buffers the body so it can be hashed, and
// the recorder implements Write and WriteHeader and nothing else. A WebSocket
// upgrade needs the raw connection, gorilla type-asserts for http.Hijacker, and
// the assertion fails — so mounted inside the group this endpoint could not
// work at all, and the 400 explaining why would itself be swallowed by the
// buffer.
//
// The export route escapes the same group for a related reason, and the pattern
// is the same: leave the group, and repeat the middleware you still want rather
// than inheriting middleware you cannot have.
//
// # WHAT THE SOCKET IS ALLOWED TO DO
//
// Read nothing and write nothing. A client may subscribe to a session id and
// that is the entire vocabulary — see internal/live. Every event it provokes is
// a signal to refetch over HTTP, where the ordinary rules apply. This endpoint
// therefore adds no new way to reach data, which is what keeps its security
// story down to one question: whose socket is this, answered once by
// requireUser at the handshake.

// serveLive upgrades an authenticated request and blocks for the connection's
// life.
func (s *Server) serveLive(w http.ResponseWriter, r *http.Request) {
	s.live.Serve(w, r, userFrom(r.Context()).ID)
}

// CloseLiveSockets ends every live connection with a going-away frame.
//
// Exported because cmd/server calls it, and it must be called BEFORE
// http.Server.Shutdown: Shutdown does not close or wait on hijacked
// connections, so without this every socket is severed as the process exits.
func (s *Server) CloseLiveSockets(ctx context.Context) {
	s.live.Close(ctx)
}

// liveEvents collects what to push, so it can be pushed AFTER the transaction
// that produced it has committed and not one instant before.
//
// This type exists for one bug. The fan-out queries run inside a transaction;
// publishing from inside it would announce a notification that a rollback then
// un-wrote, and the client would refetch an empty panel while drawing a badge
// for something that never happened. The rule is therefore: fill this inside
// the transaction, call publish() on the line after a successful Commit, and
// NEVER defer it — a defer fires on the rollback path too, which is exactly the
// case this is guarding.
//
// Go computes no recipients here. The queries decide who hears about something
// and now also return whom they decided (see notifications.sql); this only
// carries the answer across the commit boundary.
type liveEvents struct {
	hub *live.Hub
	// users is a set, so one transaction produces at most one notification
	// event per recipient however many rows it wrote.
	//
	// THE DEDUPE IS LOAD-BEARING, not an optimisation. The generated-activity
	// scheduler writes a day's worth of applause in a single transaction, and
	// without this that would be a burst of frames per connection — which the
	// hub answers by EVICTING a client whose buffer fills. Every recipient
	// refetches the same panel either way, so one event says everything the
	// burst would have.
	users map[int32]struct{}
	// sessions are ordered because a reaction and a comment on the same session
	// are different frames and both are worth sending.
	sessions []liveSessionEvent
	// levels is a bool rather than a count, for the reason users is a set: the
	// frame says only "the levels moved", so a transaction that moves three
	// lifters' levels still has exactly one thing to say. Anything else would put
	// N identical frames on every open connection, which is the burst the
	// eviction policy is there to survive and not one worth creating.
	levels bool
}

type liveSessionEvent struct {
	kind      live.Kind
	sessionID int32
}

func (s *Server) newLiveEvents() *liveEvents {
	return &liveEvents{hub: s.live, users: make(map[int32]struct{})}
}

// notify records that these lifters' panels changed.
func (e *liveEvents) notify(userIDs []int32) {
	for _, id := range userIDs {
		e.users[id] = struct{}{}
	}
}

// session records that something happened on a session somebody may be reading.
func (e *liveEvents) session(kind live.Kind, sessionID int32) {
	e.sessions = append(e.sessions, liveSessionEvent{kind: kind, sessionID: sessionID})
}

// levels records that somebody's level moved, for everybody.
//
// No recipients, because this is the one frame that has none — see
// live.Hub.Broadcast for why a level is unaddressed where everything else here is
// addressed by a query that returned its own audience.
func (e *liveEvents) level() {
	e.levels = true
}

// publish sends everything collected, then empties itself so a second call is a
// no-op.
//
// Call it on the line AFTER tx.Commit returns nil. Not in a defer.
func (e *liveEvents) publish() {
	if e.hub == nil {
		return
	}
	if len(e.users) > 0 {
		ids := make([]int32, 0, len(e.users))
		for id := range e.users {
			ids = append(ids, id)
		}
		e.hub.Notify(ids)
		e.users = make(map[int32]struct{})
	}
	for _, se := range e.sessions {
		e.hub.Session(se.kind, se.sessionID)
	}
	e.sessions = nil
	if e.levels {
		e.hub.Broadcast(live.KindLevel)
		e.levels = false
	}
}

// liveOrigin is the handshake's only gate beyond authentication.
//
// CORS_ORIGIN is deliberately NOT consulted. It defaults to "*", which as a
// CheckOrigin would mean "anybody" — and the "*" is only safe in the CORS block
// because AllowCredentials is off there. A WebSocket has no such switch: the
// cookie simply rides. The UI is same-origin in development (the Vite proxy,
// which is why changeOrigin must stay off) and in production (Traefik
// path-routes /api and preserves Host), so no legitimate cross-origin socket
// exists. If one ever does, this is the seam.
func liveOrigin(r *http.Request) bool {
	return sameOriginRequest(r)
}
