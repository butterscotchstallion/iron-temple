<script lang="ts">
  import { onMount } from "svelte";
  import { listSessions } from "../lib/api";
  import { currentStreak, STREAK_DISPLAY_THRESHOLD } from "../lib/streak";
  import ProgramDetail from "./ProgramDetail.svelte";
  import Programs from "./Programs.svelte";
  import { Card } from "$lib/components/ui/card";

  // The "normal flow" skips choosing a program: land on the most recently used
  // program's workout. First-time users (no history) get the picker instead.
  let currentProgramId = $state<number | null>(null);
  let streak = $state(0);
  let loading = $state(true);

  async function load() {
    loading = true;
    const { data } = await listSessions({ query: { limit: 100 } });
    if (data && data.items.length > 0) {
      currentProgramId = data.items[0].programId;
      streak = currentStreak(data.items);
    } else {
      currentProgramId = null;
      streak = 0;
    }
    loading = false;
  }

  onMount(load);
</script>

<div class="flex flex-col gap-6">
  {#if streak >= STREAK_DISPLAY_THRESHOLD}
    <Card class="border-primary/40 bg-primary/5 p-6 text-center ring-primary/30">
      <p class="text-2xl font-black text-primary">🔥 {streak}-session streak</p>
      <p class="mt-0.5 text-xs uppercase tracking-[0.3em] text-muted-foreground">
        Finish every set to keep it alive
      </p>
    </Card>
  {/if}

  {#if loading}
    <Card class="h-40 animate-pulse"></Card>
  {:else if currentProgramId != null}
    <ProgramDetail params={{ id: String(currentProgramId) }} />
  {:else}
    <Programs />
  {/if}
</div>
