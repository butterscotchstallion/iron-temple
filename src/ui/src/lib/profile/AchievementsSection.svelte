<script lang="ts">
  import { onMount } from "svelte";
  import { Card } from "$lib/components/ui/card";
  import AchievementList from "../AchievementList.svelte";
  import ErrorCard from "../ErrorCard.svelte";
  import ShareCardDialog from "../ShareCardDialog.svelte";
  import Loading from "../skeleton/Loading.svelte";
  import SkeletonRows from "../skeleton/SkeletonRows.svelte";
  import { cachedValue, fetchThrough, rackedKey } from "../cache.svelte";
  import {
    achievementShareCardContent,
    achievementShareCardFilename,
  } from "../achievementShareCard";
  import {
    getLifterAchievements,
    getRacked,
    type LifterAchievement,
    type RackedReport,
    type RackedUpcomingMilestone,
    type User,
  } from "../api";

  // What the signed-in lifter has earned, what they are closing in on, and a card
  // to show either off with.
  //
  // The achievements are fetched here rather than read off achievements.svelte.ts,
  // and the difference is history: that module holds who is wearing what RIGHT NOW,
  // which is all a crown beside a name needs. This section is the other half —
  // every reign, including the ones that have ended — and only the per-lifter
  // endpoint knows about those.
  let { me }: { me: User } = $props();

  let items = $state<LifterAchievement[]>([]);
  let upcoming = $state<RackedUpcomingMilestone[]>([]);
  let loading = $state(true);
  let failed = $state(false);

  // Which crown the share dialog is for, and whether it is showing. Two pieces
  // rather than one nullable: ShareCardDialog owns its own `open` and writes to it
  // (a completed share closes it), so the flag has to be bindable while the payload
  // stays put — it is what the mounted-but-closed dialog is still holding content
  // for.
  let sharing = $state<LifterAchievement | null>(null);
  let shareOpen = $state(false);

  async function load() {
    failed = false;
    loading = true;
    const result = await getLifterAchievements(me.id);
    if (result.status !== 200) {
      failed = true;
    } else {
      items = result.data.items;
    }
    loading = false;

    // The chase, after the list rather than alongside it. Two reasons: this is the
    // expensive call of the two — a whole Racked report — and it is the optional
    // half, so a failure here leaves the earned list standing rather than turning
    // the section into an error.
    //
    // Through the cache on the same key the Racked page uses, so a lifter who has
    // looked at that page this session pays nothing, and a workout ending drops it
    // for both — invalidateTraining already clears every racked: key.
    const key = rackedKey("month");
    const remembered = cachedValue<RackedReport>(key);
    if (remembered) upcoming = remembered.upcomingMilestones;

    const report = await fetchThrough(key, () => getRacked({ period: "month" }));
    if (report.status === 200) upcoming = report.data.upcomingMilestones;
  }

  // The nearest thing they are chasing rides along on the card. Null when there is
  // nothing — a lifter past every rung gets a card without that row rather than one
  // claiming a goal.
  const nearest = $derived<RackedUpcomingMilestone | null>(upcoming[0] ?? null);

  const shareContent = $derived(
    sharing
      ? achievementShareCardContent(sharing, me.displayName || me.username, nearest)
      : null,
  );

  onMount(load);
</script>

<Card class="p-6">
  <h3 class="text-lg font-bold text-card-foreground">Achievements</h3>
  <p class="mt-1 text-sm text-muted-foreground">
    What you've earned on this install, and what you've held before.
  </p>

  <div class="mt-4">
    {#if loading}
      <Loading label="Loading your achievements">
        <SkeletonRows rows={3} avatar="size-5" rowClass="py-3" />
      </Loading>
    {:else if failed}
      <ErrorCard message="Couldn't load your achievements." onRetry={load} />
    {:else}
      <AchievementList
        {items}
        you
        {upcoming}
        onShare={(held) => {
          sharing = held;
          shareOpen = true;
        }}
      />
    {/if}
  </div>
</Card>

{#if sharing && shareContent}
  <!-- Outside the Card, which is where every other ShareCardDialog sits relative
       to the button that opens it. The dialog re-renders its image when `content`
       changes, so switching achievements while it is open swaps the card rather
       than leaving the previous one's pixels behind. -->
  <ShareCardDialog
    bind:open={shareOpen}
    content={shareContent}
    filename={achievementShareCardFilename(sharing)}
    alt="{sharing.achievement.label} on Iron Temple"
    title={sharing.achievement.kind === "level"
      ? "Share this level"
      : "Share this crown"}
    subtitle="{sharing.achievement.label}, as an image."
    shareTitle={sharing.achievement.label}
  />
{/if}
