<script lang="ts">
  import UserPlus from "@lucide/svelte/icons/user-plus";
  import UserCheck from "@lucide/svelte/icons/user-check";
  import { followLifter, unfollowLifter, type Lifter } from "./api";

  // Follow or unfollow one lifter.
  //
  // Shared by the roster and the profile so the two cannot disagree about what the
  // control says or what it does. Both draw it from the same `following` field, and
  // both have to handle the same three states: not following, following, and
  // mid-press.
  //
  // WHAT IT DOES NOT DO
  //
  // It does not refetch. SessionSocial's reaction toggle deliberately does, because
  // the count beside it belongs to the server and other lifters can have changed it
  // — "guessing the new number would be the only way this could show one the server
  // disagrees with". There is no count here. A follow is a fact about the caller and
  // nobody else can alter it, so the state after a 204 is known exactly and asking
  // again would be a round trip to be told what we already did.
  //
  // It is also not optimistic. Flipping before the 204 would mean flipping back on
  // failure, and a button that changes its mind is worse than one that takes a
  // moment.

  let {
    lifter,
    following,
    onChange,
    size = "default",
  }: {
    /** Who is being followed. Only the id and the name are read. */
    lifter: Pick<Lifter, "id" | "username" | "displayName">;
    /**
     * Whether the caller follows them now.
     *
     * Passed in rather than fetched: it arrives on the row that drew this button,
     * so a request here would be a second answer to a question already answered.
     */
    following: boolean;
    /** Told the new state, so the row that owns the data can keep it. */
    onChange: (following: boolean) => void;
    /** `sm` for a roster row, `default` for the profile header. */
    size?: "sm" | "default";
  } = $props();

  // A re-entrancy guard, not merely a disabled attribute. SessionSocial makes the
  // same distinction and for the same reason: `disabled` is a property of the
  // rendered button and says nothing about a handler already in flight, so a second
  // activation arriving before the first resolves has to be turned away here.
  let saving = $state(false);
  let failed = $state(false);

  const name = $derived(lifter.displayName || lifter.username);

  async function toggle() {
    if (saving) return;
    saving = true;
    failed = false;

    const result = following
      ? await unfollowLifter(lifter.id)
      : await followLifter(lifter.id);

    if (result.status === 204) {
      onChange(!following);
    } else {
      failed = true;
    }
    saving = false;
  }
</script>

<div class="flex shrink-0 flex-col items-end gap-1">
  <button
    type="button"
    onclick={toggle}
    aria-pressed={following}
    aria-busy={saving}
    aria-label={saving
      ? `Saving`
      : following
        ? `Unfollow ${name}`
        : `Follow ${name}`}
    disabled={saving}
    class="inline-flex items-center gap-1.5 rounded-full border font-semibold transition focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary disabled:opacity-60 {size ===
    'sm'
      ? 'px-2.5 py-1 text-xs'
      : 'px-3.5 py-1.5 text-sm'} {following
      ? 'border-primary bg-primary/15 text-foreground'
      : 'border-border/60 text-muted-foreground hover:bg-white/5 hover:text-foreground'}"
  >
    {#if following}
      <UserCheck class={size === "sm" ? "size-3" : "size-4"} aria-hidden="true" />
      Following
    {:else}
      <UserPlus class={size === "sm" ? "size-3" : "size-4"} aria-hidden="true" />
      Follow
    {/if}
  </button>
  {#if failed}
    <!-- role="status" so a screen reader hears it, and terse for the reason
         SessionSocial's is: the lifter can see which button they pressed. -->
    <p class="text-xs text-destructive" role="status">Couldn't save that.</p>
  {/if}
</div>
