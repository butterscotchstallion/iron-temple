<script lang="ts">
  import { onMount } from "svelte";
  import { Card } from "$lib/components/ui/card";
  import Avatar from "../lib/Avatar.svelte";
  import ErrorCard from "../lib/ErrorCard.svelte";
  import Loading from "../lib/skeleton/Loading.svelte";
  import Skeleton from "../lib/skeleton/Skeleton.svelte";
  import SkeletonRows from "../lib/skeleton/SkeletonRows.svelte";
  import { auth } from "../lib/auth.svelte";
  import { formatPercent } from "../lib/racked";
  import { formatVolume } from "../lib/volume";
  import {
    getLeaderboard,
    type Leaderboard,
    type LeaderboardBoard,
    type LeaderboardEntry,
  } from "../lib/api";

  // How the lifters on this install compare.
  //
  // Every board arrives in one response, so switching between them costs nothing
  // and the period switcher is the only control that refetches. That is the
  // server's doing — one pass of the Racked report per lifter computes every
  // figure, so a per-metric request would re-run the heaviest query in the API to
  // read a different field off the same result.
  //
  // The board order is the server's opinion and is not re-sorted here: measures
  // relative to the lifter lead, and raw tonnage is last. `selected` starts at 0
  // for exactly that reason — the page opens on the fairest board rather than on
  // the one people ask for first.
  type Period = "week" | "month" | "year";

  let period = $state<Period>("month");
  let data = $state<Leaderboard | null>(null);
  let loading = $state(true);
  let failed = $state(false);
  let selected = $state(0);

  // One chip per board. Five, because buildBoards returns exactly five —
  // getting this wrong is a row of chips that wraps or unwraps as they land.
  const SKELETON_BOARDS = [0, 1, 2, 3, 4];

  async function load() {
    failed = false;
    loading = true;
    const result = await getLeaderboard({ period });
    if (result.status !== 200) {
      failed = true;
    } else {
      data = result.data;
      // Clamp rather than reset: switching period keeps the board you were
      // reading, which is the whole point of having a period switcher.
      if (selected >= result.data.boards.length) selected = 0;
    }
    loading = false;
  }

  async function setPeriod(next: Period) {
    if (next === period) return;
    period = next;
    await load();
  }

  const boards = $derived(data?.boards ?? []);
  const board = $derived<LeaderboardBoard | null>(boards[selected] ?? null);

  // A leaderboard of one is not a leaderboard — same call the feed card makes when
  // it renders nothing rather than an empty state.
  //
  // Counted across EVERY board rather than off the first one. The first board does
  // list every lifter today, because sessions-a-week is defined for everybody, but
  // leaning on that would make this silently wrong the day the boards are
  // reordered or a board is added ahead of it. Distinct ids over all of them needs
  // no assumption about which board is which.
  const population = $derived(
    new Set(boards.flatMap((b) => b.entries.map((e) => e.lifter.id))).size,
  );
  const alone = $derived(boards.length > 0 && population <= 1);

  function format(entry: LeaderboardEntry, unit: LeaderboardBoard["unit"]): string {
    switch (unit) {
      case "percent":
        // Fractions on the wire — 0.12 is 12% — and formatPercent is the same
        // renderer the Racked page uses.
        return formatPercent(entry.value);
      case "per_week":
        // One decimal: "2.5 a week" is the real answer where a rounded "3" would
        // claim a session that did not happen.
        return `${entry.value.toFixed(1)}×`;
      case "pounds":
        return `${formatVolume(entry.value)} lb`;
      case "count":
        return String(Math.round(entry.value));
    }
  }

  // Medals for the top three, and only when they are not a tie for last place.
  function medal(rank: number): string {
    return rank === 1 ? "🥇" : rank === 2 ? "🥈" : rank === 3 ? "🥉" : "";
  }

  const periods: Period[] = ["week", "month", "year"];

  onMount(load);
</script>

<div class="flex flex-col gap-4">
  <div>
    <h2 class="text-2xl font-black text-foreground">Leaderboard</h2>
    <p class="mt-1 text-sm text-muted-foreground">
      How everyone on this install has been training.
    </p>
  </div>

  <!-- Period. The only control that costs a request. -->
  <div class="flex justify-center">
    <div
      class="inline-flex items-center gap-1 rounded-full border border-border/60 bg-card/40 p-1"
    >
      {#each periods as option (option)}
        <button
          type="button"
          onclick={() => setPeriod(option)}
          aria-current={period === option ? "true" : undefined}
          class="rounded-full px-4 py-1.5 text-xs font-semibold uppercase tracking-[0.2em] transition {period ===
          option
            ? 'bg-primary text-primary-foreground'
            : 'text-muted-foreground hover:text-foreground'}"
        >
          {option}
        </button>
      {/each}
    </div>
  </div>

  {#if loading}
    <!-- The metric chips go too, and they matter: they sit between the period
         switcher and the board, so leaving them out let the whole board slide
         up a row and back down again on every period change. -->
    <Loading label="Loading the leaderboard">
      <div class="flex flex-wrap justify-center gap-1.5">
        {#each SKELETON_BOARDS as board (board)}
          <Skeleton class="h-[26px] w-24 rounded-full" />
        {/each}
      </div>
      <Card class="p-4">
        <SkeletonRows rows={3} avatar="size-8" rowClass="py-2.5" trailing />
        <!-- The board's own footnote, which every board carries. -->
        <div class="mt-3 border-t border-border/60 pt-3">
          <Skeleton text="xs" class="w-3/4" />
        </div>
      </Card>
    </Loading>
  {:else if failed}
    <ErrorCard message="Couldn't load the leaderboard." onRetry={load} />
  {:else if alone}
    <!-- One lifter. Said plainly, and without pretending they have won anything. -->
    <Card class="p-8 text-center">
      <p class="text-sm text-muted-foreground">
        You're the only lifter on this install, so there's nothing to compare yet.
      </p>
    </Card>
  {:else if board}
    <!-- Boards. No request to switch — they all arrived together. -->
    <div class="flex flex-wrap justify-center gap-1.5">
      {#each boards as option, i (option.metric)}
        <button
          type="button"
          onclick={() => (selected = i)}
          aria-current={selected === i ? "true" : undefined}
          class="rounded-full border px-3 py-1 text-xs font-semibold transition {selected === i
            ? 'border-primary bg-primary/15 text-foreground'
            : 'border-border/60 text-muted-foreground hover:text-foreground'}"
        >
          {option.label}
        </button>
      {/each}
    </div>

    <Card class="p-4" data-testid="leaderboard-board">
      {#if board.entries.length === 0}
        <!-- Attendance does this: a board can list nobody when the metric is
             undefined for everyone, and its own note explains why. -->
        <p class="py-4 text-center text-sm text-muted-foreground">
          Nothing to show on this board yet.
        </p>
      {:else}
        <ol class="flex flex-col divide-y divide-border/60">
          {#each board.entries as entry (entry.lifter.id)}
            <li class="flex items-center gap-3 py-2.5">
              <span
                class="w-8 shrink-0 text-center text-sm font-bold tabular-nums text-muted-foreground"
              >
                {medal(entry.rank) || entry.rank}
              </span>
              <Avatar user={entry.lifter} size={32} />
              <span class="flex min-w-0 flex-1 flex-col">
                <span class="flex items-baseline gap-2">
                  <span class="truncate text-sm font-semibold text-foreground">
                    {entry.lifter.displayName || entry.lifter.username}
                  </span>
                  {#if entry.lifter.id === auth.me?.id}
                    <span class="shrink-0 text-xs text-muted-foreground">(you)</span>
                  {/if}
                </span>
                {#if entry.detail}
                  <!-- The lift behind a most-improved figure. -->
                  <span class="truncate text-xs text-muted-foreground">{entry.detail}</span>
                {/if}
              </span>
              <span class="shrink-0 text-sm font-bold tabular-nums text-foreground">
                {format(entry, board.unit)}
              </span>
            </li>
          {/each}
        </ol>
      {/if}

      <!-- The board's own words. On the wire because a board that omits lifters —
           attendance does — has to say so where it is drawn, and because the
           volume board's note is the one place the page admits it is a poor
           contest. -->
      <p class="mt-3 border-t border-border/60 pt-3 text-xs text-muted-foreground">
        {board.note}
      </p>
    </Card>
  {/if}
</div>
