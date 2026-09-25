<script lang="ts">
  import { link } from "svelte-spa-router";
  import type { House } from "./api";
  import HouseIcon from "./HouseIcon.svelte";

  // What a House looks like in a hover card: the icon, the sigil, the name and
  // the tagline, which is exactly the set the site-wide GET /houses carries.
  //
  // That is not a coincidence — the card is built entirely from a list the client
  // already holds, so previewing a House costs no request. The long description is
  // deliberately not here and not fetched: it is the page's, and a popover is the
  // wrong place to start reading prose.
  let { house }: { house: House } = $props();

  const members = $derived(
    house.memberCount === 1 ? "1 member" : `${house.memberCount} members`,
  );
</script>

<div class="flex flex-col gap-2">
  <div class="flex items-center gap-2">
    <HouseIcon {house} size="size-6" />
    <span class="flex min-w-0 flex-col">
      <span class="truncate font-semibold text-foreground">{house.name}</span>
      <span class="text-xs tracking-wide text-muted-foreground">
        {house.sigil} · {members}
      </span>
    </span>
  </div>

  {#if house.tagline}
    <p class="text-sm text-muted-foreground">{house.tagline}</p>
  {/if}

  <!-- The card is portalled to the body, so this anchor is NOT nested inside
       whichever link the name it decorates may itself be sitting in. That is the
       whole reason the sigil is not a link and this is: see Sigil.svelte. -->
  <a
    href={`/houses/${house.id}`}
    use:link
    class="text-sm font-medium text-primary transition hover:text-neon-lift hover:underline focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary"
  >
    View House
  </a>
</div>
