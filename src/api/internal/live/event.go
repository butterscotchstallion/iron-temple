// Package live is the push half of the social layer: one authenticated
// WebSocket per signed-in lifter, told when something happened that concerns
// them.
//
// # EVERY FRAME IS A SIGNAL, NEVER CONTENT
//
// This is the decision the whole package rests on, and everything else follows
// from it. A frame says "something arrived for you" or "something happened on
// session 412". It never says what. The client answers by refetching through
// the ordinary HTTP endpoints, which already do the authorization and are
// already ETagged — so a refetch after a reconnect is usually a pair of 304s.
//
// Three things fall out of that, and each of them is a problem this package
// then does not have:
//
//   - Nothing personal is on this wire, so a frame delivered to the wrong
//     connection would leak nothing. The origin check exists for integrity, not
//     for confidentiality.
//   - The publish path reads no database. The deployment has one small pool
//     shared with the request traffic, and a fan-out that queried per recipient
//     would be the leaderboard's mistake in a hotter loop.
//   - Dropping a slow client is SAFE, because its reconnect refetches
//     everything. That is what makes the backpressure policy in conn.go
//     defensible; see the long note there.
//
// # WHAT THIS IS NOT
//
// It is not a delivery guarantee. A lifter with no socket open misses every
// frame and loses nothing, because the client keeps polling as a backstop and
// refetches on connect. Nothing here is the source of truth for anything — the
// database is, and this only ever says "go and look".
//
// The protocol is documented for humans in docs/live-socket.md. OpenAPI 3.0
// cannot describe a WebSocket, so that file is the contract rather than
// openapi.yaml, and the types below and their TypeScript counterparts in
// src/ui/src/lib/live.svelte.ts are the two ends that have to agree with it.
package live

// Kind is what a frame means. Server→client.
type Kind string

const (
	// KindWelcome is the first frame on every connection, and the one the
	// client waits for before declaring itself connected. A socket whose
	// onopen fired but whose server side is wedged never sends one.
	KindWelcome Kind = "welcome"
	// KindNotification says the caller's notification panel has changed. It
	// carries no id: the client refetches the panel, which is one request
	// whether one thing happened or five.
	KindNotification Kind = "notification"
	// KindReaction and KindComment say something happened on a session the
	// client has subscribed to. Only the session id travels.
	KindReaction Kind = "reaction"
	KindComment  Kind = "comment"
	// KindLevel says somebody's level moved. THE ONE KIND THAT IS NOT ADDRESSED
	// — it goes to every connection, because a level is drawn beside a name
	// wherever a name appears and the set of clients now showing a stale badge is
	// every client. It carries no lifter id, which is what makes sending it to
	// everybody say nothing: a client learns only that the levels are worth
	// asking for again. See Hub.Broadcast.
	KindLevel Kind = "level"
	// KindError answers a message the server could not act on. It never closes
	// the connection — see conn.go.
	KindError Kind = "error"
)

// Protocol is bumped when a frame's MEANING changes.
//
// Adding a kind or an optional field does not bump it: both ends ignore what
// they do not recognise, which is what lets a deploy roll out without every
// open socket having to be the same version as the server. It exists so that a
// change which cannot be absorbed that way has a way to announce itself.
const Protocol = 1

// Error codes carried by KindError. Small and closed on purpose: a client
// should be able to branch on these, and an error a client cannot branch on may
// as well be a log line.
const (
	CodeBadJSON      = "bad_json"
	CodeUnknownType  = "unknown_type"
	CodeBadSessionID = "bad_session_id"
	CodeTooManySubs  = "too_many_subscriptions"
)

// Event is one server→client frame.
//
// One struct with omitempty rather than a union per kind. Five kinds and three
// optional fields do not earn a custom unmarshaller on the client side, and one
// struct means the wire shape is a single screen that can be read against
// docs/live-socket.md.
//
// There is deliberately no actor, no body and no id. See the package comment:
// a frame is a signal to refetch.
type Event struct {
	Type     Kind `json:"type"`
	Protocol int  `json:"protocol,omitempty"`
	// SessionID is set on the two session kinds and nothing else.
	SessionID int32 `json:"sessionId,omitempty"`
	// Code and Message are set on KindError.
	Code    string `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
}

// clientMessage is one client→server frame: subscribe or unsubscribe, naming a
// session.
//
// The set is this small because the client has nothing else to say. It cannot
// send a notification, it cannot read anything over this socket, and it cannot
// address another account — every one of those is an HTTP endpoint with its own
// rules, and none of them is reachable from here.
type clientMessage struct {
	Type      string `json:"type"`
	SessionID int32  `json:"sessionId"`
}

const (
	msgSubscribe   = "subscribe"
	msgUnsubscribe = "unsubscribe"
)

// notificationEvent, sessionEvent and levelEvent build the frames the API
// publishes. Constructors rather than literals at the call sites, so the shape
// of a frame is decided in this file and only in this file.
func notificationEvent() Event {
	return Event{Type: KindNotification}
}

// bareEvent is a frame that is nothing but its kind, which is what an unaddressed
// signal is: KindLevel says the levels moved and deliberately not whose. Takes the
// kind rather than being one function per kind so that Hub.Broadcast stays general,
// while the shape of the frame it sends is still decided here.
func bareEvent(kind Kind) Event {
	return Event{Type: kind}
}

func sessionEvent(kind Kind, sessionID int32) Event {
	return Event{Type: kind, SessionID: sessionID}
}

func errorEvent(code, message string) Event {
	return Event{Type: KindError, Code: code, Message: message}
}

func welcomeEvent() Event {
	return Event{Type: KindWelcome, Protocol: Protocol}
}
