<script lang="ts">
  import Crown from "@lucide/svelte/icons/crown";
  import { crownsFor } from "./achievements.svelte";
  import { houseFor } from "./houses.svelte";
  import Sigil from "./Sigil.svelte";
  import type { User } from "./api";

  // A lifter's name, with whatever they are currently wearing after it.
  //
  // The name half of what <Avatar> already does for the picture, and it exists
  // for the same reason: `displayName || username` was written out at ten call
  // sites, so the fallback for an account with no display name was ten chances
  // to disagree. Crowns made that a real problem rather than a tidiness one —
  // every one of those sites has to draw them, and none of them should have to
  // know how.
  //
  // The same narrow Pick as <Avatar>, and for its reason: two different account
  // shapes reach this — `User` for the signed-in lifter, `Lifter` for anybody
  // else — and they agree on exactly these two. Derived from `User` rather than
  // written out so the field types stay tied to the generated client.
  type NamedUser = Pick<User, "id" | "username" | "displayName">;

  let {
    lifter,
    class: className = "",
    crownSize = "size-3.5",
    crowns: showCrowns = true,
    sigil: showSigil = true,
  }: {
    lifter: NamedUser;
    class?: string;
    /**
     * How big the crowns are. Rows differ a lot — a 56px profile header and a
     * 28px notification row both use this — and a crown sized for one is either
     * lost or overbearing in the other.
     */
    crownSize?: string;
    /**
     * Whether to draw them at all.
     *
     * For a surface that is ITSELF about a crown — the achievement dialog — where
     * decorating the name would repeat the heading above it and, on a lifter who
     * leads three boards, add two crowns the card is not talking about. The name's
     * `displayName || username` fallback is still worth centralising there, which
     * is why that surface uses this component rather than the raw expression.
     */
    crowns?: boolean;
    /**
     * Whether to draw the House sigil.
     *
     * Off for the same kind of surface `crowns` is off for: one that is itself
     * about the lifter's House, where the tag beside the name would repeat what
     * the heading already says.
     */
    sigil?: boolean;
  } = $props();

  const name = $derived(lifter.displayName || lifter.username);

  // ONE CROWN PER BOARD LED, not one crown per lifter. Somebody who is top of
  // three boards wears three, which is the whole information content of the
  // ornament — a single crown would say "leading something" and leave the reader
  // no way to tell a runaway from a narrow win on one metric.
  //
  // Read synchronously from a map that is built once per load, so a feed of
  // thirty names costs thirty lookups rather than thirty scans. See
  // achievements.svelte.ts.
  const crowns = $derived(showCrowns ? crownsFor(lifter.id) : []);

  // AT MOST ONE, unlike the crowns, and that is the schema's doing rather than a
  // choice made here: house_members takes the lifter as its primary key, so a
  // lifter in two Houses is a row the database cannot hold. Read from the same
  // kind of map and for the same reason — see houses.svelte.ts.
  const house = $derived(showSigil ? houseFor(lifter.id) : null);
</script>

<span class="inline-flex min-w-0 items-baseline gap-1">
  <span class="truncate {className}">{name}</span>
  <!-- Before the crowns, because it says who they are rather than what they have
       won — and because it is the one ornament whose width is fixed, so a row that
       is truncating stays readable in the same place every time. -->
  {#if house}
    <Sigil {house} />
  {/if}
  {#if crowns.length > 0}
    <!-- shrink-0 so the crowns survive a row that is truncating the name: the
         name can be cut and still read, where half a crown is just noise. -->
    <span class="inline-flex shrink-0 items-center gap-0.5">
      {#each crowns as crown (crown.slug)}
        <!-- Each crown carries its OWN label rather than the group carrying one
             list, because each is a separate claim: a reader hovering the second
             of three wants to know which board that one is. `title` for a mouse
             and the visually-hidden span for a screen reader — the icon itself
             stays aria-hidden, as every other icon in this app does. -->
        <span class="inline-flex items-center" title={crown.label}>
          <Crown class="{crownSize} text-primary" aria-hidden="true" />
          <span class="sr-only">{crown.label}</span>
        </span>
      {/each}
    </span>
  {/if}
</span>
