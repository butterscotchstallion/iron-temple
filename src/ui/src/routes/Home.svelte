<script lang="ts">
  import { onMount } from "svelte";
  import { listSessions } from "../lib/api";
  import { currentStreak, STREAK_DISPLAY_THRESHOLD } from "../lib/streak";
  import ProgramDetail from "./ProgramDetail.svelte";
  import Programs from "./Programs.svelte";
  import { Card } from "$lib/components/ui/card";
  import CalendarHeatmap from "../lib/CalendarHeatmap.svelte";

  // The "normal flow" skips choosing a program: land on the most recently used
  // program's workout. First-time users (no history) get the picker instead.
  let currentProgramId = $state<number | null>(null);
  let streak = $state(0);
  let sessionDates = $state<string[]>([]);
  let loading = $state(true);

  async function load() {
    loading = true;
    const { data } = await listSessions({ query: { limit: 100 } });
    if (data && data.items.length > 0) {
      currentProgramId = data.items[0].programId;
      streak = currentStreak(data.items);
      sessionDates = data.items.map((s) => s.performedOn);
    } else {
      currentProgramId = null;
      streak = 0;
      sessionDates = [];
    }
    loading = false;
  }

  onMount(load);
</script>

<div class="flex flex-col gap-6">
  {#if sessionDates.length > 0}
    <div
      class="grid gap-4 {streak >= STREAK_DISPLAY_THRESHOLD ? 'sm:grid-cols-2' : ''}"
    >
      {#if streak >= STREAK_DISPLAY_THRESHOLD}
        <Card
          class="flex flex-col justify-center border-primary/40 bg-primary/5 p-6 text-center ring-primary/30"
        >
          <p class="text-2xl font-black text-primary">🔥 {streak}-session streak</p>
          <p class="mt-0.5 text-xs uppercase tracking-[0.3em] text-muted-foreground">
            Finish every set to keep it alive
          </p>
        </Card>
      {/if}
      <Card class="p-4">
        <h3
          class="mb-3 text-xs font-semibold uppercase tracking-[0.2em] text-muted-foreground"
        >
          Training days
        </h3>
        <CalendarHeatmap dates={sessionDates} />
      </Card>
    </div>
  {/if}

  {#if loading}
    <Card class="h-40 animate-pulse"></Card>
  {:else if currentProgramId != null}
    <ProgramDetail params={{ id: String(currentProgramId) }} />
  {:else}
    <Programs />
  {/if}
</div>
