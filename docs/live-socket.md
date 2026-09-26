# The live socket

`GET /api/v1/live`, upgraded to a WebSocket. One connection per signed-in
lifter, telling their browser when something happened instead of making it ask
every minute.

**This file is the contract.** OpenAPI 3.0 cannot describe a WebSocket, so
`src/api/openapi.yaml` — which is the contract for everything else — points
here instead of pretending. The two ends that must agree with it are
`src/api/internal/live/event.go` and `src/ui/src/lib/live.svelte.ts`, neither of
which is generated. Change one, change all three.

## The one rule everything follows from

**Every frame is a signal, never content.**

A frame says *something arrived for you*, or *something happened on session
412*. It never says what. The client answers by refetching through the ordinary
HTTP endpoints, which already do the authorization and are already ETagged — so
the refetch is usually a 304.

Three problems disappear because of that, and it is worth knowing which:

- **Nothing personal is on this wire.** A frame delivered to the wrong
  connection would leak nothing. The origin check below exists for integrity,
  not confidentiality.
- **The publish path reads no database.** This deployment has one small
  connection pool shared with request traffic; a fan-out that queried per
  recipient would be expensive in exactly the moment it is least affordable.
- **Dropping a slow client is safe**, because its reconnect refetches
  everything. That is what makes the backpressure policy defensible.

It also means this is **not a delivery guarantee**. A lifter with no socket open
misses every frame and loses nothing: the HTTP poller is still running, and a
reconnect refetches. Nothing here is the source of truth for anything.

## Connecting

A handshake is an ordinary authenticated GET. The session cookie rides
automatically because the socket is same-origin — there is no way to attach one
to a WebSocket by hand, which is the other reason it must stay same-origin.

| Requirement | Why |
|---|---|
| Valid `it_session` cookie | `requireUser`, same as every other endpoint. A handshake without one gets `401`. |
| Password already changed | `blockUntilPasswordChanged`. An account still holding a one-time password gets `403`. |
| Same-origin, or no `Origin` header | See below. A foreign origin gets `403`. |

The route sits **outside** the authenticated route group, and that is not an
oversight. `jsonETag` replaces the response writer with a recorder that buffers
the body so it can be hashed; the recorder is not an `http.Hijacker`, and an
upgrade needs the raw connection. Inside the group this endpoint could not work
at all. Both gates are therefore spelled out on the route itself, because it
inherits nothing.

### The origin check

A WebSocket handshake is a `GET`, which the CSRF middleware skips, and it
carries the session cookie **without any CORS involvement** — there is no
`AllowCredentials` switch standing between a page and this API. Without a check,
any site a lifter had visited could open an authenticated socket in their
browser.

So the socket enforces the same-origin rule directly, using the same function
the CSRF middleware does. `CORS_ORIGIN` is deliberately not consulted: it
defaults to `*`, which is only safe in the CORS block precisely *because*
credentials are off there.

An absent `Origin` passes, matching the middleware: non-browser clients do not
send one and are not the threat this describes.

## Frames

All frames are text, and all are JSON objects with a required `type`.

### Server → client

```jsonc
{"type":"welcome","protocol":1}     // always first
{"type":"notification"}             // your panel changed; refetch it
{"type":"reaction","sessionId":412} // only if subscribed to 412
{"type":"comment","sessionId":412}  // only if subscribed to 412
{"type":"level"}                    // somebody levelled up; refetch the levels
{"type":"error","code":"unknown_type","message":"…"}
```

**`welcome` is what "connected" means.** A socket whose `onopen` fired but whose
server side never got as far as writing anything is not a working connection,
and the client must not treat it as one — doing so would quiet the poller while
nothing was arriving.

**`level` is the one frame that goes to everybody**, and it is worth knowing why
that is not a leak of the rule above it. Every other frame is addressed: to an
account, or to whoever subscribed to a session. A level is drawn beside a name on
the feed, the roster, the leaderboard and the header, so the set of people whose
screen is now wrong is "anybody with a screen open" — addressing it would mean
either computing that set or accepting that most badges stay stale for ten
minutes. It says nothing about *whose* level moved, which is what makes
broadcasting it safe: a client learns only that the levels are worth asking for
again, and the answer it gets back is the same one it could already have fetched.

It is sent when a session is finished. **A level can also change with no frame at
all** — an unfinished session ages past the twelve-hour cutoff on its own, and no
request happens for anything to be published from. The ten-minute poll is what
covers that, which is the ordinary arrangement here rather than a gap: nothing on
this socket is the source of truth for anything.

**An `error` frame never closes the connection.** A newer client saying
something an older server has not heard of must not lose its socket for it. The
read limit and the subscription cap are the actual defences.

Error codes: `bad_json`, `unknown_type`, `bad_session_id`,
`too_many_subscriptions`.

### Client → server

```jsonc
{"type":"subscribe","sessionId":412}
{"type":"unsubscribe","sessionId":412}
```

That is the entire vocabulary. A client cannot send a notification, cannot read
anything over this socket, and cannot address another account — each of those is
an HTTP endpoint with its own rules, and none is reachable from here.

**Subscribing is not checked against the database**, and that is deliberate: any
signed-in lifter may already read any session's applause and conversation, so a
subscription grants nothing they did not already have. The events carry no
content, and the refetch they provoke goes through the authorized endpoint.
Subscribing to an id that does not exist costs one map entry and yields nothing.

Unknown `type` values are answered with `unknown_type` and ignored.

## Protocol version

`welcome.protocol` is bumped only when a frame's **meaning** changes. Adding a
kind or an optional field does not bump it — both ends ignore what they do not
recognise, which is what lets a deploy roll out without every open socket being
the same version as the server.

## Limits and timeouts

| | Value | Why |
|---|---|---|
| Ping | every 54s | Comfortably under the pong deadline, so one lost ping is not fatal, and under the 180s an idle upgraded connection typically gets from a reverse proxy. |
| Pong deadline | 60s | Reaps a connection whose peer vanished without a FIN — a phone that went into a tunnel. TCP would not notice for far longer. |
| Write deadline | 10s | A peer that cannot take a 40-byte frame in ten seconds is gone, not slow. |
| Read limit | 512 bytes | A subscribe frame is about forty. This is the defence against a client streaming into the server's buffers. |
| Send buffer | 16 frames | See below. |
| Subscriptions | 8 per connection | Nobody has eight recaps open; bounds a client that subscribes in a loop. |
| Max connection age | 12h | Forces a reconnect, and with it a fresh trip through `requireUser`. |

**The age cap is a security control, not housekeeping.** A socket is
authenticated once, at the handshake, so without it a connection would outlive a
sign-out on another device and a password change — both of which revoke the
session it was opened with. The alternative, re-checking on a timer, is
per-connection database polling on a small shared pool. If twelve hours is too
long, shorten it; do not add the poll.

## Backpressure: the connection is dropped, not the message

If a connection's send buffer is full, the server **disconnects it**.

A full 16-frame buffer on a human-paced signal stream means the peer is gone,
not busy. Eviction costs that client one reconnect and one refetch — which is
where it was heading anyway. Dropping a *message* instead would leave it
silently stale with no way to ever learn it had missed something, which is the
exact failure this feature exists to remove.

**This is only safe because the publish side dedupes.** The generated-activity
scheduler writes a day's worth of applause in one transaction; without
collapsing that to one event per recipient it would be a burst that evicts every
connected client. The two decisions have to be read together — see `liveEvents`
in `internal/api/live.go`.

## Publishing happens after the commit

Every fan-out query returns its recipients, so the rule for *who hears about
something* stays in SQL where it already lived. Go collects the answer inside
the transaction and publishes on the line **after** a successful commit — never
in a `defer`, which would fire on the rollback path and announce a notification
the database does not have.

## Shutdown

On `SIGTERM` the hub is closed **before** `http.Server.Shutdown`, because
`Shutdown` neither closes nor waits on a hijacked connection. Left to it, every
socket is severed as the process exits and the browser reconnects into a
listener that is already going away. Closing first sends a proper `1001`, and
new handshakes get `503` so a reconnect backs off instead of racing the drain.

## Client behaviour

- Reconnects with backoff (1, 2, 4, 8, 15, 30s, jittered), capped.
- Does not reconnect while the tab is hidden; an already-open socket is left
  alone, since it costs nothing.
- **Refetches in full on every connect**, which is what makes a reconnect a
  complete repair rather than a partial one.
- **Keeps polling.** The HTTP poller backs off from 60s to 5 minutes while the
  socket is up rather than stopping, so a proxy that eats upgrades leaves a
  working app behind.

## Deployment

The app's Kubernetes manifests are not in this repository, so Traefik's handling
of this route is the one thing here that cannot be verified from the source.
Traefik proxies upgrades transparently by default. Two things to confirm:

- nothing strips `Connection` / `Upgrade` on the way through;
- the responding idle timeout exceeds the 54s ping period (Traefik's default,
  180s, does).

If the upgrade never lands, the app degrades to exactly its previous behaviour —
polling — which is the reason the poller was kept.

In development, `vite.config.ts` proxies `/api` with `ws: true` and
`changeOrigin: false`. Both matter: without the first the handshake is answered
as ordinary HTTP, and without the second `Host` and `Origin` disagree and the
origin check refuses every socket.

## Metrics

`iron_temple_live_connections` (gauge), `iron_temple_live_events_sent_total`,
and `iron_temple_live_connections_dropped_total`. A nonzero rate on the last is
the alert worth having: it means the backpressure policy is firing.

A socket is deliberately **not** counted as an HTTP request — held open it would
sit in the in-flight gauge and put an hours-long sample into a latency histogram
whose largest finite bucket is ten seconds. A handshake that *fails* is still
counted, which is the half worth alerting on.
