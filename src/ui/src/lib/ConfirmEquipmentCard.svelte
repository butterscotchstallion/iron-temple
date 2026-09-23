<script lang="ts">
  import { link } from "svelte-spa-router";
  import { Card } from "$lib/components/ui/card";
  import { auth } from "./auth.svelte";
  import { equipmentConfirmed } from "./gym.svelte";

  // A prompt to go and check the gym the app made up, shown until the lifter
  // has actually looked at it once.
  //
  // It exists because the banner on the profile screen can only be read by
  // somebody already on the profile screen, and the lifter this is for has no
  // reason to go there — their gym looks fine, because a seeded rack looks
  // exactly like a described one. That is the whole bug: the app has been
  // guessing since 0013 and has never once asked anybody to check, so a lifter
  // on this install was offered a 35 lb plate they have never owned.
  //
  // A card and not a toast: a toast is for something that just happened, and
  // this is a standing fact about the account. It sits above the workout for
  // the one screen it appears on and then never again.

  // Dismissal is deliberately per-load rather than remembered. "Later" should
  // clear the screen a lifter came here to read, but it is not an answer — the
  // prescriptions are still running off numbers nobody has confirmed, and a
  // permanent dismissal would leave that true and silent forever. Confirming
  // the equipment is what actually ends this, and it takes one Save.
  let dismissed = $state(false);
</script>

<!--
  Both halves of the condition are load-bearing, and the profile one is the
  subtle half: `equipmentConfirmed()` answers false for a lifter who has not
  confirmed AND for a client that has not loaded anybody, because "no" is the
  only honest answer to "has this person confirmed?" when there is no person.
  Reading that as "show the nudge" would put a claim about somebody's gym on
  screen before the gym had been fetched — which is the same invent-from-nothing
  this whole change is removing. App.svelte mounts no route without an `auth.me`,
  so this cannot currently happen; it is one new caller away from happening.
-->
{#if auth.me && !equipmentConfirmed() && !dismissed}
  <Card class="border-primary/40 bg-primary/5 p-4">
    <h3
      class="text-xs font-semibold uppercase tracking-[0.2em] text-muted-foreground"
    >
      Check your equipment
    </h3>
    <p class="mt-1.5 text-sm text-card-foreground">
      We set you up with a standard home gym when you joined — a bar, a rack of
      plates, some dumbbells. You've never told us whether that's right, and
      every weight we give you is built out of it.
    </p>
    <div class="mt-3 flex items-center gap-4">
      <a
        href="/profile"
        use:link
        class="text-sm font-semibold text-primary transition hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary"
      >
        Set up my gym
      </a>
      <!--
        "Later" rather than "Not now", which is what this said first and what
        the update prompt already says. Both can be on Home at once, and two
        buttons with the same name on one screen is ambiguous to anybody
        reading it by name rather than by position — a screen reader, or a
        Playwright selector, which is how the collision surfaced.
      -->
      <button
        type="button"
        onclick={() => (dismissed = true)}
        class="text-sm text-muted-foreground transition hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary"
      >
        Later
      </button>
    </div>
  </Card>
{/if}
