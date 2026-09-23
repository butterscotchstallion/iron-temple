<script lang="ts">
  import ChevronUp from "@lucide/svelte/icons/chevron-up";
  import ChevronDown from "@lucide/svelte/icons/chevron-down";

  // Two buttons that move a row one place.
  //
  // Not drag-and-drop, and not for want of a library. There is no dragging
  // anywhere in this app; App.svelte actively budgets the entry chunk (it defers
  // bits-ui because the dropdown and popover cost a third of it); and a drag
  // target is a poor interaction on a phone held one-handed at a rack, which is
  // where this app is used. Two buttons are reachable by thumb, work with a
  // screen reader without an announcer, and need nothing downloaded.
  //
  // The label names the row so the accessible name is "Move Squat up" rather
  // than "Move up" repeated down the list, which is unusable when the buttons
  // are the only thing a screen reader reads out.
  let {
    label,
    isFirst,
    isLast,
    busy = false,
    onUp,
    onDown,
  }: {
    label: string;
    isFirst: boolean;
    isLast: boolean;
    /**
     * A reorder is already in flight.
     *
     * Moving a row is a round trip whose response redraws the whole list, so a
     * second tap before the first lands is a move against positions that are
     * about to change underneath it. Separate from isFirst/isLast because those
     * say "this row cannot go that way" and this says "not yet" — and because
     * aria-busy has to go on the buttons that are actually working.
     */
    busy?: boolean;
    onUp: () => void;
    onDown: () => void;
  } = $props();

  const buttonClass =
    "rounded-md p-1 text-muted-foreground transition hover:text-foreground disabled:opacity-30 disabled:hover:text-muted-foreground";
</script>

<div class="flex shrink-0 flex-col" aria-busy={busy}>
  <button
    type="button"
    class={buttonClass}
    aria-label="Move {label} up"
    disabled={isFirst || busy}
    onclick={onUp}
  >
    <ChevronUp class="size-4" aria-hidden="true" />
  </button>
  <button
    type="button"
    class={buttonClass}
    aria-label="Move {label} down"
    disabled={isLast || busy}
    onclick={onDown}
  >
    <ChevronDown class="size-4" aria-hidden="true" />
  </button>
</div>
