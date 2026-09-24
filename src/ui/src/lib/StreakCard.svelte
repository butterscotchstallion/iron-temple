<script lang="ts">
  // The streak banner at the top of the home screen, which gets hotter the
  // longer the streak runs and catches fire at six sessions. See streakHeat.ts
  // for the ladder and why each rung is where it is.
  //
  // Renders nothing below the display threshold — "1 session streak" on
  // somebody's first workout back reads as mockery — so the caller does not have
  // to guard.

  import * as AlertDialog from "$lib/components/ui/alert-dialog";
  import { Card } from "$lib/components/ui/card";
  import CircleQuestionMark from "@lucide/svelte/icons/circle-question-mark";
  import Flame from "@lucide/svelte/icons/flame";
  import { prefersReducedMotion } from "./reducedMotion";
  import { STREAK_DISPLAY_THRESHOLD } from "./streak";
  import { combinedHeat, heatColor, streakHeat } from "./streakHeat";

  let { streak }: { streak: number } = $props();

  const heat = $derived(streakHeat(streak));
  const h = $derived(combinedHeat(heat));

  // The preference is a Control Centre toggle people reach for mid-session, and
  // unlike every other caller of prefersReducedMotion() this card is long-lived:
  // it stays mounted for as long as the lifter is on the home screen. So it
  // subscribes rather than asks.
  //
  // `$derived(prefersReducedMotion())` would look equivalent and be wrong — it
  // reads no reactive state, so Svelte computes it at init and memoizes it
  // forever, leaving the flames animating after someone has asked for stillness.
  // Seeded from the current value so the first paint is right, then kept in step
  // for as long as the card lives.
  let still = $state(prefersReducedMotion());

  $effect(() => {
    if (typeof window === "undefined" || typeof window.matchMedia !== "function") return;
    const query = window.matchMedia("(prefers-reduced-motion: reduce)");
    const sync = () => (still = query.matches);
    sync();
    query.addEventListener("change", sync);
    return () => query.removeEventListener("change", sync);
  });

  /** The heat colour at `pct` opacity. */
  const tint = (pct: number) =>
    `color-mix(in srgb, var(--heat-color) ${Math.round(pct)}%, transparent)`;

  const round = (n: number) => Math.round(n * 100) / 100;

  // A burning card still has to be a legible card. The edge and the wash climb
  // with heat; the text colour is the ramp itself, which is what carries the
  // purple -> magenta -> fire shift.
  const edge = $derived(tint(45 + 35 * h));
  const wash = $derived(tint(5 + 12 * h));

  // Card's base ring is a box-shadow, so setting this replaces it rather than
  // stacking with it — hence the explicit 1px ring in the first slot.
  const shadow = $derived(
    [
      `0 0 0 1px ${edge}`,
      `0 0 ${round(16 + 42 * h)}px ${tint(16 + 30 * h)}`,
      `0 ${round(6 + 10 * h)}px ${round(24 + 40 * h)}px -8px ${tint(18 + 34 * h)}`,
      // Embers banked against the inside of the bottom edge, under the flames.
      heat.ablaze
        ? `inset 0 -${round(18 + 30 * heat.blaze)}px ${round(30 + 40 * heat.blaze)}px -20px ${tint(30 + 40 * heat.blaze)}`
        : null,
    ]
      .filter(Boolean)
      .join(", "),
  );

  // Flames climb the card as the streak does, but stop well short of the text:
  // at full blaze they reach about two thirds and the copy still has to read.
  const flameHeight = $derived(round(38 + 30 * heat.blaze));
  const flameOpacity = $derived(round(0.5 + 0.45 * heat.blaze));

  // Everything burns faster at the top of the ladder. Applied inline because the
  // --animate-* tokens in app.css are static and this is a continuous ramp.
  const speed = $derived(1 - 0.35 * heat.blaze);
  const duration = (base: number) => `${round(base * speed)}s`;

  // A steady bed of heat along the bottom edge, and three tongues over it. The
  // tongues sit at uneven x positions on purpose — evenly spaced ones read as a
  // repeating pattern rather than as fire.
  const bed = $derived(
    `linear-gradient(to top, ${tint(70)} 0%, ${tint(28)} 45%, transparent 100%)`,
  );
  const tongues = $derived(
    [
      `radial-gradient(58% 100% at 19% 100%, ${tint(72)} 0%, transparent 70%)`,
      `radial-gradient(44% 125% at 51% 100%, color-mix(in srgb, var(--heat-color) 78%, white) 0%, transparent 72%)`,
      `radial-gradient(52% 88% at 83% 100%, ${tint(66)} 0%, transparent 70%)`,
    ].join(", "),
  );
</script>

{#if streak >= STREAK_DISPLAY_THRESHOLD}
  <AlertDialog.Root>
    <Card
      class="relative flex flex-col justify-center p-6 text-center"
      style="--heat-color: {heatColor(h)}; --glow: {round(heat.glow)}; --blaze: {round(
        heat.blaze,
      )}; background-color: {wash}; box-shadow: {shadow};"
      data-testid="streak-card"
    >
      {#if heat.ablaze}
        <!-- Painted before the copy so the copy stays on top: positioned elements
             paint in DOM order, and the text below is `relative` for that reason.
             Clipped to the rounded corners by Card's own overflow-hidden. -->
        <div
          class="pointer-events-none absolute inset-x-0 bottom-0"
          style="height: {flameHeight}%; opacity: {flameOpacity};"
          aria-hidden="true"
          data-testid="streak-flames"
        >
          <div
            class="absolute inset-0 origin-bottom {still ? '' : 'animate-flame-lick'}"
            style="background: {bed}; animation-duration: {duration(3.7)};"
          ></div>
          <div
            class="absolute inset-0 origin-bottom {still ? '' : 'animate-flame-flicker'}"
            style="background: {tongues}; animation-duration: {duration(2.3)};"
          ></div>
        </div>
      {/if}

      <!-- After the flames in DOM order so it paints over them, and absolutely
           positioned so it sits in the card's own padding rather than pushing the
           centred count off centre. Deliberately quiet: the streak is the reward,
           this is a footnote, and it only brightens when you go looking for it. -->
      <AlertDialog.Trigger
        class="absolute right-2 top-2 rounded-full p-1.5 text-muted-foreground/60
               transition-colors hover:text-foreground focus-visible:outline-none
               focus-visible:ring-2 focus-visible:ring-ring"
        aria-label="What counts toward a streak?"
      >
        <CircleQuestionMark class="size-4" aria-hidden="true" />
      </AlertDialog.Trigger>

      <div class="relative">
        <p
          class="flex items-center justify-center gap-2 text-2xl font-black"
          style="color: var(--heat-color); text-shadow: 0 0 {round(
            8 + 22 * h,
          )}px {tint(20 + 45 * h)};"
        >
          <Flame
            class="size-6 {heat.ablaze
              ? still
                ? 'scale-110'
                : 'animate-ember-pulse'
              : ''}"
            style={heat.ablaze && !still ? `animation-duration: ${duration(2.9)};` : ""}
            aria-hidden="true"
          />
          {streak}-session streak
        </p>
        <p class="mt-0.5 text-xs uppercase tracking-[0.3em] text-muted-foreground">
          Finish every set to keep it alive
        </p>
      </div>
    </Card>

    <!-- The two things a lifter guesses wrong: that a streak is a run of days,
         and that training off the program's schedule breaks it. Neither is true
         of this count, so say both rather than restating the subtitle. -->
    <AlertDialog.Content class="sm:max-w-md">
      <AlertDialog.Header>
        <AlertDialog.Title>What counts toward a streak</AlertDialog.Title>
        <AlertDialog.Description>
          Sessions, not days. Every workout where you complete each prescribed set
          adds one, and the run ends the first time a set comes up short.
          <br /><br />
          The schedule doesn't come into it. Training on a different day than the
          program lays out, or fitting a week's workouts in whenever you can, keeps
          the streak going all the same. Racked counts the calendar side separately,
          as consecutive weeks with at least one session.
        </AlertDialog.Description>
      </AlertDialog.Header>
      <AlertDialog.Footer>
        <AlertDialog.Cancel>Got it</AlertDialog.Cancel>
      </AlertDialog.Footer>
    </AlertDialog.Content>
  </AlertDialog.Root>
{/if}
