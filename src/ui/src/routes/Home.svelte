<script lang="ts">
  import { onMount } from "svelte";
  import { listSessions } from "../lib/api";
  import { currentStreak, STREAK_DISPLAY_THRESHOLD } from "../lib/streak";
  import ProgramDetail from "./ProgramDetail.svelte";
  import Programs from "./Programs.svelte";
  import { Card } from "$lib/components/ui/card";
  import ErrorCard from "../lib/ErrorCard.svelte";
  import CalendarHeatmap from "../lib/CalendarHeatmap.svelte";

  // The "normal flow" skips choosing a program: land on the most recently used
  // program's workout. First-time users (no history) get the picker instead.
  let currentProgramId = $state<number | null>(null);
  let streak = $state(0);
  let sessions = $state<{ performedOn: string; day: string }[]>([]);
  let loading = $state(true);
  let failed = $state(false);

  async function load() {
    loading = true;
    failed = false;
    const { data, error } = await listSessions({ query: { limit: 100 } });
    if (error) {
      failed = true;
      loading = false;
      return;
    }
    if (data && data.items.length > 0) {
      currentProgramId = data.items[0].programId;
      streak = currentStreak(data.items);
      sessions = data.items.map((s) => ({
        performedOn: s.performedOn,
        day: s.programDayName.replace(/^workout\s+/i, ""),
      }));
    } else {
      currentProgramId = null;
      streak = 0;
      sessions = [];
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
          <p class="text-2xl font-black text-primary">🔥 {streak}-session streak</p>
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
