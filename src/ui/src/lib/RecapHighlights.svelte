<script lang="ts">
  import type { RackedMilestone } from "./api";
  import { Card } from "$lib/components/ui/card";
  import { Badge } from "$lib/components/ui/badge";
  import Trophy from "@lucide/svelte/icons/trophy";
  import Flame from "@lucide/svelte/icons/flame";
  import { formatVolume } from "./volume";
  import type { RecapPRRow } from "./recap";
  import { STREAK_DISPLAY_THRESHOLD } from "./streak";

  // Records, milestones and streaks — the part worth reading first, so it sits
  // above the per-lift table on the page.

  let {
    prs,
    milestones = [],
    streakSessions = 0,
    streakWeeks = 0,
  }: {
    prs: RecapPRRow[];
    /** Empty offline: a milestone is a claim about a lifetime, and that needs the server. */
    milestones?: RackedMilestone[];
    streakSessions?: number;
    streakWeeks?: number;
  } = $props();

  // The same threshold Home uses. Below it there is no run to speak of, and a
  // "1 session streak" on somebody's first workout back reads as mockery.
  const showStreak = $derived(streakSessions >= STREAK_DISPLAY_THRESHOLD);

  // The week run is shown only when the session run is NOT, and only once it is
  // long enough to mean something. The two measure different things — one
  // precision, the other showing up — but stacked together they read as two
  // ways of flattering the same fact. This way a lifter who missed a rep last
  // Thursday still gets credit for the eight weeks they turned up.
  const showWeeks = $derived(!showStreak && streakWeeks >= STREAK_DISPLAY_THRESHOLD);
</script>

{#if prs.length > 0 || milestones.length > 0 || showStreak || showWeeks}
  <Card class="p-4" data-testid="recap-highlights">
    <h3 class="text-xs uppercase tracking-[0.2em] text-muted-foreground">Worth noting</h3>
    <ul class="mt-2 space-y-2">
      {#each prs as pr (pr.exerciseName + pr.kind)}
        <li class="flex items-center gap-2 text-sm">
          <Trophy class="size-4 shrink-0 text-primary" aria-hidden="true" />
          <span class="font-bold text-foreground">{pr.exerciseName}</span>
          <span class="tabular-nums text-foreground">
            {formatVolume(pr.valueLb)} lb
          </span>
          <!-- The two kinds are not the same news, and a lifter who sees "PR"
               against a bar that did not move will not trust the next one. -->
          <Badge variant={pr.kind === "weight" ? "default" : "secondary"}>
            {pr.kind === "weight" ? "PR" : "est. max"}
          </Badge>
          {#if pr.previousLb > 0}
            <span class="ml-auto text-xs tabular-nums text-muted-foreground">
              was {formatVolume(pr.previousLb)}
            </span>
          {/if}
        </li>
      {/each}

      {#each milestones as milestone (milestone.kind + milestone.label)}
        <li class="flex items-center gap-2 text-sm">
          <Trophy class="size-4 shrink-0 text-sun" aria-hidden="true" />
          <!-- The label arrives as a finished sentence from the server, which is
               what keeps the page and the recap email wording it identically. -->
          <span class="text-foreground">{milestone.label}</span>
        </li>
      {/each}

      {#if showStreak}
        <li class="flex items-center gap-2 text-sm">
          <Flame class="size-4 shrink-0 text-primary" aria-hidden="true" />
          <span class="text-foreground">
            <span class="font-bold tabular-nums">{streakSessions}</span> sessions in a row, every
            set logged
          </span>
        </li>
      {:else if showWeeks}
        <li class="flex items-center gap-2 text-sm">
          <Flame class="size-4 shrink-0 text-primary" aria-hidden="true" />
          <span class="text-foreground">
            <span class="font-bold tabular-nums">{streakWeeks}</span> weeks trained in a row
          </span>
        </li>
      {/if}
    </ul>
  </Card>
{/if}
