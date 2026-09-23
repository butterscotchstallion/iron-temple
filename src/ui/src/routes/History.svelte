<script lang="ts">
  import { onMount } from "svelte";
  import { link } from "svelte-spa-router";
  import { listSessions, type SessionList, type SessionSummary } from "../lib/api";
  import { CACHE_KEYS, cachedValue, fetchThrough } from "../lib/cache.svelte";
  import { formatLongDate } from "../lib/date";
  import { formatVolume } from "../lib/volume";
  import { Card } from "$lib/components/ui/card";
  import { Button } from "$lib/components/ui/button";
  import ChevronDown from "@lucide/svelte/icons/chevron-down";
  import Dumbbell from "@lucide/svelte/icons/dumbbell";
  import ErrorCard from "../lib/ErrorCard.svelte";
  import ErrorBanner from "../lib/ErrorBanner.svelte";
  import Loading from "../lib/skeleton/Loading.svelte";
  import Skeleton from "../lib/skeleton/Skeleton.svelte";

  const pageSize = 20;

  // Placeholders for the first paint. Four cards rather than a page's twenty:
  // that is roughly a phone screen, and reserving space for sixteen rows nobody
  // can see would stretch the scrollbar and then snap it back.
  const SKELETON_SESSIONS = [0, 1, 2, 3];
  // Three lifts a card, which is what a program day runs to here.
  const SKELETON_LIFTS = [0, 1, 2];

  let sessions = $state<SessionSummary[]>([]);
  let total = $state(0);
  // Every session's volume, not just the loaded page's — the API sums the whole
  // history, so paging in more sessions doesn't move this number.
  let totalVolumeLb = $state(0);
  let loading = $state(true);
  let failed = $state(false);
  let loadingMore = $state(false);
  let loadMoreFailed = $state(false);

  const hasMore = $derived(sessions.length < total);

  function apply(data: SessionList) {
    sessions = data.items;
    total = data.total;
    totalVolumeLb = data.totalVolumeLb;
  }

  // Only the first page is cached. Pages the lifter scrolled to are theirs for
  // that visit, not something to restore them into: coming back to History
  // should open at the top, which is where the first page already puts them.
  async function loadInitial() {
    failed = false;

    const remembered = cachedValue<SessionList>(CACHE_KEYS.historyFirstPage);
    if (remembered) apply(remembered);
    loading = remembered === undefined;

    const result = await fetchThrough(CACHE_KEYS.historyFirstPage, () =>
      listSessions({ limit: pageSize, offset: 0 }),
    );
    if (result.status !== 200) {
      if (!remembered) failed = true;
    } else {
      apply(result.data);
    }
    loading = false;
  }

  async function loadMore() {
    loadingMore = true;
    loadMoreFailed = false;
    const page = await listSessions({ limit: pageSize, offset: sessions.length });
    if (page.status !== 200) {
      loadMoreFailed = true;
    } else {
      sessions = [...sessions, ...page.data.items];
      total = page.data.total;
      totalVolumeLb = page.data.totalVolumeLb;
    }
    loadingMore = false;
  }

  onMount(loadInitial);
</script>

<div class="flex flex-col gap-6">
  <div>
    <h2 class="text-2xl font-black text-foreground">History</h2>
    {#if loading}
      <!-- The lifetime total is a line under the heading whether it has arrived
           or not. Left out while loading, it used to appear a beat later and
           push the whole list down by its own height. -->
      <Skeleton text="sm" class="mt-0.5 w-64" />
    {:else if !failed && sessions.length > 0}
      <p class="mt-0.5 text-sm tabular-nums text-muted-foreground">
        {formatVolume(totalVolumeLb)} lb lifted across {total}
        {total === 1 ? "session" : "sessions"}
      </p>
    {/if}
  </div>

  {#if loading}
    <Loading label="Loading your history" class="flex flex-col gap-3">
      <!-- One card per session, shaped like the real one: the program and day
           lines, the sets count opposite them, and the three-lift table that
           almost every session card carries. -->
      {#each SKELETON_SESSIONS as n (n)}
        <Card class="p-4">
          <div class="flex items-start justify-between gap-4">
            <div class="flex flex-col gap-0.5">
              <Skeleton text="sm" class="w-36" />
              <Skeleton text="sm" class="w-52" />
            </div>
            <Skeleton text="sm" class="w-16 shrink-0" />
          </div>
          <div class="mt-3 flex flex-col gap-1">
            {#each SKELETON_LIFTS as lift (lift)}
              <Skeleton text="sm" class="w-full" />
            {/each}
          </div>
        </Card>
      {/each}
    </Loading>
  {:else if failed}
    <ErrorCard message="Couldn't load your history." onRetry={loadInitial} />
  {:else if sessions.length === 0}
    <Card class="flex flex-col items-center p-6 text-center">
      <Dumbbell class="size-8 text-muted-foreground/60" aria-hidden="true" />
      <p class="mt-3 text-sm text-muted-foreground">
        No sessions logged yet. Start a workout to see it here.
      </p>
    </Card>
  {:else}
    <ul class="flex flex-col gap-3">
      {#each sessions as session (session.id)}
        <li>
          <Card class="p-4">
            <a use:link href="/sessions/{session.id}" class="group block">
              <div class="flex items-center justify-between gap-4">
                <div>
                  <p class="font-bold text-card-foreground">{session.programName}</p>
                  <p class="mt-0.5 text-sm text-muted-foreground">
                    {session.programDayName} · {formatLongDate(session.performedOn)}
                  </p>
                </div>
                <p class="text-sm tabular-nums text-muted-foreground">
                  {session.completedSetCount}/{session.setCount} sets
                </p>
              </div>
              {#if (session.exercises ?? []).length > 0}
                <table class="mt-3 w-full text-sm">
                  <tbody>
                    {#each session.exercises ?? [] as ex (ex.exerciseName)}
                      <tr>
                        <td class="py-0.5 pr-4 font-medium text-card-foreground">
                          {ex.exerciseName}
                        </td>
                        <td class="py-0.5 pr-4 tabular-nums text-muted-foreground">
                          {ex.sets}×{ex.reps}
                        </td>
                        <td class="py-0.5 text-right tabular-nums text-muted-foreground">
                          {ex.weightLb} lb
                        </td>
                      </tr>
                    {/each}
                  </tbody>
                </table>
              {/if}
            </a>
            <!-- Outside the card's own link, because an anchor cannot nest
                 inside another one. The whole card still opens the session —
                 that is the thumb-sized target — and this is the smaller,
                 deliberate tap for the recap.

                 Only on a session that is over: a workout still in progress has
                 no story yet, and offering one would invite the lifter to leave
                 the screen they are training on. -->
            {#if session.volumeLb > 0 || session.isOver}
              <div
                class="mt-2 flex items-baseline justify-between gap-4 border-t border-border/60 pt-2"
              >
                <span class="text-xs tabular-nums text-muted-foreground">
                  {#if session.volumeLb > 0}{formatVolume(session.volumeLb)} lb lifted{/if}
                </span>
                {#if session.isOver}
                  <a
                    use:link
                    href="/sessions/{session.id}/recap"
                    class="shrink-0 text-xs font-semibold text-primary hover:underline"
                  >
                    Recap
                  </a>
                {/if}
              </div>
            {/if}
          </Card>
        </li>
      {/each}
    </ul>

    {#if loadMoreFailed}
      <ErrorBanner
        message="Couldn't load more sessions."
        onRetry={loadMore}
        onDismiss={() => (loadMoreFailed = false)}
      />
    {/if}

    {#if hasMore}
      <Button
        variant="outline"
        class="self-center"
        onclick={loadMore}
        disabled={loadingMore}
      >
        <ChevronDown />
        {loadingMore ? "Loading…" : "Load more"}
      </Button>
    {/if}
  {/if}
</div>
