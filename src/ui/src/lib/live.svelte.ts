import { API_BASE_URL } from "./apiFetch";
import { poll, setLiveBacked } from "./notifications.svelte";

// The live socket: one connection per signed-in lifter, telling this tab when
// something happened instead of making it ask every minute.
//
// # EVERY FRAME IS A SIGNAL, NEVER CONTENT
//
// The server's rule, and the client's half of it: a frame says "your panel
// changed", never what changed. This module's whole job is to turn that into a
// refetch through the ordinary API, which is already authorized and already
// ETagged — so the refetch is usually a 304 and costs almost nothing.
//
// It is why nothing here parses a payload, why a dropped frame is survivable,
// and why reconnecting is a complete repair rather than a partial one.
//
// # POLLING DOES NOT STOP, IT SLOWS DOWN
//
// The socket is an optimisation over the poller, not a replacement for it. A
// proxy that eats upgrades, a captive portal, a bug in the hub — in every one of
// those the app has to keep working, and the poller is what makes "the socket
// never connected" indistinguishable from "the socket is quiet". So this tells
// notifications.svelte.ts to back off to a five-minute safety net while
// connected, and to resume its ordinary minute when not.
//
// Modelled on version.svelte.ts and notifications.svelte.ts: module-level
// state, a start function returning a teardown, visibility-aware, mounted from
// App.svelte with $effect.

export const live = $state<{
  /** True between a welcome frame and the close that follows it. */
  connected: boolean;
}>({ connected: false });

/**
 * Reconnect backoff, in milliseconds.
 *
 * Climbs to half a minute and stays there. A household install has one server;
 * if it is down, asking twice a minute forever is the right amount of patience
 * — and the poller is covering the gap anyway.
 */
const BACKOFF_MS = [1000, 2000, 4000, 8000, 15000, 30000];

/** Jitter, so several tabs reopened at once do not reconnect in lockstep. */
const JITTER = 0.2;

let socket: WebSocket | null = null;
let reconnectAt: ReturnType<typeof setTimeout> | null = null;
let attempt = 0;
let stopped = true;

/**
 * Where the socket lives.
 *
 * Derived from the page rather than configured, so where the API lives is
 * still stated once (API_BASE_URL). Same-origin means the session cookie rides
 * automatically, exactly as it does for every fetch — there is no way to attach
 * one to a WebSocket by hand, which is the other reason this must stay
 * same-origin.
 */
function socketURL(): string {
  const scheme = window.location.protocol === "https:" ? "wss:" : "ws:";
  return `${scheme}//${window.location.host}${API_BASE_URL}/live`;
}

function clearReconnect() {
  if (reconnectAt !== null) {
    clearTimeout(reconnectAt);
    reconnectAt = null;
  }
}

/**
 * What a frame means to this client.
 *
 * Unknown types are ignored rather than logged or thrown on: the server may be
 * newer than this tab — a deploy does not close every open socket — and an
 * unrecognised frame is a thing to skip, not a failure.
 */
function receive(raw: string) {
  let event: { type?: string };
  try {
    event = JSON.parse(raw) as { type?: string };
  } catch {
    return;
  }

  switch (event.type) {
    case "welcome":
      // CONNECTED IS DECLARED HERE, NOT IN onopen. A socket whose open fired
      // but whose server side never got as far as writing anything is not a
      // working connection, and treating it as one would silence the poller
      // while nothing was arriving.
      live.connected = true;
      setLiveBacked(true);
      attempt = 0;
      // The full refetch that makes a reconnect a complete repair: anything
      // that happened while this tab was disconnected is picked up here rather
      // than waited for.
      void poll();
      break;

    case "notification":
      void poll();
      break;

    default:
      break;
  }
}

function scheduleReconnect() {
  if (stopped || reconnectAt !== null) return;

  const base = BACKOFF_MS[Math.min(attempt, BACKOFF_MS.length - 1)];
  attempt += 1;
  const delay = base * (1 + (Math.random() * 2 - 1) * JITTER);

  reconnectAt = setTimeout(() => {
    reconnectAt = null;
    connect();
  }, delay);
}

function connect() {
  if (stopped || socket !== null) return;
  // A reconnect LOOP in a background tab is waste — nobody is looking at the
  // badge. An already-open socket is left alone when the tab is hidden, because
  // it costs nothing and keeps the badge current for when they come back.
  if (document.visibilityState !== "visible") return;

  let ws: WebSocket;
  try {
    ws = new WebSocket(socketURL());
  } catch {
    // Construction throws on a malformed URL, which would mean a bug here
    // rather than a network problem — but a throw on the way up would take the
    // effect in App.svelte with it.
    scheduleReconnect();
    return;
  }
  socket = ws;

  ws.onmessage = (event) => {
    if (typeof event.data === "string") receive(event.data);
  };

  const ended = () => {
    // Guard against the pair: a failed connection fires error AND close.
    if (socket !== ws) return;
    socket = null;
    live.connected = false;
    setLiveBacked(false);
    scheduleReconnect();
  };
  ws.onclose = ended;
  ws.onerror = ended;
}

/**
 * Open the socket and keep it open. Returns a teardown.
 *
 * The caller decides WHETHER to start one — App.svelte knows whether anybody is
 * signed in and re-runs its effect when that changes — which is the same
 * division startPolling draws.
 */
export function startLive(): () => void {
  stopped = false;
  attempt = 0;
  connect();

  // Coming back to a tab is when a socket is most likely to have died
  // unnoticed: a laptop that slept has a connection the browser has already
  // given up on. Reconnecting here rather than waiting out a backoff is what
  // makes the badge right by the time somebody has looked at it.
  const onVisibility = () => {
    if (document.visibilityState !== "visible") return;
    if (socket === null) connect();
  };
  document.addEventListener("visibilitychange", onVisibility);

  return () => {
    stopped = true;
    document.removeEventListener("visibilitychange", onVisibility);
    clearReconnect();
    if (socket !== null) {
      const ws = socket;
      // Cleared first, so the handlers above see a socket that is no longer
      // the current one and do not schedule a reconnect into a torn-down
      // module.
      socket = null;
      ws.close();
    }
    live.connected = false;
    setLiveBacked(false);
  };
}

/**
 * Forget everything. Called on sign-out beside resetNotifications, so the next
 * account to use this tab does not inherit a socket opened as somebody else.
 */
export function resetLive(): void {
  stopped = true;
  clearReconnect();
  if (socket !== null) {
    const ws = socket;
    socket = null;
    ws.close();
  }
  attempt = 0;
  live.connected = false;
  setLiveBacked(false);
}
