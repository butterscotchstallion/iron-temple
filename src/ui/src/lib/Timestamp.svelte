<script lang="ts">
  import {
    formatLongDate,
    formatLongDateTime,
    relativeDate,
    relativeTime,
  } from "./date";
  import { currentTime, watchClock } from "./now.svelte";

  // A date the way somebody would say it, with the exact one a hover away.
  //
  // "2 days ago" is the friendlier read and the one worth putting on screen, but
  // it is lossy in a way that occasionally matters — "was that before or after I
  // changed the program?" — so the absolute form rides along in the title.
  // Having both is what made it reasonable to use relative time at all; before
  // this, date.ts argued against relative dates precisely because they threw the
  // fact away.
  //
  // WHAT THIS IS NOT FOR. A date that *names* a workout stays absolute: a recap
  // header, a history row, a chart axis. "Workout A · 3 days ago" is a worse
  // title for a page you will open again next week, and reads as a different
  // workout each time. The line is recency versus identity, not timestamp versus
  // date.
  //
  // WHY A NATIVE title AND NOT THE HoverCard COMPONENT. The house pattern for a
  // one-line hover hint is the title attribute (RackedBars, CalendarHeatmap,
  // PlateBar, LifterName). HoverCard wraps its trigger in a button and a portal,
  // which is heavy for eleven characters and breaks inline prose like "took this
  // <Timestamp/>". The cost is real and known: a native title reaches neither
  // keyboard nor screen reader, the same gap ChartTable.svelte documents for the
  // charts. The <time datetime> at least makes the value machine-readable, and
  // this still beats what it replaced, which was a raw ISO string.
  let {
    value,
    kind = "instant",
    class: className = "",
  }: {
    /** An RFC3339 instant, or a YYYY-MM-DD date — see `kind`. */
    value: string | null | undefined;
    /**
     * Which one `value` is. Not guessable at runtime with a straight face, and
     * getting it wrong is the bug this component exists to stop: a date-only
     * string handed to the instant path parses as UTC midnight and reads a day
     * early west of Greenwich. The API's `format:` says which — `date-time` is
     * an instant, `date` is a date.
     */
    kind?: "instant" | "date";
    class?: string;
  } = $props();

  // The clock ticks; this re-renders. Nothing else subscribes on this screen's
  // behalf, so every timestamp holds its own reference and the interval lives
  // exactly as long as one of them is mounted.
  $effect(() => watchClock());

  const relative = $derived.by(() => {
    if (!value) return "";
    const now = new Date(currentTime());
    return kind === "instant"
      ? relativeTime(value, now)
      : relativeDate(value, now);
  });

  // A date-only value has no time of day to show, so its tooltip is just the
  // long date. Inventing midnight would be reporting a precision the server
  // never sent.
  const absolute = $derived(
    !value
      ? ""
      : kind === "instant"
        ? formatLongDateTime(value)
        : formatLongDate(value),
  );
</script>

<!-- Renders nothing at all when there is no date, rather than "Invalid Date" or
     an empty tooltip. Callers own the prose around it, so a field that can be
     absent needs their own {#if} to drop the "Founded"/"Last trained" with it. -->
{#if value}
  <time datetime={value} title={absolute} class={className}>{relative}</time>
{/if}
