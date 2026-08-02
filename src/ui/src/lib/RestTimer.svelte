<script lang="ts">
  import { formatTime } from "./time";

  // The design specifies a 3-minute rest between sets; default accordingly.
  let { seconds = 180 }: { seconds?: number } = $props();

  let remaining = $state(seconds);
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
