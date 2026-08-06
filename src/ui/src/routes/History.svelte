<script lang="ts">
  import { onMount } from "svelte";
  import { link } from "svelte-spa-router";
  import { listSessions, type SessionSummary } from "../lib/api";
  import { Card } from "$lib/components/ui/card";
  import { Button } from "$lib/components/ui/button";
  import { Badge } from "$lib/components/ui/badge";

  const pageSize = 20;

  let sessions = $state<SessionSummary[]>([]);
  let total = $state(0);
  let loading = $state(true);
  let failed = $state(false);
  let loadingMore = $state(false);

  const hasMore = $derived(sessions.length < total);

  function isComplete(s: SessionSummary): boolean {
    return s.setCount > 0 && s.completedSetCount === s.setCount;
  }

  async function loadInitial() {
    loading = true;
    failed = false;
    const { data, error } = await listSessions({
      query: { limit: pageSize, offset: 0 },
    });
    if (error || !data) {
      failed = true;
    } else {
      sessions = data.items;
      total = data.total;
    }
    loading = false;
  }

  async function loadMore() {
    loadingMore = true;
    const { data } = await listSessions({
      query: { limit: pageSize, offset: sessions.length },
    });
    if (data) {
      sessions = [...sessions, ...data.items];
      total = data.total;
    }
    loadingMore = false;
  }

  onMount(loadInitial);
</script>

<div class="flex flex-col gap-6">
  <h2 class="text-2xl font-black text-foreground">History</h2>

  {#if loading}
    <Card class="h-40 animate-pulse"></Card>
  {:else if failed}
    <Card class="p-6 text-center" role="alert">
      <p class="text-sm text-muted-foreground">Couldn't load your history.</p>
      <Button variant="outline" class="mt-3" onclick={loadInitial}>Retry</Button>
    </Card>
  {:else if sessions.length === 0}
    <Card class="p-6 text-center">
      <p class="text-sm text-muted-foreground">
        No sessions logged yet. Start a workout to see it here.
      </p>
    </Card>
  {:else}
    <ul class="flex flex-col gap-3">
      {#each sessions as session (session.id)}
        <li>
          <a use:link href="/sessions/{session.id}" class="group block">
            <Card
              class="flex flex-row items-center justify-between gap-4 p-4 transition group-hover:ring-primary/60"
            >
              <div>
                <p class="font-bold text-card-foreground">{session.programName}</p>
                <p class="mt-0.5 text-sm text-muted-foreground">
                  {session.programDayName} · {session.performedOn}
                </p>
              </div>
              <div class="flex flex-col items-end gap-1">
                <p class="text-sm tabular-nums text-muted-foreground">
                  {session.completedSetCount}/{session.setCount} sets
                </p>
                {#if isComplete(session)}
                  <Badge variant="secondary">✓ Complete</Badge>
                {/if}
              </div>
            </Card>
          </a>
        </li>
      {/each}
    </ul>

    {#if hasMore}
      <Button
        variant="outline"
        class="self-center"
        onclick={loadMore}
        disabled={loadingMore}
      >
        {loadingMore ? "Loading…" : "Load more"}
      </Button>
    {/if}
  {/if}
</div>
