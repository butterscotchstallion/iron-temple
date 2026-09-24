<script lang="ts">
  import Crown from "@lucide/svelte/icons/crown";
  import { formatLongDate } from "./date";
  import type { LifterAchievement } from "./api";

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
  let {
    items,
    you = false,
    name = "",
  }: { items: LifterAchievement[]; you?: boolean; name?: string } = $props();

  /**
   * What an entry says under its label.
   *
   * The two halves of the wire contract are used for what they are for: a
   * current holder is described by when this reign started, and a lapsed one by
   * when they last had it. Picking on `heldNow` rather than probing the dates is
   * what the field is there for.
   *
   * The reign count is only mentioned past one. "Held once" is what every first
   * win reads as and it adds nothing to a line that already says when.
   */
  function detail(item: LifterAchievement): string {
    const since = formatLongDate(item.lastHeldFrom);
    const again =
      item.timesHeld > 1 ? ` · held ${item.timesHeld} times` : "";
    return item.heldNow
      ? `Holding it since ${since}${again}`
      : `Last held ${since}${again}`;
  }
</script>

{#if items.length === 0}
  <p class="py-4 text-center text-sm text-muted-foreground">
    {#if you}
      Nothing yet. Lead any board on the leaderboard and its crown is yours.
    {:else}
      {name || "This lifter"} hasn't earned anything yet.
    {/if}
  </p>
{:else}
  <ul class="flex flex-col divide-y divide-border/60">
    {#each items as item (item.achievement.slug)}
      <li class="flex items-center gap-3 py-3">
        <!-- Dimmed rather than dropped for a lapsed reign. Having held a crown
             is the thing this list is for; showing only what somebody holds
             right now would make the section empty for everybody the day
             after they were overtaken. -->
        <Crown
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
            {#if item.heldNow}
              <span
                class="rounded-full bg-primary/15 px-2 py-0.5 text-xs font-semibold uppercase tracking-[0.15em] text-primary"
              >
                Holding
              </span>
            {/if}
          </p>
          <p class="text-xs text-muted-foreground">{detail(item)}</p>
          <p class="mt-1 text-xs text-muted-foreground/80">
            {item.achievement.description}
          </p>
        </div>
      </li>
    {/each}
  </ul>
{/if}
