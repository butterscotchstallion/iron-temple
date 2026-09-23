<script lang="ts">
  import { untrack } from "svelte";
  import { Card } from "$lib/components/ui/card";
  import { Button } from "$lib/components/ui/button";
  import ErrorBanner from "../ErrorBanner.svelte";
  import { setMe } from "../auth.svelte";
  import { updateMe, type User } from "../api";
  import { fieldClass, labelClass } from "./fieldStyles";

  let { me }: { me: User } = $props();

  const COLORS = ["", "#b026ff", "#ff2fb9", "#05d9e8", "#ff6ac1", "#7b2ff7"];

  // Local copies, seeded once when the section mounts. Binding straight to
  // `auth.me` would rewrite the profile as the lifter typed, and leaving the
  // section is how an unwanted edit gets discarded.
  //
  // untracked because capturing the initial value is the intent: a save writes
  // the store back through setMe, and a reactive seed would take the server's
  // answer and overwrite whatever the lifter had typed since.
  let displayName = $state(untrack(() => me.displayName) ?? "");
  let avatarColor = $state(untrack(() => me.avatarColor) ?? "");
  let profileSaving = $state(false);
  let profileError = $state<string | null>(null);
  let profileSaved = $state(false);

  async function saveProfile(event: SubmitEvent) {
    event.preventDefault();
    profileSaving = true;
    profileError = null;
    profileSaved = false;

    const saved = await updateMe({ displayName, avatarColor });
    if (saved.status !== 200) {
      profileError = "Couldn't save your profile.";
    } else {
      setMe(saved.data);
      profileSaved = true;
    }
    profileSaving = false;
  }
</script>

<Card class="p-6">
  <h3 class="text-lg font-bold text-card-foreground">Details</h3>
  <form class="mt-4 flex flex-col gap-4" onsubmit={saveProfile}>
    {#if profileError}
      <ErrorBanner
        message={profileError}
        onDismiss={() => (profileError = null)}
      />
    {/if}

    <label class="flex flex-col gap-1.5">
      <span class={labelClass}>Username</span>
      <input value={me.username} disabled class="{fieldClass} opacity-60" />
      <span class="text-xs text-muted-foreground">
        Your username can't be changed.
      </span>
    </label>

    <label class="flex flex-col gap-1.5">
      <span class={labelClass}>Display name</span>
      <input bind:value={displayName} maxlength="64" required class={fieldClass} />
    </label>

    <fieldset class="flex flex-col gap-2">
      <legend class={labelClass}>Chip colour</legend>
      <div class="flex flex-wrap gap-2">
        {#each COLORS as colour (colour)}
          <label
            class="cursor-pointer rounded-full p-0.5 ring-2 transition {avatarColor ===
            colour
              ? 'ring-primary'
              : 'ring-transparent hover:ring-border'}"
          >
            <input
              type="radio"
              name="avatarColor"
              value={colour}
              bind:group={avatarColor}
              class="sr-only"
            />
            <span
              class="block size-7 rounded-full border border-border/60 text-[0.6rem] leading-7 text-center text-ink"
              style={colour ? `background-color:${colour}` : ""}
            >
              {colour ? "" : "Auto"}
            </span>
          </label>
        {/each}
      </div>
      <span class="text-xs text-muted-foreground">
        The colour behind your initials, for when you haven't uploaded an
        avatar.
      </span>
    </fieldset>

    <div class="flex items-center gap-3">
      <Button type="submit" disabled={profileSaving}>
        {profileSaving ? "Saving…" : "Save"}
      </Button>
      {#if profileSaved}
        <span class="text-sm text-muted-foreground" role="status">Saved.</span>
      {/if}
    </div>
  </form>
</Card>
