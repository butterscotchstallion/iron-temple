<script lang="ts">
  import UserMenu from "./UserMenu.svelte";
  import VersionChangelog from "./VersionChangelog.svelte";
  import { version } from "./version.svelte";

  // The black bar across the top: build version on the left, account on the
  // right. Full-bleed and sticky, so it sits outside <main>'s centred column.

  // Version + environment come from the API's /health (single source of truth),
  // so this reflects the running backend, not a build-time constant. Moved here
  // from the footer, where it was easy to miss. VersionChangelog renders it, and
  // hangs this release's notes off it when the build shipped with any.
  //
  // The fetch itself lives in version.svelte.ts, which App.svelte keeps polling
  // so a new release can be offered. The label shows `running` rather than
  // `latest`: it names the build you are actually looking at, and the update
  // prompt is what tells you a newer one exists.
</script>

<header
  class="sticky top-0 z-40 w-full border-b border-white/10 bg-black/95 backdrop-blur"
>
  <div class="mx-auto flex h-12 max-w-5xl items-center justify-between gap-4 px-5">
    <VersionChangelog
      version={version.running}
      environment={version.environment}
    />

    <UserMenu />
  </div>
</header>
