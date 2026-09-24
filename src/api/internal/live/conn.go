package live

import (
	"encoding/json"
	"log"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	// writeWait bounds a single write. A peer that cannot accept a 40-byte
	// frame in ten seconds is not slow, it is gone.
	writeWait = 10 * time.Second
	// pongWait is how long a connection may go without answering a ping before
	// it is reaped. This is what catches a phone that went into a tunnel:
	// TCP will not notice for a great deal longer.
	pongWait = 60 * time.Second
	// pingPeriod must be comfortably under pongWait, so a single lost ping is
	// not fatal. Also under the 180s any sensible reverse proxy allows an idle
	// upgraded connection — see docs/live-socket.md.
	pingPeriod = 54 * time.Second
	// readLimit is the defence against a client streaming megabytes into this
	// process's buffers. A subscribe frame is about forty bytes.
	readLimit = 512
	// sendBuffer is how far behind a connection may fall before it is evicted.
	//
	// Sixteen frames of a human-paced signal stream is a lot of room. See
	// evict() for why exceeding it means the peer is gone rather than busy.
	sendBuffer = 16
	// maxSubscriptions bounds the per-connection map against a client that
	// subscribes in a loop. Nobody has eight recaps open.
	maxSubscriptions = 8
	// maxConnAge forces a reconnect, and with it a fresh trip through
	// requireUser.
	//
	// A socket is authenticated ONCE, at the handshake. Without a cap it would
	// outlive a sign-out on another device and a password change, both of which
	// revoke the session it was opened with. The alternative — re-checking the
	// session on a timer — is per-connection database polling on a small shared
	// pool, which is the one thing this package is careful not to do.
	maxConnAge = 12 * time.Hour
	// closeGrace bounds how long Close waits for the pumps to finish.
	closeGrace = 2 * time.Second
)

// socket is the part of *websocket.Conn this package uses.
//
// An interface rather than the concrete type, and it earns that: it is what
// lets the most concurrent code in this repository be tested under -race with
// no network, no Docker and no httptest server. *websocket.Conn satisfies it
// as written.
type socket interface {
	ReadMessage() (int, []byte, error)
	WriteMessage(messageType int, data []byte) error
	WriteControl(messageType int, data []byte, deadline time.Time) error
	SetReadLimit(int64)
	SetReadDeadline(t time.Time) error
	SetWriteDeadline(t time.Time) error
	SetPongHandler(h func(string) error)
	Close() error
}

// conn is one lifter's socket.
//
// EXACTLY ONE GOROUTINE WRITES to the websocket — gorilla requires it, and it
// is why the ping ticker lives inside writePump rather than in a third
// goroutine. Exactly one reads, on the HTTP handler's own goroutine.
type conn struct {
	hub    *Hub
	ws     socket
	userID int32

	// send carries pre-encoded frames from whoever is publishing to the one
	// goroutine allowed to write them.
	send chan []byte

	// mu guards subs and nothing else. Lock order is always hub.mu then
	// conn.mu, never the reverse, so there is no cycle to deadlock on.
	mu   sync.Mutex
	subs map[int32]struct{}

	// kill is closed exactly once, by evict or goAway, to wake the write pump.
	kill     chan struct{}
	killOnce sync.Once
	// goingAway is set when the close was the server draining rather than the
	// connection misbehaving, so the pump sends 1001 instead of 1008.
	goingAway bool
}

func newConn(h *Hub, ws socket, userID int32) *conn {
	return &conn{
		hub:    h,
		ws:     ws,
		userID: userID,
		send:   make(chan []byte, sendBuffer),
		subs:   make(map[int32]struct{}),
		kill:   make(chan struct{}),
	}
}

// run drives the connection until either pump gives up. It returns when the
// socket is finished with.
func (c *conn) run() {
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		c.writePump()
	}()

	c.readPump()
	// The read pump has ended, so the peer is done or the deadline passed.
	// Waking the writer and waiting for it means the close frame is actually
	// on the wire before the socket is closed underneath it.
	c.stop(false)
	wg.Wait()
	_ = c.ws.Close()
}

// evict ends a connection the hub has given up on.
func (c *conn) evict() { c.stop(false) }

// goAway ends a connection because the process is draining.
func (c *conn) goAway() { c.stop(true) }

func (c *conn) stop(going bool) {
	c.killOnce.Do(func() {
		c.goingAway = going
		close(c.kill)
	})
}

func (c *conn) watching(sessionID int32) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, ok := c.subs[sessionID]
	return ok
}

// readPump reads client messages until the peer goes away or stops answering
// pings.
//
// The recover is not optional. chi's Recoverer wraps the HTTP handler, and this
// runs on that goroutine — but writePump does not, and a panic on an
// unrecovered goroutine takes the whole process down. Both pumps carry one for
// the same reason, and a notification is not worth a restart.
func (c *conn) readPump() {
	defer func() {
		if v := recover(); v != nil {
			log.Printf("live: read pump panic: %v", v)
			c.evict()
		}
	}()

	c.ws.SetReadLimit(readLimit)
	_ = c.ws.SetReadDeadline(now().Add(pongWait))
	c.ws.SetPongHandler(func(string) error {
		return c.ws.SetReadDeadline(now().Add(pongWait))
	})

	for {
		_, data, err := c.ws.ReadMessage()
		if err != nil {
			return
		}
		c.handle(data)
	}
}

// handle acts on one client message.
//
// A message this cannot act on gets an error frame and the connection LIVES.
// Forward compatibility is the reason: a newer client sending a kind an older
// server has never heard of must not have its socket taken away for it. The
// read limit and the subscription cap are the actual defences here; this is a
// conversation, not a gate.
func (c *conn) handle(data []byte) {
	var msg clientMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		c.queue(errorEvent(CodeBadJSON, "that was not a JSON object"))
		return
	}

	switch msg.Type {
	case msgSubscribe:
		if msg.SessionID <= 0 {
			c.queue(errorEvent(CodeBadSessionID, "sessionId must be a positive integer"))
			return
		}
		// NO DATABASE CHECK, and that is deliberate. Any signed-in lifter may
		// already read any session's applause and conversation — see
		// sessionForRecognition, where the ownership check is deliberately
		// replaced by "anyone on this install may applaud anything on it" — so
		// a subscription grants nothing the caller did not already have. The
		// events carry no content, and the refetch they provoke goes through
		// the authorized HTTP endpoint. Subscribing to an id that does not
		// exist costs one map entry and yields no events, forever.
		c.mu.Lock()
		if len(c.subs) >= maxSubscriptions {
			c.mu.Unlock()
			c.queue(errorEvent(CodeTooManySubs, "too many sessions subscribed"))
			return
		}
		c.subs[msg.SessionID] = struct{}{}
		c.mu.Unlock()

	case msgUnsubscribe:
		c.mu.Lock()
		delete(c.subs, msg.SessionID)
		c.mu.Unlock()

	default:
		c.queue(errorEvent(CodeUnknownType, "unknown message type"))
	}
}

// queue puts a frame on this connection's own send channel, dropping it if the
// buffer is full rather than evicting.
//
// The asymmetry with Hub.deliver is on purpose: an error frame is a reply to
// something this client just said, and a client that is not draining its socket
// has bigger problems than a missing explanation. Evicting over one would turn
// a malformed message into a disconnection, which is what handle() above
// declines to do.
func (c *conn) queue(e Event) {
	frame, ok := encode(e)
	if !ok {
		return
	}
	select {
	case c.send <- frame:
	default:
	}
}

// writePump owns the socket's write side: every frame, every ping, and the
// close.
func (c *conn) writePump() {
	// CLOSING THE SOCKET HERE IS WHAT MAKES EVICTION TAKE EFFECT.
	//
	// The read pump is parked in ReadMessage and cannot see the kill channel,
	// so without this an evicted connection would sit in the hub's map until
	// its peer happened to disconnect — which for the case eviction exists to
	// handle (a peer that has silently gone away) could be the pong deadline
	// away, or never. Closing the socket makes that read return an error, the
	// read pump return, and run() finish, which is what removes it.
	//
	// Ordered after the recover so a panic here still takes the socket down.
	defer func() {
		if v := recover(); v != nil {
			log.Printf("live: write pump panic: %v", v)
		}
		_ = c.ws.Close()
	}()

	ping := time.NewTicker(pingPeriod)
	defer ping.Stop()
	age := time.NewTimer(maxConnAge)
	defer age.Stop()

	// The welcome is the first thing on the wire, and the client waits for it
	// before declaring itself connected.
	if !c.write(mustEncode(welcomeEvent())) {
		return
	}

	for {
		select {
		case frame := <-c.send:
			if !c.write(frame) {
				return
			}

		case <-ping.C:
			_ = c.ws.SetWriteDeadline(now().Add(writeWait))
			if err := c.ws.WriteControl(
				websocket.PingMessage, nil, now().Add(writeWait),
			); err != nil {
				return
			}

		case <-age.C:
			// Twelve hours. Forces the reconnect that re-authenticates.
			c.sendClose(websocket.CloseNormalClosure, "reconnect")
			return

		case <-c.kill:
			if c.goingAway {
				c.sendClose(websocket.CloseGoingAway, "going away")
			} else {
				c.sendClose(websocket.ClosePolicyViolation, "too far behind")
			}
			return
		}
	}
}

func (c *conn) write(frame []byte) bool {
	_ = c.ws.SetWriteDeadline(now().Add(writeWait))
	return c.ws.WriteMessage(websocket.TextMessage, frame) == nil
}

// sendClose puts a close frame on the wire, best effort. A peer that has
// already gone gets nothing and that is fine — this is a courtesy, and the
// socket is closed either way.
func (c *conn) sendClose(code int, reason string) {
	_ = c.ws.WriteControl(
		websocket.CloseMessage,
		websocket.FormatCloseMessage(code, reason),
		now().Add(writeWait),
	)
}

// mustEncode is for the frames built from constants in this package, which
// cannot fail to marshal. It returns nil on the impossible branch rather than
// panicking, and a nil write is a harmless empty frame.
func mustEncode(e Event) []byte {
	frame, _ := encode(e)
	return frame
}
