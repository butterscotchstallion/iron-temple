/**
 * The one fetch every generated API call goes through.
 *
 * Orval generates `getMe()`, `listSessions()` and the rest as thin wrappers that
 * build a URL and hand it to this function (see orval.config.ts). That makes
 * this the only place two cross-cutting facts are stated, instead of once per
 * call site or once per module:
 *
 *   WHERE THE API IS. Every generated path is relative to /api/v1. In
 *   development Vite proxies that to the Go server on :8080 without rewriting
 *   Host, so the API sees a same-origin request and its CSRF check passes; in
 *   production Traefik path-routes it. Both are same-origin, which is also why
 *   the session cookie needs no `credentials` handling here — it is sent by
 *   default and the cookie is HttpOnly, so nothing else can touch it anyway.
 *
 *   WHAT "NO NETWORK" LOOKS LIKE. Covered at length below, because it is the
 *   part that is easy to get wrong and expensive when you do.
 *
 * WHY A TRANSPORT FAILURE IS A STATUS AND NOT A THROW
 *
 * `fetch` rejects when the request never left the building — radio off, DNS
 * gone, connection refused. It resolves for every answer the server actually
 * gave, including 500. That distinction is exactly the one the app cares about,
 * and it is load-bearing in two places: writeQueue.svelte.ts has to know
 * whether to keep a queued write and retry it or drop it as refused, and
 * connectivity.svelte.ts drives the offline banner off it.
 *
 * The obvious encoding is to let the rejection propagate and wrap every call in
 * try/catch. That spreads a subtle three-way distinction — unreachable, refused,
 * fine — across ~50 call sites, where a missing catch does not fail loudly. It
 * silently reclassifies "we never asked" as "the server said no", which in the
 * write queue means throwing away a set the lifter actually did.
 *
 * So the rejection is caught here and returned as `status: 0` instead. Zero is
 * not a real HTTP status and cannot collide with one; `Response.status` is only
 * ever 0 for an opaque cross-origin response, which this same-origin client can
 * never produce. Callers then narrow on the status they already have to narrow
 * on, and the offline case cannot be forgotten — it is just another arm.
 *
 * ABOUT THE CAST
 *
 * Orval types each operation's return as a union of its documented responses,
 * and calls this function as `apiFetch<ThatUnion>(...)`. `status: 0` is by
 * definition not in any of them, so constructing it requires the cast below.
 * That is the deliberate cost of the design and the reason it is confined to
 * this file: one `as T` here buys the guarantee that no call site can receive a
 * shape it has not been told about. `isTransportFailure()` in
 * connectivity.svelte.ts is the sanctioned way to ask.
 */

/**
 * Prefix for every generated path. Matches the Vite proxy and Traefik's route.
 *
 * Exported because accountExport.ts deliberately bypasses the generated client
 * — it wants the server's bytes rather than a parsed object — and still has to
 * agree with it about where the API lives.
 */
export const API_BASE_URL = "/api/v1";

/**
 * The status a response carries when the request never reached the server.
 * Not a real HTTP status — see the note above on why zero specifically.
 */
export const TRANSPORT_FAILURE_STATUS = 0;

/** What a caller gets back for every request, successful or not. */
export type ApiResponse<T = unknown> = {
  status: number;
  data: T;
  headers: Headers;
};

/**
 * Whether a status means the server did what was asked.
 *
 * For code that handles a response without caring which endpoint produced it —
 * the cache, the write queue. A call site that knows its own contract should
 * compare against the status it expects instead, because that is what narrows
 * orval's per-status union down to the body it wants; this returns a boolean and
 * so narrows nothing.
 *
 * The sentinel is not 2xx, so an unreachable server is correctly not "ok".
 */
export function isOk(status: number): boolean {
  return status >= 200 && status < 300;
}

export const apiFetch = async <T>(url: string, init?: RequestInit): Promise<T> => {
  let response: Response;
  try {
    response = await fetch(`${API_BASE_URL}${url}`, init);
  } catch {
    // Never left the building. Say so in the shape callers already handle,
    // rather than as an exception they have to remember to catch.
    return {
      status: TRANSPORT_FAILURE_STATUS,
      data: undefined,
      headers: new Headers(),
    } as T;
  }

  // 204 and friends have no body, and a HEAD/GET 304 may not either. Asking for
  // JSON anyway throws, which would land in no catch block and turn a perfectly
  // good "nothing to say" answer into an unhandled rejection.
  const body = await readBody(response);

  return { status: response.status, data: body, headers: response.headers } as T;
};

/**
 * The response body, or undefined when there isn't one.
 *
 * Content-Type decides rather than status code: the API answers errors as JSON
 * too, and those bodies are the ones the error banners render, so they must not
 * be dropped just because the status was not 2xx. Anything non-JSON (an avatar,
 * an export download) comes back as a Blob, which is what those call sites want.
 */
async function readBody(response: Response): Promise<unknown> {
  if (response.status === 204 || response.status === 304) return undefined;

  const contentType = response.headers.get("content-type") ?? "";
  if (contentType.includes("application/json")) {
    // A body that claims to be JSON and isn't is a broken server, not a case to
    // model — but it must not take down the caller either.
    return response.json().catch(() => undefined);
  }
  if (contentType === "") return undefined;

  return response.blob();
}
