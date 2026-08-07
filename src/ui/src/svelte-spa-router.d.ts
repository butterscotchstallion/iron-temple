import type { Readable } from "svelte/store";

// svelte-spa-router@5.1.1 exports the `location` store at runtime but omits it from its
// bundled type declarations (upstream types gap — ItalyPaleAle/svelte-spa-router#250),
// so `import { location } from "svelte-spa-router"` fails svelte-check while `link`/`push`
// resolve fine. Augment the module to declare the missing store. Remove once upstream
// ships the type. The top-level `import type` makes this a module, so `declare module`
// MERGES with the package's own types rather than replacing them.
declare module "svelte-spa-router" {
  /** Current route — the path after the hash, e.g. "/programs/1". */
  export const location: Readable<string>;
}
