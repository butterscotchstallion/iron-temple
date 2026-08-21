<script lang="ts">
  import { untrack } from "svelte";
  import Bell from "@lucide/svelte/icons/bell";
  import BellOff from "@lucide/svelte/icons/bell-off";
  import Play from "@lucide/svelte/icons/play";
  import RotateCcw from "@lucide/svelte/icons/rotate-ccw";
  import SkipForward from "@lucide/svelte/icons/skip-forward";
  import { formatTime } from "./time";
  import { saveRest, loadRest, clearRest } from "./restStorage";
  import { fire, isMuted, prime, setMuted } from "./restAlert";

  // seconds: the rest this lift asks for. It comes from the exercise (see
  // migration 0011), so a set of deadlifts and a set of curls no longer count
  // down the same three minutes. Still defaulted, for a timer rendered with
  // nothing prescribed.
  //
  // autoStartKey: increment it to reset + auto-start the countdown (the active
  // session bumps it each time a set is completed).
  //
  // storageKey: when given, a running countdown survives a reload. It's what
  // makes taking a mid-workout update free — see restStorage.ts. Optional, so a
  // timer with no session to belong to stays purely in-memory.
  let {
    seconds = 180,
    autoStartKey = 0,
    storageKey,
  }: { seconds?: number; autoStartKey?: number; storageKey?: string } =
    $props();

  // What ± moves the clock by, and the ceiling it can be pushed to. An hour is
  // not a rest between sets; the cap only exists so a stuck finger can't put the
  // countdown somewhere it takes a reset to come back from.
  const STEP = 30;
  const MAX_SECONDS = 3600;

  // A countdown left running by the previous page load, if there is one. Read
  // during init so the first paint is already the resumed time rather than a
  // flash of 3:00 — and untracked for the same reason `seconds` is.
  const resumed = untrack(() =>
    storageKey ? loadRest(storageKey, Date.now()) : null,
  );

  // Seed the countdown from the prop once; it's mutable state from here on, so
  // untrack the initial read to make that intent explicit.
  let remaining = $state(resumed ?? untrack(() => seconds));
  let running = $state(false);
  let muted = $state(isMuted());
  let handle: ReturnType<typeof setInterval> | undefined;

  // When this rest ends (epoch ms), and the source of truth while it runs.
  //
  // The interval used to decrement `remaining` by one each time it fired, which
  // quietly assumed the browser would keep firing it once a second. A
  // backgrounded tab throttles that to a fraction of the rate and a locked phone
  // can suspend it outright — so a lifter who pocketed their phone came back to
  // a clock that had barely moved, and to an alert that hadn't gone off. Ticks
  // now only paint the difference between this number and the wall clock; they
  // don't count anything, so missing a few costs nothing.
  let endsAt = 0;

  function remember() {
    if (storageKey) saveRest(storageKey, endsAt);
  }

  function forget() {
    if (storageKey) clearRest(storageKey);
  }

  const clamp = (s: number) => Math.min(MAX_SECONDS, Math.max(0, s));

  function secondsLeft(): number {
    // Round up, so a rest with 400ms left reads as one second rather than zero —
    // the same direction restStorage.ts rounds when it restores one.
    return Math.max(0, Math.ceil((endsAt - Date.now()) / 1000));
  }

  function tick() {
    remaining = secondsLeft();
    if (remaining === 0) {
      stop();
      // The whole point of the countdown: the phone is in a pocket by now, so
      // being over has to be audible rather than merely visible.
      fire();
    }
  }

  function start() {
    if (running || remaining === 0) return;
    running = true;
    endsAt = Date.now() + remaining * 1000;
    remember();
    // Whatever led here is a tap — the Start button, or a logged rep upstream —
    // so this is the moment the browser will let us open an audio device.
    prime();
    // Four times a second: the display only changes once a second, but the
    // deadline can fall anywhere between two ticks, and a chime up to a full
    // second late is a chime you've stopped waiting for.
    handle = setInterval(tick, 250);
  }

  // Take the countdown off the clock without touching what's stored. This is
  // unmounting, not stopping: navigating to History mid-rest and coming back
  // should find the rest still running, and a reload must obviously not erase
  // the very snapshot it's about to restore from.
  function halt() {
    running = false;
    if (handle) clearInterval(handle);
    handle = undefined;
  }

  function stop() {
    halt();
    // Only a running countdown is stored, so stopping — by hand or by hitting
    // zero — drops it. A paused timer is not a rest in progress.
    forget();
  }

  function reset() {
    stop();
    remaining = seconds;
  }

  // End the rest now. Unlike Reset it doesn't put the clock back to the top: the
  // lifter is done resting, not starting over, and 0:00 is the honest picture of
  // that. The next logged rep restarts it from the lift's prescription anyway.
  function skip() {
    stop();
    remaining = 0;
  }

  // Move this rest by delta seconds.
  //
  // While the clock runs it is the deadline that moves, not the displayed
  // number: thirty seconds added has to be thirty seconds of real time, not a
  // number that starts decaying from wherever the last tick happened to leave
  // it.
  //
  // The adjustment is for THIS rest only. The next set starts over from what the
  // lift prescribes, which is deliberate — "I need longer today" and "this
  // movement needs longer" are different claims, and only the first one is
  // something a button on a countdown can know.
  function adjust(delta: number) {
    if (!running) {
      remaining = clamp(remaining + delta);
      return;
    }
    const now = Date.now();
    endsAt = Math.min(now + MAX_SECONDS * 1000, Math.max(now, endsAt + delta * 1000));
    remaining = secondsLeft();
    if (remaining === 0) {
      // Shortened to nothing. The rest is over because the lifter said so, so it
      // ends quietly — only the clock running out earns a chime.
      stop();
      return;
    }
    remember();
  }

  function toggleMute() {
    muted = !muted;
    setMuted(muted);
    // Unmuting is itself a tap, so it is a chance to open the audio device that
    // prime() was told to skip while the timer was silent.
    if (!muted) prime();
  }

  // Pick a resumed countdown back up where the last page load left it. Runs
  // once on mount, before the autoStartKey effect below can matter (that one
  // no-ops at key 0, which is where every mount starts).
  if (resumed !== null) start();

  // Reset + auto-start whenever the parent bumps autoStartKey (a set was
  // completed). Only autoStartKey is tracked; the reset/start mutations are
  // untracked so ticking state changes don't re-trigger this effect — and so
  // reset() reads the CURRENT `seconds`, which is how a rest for the lift just
  // logged replaces the one before it.
  $effect(() => {
    const key = autoStartKey;
    untrack(() => {
      if (key > 0) {
        reset();
        start();
      }
    });
  });

  // Recompute the moment the tab is looked at again. The interval keeps running
  // in the background, but throttled — so without this the first thing a lifter
  // sees on unlocking is a stale number for up to a minute, on a rest that may
  // well be over.
  $effect(() => {
    const resync = () => {
      if (running) tick();
    };
    document.addEventListener("visibilitychange", resync);
    return () => document.removeEventListener("visibilitychange", resync);
  });

  // Clear the interval if the component is destroyed mid-countdown. Unmounting
  // is also how the active session stops the timer once the workout is over —
  // it drops the whole widget rather than resetting it in place. halt() rather
  // than stop(): see above, a teardown is not the lifter ending their rest.
  $effect(() => () => halt());

  const pillButton =
    "inline-flex items-center gap-1.5 rounded-full border px-3 py-1 text-xs font-semibold text-ink transition disabled:opacity-40";
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

  <!-- Two rows: the clock's own controls on top, what to do with the rest
       below. Six buttons in one line does not fit a phone held one-handed. -->
  <div class="flex flex-col gap-1.5">
    <div class="flex gap-1.5">
      <button
        class="{pillButton} border-cyan/60 bg-cyan/10 tabular-nums hover:bg-cyan/25"
        onclick={() => adjust(-STEP)}
        aria-label="Subtract 30 seconds"
      >
        −30
      </button>
      <button
        class="{pillButton} border-cyan/60 bg-cyan/10 tabular-nums hover:bg-cyan/25"
        onclick={() => adjust(STEP)}
        aria-label="Add 30 seconds"
      >
        +30
      </button>
      <button
        class="{pillButton} border-border/60 bg-transparent px-2 hover:bg-muted/40"
        onclick={toggleMute}
        aria-pressed={muted}
        aria-label={muted ? "Unmute the rest alert" : "Mute the rest alert"}
      >
        {#if muted}
          <BellOff class="size-3.5" aria-hidden="true" />
        {:else}
          <Bell class="size-3.5" aria-hidden="true" />
        {/if}
      </button>
    </div>
    <div class="flex gap-1.5">
      <button
        class="{pillButton} border-neon/60 bg-neon/10 hover:bg-neon/25"
        onclick={start}
        disabled={running || remaining === 0}
      >
        <Play class="size-3" aria-hidden="true" />
        Start
      </button>
      <button
        class="{pillButton} border-magenta/60 bg-magenta/10 hover:bg-magenta/25"
        onclick={reset}
      >
        <RotateCcw class="size-3" aria-hidden="true" />
        Reset
      </button>
      <button
        class="{pillButton} border-border/60 bg-transparent hover:bg-muted/40"
        onclick={skip}
        disabled={remaining === 0}
      >
        <SkipForward class="size-3" aria-hidden="true" />
        Skip
      </button>
    </div>
  </div>
</div>
