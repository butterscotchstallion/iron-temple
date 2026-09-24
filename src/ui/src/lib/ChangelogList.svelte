<script lang="ts">
  // One release's notes, rendered as a list. Shared by the two places that show
  // them: the header's changelog panel (VersionChangelog.svelte), which describes
  // the build you're running, and the update prompt (UpdatePrompt.svelte), which
  // describes the one being offered. Same data, same shape, one definition.
  //
  // No spacing of its own — the two callers sit in different layouts (a plain
  // panel, and a `grid gap-6` dialog) and a margin baked in here would be right
  // in one and doubled in the other.

  let {
    entries,
    class: className = "",
  }: {
    entries: string[];
    class?: string;
  } = $props();
</script>

<ul class="space-y-2 text-sm text-ink/80 {className}">
  {#each entries as entry}
    <li class="flex gap-2">
      <!-- Decorative: a screen reader should read the entry, not a chevron per
           line. The list markup already conveys that these are separate items. -->
      <span class="select-none text-primary/70" aria-hidden="true">&rsaquo;</span>
      <!-- break-words because the dialog is max-w-xs on a phone and entries carry
           `feat(ui):` and `(abc1234)` — tokens with nowhere obvious to wrap. -->
      <span class="break-words">{entry}</span>
    </li>
  {/each}
</ul>
