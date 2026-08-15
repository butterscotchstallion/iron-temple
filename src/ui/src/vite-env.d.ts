/// <reference types="svelte" />
/// <reference types="vite/client" />

// Release notes for the running build, inlined by changelogVirtualModule() in
// vite.config.ts. `entries` is empty whenever they couldn't be determined — a dev
// checkout with no releasable commits, or an image built without them — so every
// consumer has to handle that case.
declare module "virtual:iron-temple/changelog" {
  const changelog: { version: string; entries: string[] };
  export default changelog;
}
