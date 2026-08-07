<script lang="ts">
  import { buildCalendar, todayIso } from "./calendar";

  let { sessions }: { sessions: { performedOn: string; day: string }[] } =
    $props();

  const WEEKS = 18;
  const STEP = 15; // 12px cell + 3px gap

  const MONTHS = [
    "Jan", "Feb", "Mar", "Apr", "May", "Jun",
    "Jul", "Aug", "Sep", "Oct", "Nov", "Dec",
  ];
  // Rows are Sunday → Saturday; GitHub labels Mon/Wed/Fri.
  const DAY_LABELS = ["", "Mon", "", "Wed", "", "Fri", ""];

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

  // Month labels positioned at the first week each month appears.
  const monthLabels = $derived.by(() => {
    const out: { name: string; x: number }[] = [];
    let last = -1;
    grid.forEach((week, i) => {
      const month = Number(week[0].date.split("-")[1]) - 1;
      if (month !== last) {
        out.push({ name: MONTHS[month], x: i * STEP });
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

<div class="flex gap-1.5 text-[9px] text-muted-foreground">
  <!-- day-of-week labels -->
  <div class="flex flex-col gap-[3px] pt-4">
    {#each DAY_LABELS as label, i (i)}
      <div class="flex h-3 items-center leading-none">{label}</div>
    {/each}
  </div>

  <div class="overflow-x-auto">
    <!-- month labels -->
    <div class="relative mb-1 h-3" style="width: {WEEKS * STEP}px">
      {#each monthLabels as m (m.x)}
        <span class="absolute top-0" style="left: {m.x}px">{m.name}</span>
      {/each}
    </div>
    <!-- week columns -->
    <div class="flex gap-[3px]">
      {#each grid as week, w (w)}
        <div class="flex flex-col gap-[3px]">
          {#each week as day (day.date)}
            <div
              class="size-3 rounded-[2px] {level(day.count)}"
              title={tooltip(day.date)}
            ></div>
          {/each}
        </div>
      {/each}
    </div>
  </div>
</div>
