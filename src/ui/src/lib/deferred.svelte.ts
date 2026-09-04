// Fetch a component instead of bundling it into the entry chunk.
//
// The header and the update prompt are built on bits-ui, which — with
// @floating-ui, tabbable, svelte-toolbelt and runed behind it — was about a
// third of the entry chunk. None of it is needed to paint the app: the account
// menu and changelog panel open on a click, and the update dialog only appears
// when a release lands. Importing them statically meant the browser had to
// download and EXECUTE a popover library before it could render anything.
//
// Deliberately started at mount rather than on idle. The point is to keep those
// bytes off the critical path, not to delay them: the chunk downloads alongside
// the app's own first requests and renders the moment it lands, which on a
// same-origin LAN is well before /me answers. Waiting for idle would also make
// the header depend on a timer, and Playwright's fake clock — which the update
// e2e tests install — freezes those.

/**
 * A component that arrives shortly after the page does.
 *
 * `current` is undefined until the chunk lands, so callers render a placeholder
 * meanwhile. For a component whose own content is waiting on the network
 * anyway, that placeholder only has to hold the space.
 */
export function deferred<T>(load: () => Promise<{ default: T }>) {
  let current = $state<T | undefined>(undefined);
  // Deduped, so two callers asking at once share one fetch.
  let inFlight: Promise<void> | undefined;

  return {
    /** The loaded component, or undefined while it is still on its way. */
    get current() {
      return current;
    },
    /** Start fetching. Safe to call more than once. */
    load(): Promise<void> {
      inFlight ??= load().then((module) => {
        current = module.default;
      });
      return inFlight;
    },
  };
}
