<script lang="ts">
  import { buildCalendar, todayIso } from "./calendar";

  let { dates }: { dates: string[] } = $props();

  const grid = $derived(buildCalendar(dates, todayIso(), 16));
</script>

<div class="flex justify-center gap-[3px] overflow-x-auto">
  {#each grid as week, w (w)}
    <div class="flex flex-col gap-[3px]">
      {#each week as day (day.date)}
        <div
          class="size-4 shrink-0 rounded-[2px] {day.count > 0
            ? 'bg-primary'
            : 'bg-muted/40'}"
          title={day.count > 0 ? `${day.date} · trained` : day.date}
        ></div>
      {/each}
    </div>
  {/each}
</div>
