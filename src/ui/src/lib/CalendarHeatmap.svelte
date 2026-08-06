<script lang="ts">
  import { buildCalendar, todayIso } from "./calendar";

  let { dates }: { dates: string[] } = $props();

  const grid = $derived(buildCalendar(dates, todayIso(), 16));
</script>

<div class="flex flex-col gap-2">
  <div class="flex gap-[3px] overflow-x-auto pb-1">
    {#each grid as week, w (w)}
      <div class="flex flex-col gap-[3px]">
        {#each week as day (day.date)}
          <div
            class="size-3 shrink-0 rounded-[2px] {day.count > 0
              ? 'bg-primary'
              : 'bg-muted/40'}"
            title={day.count > 0 ? `${day.date} · trained` : day.date}
          ></div>
        {/each}
      </div>
    {/each}
  </div>
  <div class="flex items-center gap-1.5 text-[10px] text-muted-foreground">
    <span>Rest</span>
    <span class="size-2.5 rounded-[2px] bg-muted/40"></span>
    <span class="size-2.5 rounded-[2px] bg-primary"></span>
    <span>Trained</span>
  </div>
</div>
