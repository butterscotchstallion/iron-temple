<script lang="ts">
  import { onMount } from "svelte";
  import { link } from "svelte-spa-router";
  import { Card } from "$lib/components/ui/card";
  import FeedList from "./FeedList.svelte";
  import { getFeed, type FeedEntry } from "./api";

  // What the others have been up to, at the foot of Home.
  //
  // RENDERS NOTHING AT ALL when the feed is empty, and that is the feature rather
  // than a tidy-up. This app is built for one lifter; on that install the feed is
  // empty by construction — the endpoint excludes the caller, so a lone lifter's
  // feed has no rows to have — and Home must look exactly as it always did. No
  // heading, no empty state, no "nobody else has trained". The card appears the
  // day a second account logs a rep and not before.
  //
  // Which is also why it loads itself rather than being fed by Home. Home would
  // otherwise have to hold state for a section that usually does not exist, and
  // decide whether to draw it — and a failed request there would be indentation
  // in the route that owns the workout.
  //
  // A failure is silence for the same reason. The workout is why the page was
  // opened; an error card about other people's sessions underneath it would be
  // louder than the thing it failed to fetch.
  let items = $state<FeedEntry[]>([]);

  // Four rows: enough to read as recent activity, short enough not to push the
  // workout off a phone screen. The full list is a tap away.
  const PREVIEW = 4;

  onMount(async () => {
    const result = await getFeed({ limit: PREVIEW });
    if (result.status === 200) {
      items = result.data.items;
    }
  });
</script>

{#if items.length > 0}
  <Card class="p-4">
    <div class="mb-1 flex items-baseline justify-between gap-3">
      <h3 class="text-xs font-semibold uppercase tracking-[0.2em] text-muted-foreground">
        Around the gym
      </h3>
      <!-- Only worth offering once there is more than this card is showing. A
           link to a page holding the same four rows is a link to nowhere. -->
      {#if items.length === PREVIEW}
        <a
          href="/feed"
          use:link
          class="text-xs font-semibold text-primary transition hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary"
        >
          See all
        </a>
      {/if}
    </div>
    <FeedList {items} />
  </Card>
{/if}
