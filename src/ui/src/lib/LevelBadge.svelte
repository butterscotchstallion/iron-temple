<script lang="ts">
  import type { LifterLevel } from "./api";
  import LevelCard from "./LevelCard.svelte";
  import * as HoverCard from "./components/ui/hover-card";

  // How much a lifter has trained, as a number worn beside their name.
  //
  // THE NUMBER ALONE, with no "Lv" in front of it. Beside a name it sits next to
  // the House sigil, which is four letters of the lifter's own choosing, and two
  // word-shaped tags in a row read as one thing with a gap in it. The word belongs
  // in the label, which is where a reader who does not recognise the badge looks.
  //
  // WHY IT DOES NOT LOOK LIKE THE SIGIL
  //
  // Outlined and round, where the sigil is filled and square. The two are adjacent
  // on every name that has both, and given the same treatment they read as one
  // two-part tag rather than as two claims — one about who somebody trains with,
  // one about how long they have been at it.
  //
  // WHY IT IS A SPAN AND NOT A LINK
  //
  // Sigil.svelte's reason exactly: <LifterName> renders inside an anchor at the
  // feed card and the roster row, and an anchor inside an anchor is invalid HTML
  // that browsers resolve by guessing. The card below is portalled to the body and
  // so is nested in nothing — but it is read-only on purpose, and must stay that
  // way for the same reason the sigil's card puts its one link there and not here.
  //
  // The sr-only text is not optional. Without it a screen reader announces a bare
  // digit after a name, which is worse than the stray letters Sigil.svelte warns
  // about — "Grace Hopper 12" reads as part of the name.
  let {
    level,
    card = false,
    suppressed = false,
  }: {
    level: LifterLevel;
    /**
     * Whether hovering opens the full card instead of a plain tooltip.
     *
     * Off everywhere but the header. A level beside somebody else's name is a
     * fact about them and the number says all of it, and a card on every row of
     * the feed would be thirty popovers waiting to happen. A reader who wants the
     * figures behind the number has somewhere to go for them: the Experience
     * section of that lifter's profile, which the name itself links to.
     */
    card?: boolean;
    /**
     * Hold the card shut while something else owns the screen.
     *
     * The header is the only caller, and it needs this because its badge sits
     * inside the account button: hovering would open this card, clicking opens the
     * account menu, and bits-ui does not coordinate the two — they would render on
     * top of each other. The menu wins, because the menu is what was asked for.
     */
    suppressed?: boolean;
  } = $props();

  const label = $derived(`Level ${level.level}`);
</script>

{#snippet chip(props: Record<string, unknown>)}
  <span
    {...props}
    class="inline-flex shrink-0 items-center rounded-full border border-primary/40 px-1.5 py-px text-[0.65rem] font-bold text-primary tabular-nums"
    data-testid="level-badge"
  >
    <span aria-hidden="true">{level.level}</span>
    <span class="sr-only">{label}</span>
  </span>
{/snippet}

{#if card && !suppressed}
  <HoverCard.Root>
    <HoverCard.Trigger>
      <!-- The child snippet rather than the default trigger, which renders an
           anchor: see Sigil.svelte. This one is never inside a link, but it IS
           inside a button, and a button in a button is the same invalid nesting. -->
      {#snippet child({ props })}
        {@render chip(props)}
      {/snippet}
    </HoverCard.Trigger>
    <HoverCard.Content>
      <LevelCard {level} />
    </HoverCard.Content>
  </HoverCard.Root>
{:else}
  <!-- `title` only here. On the card variant the browser's own tooltip would fire
       a second later and sit on top of the card saying less. -->
  {@render chip(card ? {} : { title: label })}
{/if}
