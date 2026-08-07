<script lang="ts">
  import { buildCalendar, todayIso } from "./calendar";

  let { sessions }: { sessions: { performedOn: string; day: string }[] } =
    $props();

  const WEEKS = 52; // a year, like GitHub

  const MONTHS = [
    "Jan", "Feb", "Mar", "Apr", "May", "Jun",
    "Jul", "Aug", "Sep", "Oct", "Nov", "Dec",
  ];

  const grid = $derived(
    buildCalendar(
      sessions.map((s) => s.performedOn),
      todayIso(),
      WEEKS,
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

  // Month labels at the first week each month appears, positioned as a % of
  // width (columns flex to fill, so px offsets won't do).
  const monthLabels = $derived.by(() => {
    const out: { name: string; pct: number }[] = [];
    let last = -1;
    grid.forEach((week, i) => {
      const month = Number(week[0].date.split("-")[1]) - 1;
      if (month !== last) {
        out.push({ name: MONTHS[month], pct: (i / WEEKS) * 100 });
        last = month;
      }
    });
    return out;
  });

  function level(count: number): string {
    if (count <= 0) return "bg-muted/40";
    if (count === 1) return "bg-primary/50";
    if (count === 2) return "bg-primary/75";
    return "bg-primary";
  }

  function tooltip(date: string): string {
    const labels = daysByDate.get(date);
    return labels?.length ? `${date} · ${labels.join(", ")}` : date;
  }
</script>

<div class="w-full">
  <div class="relative mb-1 h-3 text-[9px] text-muted-foreground">
    {#each monthLabels as m (m.pct)}
      <span class="absolute top-0" style="left: {m.pct}%">{m.name}</span>
    {/each}
  </div>
  <div class="flex w-full gap-[2px]">
    {#each grid as week, w (w)}
      <div class="flex flex-1 flex-col gap-[2px]">
        {#each week as day (day.date)}
          <div
            class="aspect-square rounded-[2px] {level(day.count)}"
            title={tooltip(day.date)}
          ></div>
        {/each}
      </div>
    {/each}
  </div>
</div>
