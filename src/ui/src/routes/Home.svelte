<script lang="ts">
  import { onMount } from "svelte";
  import { auth } from "../lib/auth.svelte";
  import { CACHE_KEYS, cachedValue, fetchThrough } from "../lib/cache.svelte";
  import { loadHomeSessions, type HomeSessions } from "../lib/homeData";
  import { currentStreak, STREAK_DISPLAY_THRESHOLD } from "../lib/streak";
  import ProgramDetail from "./ProgramDetail.svelte";
  import Programs from "./Programs.svelte";
  import { Card } from "$lib/components/ui/card";
  import Flame from "@lucide/svelte/icons/flame";
  import ErrorCard from "../lib/ErrorCard.svelte";
  import CalendarHeatmap from "../lib/CalendarHeatmap.svelte";

  // The "normal flow" skips choosing a program: land on the one the user last
  // opened. First-time users (nothing saved, no history) get the picker instead.
  //
  // The saved program wins, and history is the fallback for accounts that
  // predate it — they keep landing on their last workout's program until they
  // next open one, at which point ProgramDetail saves it.
  let lastSessionProgramId = $state<number | null>(null);
  let currentProgramId = $derived(auth.me?.currentProgramId ?? lastSessionProgramId);
  let streak = $state(0);
  let sessions = $state<{ performedOn: string; day: string }[]>([]);
  let loading = $state(true);
  let failed = $state(false);

  // Everything on this screen is derived from the session list, so applying it
  // is one step whether it came from the cache or the network.
  function apply(data: HomeSessions) {
    if (data.items.length > 0) {
      lastSessionProgramId = data.items[0].programId;
      streak = currentStreak(data.items);
      sessions = data.items.map((s) => ({
        performedOn: s.performedOn,
        day: s.programDayName.replace(/^workout\s+/i, ""),
      }));
    } else {
      lastSessionProgramId = null;
      streak = 0;
      sessions = [];
    }
  }

  async function load() {
    failed = false;

    // Paint whatever last loaded before waiting on anything. Coming back to
    // this tab used to blank the streak, the heatmap and the workout back to a
    // skeleton to re-fetch a list that had almost certainly not changed.
    const remembered = cachedValue<HomeSessions>(CACHE_KEYS.homeSessions);
    if (remembered) apply(remembered);
    loading = remembered === undefined;

    // Usually already in flight: the shell starts this at launch, alongside
    // /me, so by the time this route mounts there is a promise to join rather
    // than a request to make.
    const { data, error } = await loadHomeSessions();
    if (error || !data) {
      // A failed refresh keeps the numbers already on screen. Only a lifter
      // with nothing cached is told the load failed, because only they have
      // nothing to read.
      if (!remembered) failed = true;
    } else {
      apply(data);
    }
    loading = false;
  }

  onMount(load);
</script>

<div class="flex flex-col gap-6">
  {#if sessions.length > 0}
    <div class="flex flex-col gap-4">
      {#if streak >= STREAK_DISPLAY_THRESHOLD}
        <Card
          class="flex flex-col justify-center border-primary/40 bg-primary/5 p-6 text-center ring-primary/30"
        >
          <p
            class="flex items-center justify-center gap-2 text-2xl font-black text-primary"
          >
            <Flame class="size-6" aria-hidden="true" />
            {streak}-session streak
          </p>
          <p class="mt-0.5 text-xs uppercase tracking-[0.3em] text-muted-foreground">
            Finish every set to keep it alive
          </p>
        </Card>
      {/if}
      <Card class="p-4">
        <h3
          class="mb-0.5 text-xs font-semibold uppercase tracking-[0.2em] text-muted-foreground"
        >
          Training days
        </h3>
        <CalendarHeatmap {sessions} />
      </Card>
    </div>
  {/if}

  {#if loading}
    <Card class="h-40 animate-pulse"></Card>
  {:else if failed}
    <ErrorCard message="Couldn't load your workout." onRetry={load} />
  {:else if currentProgramId != null}
    <ProgramDetail params={{ id: String(currentProgramId) }} />
  {:else}
    <Programs />
  {/if}
</div>
