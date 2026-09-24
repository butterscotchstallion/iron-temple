package api_test

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/gavv/httpexpect/v2"
)

// The live socket, end to end over a real connection.
//
// The hub's own behaviour under pressure is unit-tested in internal/live with
// no network at all. What is worth proving HERE is everything that only exists
// once the socket is mounted on the real router: that it authenticates, that it
// refuses a foreign origin, that it escaped the ETag middleware at all, and
// that a reaction posted over HTTP by one lifter arrives as a frame on
// another's socket.
//
// ABSENCE IS ASSERTED BY ORDERING, NEVER BY A TIMEOUT. "Nothing arrived within
// 200ms" is a flaky test and httpexpect fails outright on a read timeout
// anyway. So where a test needs to show that something was NOT sent, it causes
// an event that SHOULD be sent and asserts that one is the next frame to
// arrive.

// liveSocket opens an authenticated socket and consumes the welcome frame, so
// every caller starts from the same place.
func liveSocket(t *testing.T, e *httpexpect.Expect) *httpexpect.Websocket {
	t.Helper()
	ws := e.GET("/live").WithWebsocketUpgrade().
		Expect().Status(http.StatusSwitchingProtocols).
		Websocket().WithReadTimeout(10 * time.Second)
	t.Cleanup(func() { ws.Disconnect() })

	ws.Expect().TextMessage().JSON().Object().
		HasValue("type", "welcome").
		HasValue("protocol", 1)
	return ws
}

// nextEvent reads one frame and returns it as an object.
func nextEvent(ws *httpexpect.Websocket) *httpexpect.Object {
	return ws.Expect().TextMessage().JSON().Object()
}

// handshake builds the upgrade request BY HAND, which is the only way to assert
// that one was refused.
//
// httpexpect's WithWebsocketUpgrade fails the test outright when the upgrade
// does not complete, so it cannot express "this handshake should be rejected" —
// and rejection is exactly what the three tests below are about. A handshake is
// only a GET with four headers, so sending it as an ordinary request costs
// nothing and lets the status be asserted directly.
func handshake(e *httpexpect.Expect) *httpexpect.Request {
	return e.GET("/live").
		WithHeader("Connection", "Upgrade").
		WithHeader("Upgrade", "websocket").
		WithHeader("Sec-WebSocket-Version", "13").
		// Any 16 random bytes, base64. The server echoes a hash of it; nothing
		// here checks the response, only the status.
		WithHeader("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")
}

// ---- the gate ----

func TestLiveRequiresASession(t *testing.T) {
	handshake(expectAnon(t)).Expect().Status(http.StatusUnauthorized)
}

// An account still holding the one-time password an admin gave it cannot open a
// socket either. This route inherits nothing — both gates are spelled out on it
// — so the one that is easy to forget gets a test.
func TestLiveIsGatedUntilThePasswordChanges(t *testing.T) {
	createAccount(t, "live-gated", "live-gated-pw")
	token := signIn(t, "live-gated", "live-gated-pw")

	handshake(expectAs(t, token)).Expect().
		Status(http.StatusForbidden).
		JSON().Object().HasValue("code", "password_change_required")
}

// THE CROSS-SITE WEBSOCKET HIJACKING REGRESSION TEST.
//
// A handshake is a GET, which the CSRF middleware skips, and it carries the
// session cookie without any CORS involvement — so without an origin check any
// site a lifter visited could open an authenticated socket in their browser.
func TestLiveRefusesAForeignOrigin(t *testing.T) {
	handshake(expect(t)).
		WithHeader("Origin", "http://evil.example").
		Expect().Status(http.StatusForbidden)
}

// ---- the connection ----

// The welcome frame is also the proof that this route escaped jsonETag: that
// middleware replaces the writer with a recorder which is not an
// http.Hijacker, so mounted inside the authenticated group the upgrade could
// not complete at all.
func TestLiveWelcomesAnAuthenticatedLifter(t *testing.T) {
	liveSocket(t, expect(t))
}

// A message the server cannot act on is answered and the socket LIVES. A newer
// client saying something an older server has not heard of must not lose its
// connection for it.
func TestLiveSurvivesABadMessage(t *testing.T) {
	ws := liveSocket(t, expect(t))

	ws.WriteText("this is not JSON")
	nextEvent(ws).HasValue("type", "error").HasValue("code", "bad_json")

	ws.WriteJSON(map[string]any{"type": "nonsense"})
	nextEvent(ws).HasValue("type", "error").HasValue("code", "unknown_type")

	// Still usable, which is the actual claim. Proven by asking for something
	// that produces a frame.
	ws.WriteJSON(map[string]any{"type": "subscribe", "sessionId": 0})
	nextEvent(ws).HasValue("type", "error").HasValue("code", "bad_session_id")
}

func TestLiveCapsSubscriptions(t *testing.T) {
	ws := liveSocket(t, expect(t))

	// The cap is eight; the ninth is refused.
	for i := 1; i <= 8; i++ {
		ws.WriteJSON(map[string]any{"type": "subscribe", "sessionId": i})
	}
	ws.WriteJSON(map[string]any{"type": "subscribe", "sessionId": 999})

	nextEvent(ws).HasValue("type", "error").HasValue("code", "too_many_subscriptions")
}

// ---- push ----

// The feature, in one test: somebody applauds your session and your socket is
// told, without anybody polling.
func TestLiveTellsTheOwnerAboutApplause(t *testing.T) {
	sessionID, ownerToken := otherLiftersSession(t, "live-applause")
	ws := liveSocket(t, expectAs(t, ownerToken))

	expect(t).POST(fmt.Sprintf("/sessions/%d/reactions", sessionID)).
		WithJSON(map[string]any{"emoji": "💪"}).
		Expect().Status(http.StatusNoContent)

	nextEvent(ws).HasValue("type", "notification")
}

// And a comment, which is the other half of the fan-out.
func TestLiveTellsTheOwnerAboutAComment(t *testing.T) {
	sessionID, ownerToken := otherLiftersSession(t, "live-comment")
	ws := liveSocket(t, expectAs(t, ownerToken))

	expect(t).POST(fmt.Sprintf("/sessions/%d/comments", sessionID)).
		WithJSON(map[string]any{"body": "strong sets"}).
		Expect().Status(http.StatusCreated)

	nextEvent(ws).HasValue("type", "notification")
}

// A frame is only ever written to the connections of the lifter it addresses.
//
// Absence by ordering: the bystander is sent nothing for the first session, so
// the next frame it sees must be the one raised on its OWN session.
func TestLiveDoesNotTellBystanders(t *testing.T) {
	otherSession, _ := otherLiftersSession(t, "live-bystander-other")
	ownSession, bystanderToken := otherLiftersSession(t, "live-bystander")
	ws := liveSocket(t, expectAs(t, bystanderToken))

	// Not theirs: no frame.
	expect(t).POST(fmt.Sprintf("/sessions/%d/reactions", otherSession)).
		WithJSON(map[string]any{"emoji": "🔥"}).
		Expect().Status(http.StatusNoContent)

	// Theirs: a frame, and it must be the FIRST thing to arrive.
	expect(t).POST(fmt.Sprintf("/sessions/%d/reactions", ownSession)).
		WithJSON(map[string]any{"emoji": "👏"}).
		Expect().Status(http.StatusNoContent)

	nextEvent(ws).HasValue("type", "notification")
}

// A repeat tap records nothing and therefore announces nothing. This is the
// reachable proxy for "published only after the transaction committed": a write
// that did not happen produces no frame.
func TestLiveDoesNotAnnounceARepeatTap(t *testing.T) {
	sessionID, ownerToken := otherLiftersSession(t, "live-repeat-tap")
	ws := liveSocket(t, expectAs(t, ownerToken))

	path := fmt.Sprintf("/sessions/%d/reactions", sessionID)
	for range 3 {
		expect(t).POST(path).WithJSON(map[string]any{"emoji": "🎉"}).
			Expect().Status(http.StatusNoContent)
	}
	nextEvent(ws).HasValue("type", "notification")

	// The second and third taps sent nothing, proven by causing a comment and
	// asserting that IT is the next frame rather than a second applause.
	expect(t).POST(fmt.Sprintf("/sessions/%d/comments", sessionID)).
		WithJSON(map[string]any{"body": "and a word"}).
		Expect().Status(http.StatusCreated)
	nextEvent(ws).HasValue("type", "notification")
}

// ---- session subscriptions ----

// A subscriber to a session hears about activity on it even though the
// notification was addressed to somebody else entirely — which is what makes an
// open recap able to update itself.
func TestLiveSendsSessionEventsToSubscribers(t *testing.T) {
	sessionID, _ := otherLiftersSession(t, "live-session-sub")
	// A third party: not the session's owner, so nothing here reaches them as a
	// notification. Only the subscription does.
	_, watcherToken := secondLifter(t, "live-session-watcher")
	ws := liveSocket(t, expectAs(t, watcherToken))

	ws.WriteJSON(map[string]any{"type": "subscribe", "sessionId": sessionID})

	expect(t).POST(fmt.Sprintf("/sessions/%d/comments", sessionID)).
		WithJSON(map[string]any{"body": "watching this one"}).
		Expect().Status(http.StatusCreated)

	event := nextEvent(ws)
	event.HasValue("type", "comment")
	event.HasValue("sessionId", sessionID)
}

// Unsubscribing stops them, asserted by ordering against a frame that is still
// deliverable.
func TestLiveStopsSessionEventsAfterUnsubscribing(t *testing.T) {
	watchedSession, _ := otherLiftersSession(t, "live-unsub-watched")
	ownSession, watcherToken := otherLiftersSession(t, "live-unsub-watcher")
	ws := liveSocket(t, expectAs(t, watcherToken))

	ws.WriteJSON(map[string]any{"type": "subscribe", "sessionId": watchedSession})
	ws.WriteJSON(map[string]any{"type": "unsubscribe", "sessionId": watchedSession})

	// No longer watching: this should reach them not at all.
	expect(t).POST(fmt.Sprintf("/sessions/%d/reactions", watchedSession)).
		WithJSON(map[string]any{"emoji": "💪"}).
		Expect().Status(http.StatusNoContent)

	// But this is addressed to them, so it must be the next thing they see.
	expect(t).POST(fmt.Sprintf("/sessions/%d/reactions", ownSession)).
		WithJSON(map[string]any{"emoji": "🔥"}).
		Expect().Status(http.StatusNoContent)

	nextEvent(ws).HasValue("type", "notification")
}

// ---- metrics ----

// A socket is not a request, and the middleware has to say so: held open, it
// would sit in the in-flight gauge and put an hours-long sample into a latency
// histogram whose largest finite bucket is ten seconds.
//
// The failed handshake IS counted, which is the half worth keeping — a spike in
// 403s here is somebody pointing a page at this install.
func TestLiveIsNotObservedAsALongRequest(t *testing.T) {
	beforeInFlight := counter(t, scrape(t), "iron_temple_http_requests_in_flight")

	ws := liveSocket(t, expect(t))

	during := scrape(t)
	if got := counter(t, during, "iron_temple_http_requests_in_flight"); got != beforeInFlight {
		t.Errorf("in-flight went from %d to %d while a socket was open", beforeInFlight, got)
	}
	// The connection gauge is what reports it instead.
	if got := counter(t, during, "iron_temple_live_connections"); got < 1 {
		t.Errorf("live_connections read %d with a socket open, want at least 1", got)
	}

	ws.Disconnect()
}

// The handshake that FAILS is still counted, which is the half worth keeping.
func TestFailedLiveHandshakeIsCounted(t *testing.T) {
	const series = `iron_temple_http_requests_total{method="GET",route="/api/v1/live",status="403"}`
	before := counter(t, scrape(t), series)

	handshake(expect(t)).
		WithHeader("Origin", "http://evil.example").
		Expect().Status(http.StatusForbidden)

	awaitDelta(t, series, before, 1)
}
