<script lang="ts">
  import type { LifterLevel } from "./api";

  // What a level looks like in a hover card: the level itself, and how far into it
  // the lifter is.
  //
  // Built entirely from the list the client already holds, so opening this costs no
  // request — HouseCard's arrangement and for its reason.
  //
  // NOTHING HERE COMPUTES THE CURVE. Every number below is a field on the wire.
  // The temptation is to derive "XP to go" or the percentage from a session count,
  // and doing it would put a second copy of the curve in the browser to disagree
  // with src/api/internal/levels the first time either changed.
  let { level }: { level: LifterLevel } = $props();

  const remaining = $derived(level.xpForNextLevel - level.xpIntoLevel);
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

  <!-- aria-hidden with the figures spelled out below it: a bar announced as a
       progressbar gives a screen reader a percentage, and the sentence under it
       says the same thing in the units the lifter actually earns. -->
  <div
    class="h-1.5 w-full overflow-hidden rounded-full bg-primary/15"
    aria-hidden="true"
  >
    <div class="h-full rounded-full bg-primary" style:width="{percent}%"></div>
  </div>

  <p class="text-sm text-muted-foreground">
    {grouped(level.xpIntoLevel)} / {grouped(level.xpForNextLevel)} XP —
    {grouped(remaining)} to Level {level.level + 1}
  </p>
</div>
