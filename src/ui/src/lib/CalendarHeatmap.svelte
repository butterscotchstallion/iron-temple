<script lang="ts">
  import { buildCalendar, todayIso } from "./calendar";

  let { sessions }: { sessions: { performedOn: string; day: string }[] } =
    $props();

  const grid = $derived(
    buildCalendar(
      sessions.map((s) => s.performedOn),
      todayIso(),
      16,
    ),
  );

  // Workout types performed on each date, for the tooltip.
  const daysByDate = $derived.by(() => {
    const map = new Map<string, string[]>();
    for (const s of sessions) {
      const list = map.get(s.performedOn) ?? [];
      list.push(s.day);
      map.set(s.performedOn, list);
    }
    return map;
  });

  function tooltip(date: string): string {
    const labels = daysByDate.get(date);
    return labels?.length ? `${date} · ${labels.join(", ")}` : date;
  }
</script>

<div class="flex w-full justify-between">
  {#each grid as week, w (w)}
    <div class="flex flex-col gap-[3px]">
      {#each week as day (day.date)}
        <div
          class="size-4 rounded-[2px] {day.count > 0
            ? 'bg-primary'
            : 'bg-muted/40'}"
          title={tooltip(day.date)}
        ></div>
      {/each}
    </div>
  {/each}
</div>
