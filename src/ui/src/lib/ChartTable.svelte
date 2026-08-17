<script lang="ts">
  // The numbers behind a chart, as a table.
  //
  // Every chart on the Racked page conveys its detail through pointer hover or a
  // title attribute, and neither reaches a keyboard or a screen reader. An
  // aria-label on the SVG can carry the headline but not thirty days of tonnage,
  // so the data itself needs somewhere to live.
  //
  // A <details> rather than a visually-hidden table: it is reachable by keyboard,
  // announced as a disclosure, and useful to sighted readers too — "what exactly
  // was that Tuesday" is a fair question to ask of a chart. Collapsed by default
  // so it stays out of the way of the chart it belongs to.

  let {
    label,
    columns,
    rows,
  }: {
    /** Names the data, not the widget: "Volume by weekday", not "Table". */
    label: string;
    columns: [string, string];
    rows: { label: string; value: string }[];
  } = $props();
</script>

{#if rows.length > 0}
  <details class="mt-2 text-xs">
    <summary
      class="cursor-pointer text-muted-foreground transition hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary"
    >
      {label} as a table
    </summary>
    <table class="mt-2 w-full border-collapse">
      <caption class="sr-only">{label}</caption>
      <thead>
        <tr class="text-left text-muted-foreground">
          <th scope="col" class="border-b border-border/60 py-1 pr-3 font-semibold">
            {columns[0]}
          </th>
          <th scope="col" class="border-b border-border/60 py-1 text-right font-semibold">
            {columns[1]}
          </th>
        </tr>
      </thead>
      <tbody>
        {#each rows as row, i (i)}
          <tr>
            <th scope="row" class="py-1 pr-3 font-normal text-foreground">{row.label}</th>
            <td class="py-1 text-right tabular-nums text-muted-foreground">{row.value}</td>
          </tr>
        {/each}
      </tbody>
    </table>
  </details>
{/if}
