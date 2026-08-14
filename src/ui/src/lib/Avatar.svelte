<script lang="ts">
  import { avatarColor, avatarUrl, initials } from "./userAvatar";
  import type { User } from "./api";

  // Renders a user's avatar: their uploaded image, or an initials chip on a
  // colour derived from their id. The chip is not a placeholder for a missing
  // image — it is the default, so most users never need to upload anything.
  let {
    user,
    size = 32,
    class: className = "",
  }: { user: User; size?: number; class?: string } = $props();

  // An upload can 404 if it was removed in another tab. Falling back to the
  // chip beats a broken-image icon.
  let imageFailed = $state(false);

  // Reset when the image changes, or a single failure would pin the chip for
  // the rest of the session.
  let src = $derived(avatarUrl(user.id, user.avatarEtag));
  $effect(() => {
    void src;
    imageFailed = false;
  });

  let showImage = $derived(user.hasAvatar && !imageFailed);
</script>

{#if showImage}
  <img
    {src}
    alt=""
    width={size}
    height={size}
    style="width:{size}px;height:{size}px"
    class="shrink-0 rounded-full object-cover ring-1 ring-border/60 {className}"
    onerror={() => (imageFailed = true)}
  />
{:else}
  <!-- aria-hidden with the name adjacent: the display name is always rendered
       next to this in the header and profile, so announcing initials too would
       just repeat it. -->
  <span
    aria-hidden="true"
    style="width:{size}px;height:{size}px;background-color:{avatarColor(
      user.id,
      user.avatarColor,
    )};font-size:{Math.round(size * 0.4)}px"
    class="inline-flex shrink-0 select-none items-center justify-center rounded-full font-bold uppercase leading-none text-void ring-1 ring-border/60 {className}"
  >
    {initials(user.displayName || user.username)}
  </span>
{/if}
