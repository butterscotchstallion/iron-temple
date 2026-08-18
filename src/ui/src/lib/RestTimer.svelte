<script lang="ts">
  import { untrack } from "svelte";
  import Play from "@lucide/svelte/icons/play";
  import RotateCcw from "@lucide/svelte/icons/rotate-ccw";
  import { formatTime } from "./time";

  // The design specifies a 3-minute rest between sets; default accordingly.
  // autoStartKey: increment it to reset + auto-start the countdown (the active
  // session bumps it each time a set is completed).
  let {
    seconds = 180,
    autoStartKey = 0,
  }: { seconds?: number; autoStartKey?: number } = $props();

  // Seed the countdown from the prop once; it's mutable state from here on, so
  // untrack the initial read to make that intent explicit.
  let remaining = $state(untrack(() => seconds));
  let running = $state(false);
  let handle: ReturnType<typeof setInterval> | undefined;

  function tick() {
    remaining = Math.max(0, remaining - 1);
    if (remaining === 0) stop();
  }

  function start() {
    if (running || remaining === 0) return;
    running = true;
    handle = setInterval(tick, 1000);
  }

  function stop() {
    running = false;
    if (handle) clearInterval(handle);
    handle = undefined;
  }

  function reset() {
    stop();
    remaining = seconds;
  }

  // Reset + auto-start whenever the parent bumps autoStartKey (a set was
  // completed). Only autoStartKey is tracked; the reset/start mutations are
  // untracked so ticking state changes don't re-trigger this effect.
  $effect(() => {
    const key = autoStartKey;
    untrack(() => {
      if (key > 0) {
        reset();
        start();
      }
    });
  });

  // Clear the interval if the component is destroyed mid-countdown. Unmounting
  // is also how the active session stops the timer once the workout is over —
  // it drops the whole widget rather than resetting it in place.
  $effect(() => () => stop());
</script>

<!-- A floating pill rather than a card in the page flow: the countdown matters
     between sets, not while you're reading the session, so it sits out of the
     way in the corner. Anchored bottom-right because the nav lives up top. -->
<div
  class="fixed bottom-4 right-4 z-40 flex items-center gap-3 rounded-2xl border border-border/60 bg-card/90 py-2 pl-4 pr-2 shadow-lg backdrop-blur"
  data-testid="rest-timer"
>
  <div class="flex flex-col leading-none">
    <span class="text-[0.6rem] uppercase tracking-[0.2em] text-muted-foreground">
      Rest
    </span>
    <span
      class="mt-1 font-[var(--font-display)] text-2xl font-black tabular-nums tracking-wider text-cyan drop-shadow-[0_0_8px_rgba(5,217,232,0.5)]"
      aria-live="polite"
      data-testid="rest-remaining"
    >
      {formatTime(remaining)}
    </span>
  </div>
  <div class="flex gap-1.5">
    <button
      class="inline-flex items-center gap-1.5 rounded-full border border-neon/60 bg-neon/10 px-3 py-1 text-xs font-semibold text-ink transition hover:bg-neon/25 disabled:opacity-40"
      onclick={start}
      disabled={running || remaining === 0}
    >
      <Play class="size-3" aria-hidden="true" />
      Start
    </button>
    <button
      class="inline-flex items-center gap-1.5 rounded-full border border-magenta/60 bg-magenta/10 px-3 py-1 text-xs font-semibold text-ink transition hover:bg-magenta/25"
      onclick={reset}
    >
      <RotateCcw class="size-3" aria-hidden="true" />
      Reset
    </button>
  </div>
</div>
