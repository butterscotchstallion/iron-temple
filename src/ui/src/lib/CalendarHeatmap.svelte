<script lang="ts">
  import { activeWeekdays, buildCalendar, todayIso, volumeLevel } from "./calendar";
  import { SHORT_WEEKDAYS } from "./weekday";
  import type { CalendarDay } from "./calendar";

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
    // The weekdays the current program's days are assigned to, if it has a
    // schedule. Sessions alone already collapse the grid; what this adds is the
    // day you were meant to train and didn't, which no session can describe.
    scheduledWeekdays = [],
  }: {
    sessions: { performedOn: string; day: string }[];
    endDate?: string;
    weeks?: number;
    volumes?: Record<string, number> | null;
    scheduledWeekdays?: number[];
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
  // Days past the end of the window the grid is reporting on. The columns run
  // to the Saturday of the last week, so there are nearly always a few, and on
  // a collapsed grid — where an empty cell on a scheduled row reads as a
  // session missed — a day that hasn't come round yet must not read as one.
  const UNREACHED = "bg-muted/20";

  const rows = $derived(
    activeWeekdays(sessions.map((s) => s.performedOn), scheduledWeekdays),
  );
  // Under seven rows the grid is a schedule rather than a calendar, so it gains
  // weekday labels and loses the square cells: two rows of squares across 44
  // columns is a 14px-tall thread, which is not a card.
  const collapsed = $derived(rows.length < 7);
  const rowSet = $derived(new Set(rows));

  // Filtered by index rather than read as week[row], so a week shorter than
  // seven days renders what it has instead of throwing.
  function visible(week: CalendarDay[]): CalendarDay[] {
    return week.filter((_, weekday) => rowSet.has(weekday));
  }

  function level(date: string, count: number): string {
    if (date > endDate) return UNREACHED;
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

<div class="flex w-full gap-1">
  {#if collapsed}
    <!-- mt-4 clears the month-label strip (h-3 + mb-1), so the labels sit
         against their own row of cells. -->
    <div class="mt-4 flex flex-col gap-[2px] text-[9px] text-muted-foreground">
      {#each rows as weekday (weekday)}
        <div class="flex h-3 items-center">{SHORT_WEEKDAYS[weekday]}</div>
      {/each}
    </div>
  {/if}
  <!-- min-w-0 so the month labels, positioned as a % of this box, measure
       against the grid alone and not the labels beside it. -->
  <div class="min-w-0 flex-1">
    <div class="relative mb-1 h-3 text-[9px] text-muted-foreground">
      {#each monthLabels as m (m.pct)}
        <span class="absolute top-0" style="left: {m.pct}%">{m.name}</span>
      {/each}
    </div>
    <div class="flex w-full gap-[2px]">
      {#each grid as week, w (w)}
        <div class="flex flex-1 flex-col gap-[2px]">
          {#each visible(week) as day (day.date)}
            <div
              class="rounded-[2px] {collapsed ? 'h-3' : 'aspect-square'} {level(
                day.date,
                day.count,
              )}"
              title={tooltip(day.date)}
            ></div>
          {/each}
        </div>
      {/each}
    </div>
  </div>
</div>
