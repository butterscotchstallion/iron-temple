<script lang="ts">
  import { buildCalendar, todayIso, volumeLevel } from "./calendar";

  let {
    sessions,
    // Home shows the run-up to today; the Racked recap shows one closed period,
    // which may have ended months ago and is not 44 weeks long. Both defaults
    // match the original behaviour, so Home passes neither.
    endDate = todayIso(),
    weeks = 44, // ~10 months; fewer columns = slightly larger cells
    // Per-day tonnage. When given, cells shade by how much was moved rather
    // than by how many sessions were logged — on a program that trains once a
    // day, a session count only ever has two states and the grid says little.
    volumes = null,
  }: {
    sessions: { performedOn: string; day: string }[];
    endDate?: string;
    weeks?: number;
    volumes?: Record<string, number> | null;
  } = $props();

  const WEEKS = $derived(weeks);

  const MONTHS = [
    "Jan", "Feb", "Mar", "Apr", "May", "Jun",
    "Jul", "Aug", "Sep", "Oct", "Nov", "Dec",
  ];

  const grid = $derived(
    buildCalendar(
      sessions.map((s) => s.performedOn),
      endDate,
      WEEKS,
    ),
  );

  const maxVolume = $derived(volumes ? Math.max(0, ...Object.values(volumes)) : 0);

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

  const SHADES = ["bg-muted/40", "bg-primary/50", "bg-primary/75", "bg-primary"];

  function level(date: string, count: number): string {
    const step = volumes
      ? volumeLevel(volumes[date] ?? 0, maxVolume)
      : Math.min(count, 3);
    return SHADES[step];
  }

  function tooltip(date: string): string {
    const labels = daysByDate.get(date);
    const base = labels?.length ? `${date} · ${labels.join(", ")}` : date;
    const volume = volumes?.[date];
    return volume ? `${base} · ${Math.round(volume).toLocaleString("en-US")} lb` : base;
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
            class="aspect-square rounded-[2px] {level(day.date, day.count)}"
            title={tooltip(day.date)}
          ></div>
        {/each}
      </div>
    {/each}
  </div>
</div>
