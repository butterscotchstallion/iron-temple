/**
 * A WebSocket that never touches a network.
 *
 * jsdom provides no WebSocket at all, and this suite has no precedent for
 * faking one — every other async boundary here is a mocked client function
 * returning a resolved promise. So this is the seam: installed with
 * `vi.stubGlobal("WebSocket", FakeWebSocket)`, which works because
 * live.svelte.ts constructs its socket inside a function rather than at import
 * time.
 *
 * LIVES OUTSIDE `src/` on purpose. The vitest `include` glob is
 * `src/**\/*.{test,spec}.ts` and coverage is collected over `src/`, so a helper
 * here is neither run as a test nor counted as uncovered production code, and
 * no config change is needed to keep it that way.
 *
 * Only what live.svelte.ts uses is implemented. It is not a WebSocket; it is
 * the part of one that module can observe.
 */
export class FakeWebSocket {
  static readonly CONNECTING = 0;
  static readonly OPEN = 1;
  static readonly CLOSING = 2;
  static readonly CLOSED = 3;

  /** Every socket constructed since the last `reset()`, in order. */
  static instances: FakeWebSocket[] = [];

  /** The most recent one, which is what a test almost always means. */
  static get last(): FakeWebSocket {
    const ws = FakeWebSocket.instances.at(-1);
    if (!ws) throw new Error("no WebSocket was constructed");
    return ws;
  }

  static reset(): void {
    FakeWebSocket.instances = [];
  }

  readonly url: string;
  readyState: number = FakeWebSocket.CONNECTING;
  /** Frames the client sent, as raw strings. */
  readonly sent: string[] = [];
  /** How many times close() was called, so a test can assert a teardown. */
  closed = 0;

  onopen: (() => void) | null = null;
  onclose: (() => void) | null = null;
  onerror: (() => void) | null = null;
  onmessage: ((event: { data: unknown }) => void) | null = null;

  constructor(url: string) {
    this.url = url;
    FakeWebSocket.instances.push(this);
  }

  send(data: string): void {
    this.sent.push(data);
  }

  close(): void {
    this.closed += 1;
    this.readyState = FakeWebSocket.CLOSED;
  }

  // ---- what a test drives ----

  /** The transport opened. Deliberately NOT the same as being connected. */
  open(): void {
    this.readyState = FakeWebSocket.OPEN;
    this.onopen?.();
  }

  /** Deliver a frame, as the server would. */
  emit(event: Record<string, unknown>): void {
    this.onmessage?.({ data: JSON.stringify(event) });
  }

  /** Deliver raw text, for the malformed-frame cases. */
  emitRaw(data: unknown): void {
    this.onmessage?.({ data });
  }

  /** The welcome frame, which is what "connected" actually means. */
  welcome(protocol = 1): void {
    this.open();
    this.emit({ type: "welcome", protocol });
  }

  /** The connection ended, from the server's side or the network's. */
  fail(): void {
    this.readyState = FakeWebSocket.CLOSED;
    this.onclose?.();
  }

  /** What the client asked for, parsed. */
  messages(): Array<Record<string, unknown>> {
    return this.sent.map((raw) => JSON.parse(raw) as Record<string, unknown>);
  }
}
