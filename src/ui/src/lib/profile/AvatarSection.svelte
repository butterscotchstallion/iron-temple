<script lang="ts">
  import { Card } from "$lib/components/ui/card";
  import { Button } from "$lib/components/ui/button";
  import Trash2 from "@lucide/svelte/icons/trash-2";
  import Upload from "@lucide/svelte/icons/upload";
  import Avatar from "../Avatar.svelte";
  import ErrorBanner from "../ErrorBanner.svelte";
  import { setMe } from "../auth.svelte";
  import { deleteAvatar, uploadAvatar, type User } from "../api";

  // `me` arrives as a prop rather than being read off the store: the route
  // already proved it is non-null to decide it had anything to render, and
  // taking it here means this component has no null branch to draw. Writes
  // still go through setMe, so the store stays the one copy that matters.
  let { me }: { me: User } = $props();

  let avatarBusy = $state(false);
  let avatarError = $state<string | null>(null);
  let fileInput = $state<HTMLInputElement | null>(null);

  async function onFileChosen(event: Event) {
    const input = event.currentTarget as HTMLInputElement;
    const file = input.files?.[0];
    if (!file) return;

    avatarBusy = true;
    avatarError = null;
    const uploaded = await uploadAvatar({ avatar: file });
    if (uploaded.status !== 200) {
      avatarError = "Couldn't upload that image. PNG or JPEG, up to 256 KB.";
    } else {
      // Patch the etag locally so the <img> cache-buster changes and the new
      // picture appears at once, without a round trip to /me.
      setMe({ ...me, hasAvatar: true, avatarEtag: uploaded.data.etag });
    }
    avatarBusy = false;
    // Clear the input so re-picking the same file fires change again.
    input.value = "";
  }

  async function removeAvatar() {
    avatarBusy = true;
    avatarError = null;
    const removed = await deleteAvatar();
    if (removed.status !== 204) {
      avatarError = "Couldn't remove your avatar.";
    } else {
      setMe({ ...me, hasAvatar: false, avatarEtag: undefined });
    }
    avatarBusy = false;
  }
</script>

<Card class="flex flex-col gap-5 p-6">
  <h3 class="text-lg font-bold text-card-foreground">Avatar</h3>
  {#if avatarError}
    <ErrorBanner message={avatarError} onDismiss={() => (avatarError = null)} />
  {/if}
  <div class="flex items-center gap-5">
    <Avatar user={me} size={72} />
    <div class="flex flex-col gap-2">
      <p class="text-sm text-muted-foreground">
        PNG or JPEG, up to 256 KB and 1024×1024. Without one you get an initials
        chip in the colour set under Details.
      </p>
      <div class="flex flex-wrap gap-2">
        <input
          bind:this={fileInput}
          type="file"
          accept="image/png,image/jpeg"
          class="hidden"
          onchange={onFileChosen}
        />
        <Button
          type="button"
          disabled={avatarBusy}
          onclick={() => fileInput?.click()}
        >
          <Upload />
          {avatarBusy ? "Working…" : "Upload image"}
        </Button>
        {#if me.hasAvatar}
          <Button
            type="button"
            variant="secondary"
            disabled={avatarBusy}
            onclick={removeAvatar}
          >
            <Trash2 />
            Remove
          </Button>
        {/if}
      </div>
    </div>
  </div>
</Card>
