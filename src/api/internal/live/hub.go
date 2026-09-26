package live

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
)

// Options is what the hub cannot decide for itself.
type Options struct {
	// CheckOrigin decides whether a handshake may proceed.
	//
	// Supplied by the caller rather than decided here, because the rule already
	// exists — internal/api enforces the same one on every non-GET request —
	// and a security rule stated twice is a security rule that will eventually
	// be stated two different ways.
	//
	// Nil means gorilla's default, which refuses any cross-origin handshake.
	// That is the safe direction to fail in, so a caller that forgets is not
	// thereby opened up.
	CheckOrigin func(*http.Request) bool
}

// Stats is what the metrics registry reads, per scrape.
type Stats struct {
	Connections int
	Sent        uint64
	Dropped     uint64
}

// Hub is every live connection on this process.
//
// # ONE MAP, FANNED OUT BY SCAN
//
// Not a userID→conns index and not a sessionID→conns reverse index. Three
// reasons, in order of how much they matter:
//
// One map is one invariant. Closing a connection removes it from exactly one
// place, so there is no second structure that can be left holding a dead
// pointer — which is the bug this kind of code actually gets wrong, and it does
// not announce itself, it leaks.
//
// The scale does not ask for more. This is an install with a handful of
// accounts on it, every event is human-paced — somebody tapped something — and
// CreateJoinNotifications already fans out to literally everybody, which is
// what a scan over the map is anyway.
//
// And if it ever does serve a gym, the reverse index is the obvious next move
// and this comment is where to start.
type Hub struct {
	up websocket.Upgrader

	mu     sync.RWMutex
	conns  map[*conn]struct{}
	closed bool

	// wg tracks the per-connection goroutines so Close can wait for them
	// rather than letting the process exit mid-frame.
	wg sync.WaitGroup

	sent    atomic.Uint64
	dropped atomic.Uint64
}

// New builds a hub. It starts nothing: a hub has no background loop, only the
// goroutines its connections bring with them.
func New(opts Options) *Hub {
	return &Hub{
		up: websocket.Upgrader{
			// Small, because a handshake is small and the frames this reads are
			// smaller. See readLimit in conn.go for the one that matters.
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
			CheckOrigin:     opts.CheckOrigin,
		},
		conns: make(map[*conn]struct{}),
	}
}

// Serve upgrades the request and then BLOCKS for the life of the connection.
//
// Blocking is deliberate: the read pump runs on the caller's goroutine, so the
// HTTP handler's lifetime and the connection's are the same thing and there is
// no question about who owns what. The write pump is the one goroutine this
// spawns.
//
// userID is the authenticated caller. The hub never authenticates anybody —
// that already happened in the middleware, which is the only place it should.
func (h *Hub) Serve(w http.ResponseWriter, r *http.Request, userID int32) {
	// Refused before the upgrade rather than after, so a client draining into a
	// process that is going away gets an HTTP error it can back off on instead
	// of a socket that dies a second later.
	h.mu.RLock()
	closed := h.closed
	h.mu.RUnlock()
	if closed {
		http.Error(w, "shutting down", http.StatusServiceUnavailable)
		return
	}

	ws, err := h.up.Upgrade(w, r, nil)
	if err != nil {
		// Upgrade has already written its own response — a 403 for a refused
		// origin, a 400 for a malformed handshake — so there is nothing to
		// write here and nothing worth logging: a browser navigating away
		// mid-handshake is ordinary.
		return
	}

	c := newConn(h, ws, userID)

	h.mu.Lock()
	if h.closed {
		// Lost a race with Close between the check above and here. Take the
		// socket down rather than registering it into a hub that is draining.
		h.mu.Unlock()
		_ = ws.Close()
		return
	}
	h.conns[c] = struct{}{}
	h.wg.Add(1)
	h.mu.Unlock()

	defer func() {
		h.remove(c)
		h.wg.Done()
	}()

	c.run()
}

// remove takes a connection out of the map. Idempotent: a connection whose
// pumps both end removes itself once, and Close may have got there first.
func (h *Hub) remove(c *conn) {
	h.mu.Lock()
	delete(h.conns, c)
	h.mu.Unlock()
}

// Notify tells each of these lifters that their notification panel changed.
//
// Takes a slice rather than one id because that is the shape the fan-out
// queries return — see notifications.sql, where the rule for WHO hears about
// something lives. Duplicates are harmless but wasteful; the caller dedupes.
func (h *Hub) Notify(userIDs []int32) {
	if len(userIDs) == 0 {
		return
	}
	frame, ok := encode(notificationEvent())
	if !ok {
		return
	}
	want := make(map[int32]struct{}, len(userIDs))
	for _, id := range userIDs {
		want[id] = struct{}{}
	}

	h.each(func(c *conn) {
		if _, ok := want[c.userID]; ok {
			h.deliver(c, frame)
		}
	})
}

// Broadcast tells every connection at once, addressed to nobody.
//
// THE ONLY UNADDRESSED FAN-OUT, and it exists for one kind. A level is drawn
// beside a name on the feed, the roster, the leaderboard and the header, so when
// one moves the set of clients holding a stale badge is every client — there is
// no smaller set to compute. Addressing it to the lifter and their followers was
// the obvious alternative and buys nothing: anybody looking at the roster who
// does not follow them would keep the old number until the poll, which is the
// staleness this frame exists to remove.
//
// Safe because the frame says nothing. It carries no lifter id and no figure, so
// a connection learns only that a read is worth repeating, and that read goes
// through the ordinary authorized endpoint. See the note on Event.
//
// Takes a Kind rather than hard-coding one so a second unaddressed frame does not
// need a second method — but think twice before adding one. Every frame here
// costs one queue slot on every open connection, and the eviction policy below is
// what makes that safe rather than free.
func (h *Hub) Broadcast(kind Kind) {
	frame, ok := encode(bareEvent(kind))
	if !ok {
		return
	}
	h.each(func(c *conn) {
		h.deliver(c, frame)
	})
}

// Session tells whoever is watching this session that something happened on it.
func (h *Hub) Session(kind Kind, sessionID int32) {
	frame, ok := encode(sessionEvent(kind, sessionID))
	if !ok {
		return
	}
	h.each(func(c *conn) {
		if c.watching(sessionID) {
			h.deliver(c, frame)
		}
	})
}

// each runs fn over every connection under the read lock.
//
// THE LOCK IS NEVER HELD ACROSS A SOCKET WRITE, which is the invariant that
// keeps a slow peer from stalling the whole hub. Everything fn does is a map
// lookup and a non-blocking channel send, neither of which can block — the
// actual writing happens on each connection's own write pump.
func (h *Hub) each(fn func(*conn)) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for c := range h.conns {
		fn(c)
	}
}

// deliver hands one pre-encoded frame to one connection, or evicts it.
//
// See conn.go's sendBuffer note for why a full buffer means the peer is gone
// rather than merely slow, and why dropping the CONNECTION is the safe choice
// where dropping the message would not be.
func (h *Hub) deliver(c *conn, frame []byte) {
	select {
	case c.send <- frame:
		h.sent.Add(1)
	default:
		h.dropped.Add(1)
		c.evict()
	}
}

// encode marshals a frame once, so N recipients cost one encode and share an
// immutable slice.
//
// A failure here cannot happen — every Event is plain scalars — but returning
// rather than panicking keeps a hypothetical one from taking down a process
// over a notification.
func encode(e Event) ([]byte, bool) {
	frame, err := json.Marshal(e)
	if err != nil {
		log.Printf("live: encode %s: %v", e.Type, err)
		return nil, false
	}
	return frame, true
}

// Close ends every connection with a going-away frame and waits, briefly.
//
// Ordered BEFORE http.Server.Shutdown by the caller, and that ordering is the
// point: Shutdown does not close or even wait on a hijacked connection, so
// without this every socket is simply severed as the process exits — the
// browser sees an abnormal close and reconnects into a listener that is already
// going away.
//
// Marking the hub closed first means a reconnect that beats the drain gets a
// 503 and backs off, rather than establishing a socket that cannot outlive the
// process.
func (h *Hub) Close(ctx context.Context) {
	h.mu.Lock()
	if h.closed {
		h.mu.Unlock()
		return
	}
	h.closed = true
	going := make([]*conn, 0, len(h.conns))
	for c := range h.conns {
		going = append(going, c)
	}
	h.mu.Unlock()

	for _, c := range going {
		c.goAway()
	}

	// Bounded by whichever expires first: the caller's deadline, or our own
	// patience. A socket whose peer has vanished without a FIN would otherwise
	// hold the drain open for as long as the write deadline allows.
	done := make(chan struct{})
	go func() {
		h.wg.Wait()
		close(done)
	}()

	grace, cancel := context.WithTimeout(ctx, closeGrace)
	defer cancel()
	select {
	case <-done:
	case <-grace.Done():
	}
}

// Stats is read per scrape by the metrics registry.
func (h *Hub) Stats() Stats {
	h.mu.RLock()
	n := len(h.conns)
	h.mu.RUnlock()
	return Stats{
		Connections: n,
		Sent:        h.sent.Load(),
		Dropped:     h.dropped.Load(),
	}
}

// now is a seam for the tests, which need deadlines to be checkable without
// sleeping through them.
var now = time.Now
