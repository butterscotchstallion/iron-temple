<script lang="ts">
  import NotificationBell from "./NotificationBell.svelte";
  import UserMenu from "./UserMenu.svelte";
  import VersionChangelog from "./VersionChangelog.svelte";
  import { auth } from "./auth.svelte";
  import { levelFor, percentIntoLevel } from "./levels.svelte";
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

  // YOUR OWN progress into your current level, drawn along the bottom edge of the
  // bar. Nobody else's: the badge in the account button is a fact about the reader
  // already, and a bar across the whole screen is the most prominent thing this app
  // could say about a number — it is worth that only for the person who can move it.
  //
  // Null until the site-wide list lands, and for a signed-out reader, and for an
  // account the list does not carry. All three draw no line rather than an empty
  // one: a track with nothing in it is a widget asking to be explained, and this
  // is meant to be noticed only by somebody who already knows what it is.
  const level = $derived(levelFor(auth.me?.id));
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

  <!-- The experience line: how far into your current level you are, along the whole
       width of the bar.

       NO TRACK, because the bar already has one. `-bottom-px` straddles the
       `border-b border-white/10` above rather than stacking on top of it, so as far
       as the line reaches the bar's bottom edge simply IS neon, and past it the same
       edge carries on as the hairline it always was. That is what lets this be so
       quiet and still legible: nothing new appears on the screen at all.
       Absolutely positioned against the <header>, which is `sticky` and therefore
       already a containing block — a `relative` here would fight it, since both are
       `position` utilities and the stylesheet's order would decide the winner.

       SCALED, NOT RESIZED. `transform` rather than `width` because this bar is
       sticky: the page scrolls underneath it, and animating a width would put a
       layout pass in the middle of that on a phone, where a transform stays on the
       compositor. The same rule app.css states for the streak card's flames.

       The transition is for the two ways this moves while somebody is looking at it,
       both of which arrive on their own: finishing a session refreshes the levels
       directly, and the socket's `level` frame refreshes them again. Levelling up
       moves it BACKWARDS — a new level starts empty — which is a slide rather than a
       jump for that reason, and the toast and confetti that fire with it are what
       say why. Dropped under prefers-reduced-motion, as every other motion here is.

       aria-hidden, and it takes no clicks. The reader's own level is already
       announced beside their name in the account button, with the figures behind it
       in that badge's card; a second progressbar in the banner landmark would be one
       more thing to walk past on every screen to hear a number already given. -->
  {#if level}
    <div
      class="pointer-events-none absolute -bottom-px left-0 h-0.5 w-full origin-left bg-primary/70 shadow-[0_0_8px_rgba(176,38,255,0.45)] transition-transform duration-700 ease-out motion-reduce:transition-none"
      style:transform="scaleX({percentIntoLevel(level) / 100})"
      aria-hidden="true"
      data-testid="level-line"
    ></div>
  {/if}
</header>
