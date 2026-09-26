<script lang="ts">
  import type { LifterLevel } from "./api";

  // What a level looks like written out: the level itself, and how far into it the
  // lifter is.
  //
  // Two callers, and they are the reason this is a component rather than markup
  // inside LevelBadge — the header's hover card, and the Experience section of a
  // lifter's profile, which is the one place these figures get a heading and room
  // to breathe. Width is the caller's business; everything here lays out against
  // whatever it is given.
  //
  // Built entirely from the list the client already holds, so drawing this costs no
  // request — HouseCard's arrangement and for its reason.
  //
  // NOTHING HERE COMPUTES THE CURVE. Every number below is a field on the wire.
  // The temptation is to work the percentage below out from a session count, and
  // doing it would put a second copy of the curve in the browser to disagree with
  // src/api/internal/levels the first time either changed. A ratio of two figures
  // the server sent is not that: it knows what a level costs only because it was
  // told.
  let { level }: { level: LifterLevel } = $props();

  // ONE figure, drawn twice — as the bar's width and as the sentence under it, so
  // the two cannot say different things. It replaced an "XP to go" remainder, which
  // was the more precise number and the less useful one: what a lifter wants from a
  // glance is how far along they are, and a remainder makes them hold the level's
  // cost in their head to work that out. The exact figures are still spelled out
  // beside it for anybody who wants them.
  //
  // Rounded, and the fraction next to it is what keeps that honest: a level is
  // earned in whole sessions, so a reading like "9%" is a rounding of a real
  // fraction the same sentence shows in full.
  const percent = $derived(
    Math.round((level.xpIntoLevel / level.xpForNextLevel) * 100),
  );

  // Grouped, because lifetime XP reaches five figures on an install that has been
  // running a year and "24350" is a number nobody reads at a glance.
  const grouped = (n: number) => n.toLocaleString();
</script>

<div class="flex flex-col gap-2">
  <div class="flex items-baseline justify-between gap-2">
    <span class="font-semibold text-foreground">Level {level.level}</span>
    <span class="text-xs tracking-wide text-muted-foreground">
      {grouped(level.xp)} XP
    </span>
  </div>

  <!-- aria-hidden with the figures spelled out below it: the sentence under this
       carries the same percentage the bar is drawn at AND the XP behind it, so
       announcing the bar as a progressbar would repeat one of those and add
       nothing. It stays decoration for exactly as long as that sentence says
       everything it does. -->
  <div
    class="h-1.5 w-full overflow-hidden rounded-full bg-primary/15"
    aria-hidden="true"
  >
    <div class="h-full rounded-full bg-primary" style:width="{percent}%"></div>
  </div>

  <p class="text-sm text-muted-foreground">
    {grouped(level.xpIntoLevel)} / {grouped(level.xpForNextLevel)} XP —
    {percent}% of the way to Level {level.level + 1}
  </p>
</div>
