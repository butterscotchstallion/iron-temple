<script lang="ts">
  import { buildCalendar, todayIso } from "./calendar";

  let { dates }: { dates: string[] } = $props();

  const grid = $derived(buildCalendar(dates, todayIso(), 16));
</script>

<div class="flex w-full gap-[3px]">
  {#each grid as week, w (w)}
    <div class="flex flex-1 flex-col gap-[3px]">
      {#each week as day (day.date)}
        <div
          class="h-4 w-full rounded-[2px] {day.count > 0
            ? 'bg-primary'
            : 'bg-muted/40'}"
          title={day.count > 0 ? `${day.date} · trained` : day.date}
        ></div>
      {/each}
    </div>
  {/each}
</div>
