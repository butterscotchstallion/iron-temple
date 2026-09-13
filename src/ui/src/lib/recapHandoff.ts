import type { Session } from "./api";

/**
 * One slot, holding the session the lifter just finished, for the recap route
 * that is about to mount.
 *
 * This exists for the gym in the basement. Finishing is a write, and a write
 * with no signal is queued and answered optimistically — so `finish()` succeeds
 * and pushes to the recap even when nothing reached the server. The recap then
 * asks for `/sessions/:id/recap`, which is a GET: not queued, and it comes back
 * as a transport failure. So does `getSession`, which rules out refetching the
 * session instead.
 *
 * Handing the object across in memory is what is left, and it is enough: a
 * session carries its own sets and its own previousBests, which between them
 * make duration, volume, the per-lift breakdown and the weight records — see
 * localRecap. The lifter gets a recap at the rack; the parts that need history
 * fill in when the app is next on signal.
 *
 * Take-once and id-matched, both on purpose. Id-matched so a stale object can
 * never decorate a different session's recap. Take-once so a reload — which
 * clears module state anyway — and a second visit both fall through to the
 * network path rather than silently repainting a recap the server has since
 * had more to say about.
 */
let handed: Session | null = null;

export function handOffSession(session: Session): void {
  handed = session;
}

/** Take the handed-off session if it is this one. Clears the slot either way. */
export function takeHandedSession(sessionId: number): Session | null {
  const held = handed;
  handed = null;
  return held?.id === sessionId ? held : null;
}
