<script lang="ts">
  import ChevronsUp from "@lucide/svelte/icons/chevrons-up";
  import Crown from "@lucide/svelte/icons/crown";
  import Share2 from "@lucide/svelte/icons/share-2";
  import Timestamp from "./Timestamp.svelte";
  import { formatRemaining } from "./achievementShareCard";
  import { barFraction } from "./racked";
  import type { LifterAchievement, RackedUpcomingMilestone } from "./api";

  // One lifter's achievements, as a list.
  //
  // Takes items rather than fetching them, for FeedList's reason: two surfaces
  // draw the same rows — your own profile section and somebody else's profile —
  // and they differ only in whose id they ask about and what they say when there
  // is nothing.
  //
  // `you` is what makes the empty state and the prose read correctly in both.
  // Second person to yourself and third person about anybody else is the
  // difference between "you haven't won anything yet" and a sentence about a
  // person who is not reading it.
  //
  // `upcoming` and `onShare` are the halves that only make sense about yourself.
  // What somebody else is closing in on is their business, and sharing their crown
  // is not the reader's to do — so both are absent on another lifter's profile and
  // the component draws neither.
  let {
    items,
    you = false,
    name = "",
    upcoming = [],
    onShare,
  }: {
    items: LifterAchievement[];
    you?: boolean;
    name?: string;
    upcoming?: RackedUpcomingMilestone[];
    onShare?: (held: LifterAchievement) => void;
  } = $props();

  /**
   * The reign count, which is only mentioned past one. "Held once" is what every
   * first win reads as, and it adds nothing to a line that already says when.
   *
   * The rest of that line is markup rather than a string, because the date in it
   * is a <Timestamp> and a component cannot be interpolated into a template
   * literal. The two halves of the wire contract are still used for what they
   * are for — a current holder is described by when this reign started, a lapsed
   * one by when they last had it — and it still branches on `heldNow` rather
   * than probing the dates, which is what that field is there for.
   */
  function again(item: LifterAchievement): string {
    return item.timesHeld > 1 ? ` · held ${item.timesHeld} times` : "";
  }

  /**
   * Which icon a row wears, by kind.
   *
   * A map rather than a ternary because the catalogue is meant to hold more than
   * two, and HouseIcon already establishes the enum→icon shape in this codebase.
   * Falls back to the crown for a kind this build has never heard of: the server
   * may be newer than the tab, and a row with no icon reads as a broken image
   * where a slightly wrong one still reads as an achievement.
   */
  const ICONS = { crown: Crown, level: ChevronsUp } as const;
  function iconFor(kind: string) {
    return ICONS[kind as keyof typeof ICONS] ?? Crown;
  }

  /**
   * A LEVEL RUNG IS NEVER LOST, and almost every word on these rows assumed
   * otherwise. "Holding", "Holding it since", "Last held", "held 3 times" all
   * describe a standing that can change hands — which is exactly what a crown is
   * and exactly what a crossing is not. Saying "Holding Level 5" would imply
   * somebody could take it.
   *
   * So the wording branches on kind rather than on heldNow for the level case,
   * where heldNow is true forever and tells a reader nothing.
   */
  function pill(item: LifterAchievement): string | null {
    if (item.achievement.kind === "level") return "Reached";
    return item.heldNow ? "Holding" : null;
  }

  function when(item: LifterAchievement): string {
    if (item.achievement.kind === "level") return "Reached";
    return item.heldNow ? "Holding it since" : "Last held";
  }
</script>

{#if items.length === 0}
  <p class="py-4 text-center text-sm text-muted-foreground">
    {#if you}
      Nothing yet. Keep training for your first level, or lead any board on the
      leaderboard and its crown is yours.
    {:else}
      {name || "This lifter"} hasn't earned anything yet.
    {/if}
  </p>
{:else}
  <ul class="flex flex-col divide-y divide-border/60">
    {#each items as item (item.achievement.slug)}
      <!-- Hoisted out of the <li>: {@const} has to be an immediate child of the
           block that introduces it. -->
      {@const Icon = iconFor(item.achievement.kind)}
      <li class="flex items-center gap-3 py-3">
        <!-- Dimmed rather than dropped for a lapsed reign. Having held a crown
             is the thing this list is for; showing only what somebody holds
             right now would make the section empty for everybody the day
             after they were overtaken.

             A level rung is never lapsed, so it is never the dimmed one — which
             falls out of heldNow rather than needing its own branch. -->
        <Icon
          class="size-5 shrink-0 {item.heldNow
            ? 'text-primary'
            : 'text-muted-foreground/50'}"
          aria-hidden="true"
        />
        <div class="min-w-0 flex-1">
          <p class="flex flex-wrap items-baseline gap-x-2">
            <span class="text-sm font-semibold text-foreground">
              {item.achievement.label}
            </span>
            {#if pill(item)}
              <span
                class="rounded-full bg-primary/15 px-2 py-0.5 text-xs font-semibold uppercase tracking-[0.15em] text-primary"
              >
                {pill(item)}
              </span>
            {/if}
          </p>
          <p class="text-xs text-muted-foreground">
            {when(item)}
            <Timestamp value={item.lastHeldFrom} />{again(item)}
          </p>
          <p class="mt-1 text-xs text-muted-foreground/80">
            {item.achievement.description}
          </p>
        </div>
        <!-- Only on a crown they still hold, and only on their own profile. A
             card reading "I used to be top of this" is not a brag, and sharing
             somebody else's is not the reader's to do. -->
        {#if onShare && item.heldNow}
          <button
            type="button"
            class="shrink-0 rounded-full p-2 text-muted-foreground transition hover:bg-white/5 hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary"
            aria-label="Share {item.achievement.label}"
            onclick={() => onShare(item)}
          >
            <Share2 class="size-4" aria-hidden="true" />
          </button>
        {/if}
      </li>
    {/each}
  </ul>
{/if}

<!-- What is still ahead. Below what has been earned, because the list above is
     what the section was opened for — and absent entirely when there is nothing
     to chase, the same call RecapHighlights makes rather than drawing an empty
     congratulation. -->
{#if upcoming.length > 0}
  <div class="mt-4 border-t border-border/60 pt-4">
    <h4 class="text-xs font-semibold uppercase tracking-[0.2em] text-muted-foreground">
      Closing in
    </h4>
    <ul class="mt-3 flex flex-col gap-3">
      {#each upcoming as next (next.label)}
        <li>
          <p class="flex items-baseline justify-between gap-2">
            <span class="truncate text-sm text-foreground">{next.label}</span>
            <span class="shrink-0 text-xs font-semibold tabular-nums text-primary">
              {formatRemaining(next)}
            </span>
          </p>
          <!-- aria-hidden: the bar is a picture of the figure beside it, and a
               screen reader reading both says the same thing twice. The same
               call every other bar in this app makes. -->
          <div
            class="mt-1.5 h-1.5 overflow-hidden rounded-full bg-white/10"
            aria-hidden="true"
          >
            <div
              class="h-full rounded-full bg-primary"
              style="width:{barFraction(next.currentLb, next.targetLb) * 100}%"
            ></div>
          </div>
        </li>
      {/each}
    </ul>
  </div>
{/if}
