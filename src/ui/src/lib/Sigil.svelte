<script lang="ts">
  import type { House } from "./api";
  import HouseCard from "./HouseCard.svelte";
  import * as HoverCard from "./components/ui/hover-card";

  // The tag a House's members wear beside their name.
  //
  // WHY THIS IS A SPAN AND NOT A LINK
  //
  // <LifterName> is rendered INSIDE an anchor at several call sites — the feed
  // card, the roster row, the notification-adjacent surfaces — and an anchor
  // inside an anchor is invalid HTML that browsers resolve by guessing. So the
  // trigger is a plain span, and the link to the House lives in the card, which is
  // portalled to the body and therefore nested in nothing.
  //
  // That leaves touch without an affordance, because there is no hover on a phone
  // and a span is not tappable. It is a real gap and it is covered elsewhere
  // rather than papered over here: the Houses page lists every House, and a
  // lifter's profile links to theirs. Making this tappable would mean either
  // invalid markup or teaching nine call sites not to wrap the name in a link.
  //
  // The sr-only text is not optional. Without it a screen reader announces four
  // stray letters after a name; the crowns in <LifterName> carry their own labels
  // for the same reason.
  let {
    house,
    class: className = "",
  }: {
    house: House;
    class?: string;
  } = $props();

  // "Member of" rather than "House", because the name is whatever its founder
  // typed: a House called "House Iron" would be announced as "House House Iron",
  // and one called "Iron" would lose the fact that this is a group at all.
  const label = $derived(
    house.tagline
      ? `Member of ${house.name} — ${house.tagline}`
      : `Member of ${house.name}`,
  );
</script>

<HoverCard.Root>
  <HoverCard.Trigger>
    {#snippet child({ props })}
      <span
        {...props}
        class="inline-flex shrink-0 items-center rounded-sm bg-primary/15 px-1 py-px text-[0.65rem] font-semibold tracking-wide text-primary uppercase {className}"
        data-testid="sigil"
      >
        <span aria-hidden="true">{house.sigil}</span>
        <span class="sr-only">{label}</span>
      </span>
    {/snippet}
  </HoverCard.Trigger>
  <HoverCard.Content>
    <HouseCard {house} />
  </HoverCard.Content>
</HoverCard.Root>
