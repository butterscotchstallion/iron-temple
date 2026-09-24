<script lang="ts">
  import { onMount } from "svelte";
  import { Card } from "$lib/components/ui/card";
  import AchievementList from "../AchievementList.svelte";
  import ErrorCard from "../ErrorCard.svelte";
  import Loading from "../skeleton/Loading.svelte";
  import SkeletonRows from "../skeleton/SkeletonRows.svelte";
  import { getLifterAchievements, type LifterAchievement, type User } from "../api";

  // What the signed-in lifter has earned.
  //
  // Fetched here rather than read off achievements.svelte.ts, and the difference
  // is history: that module holds who is wearing what RIGHT NOW, which is all a
  // crown beside a name needs. This section is the other half — every reign,
  // including the ones that have ended — and only the per-lifter endpoint knows
  // about those.
  let { me }: { me: User } = $props();

  let items = $state<LifterAchievement[]>([]);
  let loading = $state(true);
  let failed = $state(false);

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
  }

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
      <AchievementList {items} you />
    {/if}
  </div>
</Card>
