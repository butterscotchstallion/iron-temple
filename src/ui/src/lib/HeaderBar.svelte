<script lang="ts">
  import NotificationBell from "./NotificationBell.svelte";
  import UserMenu from "./UserMenu.svelte";
  import VersionChangelog from "./VersionChangelog.svelte";
  import { auth } from "./auth.svelte";
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

<!-- No backdrop-blur. The bar is sticky, so the whole page passes underneath it:
     a blurred backdrop is one the compositor has to re-read and re-blur on every
     scrolled frame, and Firefox pays noticeably more for that than Chrome does.
     What it bought was nothing — bg-black/95 covers the backdrop to within 5%,
     and a 5% ghost of blurred-vs-sharp text is not a visible effect. Anything
     translucent enough to blur usefully (see NavBar, bg-card/40) is welcome to
     it; this is not that. -->
<header
  class="sticky top-0 z-40 w-full border-b border-white/10 bg-black/95"
>
  <div class="mx-auto flex h-12 max-w-5xl items-center justify-between gap-4 px-5">
    <VersionChangelog
      version={version.running}
      environment={version.environment}
    />

    <div class="flex items-center gap-1">
      <!-- Only once there is somebody to notify. Signed out, the right-hand
           side is a Sign in link and a bell would be counting nothing — and
           the endpoint behind it answers 401 anyway.

           Also held back during a forced password change: that state refuses
           every endpoint except the two it needs to escape, so polling here
           would be a 403 a minute for a panel the lifter cannot act on. Same
           condition App.svelte uses to start the polling at all. -->
      {#if auth.me && !auth.me.mustChangePassword}
        <NotificationBell />
      {/if}
      <UserMenu />
    </div>
  </div>
</header>
