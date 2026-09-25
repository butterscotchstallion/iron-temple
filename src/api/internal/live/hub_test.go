package live

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// The hub, tested without a network.
//
// This is the most concurrent code in the repository, so it is tested where the
// race detector can see all of it: no httptest server, no Docker, no Postgres,
// and therefore no reason for any of it to be skipped by `go test -short`. The
// `socket` interface is what makes that possible and is the whole justification
// for the indirection.
//
// What is worth asserting here is the behaviour under pressure: that a client
// falling behind is disconnected rather than silently starved, that every OTHER
// client is unaffected when one is, and that a frame is encoded once and shared
// immutably. The happy paths are covered end-to-end over a real socket in the
// API's integration suite.

// fakeSocket is a *websocket.Conn that never touches a network.
//
// reads is what the client "sends"; writes is what the server wrote. block
// holds the write side open so a test can fill a send buffer without the pump
// draining it.
type fakeSocket struct {
	mu     sync.Mutex
	writes [][]byte
	closes []int

	reads   chan []byte
	block   chan struct{}
	closed  bool
	pongSet bool
}

func newFakeSocket() *fakeSocket {
	return &fakeSocket{reads: make(chan []byte, 8)}
}

func (f *fakeSocket) ReadMessage() (int, []byte, error) {
	data, ok := <-f.reads
	if !ok {
		return 0, nil, websocket.ErrCloseSent
	}
	return websocket.TextMessage, data, nil
}

func (f *fakeSocket) WriteMessage(_ int, data []byte) error {
	// Held open on purpose by the backpressure test, which needs the pump to
	// stop draining so the buffer behind it can fill.
	if f.block != nil {
		<-f.block
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	// Copied, so an assertion about what went out cannot be fooled by the
	// caller reusing the slice — which is exactly what sharing one encoded
	// frame across recipients would look like if it were done unsafely.
	cp := make([]byte, len(data))
	copy(cp, data)
	f.writes = append(f.writes, cp)
	return nil
}

func (f *fakeSocket) WriteControl(messageType int, data []byte, _ time.Time) error {
	if messageType != websocket.CloseMessage {
		return nil
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(data) >= 2 {
		f.closes = append(f.closes, int(data[0])<<8|int(data[1]))
	}
	return nil
}

func (f *fakeSocket) SetReadLimit(int64)                {}
func (f *fakeSocket) SetReadDeadline(time.Time) error   { return nil }
func (f *fakeSocket) SetWriteDeadline(time.Time) error  { return nil }
func (f *fakeSocket) SetPongHandler(func(string) error) { f.pongSet = true }

func (f *fakeSocket) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.closed {
		f.closed = true
		close(f.reads)
	}
	return nil
}

func (f *fakeSocket) sent() [][]byte {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([][]byte, len(f.writes))
	copy(out, f.writes)
	return out
}

func (f *fakeSocket) closeCodes() []int {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]int, len(f.closes))
	copy(out, f.closes)
	return out
}

// join registers a connection on the hub and runs its pumps, as Serve would
// after a successful upgrade. Returns a teardown.
func join(t *testing.T, h *Hub, userID int32) (*fakeSocket, *conn, func()) {
	t.Helper()
	ws := newFakeSocket()
	c := newConn(h, ws, userID)

	h.mu.Lock()
	h.conns[c] = struct{}{}
	h.wg.Add(1)
	h.mu.Unlock()

	done := make(chan struct{})
	go func() {
		defer close(done)
		defer h.wg.Done()
		defer h.remove(c)
		c.run()
	}()

	return ws, c, func() {
		_ = ws.Close()
		<-done
	}
}

// waitFor polls until cond holds, failing rather than hanging. Every wait here
// is on a goroutine handing something to another goroutine, so there is no
// deterministic moment to read at.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func decode(t *testing.T, frame []byte) Event {
	t.Helper()
	var e Event
	if err := json.Unmarshal(frame, &e); err != nil {
		t.Fatalf("decode %q: %v", frame, err)
	}
	return e
}

// kinds reads what a socket has been sent, in order.
func kinds(t *testing.T, ws *fakeSocket) []Kind {
	t.Helper()
	out := []Kind{}
	for _, frame := range ws.sent() {
		out = append(out, decode(t, frame).Type)
	}
	return out
}

func TestWelcomeIsTheFirstFrame(t *testing.T) {
	h := New(Options{})
	ws, _, done := join(t, h, 1)
	defer done()

	waitFor(t, "the welcome", func() bool { return len(ws.sent()) > 0 })

	first := decode(t, ws.sent()[0])
	if first.Type != KindWelcome {
		t.Errorf("first frame was %q, want %q", first.Type, KindWelcome)
	}
	if first.Protocol != Protocol {
		t.Errorf("welcome carried protocol %d, want %d", first.Protocol, Protocol)
	}
}

// A notification reaches the account it names and nobody else. This is the
// whole access model of the push half: a frame is only ever written to the
// connections belonging to its addressee.
func TestNotifyReachesOnlyTheAddressee(t *testing.T) {
	h := New(Options{})
	mine, _, done1 := join(t, h, 1)
	theirs, _, done2 := join(t, h, 2)
	defer done1()
	defer done2()
	waitFor(t, "both welcomes", func() bool {
		return len(mine.sent()) > 0 && len(theirs.sent()) > 0
	})

	h.Notify([]int32{1})

	waitFor(t, "the notification", func() bool { return len(mine.sent()) == 2 })
	if got := kinds(t, mine)[1]; got != KindNotification {
		t.Errorf("addressee got %q, want %q", got, KindNotification)
	}
	// The bystander has its welcome and nothing else.
	if got := len(theirs.sent()); got != 1 {
		t.Errorf("bystander received %d frames, want only its welcome", got)
	}
}

// Two devices, one account. Both are told, which is what makes the badge agree
// across a phone and a laptop.
func TestNotifyReachesEveryConnectionOfOneLifter(t *testing.T) {
	h := New(Options{})
	phone, _, done1 := join(t, h, 7)
	laptop, _, done2 := join(t, h, 7)
	defer done1()
	defer done2()
	waitFor(t, "both welcomes", func() bool {
		return len(phone.sent()) > 0 && len(laptop.sent()) > 0
	})

	h.Notify([]int32{7})

	waitFor(t, "both notifications", func() bool {
		return len(phone.sent()) == 2 && len(laptop.sent()) == 2
	})
}

// Session events go only to connections that asked for that session.
func TestSessionEventsNeedASubscription(t *testing.T) {
	h := New(Options{})
	watching, c, done1 := join(t, h, 1)
	idle, _, done2 := join(t, h, 2)
	defer done1()
	defer done2()
	waitFor(t, "both welcomes", func() bool {
		return len(watching.sent()) > 0 && len(idle.sent()) > 0
	})

	c.handle([]byte(`{"type":"subscribe","sessionId":42}`))
	h.Session(KindComment, 42)

	waitFor(t, "the comment event", func() bool { return len(watching.sent()) == 2 })
	event := decode(t, watching.sent()[1])
	if event.Type != KindComment || event.SessionID != 42 {
		t.Errorf("got %+v, want a comment on session 42", event)
	}
	if got := len(idle.sent()); got != 1 {
		t.Errorf("a connection that subscribed to nothing received %d frames", got)
	}

	// And unsubscribing stops them.
	c.handle([]byte(`{"type":"unsubscribe","sessionId":42}`))
	h.Session(KindReaction, 42)
	// Asserted by ordering rather than by a timeout: cause something that IS
	// deliverable and check it is the next thing to arrive.
	h.Notify([]int32{1})
	waitFor(t, "the notification", func() bool { return len(watching.sent()) == 3 })
	if got := decode(t, watching.sent()[2]).Type; got != KindNotification {
		t.Errorf("frame after unsubscribing was %q, want %q", got, KindNotification)
	}
}

// A session event for a session nobody is watching reaches nobody, rather than
// being broadcast to every connection.
func TestSessionEventWithNoSubscribersGoesNowhere(t *testing.T) {
	h := New(Options{})
	ws, _, done := join(t, h, 1)
	defer done()
	waitFor(t, "the welcome", func() bool { return len(ws.sent()) > 0 })

	h.Session(KindReaction, 99)
	h.Notify([]int32{1})

	waitFor(t, "the notification", func() bool { return len(ws.sent()) == 2 })
	if got := kinds(t, ws)[1]; got != KindNotification {
		t.Errorf("got %q, want the session event to have been skipped", got)
	}
}

// A message the server cannot act on gets an error frame and the connection
// LIVES. Forward compatibility: a newer client must not lose its socket for
// saying something an older server has not heard of.
func TestABadMessageDoesNotEndTheConnection(t *testing.T) {
	h := New(Options{})
	ws, c, done := join(t, h, 1)
	defer done()
	waitFor(t, "the welcome", func() bool { return len(ws.sent()) > 0 })

	for _, msg := range []string{`not json at all`, `{"type":"nope"}`, `{"type":"subscribe","sessionId":0}`} {
		c.handle([]byte(msg))
	}
	waitFor(t, "three error frames", func() bool { return len(ws.sent()) == 4 })

	codes := []string{}
	for _, frame := range ws.sent()[1:] {
		codes = append(codes, decode(t, frame).Code)
	}
	want := []string{CodeBadJSON, CodeUnknownType, CodeBadSessionID}
	for i := range want {
		if codes[i] != want[i] {
			t.Errorf("error %d was %q, want %q", i, codes[i], want[i])
		}
	}

	// Still usable, which is the actual claim.
	c.handle([]byte(`{"type":"subscribe","sessionId":5}`))
	if !c.watching(5) {
		t.Error("the connection stopped working after a bad message")
	}
}

func TestSubscriptionsAreCapped(t *testing.T) {
	h := New(Options{})
	ws, c, done := join(t, h, 1)
	defer done()
	waitFor(t, "the welcome", func() bool { return len(ws.sent()) > 0 })

	for i := 1; i <= maxSubscriptions; i++ {
		c.handle([]byte(`{"type":"subscribe","sessionId":` + itoa(i) + `}`))
	}
	c.handle([]byte(`{"type":"subscribe","sessionId":999}`))

	waitFor(t, "the refusal", func() bool { return len(ws.sent()) == 2 })
	if got := decode(t, ws.sent()[1]).Code; got != CodeTooManySubs {
		t.Errorf("refusal code was %q, want %q", got, CodeTooManySubs)
	}
	if c.watching(999) {
		t.Error("the subscription past the cap was recorded anyway")
	}
}

// THE BACKPRESSURE POLICY, which is the decision most worth a test.
//
// A connection that will not drain is disconnected, and — the part that
// matters — every other connection still receives everything. The alternative,
// dropping the message, would leave that client silently stale with no way to
// learn it had missed anything, which is the failure this feature exists to
// remove.
func TestASlowConnectionIsDroppedAndTheRestAreUnaffected(t *testing.T) {
	h := New(Options{})

	// Two different accounts, so the burst below is aimed at the stuck one
	// alone. Sharing an id would make this test a race against the healthy
	// connection's own pump rather than an assertion about the policy.
	stuck := newFakeSocket()
	stuck.block = make(chan struct{})
	slow := newConn(h, stuck, 42)

	healthy, _, done := join(t, h, 1)
	defer done()
	waitFor(t, "the healthy welcome", func() bool { return len(healthy.sent()) > 0 })

	h.mu.Lock()
	h.conns[slow] = struct{}{}
	h.wg.Add(1)
	h.mu.Unlock()
	slowDone := make(chan struct{})
	go func() {
		defer close(slowDone)
		defer h.wg.Done()
		defer h.remove(slow)
		slow.run()
	}()

	// More than the buffer holds, at a connection whose pump is parked on its
	// welcome and will never drain one of them.
	for range sendBuffer + 2 {
		h.Notify([]int32{42})
	}

	waitFor(t, "the eviction", func() bool { return h.Stats().Dropped > 0 })

	// THE ASSERTION THAT MATTERS: the other connection is untouched and still
	// working. Dropping the message instead of the connection would leave the
	// slow client silently stale; dropping the connection badly would take this
	// one with it.
	before := len(healthy.sent())
	h.Notify([]int32{1})
	waitFor(t, "the healthy connection to keep working", func() bool {
		return len(healthy.sent()) == before+1
	})
	if got := kinds(t, healthy)[before]; got != KindNotification {
		t.Errorf("healthy connection received %q after the eviction, want %q",
			got, KindNotification)
	}

	// And once the stalled write finally returns, the connection actually goes.
	//
	// In production that wait is bounded by writeWait: a peer that cannot take
	// a forty-byte frame in ten seconds makes WriteMessage fail, the pump
	// returns, its deferred Close unblocks the read pump, and run() removes the
	// connection. This fake blocks indefinitely instead of on a deadline, so
	// the test plays the part of the deadline expiring.
	close(stuck.block)
	<-slowDone
	waitFor(t, "the slow connection to leave the hub", func() bool {
		return h.Stats().Connections == 1
	})
}

// Close ends every connection with a going-away frame rather than severing it,
// and refuses new ones so a reconnect backs off instead of racing the drain.
func TestCloseSendsGoingAwayAndRefusesNewConnections(t *testing.T) {
	h := New(Options{})
	ws, _, done := join(t, h, 1)
	defer done()
	waitFor(t, "the welcome", func() bool { return len(ws.sent()) > 0 })

	h.Close(context.Background())

	waitFor(t, "the close frame", func() bool { return len(ws.closeCodes()) > 0 })
	if got := ws.closeCodes()[0]; got != websocket.CloseGoingAway {
		t.Errorf("close code %d, want %d (going away)", got, websocket.CloseGoingAway)
	}

	// A second Close is a no-op rather than a panic on a closed channel.
	h.Close(context.Background())
}

// One encode, one slice, shared by every recipient. A test that this is safe as
// well as cheap: if the hub ever mutated the frame per connection, these would
// differ.
func TestAFrameIsEncodedOnceAndSharedIntact(t *testing.T) {
	h := New(Options{})
	a, _, done1 := join(t, h, 1)
	b, _, done2 := join(t, h, 1)
	defer done1()
	defer done2()
	waitFor(t, "both welcomes", func() bool {
		return len(a.sent()) > 0 && len(b.sent()) > 0
	})

	h.Notify([]int32{1})
	waitFor(t, "both notifications", func() bool {
		return len(a.sent()) == 2 && len(b.sent()) == 2
	})

	if string(a.sent()[1]) != string(b.sent()[1]) {
		t.Errorf("recipients got different bytes: %q vs %q", a.sent()[1], b.sent()[1])
	}
}

// Notify with nobody to tell does not encode a frame or walk the map.
// Broadcast reaches everybody, which is the whole of what makes it different from
// Notify — and in particular it reaches a connection that has asked for nothing:
// no id of its own in a recipient list, and no subscription.
func TestBroadcastReachesEveryConnection(t *testing.T) {
	h := New(Options{})

	first, _, doneFirst := join(t, h, 1)
	defer doneFirst()
	second, _, doneSecond := join(t, h, 2)
	defer doneSecond()

	waitFor(t, "both welcomes", func() bool {
		return len(first.sent()) > 0 && len(second.sent()) > 0
	})

	h.Broadcast(KindLevel)

	waitFor(t, "the level frame at both", func() bool {
		return len(first.sent()) > 1 && len(second.sent()) > 1
	})
	for name, ws := range map[string]*fakeSocket{"first": first, "second": second} {
		if got := kinds(t, ws)[1]; got != KindLevel {
			t.Errorf("%s connection received %q, want %q", name, got, KindLevel)
		}
	}
}

// A broadcast frame carries its kind and nothing else. It is unaddressed, so the
// thing that keeps it safe to send to everybody is that there is nothing on it to
// be told — no lifter id, no figure, no session.
func TestBroadcastCarriesNothingButItsKind(t *testing.T) {
	h := New(Options{})

	ws, _, done := join(t, h, 1)
	defer done()
	waitFor(t, "the welcome", func() bool { return len(ws.sent()) > 0 })

	h.Broadcast(KindLevel)
	waitFor(t, "the level frame", func() bool { return len(ws.sent()) > 1 })

	got := decode(t, ws.sent()[1])
	if got.Type != KindLevel {
		t.Fatalf("type = %q, want %q", got.Type, KindLevel)
	}
	if got.SessionID != 0 || got.Code != "" || got.Message != "" || got.Protocol != 0 {
		t.Errorf("frame carries more than its kind: %+v", got)
	}
}

// The eviction policy applies here too, and it has to: a broadcast puts a frame on
// EVERY queue at once, so it is the fan-out most able to fill a parked peer's
// buffer.
//
// Note what this test canNOT assert, and why that is the interesting part. The
// Notify version of it aims the burst at one account and checks the OTHER
// connection is unharmed; a broadcast has no way to spare anybody, so a burst of
// them threatens every open socket at once. That is precisely why liveEvents.levels
// is a bool rather than a list — one transaction gets one frame however many
// lifters' levels it moved — and the guard against this case lives there rather
// than here.
func TestBroadcastStillEvictsAConnectionThatIsTooFarBehind(t *testing.T) {
	h := New(Options{})

	// The stuck connection alone, unlike the Notify version: a healthy neighbour
	// would be flooded by the same burst, so there is nothing to compare against.
	stuck := newFakeSocket()
	stuck.block = make(chan struct{})
	slow := newConn(h, stuck, 42)

	h.mu.Lock()
	h.conns[slow] = struct{}{}
	h.wg.Add(1)
	h.mu.Unlock()
	slowDone := make(chan struct{})
	go func() {
		defer close(slowDone)
		defer h.wg.Done()
		defer h.remove(slow)
		slow.run()
	}()

	for range sendBuffer + 2 {
		h.Broadcast(KindLevel)
	}

	waitFor(t, "the eviction", func() bool { return h.Stats().Dropped > 0 })

	// And once the stalled write returns, the connection actually goes. See the
	// Notify version: the fake blocks indefinitely where production bounds the
	// write by writeWait, so the test plays the deadline expiring.
	close(stuck.block)
	<-slowDone
	waitFor(t, "the slow connection to leave the hub", func() bool {
		return h.Stats().Connections == 0
	})
}

func TestNotifyWithNoRecipientsDoesNothing(t *testing.T) {
	h := New(Options{})
	ws, _, done := join(t, h, 1)
	defer done()
	waitFor(t, "the welcome", func() bool { return len(ws.sent()) > 0 })

	h.Notify(nil)
	h.Notify([]int32{})
	h.Notify([]int32{2})

	// Prove by ordering: cause one that IS deliverable and check it is next.
	h.Notify([]int32{1})
	waitFor(t, "the deliverable notification", func() bool { return len(ws.sent()) == 2 })
	if got := len(ws.sent()); got != 2 {
		t.Errorf("sent %d frames, want the welcome and one notification", got)
	}
}

func TestStatsCountWhatWentOut(t *testing.T) {
	h := New(Options{})
	ws, _, done := join(t, h, 3)
	defer done()
	waitFor(t, "the welcome", func() bool { return len(ws.sent()) > 0 })

	if got := h.Stats().Connections; got != 1 {
		t.Errorf("connections %d, want 1", got)
	}
	h.Notify([]int32{3})
	waitFor(t, "the send to be counted", func() bool { return h.Stats().Sent == 1 })
}

// itoa without importing strconv into a test file that needs nothing else
// from it.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}
