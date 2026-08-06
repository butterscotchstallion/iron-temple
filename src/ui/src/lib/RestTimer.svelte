<script lang="ts">
  import { untrack } from "svelte";
  import { formatTime } from "./time";

  // The design specifies a 3-minute rest between sets; default accordingly.
  // autoStartKey: increment it to reset + auto-start the countdown (the active
  // session bumps it each time a set is completed).
  // resetKey: increment to reset the countdown WITHOUT starting it (used when
  // the whole session is finished — no rest needed).
  let {
    seconds = 180,
    autoStartKey = 0,
    resetKey = 0,
  }: { seconds?: number; autoStartKey?: number; resetKey?: number } = $props();

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

  // Reset (stop) whenever the parent bumps resetKey.
  $effect(() => {
    const key = resetKey;
    untrack(() => {
      if (key > 0) reset();
    });
  });

  // Clear the interval if the component is destroyed mid-countdown.
  $effect(() => () => stop());
</script>

<div class="flex flex-col items-center gap-3">
  <div
    class="font-[var(--font-display)] text-6xl font-black tabular-nums tracking-widest text-cyan drop-shadow-[0_0_12px_rgba(5,217,232,0.6)]"
    aria-live="polite"
    data-testid="rest-remaining"
  >
    {formatTime(remaining)}
  </div>
  <div class="flex gap-2">
    <button
      class="rounded-full border border-neon/60 bg-neon/10 px-5 py-2 font-semibold text-ink transition hover:bg-neon/25 disabled:opacity-40"
      onclick={start}
      disabled={running || remaining === 0}
    >
      Start
    </button>
    <button
      class="rounded-full border border-magenta/60 bg-magenta/10 px-5 py-2 font-semibold text-ink transition hover:bg-magenta/25"
      onclick={reset}
    >
      Reset
    </button>
  </div>
</div>
