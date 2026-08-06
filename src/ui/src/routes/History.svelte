<script lang="ts">
  import { onMount } from "svelte";
  import { link } from "svelte-spa-router";
  import { listSessions, type SessionSummary } from "../lib/api";

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
  <h2 class="text-2xl font-black text-ink">History</h2>

  {#if loading}
    <div class="h-40 animate-pulse rounded-2xl border border-neon/20 bg-surface/50"></div>
  {:else if failed}
    <div
      class="rounded-2xl border border-magenta/40 bg-surface/60 p-6 text-center"
      role="alert"
    >
      <p class="text-sm text-ink/80">Couldn't load your history.</p>
      <button
        class="mt-3 rounded-full border border-cyan/60 bg-cyan/10 px-5 py-2 font-semibold text-ink transition hover:bg-cyan/25"
        onclick={loadInitial}
      >
        Retry
      </button>
    </div>
  {:else if sessions.length === 0}
    <p class="rounded-2xl border border-neon/20 bg-surface/50 p-6 text-center text-sm text-ink/60">
      No sessions logged yet. Start a workout to see it here.
    </p>
  {:else}
    <ul class="flex flex-col gap-3">
      {#each sessions as session (session.id)}
        <li>
          <a
            href="/sessions/{session.id}"
            use:link
            class="flex items-center justify-between gap-4 rounded-2xl border border-neon/30 bg-surface/70 p-4 backdrop-blur transition hover:border-neon/70"
          >
            <div>
              <p class="font-bold text-ink">{session.programName}</p>
              <p class="mt-0.5 text-sm text-ink/70">
                {session.programDayName} · {session.performedOn}
              </p>
            </div>
            <div class="text-right">
              <p class="text-sm tabular-nums text-ink/70">
                {session.completedSetCount}/{session.setCount} sets
              </p>
              {#if isComplete(session)}
                <span class="text-xs font-semibold uppercase tracking-wide text-cyan">
                  ✓ Complete
                </span>
              {/if}
            </div>
          </a>
        </li>
      {/each}
    </ul>

    {#if hasMore}
      <button
        class="self-center rounded-full border border-neon/60 bg-neon/10 px-6 py-2 text-sm font-semibold text-ink transition hover:bg-neon/25 disabled:opacity-40"
        onclick={loadMore}
        disabled={loadingMore}
      >
        {loadingMore ? "Loading…" : "Load more"}
      </button>
    {/if}
  {/if}
</div>
